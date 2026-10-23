package engine

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const DefaultEndpoint = "https://pinesandbox.io"

var ErrAuthMissing = errors.New("auth missing")

// AuthStore stores one atomic bundle per endpoint. Add must never overwrite.
// Implementations must not expose secret data in errors.
type AuthStore interface {
	Get(string) ([]byte, error)
	Add(string, []byte) error
}
type authRecord struct {
	Version     int    `json:"version"`
	APIKey      string `json:"api_key"`
	WrappingKey string `json:"wrapping_key"`
}

func authEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return "", Fail("config", "an HTTPS project endpoint without user info, query or fragment is required", 2)
	}
	u.Host = strings.ToLower(u.Host)
	return strings.TrimRight(u.String(), "/"), nil
}
func validAPIKey(key string) bool {
	return key != "" && len(key) <= 16384 && !strings.ContainsAny(key, " \t\r\n\x00")
}
func wrappingKey(key string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(b) != 32 {
		return nil, Fail("vault_key", "a 32-byte base64 vault wrapping key is required", 4)
	}
	return b, nil
}
func readAuth(store AuthStore, endpoint string) (*authRecord, error) {
	raw, err := store.Get(endpoint)
	if errors.Is(err, ErrAuthMissing) {
		return nil, ErrAuthMissing
	}
	if err != nil {
		return nil, Fail("keychain", "cannot read Pine auth from Keychain; requires macOS+cgo and unlocked/allowed Keychain access; otherwise inject environment credentials", 4)
	}
	var r authRecord
	if json.Unmarshal(raw, &r) != nil || r.Version != 1 || !validAPIKey(r.APIKey) {
		return nil, Fail("auth_record", "invalid saved auth; do not replace its wrapping key", 4)
	}
	if _, err = wrappingKey(r.WrappingKey); err != nil {
		return nil, Fail("auth_record", "invalid saved auth; do not replace its wrapping key", 4)
	}
	return &r, nil
}

// SetupAuth only touches local auth storage. It never contacts Pine or creates
// an identity. Existing records are immutable to avoid losing access to vaults.
func SetupAuth(endpoint string, store AuthStore, lookup func(string) (string, bool), prompt func() (string, error)) error {
	endpoint, err := authEndpoint(endpoint)
	if err != nil {
		return err
	}
	r, err := readAuth(store, endpoint)
	if err == nil {
		if key, ok := lookup("PINE_API_KEY"); ok && key != r.APIKey {
			return Fail("auth_conflict", "environment API key differs from saved auth; setup will not overwrite", 6)
		}
		if key, ok := lookup("PINE_VAULT_KEY_BASE64"); ok && key != r.WrappingKey {
			return Fail("auth_conflict", "environment wrapping key differs from saved auth; setup will not rotate it", 6)
		}
		return nil
	}
	if !errors.Is(err, ErrAuthMissing) {
		return err
	}
	key, ok := lookup("PINE_API_KEY")
	if !ok {
		key, err = prompt()
		if err != nil {
			return Fail("auth_input", "API key input unavailable; run auth setup in a terminal or inject PINE_API_KEY", 4)
		}
	}
	if !validAPIKey(key) {
		return Fail("credentials", "a nonempty API key without whitespace is required", 4)
	}
	wrap, ok := lookup("PINE_VAULT_KEY_BASE64")
	if ok {
		if _, err = wrappingKey(wrap); err != nil {
			return err
		}
	} else {
		b := make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			return Fail("auth_random", "cannot generate wrapping key", 4)
		}
		wrap = base64.StdEncoding.EncodeToString(b)
	}
	raw, _ := json.Marshal(authRecord{1, key, wrap})
	if err = store.Add(endpoint, raw); err != nil {
		return Fail("auth_save", "cannot save auth; existing records are never overwritten; inspect Keychain access before retrying setup", 4)
	}
	return nil
}

func resolveAuthValue(endpoint, name string, store AuthStore, lookup func(string) (string, bool)) (string, error) {
	// Validate endpoint even when an environment override is present.
	endpoint, err := authEndpoint(endpoint)
	if err != nil {
		return "", err
	}
	value, ok := lookup(name)
	if !ok {
		r, err := readAuth(store, endpoint)
		if errors.Is(err, ErrAuthMissing) {
			return "", Fail("auth_missing", "run auth setup on macOS or inject PINE_API_KEY and PINE_VAULT_KEY_BASE64", 4)
		}
		if err != nil {
			return "", err
		}
		if name == "PINE_API_KEY" {
			value = r.APIKey
		} else {
			value = r.WrappingKey
		}
	}
	if name == "PINE_API_KEY" {
		if !validAPIKey(value) {
			return "", Fail("credentials", "invalid PINE_API_KEY; an explicit empty value does not fall back to Keychain", 4)
		}
	} else {
		if _, err = wrappingKey(value); err != nil {
			return "", err
		}
	}
	return value, nil
}
func APIKeyForEndpoint(endpoint string) (string, error) {
	return resolveAuthValue(endpoint, "PINE_API_KEY", NativeAuthStore(), os.LookupEnv)
}
func VaultForEndpoint(dir, endpoint string) (*EncryptedVault, error) {
	value, err := resolveAuthValue(endpoint, "PINE_VAULT_KEY_BASE64", NativeAuthStore(), os.LookupEnv)
	if err != nil {
		return nil, err
	}
	key, err := wrappingKey(value)
	if err != nil {
		return nil, err
	}
	return &EncryptedVault{Path: filepath.Join(dir, "vault.enc"), Key: key}, nil
}
