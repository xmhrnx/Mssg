// Package invite carries contact cards through a magic wormhole: one
// side gets a short code ("7-guitarist-revenge") and reads it to the
// other, and the PAKE-encrypted rendezvous hands over the card. After
// that the peers talk directly over libp2p and never need it again.
package invite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/psanford/wormhole-william/wormhole"

	"github.com/xmhrnx/mssg/internal/wire"
)

// AppID keeps this app's codes apart from other wormhole apps.
const AppID = "mssg.app/contact-invite/v1"

func client() *wormhole.Client {
	return &wormhole.Client{AppID: AppID, PassPhraseComponentLength: 2}
}

// Offer is an invite waiting to be redeemed.
type Offer struct {
	// Code is what the other person types.
	Code string
	// Done reports once the other side read the card, or failed to.
	Done <-chan error
}

// Send publishes a card and returns its code. The wormhole stays open
// until the other side redeems it or ctx ends.
func Send(ctx context.Context, card wire.Card) (Offer, error) {
	b, err := json.Marshal(card)
	if err != nil {
		return Offer{}, err
	}
	code, results, err := client().SendText(ctx, string(b))
	if err != nil {
		return Offer{}, fmt.Errorf("open wormhole: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		select {
		case r, ok := <-results:
			switch {
			case !ok:
				done <- fmt.Errorf("wormhole closed")
			case r.Error != nil:
				done <- r.Error
			default:
				done <- nil
			}
		case <-ctx.Done():
			done <- ctx.Err()
		}
	}()
	return Offer{Code: code, Done: done}, nil
}

// NormalizeCode tidies a typed code: spaces become dashes, case folds.
func NormalizeCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.Join(strings.Fields(code), "-")
	return code
}

// Receive redeems a code and returns the card it carried.
func Receive(ctx context.Context, code string) (wire.Card, error) {
	code = NormalizeCode(code)
	if !strings.Contains(code, "-") {
		return wire.Card{}, fmt.Errorf("a code looks like 7-guitarist-revenge")
	}
	msg, err := client().Receive(ctx, code)
	if err != nil {
		return wire.Card{}, fmt.Errorf("redeem code: %w", err)
	}
	if msg.Type != wormhole.TransferText {
		msg.Reject()
		return wire.Card{}, fmt.Errorf("this code isn't a contact invite")
	}
	b, err := io.ReadAll(io.LimitReader(msg, 64<<10))
	if err != nil {
		return wire.Card{}, fmt.Errorf("read invite: %w", err)
	}
	var card wire.Card
	if err := json.Unmarshal(b, &card); err != nil || card.PeerID == "" {
		return wire.Card{}, fmt.Errorf("this code isn't a contact invite")
	}
	return card, nil
}
