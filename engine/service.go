package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	Store   *Store
	Backend Backend
}

func (s *Service) Bind() error {
	st := &s.Store.State
	if st.Computer != "" && st.Computer != s.Backend.Identity() {
		return Fail("identity", "state belongs to another Computer", 6)
	}
	if st.Runtime != "" && st.Runtime != s.Backend.Runtime() {
		return Fail("runtime", "binding changed runtime; use a new state directory", 6)
	}
	st.Runtime = s.Backend.Runtime()
	if st.Stopped {
		return Fail("stopped", "state belongs to a stopped runtime; use a new directory", 6)
	}
	st.Computer = s.Backend.Identity()
	return s.Store.Save()
}
func (s *Service) Create(ctx context.Context, name string) error {
	st := &s.Store.State
	if st.Session != "" {
		return Fail("session_exists", "session already recorded; use session resume to inspect it", 6)
	}
	st.Session = name
	if e := s.Store.Save(); e != nil {
		return e
	}
	return s.Backend.Create(ctx, name)
}
func (s *Service) Run(ctx context.Context, request, goal string, l Limits) (*Turn, error) {
	st := &s.Store.State
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", goal, l.Tokens, l.Wall)))
	digest := hex.EncodeToString(sum[:])
	r, ok := st.Requests[request]
	if ok && r.Digest != digest {
		return nil, Fail("request_mismatch", "request ID already used with different input or limits", 6)
	}
	if !ok {
		for _, pending := range st.Requests {
			if pending.Turn == "" {
				return nil, Fail("pending_request", "resolve saved unconfirmed request with the same request ID and input first", 7)
			}
		}
		r = Submission{Key: ID(), Digest: digest}
		st.Requests[request] = r
		if e := s.Store.Save(); e != nil {
			return nil, e
		}
	}
	t, e := s.Backend.Run(ctx, st.Session, goal, r.Key, l)
	if e != nil {
		return nil, e
	}
	r.Turn = t.ID
	st.Requests[request] = r
	return s.Remember(t)
}
func (s *Service) Remember(t *Turn) (*Turn, error) {
	if t != nil {
		s.Store.State.Turn = t.ID
	}
	return t, s.Store.Save()
}
func (s *Service) Read(ctx context.Context, id string, w time.Duration) (*Turn, error) {
	t, e := s.Backend.Read(ctx, s.Store.State.Session, id, w)
	if e != nil {
		return nil, e
	}
	return s.Remember(t)
}
func (s *Service) Pull(ctx context.Context, out string) (int, error) {
	a, e := s.Backend.Artifacts(ctx, s.Store.State.Session)
	if e != nil {
		return 0, e
	}
	if e = os.MkdirAll(out, 0700); e != nil {
		return 0, e
	}
	root, e := os.OpenRoot(out)
	if e != nil {
		return 0, e
	}
	defer root.Close()
	count := 0
	for _, v := range a {
		if e = ctx.Err(); e != nil {
			return count, e
		}
		if v.Name == "" || v.Name == "." || v.Name == ".." || strings.ContainsAny(v.Name, "/\\\x00") {
			return count, Fail("artifact_path", "unsafe artifact filename", 6)
		}
		key := sha256.Sum256([]byte(s.Store.State.Session + "\x00" + v.ID))
		dir := hex.EncodeToString(key[:])
		if e = root.MkdirAll(dir, 0700); e != nil {
			return count, e
		}
		dest := filepath.Join(dir, v.Name)
		tmp := filepath.Join(dir, ".part-"+ID())
		f, e := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return count, e
		}
		r, e := s.Backend.Open(ctx, s.Store.State.Session, v.ID)
		if e != nil {
			f.Close()
			root.Remove(tmp)
			return count, e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, 256*1024*1024+1))
		re := r.Close()
		if e == nil {
			e = re
		}
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e == nil {
			e = ce
		}
		hash := hex.EncodeToString(h.Sum(nil))
		if e == nil && (n > 256*1024*1024 || (v.Size >= 0 && v.Size != n) || (v.SHA256 != "" && !strings.EqualFold(v.SHA256, hash))) {
			e = Fail("artifact_integrity", "artifact exceeds 256 MiB or size/checksum mismatch", 6)
		}
		if e != nil {
			root.Remove(tmp)
			return count, e
		}
		// Link is no-clobber; a previous download is accepted only after verification.
		e = root.Link(tmp, dest)
		if os.IsExist(e) {
			old, er := root.Open(dest)
			if er != nil {
				root.Remove(tmp)
				return count, er
			}
			oh := sha256.New()
			_, er = io.Copy(oh, io.LimitReader(old, 256*1024*1024+1))
			old.Close()
			if er == nil && hex.EncodeToString(oh.Sum(nil)) == hash {
				e = nil
			} else {
				e = Fail("artifact_conflict", "existing artifact differs; choose another output directory", 6)
			}
		}
		root.Remove(tmp)
		if e != nil {
			return count, e
		}
		d, e := root.Open(dir)
		if e != nil {
			return count, e
		}
		e = d.Sync()
		d.Close()
		if e != nil {
			return count, e
		}
		abs, e := filepath.Abs(filepath.Join(out, dest))
		if e != nil {
			return count, e
		}
		s.Store.State.Saved[v.ID] = abs
		if e = s.Store.Save(); e != nil {
			return count, e
		}
		count++
	}
	return count, nil
}
func (s *Service) Stop(ctx context.Context, out string) error {
	if s.Store.State.Session != "" {
		t, e := s.Backend.Read(ctx, s.Store.State.Session, "", 0)
		if e != nil {
			return e
		}
		if t != nil && t.State != "completed" && t.State != "failed" && t.State != "canceled" {
			return Fail("active_turn", "finish or cancel current turn and wait before stopping", 6)
		}
		if _, e := s.Pull(ctx, out); e != nil {
			return e
		}
	}
	if e := s.Backend.Stop(ctx, s.Store.State.Session); e != nil {
		return e
	}
	s.Store.State.Stopped = true
	return s.Store.Save()
}
