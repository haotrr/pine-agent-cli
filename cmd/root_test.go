package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func invoke(t *testing.T, args ...string) (int, map[string]any, string) {
	t.Helper()
	var out, err bytes.Buffer
	code := Execute(context.Background(), args, &out, &err)
	var v map[string]any
	if e := json.Unmarshal(out.Bytes(), &v); e != nil {
		t.Fatalf("invalid JSON %s: %v", out.String(), e)
	}
	return code, v, err.String()
}
func TestSafeDefaults(t *testing.T) {
	t.Setenv("PINE_API_KEY", "DO-NOT-PRINT")
	for _, a := range [][]string{{"computer", "start"}, {"--mode", "live", "task", "status"}, {"--mode", "live", "computer", "start", "--allow-live"}} {
		code, _, _ := invoke(t, a...)
		if code != 3 {
			t.Fatalf("code %d", code)
		}
	}
	code, v, _ := invoke(t, "doctor")
	if code != 0 || v["data"].(map[string]any)["credentials_read"] != false {
		t.Fatal(v)
	}
}
func TestCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	base := []string{"--mode", "mock", "--state-dir", filepath.Join(dir, "state")}
	run := func(a ...string) map[string]any {
		args := append(append([]string{}, base...), a...)
		code, v, _ := invoke(t, args...)
		if code != 0 {
			t.Fatalf("%v: %v", a, v)
		}
		return v
	}
	run("computer", "start")
	run("session", "create", "demo")
	goal := filepath.Join(dir, "goal.txt")
	os.WriteFile(goal, []byte("hello"), 0600)
	a := run("task", "run", "--request-id", "r", "--goal-file", goal)
	b := run("task", "run", "--request-id", "r", "--goal-file", goal)
	if a["data"].(map[string]any)["turn_id"] != b["data"].(map[string]any)["turn_id"] {
		t.Fatal("duplicate")
	}
	run("task", "wait")
	run("artifacts", "pull", "--out", filepath.Join(dir, "out"))
	run("computer", "stop", "--confirm-quiescent", "--out", filepath.Join(dir, "out"))
}

func TestCreateApprovalGates(t *testing.T) {
	for _, flags := range [][]string{nil, {"--mode", "live"}, {"--mode", "live", "--allow-live"}, {"--mode", "live", "--allow-live", "--confirm-billing"}, {"--mode", "live", "--allow-live", "--confirm-new-identity"}} {
		dir := filepath.Join(t.TempDir(), "untouched")
		args := append([]string{"computer", "create", "--state-dir", dir}, flags...)
		code, _, _ := invoke(t, args...)
		if code != 3 {
			t.Fatalf("missing approval: %v code %d", flags, code)
		}
		if _, e := os.Stat(dir); !os.IsNotExist(e) {
			t.Fatal("created state before approval")
		}
	}
}

func TestRunTokenBudget(t *testing.T) {
	dir := t.TempDir()
	base := []string{"--mode", "mock", "--state-dir", filepath.Join(dir, "state")}
	call := func(args ...string) (int, map[string]any) {
		code, v, _ := invoke(t, append(append([]string{}, base...), args...)...)
		return code, v
	}
	if code, v := call("session", "create", "demo"); code != 0 {
		t.Fatal(v)
	}
	goal := filepath.Join(dir, "goal.txt")
	if err := os.WriteFile(goal, []byte("read only"), 0600); err != nil {
		t.Fatal(err)
	}
	run := []string{"task", "run", "--request-id", "same", "--goal-file", goal}
	code, first := call(run...)
	if code != 0 {
		t.Fatal(first)
	}
	code, zero := call(append(append([]string{}, run...), "--max-tokens", "0")...)
	if code != 0 || first["data"].(map[string]any)["turn_id"] != zero["data"].(map[string]any)["turn_id"] {
		t.Fatal("default and explicit zero must replay same request", zero)
	}
	// A positive override changes the persisted request digest; it must not silently replay.
	code, changed := call(append(append([]string{}, run...), "--max-tokens", "50000")...)
	if code != 6 || changed["error"].(map[string]any)["code"] != "request_mismatch" {
		t.Fatal(changed)
	}
	for _, flags := range [][]string{{"--max-tokens", "-1"}, {"--max-wall", "0s"}, {"--max-wall", "61m"}} {
		code, v := call(append(append([]string{}, run...), flags...)...)
		if code != 2 || v["error"].(map[string]any)["code"] != "argument" {
			t.Fatal(flags, v)
		}
	}
	// A positive budget is accepted on a fresh request.
	if code, v := call("task", "wait"); code != 0 {
		t.Fatal(v)
	}
	if code, v := call("task", "run", "--request-id", "positive", "--goal-file", goal, "--max-tokens", "50000"); code != 0 {
		t.Fatal(v)
	}
}
