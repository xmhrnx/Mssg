package wire

import (
	"bufio"
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Frame{Type: TMessage, Message: &Message{ID: "a", Text: "héllo 👋"}}
	if err := Write(&buf, in); err != nil {
		t.Fatal(err)
	}
	var out Frame
	if err := Read(bufio.NewReader(&buf), &out); err != nil {
		t.Fatal(err)
	}
	if out.Type != TMessage || out.Message.Text != in.Message.Text {
		t.Fatalf("got %+v", out)
	}
}

func TestRejectsOversize(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff}))
	var f Frame
	if err := Read(r, &f); err != ErrTooLarge {
		t.Fatalf("got %v", err)
	}
}
