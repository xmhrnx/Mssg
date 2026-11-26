// Package messenger is the app's engine: it owns the identity, the
// libp2p node and the store, and sends and receives messages, receipts,
// profiles, call signaling and files with contacts. Messages to offline
// contacts wait in an outbox and go out once both are online.
package messenger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/xmhrnx/mssg/internal/p2p"
	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

// InviteProtocol is the one protocol strangers may reach: redeeming an
// invite code.
const InviteProtocol = "/mssg/invite/1.0.0"

// Events receive what happens, on the service's goroutines.
type Events struct {
	ContactsChanged func()
	// Message reports a new or changed message.
	Message  func(m store.Message)
	Presence func(peer string, online bool)
	Typing   func(peer string)
	Call     func(peer string, c wire.Call)
	// InviteRedeemed reports a contact who redeemed this device's code.
	InviteRedeemed func(c store.Contact)
	// Incoming reports a message from a contact, for notifications.
	Incoming func(c store.Contact, m store.Message)
	Logf     func(format string, args ...any)
}

// Options configure the service.
type Options struct {
	Store   *store.Store
	DataDir string
	// DHTServer makes this node serve the DHT (desktops).
	DHTServer bool
	Events    Events
	// Node overrides the libp2p configuration (tests).
	Node *p2p.Config
}

// Service is the running messenger.
type Service struct {
	st     *store.Store
	node   *p2p.Node
	ev     Events
	attDir string

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	contacts  map[peer.ID]bool
	profile   store.Profile
	settings  store.Settings
	active    string // the open chat
	delivery  map[string]bool
	backoff   map[string]time.Time
	downloads map[string]bool
	typingAt  map[string]time.Time

	kick chan struct{}
}

// New opens the identity (creating one on first run) and starts the node.
func New(ctx context.Context, opt Options) (*Service, error) {
	priv, err := loadIdentity(opt.Store)
	if err != nil {
		return nil, err
	}
	prof, err := opt.Store.Profile()
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	set, err := opt.Store.Settings()
	if err != nil {
		return nil, err
	}
	s := &Service{
		st:        opt.Store,
		ev:        opt.Events,
		attDir:    filepath.Join(opt.DataDir, "attachments"),
		contacts:  map[peer.ID]bool{},
		profile:   prof,
		settings:  set,
		delivery:  map[string]bool{},
		backoff:   map[string]time.Time{},
		downloads: map[string]bool{},
		typingAt:  map[string]time.Time{},
		kick:      make(chan struct{}, 1),
	}
	if s.ev.Logf == nil {
		s.ev.Logf = func(string, ...any) {}
	}
	if err := os.MkdirAll(s.attDir, 0o700); err != nil {
		return nil, fmt.Errorf("attachments folder: %w", err)
	}
	cs, err := s.st.Contacts()
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if id, err := peer.Decode(c.PeerID); err == nil {
			s.contacts[id] = true
		}
	}
	cfg := p2p.Config{}
	if opt.Node != nil {
		cfg = *opt.Node
	}
	cfg.PrivateKey = priv
	cfg.DHTServer = opt.DHTServer
	cfg.IsContact = s.isContact
	cfg.OnConnectedness = s.onConnectedness
	cfg.Logf = s.ev.Logf
	s.node, err = p2p.Start(ctx, cfg)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if id, err := peer.Decode(c.PeerID); err == nil {
			s.node.AddAddrs(id, c.Addrs)
		}
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.node.SetHandler(wire.MsgProtocol, s.handleMsgStream)
	s.node.SetHandler(wire.FileProtocol, s.handleFileStream)
	s.node.SetOpenHandler(InviteProtocol, s.handleInviteStream)
	go s.outboxLoop()
	return s, nil
}

func loadIdentity(st *store.Store) (crypto.PrivKey, error) {
	raw, err := st.Identity()
	if err == nil {
		return crypto.UnmarshalPrivateKey(raw)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	raw, err = crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return priv, st.SaveIdentity(raw)
}

// Close stops the service.
func (s *Service) Close() error {
	s.cancel()
	return s.node.Close()
}

// ID is this device's permanent peer ID.
func (s *Service) ID() string { return s.node.ID().String() }

// Store exposes the database for reads.
func (s *Service) Store() *store.Store { return s.st }

// Profile returns this device's profile.
func (s *Service) Profile() store.Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profile
}

// Settings returns the current settings.
func (s *Service) Settings() store.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// SaveSettings stores new settings.
func (s *Service) SaveSettings(set store.Settings) error {
	if err := s.st.SaveSettings(set); err != nil {
		return err
	}
	s.mu.Lock()
	s.settings = set
	s.mu.Unlock()
	return nil
}

// SetProfile saves the profile and sends it to every contact.
func (s *Service) SetProfile(name string, avatar []byte) error {
	s.mu.Lock()
	p := store.Profile{Name: name, Avatar: avatar, Rev: time.Now().UnixMilli()}
	s.profile = p
	s.mu.Unlock()
	if err := s.st.SaveProfile(p); err != nil {
		return err
	}
	cs, err := s.st.Contacts()
	if err != nil {
		return err
	}
	for _, c := range cs {
		if c.Confirmed {
			s.queue("profile:"+c.PeerID, c.PeerID, wire.Frame{Type: wire.TProfile, Profile: s.wireProfile()})
		}
	}
	return nil
}

func (s *Service) wireProfile() *wire.Profile {
	p := s.Profile()
	return &wire.Profile{Name: p.Name, Avatar: p.Avatar, Rev: p.Rev}
}

func (s *Service) isContact(p peer.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contacts[p]
}

func (s *Service) addContact(c store.Contact) error {
	if err := s.st.PutContact(c); err != nil {
		return err
	}
	if id, err := peer.Decode(c.PeerID); err == nil {
		s.mu.Lock()
		s.contacts[id] = true
		s.mu.Unlock()
	}
	s.emitContacts()
	return nil
}

func (s *Service) onConnectedness(p peer.ID, online bool) {
	id := p.String()
	if online {
		_, err := s.st.UpdateContact(id, func(c *store.Contact) {
			c.LastSeen = time.Now()
			if addrs := s.node.KnownAddrs(p); len(addrs) > 0 {
				c.Addrs = addrs
			}
		})
		if err != nil {
			s.ev.Logf("update contact: %v", err)
		}
		s.mu.Lock()
		delete(s.backoff, id)
		s.mu.Unlock()
		s.Kick()
	}
	if s.ev.Presence != nil {
		s.ev.Presence(id, online)
	}
}

// Online reports whether a contact is connected now.
func (s *Service) Online(peerID string) bool {
	id, err := peer.Decode(peerID)
	return err == nil && s.node.Connected(id)
}

// Contacts returns all contacts.
func (s *Service) Contacts() ([]store.Contact, error) { return s.st.Contacts() }

// DeleteContact removes a contact and the conversation.
func (s *Service) DeleteContact(peerID string) error {
	if err := s.st.DeleteContact(peerID); err != nil {
		return err
	}
	if id, err := peer.Decode(peerID); err == nil {
		s.mu.Lock()
		delete(s.contacts, id)
		s.mu.Unlock()
		for _, c := range s.node.Host.Network().ConnsToPeer(id) {
			c.Close()
		}
	}
	s.emitContacts()
	return nil
}

// SetActiveChat tells the service which conversation is on screen, so
// its messages count as read as they arrive.
func (s *Service) SetActiveChat(peerID string) {
	s.mu.Lock()
	s.active = peerID
	s.mu.Unlock()
	if peerID != "" {
		s.MarkRead(peerID)
	}
}

func (s *Service) emitContacts() {
	if s.ev.ContactsChanged != nil {
		s.ev.ContactsChanged()
	}
}

func (s *Service) emitMessage(m store.Message) {
	if s.ev.Message != nil {
		s.ev.Message(m)
	}
}

// Kick asks the outbox to try delivering now.
func (s *Service) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
