package rauthy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Retry defaults. Four attempts with a 200ms base pause for at most 1.4 seconds
// in all between attempts (backoff windows of 200ms, 400ms and 800ms), which is
// short enough that a caller (a `terraform apply`, say) does not look hung and
// long enough to ride out a reverse proxy reloading or a Rauthy pod being
// rescheduled. That bound covers the pauses only: each attempt also lasts as
// long as the server takes to answer, up to the http.Client timeout (30 seconds
// by default), and a Retry-After header can stretch a pause to
// maxHonouredRetryAfter.
const (
	defaultMaxAttempts = 4
	defaultBaseDelay   = 200 * time.Millisecond
	defaultMaxDelay    = 5 * time.Second
	// A server asking us to wait longer than this is not having a blip, it is
	// down. Failing the call beats blocking the caller for a minute.
	maxHonouredRetryAfter = 30 * time.Second
)

// retryPolicy bounds how hard the client tries. Callers change it through
// WithRetryPolicy.
type retryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

// rawResponse is a response with its body already drained, so that a retry
// never has to reason about a half-read connection.
type rawResponse struct {
	body        []byte
	contentType string
	status      int
	retryAfter  string
	// redirect replaces the body as the error message of a refused 3xx; it is
	// empty for any other status.
	redirect string
}

// buildError marks a request that could not be constructed at all, as opposed
// to one that failed on the wire. A malformed method or URL will not fix itself
// on a second attempt, so it must not be mistaken for a transport failure.
type buildError struct{ err error }

// withoutBaseURL replaces the absolute URL a *url.Error repeats with the
// request path. The base URL is a Secret: a mistaken one can carry a
// credential in its path, and errors end up in logs.
func withoutBaseURL(err error, path string) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		urlErr.URL = path
	}
	return err
}

func (e *buildError) Error() string { return e.err.Error() }

func (e *buildError) Unwrap() error { return e.err }

// send performs one API call, retrying transient failures. body is held as
// bytes rather than as a reader precisely so that every attempt can build a
// fresh one: a reader consumed by attempt one has nothing left to give attempt
// two, and silently sending an empty body would be worse than not retrying.
// headers are set on every attempt; an empty value removes the header instead.
func (c *Client) send(
	ctx context.Context,
	method, path string,
	body []byte,
	headers map[string]string,
) (*rawResponse, error) {
	var lastErr error
	for attempt := 1; ; attempt++ {
		resp, err := c.attempt(ctx, method, path, body, headers)
		status := 0
		retryAfter := ""
		switch {
		case err != nil:
			lastErr = err
		case resp.status >= 200 && resp.status < 300,
			method == http.MethodGet && strings.HasPrefix(path, "/providers/") && strings.HasSuffix(path, "/delete_safe") && resp.status == http.StatusNotAcceptable:
			return resp, nil
		default:
			status, retryAfter = resp.status, resp.retryAfter
			apiErr := newAPIError(method, path, resp.status, resp.body)
			if resp.redirect != "" {
				apiErr.Message = resp.redirect
			}
			lastErr = apiErr
		}

		if attempt >= c.retry.maxAttempts || !c.retryableRequest(method, path, body, status, err) {
			return nil, lastErr
		}
		delay, ok := c.delayFor(attempt, retryAfter)
		if !ok {
			return nil, lastErr
		}
		if waitErr := sleep(ctx, delay); waitErr != nil {
			// The context died mid-backoff. Report both why the request failed,
			// so the operator sees the cause, and the cancellation, so a caller
			// can tell that it gave up.
			return nil, fmt.Errorf("%w (gave up waiting to retry: %w)", lastErr, waitErr)
		}
	}
}

// attempt issues the request exactly once and drains the response.
func (c *Client) attempt(
	ctx context.Context,
	method, path string,
	body []byte,
	headers map[string]string,
) (*rawResponse, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := c.newRequest(ctx, method, path, reader)
	if err != nil {
		return nil, &buildError{err: err}
	}
	for k, v := range headers {
		if v == "" {
			// An empty value takes back a header newRequest set: it is how a
			// call to a public endpoint leaves out the API key.
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isUnparseableLocation(err) {
			return nil, &redirectError{method: method, path: path}
		}
		return nil, fmt.Errorf("%s %s: %w", method, path, withoutBaseURL(err, path))
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		// A truncated read is a transport failure like any other, and is
		// reported as one so the retry rules see it that way.
		return nil, fmt.Errorf("read %s %s response: %w", method, path, err)
	}
	return &rawResponse{
		body:        raw,
		contentType: resp.Header.Get("Content-Type"),
		status:      resp.StatusCode,
		retryAfter:  resp.Header.Get("Retry-After"),
		redirect:    redirectMessage(resp.StatusCode, req.URL, resp.Header.Get("Location")),
	}, nil
}

// retryable decides whether the failure just seen may be tried again. status is
// zero when no response was received at all.
//
// The whole question is idempotency, and the cost of the two mistakes is not
// symmetric: retrying too timidly fails a call the operator can simply
// re-run, while retrying a create that actually succeeded leaves a duplicate
// user behind that the caller does not know about.
//
// What is retried at all: a 429, a 5xx, and a transport failure — a refused
// dial, a reset, an EOF. Never retried, whatever the method: any other 4xx, a
// 3xx, and an attempt that ended because a deadline passed, be it the
// http.Client timeout or the caller's context. A request that timed out may
// still be running on the server, and re-sending it would overlap it there;
// with a 30 second timeout it would also turn a hung server into minutes of
// waiting.
//
// GET, PUT and DELETE are idempotent across this API — Rauthy's PUTs are full
// replacements (except password changes) and its DELETEs answer the same way for an object that is
// already gone — so they retry on every failure in the first list.
//
// POST is not, in general, and gets two narrow exemptions:
//
//   - A 429 was refused before the handler ran, whoever produced it. Waiting
//     and re-sending is precisely what the status asks for, so it is safe here.
//   - A failure that happened before any byte reached the server (name
//     resolution, a refused or unreachable dial) cannot have created anything.
//     Past that point we no longer know: a 502 or a timeout may well be sitting
//     on top of a create that committed, so those are not retried for POST.
//
// The one POST endpoint that is an upsert by design — POST /blacklist, see
// BlacklistIP — opts out of that caution by path, since re-posting it only
// rewrites an expiry it was already going to write.
//
// The PUTs that rotate a secret — /clients/{id}/secret and
// /api_keys/{name}/secret — are the exception the other way: each one mints a
// new secret, so re-sending one that committed rotates twice. They get the
// POST rule.
func (c *Client) retryable(method, path string, status int, err error) bool {
	if !retryableFailure(status, err) {
		return false
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		return true
	case http.MethodPut:
		// A 500 is Rauthy's own error after it handled the request: it may
		// have stored the change and then failed a side effect, such as the
		// SCIM sync after a client update. A replay then finds nothing to
		// change, answers 200 and hides that failure.
		if status == http.StatusInternalServerError {
			return false
		}
		if !strings.HasSuffix(path, secretRotationSuffix) {
			return true
		}
		return status == http.StatusTooManyRequests || (err != nil && sentNothing(err))
	case http.MethodPost:
		if path == blacklistPathBase || status == http.StatusTooManyRequests {
			return true
		}
		return err != nil && sentNothing(err)
	default:
		return false
	}
}

// A user password update checks password history, so replaying a committed
// update can reject the password that the first attempt just installed.
func (c *Client) retryableRequest(method, path string, body []byte, status int, err error) bool {
	if method == http.MethodPut && strings.HasPrefix(path, "/users/") {
		var request struct {
			Password *string `json:"password"`
		}
		if json.Unmarshal(body, &request) == nil && request.Password != nil {
			return retryableFailure(status, err) &&
				(status == http.StatusTooManyRequests || (err != nil && sentNothing(err)))
		}
	}
	return c.retryable(method, path, status, err)
}

// secretRotationSuffix ends the path of every PUT that rotates a secret. An
// identifier cannot forge it: url.PathEscape turns a "/" inside one into %2F.
const secretRotationSuffix = "/secret"

// delayFor returns how long to wait before the next attempt, and whether to
// wait at all. A Retry-After from the server wins over our own backoff: it is
// the only party that knows when its rate-limit window reopens.
//
// Rauthy 0.36.2 itself never sends the header — its rate limiting sits on the
// public auth endpoints rather than the admin API, and the header name appears
// nowhere in its binary outside the http crate's static table. It is honoured
// for the hop that does produce our 429s and 503s in practice: the reverse
// proxy, ingress or CDN in front of Rauthy.
func (c *Client) delayFor(attempt int, retryAfter string) (time.Duration, bool) {
	if d, ok := parseRetryAfter(retryAfter); ok {
		if d > maxHonouredRetryAfter {
			return 0, false
		}
		return d, true
	}
	return c.retry.backoff(attempt), true
}

// backoff is exponential with jitter over the upper half of the window. Full
// jitter down to zero would let a fleet of parallel applies retry almost
// immediately and re-collide; keeping the floor at half the window spreads them
// out without throwing the backoff away.
//
// A zero base delay means no delay at all. The window is clamped to maxDelay
// before the shift, not after it: a shift that pushed bits out of the int64
// could otherwise leave a small positive window below the cap.
func (p retryPolicy) backoff(attempt int) time.Duration {
	if p.baseDelay <= 0 || p.maxDelay <= 0 {
		return 0
	}
	window := p.maxDelay
	if shift := attempt - 1; shift < maxBackoffShift && p.baseDelay <= p.maxDelay>>shift {
		window = p.baseDelay << shift
	}
	const jitterFloor = 2 // half the window, the lowest a jittered delay may fall to
	floor := window / jitterFloor
	return floor + time.Duration(rand.Int64N(int64(floor)+1)) //nolint:gosec // scheduling jitter, not a secret
}

// maxBackoffShift is the widest shift of a positive int64 that can still be
// compared against the cap.
const maxBackoffShift = 63

// validate refuses a negative attempt budget or delay, neither of which has a
// meaning.
func (p retryPolicy) validate() error {
	if p.maxAttempts < 0 || p.baseDelay < 0 || p.maxDelay < 0 {
		return fmt.Errorf(
			"rauthy retry policy must not be negative: %d attempts, %v base delay, %v max delay",
			p.maxAttempts, p.baseDelay, p.maxDelay,
		)
	}
	return nil
}

// retryableFailure reports whether a failure is transient at all, before any
// question of which method produced it. 4xx other than 429 are the caller's own
// doing — a malformed body, a missing right — and will fail identically however
// many times they are re-sent.
func retryableFailure(status int, err error) bool {
	if err != nil {
		return isTransport(err)
	}
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// isTransport separates a failed exchange from the failures that are ours or
// that a retry cannot fix: a request we could not build, a redirect we refused,
// and an attempt that ran out of time. The last covers both a context the
// caller ended and an http.Client timeout, which net/http also reports as
// context.DeadlineExceeded; neither is retried (see retryable for why).
func isTransport(err error) bool {
	if _, ok := errors.AsType[*buildError](err); ok {
		return false
	}
	if _, ok := errors.AsType[*redirectError](err); ok {
		return false
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// sentNothing reports whether err proves the request never left the client.
// Only name resolution and dial failures qualify: everything past that point —
// a write error, a reset, an EOF, a timeout — leaves open the possibility that
// the server read the request and acted on it.
func sentNothing(err error) bool {
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

// parseRetryAfter reads both spellings RFC 9110 allows: delay-seconds and an
// HTTP-date. A date already in the past means "now".
func parseRetryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if when, err := http.ParseTime(v); err == nil {
		return max(time.Until(when), 0), true
	}
	return 0, false
}

// sleep waits for d unless the context ends first.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
