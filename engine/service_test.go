package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Service, *Mock) {
	t.Helper()
	st, e := Load(filepath.Join(t.TempDir(), "state"), "mock")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(st.Close)
	m, e := NewMock(st.Dir)
	if e != nil {
		t.Fatal(e)
	}
	s := &Service{st, m}
	if e = s.Bind(); e != nil {
		t.Fatal(e)
	}
	if e = s.Create(context.Background(), "test"); e != nil {
		t.Fatal(e)
	}
	return s, m
}

type lost struct {
	*Mock
	first bool
	store *Store
}

func (m *lost) Run(ctx context.Context, s, g, k string, l Limits) (*Turn, error) {
	b, e := os.ReadFile(filepath.Join(m.store.Dir, "state.json"))
	if e != nil || len(b) == 0 {
		return nil, errors.New("journal absent")
	}
	var disk State
	if json.Unmarshal(b, &disk) != nil {
		return nil, errors.New("invalid journal")
	}
	found := false
	for _, r := range disk.Requests {
		if r.Key == k {
			found = true
		}
	}
	if !found {
		return nil, errors.New("key not saved before submission")
	}
	v, e := m.Mock.Run(ctx, s, g, k, l)
	if m.first {
		m.first = false
		return nil, Fail("unconfirmed", "lost response", 7)
	}
	return v, e
}
func TestRunLostResponseAndDigest(t *testing.T) {
	s, m := fixture(t)
	s.Backend = &lost{m, true, s.Store}
	ctx := context.Background()
	l := Limits{10, time.Second}
	if _, e := s.Run(ctx, "r1", "goal", l); e == nil {
		t.Fatal("expected loss")
	}
	if s.Store.State.Requests["r1"].Key == "" {
		t.Fatal("key not persisted")
	}
	s.Backend = m
	v, e := s.Run(ctx, "r1", "goal", l)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Data.Turns) != 1 {
		t.Fatal("duplicate run")
	}
	w, e := s.Run(ctx, "r1", "goal", l)
	if e != nil || v.ID != w.ID {
		t.Fatal("not idempotent")
	}
	if _, e = s.Run(ctx, "r1", "changed", l); e == nil {
		t.Fatal("accepted changed request")
	}
}
func TestAnswerWaitCancel(t *testing.T) {
	s, m := fixture(t)
	ctx := context.Background()
	v, e := s.Run(ctx, "q", "[ask]", Limits{})
	if e != nil || v.State != "needs_input" {
		t.Fatal(e)
	}
	n, e := m.Answer(ctx, "test", v.ID, "yes", Limits{})
	if e != nil {
		t.Fatal(e)
	}
	again, e := m.Answer(ctx, "test", v.ID, "yes", Limits{})
	if e != nil || n.ID != again.ID {
		t.Fatal("answer not idempotent")
	}
	old, _ := m.Read(ctx, "test", v.ID, 0)
	if old.State != "answered" || old.Next != n.ID {
		t.Fatal("missing continuation")
	}
	if _, e = m.Answer(ctx, "test", v.ID, "no", Limits{}); e == nil {
		t.Fatal("accepted stale answer")
	}
	c, _ := m.Cancel(ctx, "test", n.ID)
	if c.State != "canceling" {
		t.Fatal(c)
	}
	c, e = m.Read(ctx, "test", n.ID, time.Second)
	if e != nil || c.State != "canceled" {
		t.Fatal(c, e)
	}
}
func TestPullBeforeStopAndRestart(t *testing.T) {
	s, m := fixture(t)
	ctx := context.Background()
	v, _ := s.Run(ctx, "r", "goal", Limits{})
	out := t.TempDir()
	if e := s.Stop(ctx, out); e == nil {
		t.Fatal("stopped running task")
	}
	s.Read(ctx, v.ID, time.Second)
	if n, e := s.Pull(ctx, out); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	for _, p := range s.Store.State.Saved {
		if b, e := os.ReadFile(p); e != nil || string(b) != mockContent {
			t.Fatal(e)
		}
	}
	if e := s.Stop(ctx, out); e != nil {
		t.Fatal(e)
	}
	if !m.Data.Stopped || !s.Store.State.Stopped {
		t.Fatal("stop not persisted")
	}
	if e := s.Bind(); e == nil {
		t.Fatal("reused stopped state")
	}
}
func TestLockAndMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	a, e := Load(dir, "mock")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Load(dir, "mock"); e == nil {
		t.Fatal("lock not exclusive")
	}
	a.Save()
	a.Close()
	if _, e = Load(dir, "live"); e == nil {
		t.Fatal("mixed backends")
	}
}

type unsafeArtifacts struct {
	*Mock
	name     string
	checksum string
}

func (m unsafeArtifacts) Artifacts(context.Context, string) ([]Artifact, error) {
	return []Artifact{{ID: "a", Name: m.name, Size: int64(len(mockContent)), SHA256: m.checksum}}, nil
}
func TestArtifactGuards(t *testing.T) {
	for _, name := range []string{"../escape", "a/b", "..", "a\\b"} {
		t.Run(name, func(t *testing.T) {
			s, m := fixture(t)
			s.Backend = unsafeArtifacts{m, name, ""}
			if _, e := s.Pull(context.Background(), t.TempDir()); e == nil {
				t.Fatal("accepted unsafe path")
			}
		})
	}
	s, m := fixture(t)
	s.Backend = unsafeArtifacts{m, "safe.txt", "bad"}
	if _, e := s.Pull(context.Background(), t.TempDir()); e == nil {
		t.Fatal("accepted bad checksum")
	}
}
func TestFailedDownloadNeverStops(t *testing.T) {
	s, m := fixture(t)
	v, _ := s.Run(context.Background(), "r", "g", Limits{})
	s.Read(context.Background(), v.ID, time.Second)
	s.Backend = unsafeArtifacts{m, "../escape", ""}
	if e := s.Stop(context.Background(), t.TempDir()); e == nil || m.Data.Stopped {
		t.Fatal("stop after failed collection")
	}
}

type differentRuntime struct{ *Mock }

func (d differentRuntime) Runtime() string { return "another-runtime" }
func TestRuntimeMismatch(t *testing.T) {
	s, m := fixture(t)
	s.Backend = differentRuntime{m}
	if e := s.Bind(); e == nil {
		t.Fatal("reused old runtime state")
	}
}
func TestPendingRequestBlocksNewID(t *testing.T) {
	s, m := fixture(t)
	s.Backend = &lost{m, true, s.Store}
	s.Run(context.Background(), "first", "goal", Limits{})
	if _, e := s.Run(context.Background(), "second", "goal", Limits{}); e == nil {
		t.Fatal("new request after uncertain outcome")
	}
}
func TestArtifactSymlinkEscape(t *testing.T) {
	s, m := fixture(t)
	v, _ := s.Run(context.Background(), "r", "g", Limits{})
	s.Read(context.Background(), v.ID, time.Second)
	out := t.TempDir()
	if _, e := s.Pull(context.Background(), out); e != nil {
		t.Fatal(e)
	}
	for _, path := range s.Store.State.Saved {
		if e := os.Remove(path); e != nil {
			t.Fatal(e)
		}
		outside := filepath.Join(t.TempDir(), "secret")
		os.WriteFile(outside, []byte("unchanged"), 0600)
		os.Symlink(outside, path)
		if _, e := s.Pull(context.Background(), out); e == nil {
			t.Fatal("followed symlink outside root")
		}
		b, _ := os.ReadFile(outside)
		if string(b) != "unchanged" {
			t.Fatal("overwrote external file")
		}
	}
	_ = m
}

func TestStopIdleComputer(t *testing.T) {
	s, m := fixture(t)
	s.Store.State.Session = ""
	m.Data.Session = ""
	if e := s.Stop(context.Background(), t.TempDir()); e != nil || !m.Data.Stopped {
		t.Fatal("cannot release idle resource", e)
	}
}
