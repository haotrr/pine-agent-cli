package engine

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	pine "go.pinesandbox.io/computer"
	"os"
	"path/filepath"
)

// Vault can be replaced by a platform Keychain or remote secret-store provider.
// Implementations must replace records atomically and never log their contents.
type Vault interface {
	Load() (*VaultRecord, error)
	Save(*VaultRecord) error
}

var ErrVaultMissing = errors.New("vault missing")

type VaultRecord struct {
	Operation   string `json:",omitempty"` // Empty means legacy start.
	Endpoint    string
	Credentials pine.Credentials
	Binding     *pine.Binding
	Phase       string
}
type EncryptedVault struct {
	Path string
	Key  []byte
}

func VaultFromEnv(dir string) (*EncryptedVault, error) {
	key, e := base64.StdEncoding.DecodeString(os.Getenv("PINE_VAULT_KEY_BASE64"))
	if e != nil || len(key) != 32 {
		return nil, Fail("vault_key", "inject a 32-byte base64 vault wrapping key from a trusted secret provider", 4)
	}
	return &EncryptedVault{filepath.Join(dir, "vault.enc"), key}, nil
}
func (v *EncryptedVault) aead() (cipher.AEAD, error) {
	if len(v.Key) != 32 {
		return nil, Fail("vault_key", "vault requires a 32-byte wrapping key", 4)
	}
	b, e := aes.NewCipher(v.Key)
	if e != nil {
		return nil, Fail("vault_key", "invalid vault wrapping key", 4)
	}
	return cipher.NewGCM(b)
}
func (v *EncryptedVault) Load() (*VaultRecord, error) {
	b, e := os.ReadFile(v.Path)
	if os.IsNotExist(e) {
		return nil, ErrVaultMissing
	}
	if e != nil {
		return nil, Fail("vault", "cannot read encrypted vault", 6)
	}
	a, e := v.aead()
	if e != nil {
		return nil, e
	}
	if len(b) < a.NonceSize() {
		return nil, Fail("vault", "invalid encrypted vault", 6)
	}
	plain, e := a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], []byte("pine-agent-vault-v1"))
	if e != nil {
		return nil, Fail("vault", "vault authentication failed", 4)
	}
	var r VaultRecord
	if json.Unmarshal(plain, &r) != nil {
		return nil, Fail("vault", "invalid vault record", 6)
	}
	return &r, nil
}
func (v *EncryptedVault) Save(r *VaultRecord) error {
	a, e := v.aead()
	if e != nil {
		return e
	}
	plain, e := json.Marshal(r)
	if e != nil {
		return Fail("vault", "invalid vault record", 6)
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	sealed := a.Seal(nonce, nonce, plain, []byte("pine-agent-vault-v1"))
	f, e := os.CreateTemp(filepath.Dir(v.Path), ".vault-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(sealed); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), v.Path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(v.Path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
