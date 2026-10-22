package engine

import (
	"bytes"
	"context"
	"errors"
	pine "go.pinesandbox.io/computer"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Fixed, synthetic fixtures. They are never sent to any service.
func fakeRecord() *VaultRecord {
	id := "00000000-0000-7000-8000-000000000001"
	return &VaultRecord{Endpoint: "https://example.invalid", Credentials: pine.Credentials{ID: id, Key: bytes.Repeat([]byte{42}, 32), Ephemeral: true}, Binding: &pine.Binding{ComputerID: id, SandboxID: "fake-runtime", ComputerToken: "ct_FAKE_NOT_A_REAL_TOKEN", BindingRevision: 1}, Phase: "ready"}
}
func vaultFixture(t *testing.T) *EncryptedVault {
	t.Helper()
	return &EncryptedVault{Path: filepath.Join(t.TempDir(), "vault.enc"), Key: bytes.Repeat([]byte{7}, 32)}
}
func TestVaultEncryptedAuthenticated(t *testing.T) {
	v := vaultFixture(t)
	r := fakeRecord()
	if e := v.Save(r); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(v.Path)
	if bytes.Contains(b, []byte("ct_FAKE")) || bytes.Contains(b, []byte(r.Credentials.ID)) {
		t.Fatal("plaintext leaked")
	}
	got, e := v.Load()
	if e != nil || got.Binding.ComputerToken != r.Binding.ComputerToken {
		t.Fatal(e)
	}
	v.Key[0]++
	if _, e = v.Load(); e == nil {
		t.Fatal("wrong key accepted")
	}
	v.Key[0]--
	b[len(b)-1] ^= 1
	os.WriteFile(v.Path, b, 0600)
	if _, e = v.Load(); e == nil {
		t.Fatal("tampering accepted")
	}
	info, _ := os.Stat(v.Path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("bad permissions")
	}
}
func TestStartJournalAndReplay(t *testing.T) {
	v := vaultFixture(t)
	r := fakeRecord()
	calls := 0
	start := func(_ context.Context, c pine.Credentials, _ time.Duration) (pine.Credentials, pine.Binding, error) {
		calls++
		saved, e := v.Load()
		if e != nil || saved.Phase != "pending" || saved.Credentials.ID != c.ID {
			t.Fatal("credentials/pending not durable before start")
		}
		return c, *r.Binding, nil
	}
	for i := 0; i < 2; i++ {
		if _, e := StartStored(context.Background(), v, r.Endpoint, r.Credentials, time.Minute, start); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 1 {
		t.Fatal("replayed start")
	}
}
func TestUncertainStartCannotRetry(t *testing.T) {
	v := vaultFixture(t)
	r := fakeRecord()
	calls := 0
	start := func(_ context.Context, c pine.Credentials, _ time.Duration) (pine.Credentials, pine.Binding, error) {
		calls++
		return c, pine.Binding{}, errors.New("secret upstream failure")
	}
	for i := 0; i < 2; i++ {
		if _, e := StartStored(context.Background(), v, r.Endpoint, r.Credentials, time.Minute, start); e == nil {
			t.Fatal("expected uncertain outcome")
		}
	}
	if calls != 1 {
		t.Fatal("unsafe retry")
	}
	bad := *r.Binding
	bad.ComputerID = "00000000-0000-7000-8000-000000000002"
	if e := Reconcile(v, r.Endpoint, bad); e == nil {
		t.Fatal("accepted other identity")
	}
	if e := Reconcile(v, r.Endpoint, *r.Binding); e != nil {
		t.Fatal(e)
	}
	if _, e := StartStored(context.Background(), v, r.Endpoint, r.Credentials, time.Minute, start); e != nil || calls != 1 {
		t.Fatal("reconcile caused another start", e)
	}
}

type failingVault struct {
	record *VaultRecord
	calls  int
	failAt int
}

func (v *failingVault) Load() (*VaultRecord, error) {
	if v.record == nil {
		return nil, ErrVaultMissing
	}
	r := *v.record
	return &r, nil
}
func (v *failingVault) Save(r *VaultRecord) error {
	v.calls++
	if v.calls == v.failAt {
		return errors.New("disk failure")
	}
	copy := *r
	v.record = &copy
	return nil
}
func TestStartPersistenceFailure(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		v := &failingVault{failAt: failAt}
		r := fakeRecord()
		calls := 0
		_, e := StartStored(context.Background(), v, r.Endpoint, r.Credentials, time.Minute, func(_ context.Context, c pine.Credentials, _ time.Duration) (pine.Credentials, pine.Binding, error) {
			calls++
			return c, *r.Binding, nil
		})
		if e == nil {
			t.Fatal("expected storage failure")
		}
		if failAt < 3 && calls != 0 {
			t.Fatal("started before durable pending")
		}
		if failAt == 3 && v.record.Phase != "pending" {
			t.Fatal("lost uncertainty marker")
		}
	}
}
func TestPrivateStateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "unsafe")
	os.Mkdir(dir, 0755)
	if _, e := Load(dir, "mock"); e == nil {
		t.Fatal("accepted public state directory")
	}
}
