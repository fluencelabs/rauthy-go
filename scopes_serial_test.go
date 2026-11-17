package rauthy_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fluencelabs/rauthy-go"
)

// Rauthy rewrites its cached scope list on every write without locking, so two
// overlapping writes lose one scope from the cache: it then reads as missing
// although it exists. The client serialises scope writes, as Terraform creates
// independent scopes ten at a time.
func TestScopeWritesAreSerialised(t *testing.T) {
	t.Parallel()

	var inFlight, overlaps atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if inFlight.Add(1) > 1 {
			overlaps.Add(1)
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"s1","name":"read"}`))
	})

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			var err error
			switch i % 3 {
			case 0:
				_, err = c.CreateScope(context.Background(), rauthy.ScopeRequest{Scope: "read"})
			case 1:
				_, err = c.UpdateScope(context.Background(), "s1", rauthy.ScopeRequest{Scope: "read"})
			default:
				err = c.DeleteScope(context.Background(), "s1")
			}
			if err != nil {
				t.Errorf("write: %v", err)
			}
		})
	}
	wg.Wait()

	if got := overlaps.Load(); got != 0 {
		t.Errorf("%d scope writes overlapped; they must be serialised", got)
	}
}

// A scope write waiting behind another gives up when its context ends.
func TestQueuedScopeWriteHonoursDeadline(t *testing.T) {
	t.Parallel()

	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"s1","name":"read"}`))
	})
	done := make(chan error, 1)
	go func() {
		_, err := c.CreateScope(context.Background(), rauthy.ScopeRequest{Scope: "read"})
		done <- err
	}()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err := c.DeleteScope(ctx, "s1")
	close(release)
	if firstErr := <-done; firstErr != nil {
		t.Fatal(firstErr)
	}
	if err == nil || ctx.Err() == nil {
		t.Fatalf("queued DeleteScope = %v, want the context's deadline", err)
	}
	if calls.Load() != 1 {
		t.Errorf("the queued write reached the server: %d calls", calls.Load())
	}
}
