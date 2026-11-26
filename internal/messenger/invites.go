package messenger

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/xmhrnx/mssg/internal/invite"
	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

// InviteLifetime is how long a code's token stays redeemable.
const InviteLifetime = 24 * time.Hour

func (s *Service) card(token string) wire.Card {
	return wire.Card{Version: 1, PeerID: s.ID(), Name: s.Profile().Name, Addrs: s.node.Addrs(), Token: token}
}

// CreateInvite opens a wormhole with this device's card and returns the
// code to share. ctx bounds how long the wormhole waits.
func (s *Service) CreateInvite(ctx context.Context) (invite.Offer, error) {
	token := newID()
	if err := s.st.PutInvite(token, time.Now().Add(InviteLifetime)); err != nil {
		return invite.Offer{}, err
	}
	return invite.Send(ctx, s.card(token))
}

// RedeemInvite reads a contact's card from a code, adds them, and sends
// them this device's card over libp2p to finish the handshake.
func (s *Service) RedeemInvite(ctx context.Context, code string) (store.Contact, error) {
	card, err := invite.Receive(ctx, code)
	if err != nil {
		return store.Contact{}, err
	}
	id, err := peer.Decode(card.PeerID)
	if err != nil {
		return store.Contact{}, fmt.Errorf("the invite has a bad address")
	}
	if id == s.node.ID() {
		return store.Contact{}, fmt.Errorf("that's your own code")
	}
	if c, err := s.st.Contact(card.PeerID); err == nil && c.Confirmed {
		return c, nil
	}
	return s.AddFromCard(card)
}

// AddFromCard adds a contact from their card and queues the handshake.
func (s *Service) AddFromCard(card wire.Card) (store.Contact, error) {
	id, err := peer.Decode(card.PeerID)
	if err != nil {
		return store.Contact{}, err
	}
	c := store.Contact{
		PeerID:      card.PeerID,
		Name:        clip(card.Name, 64),
		Addrs:       card.Addrs,
		Added:       time.Now(),
		InviteToken: card.Token,
	}
	if err := s.addContact(c); err != nil {
		return store.Contact{}, err
	}
	s.node.AddAddrs(id, card.Addrs)
	s.queue("contact:"+card.PeerID, card.PeerID, wire.Frame{
		Type:    wire.TContactRequest,
		Card:    &wire.Card{Version: 1, PeerID: s.ID(), Name: s.Profile().Name, Addrs: s.node.Addrs(), Token: card.Token},
		Profile: s.wireProfile(),
	})
	return c, nil
}

// MyCard returns this device's card with a fresh invite token, for
// sharing as a link or QR code instead of a wormhole code.
func (s *Service) MyCard() (wire.Card, error) {
	token := newID()
	if err := s.st.PutInvite(token, time.Now().Add(InviteLifetime)); err != nil {
		return wire.Card{}, err
	}
	return s.card(token), nil
}
