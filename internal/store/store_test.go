package store

import (
	"path/filepath"
	"testing"
)

func TestWrongKeyFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	k1, _ := NewKey()
	st, err := Open(path, k1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSettings(DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	st.Close()
	k2, _ := NewKey()
	if _, err := Open(path, k2); err == nil {
		t.Fatal("opened with the wrong key")
	}
}

func TestMessagesPaging(t *testing.T) {
	k, _ := NewKey()
	st, err := Open(filepath.Join(t.TempDir(), "db"), k)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for i := range 10 {
		m := Message{ID: string(rune('a' + i)), Peer: "p", Kind: KindText}
		if err := st.AddMessage(&m); err != nil {
			t.Fatal(err)
		}
	}
	page, _ := st.Messages("p", 0, 4)
	if len(page) != 4 || page[0].ID != "g" || page[3].ID != "j" {
		t.Fatalf("latest page %v", ids(page))
	}
	older, _ := st.Messages("p", page[0].Seq, 4)
	if len(older) != 4 || older[3].ID != "f" {
		t.Fatalf("older page %v", ids(older))
	}
	if m := (Message{ID: "a", Peer: "p"}); st.AddMessage(&m) != ErrDuplicate {
		t.Fatal("duplicate accepted")
	}
	u, err := st.UpdateMessage("c", func(m *Message) { m.Text = "edited" })
	if err != nil || u.Text != "edited" {
		t.Fatalf("update: %v %+v", err, u)
	}
	if err := st.DeleteContact("p"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Message("c"); err != ErrNotFound {
		t.Fatalf("message survived contact deletion: %v", err)
	}
}

func ids(ms []Message) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}
