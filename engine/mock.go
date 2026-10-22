package engine

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mock persists a separate simulated server, allowing process-restart exercises.
type Mock struct {
	path string
	Data MockData
}
type MockData struct {
	Session string
	Latest  string
	Turns   map[string]*Turn
	Keys    map[string]string
	Answers map[string]string
	Stopped bool
}

func NewMock(dir string) (*Mock, error) {
	m := &Mock{path: filepath.Join(dir, "mock-server.json"), Data: MockData{Turns: map[string]*Turn{}, Keys: map[string]string{}, Answers: map[string]string{}}}
	b, e := os.ReadFile(m.path)
	if e == nil {
		e = json.Unmarshal(b, &m.Data)
	}
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	return m, nil
}
func (m *Mock) save() error      { return AtomicJSON(m.path, m.Data) }
func (m *Mock) Identity() string { return "mock-computer" }
func (m *Mock) Create(_ context.Context, s string) error {
	if m.Data.Session != "" && m.Data.Session != s {
		return Fail("conflict", "mock session already exists", 6)
	}
	m.Data.Session = s
	m.Data.Stopped = false
	return m.save()
}
func (m *Mock) Read(_ context.Context, s, id string, w time.Duration) (*Turn, error) {
	if id == "" {
		id = m.Data.Latest
	}
	t := m.Data.Turns[id]
	if t == nil {
		if id == "" {
			return nil, nil
		}
		return nil, Fail("not_found", "turn not found", 6)
	}
	if w > 0 && (t.State == "running" || t.State == "canceling") {
		if t.State == "canceling" {
			t.State = "canceled"
		} else {
			t.State = "completed"
			t.Summary = "MOCK result; no external action executed"
		}
		if e := m.save(); e != nil {
			return nil, e
		}
	}
	return t, nil
}
func (m *Mock) Run(_ context.Context, s, goal, key string, l Limits) (*Turn, error) {
	if id := m.Data.Keys[key]; id != "" {
		return m.Data.Turns[id], nil
	}
	if t := m.Data.Turns[m.Data.Latest]; t != nil && (t.State == "running" || t.State == "canceling" || t.State == "needs_input") {
		return nil, Fail("conflict", "finish or answer current turn first", 6)
	}
	t := &Turn{ID: "mock-" + ID(), State: "running"}
	if strings.Contains(goal, "[ask]") {
		t.State = "needs_input"
		t.Summary = "MOCK: may I continue?"
	}
	m.Data.Turns[t.ID] = t
	m.Data.Latest = t.ID
	m.Data.Keys[key] = t.ID
	return t, m.save()
}
func (m *Mock) Answer(_ context.Context, s, id, answer string, l Limits) (*Turn, error) {
	t := m.Data.Turns[id]
	if t == nil {
		return nil, Fail("not_found", "turn not found", 6)
	}
	if t.State == "answered" && m.Data.Answers[id] == answer {
		return m.Data.Turns[t.Next], nil
	}
	if t.State != "needs_input" || id != m.Data.Latest {
		return nil, Fail("stale_turn", "cannot answer this turn", 6)
	}
	n := &Turn{ID: "mock-" + ID(), State: "running"}
	t.State = "answered"
	t.Next = n.ID
	m.Data.Answers[id] = answer
	m.Data.Turns[n.ID] = n
	m.Data.Latest = n.ID
	return n, m.save()
}
func (m *Mock) Cancel(_ context.Context, s, id string) (*Turn, error) {
	t := m.Data.Turns[id]
	if t == nil {
		return nil, Fail("not_found", "turn not found", 6)
	}
	switch t.State {
	case "running":
		t.State = "canceling"
	case "needs_input":
		t.State = "canceled"
	case "answered":
		return nil, Fail("stale_turn", "cancel the continuation explicitly", 6)
	}
	return t, m.save()
}
func (m *Mock) Artifacts(_ context.Context, s string) ([]Artifact, error) {
	var out []Artifact
	for _, t := range m.Data.Turns {
		if t.State == "completed" {
			out = append(out, Artifact{ID: t.ID, Name: "result.txt", Source: "agent", Size: int64(len(mockContent))})
		}
	}
	return out, nil
}

const mockContent = "MOCK artifact. No real Pine task ran.\n"

func (m *Mock) Open(_ context.Context, s, id string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(mockContent)), nil
}
func (m *Mock) Stop(_ context.Context, s string) error { m.Data.Stopped = true; return m.save() }

func (m *Mock) Runtime() string { return "mock-runtime" }
