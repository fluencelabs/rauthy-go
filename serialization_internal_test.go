package rauthy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func attributeWrites() map[string]func(context.Context, *Client) error {
	return map[string]func(context.Context, *Client) error{
		"create attribute": func(ctx context.Context, c *Client) error {
			_, err := c.CreateUserAttr(ctx, UserAttrRequest{Name: "department"})
			return err
		},
		"update attribute": func(ctx context.Context, c *Client) error {
			return c.UpdateUserAttr(ctx, "department", UserAttrRequest{Name: "department"})
		},
		"delete attribute": func(ctx context.Context, c *Client) error {
			return c.DeleteUserAttr(ctx, "department")
		},
	}
}

// Done is evaluated by each lock's select. The second evaluation proves that
// the attribute lock was acquired before the call waits for the scope lock.
type secondLockContext struct {
	context.Context

	calls   atomic.Int32
	waiting chan struct{}
}

func (c *secondLockContext) Done() <-chan struct{} {
	if c.calls.Add(1) == 2 {
		close(c.waiting)
	}
	return c.Context.Done()
}

func TestAttributeWritesCancelAtSecondLock(t *testing.T) {
	t.Parallel()
	for name, call := range attributeWrites() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			checkSecondLockCancellation(t, call)
		})
	}
}

func checkSecondLockCancellation(t *testing.T, call func(context.Context, *Client) error) {
	t.Helper()
	var requests atomic.Int32
	c := serializationClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := c.scopeMu.lock(ctx); err != nil {
		t.Fatal(err)
	}
	defer c.scopeMu.unlock()
	queued, stop := context.WithCancel(ctx)
	defer stop()
	observed := &secondLockContext{Context: queued, waiting: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- call(observed, c) }()
	select {
	case <-observed.waiting:
	case <-ctx.Done():
		t.Fatal("attribute write never reached the second lock")
	}
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued write = %v, want context.Canceled", err)
		}
	case <-ctx.Done():
		t.Fatal("second lock ignored cancellation")
	}
	if requests.Load() != 0 {
		t.Fatal("cancelled write reached the server")
	}
	if _, err := c.ListUserAttrs(ctx); err != nil {
		t.Fatalf("attribute lock was not released: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatal("subsequent attribute read did not reach the server")
	}
}

func serializationClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "synthetic-key")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSerialisedCallsReleaseLocksAfterErrors(t *testing.T) {
	t.Parallel()
	calls := attributeWrites()
	calls["list attributes"] = func(ctx context.Context, c *Client) error {
		_, err := c.ListUserAttrs(ctx)
		return err
	}
	calls["create scope"] = func(ctx context.Context, c *Client) error {
		_, err := c.CreateScope(ctx, ScopeRequest{Scope: "read"})
		return err
	}
	calls["update scope"] = func(ctx context.Context, c *Client) error {
		_, err := c.UpdateScope(ctx, "s1", ScopeRequest{Scope: "read"})
		return err
	}
	calls["delete scope"] = func(ctx context.Context, c *Client) error {
		return c.DeleteScope(ctx, "s1")
	}
	calls["list scopes"] = func(ctx context.Context, c *Client) error {
		_, err := c.ListScopes(ctx)
		return err
	}
	calls["update client"] = func(ctx context.Context, c *Client) error {
		_, err := c.UpdateClient(ctx, "app", UpdateClientRequest{ID: "app"})
		return err
	}
	for name, call := range calls {
		for _, failure := range []string{"http error", "malformed response"} {
			// These methods intentionally discard successful response bodies.
			discardsBody := name == "update attribute" || name == "delete attribute" || name == "delete scope"
			if failure == "malformed response" && discardsBody {
				continue
			}
			t.Run(name+"/"+failure, func(t *testing.T) {
				t.Parallel()
				checkLockRelease(t, call, failure)
			})
		}
	}
}

func checkLockRelease(t *testing.T, call func(context.Context, *Client) error, failure string) {
	t.Helper()
	var requests atomic.Int32
	c := serializationClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			if failure == "http error" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"BadRequest","message":"synthetic failure"}`))
			} else {
				_, _ = w.Write([]byte(`{`))
			}
			return
		}
		if r.URL.Path == "/auth/v1/scopes" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
		} else {
			_, _ = w.Write([]byte(`{"values":[],"id":"s1","name":"read"}`))
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := call(ctx, c); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first call = %v, want response error", err)
	}
	if err := call(ctx, c); err != nil {
		t.Fatalf("next call failed after response error: %v", err)
	}
	if _, err := c.ListUserAttrs(ctx); err != nil {
		t.Fatalf("attribute lock leaked: %v", err)
	}
	if _, err := c.ListScopes(ctx); err != nil {
		t.Fatalf("scope lock leaked: %v", err)
	}
	if requests.Load() != 4 {
		t.Fatalf("got %d requests, want 4", requests.Load())
	}
}
