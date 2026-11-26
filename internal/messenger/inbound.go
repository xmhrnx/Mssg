package messenger

import (
	"bufio"
	"errors"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

const streamTimeout = 30 * time.Second

// handleMsgStream answers one frame from a contact.
func (s *Service) handleMsgStream(st network.Stream) {
	defer st.Close()
	st.SetDeadline(time.Now().Add(streamTimeout))
	from := st.Conn().RemotePeer().String()
	var f wire.Frame
	if err := wire.Read(bufio.NewReader(st), &f); err != nil {
		st.Reset()
		return
	}
	reply := s.handleFrame(from, f)
	if err := wire.Write(st, reply); err != nil {
		st.Reset()
	}
}

func (s *Service) handleFrame(from string, f wire.Frame) wire.Frame {
	ack := wire.Frame{Type: wire.TAck}
	switch f.Type {
	case wire.TMessage:
		if f.Message == nil {
			return errFrame("empty message")
		}
		if err := s.receiveMessage(from, *f.Message); err != nil {
			return errFrame("store failed")
		}
	case wire.TRead:
		for _, id := range f.IDs {
			m, err := s.st.UpdateMessage(id, func(m *store.Message) {
				if m.Outgoing && m.Peer == from && m.Status < store.StatusRead {
					m.Status = store.StatusRead
				}
			})
			if err == nil {
				s.emitMessage(m)
			}
		}
	case wire.TTyping:
		if s.ev.Typing != nil {
			s.ev.Typing(from)
		}
	case wire.TProfile:
		if p := f.Profile; p != nil {
			s.applyProfile(from, *p)
		}
	case wire.TCall:
		if f.Call != nil && s.ev.Call != nil {
			s.ev.Call(from, *f.Call)
		}
	default:
		return errFrame("unknown frame")
	}
	return ack
}

func errFrame(reason string) wire.Frame { return wire.Frame{Type: wire.TError, Error: reason} }

func (s *Service) applyProfile(from string, p wire.Profile) {
	_, err := s.st.UpdateContact(from, func(c *store.Contact) {
		if p.Rev >= c.Rev {
			c.Name, c.Avatar, c.Rev = clip(p.Name, 64), p.Avatar, p.Rev
		}
		// A profile from them means they know us: the handshake is done.
		c.Confirmed = true
	})
	if err == nil {
		s.emitContacts()
	}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func (s *Service) receiveMessage(from string, wm wire.Message) error {
	m := store.Message{
		ID:       wm.ID,
		Peer:     from,
		Outgoing: false,
		Time:     time.UnixMilli(wm.Time),
		Kind:     wm.Kind,
		Text:     wm.Text,
		Call:     wm.Call,
		Status:   store.StatusDelivered, // incoming: delivered means unread
	}
	if m.Time.IsZero() || m.Time.After(time.Now().Add(time.Hour)) {
		m.Time = time.Now()
	}
	if wm.Attachment != nil {
		a := *wm.Attachment
		a.Path, a.Received, a.State = "", 0, store.TransferNone
		m.Attachment = &a
	}
	s.mu.Lock()
	active := s.active == from
	s.mu.Unlock()
	if active {
		m.Status = store.StatusRead
	}
	if err := s.st.AddMessage(&m); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil
		}
		return err
	}
	c, err := s.st.UpdateContact(from, func(c *store.Contact) {
		c.Confirmed = true
		if !active {
			c.Unread++
		}
	})
	if err != nil {
		return err
	}
	s.emitMessage(m)
	s.emitContacts()
	if active {
		s.sendReceipts(from, []string{m.ID})
	} else if s.ev.Incoming != nil {
		s.ev.Incoming(c, m)
	}
	if a := m.Attachment; a != nil && s.autoDownload(m) {
		go s.Download(m.ID)
	}
	return nil
}

func (s *Service) autoDownload(m store.Message) bool {
	limit := int64(s.Settings().AutoDownloadMB) << 20
	switch m.Kind {
	case store.KindImage, store.KindGIF, store.KindAudio, store.KindVideo:
		return m.Attachment.Size <= limit
	}
	return m.Attachment.Size <= 2<<20
}

// handleInviteStream redeems an invite code from a stranger.
func (s *Service) handleInviteStream(st network.Stream) {
	defer st.Close()
	st.SetDeadline(time.Now().Add(streamTimeout))
	from := st.Conn().RemotePeer()
	var f wire.Frame
	if err := wire.Read(bufio.NewReader(st), &f); err != nil || f.Type != wire.TContactRequest || f.Card == nil {
		st.Reset()
		return
	}
	reply := s.redeem(from, f)
	if err := wire.Write(st, reply); err != nil {
		st.Reset()
	}
}

func (s *Service) redeem(from peer.ID, f wire.Frame) wire.Frame {
	if f.Card.PeerID != from.String() {
		return errFrame("card doesn't match the connection")
	}
	ok := wire.Frame{Type: wire.TContactOK, Profile: s.wireProfile()}
	if c, err := s.st.Contact(from.String()); err == nil && c.Confirmed {
		return ok // a retry of a request that already succeeded
	}
	valid, err := s.st.TakeInvite(f.Card.Token)
	if err != nil {
		return errFrame("store failed")
	}
	if !valid {
		return errFrame("invite expired or already used")
	}
	c := store.Contact{
		PeerID:    from.String(),
		Name:      clip(f.Card.Name, 64),
		Addrs:     f.Card.Addrs,
		Added:     time.Now(),
		LastSeen:  time.Now(),
		Confirmed: true,
	}
	if p := f.Profile; p != nil {
		c.Name, c.Avatar, c.Rev = clip(p.Name, 64), p.Avatar, p.Rev
	}
	if err := s.addContact(c); err != nil {
		return errFrame("store failed")
	}
	s.node.AddAddrs(from, f.Card.Addrs)
	if s.ev.InviteRedeemed != nil {
		s.ev.InviteRedeemed(c)
	}
	return ok
}
