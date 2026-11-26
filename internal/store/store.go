package store

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bMeta     = []byte("meta")
	bContacts = []byte("contacts")
	bMessages = []byte("messages")
	bIndex    = []byte("msgindex")
	bOutbox   = []byte("outbox")
	bInvites  = []byte("invites")
)

// ErrNotFound is returned for missing records.
var ErrNotFound = errors.New("store: not found")

// Store is the encrypted database. Its methods are safe for concurrent use.
type Store struct {
	db *bolt.DB
	s  *sealer
}

// Open opens (creating if needed) the database at path, sealed with key.
func Open(path string, key []byte) (*Store, error) {
	s, err := newSealer(key)
	if err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bMeta, bContacts, bMessages, bIndex, bOutbox, bInvites} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init database: %w", err)
	}
	st := &Store{db: db, s: s}
	// A wrong key shows on the first sealed read; check now so the
	// caller can tell the user instead of failing later.
	if _, err := st.Settings(); err != nil && !errors.Is(err, ErrNotFound) {
		db.Close()
		return nil, err
	}
	return st, nil
}

// Close closes the database.
func (st *Store) Close() error { return st.db.Close() }

func (st *Store) put(tx *bolt.Tx, bucket *bolt.Bucket, name, key []byte, v any) error {
	plain, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sealed, err := st.s.seal(name, key, plain)
	if err != nil {
		return err
	}
	return bucket.Put(key, sealed)
}

func (st *Store) get(bucket *bolt.Bucket, name, key []byte, v any) error {
	raw := bucket.Get(key)
	if raw == nil {
		return ErrNotFound
	}
	plain, err := st.s.open(name, key, raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, v)
}

func (st *Store) putMeta(key string, v any) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		return st.put(tx, tx.Bucket(bMeta), bMeta, []byte(key), v)
	})
}

func (st *Store) getMeta(key string, v any) error {
	return st.db.View(func(tx *bolt.Tx) error {
		return st.get(tx.Bucket(bMeta), bMeta, []byte(key), v)
	})
}

// Profile returns the saved profile, ErrNotFound before onboarding.
func (st *Store) Profile() (Profile, error) {
	var p Profile
	err := st.getMeta("profile", &p)
	return p, err
}

// SaveProfile saves the profile.
func (st *Store) SaveProfile(p Profile) error { return st.putMeta("profile", p) }

// Settings returns the saved settings, or the defaults.
func (st *Store) Settings() (Settings, error) {
	s := DefaultSettings()
	err := st.getMeta("settings", &s)
	if errors.Is(err, ErrNotFound) {
		return DefaultSettings(), nil
	}
	return s, err
}

// SaveSettings saves the settings.
func (st *Store) SaveSettings(s Settings) error { return st.putMeta("settings", s) }

// Identity returns the node's marshalled private key.
func (st *Store) Identity() ([]byte, error) {
	var k []byte
	err := st.getMeta("identity", &k)
	return k, err
}

// SaveIdentity saves the node's marshalled private key.
func (st *Store) SaveIdentity(k []byte) error { return st.putMeta("identity", k) }

// Contacts returns every contact, most recently active first.
func (st *Store) Contacts() ([]Contact, error) {
	var out []Contact
	err := st.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bContacts)
		return b.ForEach(func(k, _ []byte) error {
			var c Contact
			if err := st.get(b, bContacts, k, &c); err != nil {
				return err
			}
			out = append(out, c)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Added.After(out[j].Added) })
	return out, err
}

// Contact returns one contact.
func (st *Store) Contact(peer string) (Contact, error) {
	var c Contact
	err := st.db.View(func(tx *bolt.Tx) error {
		return st.get(tx.Bucket(bContacts), bContacts, []byte(peer), &c)
	})
	return c, err
}

// PutContact saves a contact.
func (st *Store) PutContact(c Contact) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		return st.put(tx, tx.Bucket(bContacts), bContacts, []byte(c.PeerID), c)
	})
}

// UpdateContact applies fn to a stored contact atomically.
func (st *Store) UpdateContact(peer string, fn func(*Contact)) (Contact, error) {
	var c Contact
	err := st.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bContacts)
		if err := st.get(b, bContacts, []byte(peer), &c); err != nil {
			return err
		}
		fn(&c)
		return st.put(tx, b, bContacts, []byte(peer), c)
	})
	return c, err
}

// DeleteContact removes a contact, its conversation and queued messages.
func (st *Store) DeleteContact(peer string) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bContacts).Delete([]byte(peer)); err != nil {
			return err
		}
		msgs := tx.Bucket(bMessages)
		if conv := msgs.Bucket([]byte(peer)); conv != nil {
			idx := tx.Bucket(bIndex)
			err := conv.ForEach(func(k, _ []byte) error {
				var m Message
				if st.get(conv, convName(peer), k, &m) == nil {
					return idx.Delete([]byte(m.ID))
				}
				return nil
			})
			if err != nil {
				return err
			}
			if err := msgs.DeleteBucket([]byte(peer)); err != nil {
				return err
			}
		}
		ob := tx.Bucket(bOutbox)
		var drop [][]byte
		err := ob.ForEach(func(k, _ []byte) error {
			var it OutboxItem
			if st.get(ob, bOutbox, k, &it) == nil && it.Peer == peer {
				drop = append(drop, append([]byte(nil), k...))
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, k := range drop {
			if err := ob.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func convName(peer string) []byte { return append([]byte("conv:"), peer...) }

func seqKey(seq uint64) []byte {
	k := make([]byte, 8)
	binary.BigEndian.PutUint64(k, seq)
	return k
}

// AddMessage stores a new message, assigning its Seq.
func (st *Store) AddMessage(m *Message) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		idx := tx.Bucket(bIndex)
		if idx.Get([]byte(m.ID)) != nil {
			return ErrDuplicate
		}
		conv, err := tx.Bucket(bMessages).CreateBucketIfNotExists([]byte(m.Peer))
		if err != nil {
			return err
		}
		seq, err := conv.NextSequence()
		if err != nil {
			return err
		}
		m.Seq = seq
		if err := st.put(tx, conv, convName(m.Peer), seqKey(seq), m); err != nil {
			return err
		}
		return idx.Put([]byte(m.ID), append([]byte(m.Peer+"\x00"), seqKey(seq)...))
	})
}

// ErrDuplicate is returned when a message ID is already stored.
var ErrDuplicate = errors.New("store: duplicate message")

// UpdateMessage applies fn to a stored message atomically and returns it.
func (st *Store) UpdateMessage(id string, fn func(*Message)) (Message, error) {
	var m Message
	err := st.db.Update(func(tx *bolt.Tx) error {
		conv, key, err := st.locate(tx, id)
		if err != nil {
			return err
		}
		if err := st.get(conv, convName(peerOf(tx, id)), key, &m); err != nil {
			return err
		}
		fn(&m)
		return st.put(tx, conv, convName(m.Peer), key, m)
	})
	return m, err
}

// Message returns a message by ID.
func (st *Store) Message(id string) (Message, error) {
	var m Message
	err := st.db.View(func(tx *bolt.Tx) error {
		conv, key, err := st.locate(tx, id)
		if err != nil {
			return err
		}
		return st.get(conv, convName(peerOf(tx, id)), key, &m)
	})
	return m, err
}

func peerOf(tx *bolt.Tx, id string) string {
	v := tx.Bucket(bIndex).Get([]byte(id))
	if len(v) < 9 {
		return ""
	}
	return string(v[:len(v)-9])
}

func (st *Store) locate(tx *bolt.Tx, id string) (*bolt.Bucket, []byte, error) {
	v := tx.Bucket(bIndex).Get([]byte(id))
	if len(v) < 9 {
		return nil, nil, ErrNotFound
	}
	peer, key := v[:len(v)-9], v[len(v)-8:]
	conv := tx.Bucket(bMessages).Bucket(peer)
	if conv == nil {
		return nil, nil, ErrNotFound
	}
	return conv, append([]byte(nil), key...), nil
}

// Messages returns up to limit messages of a conversation before seq
// (0 for the latest), oldest first.
func (st *Store) Messages(peer string, before uint64, limit int) ([]Message, error) {
	var out []Message
	err := st.db.View(func(tx *bolt.Tx) error {
		conv := tx.Bucket(bMessages).Bucket([]byte(peer))
		if conv == nil {
			return nil
		}
		c := conv.Cursor()
		var k, v []byte
		if before == 0 {
			k, v = c.Last()
		} else {
			c.Seek(seqKey(before))
			k, v = c.Prev()
		}
		for ; k != nil && len(out) < limit; k, v = c.Prev() {
			_ = v
			var m Message
			if err := st.get(conv, convName(peer), k, &m); err != nil {
				return err
			}
			out = append(out, m)
		}
		return nil
	})
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, err
}

// LastMessage returns a conversation's newest message.
func (st *Store) LastMessage(peer string) (Message, bool, error) {
	ms, err := st.Messages(peer, 0, 1)
	if err != nil || len(ms) == 0 {
		return Message{}, false, err
	}
	return ms[0], true, nil
}

// OutboxItem is a frame waiting for its peer to be reachable.
type OutboxItem struct {
	ID       string    `json:"id"`
	Peer     string    `json:"peer"`
	Frame    []byte    `json:"frame"`
	Queued   time.Time `json:"queued"`
	Attempts int       `json:"attempts"`
}

// Enqueue adds or replaces an outbox item.
func (st *Store) Enqueue(it OutboxItem) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		return st.put(tx, tx.Bucket(bOutbox), bOutbox, []byte(it.ID), it)
	})
}

// Dequeue removes an outbox item.
func (st *Store) Dequeue(id string) error {
	return st.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(bOutbox).Delete([]byte(id)) })
}

// Outbox returns the queued items for a peer ("" for all), oldest first.
func (st *Store) Outbox(peer string) ([]OutboxItem, error) {
	var out []OutboxItem
	err := st.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bOutbox)
		return b.ForEach(func(k, _ []byte) error {
			var it OutboxItem
			if err := st.get(b, bOutbox, k, &it); err != nil {
				return err
			}
			if peer == "" || it.Peer == peer {
				out = append(out, it)
			}
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Queued.Before(out[j].Queued) })
	return out, err
}

// PutInvite records an invite token this device handed out.
func (st *Store) PutInvite(token string, expires time.Time) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		return st.put(tx, tx.Bucket(bInvites), bInvites, []byte(token), expires)
	})
}

// TakeInvite consumes an invite token, reporting whether it was valid.
func (st *Store) TakeInvite(token string) (bool, error) {
	ok := false
	err := st.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bInvites)
		var exp time.Time
		if err := st.get(b, bInvites, []byte(token), &exp); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			return err
		}
		ok = time.Now().Before(exp)
		return b.Delete([]byte(token))
	})
	return ok, err
}
