package engine

import (
	"errors"
	pine "go.pinesandbox.io/computer"
	"testing"
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
