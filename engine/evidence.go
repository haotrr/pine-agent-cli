package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	pine "go.pinesandbox.io/computer"
)

// Consumer-side seams retain the real SDK signatures. Tests never start a client.
type pineComputer interface {
	ID() string
	SandboxID() string
	Session(context.Context, string) (*pine.Session, error)
	CreateSession(context.Context, pine.CreateSessionOptions) (*pine.Session, error)
	Sessions(context.Context) ([]*pine.Session, error)
	RequireAgentProtocol(context.Context) error
	Stop(context.Context) (bool, error)
}

var metadataPattern = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,256}$`)

func (p *Pine) metadata(s string) string {
	if !metadataPattern.MatchString(s) {
		return ""
	}
	for _, secret := range p.secrets {
		if secret != "" && strings.Contains(s, secret) {
			return ""
		}
	}
	// Reject credential-shaped values even when not part of this process's inputs.
	for _, prefix := range []string{"ct_", "ps_", "sk_", "pk_"} {
		if strings.HasPrefix(s, prefix) {
			return ""
		}
	}
	return s
}
func safeWorkdirPath(s string) bool {
	return s != "" && s != "." && !strings.HasPrefix(s, "/") && path.Clean(s) == s && s != ".." && !strings.HasPrefix(s, "../") && !strings.ContainsAny(s, "\\\x00:")
}
func workdirArtifacts(turns []pine.AgentTurn) ([]Artifact, error) {
	var out []Artifact
	for _, t := range turns {
		if t.Result == nil {
			continue
		}
		for _, f := range t.Result.Artifacts {
			if f.Root != "workdir" || !safeWorkdirPath(f.RelativePath) {
				return nil, Fail("artifact_path", "unsupported result file root or unsafe workdir path", 6)
			}
			sum := sha256.Sum256([]byte(t.TurnID + "\x00" + f.RelativePath))
			out = append(out, Artifact{ID: "workdir:" + hex.EncodeToString(sum[:]), Name: f.RelativePath, Path: f.RelativePath, Source: "workdir", Size: f.Size, SHA256: f.SHA256})
		}
	}
	return out, nil
}
func (p *Pine) OpenFile(ctx context.Context, session, name string) (io.ReadCloser, error) {
	if !safeWorkdirPath(name) {
		return nil, Fail("artifact_path", "unsafe workdir path", 6)
	}
	s, err := p.session(ctx, session)
	if err != nil {
		return nil, err
	}
	r, err := s.OpenFile(ctx, name)
	return r, p.remote(err)
}

// Only metadata is retained: Payload/Raw may contain input text, screenshots or secrets.
type Event struct {
	ID   int64      `json:"event_id"`
	Turn string     `json:"turn_id,omitempty"`
	Type string     `json:"type,omitempty"`
	Time *time.Time `json:"time,omitempty"`
}
type EventPage struct {
	Task   string  `json:"task_id"`
	After  int64   `json:"after"`
	Next   int64   `json:"next_after"`
	Latest int64   `json:"latest_event_id"`
	Events []Event `json:"events"`
}
type eventReader interface {
	Status(context.Context, pine.TaskOptions) (*pine.AgentTask, error)
	EventsPage(context.Context, pine.EventsPageOptions) (*pine.AgentEventsPage, error)
}

func (p *Pine) eventPage(ctx context.Context, a eventReader, task string, after int64) (*EventPage, error) {
	if after < 0 || (task != "" && p.metadata(task) != task) {
		return nil, Fail("evidence", "invalid task identifier or cursor", 6)
	}
	if task == "" {
		t, err := a.Status(ctx, pine.TaskOptions{})
		if err != nil {
			return nil, p.remote(err)
		}
		if t == nil || t.TaskID == "" || p.metadata(t.TaskID) != t.TaskID {
			return nil, Fail("evidence", "missing safe task identifier", 6)
		}
		task = t.TaskID
	}
	page, err := a.EventsPage(ctx, pine.EventsPageOptions{ExpectedTaskID: task, After: after, Limit: 500, SkipFrames: true})
	if err != nil {
		return nil, p.remote(err)
	}
	if page == nil || page.NextAfter < after || page.NextAfter > page.LatestEventID || (page.NextAfter == after && page.LatestEventID > after) {
		return nil, Fail("evidence", "invalid event cursor; saved cursor unchanged", 6)
	}
	out := &EventPage{Task: task, After: after, Next: page.NextAfter, Latest: page.LatestEventID, Events: []Event{}}
	for _, ev := range page.Events {
		if ev.TaskID != task || ev.EventID <= after || ev.EventID > page.NextAfter {
			return nil, Fail("evidence", "event task or cursor mismatch", 6)
		}
		out.Events = append(out.Events, Event{ID: ev.EventID, Turn: p.metadata(ev.TurnID), Type: p.metadata(ev.Type), Time: ev.Ts})
	}
	return out, nil
}
func (p *Pine) Events(ctx context.Context, session, task string, after int64) (*EventPage, error) {
	s, err := p.session(ctx, session)
	if err != nil {
		return nil, err
	}
	return p.eventPage(ctx, s.Agent(), task, after)
}

// One bounded page per invocation. Empty pages still advance the scan cursor.
func (s *Service) Events(ctx context.Context, task string) (*EventPage, error) {
	backend, ok := s.Backend.(interface {
		Events(context.Context, string, string, int64) (*EventPage, error)
	})
	if !ok {
		return nil, Fail("unsupported", "event metadata collection requires Pine backend", 3)
	}
	after := int64(0)
	if old := s.Store.State.Events[task]; old != nil {
		after = old.Next
	}
	page, err := backend.Events(ctx, s.Store.State.Session, task, after)
	if err != nil {
		return nil, err
	}
	// Discovery with no --task starts a fresh page; explicit task resumes its saved cursor.
	if s.Store.State.Events == nil {
		s.Store.State.Events = map[string]*EventPage{}
	}
	saved := *page
	if old := s.Store.State.Events[page.Task]; old != nil && after == old.Next {
		saved.After = old.After
		saved.Events = append(append([]Event{}, old.Events...), page.Events...)
	}
	if old := s.Store.State.Events[page.Task]; old == nil || saved.Next >= old.Next {
		s.Store.State.Events[page.Task] = &saved
	}
	return page, s.Store.Save()
}
