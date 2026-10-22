package engine

import (
	"context"
	"io"
	"time"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Exit    int    `json:"-"`
}

func (e *Error) Error() string                  { return e.Message }
func Fail(code, message string, exit int) error { return &Error{code, message, exit} }

type Turn struct {
	ID      string `json:"turn_id"`
	State   string `json:"state"`
	Next    string `json:"next_turn_id,omitempty"`
	Summary string `json:"summary,omitempty"`
	Usage   any    `json:"usage,omitempty"`
}
type Artifact struct {
	ID, Name, SHA256 string
	Size             int64
	Source           string
}
type Limits struct {
	Tokens int
	Wall   time.Duration
}

// Backend is our consumer contract, not a declaration of Pine's wire API.
type Backend interface {
	Identity() string
	Runtime() string
	Create(context.Context, string) error
	Read(context.Context, string, string, time.Duration) (*Turn, error)
	Run(context.Context, string, string, string, Limits) (*Turn, error)
	Answer(context.Context, string, string, string, Limits) (*Turn, error)
	Cancel(context.Context, string, string) (*Turn, error)
	Artifacts(context.Context, string) ([]Artifact, error)
	Open(context.Context, string, string) (io.ReadCloser, error)
	Stop(context.Context, string) error
}
