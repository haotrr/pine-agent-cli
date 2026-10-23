package engine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeAuthStore struct {
	records        map[string][]byte
	gets, adds     int
	getErr, addErr error
}

func (s *fakeAuthStore) Get(k string) ([]byte, error) {
	s.gets++
	if s.getErr != nil {
		return nil, s.getErr
	}
	b, ok := s.records[k]
	if !ok {
		return nil, ErrAuthMissing
	}
	return b, nil
}
func (s *fakeAuthStore) Add(k string, b []byte) error {
	s.adds++
	if s.addErr != nil {
		return s.addErr
	}
	if s.records == nil {
		s.records = map[string][]byte{}
	}
	if _, ok := s.records[k]; ok {
		return errors.New("exists")
	}
	s.records[k] = append([]byte(nil), b...)
	return nil
}
func lookupAuth(values map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := values[k]; return v, ok }
}
func noPrompt() (string, error) { panic("unexpected prompt") }
func TestSetupAuthAtomicAndIdempotent(t *testing.T) {
	s := &fakeAuthStore{}
	prompts := 0
	prompt := func() (string, error) { prompts++; return "synthetic-api", nil }
	if err := SetupAuth(DefaultEndpoint+"/", s, lookupAuth(nil), prompt); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), s.records[DefaultEndpoint]...)
	var r authRecord
	if err := json.Unmarshal(original, &r); err != nil {
		t.Fatal(err)
	}
	b, err := wrappingKey(r.WrappingKey)
	if err != nil || len(b) != 32 || bytes.Equal(b, make([]byte, 32)) || r.APIKey != "synthetic-api" {
		t.Fatal("invalid generated record")
	}
	if err := SetupAuth(DefaultEndpoint, s, lookupAuth(nil), noPrompt); err != nil {
		t.Fatal(err)
	}
	if prompts != 1 || s.adds != 1 || !bytes.Equal(original, s.records[DefaultEndpoint]) {
		t.Fatal("setup rotated credentials")
	}
	for _, name := range []string{"PINE_API_KEY", "PINE_VAULT_KEY_BASE64"} {
		if SetupAuth(DefaultEndpoint, s, lookupAuth(map[string]string{name: "changed"}), noPrompt) == nil {
			t.Fatal("conflict accepted")
		}
	}
	if s.adds != 1 {
		t.Fatal("conflict overwrote record")
	}
}
func TestSetupAuthImportAndFailures(t *testing.T) {
	wrap := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	env := lookupAuth(map[string]string{"PINE_API_KEY": "synthetic-api", "PINE_VAULT_KEY_BASE64": wrap})
	s := &fakeAuthStore{}
	if err := SetupAuth(DefaultEndpoint, s, env, noPrompt); err != nil {
		t.Fatal(err)
	}
	r, err := readAuth(s, DefaultEndpoint)
	if err != nil || r.WrappingKey != wrap {
		t.Fatal("import changed wrapping key")
	}
	for _, tc := range []struct {
		name   string
		s      *fakeAuthStore
		values map[string]string
	}{
		{"empty", &fakeAuthStore{}, map[string]string{"PINE_API_KEY": ""}},
		{"newline", &fakeAuthStore{}, map[string]string{"PINE_API_KEY": "secret\n"}},
		{"bad wrap", &fakeAuthStore{}, map[string]string{"PINE_API_KEY": "synthetic-api", "PINE_VAULT_KEY_BASE64": "bad"}},
		{"denied", &fakeAuthStore{getErr: errors.New("secret-error")}, nil},
		{"corrupt", &fakeAuthStore{records: map[string][]byte{DefaultEndpoint: []byte("secret-corrupt")}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := SetupAuth(DefaultEndpoint, tc.s, lookupAuth(tc.values), noPrompt)
			if err == nil || strings.Contains(err.Error(), "secret-") || tc.s.adds != 0 {
				t.Fatal("unsafe setup failure")
			}
		})
	}
	s = &fakeAuthStore{addErr: errors.New("secret-error")}
	if err := SetupAuth(DefaultEndpoint, s, env, noPrompt); err == nil || strings.Contains(err.Error(), "secret-error") || len(s.records) != 0 {
		t.Fatal("unsafe save failure")
	}
	s = &fakeAuthStore{}
	if SetupAuth(DefaultEndpoint, s, lookupAuth(nil), func() (string, error) { return "", errors.New("secret-error") }) == nil || s.adds != 0 {
		t.Fatal("input error ignored")
	}
}
func TestAuthResolutionPriorityAndIsolation(t *testing.T) {
	wrap := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	s := &fakeAuthStore{}
	if err := SetupAuth(DefaultEndpoint, s, lookupAuth(map[string]string{"PINE_API_KEY": "stored-api", "PINE_VAULT_KEY_BASE64": wrap}), noPrompt); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"PINE_API_KEY", "PINE_VAULT_KEY_BASE64"} {
		expected := "stored-api"
		override := "override-api"
		if name != "PINE_API_KEY" {
			expected = wrap
			override = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
		}
		v, err := resolveAuthValue(DefaultEndpoint, name, s, lookupAuth(nil))
		if err != nil || v != expected {
			t.Fatal("saved lookup failed")
		}
		before := s.gets
		v, err = resolveAuthValue(DefaultEndpoint, name, s, lookupAuth(map[string]string{name: override}))
		if err != nil || v != override || s.gets != before {
			t.Fatal("override accessed Keychain")
		}
		if _, err = resolveAuthValue(DefaultEndpoint, name, s, lookupAuth(map[string]string{name: ""})); err == nil || s.gets != before {
			t.Fatal("empty override fell back")
		}
		if _, err = resolveAuthValue("https://different.example", name, s, lookupAuth(nil)); err == nil {
			t.Fatal("cross endpoint reuse")
		}
	}
	for _, endpoint := range []string{"", "http://host", "https://user:secret@host", "https://host?key=secret", "https://host#secret", "https:///missing"} {
		before := s.gets
		if SetupAuth(endpoint, s, lookupAuth(nil), noPrompt) == nil || s.gets != before {
			t.Fatal("invalid endpoint reached store")
		}
	}
}
func TestEnvironmentVaultCompatibility(t *testing.T) {
	wrap := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	t.Setenv("PINE_API_KEY", "synthetic-api")
	t.Setenv("PINE_VAULT_KEY_BASE64", wrap)
	key, err := APIKeyForEndpoint(DefaultEndpoint)
	if err != nil || key != "synthetic-api" {
		t.Fatal("env API failed")
	}
	dir := t.TempDir()
	old, err := VaultFromEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	current, err := VaultForEndpoint(dir, DefaultEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	r := fakeRecord()
	r.Endpoint, r.Phase, r.Operation = DefaultEndpoint, "pending", "create"
	if err = old.Save(r); err != nil {
		t.Fatal(err)
	}
	loaded, err := current.Load()
	if err != nil || loaded.Phase != "pending" {
		t.Fatal("legacy vault lost")
	}
	t.Setenv("PINE_VAULT_KEY_BASE64", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)))
	wrong, err := VaultForEndpoint(dir, DefaultEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrong.Load(); err == nil {
		t.Fatal("wrong key accepted")
	}
	loaded, err = old.Load()
	if err != nil || loaded.Phase != "pending" {
		t.Fatal("wrong key changed vault")
	}
}
