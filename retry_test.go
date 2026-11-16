package rauthy_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fluencelabs/rauthy-go"
)

// retryTestPolicy shrinks the budget to something a test can afford. The point
// of the retry code is which requests are re-sent, not how long the pauses are,
// and the pauses are the only expensive part.
func retryTestPolicy() rauthy.Option {
	return rauthy.WithRetryPolicy(4, time.Microsecond, time.Millisecond)
}

// newRetryClient wires a client with the fast policy to a counting server, and
// returns the request counter.
func newRetryClient(t *testing.T, handler http.HandlerFunc) (*rauthy.Client, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	c, err := rauthy.New(srv.URL, "tf$secret", retryTestPolicy())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, &calls
}

func TestRetry_TransientFailuresThenSuccess(t *testing.T) {
	t.Parallel()

	for _, status := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusTooManyRequests,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			var seen atomic.Int32
			c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if seen.Add(1) <= 2 {
					w.WriteHeader(status)
					return
				}
				_, _ = io.WriteString(w, `{"id":"u1","email":"a@example.com"}`)
			})

			user, err := c.GetUser(t.Context(), "u1")
			if err != nil {
				t.Fatalf("GetUser after 2 x %d: %v", status, err)
			}
			if user.ID != "u1" {
				t.Errorf("ID = %q, want u1", user.ID)
			}
			if got := calls.Load(); got != 3 {
				t.Errorf("requests = %d, want 3", got)
			}
		})
	}
}

func TestRetry_ExhaustsBudgetAndReportsLastError(t *testing.T) {
	t.Parallel()

	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"Internal","message":"upstream is down"}`)
	})

	_, err := c.GetUser(t.Context(), "u1")

	var apiErr *rauthy.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadGateway || apiErr.Message != "upstream is down" ||
		apiErr.ErrorCode != "Internal" {
		t.Errorf("err = %v, want the last 502 status and error code", apiErr)
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("requests = %d, want the full budget of 4", got)
	}
}

func TestRetry_ClientErrorsAreNotRetried(t *testing.T) {
	t.Parallel()

	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"BadRequest","message":"nope"}`)
			})

			if _, err := c.GetUser(t.Context(), "u1"); err == nil {
				t.Fatalf("GetUser: want error for HTTP %d", status)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("requests = %d, want 1: HTTP %d will never succeed", got, status)
			}
		})
	}
}

// A Retry-After the client can afford is waited out and the call proceeds; one
// far beyond the honoured ceiling ends the call without another attempt.
func TestRetry_HonoursRetryAfter(t *testing.T) {
	t.Parallel()

	t.Run("short delay is waited out", func(t *testing.T) {
		t.Parallel()

		var seen atomic.Int32
		first := make(chan time.Time, 1)
		c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if seen.Add(1) == 1 {
				first <- time.Now()
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			started := <-first
			first <- started
			if elapsed := time.Since(started); elapsed < time.Second {
				t.Errorf("retry after %v, want at least 1s", elapsed)
			}
			_, _ = io.WriteString(w, `{"id":"u1"}`)
		})

		user, err := c.GetUser(t.Context(), "u1")
		if err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		if user.ID != "u1" {
			t.Errorf("user = %+v", user)
		}
		if got := calls.Load(); got != 2 {
			t.Errorf("requests = %d, want 2", got)
		}
	})

	t.Run("long delay gives up immediately", func(t *testing.T) {
		t.Parallel()

		c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		if _, err := c.GetUser(t.Context(), "u1"); err == nil {
			t.Fatal("GetUser: want error")
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("requests = %d, want 1: a ten-minute wait is not a blip", got)
		}
	})

	t.Run("http-date is understood", func(t *testing.T) {
		t.Parallel()

		c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		if _, err := c.GetUser(t.Context(), "u1"); err == nil {
			t.Fatal("GetUser: want error")
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("requests = %d, want 1", got)
		}
	})
}

// A retried write must arrive whole on the second attempt: the body reader is
// consumed by the first one, so it has to be rebuilt rather than reused.
func TestRetry_ReplaysRequestBody(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	bodies := make(chan string, 4)
	c, calls := newRetryClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies <- string(raw)
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// POST /blacklist rather than POST /users: it is the upsert, and so the one
	// POST this client will re-send after a 503.
	err := c.BlacklistIP(t.Context(), rauthy.IPBlacklistRequest{IP: "192.0.2.7", Exp: 4102444800})
	if err != nil {
		t.Fatalf("BlacklistIP: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}

	close(bodies)
	for body := range bodies {
		var got rauthy.IPBlacklistRequest
		if unmarshalErr := json.Unmarshal([]byte(body), &got); unmarshalErr != nil {
			t.Fatalf("attempt body %q is not the request: %v", body, unmarshalErr)
		}
		if got.IP != "192.0.2.7" || got.Exp != 4102444800 {
			t.Errorf("attempt body = %+v, want the original request", got)
		}
	}
}

// The idempotency rule the whole design turns on: a POST that creates is not
// re-sent once the server has seen it, however transient the failure looks.
func TestRetry_CreatingPOSTIsNotRetried(t *testing.T) {
	t.Parallel()

	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	_, err := c.CreateUser(t.Context(), rauthy.NewUserRequest{Email: "a@example.com"})
	if err == nil {
		t.Fatal("CreateUser: want error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1: a 502 may sit on top of a user that was created", got)
	}
}

// A 429 is different: it was refused before the handler ran, so even a create
// may be re-sent.
func TestRetry_CreatingPOSTIsRetriedOnTooManyRequests(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"id":"u1","email":"a@example.com"}`)
	})

	user, err := c.CreateUser(t.Context(), rauthy.NewUserRequest{Email: "a@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID != "u1" {
		t.Errorf("ID = %q, want u1", user.ID)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// The binary endpoints go through the same plumbing, so they retry too.
func TestRetry_CoversDownload(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte{0x00, 0x01})
	})

	img, err := c.GetClientImage(t.Context(), "cl1", rauthy.ImageLogo)
	if err != nil {
		t.Fatalf("GetClientImage: %v", err)
	}
	if img.ContentType != "image/webp" || len(img.Data) != 2 {
		t.Errorf("image = %+v, want the second attempt's body and type", img)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// The upload's pre-encoded multipart body has to be replayed whole, exactly as
// the JSON one does.
func TestRetry_CoversUpload(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	sizes := make(chan int, 4)
	c, calls := newRetryClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		sizes <- len(raw)
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	img := rauthy.Image{Data: []byte("<svg/>"), ContentType: rauthy.MimeSVG}
	if err := c.PutClientImage(t.Context(), "cl1", rauthy.ImageLogo, img); err != nil {
		t.Fatalf("PutClientImage: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}

	close(sizes)
	var first, second int
	for n := range sizes {
		if first == 0 {
			first = n
			continue
		}
		second = n
	}
	if first == 0 || first != second {
		t.Errorf("body sizes = %d then %d, want the multipart body replayed whole", first, second)
	}
}

// A dead server is a transport failure, and for a GET that is retried until the
// budget runs out.
func TestRetry_ConnectionErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	c, err := rauthy.New(url, "tf$secret", retryTestPolicy())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err = c.GetUser(t.Context(), "u1"); err == nil {
		t.Fatal("GetUser: want a connection error")
	}
	if _, ok := errors.AsType[*rauthy.APIError](err); ok {
		t.Errorf("err = %v, want a transport error rather than an APIError", err)
	}
}

// A cancelled context must end the call rather than sit out the backoff.
func TestRetry_StopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	// A budget with real pauses, so that a retry that ignored the context would
	// visibly outlast the test's own patience.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	c, err := rauthy.New(srv.URL, "tf$secret", rauthy.WithRetryPolicy(10, 30*time.Second, time.Minute))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		for calls.Load() == 0 { //nolint:revive // spin until the first attempt has landed
		}
		// Let the 503 reach the client, so the cancellation lands in the
		// backoff rather than on the first exchange.
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() { _, getErr := c.GetUser(ctx, "u1"); done <- getErr }()

	select {
	case getErr := <-done:
		if !errors.Is(getErr, context.Canceled) {
			t.Errorf("err = %v, want it to carry context.Canceled", getErr)
		}
		apiErr, ok := errors.AsType[*rauthy.APIError](getErr)
		if !ok || apiErr.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("err = %v, want it to keep the 503 that caused the retry", getErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GetUser did not return: the backoff ignored the cancelled context")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

// Rotating a secret is not a replacement: re-sending a rotation that did
// commit mints a second secret and pushes the one the caller will see out of
// the grace window. Once the request may have reached the server it is not
// repeated.
func TestRetry_SecretRotationIsNotRetried(t *testing.T) {
	t.Parallel()

	rotations := map[string]func(*rauthy.Client) error{
		"client secret": func(c *rauthy.Client) error {
			_, err := c.RotateClientSecret(t.Context(), "app", nil)
			return err
		},
		"api key secret": func(c *rauthy.Client) error {
			_, err := c.RotateAPIKeySecret(t.Context(), "deploy")
			return err
		},
	}
	for name, rotate := range rotations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			})
			if err := rotate(c); err == nil {
				t.Fatal("rotation: want error")
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("requests = %d, want 1: a 502 may sit on top of a rotation that committed", got)
			}
		})
	}
}

// A 429 was refused before the handler ran, so a rotation may be re-sent.
func TestRetry_SecretRotationIsRetriedOnTooManyRequests(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"id":"app","confidential":true,"secret":"new"}`)
	})

	if _, err := c.RotateClientSecret(t.Context(), "app", nil); err != nil {
		t.Fatalf("RotateClientSecret: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// slowFirstServer holds its first request past the client's timeout and
// answers the rest at once, so a retry would succeed.
func slowFirstServer(t *testing.T, hold time.Duration, body string) (string, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			select {
			case <-r.Context().Done():
			case <-time.After(hold):
			}
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &calls
}

// An attempt cut off by the http.Client timeout may still be running on the
// server, so it is not re-sent, whatever the method.
func TestRetry_HTTPClientTimeoutIsNotRetried(t *testing.T) {
	t.Parallel()

	calls := map[string]func(*rauthy.Client) error{
		"GET": func(c *rauthy.Client) error {
			_, err := c.GetUser(t.Context(), "u1")
			return err
		},
		"PUT": func(c *rauthy.Client) error {
			return c.UpdateUserAttr(t.Context(), "dept", rauthy.UserAttrRequest{Name: "dept"})
		},
		"DELETE": func(c *rauthy.Client) error {
			return c.DeleteUser(t.Context(), "u1")
		},
		"POST": func(c *rauthy.Client) error {
			_, err := c.CreateUser(t.Context(), rauthy.NewUserRequest{Email: "a@example.com"})
			return err
		},
	}
	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			url, served := slowFirstServer(t, 5*time.Second, `{"id":"u1","email":"a@example.com"}`)
			c, err := rauthy.New(url, "tf$secret", retryTestPolicy(),
				rauthy.WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err = call(c); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want the client timeout", err)
			}
			if got := served.Load(); got != 1 {
				t.Errorf("requests = %d, want 1", got)
			}
		})
	}
}

// A deadline on the caller's own context ends the call without a retry.
func TestRetry_CallerDeadlineIsNotRetried(t *testing.T) {
	t.Parallel()

	url, calls := slowFirstServer(t, 5*time.Second, `{"id":"u1","email":"a@example.com"}`)
	c, err := rauthy.New(url, "tf$secret", retryTestPolicy())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err = c.GetUser(ctx, "u1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetUser: err = %v, want context.DeadlineExceeded", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestWithRetryPolicy_RejectsNegativeValues(t *testing.T) {
	t.Parallel()

	cases := map[string]rauthy.Option{
		"attempts":   rauthy.WithRetryPolicy(-1, time.Millisecond, time.Second),
		"base delay": rauthy.WithRetryPolicy(4, -time.Millisecond, time.Second),
		"max delay":  rauthy.WithRetryPolicy(4, time.Millisecond, -time.Second),
	}
	for name, opt := range cases {
		if _, err := rauthy.New("https://auth.example.com", "tf$secret", opt); err == nil {
			t.Errorf("negative %s: New succeeded, want error", name)
		}
	}
}

// A zero base delay asks for retries without a pause, not for the longest
// pause the cap allows.
func TestWithRetryPolicy_ZeroBaseDelayMeansNoDelay(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if seen.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"id":"u1","email":"a@example.com"}`)
	}))
	t.Cleanup(srv.Close)

	c, err := rauthy.New(srv.URL, "tf$secret", rauthy.WithRetryPolicy(4, 0, time.Hour))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err = c.GetUser(ctx, "u1"); err != nil {
		t.Fatalf("GetUser: %v", err)
	}
}

type retryRoundTripper func(*http.Request) (*http.Response, error)

func (f retryRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestRetry_TransportErrorThenSuccess(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	transport := retryRoundTripper(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"u1","email":"a@example.com"}`)),
			Request:    r,
		}, nil
	})
	c, err := rauthy.New("https://auth.example.com", "tf$secret",
		retryTestPolicy(), rauthy.WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	user, err := c.GetUser(t.Context(), "u1")
	if got := calls.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.ID != "u1" || user.Email != "a@example.com" {
		t.Errorf("user = %+v", user)
	}
}
