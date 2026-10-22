package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
)

type Submission struct {
	Key    string `json:"key"`
	Digest string `json:"digest"`
	Turn   string `json:"turn_id,omitempty"`
}
type State struct {
	Runtime  string                `json:"runtime_id"`
	Version  int                   `json:"version"`
	Mode     string                `json:"mode"`
	Computer string                `json:"computer_id"`
	Session  string                `json:"session"`
	Turn     string                `json:"turn_id"`
	Stopped  bool                  `json:"stopped"`
	Requests map[string]Submission `json:"requests"`
	Saved    map[string]string     `json:"saved_artifacts"`
}
type Store struct {
	Dir   string
	lock  *os.File
	State State
}

func Load(dir, mode string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, Fail("state_permissions", "state directory must be a real private directory (0700)", 6)
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, Fail("busy", "another command holds this state directory", 6)
	}
	s := &Store{Dir: dir, lock: f, State: State{Version: 1, Mode: mode, Requests: map[string]Submission{}, Saved: map[string]string{}}}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err == nil {
		err = json.Unmarshal(b, &s.State)
	}
	if err != nil && !os.IsNotExist(err) {
		s.Close()
		return nil, Fail("state", "cannot read state", 6)
	}
	if s.State.Version != 1 || s.State.Mode != mode {
		s.Close()
		return nil, Fail("state", "state version or backend mismatch; use a separate directory", 6)
	}
	if s.State.Requests == nil {
		s.State.Requests = map[string]Submission{}
	}
	if s.State.Saved == nil {
		s.State.Saved = map[string]string{}
	}
	return s, nil
}
func (s *Store) Close()      { syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN); s.lock.Close() }
func (s *Store) Save() error { return AtomicJSON(filepath.Join(s.Dir, "state.json"), s.State) }
func AtomicJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".save-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func ID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
