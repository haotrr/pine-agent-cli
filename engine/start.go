package engine

import (
	"context"
	"encoding/json"
	"errors"
	pine "go.pinesandbox.io/computer"
	"os"
	"strings"
	"time"
)

type StartFunc func(context.Context, pine.Credentials, time.Duration) (pine.Credentials, pine.Binding, error)

// StartStored never retries a start whose outcome has not been durably recorded.
func StartStored(ctx context.Context, v Vault, endpoint string, initial pine.Credentials, ttl time.Duration, start StartFunc) (*VaultRecord, error) {
	return provisionStored(ctx, v, endpoint, "start", func() (pine.Credentials, error) { return initial, nil }, ttl, start)
}

// CreateStored generates credentials only when no vault exists, before any remote call.
func CreateStored(ctx context.Context, v Vault, endpoint string, generate func() (pine.Credentials, error), ttl time.Duration, create StartFunc) (*VaultRecord, error) {
	return provisionStored(ctx, v, endpoint, "create", generate, ttl, create)
}

func provisionStored(ctx context.Context, v Vault, endpoint, operation string, generate func() (pine.Credentials, error), ttl time.Duration, start StartFunc) (*VaultRecord, error) {
	r, e := v.Load()
	if errors.Is(e, ErrVaultMissing) {
		initial, err := generate()
		if err != nil {
			return nil, Fail("credentials", "cannot prepare persistent identity", 4)
		}
		r = &VaultRecord{Operation: operation, Endpoint: endpoint, Credentials: initial, Phase: "prepared"}
		if e = v.Save(r); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	if r.Endpoint != endpoint {
		return nil, Fail("vault_endpoint", "vault belongs to another endpoint", 6)
	}
	storedOperation := r.Operation
	if storedOperation == "" {
		storedOperation = "start"
	}
	if storedOperation != operation {
		return nil, Fail("operation", "vault belongs to another lifecycle operation; use its original command or reconcile", 6)
	}
	if r.Phase == "ready" {
		return r, nil
	}
	if r.Phase != "prepared" {
		return nil, Fail("start_unconfirmed", "create/start may have executed; reconcile an independently verified binding, do not retry create/start", 7)
	}
	r.Phase = "pending"
	if e = v.Save(r); e != nil {
		return nil, e
	}
	creds, b, e := start(ctx, r.Credentials, ttl)
	if e != nil {
		return nil, Fail("start_unconfirmed", "create/start failed or outcome unknown; encrypted credentials retained; independently inspect Pine before reconciling", 7)
	}
	if creds.ID != r.Credentials.ID || b.ComputerID != creds.ID {
		return nil, Fail("identity", "create/start returned a different identity; outcome unresolved", 7)
	}
	r.Credentials = creds
	r.Binding = &b
	r.Phase = "ready"
	if e = v.Save(r); e != nil {
		return nil, Fail("start_unconfirmed", "Computer may be running but binding persistence failed; independently inspect Pine", 7)
	}
	return r, nil
}
func LiveStart(ctx context.Context, dir, endpoint string, ttl time.Duration) (*Pine, error) {
	if !strings.HasPrefix(endpoint, "https://") {
		return nil, Fail("config", "HTTPS endpoint required", 2)
	}
	v, e := VaultFromEnv(dir)
	if e != nil {
		return nil, e
	}
	var initial pine.Credentials
	if _, e = v.Load(); errors.Is(e, ErrVaultMissing) {
		if json.Unmarshal([]byte(os.Getenv("PINE_CREDENTIALS_JSON")), &initial) != nil {
			return nil, Fail("credentials", "inject existing persistent SDK credentials; use computer create for a new identity", 4)
		}
	} else if e != nil {
		return nil, e
	}
	client, e := pine.NewClient(pine.ClientOptions{Endpoint: endpoint, APIKey: os.Getenv("PINE_API_KEY")})
	if e != nil {
		return nil, Fail("credentials", "invalid SDK client configuration", 4)
	}
	r, e := StartStored(ctx, v, endpoint, initial, ttl, func(ctx context.Context, c pine.Credentials, ttl time.Duration) (pine.Credentials, pine.Binding, error) {
		comp, e := client.StartComputer(ctx, c, pine.StartOptions{Timeout: ttl, ReadyTimeout: 60 * time.Second, MaxBindAttempts: 1})
		if e != nil {
			return c, pine.Binding{}, e
		}
		b, e := comp.Binding()
		return comp.Credentials(), b, e
	})
	if e != nil {
		return nil, e
	}
	return reconnectRecord(ctx, client, r)
}
func reconnectRecord(ctx context.Context, client *pine.Client, r *VaultRecord) (*Pine, error) {
	if r.Phase != "ready" || r.Binding == nil {
		return nil, Fail("start_unconfirmed", "vault has no confirmed running binding", 7)
	}
	comp, e := client.ReconnectComputer(ctx, r.Credentials, *r.Binding)
	if e != nil {
		return nil, remote(e)
	}
	b, _ := json.Marshal(r)
	var raw any
	_ = json.Unmarshal(b, &raw)
	return &Pine{computer: comp, secrets: append(secretStrings(raw), os.Getenv("PINE_API_KEY"))}, nil
}
func NewPineStored(ctx context.Context, dir, endpoint string) (*Pine, error) {
	if _, e := os.Stat(dir + "/vault.enc"); os.IsNotExist(e) {
		return NewPine(ctx, endpoint)
	}
	v, e := VaultFromEnv(dir)
	if e != nil {
		return nil, e
	}
	r, e := v.Load()
	if e != nil {
		return nil, e
	}
	if endpoint != r.Endpoint {
		return nil, Fail("vault_endpoint", "vault belongs to another endpoint", 6)
	}
	client, e := pine.NewClient(pine.ClientOptions{Endpoint: endpoint, APIKey: os.Getenv("PINE_API_KEY")})
	if e != nil {
		return nil, Fail("credentials", "invalid SDK client configuration", 4)
	}
	return reconnectRecord(ctx, client, r)
}

// Reconcile uses only explicitly supplied, externally verified binding data. No Start/Recover.
func Reconcile(v Vault, endpoint string, b pine.Binding) error {
	r, e := v.Load()
	if e != nil {
		return e
	}
	if r.Endpoint != endpoint || r.Credentials.ID != b.ComputerID {
		return Fail("identity", "binding does not match vault identity and endpoint", 6)
	}
	if r.Phase != "pending" {
		return Fail("state", "only an unresolved create/start can be reconciled", 6)
	}
	r.Binding = &b
	r.Phase = "ready"
	return v.Save(r)
}

// LiveCreate creates a fresh persistent Computer only after encrypted preparation.
func LiveCreate(ctx context.Context, dir, endpoint string, ttl time.Duration) (*Pine, error) {
	if !strings.HasPrefix(endpoint, "https://") {
		return nil, Fail("config", "HTTPS endpoint required", 2)
	}
	v, e := VaultFromEnv(dir)
	if e != nil {
		return nil, e
	}
	client, e := pine.NewClient(pine.ClientOptions{Endpoint: endpoint, APIKey: os.Getenv("PINE_API_KEY")})
	if e != nil {
		return nil, Fail("credentials", "invalid SDK client configuration", 4)
	}
	r, e := CreateStored(ctx, v, endpoint, pine.GenerateCredentials, ttl, func(ctx context.Context, c pine.Credentials, ttl time.Duration) (pine.Credentials, pine.Binding, error) {
		comp, err := client.CreateComputer(ctx, pine.AttachOptions{Credentials: c, Timeout: ttl, ReadyTimeout: 60 * time.Second, MaxBindAttempts: 1})
		if err != nil {
			return c, pine.Binding{}, err
		}
		binding, err := comp.Binding()
		return comp.Credentials(), binding, err
	})
	if e != nil {
		return nil, e
	}
	return reconnectRecord(ctx, client, r)
}
