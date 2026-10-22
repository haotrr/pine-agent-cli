package engine

import (
	"context"
	"encoding/json"
	"errors"
	pine "go.pinesandbox.io/computer"
	"io"
	"os"
	"strings"
	"time"
)

type Pine struct {
	computer *pine.Computer
	secrets  []string
}

// Only explicit live commands call this. Never generate credentials or start resources.
func NewPine(ctx context.Context, endpoint string) (*Pine, error) {
	if !strings.HasPrefix(endpoint, "https://") {
		return nil, Fail("config", "live endpoint must use HTTPS", 2)
	}
	var creds pine.Credentials
	var binding pine.Binding
	if json.Unmarshal([]byte(os.Getenv("PINE_CREDENTIALS_JSON")), &creds) != nil || json.Unmarshal([]byte(os.Getenv("PINE_BINDING_JSON")), &binding) != nil {
		return nil, Fail("credentials", "inject SDK credentials and binding JSON through the environment", 4)
	}
	client, e := pine.NewClient(pine.ClientOptions{Endpoint: endpoint, APIKey: os.Getenv("PINE_API_KEY")})
	if e != nil {
		return nil, Fail("credentials", "invalid SDK client configuration", 4)
	}
	computer, e := client.ReconnectComputer(ctx, creds, binding)
	if e != nil {
		return nil, remote(e)
	}
	secrets := []string{os.Getenv("PINE_API_KEY")}
	for _, name := range []string{"PINE_CREDENTIALS_JSON", "PINE_BINDING_JSON"} {
		var v any
		if json.Unmarshal([]byte(os.Getenv(name)), &v) == nil {
			secrets = append(secrets, secretStrings(v)...)
		}
	}
	return &Pine{computer: computer, secrets: secrets}, nil
}
func remote(e error) error {
	if e == nil {
		return nil
	}
	var u *pine.RunUnconfirmedError
	if errors.As(e, &u) {
		return Fail("unconfirmed", "run outcome unknown; retry identical request ID and input", 7)
	}
	var a *pine.APIError
	if errors.As(e, &a) {
		if a.Status == 401 || a.Status == 403 {
			return Fail("permission", "Pine denied access; do not retry without changed authorization", 4)
		}
		if a.Status == 409 {
			return Fail("conflict", "Pine refused operation due to current state; inspect saved turn", 6)
		}
	}
	return Fail("remote", "Pine request failed or outcome unknown; inspect state before retrying mutations", 7)
}
func (p *Pine) Identity() string { return p.computer.ID() }
func (p *Pine) session(ctx context.Context, s string) (*pine.Session, error) {
	v, e := p.computer.Session(ctx, s)
	return v, remote(e)
}
func (p *Pine) Create(ctx context.Context, s string) error {
	_, e := p.computer.CreateSession(ctx, pine.CreateSessionOptions{Name: s, Browser: true})
	return remote(e)
}
func (p *Pine) convert(t *pine.AgentTurn) *Turn {
	if t == nil {
		return nil
	}
	v := &Turn{ID: t.TurnID, State: t.State, Next: t.NextTurnID}
	if t.Result != nil {
		v.Summary = t.Result.Summary
		for _, secret := range p.secrets {
			if secret != "" {
				v.Summary = strings.ReplaceAll(v.Summary, secret, "[redacted]")
			}
		}
		v.Usage = t.Result.Usage
	}
	return v
}
func constraints(l Limits) *pine.RunConstraints {
	return &pine.RunConstraints{MaxTokens: l.Tokens, MaxWallMs: l.Wall.Milliseconds()}
}
func (p *Pine) Read(ctx context.Context, s, id string, w time.Duration) (*Turn, error) {
	v, e := p.session(ctx, s)
	if e != nil {
		return nil, e
	}
	if id == "" {
		t, e := v.Agent().Status(ctx, pine.TaskOptions{})
		if errors.Is(e, pine.ErrTaskNotFound) {
			return nil, nil
		}
		if e != nil {
			return nil, remote(e)
		}
		return p.convert(t.Turn), nil
	}
	t, e := v.Agent().Wait(ctx, id, w)
	return p.convert(t), remote(e)
}
func (p *Pine) Run(ctx context.Context, s, g, k string, l Limits) (*Turn, error) {
	if e := p.computer.RequireAgentProtocol(ctx); e != nil {
		return nil, remote(e)
	}
	v, e := p.session(ctx, s)
	if e != nil {
		return nil, e
	}
	t, e := v.Agent().Run(ctx, g, pine.RunOptions{IdempotencyKey: k, Constraints: constraints(l)})
	return p.convert(t), remote(e)
}
func (p *Pine) Answer(ctx context.Context, s, id, a string, l Limits) (*Turn, error) {
	v, e := p.session(ctx, s)
	if e != nil {
		return nil, e
	}
	t, e := v.Agent().Answer(ctx, id, a, pine.AnswerOptions{})
	return p.convert(t), remote(e)
}
func (p *Pine) Cancel(ctx context.Context, s, id string) (*Turn, error) {
	v, e := p.session(ctx, s)
	if e != nil {
		return nil, e
	}
	t, e := v.Agent().Cancel(ctx, id)
	return p.convert(t), remote(e)
}
func (p *Pine) Artifacts(ctx context.Context, s string) ([]Artifact, error) {
	v, e := p.session(ctx, s)
	if e != nil {
		return nil, e
	}
	a, e := v.ListArtifacts(ctx, "")
	if e != nil {
		return nil, remote(e)
	}
	var out []Artifact
	for _, x := range a {
		if x.CreatedBy != "upload" {
			out = append(out, Artifact{x.ID, x.Filename, x.SHA256, x.Size, x.CreatedBy})
		}
	}
	return out, nil
}
func (p *Pine) Open(ctx context.Context, s, id string) (io.ReadCloser, error) {
	v, e := p.session(ctx, s)
	if e != nil {
		return nil, e
	}
	r, e := v.OpenArtifact(ctx, id)
	return r, remote(e)
}
func (p *Pine) Stop(ctx context.Context, s string) error {
	sessions, e := p.computer.Sessions(ctx)
	if e != nil {
		return remote(e)
	}
	for _, v := range sessions {
		if v.Name() != s {
			return Fail("other_sessions", "other sessions exist; collect their outputs before stopping", 6)
		}
	}
	_, e = p.computer.Stop(ctx)
	return remote(e)
}

func (p *Pine) Runtime() string { return p.computer.SandboxID() }
func secretStrings(v any) []string {
	var out []string
	switch x := v.(type) {
	case string:
		if len(x) >= 16 {
			out = append(out, x)
		}
	case map[string]any:
		for _, a := range x {
			out = append(out, secretStrings(a)...)
		}
	case []any:
		for _, a := range x {
			out = append(out, secretStrings(a)...)
		}
	}
	return out
}
