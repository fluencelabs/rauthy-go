package rauthy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Retry defaults. Four attempts with a 200ms base spend under two seconds in
// the worst case, which is short enough that a caller (a `terraform apply`, say)
// does not look hung and long enough to ride out a reverse proxy reloading or a Rauthy pod
// being rescheduled.
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
}

// buildError marks a request that could not be constructed at all, as opposed
// to one that failed on the wire. A malformed method or URL will not fix itself
// on a second attempt, so it must not be mistaken for a transport failure.
type buildError struct{ err error }

func (e *buildError) Error() string { return e.err.Error() }

func (e *buildError) Unwrap() error { return e.err }

// send performs one API call, retrying transient failures. body is held as
// bytes rather than as a reader precisely so that every attempt can build a
// fresh one: a reader consumed by attempt one has nothing left to give attempt
// two, and silently sending an empty body would be worse than not retrying.
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
		case resp.status >= 200 && resp.status < 300:
			return resp, nil
		default:
			status, retryAfter = resp.status, resp.retryAfter
			lastErr = newAPIError(method, path, resp.status, resp.body)
		}

		if attempt >= c.retry.maxAttempts || !c.retryable(ctx, method, path, status, err) {
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
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
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
// GET, PUT and DELETE are idempotent across this API — Rauthy's PUTs are full
// replacements and its DELETEs answer the same way for an object that is
// already gone — so they retry on anything transient.
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
func (c *Client) retryable(ctx context.Context, method, path string, status int, err error) bool {
	if !retryableFailure(ctx, status, err) {
		return false
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		return true
	case http.MethodPut:
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
func retryableFailure(ctx context.Context, status int, err error) bool {
	if err != nil {
		return isTransport(ctx, err)
	}
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// isTransport separates a failed exchange from the failures that are ours: a
// request we could not build, or a context the caller ended.
//
// The caller's context decides the second, not the error: an http.Client
// timeout also matches context.DeadlineExceeded, but while ctx is still live
// it is the client's own limit on one attempt and counts as a transport
// failure.
func isTransport(ctx context.Context, err error) bool {
	if _, ok := errors.AsType[*buildError](err); ok {
		return false
	}
	return ctx.Err() == nil
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
