package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pine "go.pinesandbox.io/computer"
)

type stopComputer struct {
	pineComputer
	confirmed bool
	err       error
	calls     int
}

func (c *stopComputer) Sessions(context.Context) ([]*pine.Session, error) { return nil, nil }
func (c *stopComputer) Stop(context.Context) (bool, error)                { c.calls++; return c.confirmed, c.err }
func TestSDKStopConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		confirmed bool
		err       error
		code      string
	}{
		{"false", false, nil, "stop_unconfirmed"}, {"true", true, nil, ""}, {"error", false, errors.New("secret"), "remote"}, {"true_with_error", true, errors.New("secret"), "remote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := fixture(t)
			s.Store.State.Session = ""
			c := &stopComputer{confirmed: tc.confirmed, err: tc.err}
			s.Backend = &Pine{computer: c}
			err := s.Stop(context.Background(), t.TempDir())
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if e, ok := err.(*Error); !ok || e.Code != tc.code {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.calls != 1 || s.Store.State.Stopped != (tc.code == "") {
				t.Fatal("unconfirmed stop recorded or retried")
			}
			b, _ := os.ReadFile(filepath.Join(s.Store.Dir, "state.json"))
			var disk State
			if json.Unmarshal(b, &disk) != nil || disk.Stopped != (tc.code == "") {
				t.Fatal("wrong persisted stop state")
			}
		})
	}
}

type fileCatalog struct {
	turns  []pine.AgentTurn
	err    error
	filter string
}

func (f *fileCatalog) ListArtifacts(_ context.Context, filter string) ([]*pine.Artifact, error) {
	f.filter = filter
	return []*pine.Artifact{{ID: "download", Filename: "download.txt", CreatedBy: "download", Size: int64(len(mockContent))}, {ID: "upload", Filename: "input", CreatedBy: "upload"}}, nil
}
func (f *fileCatalog) Turns(context.Context, pine.TaskOptions) ([]pine.AgentTurn, error) {
	return f.turns, f.err
}
func resultTurns(name, root, hash string) []pine.AgentTurn {
	return []pine.AgentTurn{{TurnID: "turn", Result: &pine.AgentResult{Artifacts: []pine.FileRef{{Root: root, RelativePath: name, Size: int64(len(mockContent)), SHA256: hash}}}}}
}

type workdirBackend struct {
	*Mock
	items                    []Artifact
	fileCalls, artifactCalls int
}

func (b *workdirBackend) Artifacts(context.Context, string) ([]Artifact, error) { return b.items, nil }
func (b *workdirBackend) OpenFile(_ context.Context, _, name string) (io.ReadCloser, error) {
	if name != "reports/result.txt" {
		return nil, errors.New("incorrect workdir path")
	}
	b.fileCalls++
	return io.NopCloser(strings.NewReader(mockContent)), nil
}
func (b *workdirBackend) Open(ctx context.Context, s, id string) (io.ReadCloser, error) {
	b.artifactCalls++
	return b.Mock.Open(ctx, s, id)
}
func TestSDKResultFilesAndDownloads(t *testing.T) {
	hash := sha256.Sum256([]byte(mockContent))
	c := &fileCatalog{turns: resultTurns("reports/result.txt", "workdir", hex.EncodeToString(hash[:]))}
	items, err := (&Pine{}).artifacts(context.Background(), c, c)
	if err != nil || len(items) != 2 || c.filter != "" {
		t.Fatal(items, err)
	}
	s, m := fixture(t)
	b := &workdirBackend{Mock: m, items: items}
	s.Backend = b
	if n, err := s.Pull(context.Background(), t.TempDir()); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if b.fileCalls != 1 || b.artifactCalls != 1 {
		t.Fatal("wrong file API routing")
	}
	saved := s.Store.State.Saved[items[1].ID]
	if !strings.HasSuffix(saved, "/reports/result.txt") {
		t.Fatal("relative path lost")
	}
	data, err := os.ReadFile(saved)
	if err != nil || string(data) != mockContent {
		t.Fatal(err)
	}
	c.err = pine.ErrTaskNotFound
	if items, err = (&Pine{}).artifacts(context.Background(), c, c); err != nil || len(items) != 1 {
		t.Fatal("idle session downloads lost", err)
	}
	c.err = errors.New("unavailable")
	if _, err = (&Pine{}).artifacts(context.Background(), c, c); err == nil {
		t.Fatal("turn read failure hidden")
	}
}
func TestSDKWorkdirGuards(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "a/../b", "a//b", ".", "a\\b", "a\x00b", "C:/file"} {
		if _, err := workdirArtifacts(resultTurns(name, "workdir", "")); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if _, err := workdirArtifacts(resultTurns("safe", "artifact", "")); err == nil {
		t.Fatal("accepted unsupported root")
	}
	s, m := fixture(t)
	items, _ := workdirArtifacts(resultTurns("reports/result.txt", "workdir", "wrong-hash"))
	b := &workdirBackend{Mock: m, items: items}
	s.Backend = b
	if err := s.Stop(context.Background(), t.TempDir()); err == nil || m.Data.Stopped {
		t.Fatal("stopped despite changed workdir bytes")
	}
}
func TestSDKFailureEvidence(t *testing.T) {
	p := &Pine{secrets: []string{"secret-value"}}
	s, _ := fixture(t)
	turn := p.convert(&pine.AgentTurn{TurnID: "t", State: "failed", Result: &pine.AgentResult{TerminalReason: "environment_unavailable", Summary: "secret-value"}})
	if _, err := s.Remember(turn); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(s.Store.Dir, "state.json"))
	if !strings.Contains(string(data), "environment_unavailable") || strings.Contains(string(data), "secret-value") {
		t.Fatal("missing or unsafe result evidence")
	}
	api := &pine.APIError{Status: 503, RequestID: "req_example", ProblemType: "https://example.invalid/problems/unavailable", Detail: "secret-value", Title: "secret-value"}
	err := p.remote(&pine.RunUnconfirmedError{Err: api}).(*Error)
	data, _ = json.Marshal(err)
	if err.Code != "unconfirmed" || err.Status != 503 || err.RequestID != "req_example" || err.ProblemType != api.ProblemType || strings.Contains(string(data), "secret-value") {
		t.Fatal("lost structured error or leaked detail")
	}
	api.RequestID = "secret-value"
	api.ProblemType = "https://example.invalid/?token=secret-value"
	err = p.remote(api).(*Error)
	if err.RequestID != "" || err.ProblemType != "" {
		t.Fatal("unsafe metadata retained")
	}
}

type eventSDK struct {
	task    string
	pages   []*pine.AgentEventsPage
	options []pine.EventsPageOptions
	err     error
}

func (e *eventSDK) Status(context.Context, pine.TaskOptions) (*pine.AgentTask, error) {
	return &pine.AgentTask{TaskID: e.task}, nil
}
func (e *eventSDK) EventsPage(_ context.Context, o pine.EventsPageOptions) (*pine.AgentEventsPage, error) {
	e.options = append(e.options, o)
	if e.err != nil {
		return nil, e.err
	}
	p := e.pages[0]
	e.pages = e.pages[1:]
	return p, nil
}

type eventBackend struct {
	*Mock
	p   *Pine
	sdk *eventSDK
}

func (b *eventBackend) Events(ctx context.Context, _, task string, after int64) (*EventPage, error) {
	return b.p.eventPage(ctx, b.sdk, task, after)
}
func TestSDKEventCursorPersistence(t *testing.T) {
	s, m := fixture(t)
	sdk := &eventSDK{task: "task_one", pages: []*pine.AgentEventsPage{
		{NextAfter: 5, LatestEventID: 8},
		{NextAfter: 8, LatestEventID: 8, Events: []pine.AgentEvent{{TaskID: "task_one", EventID: 8, TurnID: "turn", Type: "result", Payload: json.RawMessage(`{"secret":"secret-value"}`), Raw: json.RawMessage(`{"secret":"secret-value"}`)}}},
	}}
	b := &eventBackend{Mock: m, p: &Pine{}, sdk: sdk}
	s.Backend = b
	page, err := s.Events(context.Background(), "")
	if err != nil || page.Next != 5 || len(page.Events) != 0 {
		t.Fatal(page, err)
	}
	// Reload the JSON to prove the next invocation does not rely on in-memory cursors.
	data, _ := os.ReadFile(filepath.Join(s.Store.Dir, "state.json"))
	s.Store.State = State{}
	if err = json.Unmarshal(data, &s.Store.State); err != nil {
		t.Fatal(err)
	}
	page, err = s.Events(context.Background(), "task_one")
	if err != nil || page.Next != 8 {
		t.Fatal(page, err)
	}
	o := sdk.options[1]
	if o.After != 5 || o.ExpectedTaskID != "task_one" || !o.SkipFrames || o.Limit != 500 {
		t.Fatal(o)
	}
	data, _ = os.ReadFile(filepath.Join(s.Store.Dir, "state.json"))
	if strings.Contains(string(data), "secret-value") || !strings.Contains(string(data), "event_id") {
		t.Fatal("unsafe or missing events")
	}
	sdk.err = &pine.APIError{Status: 409, RequestID: "req_test"}
	if _, err = s.Events(context.Background(), "task_one"); err == nil || s.Store.State.Events["task_one"].Next != 8 {
		t.Fatal("cursor advanced after error")
	}
}
func TestSDKEventInvalidPages(t *testing.T) {
	for _, page := range []*pine.AgentEventsPage{
		{NextAfter: 4, LatestEventID: 8}, {NextAfter: 5, LatestEventID: 8}, {NextAfter: 10, LatestEventID: 8},
		{NextAfter: 8, LatestEventID: 8, Events: []pine.AgentEvent{{TaskID: "another_task", EventID: 8}}},
	} {
		sdk := &eventSDK{pages: []*pine.AgentEventsPage{page}}
		if _, err := (&Pine{}).eventPage(context.Background(), sdk, "task", 5); err == nil {
			t.Fatal("accepted invalid cursor or task")
		}
	}
}
