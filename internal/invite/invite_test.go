package invite

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/xmhrnx/mssg/internal/wire"
)

func TestNormalizeCode(t *testing.T) {
	if got := NormalizeCode("  7 Guitarist  revenge "); got != "7-guitarist-revenge" {
		t.Fatalf("got %q", got)
	}
}

// TestRoundTrip uses the public rendezvous server; set MSSG_NET_TESTS=1.
func TestRoundTrip(t *testing.T) {
	if os.Getenv("MSSG_NET_TESTS") == "" {
		t.Skip("set MSSG_NET_TESTS=1 to use the public wormhole server")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	in := wire.Card{Version: 1, PeerID: "12D3KooWtest", Name: "Alice", Token: "t"}
	offer, err := Send(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("code %s", offer.Code)
	out, err := Receive(ctx, offer.Code)
	if err != nil {
		t.Fatal(err)
	}
	if out.PeerID != in.PeerID || out.Name != in.Name || out.Token != in.Token {
		t.Fatalf("got %+v", out)
	}
	if err := <-offer.Done; err != nil {
		t.Fatal(err)
	}
}
