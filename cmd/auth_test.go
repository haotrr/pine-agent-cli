package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"pine-agent-cli/engine"
)

type authTestStore struct {
	raw  []byte
	adds int
	fail bool
}

func (s *authTestStore) Get(string) ([]byte, error) {
	if s.raw == nil {
		return nil, engine.ErrAuthMissing
	}
	return s.raw, nil
}
func (s *authTestStore) Add(_ string, b []byte) error {
	if s.fail {
		return errors.New("synthetic-secret-error")
	}
	s.raw = append([]byte(nil), b...)
	s.adds++
	return nil
}
func TestAuthSetupCommandLocalAndSecretFree(t *testing.T) {
	for _, fail := range []bool{false, true} {
		store := &authTestStore{fail: fail}
		endpoint := engine.DefaultEndpoint
		var out bytes.Buffer
		root := &cobra.Command{Use: "test", SilenceErrors: true, SilenceUsage: true}
		root.AddCommand(authCommand(&endpoint, store, func(string) (string, bool) { return "", false }, func() (string, error) { return "synthetic-secret-api", nil }))
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"auth", "setup"})
		err := root.Execute()
		if (err != nil) != fail {
			t.Fatalf("unexpected outcome: %v", err)
		}
		if strings.Contains(out.String(), "synthetic-secret") || (err != nil && strings.Contains(err.Error(), "synthetic-secret")) {
			t.Fatal("secret output")
		}
		if !fail {
			var result map[string]any
			if json.Unmarshal(out.Bytes(), &result) != nil || result["ok"] != true || store.adds != 1 {
				t.Fatal("invalid success")
			}
		}
	}
}
func TestAuthDefaultsAndHelp(t *testing.T) {
	root := NewRoot()
	endpoint, _ := root.PersistentFlags().GetString("endpoint")
	if endpoint != engine.DefaultEndpoint {
		t.Fatal("wrong endpoint default")
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"auth", "setup", "--help"})
	if err := root.Execute(); err != nil || !strings.Contains(out.String(), "Keychain") {
		t.Fatal("help failed")
	}
}
