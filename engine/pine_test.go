package engine

import (
	"encoding/json"
	"errors"
	pine "go.pinesandbox.io/computer"
	"testing"
	"time"
)

func TestSDKSummaryRedaction(t *testing.T) {
	p := &Pine{secrets: []string{"secret-value"}}
	v := p.convert(&pine.AgentTurn{TurnID: "t", State: "completed", Result: &pine.AgentResult{Summary: "accidental secret-value echo"}})
	if v.Summary != "accidental [redacted] echo" {
		t.Fatal(v.Summary)
	}
}
func TestRemoteErrorsNeverExposeDetail(t *testing.T) {
	for _, err := range []error{errors.New("secret-value"), &pine.APIError{Status: 403, Detail: "secret-value"}} {
		e := remote(err).(*Error)
		if e.Message == "secret-value" {
			t.Fatal("leak")
		}
		if e.Exit != 4 && e.Exit != 7 {
			t.Fatal(e)
		}
	}
}

func TestSDKTokenConstraintSerialization(t *testing.T) {
	for _, tokens := range []int{0, 50000} {
		b, err := json.Marshal(constraints(Limits{Tokens: tokens, Wall: 60 * time.Second}))
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]int64
		if err := json.Unmarshal(b, &wire); err != nil {
			t.Fatal(err)
		}
		value, present := wire["max_tokens"]
		if present != (tokens > 0) || value != int64(tokens) {
			t.Fatal("incorrect token constraint", string(b))
		}
		if wire["max_wall_ms"] != 60000 {
			t.Fatal("wall constraint lost", string(b))
		}
	}
}
