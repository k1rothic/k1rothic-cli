// Package secrets stores provider API keys in the OS keychain (macOS
// Keychain, Windows Credential Manager, Secret Service on Linux).
//
// Keys are never written to disk by k1rothic. When no keychain is available,
// a key can be supplied for the session through K1ROTHIC_<PROVIDER>_API_KEY;
// it is read from the environment only and never persisted.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

// Service is the keychain service name all keys are stored under.
const Service = "k1rothic"

// Source says where a key came from.
type Source int

const (
	None Source = iota
	Keychain
	Env
)

func (s Source) String() string {
	switch s {
	case Keychain:
		return "keychain"
	case Env:
		return "env"
	default:
		return "none"
	}
}

// Store reads and writes keys. Tests substitute a fake.
type Store interface {
	Get(provider string) (key string, src Source, err error)
	Set(provider, key string) error
	Delete(provider string) error
}

// OS is the keychain-backed store with the environment fallback.
type OS struct{}

// EnvVar is the session-only environment variable for provider.
func EnvVar(provider string) string {
	p := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", ":", "_").Replace(provider))
	return "K1ROTHIC_" + p + "_API_KEY"
}

// Get returns the key from the keychain, falling back to the environment.
// A missing key returns ("", None, nil); keychain errors other than "not
// found" are returned only when the environment has no key either.
func (OS) Get(provider string) (string, Source, error) {
	k, err := keyring.Get(Service, provider)
	if err == nil && k != "" {
		return k, Keychain, nil
	}
	if v := strings.TrimSpace(os.Getenv(EnvVar(provider))); v != "" {
		return v, Env, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return "", None, fmt.Errorf("keychain: %w", err)
	}
	return "", None, nil
}

// Set stores key in the keychain.
func (OS) Set(provider, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("empty key")
	}
	if err := keyring.Set(Service, provider, key); err != nil {
		return fmt.Errorf("keychain unavailable (%v); set %s for this session instead", err, EnvVar(provider))
	}
	return nil
}

// Delete removes the key from the keychain. A missing key is not an error.
func (OS) Delete(provider string) error {
	if err := keyring.Delete(Service, provider); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keychain: %w", err)
	}
	return nil
}

// Mem is an in-memory store for tests.
type Mem map[string]string

func (m Mem) Get(p string) (string, Source, error) {
	if k, ok := m[p]; ok {
		return k, Keychain, nil
	}
	return "", None, nil
}
func (m Mem) Set(p, k string) error { m[p] = k; return nil }
func (m Mem) Delete(p string) error { delete(m, p); return nil }
