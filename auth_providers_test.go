package rauthy_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

const oneProviderJSON = `[{"id":"provider-1","name":"Example IdP","typ":"oidc","enabled":true,` +
	`"issuer":"https://idp.example.com",` +
	`"authorization_endpoint":"https://idp.example.com/authorize",` +
	`"token_endpoint":"https://idp.example.com/token",` +
	`"userinfo_endpoint":"https://idp.example.com/userinfo","jwks_endpoint":null,` +
	`"client_id":"rauthy","client_secret":"upstream-secret","scope":"openid+profile",` +
	`"admin_claim_path":null,"admin_claim_value":null,"mfa_claim_path":null,"mfa_claim_value":null,` +
	`"use_pkce":true,"client_secret_basic":true,"client_secret_post":false,` +
	`"auto_onboarding":false,"auto_link":false}]`

// The two verbs are easy to transpose, and transposing them is not a compile
// error: creation is POST /providers/create, and POST /providers is the list.
func TestCreateAuthProvider_PostsToProvidersCreate(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.TrimSuffix(strings.TrimPrefix(oneProviderJSON, "["), "]")))
	})

	secret := "upstream-secret"
	got, err := c.CreateAuthProvider(context.Background(), rauthy.AuthProviderRequest{
		Name: "Example IdP", Typ: "oidc", Enabled: true,
		Issuer:                "https://idp.example.com",
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		TokenEndpoint:         "https://idp.example.com/token",
		UserinfoEndpoint:      "https://idp.example.com/userinfo",
		ClientID:              "rauthy",
		ClientSecret:          new(rauthy.NewSecret(secret)),
		Scope:                 "openid profile",
	})
	if err != nil {
		t.Fatalf("CreateAuthProvider: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/providers/create" {
		t.Errorf("got %s %s, want POST /auth/v1/providers/create", gotMethod, gotPath)
	}
	if gotBody["scope"] != "openid profile" {
		t.Errorf("scope = %v, want the space-separated form", gotBody["scope"])
	}
	if got.ID != "provider-1" {
		t.Errorf("got %+v", got)
	}
}

// The listing is a POST with no body, which is unusual enough to pin.
func TestListAuthProviders_PostsWithNoBody(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody []byte

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(oneProviderJSON))
	})

	got, err := c.ListAuthProviders(context.Background())
	if err != nil {
		t.Fatalf("ListAuthProviders: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/providers" {
		t.Errorf("got %s %s, want POST /auth/v1/providers", gotMethod, gotPath)
	}
	if len(gotBody) != 0 {
		t.Errorf("body = %q, want empty", gotBody)
	}
	if len(got) != 1 || got[0].Name != "Example IdP" {
		t.Errorf("got %+v", got)
	}
}

// There is no GET /providers/{id}, so a provider that is not in the list has to
// look like a 404 to the resource layer, which relies on IsNotFound to drop the
// resource from state.
func TestGetAuthProvider_MissingIsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(oneProviderJSON))
	})

	got, err := c.GetAuthProvider(context.Background(), "provider-1")
	if err != nil {
		t.Fatalf("GetAuthProvider: %v", err)
	}
	for _, f := range []struct {
		field     string
		got, want any
	}{
		{"id", got.ID, "provider-1"},
		{"name", got.Name, "Example IdP"},
		{"typ", got.Typ, "oidc"},
		{"enabled", got.Enabled, true},
		{"issuer", got.Issuer, "https://idp.example.com"},
		{"token_endpoint", got.TokenEndpoint, "https://idp.example.com/token"},
		{"client_id", got.ClientID, "rauthy"},
		{"scope", got.Scope, "openid+profile"},
		{"use_pkce", got.UsePKCE, true},
		{"client_secret_basic", got.ClientSecretBasic, true},
	} {
		if f.got != f.want {
			t.Errorf("%s = %v, want %v", f.field, f.got, f.want)
		}
	}
	if got.ClientSecret == nil || got.ClientSecret.Reveal() != "upstream-secret" {
		t.Error("client_secret is not the listed provider's secret")
	}
	_, err = c.GetAuthProvider(context.Background(), "provider-nope")
	if !rauthy.IsNotFound(err) {
		t.Errorf("err = %v, want a synthetic 404", err)
	}
}

// Rauthy does not enforce unique provider names, so the by-name lookup has to
// refuse an ambiguous one rather than pick.
func TestGetAuthProviderByName_AmbiguousNameIsRefused(t *testing.T) {
	t.Parallel()

	twins := `[` + strings.TrimSuffix(strings.TrimPrefix(oneProviderJSON, "["), "]") + `,` +
		strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(oneProviderJSON, "["), "]"),
			`"id":"provider-1"`, `"id":"provider-2"`) + `]`

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twins))
	})

	_, err := c.GetAuthProviderByName(context.Background(), "Example IdP")
	if err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Errorf("err = %v, want a complaint about the duplicate name", err)
	}
	// A caller that drops a resource on IsNotFound must not take a duplicate
	// for an absence.
	if rauthy.IsNotFound(err) {
		t.Errorf("IsNotFound(%v) = true, want an ambiguous name to be told apart from a missing one", err)
	}
	if !errors.Is(err, rauthy.ErrAmbiguousName) {
		t.Errorf("err = %v, want it to match ErrAmbiguousName", err)
	}
}

// A PUT answers with an empty body, so UpdateAuthProvider must not try to
// decode one.
func TestUpdateAuthProvider_ToleratesEmptyBody(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode provider: %v", err)
		}
		if body["name"] != "Example IdP" || body["typ"] != "oidc" {
			t.Errorf("body = %v", body)
		}
		w.WriteHeader(http.StatusOK)
	})

	if err := c.UpdateAuthProvider(context.Background(), "provider-1", rauthy.AuthProviderRequest{
		Name: "Example IdP", Typ: "oidc",
	}); err != nil {
		t.Fatalf("UpdateAuthProvider: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/auth/v1/providers/provider-1" {
		t.Errorf("got %s %s, want PUT /auth/v1/providers/provider-1", gotMethod, gotPath)
	}
}

// A 403 on these endpoints almost always means the API key lacks the
// AuthProviders group, which only exists from Rauthy 0.36 — worth saying.
func TestAuthProvider_ForbiddenNamesTheAccessGroup(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Forbidden","message":"Access denied"}`))
	})

	_, err := c.ListAuthProviders(context.Background())
	if err == nil || !strings.Contains(err.Error(), "AuthProviders:Read") {
		t.Errorf("err = %v, want it to name the missing right", err)
	}
}
