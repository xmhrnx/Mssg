package messenger

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	presenceMin = 30 * time.Second
	presenceMax = 30 * time.Minute
)

// presenceLoop keeps connections to contacts open, so they show online
// and their messages and calls arrive without waiting. Each contact
// that can't be reached is retried less and less often.
func (s *Service) presenceLoop() {
	next := map[string]time.Time{}
	wait := map[string]time.Duration{}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	sem := make(chan struct{}, 4)
	for {
		cs, err := s.st.Contacts()
		if err == nil {
			now := time.Now()
			for _, c := range cs {
				id, err := peer.Decode(c.PeerID)
				if err != nil || s.node.Connected(id) || now.Before(next[c.PeerID]) {
					continue
				}
				d := wait[c.PeerID]
				d = min(presenceMax, max(presenceMin, d*2))
				wait[c.PeerID], next[c.PeerID] = d, now.Add(d)
				go func(id peer.ID, key string) {
					sem <- struct{}{}
					defer func() { <-sem }()
					ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
					defer cancel()
					if s.node.Reach(ctx, id) == nil {
						s.mu.Lock()
						s.reached = append(s.reached, key)
						s.mu.Unlock()
					}
				}(id, c.PeerID)
			}
		}
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
		case <-s.refresh:
			clear(next)
		}
		s.mu.Lock()
		for _, k := range s.reached {
			delete(wait, k)
		}
		s.reached = s.reached[:0]
		s.mu.Unlock()
	}
}

// Refresh retries every offline contact now: the app came back to the
// screen, or the network changed.
func (s *Service) Refresh() {
	select {
	case s.refresh <- struct{}{}:
	default:
	}
	s.Kick()
}
