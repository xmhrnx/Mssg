package store

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// KeySize is the length of the database key.
const KeySize = chacha20poly1305.KeySize

// NewKey returns a fresh random database key.
func NewKey() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return k, nil
}

// sealer encrypts values bound to where they're stored, so a value
// copied to another key or bucket fails to open.
type sealer struct {
	aead interface {
		Seal(dst, nonce, plaintext, additionalData []byte) []byte
		Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
		NonceSize() int
	}
}

func newSealer(key []byte) (*sealer, error) {
	a, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("database key: %w", err)
	}
	return &sealer{aead: a}, nil
}

func (s *sealer) seal(bucket, key, plain []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize(), s.aead.NonceSize()+len(plain)+16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plain, ad(bucket, key)), nil
}

var errCorrupt = errors.New("store: value failed authentication")

func (s *sealer) open(bucket, key, sealed []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n {
		return nil, errCorrupt
	}
	out, err := s.aead.Open(nil, sealed[:n], sealed[n:], ad(bucket, key))
	if err != nil {
		return nil, errCorrupt
	}
	return out, nil
}

func ad(bucket, key []byte) []byte {
	b := make([]byte, 0, len(bucket)+1+len(key))
	b = append(b, bucket...)
	b = append(b, 0)
	return append(b, key...)
}
