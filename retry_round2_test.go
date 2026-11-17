package rauthy_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
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

func TestPasswordUpdateIsNotReplayedAfterBadGateway(t *testing.T) {
	t.Parallel()
	for _, password := range []*rauthy.Secret{nil, new(rauthy.NewSecret("SYNTHETIC_PASSWORD"))} {
		t.Run(strconv.FormatBool(password != nil), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			transport := retryRoundTripper(func(_ *http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"synthetic proxy failure"}`)),
				}, nil
			})
			c, err := rauthy.New("https://auth.example.com", "key",
				rauthy.WithHTTPClient(&http.Client{Transport: transport}),
				rauthy.WithRetryPolicy(3, 0, 0))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.UpdateUser(t.Context(), "user1", rauthy.UpdateUserRequest{Password: password})
			want := int32(3)
			if password != nil {
				want = 1
			}
			var apiErr *rauthy.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadGateway {
				t.Errorf("expected original HTTP 502, got %v", err)
			}
			if calls.Load() != want {
				t.Errorf("calls=%d want=%d", calls.Load(), want)
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

// A PUT that changes a password is replayed only when Rauthy cannot have
// applied it: a 429, or a failure before the request was sent. A 5xx or a
// broken read may follow a committed change.
func TestPasswordUpdateRetryMatrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		cause  error
		want   int32
	}{
		{"500", http.StatusInternalServerError, nil, 1},
		{"503", http.StatusServiceUnavailable, nil, 1},
		{"504", http.StatusGatewayTimeout, nil, 1},
		{"429", http.StatusTooManyRequests, nil, 3},
		{"dns", 0, &net.DNSError{Err: "no such host", Name: "auth.invalid", IsNotFound: true}, 3},
		{"refused", 0, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, 3},
		{"reset", 0, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			transport := retryRoundTripper(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				if tc.cause != nil {
					return nil, tc.cause
				}
				return &http.Response{
					StatusCode: tc.status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"failure"}`)),
				}, nil
			})
			c, err := rauthy.New("https://auth.invalid", "key",
				rauthy.WithHTTPClient(&http.Client{Transport: transport}),
				rauthy.WithRetryPolicy(3, 0, 0))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.UpdateUser(t.Context(), "user1",
				rauthy.UpdateUserRequest{Password: new(rauthy.NewSecret("SYNTHETIC_PASSWORD"))})
			if err == nil {
				t.Fatal("UpdateUser with a failing transport succeeded")
			}
			if calls.Load() != tc.want {
				t.Errorf("calls=%d want=%d", calls.Load(), tc.want)
			}
		})
	}
}

// A PUT can be stored before a 500, regardless of its error code. Replaying
// it could hide that failure. Proxy failures and rate limits remain retryable.
func TestPutRetryOnServerErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		body   string
		code   string
		want   int32
	}{
		{"scim", http.StatusInternalServerError, `{"error":"Scim","message":"sync failed"}`, "Scim", 1},
		{"connection", http.StatusInternalServerError, `{"error":"Connection","message":"leader changed"}`, "Connection", 1},
		{"unknown", http.StatusInternalServerError, `{"error":"Unknown","message":"failure"}`, "Unknown", 1},
		{"empty", http.StatusInternalServerError, ``, "", 1},
		{"bad gateway", http.StatusBadGateway, `{"message":"proxy"}`, "", 2},
		{"unavailable", http.StatusServiceUnavailable, `{"message":"proxy"}`, "", 2},
		{"timeout", http.StatusGatewayTimeout, `{"message":"proxy"}`, "", 2},
		{"too many", http.StatusTooManyRequests, `{"message":"slow down"}`, "", 2},
	} {
		for _, call := range []string{"client", "group"} {
			t.Run(tc.name+"/"+call, func(t *testing.T) {
				t.Parallel()
				calls, err := putAfterFailure(t, call, tc.status, tc.body)
				if calls != tc.want {
					t.Errorf("calls=%d want=%d", calls, tc.want)
				}
				var apiErr *rauthy.APIError
				switch {
				case tc.want == 1 && (!errors.As(err, &apiErr) || apiErr.ErrorCode != tc.code ||
					apiErr.StatusCode != http.StatusInternalServerError):
					t.Errorf("err = %v, want the original %s 500", err, tc.code)
				case tc.want > 1 && err != nil:
					t.Errorf("err = %v, want success after the retry", err)
				}
			})
		}
	}
}

// putAfterFailure sends one PUT that first meets status with body, then 200,
// and returns how many attempts reached the server.
func putAfterFailure(t *testing.T, call string, status int, body string) (int32, error) {
	t.Helper()
	var calls atomic.Int32
	transport := retryRoundTripper(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return &http.Response{
				StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":"x","name":"x"}`)),
		}, nil
	})
	c, err := rauthy.New("https://auth.invalid", "key",
		rauthy.WithHTTPClient(&http.Client{Transport: transport}),
		rauthy.WithRetryPolicy(3, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	update := func() error {
		if call == "client" {
			_, updateErr := c.UpdateClient(t.Context(), "x", rauthy.UpdateClientRequest{ID: "x"})
			return updateErr
		}
		_, updateErr := c.UpdateGroup(t.Context(), "x", rauthy.GroupRequest{Group: "x"})
		return updateErr
	}
	err = update()
	firstCalls := calls.Load()
	if nextErr := update(); nextErr != nil {
		t.Errorf("next update on the same Client: %v", nextErr)
	}
	if got := calls.Load() - firstCalls; got != 1 {
		t.Errorf("next update requests=%d, want 1", got)
	}
	return firstCalls, err
}

func TestGetDeleteRetryOnInternalServerError(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			var seen atomic.Int32
			c, calls := newRetryClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("method=%s, want %s", r.Method, method)
				}
				if seen.Add(1) == 1 {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = io.WriteString(w, `{"error":"Connection","message":"leader changed"}`)
					return
				}
				_, _ = io.WriteString(w, `{"id":"x","name":"x"}`)
			})
			var err error
			if method == http.MethodGet {
				_, err = c.GetUser(t.Context(), "x")
			} else {
				err = c.DeleteGroup(t.Context(), "x")
			}
			if err != nil {
				t.Errorf("%s after 500: %v", method, err)
			}
			if got := calls.Load(); got != 2 {
				t.Errorf("requests=%d, want 2", got)
			}
		})
	}
}
