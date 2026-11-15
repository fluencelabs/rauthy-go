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

	"github.com/fluencelabs/rauthy-go"
)

// newTestClient wires a Client to a test server and records the last request it
// received.
func newTestClient(t *testing.T, handler http.HandlerFunc) *rauthy.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := rauthy.New(srv.URL, "tf$secret")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNew_AppendsAPIBasePath(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"https://auth.example.com":          "https://auth.example.com/auth/v1",
		"https://auth.example.com/":         "https://auth.example.com/auth/v1",
		"https://auth.example.com/auth/v1":  "https://auth.example.com/auth/v1",
		"https://auth.example.com/auth/v1/": "https://auth.example.com/auth/v1",
	}
	for in, want := range cases {
		c, err := rauthy.New(in, "tf$secret")
		if err != nil {
			t.Fatalf("New(%q): %v", in, err)
		}
		if got := c.BaseURL(); got != want {
			t.Errorf("New(%q).BaseURL() = %q, want %q", in, got, want)
		}
	}
}

func TestNew_RejectsBadConfig(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ url, key string }{
		"empty url":     {"", "tf$secret"},
		"empty key":     {"https://auth.example.com", ""},
		"no scheme":     {"auth.example.com", "tf$secret"},
		"wrong scheme":  {"ftp://auth.example.com", "tf$secret"},
		"blank api key": {"https://auth.example.com", "   "},
	}
	for name, tc := range cases {
		if _, err := rauthy.New(tc.url, tc.key); err == nil {
			t.Errorf("%s: New(%q, %q) succeeded, want error", name, tc.url, tc.key)
		}
	}
}

func TestCreateClient_SendsAPIKeyHeaderAndMinimalBody(t *testing.T) {
	t.Parallel()

	var gotAuth, gotPath, gotMethod string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","enabled":true,"confidential":true,` +
			`"redirect_uris":["https://app.example.com/cb"],"flows_enabled":["authorization_code"],` +
			`"access_token_alg":"EdDSA","id_token_alg":"EdDSA","auth_code_lifetime":60,` +
			`"access_token_lifetime":600,"scopes":["openid"],"default_scopes":["openid"],"force_mfa":false}`))
	})

	name := "App"
	_, err := c.CreateClient(context.Background(), rauthy.NewClientRequest{
		ID:           "app",
		Name:         &name,
		Confidential: true,
		RedirectURIs: []string{"https://app.example.com/cb"},
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	if gotAuth != "API-Key tf$secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "API-Key tf$secret")
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/clients" {
		t.Errorf("got %s %s, want POST /auth/v1/clients", gotMethod, gotPath)
	}
	// Anything beyond these five fields is silently dropped by Rauthy, so the
	// request type must not carry them at all.
	for _, unwanted := range []string{"flows_enabled", "scopes", "access_token_alg", "enabled", "secret"} {
		if _, ok := gotBody[unwanted]; ok {
			t.Errorf("POST /clients body carries %q, which Rauthy ignores", unwanted)
		}
	}
}

func TestGetClientSecret_UsesPOST(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","confidential":true,"secret":"s3cr3t"}`))
	})

	got, err := c.GetClientSecret(context.Background(), "app")
	if err != nil {
		t.Fatalf("GetClientSecret: %v", err)
	}
	// Rauthy registers this read on POST, not GET.
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/clients/app/secret" {
		t.Errorf("got %s %s, want POST /auth/v1/clients/app/secret", gotMethod, gotPath)
	}
	if got.Secret == nil || *got.Secret != "s3cr3t" {
		t.Errorf("Secret = %v, want s3cr3t", got.Secret)
	}
}

func TestGetClientSecret_PublicClientHasNoSecret(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"spa","confidential":false}`))
	})

	got, err := c.GetClientSecret(context.Background(), "spa")
	if err != nil {
		t.Fatalf("GetClientSecret: %v", err)
	}
	if got.Secret != nil {
		t.Errorf("Secret = %v, want nil for a public client", *got.Secret)
	}
}

func TestGetClientSecret_403NamesTheMissingRight(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Forbidden","message":"Access denied"}`))
	})

	_, err := c.GetClientSecret(context.Background(), "app")
	if err == nil {
		t.Fatal("GetClientSecret succeeded on 403, want error")
	}
	if !strings.Contains(err.Error(), "Secrets:Read") {
		t.Errorf("error %q does not name the missing Secrets:Read right", err)
	}
	if !rauthy.IsForbidden(err) {
		t.Error("IsForbidden = false on a wrapped 403")
	}
}

func TestRotateClientSecret_SendsCacheCurrentHours(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","confidential":true,"secret":"new"}`))
	})

	hours := int64(6)
	if _, err := c.RotateClientSecret(context.Background(), "app", &hours); err != nil {
		t.Fatalf("RotateClientSecret: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotBody["cache_current_hours"] != float64(6) {
		t.Errorf("cache_current_hours = %v, want 6", gotBody["cache_current_hours"])
	}
}

func TestRotateClientSecret_OmitsCacheCurrentHoursWhenUnset(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","confidential":true,"secret":"new"}`))
	})

	if _, err := c.RotateClientSecret(context.Background(), "app", nil); err != nil {
		t.Fatalf("RotateClientSecret: %v", err)
	}
	if _, ok := gotBody["cache_current_hours"]; ok {
		t.Error("cache_current_hours present in body, want it omitted so Rauthy applies its own default")
	}
}

func TestNotFoundIsDetectable(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"NotFound","message":"Client 'gone' not found"}`))
	})

	_, err := c.GetClient(context.Background(), "gone")
	if !rauthy.IsNotFound(err) {
		t.Fatalf("IsNotFound = false for err %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not carry Rauthy's message", err)
	}
}

func TestIDIsPathEscaped(t *testing.T) {
	t.Parallel()

	// Rauthy client IDs may contain characters that are legal in the id regex
	// but not in a raw URL path, such as `?`, `#` and `%`.
	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteClient(context.Background(), "a#b?c%d"); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	if want := "/auth/v1/clients/a%23b%3Fc%25d"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

// The admin API never redirects. Following one would re-send the API key, and
// on a 307 or 308 the request body, to wherever the Location points, so the
// default client stops at the 3xx and reports it.
func TestNew_DoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(other.Close)

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	})

	_, err := c.UpdateRole(t.Context(), "r1", rauthy.RoleRequest{Role: "admin"})
	apiErr, ok := errors.AsType[*rauthy.APIError](err)
	if !ok || apiErr.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("err = %v, want an APIError carrying the 307", err)
	}
	if got := elsewhere.Load(); got != 0 {
		t.Errorf("redirect target received %d request(s), want none", got)
	}
}

// A refused redirect names where the server tried to send the call, as
// scheme, host and path only: userinfo and the query may carry credentials.
func TestRefusedRedirect_NamesTheLocation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		location string
		want     string
		relative bool
	}{
		"absolute": {
			location: "https://admin:hunter2@login.example.com/sso/start?token=abc#frag",
			want:     "https://login.example.com/sso/start",
		},
		"relative":    {location: "/moved/here?token=abc", want: "/moved/here", relative: true},
		"missing":     {location: "", want: "no Location"},
		"unparseable": {location: "http://[::1", want: "does not parse"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var srvURL string
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				srvURL = "http://" + r.Host
				if tc.location != "" {
					w.Header().Set("Location", tc.location)
				}
				w.WriteHeader(http.StatusFound)
				_, _ = io.WriteString(w, `<a href="`+tc.location+`">Found</a>`)
			})

			_, err := c.GetUser(t.Context(), "u1")
			want := tc.want
			if tc.relative {
				want = srvURL + want
			}
			checkRedirectError(t, err, want)
		})
	}
}

func checkRedirectError(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to contain %q", err, want)
	}
	for _, secret := range []string{"hunter2", "token=abc", "frag"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("err = %q leaks %q", err, secret)
		}
	}
}

// A caller-supplied client keeps its own redirect policy: New must not
// rewrite it behind the caller's back.
func TestWithHTTPClient_LeavesTheCallersClientAlone(t *testing.T) {
	t.Parallel()

	hc := &http.Client{}
	if _, err := rauthy.New("https://auth.example.com", "tf$secret", rauthy.WithHTTPClient(hc)); err != nil {
		t.Fatalf("New: %v", err)
	}
	if hc.CheckRedirect != nil {
		t.Error("New set CheckRedirect on the caller's client")
	}
}

func TestNew_RejectsNilHTTPClient(t *testing.T) {
	t.Parallel()

	if _, err := rauthy.New("https://auth.example.com", "tf$secret", rauthy.WithHTTPClient(nil)); err == nil {
		t.Error("New with a nil http.Client succeeded, want error")
	}
}

// A query or fragment would land in front of /auth/v1 and every request path.
func TestNew_RejectsQueryAndFragment(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://auth.example.com/?tenant=a",
		"https://auth.example.com?",
		"https://auth.example.com/#top",
	} {
		if _, err := rauthy.New(raw, "tf$secret"); err == nil {
			t.Errorf("New(%q) succeeded, want error", raw)
		}
	}
}

// A URL carrying userinfo must not have its password echoed into an error,
// which ends up in logs and Terraform diagnostics.
func TestNew_ErrorsDoNotEchoTheURLPassword(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"ftp://admin:hunter2@auth.example.com",
		"https://admin:hunter2@auth.example.com/?x=1",
		"https://admin:hunter2@auth.example.com/%zz",
	} {
		_, err := rauthy.New(raw, "tf$secret")
		if err == nil {
			t.Errorf("New(%q) succeeded, want error", raw)
			continue
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("New(%q) error %q leaks the password", raw, err)
		}
	}
}
