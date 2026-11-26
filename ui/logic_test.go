package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/xmhrnx/mssg/internal/store"
)

func TestSafetyNumberIsSymmetric(t *testing.T) {
	a, b := "12D3KooWAlice", "12D3KooWBob"
	if safetyNumber(a, b) != safetyNumber(b, a) {
		t.Fatal("the two sides would see different numbers")
	}
	if safetyNumber(a, b) == safetyNumber(a, "12D3KooWMallory") {
		t.Fatal("different pairs share a number")
	}
	if got := len(strings.Fields(safetyNumber(a, b))); got != 12 {
		t.Fatalf("%d groups", got)
	}
}

func TestIsJumbo(t *testing.T) {
	for s, want := range map[string]bool{
		"😂":     true,
		"😂🔥":    true,
		"👍🏽":    true,
		"👨‍👩‍👧": true,
		"😂😂😂😂":  false,
		"ok 👍":  false,
		"12":    false,
		"":      false,
	} {
		if got := isJumbo(s); got != want {
			t.Errorf("isJumbo(%q) = %v", s, got)
		}
	}
}

func TestCallSummary(t *testing.T) {
	missed := store.Message{Kind: store.KindCall, Call: &store.CallInfo{Missed: true, Video: true}}
	if got := callSummary(missed); got != "Missed video call" {
		t.Fatal(got)
	}
	out := store.Message{Kind: store.KindCall, Outgoing: true, Call: &store.CallInfo{Seconds: 75}}
	if got := callSummary(out); got != "Outgoing voice call · 1:15" {
		t.Fatal(got)
	}
}

func TestMediaBoxKeepsShape(t *testing.T) {
	w, h := mediaBox(4000, 3000)
	if w != 280 || h != 210 {
		t.Fatalf("landscape %dx%d", w, h)
	}
	w, h = mediaBox(1000, 4000)
	if h != 360 || w < 90 {
		t.Fatalf("portrait %dx%d", w, h)
	}
}

func TestGroupsWith(t *testing.T) {
	now := time.Now()
	a := store.Message{Outgoing: true, Kind: store.KindText, Time: now}
	b := store.Message{Outgoing: true, Kind: store.KindText, Time: now.Add(time.Minute)}
	if !groupsWith(a, &b) {
		t.Fatal("close messages from one side should group")
	}
	b.Outgoing = false
	if groupsWith(a, &b) {
		t.Fatal("messages from different sides grouped")
	}
}
