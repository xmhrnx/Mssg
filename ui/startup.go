package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gioapp "gioui.org/app"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

const (
	secureService = "app.mssg"
	keyName       = "database-key"
)

// openData opens the app's folder and its encrypted database. The key
// lives in the platform's secure storage (Keychain, Android Keystore,
// DPAPI, Secret Service); where there is none, in a private file. It
// blocks, so call it off the UI goroutine.
func openData() (string, *store.Store, error) {
	base, err := gioapp.DataDir()
	if err != nil {
		return "", nil, fmt.Errorf("app folder: %w", err)
	}
	dir := filepath.Join(base, "mssg")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, fmt.Errorf("app folder: %w", err)
	}
	key, err := databaseKey(dir)
	if err != nil {
		return "", nil, err
	}
	st, err := store.Open(filepath.Join(dir, "mssg.db"), key)
	if err != nil {
		return "", nil, err
	}
	return dir, st, nil
}

func databaseKey(dir string) ([]byte, error) {
	fallback := filepath.Join(dir, "key.bin")
	if k, err := os.ReadFile(fallback); err == nil && len(k) == store.KeySize {
		return k, nil
	}
	giode.SecureStore.Service = secureService
	k, err := giode.SecureStore.Get(keyName)
	if err == nil && len(k) == store.KeySize {
		return k, nil
	}
	if err != nil && !errors.Is(err, giode.ErrNotFound) {
		// No usable secure storage on this system (a desktop without
		// a keyring): keep a private file instead.
		k, err := store.NewKey()
		if err != nil {
			return nil, err
		}
		return k, os.WriteFile(fallback, k, 0o600)
	}
	k, err = store.NewKey()
	if err != nil {
		return nil, err
	}
	if err := giode.SecureStore.Set(keyName, k); err != nil {
		return k, os.WriteFile(fallback, k, 0o600)
	}
	return k, nil
}
