package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"pine-agent-cli/engine"
)

func authCommand(endpoint *string, store engine.AuthStore, lookup func(string) (string, bool), prompt func() (string, error)) *cobra.Command {
	auth := &cobra.Command{Use: "auth", Short: "Manage local authentication; never contacts Pine"}
	setup := &cobra.Command{Use: "setup", Short: "Save API key and vault wrapping key in macOS Keychain", Args: cobra.NoArgs}
	setup.RunE = func(c *cobra.Command, _ []string) error {
		read := prompt
		if read == nil {
			read = func() (string, error) {
				fd := int(os.Stdin.Fd())
				if !term.IsTerminal(fd) {
					return "", fmt.Errorf("terminal required")
				}
				fmt.Fprint(c.ErrOrStderr(), "Pine API key (hidden): ")
				b, err := term.ReadPassword(fd)
				fmt.Fprintln(c.ErrOrStderr())
				if err != nil {
					return "", err
				}
				value := string(b)
				clear(b)
				return value, nil
			}
		}
		if err := engine.SetupAuth(*endpoint, store, lookup, read); err != nil {
			return err
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"ok": true, "data": map[string]any{"configured": true, "storage": "macOS Keychain", "remote_checked": false}})
	}
	auth.AddCommand(setup)
	return auth
}
