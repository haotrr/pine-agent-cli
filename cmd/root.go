package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	pine "go.pinesandbox.io/computer"
	"io"
	"os"
	"pine-agent-cli/engine"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Mode     string `json:"mode"`
	Endpoint string `json:"endpoint"`
}

func Execute(ctx context.Context, args []string, out, errout io.Writer) int {
	c := NewRoot()
	c.SetArgs(args)
	c.SetOut(out)
	c.SetErr(errout)
	e := c.ExecuteContext(ctx)
	if e == nil {
		return 0
	}
	var typed *engine.Error
	if !errors.As(e, &typed) {
		typed = &engine.Error{Code: "local", Message: "command failed; check arguments, local files and permissions", Exit: 2}
	}
	_ = json.NewEncoder(out).Encode(map[string]any{"ok": false, "error": typed})
	return typed.Exit
}
func NewRoot() *cobra.Command {
	var cfgPath, mode, dir, endpoint string
	var live bool
	root := &cobra.Command{Use: "pine-agent", Short: "Delegate to Pine Agent; safe offline default", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&cfgPath, "config", "", "non-secret JSON config")
	root.PersistentFlags().StringVar(&mode, "mode", "disabled", "disabled, mock, live")
	root.PersistentFlags().StringVar(&endpoint, "endpoint", "", "Pine project HTTPS endpoint")
	root.PersistentFlags().StringVar(&dir, "state-dir", ".pine-agent", "private local operation journal")
	root.PersistentFlags().BoolVar(&live, "allow-live", false, "confirm this invocation is authorized to access Pine")
	root.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		if cfgPath != "" {
			f, e := os.Open(cfgPath)
			if e != nil {
				return e
			}
			defer f.Close()
			var v Config
			d := json.NewDecoder(f)
			d.DisallowUnknownFields()
			if e = d.Decode(&v); e != nil {
				return engine.Fail("config", "invalid non-secret config", 2)
			}
			if !c.Flags().Changed("mode") && !root.PersistentFlags().Changed("mode") {
				mode = v.Mode
			}
			if !root.PersistentFlags().Changed("endpoint") {
				endpoint = v.Endpoint
			}
		}
		if mode != "disabled" && mode != "mock" && mode != "live" {
			return engine.Fail("config", "invalid backend mode", 2)
		}
		return nil
	}
	emit := func(c *cobra.Command, data any) error {
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"ok": true, "mode": mode, "data": data})
	}
	open := func(ctx context.Context) (*engine.Service, func(), error) {
		if mode == "disabled" {
			return nil, nil, engine.Fail("disabled", "select --mode mock or explicitly authorized live mode", 3)
		}
		if mode == "live" && !live {
			return nil, nil, engine.Fail("approval_required", "live mode requires --allow-live after user authorization", 3)
		}
		st, e := engine.Load(dir, mode)
		if e != nil {
			return nil, nil, e
		}
		var b engine.Backend
		if mode == "mock" {
			b, e = engine.NewMock(dir)
		} else {
			b, e = engine.NewPineStored(ctx, dir, endpoint)
		}
		if e != nil {
			st.Close()
			return nil, nil, e
		}
		s := &engine.Service{Store: st, Backend: b}
		if e = s.Bind(); e != nil {
			st.Close()
			return nil, nil, e
		}
		return s, st.Close, nil
	}
	with := func(fn func(*cobra.Command, *engine.Service, []string) error) func(*cobra.Command, []string) error {
		return func(c *cobra.Command, a []string) error {
			ctx, cancel := context.WithTimeout(c.Context(), 90*time.Second)
			defer cancel()
			c.SetContext(ctx)
			s, close, e := open(ctx)
			if e != nil {
				return e
			}
			defer close()
			return fn(c, s, a)
		}
	}
	root.AddCommand(&cobra.Command{Use: "doctor", Args: cobra.NoArgs, RunE: func(c *cobra.Command, a []string) error {
		return emit(c, map[string]any{"sdk": "go.pinesandbox.io/computer v0.3.19", "network_checked": false, "credentials_read": false, "live_start_supported": true, "live_create_supported": true, "ready": mode == "mock", "note": "live create/start requires separately authorized identity storage and billing; doctor is offline"})
	}})
	computer := &cobra.Command{Use: "computer"}
	for _, operation := range []string{"start", "create"} {
		operation := operation
		var billing bool
		var identity bool
		var ttl time.Duration
		start := &cobra.Command{Use: operation, Args: cobra.NoArgs, RunE: func(c *cobra.Command, a []string) error {
			if mode == "mock" {
				return with(func(c *cobra.Command, s *engine.Service, a []string) error {
					return emit(c, map[string]string{"computer_id": s.Backend.Identity()})
				})(c, a)
			}
			if mode != "live" || !live || !billing || (operation == "create" && !identity) {
				return engine.Fail("approval_required", "live create/start requires --allow-live and --confirm-billing; create also requires --confirm-new-identity after user authorization", 3)
			}
			if ttl <= 0 || ttl > time.Hour {
				return engine.Fail("argument", "idle timeout must be positive and <= 1h", 2)
			}
			st, e := engine.Load(dir, mode)
			if e != nil {
				return e
			}
			defer st.Close()
			if st.State.Stopped {
				return engine.Fail("stopped", "use a fresh state directory with existing credentials after a stop", 6)
			}
			ctx, cancel := context.WithTimeout(c.Context(), 90*time.Second)
			defer cancel()
			if st.State.Computer != "" {
				v, er := engine.VaultFromEnv(dir)
				if er != nil {
					return er
				}
				r, er := v.Load()
				if er != nil {
					return er
				}
				if r.Phase != "ready" || r.Credentials.ID != st.State.Computer {
					return engine.Fail("state", "existing journal cannot initiate another start; inspect the existing vault", 6)
				}
			}
			provision := engine.LiveStart
			if operation == "create" {
				provision = engine.LiveCreate
			}
			backend, e := provision(ctx, dir, endpoint, ttl)
			if e != nil {
				return e
			}
			service := &engine.Service{Store: st, Backend: backend}
			if e = service.Bind(); e != nil {
				return e
			}
			return emit(c, map[string]string{"computer_id": backend.Identity(), "runtime_id": backend.Runtime()})
		}}
		start.Flags().BoolVar(&billing, "confirm-billing", false, "explicitly authorize billing for this Computer start")
		start.Flags().DurationVar(&ttl, "idle-timeout", 5*time.Minute, "SDK lifetime/idle policy; not a hard total cost cap")
		if operation == "create" {
			start.Flags().BoolVar(&identity, "confirm-new-identity", false, "authorize generating and persisting a new Computer identity")
		}
		computer.AddCommand(start)
	}
	var verified bool
	reconcile := &cobra.Command{Use: "reconcile", Args: cobra.NoArgs, RunE: func(c *cobra.Command, a []string) error {
		if mode != "live" || !live || !verified {
			return engine.Fail("approval_required", "reconcile requires live authorization and --confirm-binding-verified", 3)
		}
		st, e := engine.Load(dir, mode)
		if e != nil {
			return e
		}
		defer st.Close()
		v, e := engine.VaultFromEnv(dir)
		if e != nil {
			return e
		}
		var b pine.Binding
		if json.Unmarshal([]byte(os.Getenv("PINE_BINDING_JSON")), &b) != nil {
			return engine.Fail("credentials", "inject independently verified SDK binding JSON", 4)
		}
		if e = engine.Reconcile(v, endpoint, b); e != nil {
			return e
		}
		return emit(c, map[string]bool{"reconciled": true})
	}}
	reconcile.Flags().BoolVar(&verified, "confirm-binding-verified", false, "binding was independently verified against Pine")
	computer.AddCommand(reconcile)

	var stopOut string
	var stopAck bool
	stop := &cobra.Command{Use: "stop", Args: cobra.NoArgs, RunE: with(func(c *cobra.Command, s *engine.Service, a []string) error {
		if !stopAck {
			return engine.Fail("approval_required", "confirm no pending downloads or other actors with --confirm-quiescent", 3)
		}
		if e := s.Stop(c.Context(), stopOut); e != nil {
			return e
		}
		return emit(c, map[string]bool{"stopped": true})
	})}
	stop.Flags().StringVar(&stopOut, "out", "artifacts", "download before stop")
	stop.Flags().BoolVar(&stopAck, "confirm-quiescent", false, "user confirmed no unfinished downloads or concurrent actors")
	computer.AddCommand(stop)
	root.AddCommand(computer)
	session := &cobra.Command{Use: "session"}
	session.AddCommand(&cobra.Command{Use: "create NAME", Args: cobra.ExactArgs(1), RunE: with(func(c *cobra.Command, s *engine.Service, a []string) error {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(a[0]) {
			return engine.Fail("argument", "invalid session name", 2)
		}
		if e := s.Create(c.Context(), a[0]); e != nil {
			return e
		}
		return emit(c, map[string]string{"session": a[0]})
	})})
	session.AddCommand(&cobra.Command{Use: "resume NAME", Short: "Record an existing session after verifying it with status", Args: cobra.ExactArgs(1), RunE: with(func(c *cobra.Command, s *engine.Service, a []string) error {
		if s.Store.State.Session != "" && s.Store.State.Session != a[0] {
			return engine.Fail("session", "state already names another session", 6)
		}
		_, e := s.Backend.Artifacts(c.Context(), a[0])
		if e != nil {
			return e
		}
		s.Store.State.Session = a[0]
		if e = s.Store.Save(); e != nil {
			return e
		}
		return emit(c, map[string]string{"session": a[0]})
	})})
	root.AddCommand(session)
	task := &cobra.Command{Use: "task"}
	var request, file string
	var tokens int
	var wall time.Duration
	run := &cobra.Command{Use: "run", Args: cobra.NoArgs, RunE: with(func(c *cobra.Command, s *engine.Service, a []string) error {
		if s.Store.State.Session == "" {
			return engine.Fail("session", "create or resume a session", 6)
		}
		if request == "" || file == "" || tokens <= 0 || wall <= 0 || wall > time.Hour {
			return engine.Fail("argument", "request-id, goal-file and positive limits (wall <= 1h) required", 2)
		}
		b, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		if len(b) == 0 || len(b) > 1024*1024 {
			return engine.Fail("argument", "goal must be 1 byte to 1 MiB", 2)
		}
		t, e := s.Run(c.Context(), request, string(b), engine.Limits{Tokens: tokens, Wall: wall})
		if e != nil {
			return e
		}
		return emit(c, t)
	})}
	run.Flags().StringVar(&request, "request-id", "", "durable application message ID; reuse on lost response")
	run.Flags().StringVar(&file, "goal-file", "", "UTF-8 task text; never put credentials in goals")
	run.Flags().IntVar(&tokens, "max-tokens", 4000, "per turn token budget")
	run.Flags().DurationVar(&wall, "max-wall", 2*time.Minute, "per turn wall limit")
	task.AddCommand(run)
	for _, op := range []string{"status", "wait", "cancel", "answer"} {
		op := op
		var turn, answerFile string
		var wait time.Duration
		var acceptLimits bool
		c := &cobra.Command{Use: op, Args: cobra.NoArgs, RunE: with(func(c *cobra.Command, s *engine.Service, a []string) error {
			if s.Store.State.Session == "" {
				return engine.Fail("session", "no session recorded", 6)
			}
			id := turn
			if id == "" && op == "wait" {
				id = s.Store.State.Turn
			}
			if op != "status" && id == "" {
				return engine.Fail("argument", "explicit --turn required for answer/cancel; wait needs saved turn", 2)
			}
			var t *engine.Turn
			var e error
			switch op {
			case "status":
				t, e = s.Read(c.Context(), id, 0)
			case "wait":
				if wait < 0 || wait > 60*time.Second {
					return engine.Fail("argument", "wait timeout must be 0..60s", 2)
				}
				c.PrintErrln("Waiting for saved turn; timeout does not cancel remote work.")
				t, e = s.Read(c.Context(), id, wait)
			case "cancel":
				t, e = s.Backend.Cancel(c.Context(), s.Store.State.Session, id)
				if e == nil {
					t, e = s.Remember(t)
				}
			case "answer":
				if mode == "live" && !acceptLimits {
					return engine.Fail("approval_required", "SDK answer has no per-turn budget options; --accept-runtime-limits requires explicit authorization", 3)
				}
				b, er := os.ReadFile(answerFile)
				if er != nil {
					return er
				}
				if strings.TrimSpace(string(b)) == "" || len(b) > 1024*1024 {
					return engine.Fail("argument", "invalid answer text", 2)
				}
				t, e = s.Backend.Answer(c.Context(), s.Store.State.Session, id, string(b), engine.Limits{})
				if e == nil {
					t, e = s.Remember(t)
				}
			}
			if e != nil {
				return e
			}
			if e = emit(c, t); e != nil {
				return e
			}
			return nil
		})}
		c.Flags().StringVar(&turn, "turn", "", "exact turn ID; status without this discovers latest")
		if op == "wait" {
			c.Flags().DurationVar(&wait, "timeout", 25*time.Second, "bounded wait, no cancellation")
		}
		if op == "answer" {
			c.Flags().StringVar(&answerFile, "answer-file", "", "answer text file")
			c.Flags().BoolVar(&acceptLimits, "accept-runtime-limits", false, "accept Pine runtime limits for answer continuation")
		}
		task.AddCommand(c)
	}
	var eventTask string
	events := &cobra.Command{Use: "events", Args: cobra.NoArgs, RunE: with(func(c *cobra.Command, s *engine.Service, a []string) error {
		if s.Store.State.Session == "" {
			return engine.Fail("session", "no session recorded", 6)
		}
		page, e := s.Events(c.Context(), eventTask)
		if e != nil {
			return e
		}
		return emit(c, page)
	})}
	events.Flags().StringVar(&eventTask, "task", "", "task ID to resume saved event cursor; omit to discover current task")
	task.AddCommand(events)
	root.AddCommand(task)
	artifacts := &cobra.Command{Use: "artifacts"}
	var output string
	pull := &cobra.Command{Use: "pull", Args: cobra.NoArgs, RunE: with(func(c *cobra.Command, s *engine.Service, a []string) error {
		if s.Store.State.Session == "" {
			return engine.Fail("session", "no session recorded", 6)
		}
		n, e := s.Pull(c.Context(), output)
		if e != nil {
			return e
		}
		return emit(c, map[string]int{"saved": n})
	})}
	pull.Flags().StringVar(&output, "out", "artifacts", "local output directory")
	artifacts.AddCommand(pull)
	root.AddCommand(artifacts)
	return root
}
