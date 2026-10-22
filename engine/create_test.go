package engine

import (
	"context"
	"errors"
	pine "go.pinesandbox.io/computer"
	"testing"
	"time"
)

func TestCreateDurableIdentityAndRecovery(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "lost-response"}[uncertain], func(t *testing.T) {
			v := vaultFixture(t)
			r := fakeRecord()
			generated, calls := 0, 0
			generate := func() (pine.Credentials, error) { generated++; return r.Credentials, nil }
			create := func(_ context.Context, c pine.Credentials, _ time.Duration) (pine.Credentials, pine.Binding, error) {
				calls++
				saved, e := v.Load()
				if e != nil || saved.Phase != "pending" || saved.Operation != "create" || saved.Credentials.ID != c.ID {
					t.Fatal("identity not durable before create", e)
				}
				if uncertain {
					return c, pine.Binding{}, errors.New("SECRET upstream response")
				}
				return c, *r.Binding, nil
			}
			for i := 0; i < 2; i++ {
				_, e := CreateStored(context.Background(), v, r.Endpoint, generate, time.Minute, create)
				if (e != nil) != uncertain {
					t.Fatal(e)
				}
			}
			if generated != 1 || calls != 1 {
				t.Fatalf("generation=%d remote=%d", generated, calls)
			}
			if _, e := StartStored(context.Background(), v, r.Endpoint, r.Credentials, time.Minute, create); e == nil {
				t.Fatal("create vault accepted as start")
			}
			if uncertain {
				if e := Reconcile(v, r.Endpoint, *r.Binding); e != nil {
					t.Fatal(e)
				}
				if _, e := CreateStored(context.Background(), v, r.Endpoint, generate, time.Minute, create); e != nil {
					t.Fatal(e)
				}
				if calls != 1 || generated != 1 {
					t.Fatal("reconcile replayed create")
				}
			}
		})
	}
}

func TestCreatePersistenceFailures(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		v := &failingVault{failAt: failAt}
		r := fakeRecord()
		calls, generated := 0, 0
		generate := func() (pine.Credentials, error) { generated++; return r.Credentials, nil }
		create := func(_ context.Context, c pine.Credentials, _ time.Duration) (pine.Credentials, pine.Binding, error) {
			calls++
			return c, *r.Binding, nil
		}
		if _, e := CreateStored(context.Background(), v, r.Endpoint, generate, time.Minute, create); e == nil {
			t.Fatal("expected failure")
		}
		if failAt < 3 && calls != 0 {
			t.Fatal("remote call before durable pending")
		}
		if failAt == 3 {
			if v.record.Phase != "pending" {
				t.Fatal("lost pending")
			}
			if _, e := CreateStored(context.Background(), v, r.Endpoint, generate, time.Minute, create); e == nil || calls != 1 {
				t.Fatal("unsafe retry")
			}
		}
		if failAt == 2 {
			if _, e := CreateStored(context.Background(), v, r.Endpoint, generate, time.Minute, create); e != nil || generated != 1 || calls != 1 {
				t.Fatal("prepared identity not reused", e)
			}
		}
	}
}

func TestCreateRejectsWrongEndpointAndIdentity(t *testing.T) {
	v := vaultFixture(t)
	r := fakeRecord()
	calls := 0
	generate := func() (pine.Credentials, error) { return r.Credentials, nil }
	create := func(_ context.Context, c pine.Credentials, _ time.Duration) (pine.Credentials, pine.Binding, error) {
		calls++
		b := *r.Binding
		b.ComputerID = "different"
		return c, b, nil
	}
	if _, e := CreateStored(context.Background(), v, r.Endpoint, generate, time.Minute, create); e == nil {
		t.Fatal("wrong remote identity accepted")
	}
	if _, e := CreateStored(context.Background(), v, "https://other.invalid", generate, time.Minute, create); e == nil || calls != 1 {
		t.Fatal("wrong endpoint accepted")
	}
	if _, e := CreateStored(context.Background(), v, r.Endpoint, generate, time.Minute, create); e == nil || calls != 1 {
		t.Fatal("identity mismatch retried")
	}
}
