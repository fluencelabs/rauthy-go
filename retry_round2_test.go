package rauthy_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	rauthy "github.com/fluencelabs/rauthy-go"
)

func TestPasswordUpdateIsNotReplayed(t *testing.T) {
	t.Parallel()
	for _, password := range []*rauthy.Secret{nil, new(rauthy.NewSecret("SYNTHETIC_PASSWORD"))} {
		t.Run(strconv.FormatBool(password != nil), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			transport := retryRoundTripper(
				func(_ *http.Request) (*http.Response, error) { calls.Add(1); return nil, io.ErrUnexpectedEOF },
			)
			c, err := rauthy.New(
				"https://auth.example.com",
				"key",
				rauthy.WithHTTPClient(&http.Client{Transport: transport}),
				rauthy.WithRetryPolicy(3, 0, 0),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.UpdateUser(t.Context(), "user1", rauthy.UpdateUserRequest{Password: password})
			want := int32(3)
			if password != nil {
				want = 1
			}
			if !errors.Is(err, io.ErrUnexpectedEOF) || calls.Load() != want {
				t.Errorf("calls=%d want=%d err=%v", calls.Load(), want, err)
			}
		})
	}
}

func TestQueuedAttributeDeadline(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		_, _ = io.WriteString(w, `{"name":"first"}`)
	})
	done := make(chan error, 1)
	go func() { _, err := c.CreateUserAttr(t.Context(), rauthy.UserAttrRequest{Name: "first"}); done <- err }()
	<-entered
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	for _, operation := range []string{"create", "update", "delete"} {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			var err error
			switch operation {
			case "create":
				_, err = c.CreateUserAttr(ctx, rauthy.UserAttrRequest{Name: "second"})
			case "update":
				err = c.UpdateUserAttr(ctx, "second", rauthy.UserAttrRequest{Name: "second"})
			case "delete":
				err = c.DeleteUserAttr(ctx, "second")
			}
			result <- err
		}()
		select {
		case err := <-result:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("%s: %v", operation, err)
			}
		case <-time.After(time.Second):
			t.Errorf("%s ignored deadline while queued", operation)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("queued requests reached server: %d", calls.Load())
	}
}
