package messenger

import (
	"context"
	"strings"
	"time"

	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

// MaxTextRunes bounds a text message.
const MaxTextRunes = 8000

// SendText sends a text message (emoji included) to a contact.
func (s *Service) SendText(peerID, text string) (store.Message, error) {
	text = strings.TrimSpace(text)
	return s.send(store.Message{Peer: peerID, Kind: store.KindText, Text: clip(text, MaxTextRunes)})
}

// send stores an outgoing message and queues it.
func (s *Service) send(m store.Message) (store.Message, error) {
	m.ID = newID()
	m.Outgoing = true
	m.Time = time.Now()
	m.Status = store.StatusPending
	if err := s.st.AddMessage(&m); err != nil {
		return m, err
	}
	s.emitMessage(m)
	s.emitContacts()
	s.queue("msg:"+m.ID, m.Peer, wire.Frame{Type: wire.TMessage, Message: toWire(m)})
	return m, nil
}

func toWire(m store.Message) *wire.Message {
	wm := &wire.Message{ID: m.ID, Time: m.Time.UnixMilli(), Kind: m.Kind, Text: m.Text, Call: m.Call}
	if a := m.Attachment; a != nil {
		c := *a
		c.Path, c.Received, c.State = "", 0, store.TransferNone
		wm.Attachment = &c
	}
	return wm
}

// AddCallLog records a call in the conversation, on this side only:
// each side logs the call it saw.
func (s *Service) AddCallLog(peerID string, outgoing bool, info store.CallInfo) (store.Message, error) {
	m := store.Message{
		ID: newID(), Peer: peerID, Outgoing: outgoing, Time: time.Now(),
		Kind: store.KindCall, Call: &info, Status: store.StatusRead,
	}
	if !outgoing && info.Missed {
		m.Status = store.StatusDelivered
	}
	if err := s.st.AddMessage(&m); err != nil {
		return m, err
	}
	if !outgoing && info.Missed {
		s.st.UpdateContact(peerID, func(c *store.Contact) { c.Unread++ })
	}
	s.emitMessage(m)
	s.emitContacts()
	return m, nil
}

// Messages returns a page of a conversation, oldest first.
func (s *Service) Messages(peerID string, before uint64, limit int) ([]store.Message, error) {
	return s.st.Messages(peerID, before, limit)
}

// MarkRead marks a conversation read and tells the contact, if read
// receipts are on.
func (s *Service) MarkRead(peerID string) {
	c, err := s.st.Contact(peerID)
	if err != nil {
		return
	}
	msgs, err := s.st.Messages(peerID, 0, 200)
	if err != nil {
		return
	}
	var ids []string
	for _, m := range msgs {
		if !m.Outgoing && m.Status < store.StatusRead {
			if u, err := s.st.UpdateMessage(m.ID, func(m *store.Message) { m.Status = store.StatusRead }); err == nil {
				s.emitMessage(u)
			}
			if m.Kind != store.KindCall {
				ids = append(ids, m.ID)
			}
		}
	}
	if c.Unread != 0 {
		s.st.UpdateContact(peerID, func(c *store.Contact) { c.Unread = 0 })
		s.emitContacts()
	}
	s.sendReceipts(peerID, ids)
}

func (s *Service) sendReceipts(peerID string, ids []string) {
	if len(ids) == 0 || !s.Settings().ReadReceipts {
		return
	}
	s.queue("read:"+newID(), peerID, wire.Frame{Type: wire.TRead, IDs: ids})
}

// SendTyping tells a connected contact that the user is typing, at most
// every few seconds. It's dropped when the contact is offline.
func (s *Service) SendTyping(peerID string) {
	s.mu.Lock()
	if time.Since(s.typingAt[peerID]) < 3*time.Second {
		s.mu.Unlock()
		return
	}
	s.typingAt[peerID] = time.Now()
	s.mu.Unlock()
	if !s.Online(peerID) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		s.sendNow(ctx, peerID, wire.Frame{Type: wire.TTyping})
	}()
}

// SendCall sends a call signaling frame right away.
func (s *Service) SendCall(ctx context.Context, peerID string, c wire.Call) error {
	return s.sendNow(ctx, peerID, wire.Frame{Type: wire.TCall, Call: &c})
}

// DeleteMessage removes a message from this device.
func (s *Service) DeleteMessage(id string) error {
	m, err := s.st.UpdateMessage(id, func(m *store.Message) {
		m.Kind, m.Text, m.Attachment, m.Call = store.KindText, "", nil, nil
	})
	if err == nil {
		s.st.Dequeue("msg:" + id)
		s.emitMessage(m)
	}
	return err
}
