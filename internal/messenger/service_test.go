package messenger

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/xmhrnx/mssg/internal/p2p"
	"github.com/xmhrnx/mssg/internal/store"
)

type recorder struct {
	mu       sync.Mutex
	msgs     map[string]store.Message
	redeemed chan store.Contact
}

func newPeer(t *testing.T, name string) (*Service, *recorder) {
	t.Helper()
	dir := t.TempDir()
	key, _ := store.NewKey()
	st, err := store.Open(filepath.Join(dir, "db"), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProfile(store.Profile{Name: name, Rev: 1}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{msgs: map[string]store.Message{}, redeemed: make(chan store.Contact, 1)}
	s, err := New(context.Background(), Options{
		Store: st, DataDir: dir,
		Node: &p2p.Config{Bootstrap: []peer.AddrInfo{}, ListenAddrs: []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"}, NoMDNS: true},
		Events: Events{
			Message: func(m store.Message) {
				rec.mu.Lock()
				rec.msgs[m.ID] = m
				rec.mu.Unlock()
			},
			InviteRedeemed: func(c store.Contact) { rec.redeemed <- c },
			Logf:           t.Logf,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); st.Close() })
	return s, rec
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestHandshakeMessagesAndFiles(t *testing.T) {
	alice, _ := newPeer(t, "Alice")
	bob, bobRec := newPeer(t, "Bob")

	card, err := alice.MyCard()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.AddFromCard(card); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "alice to know bob", func() bool {
		_, err := alice.st.Contact(bob.ID())
		return err == nil
	})
	got, _ := alice.st.Contact(bob.ID())
	if got.Name != "Bob" || !got.Confirmed {
		t.Fatalf("alice's contact: %+v", got)
	}
	waitFor(t, "bob's contact confirmed", func() bool {
		c, err := bob.st.Contact(alice.ID())
		return err == nil && c.Confirmed && c.Name == "Alice"
	})

	m, err := bob.SendText(alice.ID(), "hello 👋")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "delivery", func() bool {
		bobRec.mu.Lock()
		defer bobRec.mu.Unlock()
		return bobRec.msgs[m.ID].Status == store.StatusDelivered
	})
	in, err := alice.st.Message(m.ID)
	if err != nil || in.Text != "hello 👋" || in.Outgoing {
		t.Fatalf("alice got %+v, %v", in, err)
	}
	alice.SetActiveChat(bob.ID())
	waitFor(t, "read receipt", func() bool {
		bobRec.mu.Lock()
		defer bobRec.mu.Unlock()
		return bobRec.msgs[m.ID].Status == store.StatusRead
	})

	data := bytes.Repeat([]byte("mssg-file-"), 300_000) // 3 MB: past auto-download for files
	fm, err := alice.SendFile(bob.ID(), bytes.NewReader(data), FileMeta{Name: "../notes.bin"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "file offer", func() bool {
		_, err := bob.st.Message(fm.ID)
		return err == nil
	})
	go bob.Download(fm.ID)
	waitFor(t, "download", func() bool {
		bm, err := bob.st.Message(fm.ID)
		return err == nil && bm.Attachment.State == store.TransferDone
	})
	bm, _ := bob.st.Message(fm.ID)
	if filepath.Base(bm.Attachment.Path) != "notes.bin" {
		t.Fatalf("path %q", bm.Attachment.Path)
	}
	b, err := os.ReadFile(bm.Attachment.Path)
	if err != nil || !bytes.Equal(b, data) {
		t.Fatalf("downloaded file differs: %v", err)
	}
}

func TestOfflineQueue(t *testing.T) {
	alice, _ := newPeer(t, "Alice")
	bob, bobRec := newPeer(t, "Bob")
	card, _ := alice.MyCard()
	bob.AddFromCard(card)
	waitFor(t, "handshake", func() bool {
		c, err := bob.st.Contact(alice.ID())
		return err == nil && c.Confirmed
	})
	// Alice goes away: Bob's message waits in the outbox.
	aliceID := alice.ID()
	alice.Close()
	m, _ := bob.SendText(aliceID, "are you there?")
	time.Sleep(time.Second)
	bobRec.mu.Lock()
	status := bobRec.msgs[m.ID].Status
	bobRec.mu.Unlock()
	if status != store.StatusPending {
		t.Fatalf("status %v", status)
	}
	items, _ := bob.st.Outbox(aliceID)
	if len(items) == 0 {
		t.Fatal("not queued")
	}
}

func TestInviteTokenSingleUse(t *testing.T) {
	alice, _ := newPeer(t, "Alice")
	card, _ := alice.MyCard()
	ok, _ := alice.st.TakeInvite(card.Token)
	if !ok {
		t.Fatal("fresh token rejected")
	}
	ok, _ = alice.st.TakeInvite(card.Token)
	if ok {
		t.Fatal("token accepted twice")
	}
}
