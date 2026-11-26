package messenger

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

// queue stores a frame for delivery and wakes the outbox. An item with
// the same ID replaces the waiting one (a newer profile, say).
func (s *Service) queue(id, peerID string, f wire.Frame) {
	b, err := json.Marshal(f)
	if err != nil {
		s.ev.Logf("queue %s: %v", id, err)
		return
	}
	if err := s.st.Enqueue(store.OutboxItem{ID: id, Peer: peerID, Frame: b, Queued: time.Now()}); err != nil {
		s.ev.Logf("queue %s: %v", id, err)
		return
	}
	s.mu.Lock()
	delete(s.backoff, peerID)
	s.mu.Unlock()
	s.Kick()
}

func (s *Service) outboxLoop() {
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		s.flushOutbox()
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
		case <-s.kick:
		}
	}
}

func (s *Service) flushOutbox() {
	items, err := s.st.Outbox("")
	if err != nil {
		s.ev.Logf("outbox: %v", err)
		return
	}
	byPeer := map[string]bool{}
	for _, it := range items {
		byPeer[it.Peer] = true
	}
	now := time.Now()
	for p := range byPeer {
		s.mu.Lock()
		busy := s.delivery[p]
		wait := s.backoff[p].After(now)
		if !busy && !wait {
			s.delivery[p] = true
		}
		s.mu.Unlock()
		if !busy && !wait {
			go s.deliverTo(p)
		}
	}
}

// deliverTo sends a peer's queued frames in order, backing off when the
// peer can't be reached.
func (s *Service) deliverTo(peerID string) {
	defer func() {
		s.mu.Lock()
		delete(s.delivery, peerID)
		s.mu.Unlock()
	}()
	id, err := peer.Decode(peerID)
	if err != nil {
		return
	}
	for {
		items, err := s.st.Outbox(peerID)
		if err != nil || len(items) == 0 {
			return
		}
		it := items[0]
		var f wire.Frame
		if err := json.Unmarshal(it.Frame, &f); err != nil {
			s.st.Dequeue(it.ID)
			continue
		}
		ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
		reply, err := s.exchange(ctx, id, f)
		cancel()
		if err != nil && reply.Type == wire.TError {
			s.st.Dequeue(it.ID)
			s.refused(peerID, f, reply.Error)
			continue
		}
		if err != nil {
			s.ev.Logf("deliver to %s: %v", id.ShortString(), err)
			it.Attempts++
			s.st.Enqueue(it)
			delay := min(5*time.Minute, time.Duration(1<<min(it.Attempts, 6))*5*time.Second)
			s.mu.Lock()
			s.backoff[peerID] = time.Now().Add(delay)
			s.mu.Unlock()
			return
		}
		s.st.Dequeue(it.ID)
		s.delivered(peerID, f, reply)
	}
}

// delivered records what a peer's reply to a queued frame means.
func (s *Service) delivered(peerID string, f wire.Frame, reply wire.Frame) {
	switch f.Type {
	case wire.TMessage:
		m, err := s.st.UpdateMessage(f.Message.ID, func(m *store.Message) {
			if m.Status < store.StatusDelivered {
				m.Status = store.StatusDelivered
			}
		})
		if err == nil {
			s.emitMessage(m)
		}
	case wire.TContactRequest:
		if reply.Type != wire.TContactOK {
			return
		}
		_, err := s.st.UpdateContact(peerID, func(c *store.Contact) {
			c.Confirmed = true
			c.InviteToken = ""
			if p := reply.Profile; p != nil && p.Rev >= c.Rev {
				c.Name, c.Avatar, c.Rev = p.Name, p.Avatar, p.Rev
			}
		})
		if err != nil {
			s.ev.Logf("confirm contact: %v", err)
		}
		s.emitContacts()
	}
}

// refused records a frame the peer turned down for good.
func (s *Service) refused(peerID string, f wire.Frame, reason string) {
	s.ev.Logf("%s refused %s: %s", peerID, f.Type, reason)
	switch f.Type {
	case wire.TMessage:
		if m, err := s.st.UpdateMessage(f.Message.ID, func(m *store.Message) { m.Status = store.StatusFailed }); err == nil {
			s.emitMessage(m)
		}
	case wire.TContactRequest:
		s.st.UpdateContact(peerID, func(c *store.Contact) { c.InviteToken = "" })
		s.emitContacts()
	}
}

// exchange sends one frame on a new stream and reads the reply.
func (s *Service) exchange(ctx context.Context, p peer.ID, f wire.Frame) (wire.Frame, error) {
	proto := wire.MsgProtocol
	if f.Type == wire.TContactRequest {
		proto = InviteProtocol
	}
	st, err := s.node.NewStream(ctx, p, proto)
	if err != nil {
		return wire.Frame{}, err
	}
	defer st.Close()
	if dl, ok := ctx.Deadline(); ok {
		st.SetDeadline(dl)
	}
	if err := wire.Write(st, f); err != nil {
		st.Reset()
		return wire.Frame{}, err
	}
	var reply wire.Frame
	if err := wire.Read(bufio.NewReader(st), &reply); err != nil {
		st.Reset()
		return wire.Frame{}, err
	}
	if reply.Type == wire.TError {
		return reply, fmt.Errorf("peer refused: %s", reply.Error)
	}
	return reply, nil
}

// sendNow sends a frame without queueing it, for calls and typing.
func (s *Service) sendNow(ctx context.Context, peerID string, f wire.Frame) error {
	id, err := peer.Decode(peerID)
	if err != nil {
		return err
	}
	_, err = s.exchange(ctx, id, f)
	return err
}
