package rauthy_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

// Each fixture sets exactly one boolean to true, so every pair of boolean
// fields differs in at least one fixture and a swapped wire tag cannot hide.
func TestClientResponseBooleanFields(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body string
		want rauthy.ClientResponse
	}{
		"enabled":        {`{"id":"example-app","enabled":true,"confidential":false,"force_mfa":false,"claims_at_root":false}`, rauthy.ClientResponse{ID: "example-app", Enabled: true}},
		"confidential":   {`{"id":"example-app","enabled":false,"confidential":true,"force_mfa":false,"claims_at_root":false}`, rauthy.ClientResponse{ID: "example-app", Confidential: true}},
		"force_mfa":      {`{"id":"example-app","enabled":false,"confidential":false,"force_mfa":true,"claims_at_root":false}`, rauthy.ClientResponse{ID: "example-app", ForceMFA: true}},
		"claims_at_root": {`{"id":"example-app","enabled":false,"confidential":false,"force_mfa":false,"claims_at_root":true}`, rauthy.ClientResponse{ID: "example-app", ClaimsAtRoot: true}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.ClientResponse, error) {
				return c.GetClient(ctx, "example-app")
			}, tc.want)(t, c)
		})
	}
}

func TestAuthProviderResponseBooleanFields(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body string
		want rauthy.AuthProviderResponse
	}{
		"enabled":             {`[{"id":"provider-1","name":"Example IdP","enabled":true,"use_pkce":false,"client_secret_basic":false,"client_secret_post":false,"auto_onboarding":false,"auto_link":false}]`, rauthy.AuthProviderResponse{ID: "provider-1", Name: "Example IdP", Enabled: true}},
		"use_pkce":            {`[{"id":"provider-1","name":"Example IdP","enabled":false,"use_pkce":true,"client_secret_basic":false,"client_secret_post":false,"auto_onboarding":false,"auto_link":false}]`, rauthy.AuthProviderResponse{ID: "provider-1", Name: "Example IdP", UsePKCE: true}},
		"client_secret_basic": {`[{"id":"provider-1","name":"Example IdP","enabled":false,"use_pkce":false,"client_secret_basic":true,"client_secret_post":false,"auto_onboarding":false,"auto_link":false}]`, rauthy.AuthProviderResponse{ID: "provider-1", Name: "Example IdP", ClientSecretBasic: true}},
		"client_secret_post":  {`[{"id":"provider-1","name":"Example IdP","enabled":false,"use_pkce":false,"client_secret_basic":false,"client_secret_post":true,"auto_onboarding":false,"auto_link":false}]`, rauthy.AuthProviderResponse{ID: "provider-1", Name: "Example IdP", ClientSecretPost: true}},
		"auto_onboarding":     {`[{"id":"provider-1","name":"Example IdP","enabled":false,"use_pkce":false,"client_secret_basic":false,"client_secret_post":false,"auto_onboarding":true,"auto_link":false}]`, rauthy.AuthProviderResponse{ID: "provider-1", Name: "Example IdP", AutoOnboarding: true}},
		"auto_link":           {`[{"id":"provider-1","name":"Example IdP","enabled":false,"use_pkce":false,"client_secret_basic":false,"client_secret_post":false,"auto_onboarding":false,"auto_link":true}]`, rauthy.AuthProviderResponse{ID: "provider-1", Name: "Example IdP", AutoLink: true}},
	} {
		for method, get := range map[string]func(context.Context, *rauthy.Client) (*rauthy.AuthProviderResponse, error){
			"by id": func(ctx context.Context, c *rauthy.Client) (*rauthy.AuthProviderResponse, error) {
				return c.GetAuthProvider(ctx, "provider-1")
			},
			"by name": func(ctx context.Context, c *rauthy.Client) (*rauthy.AuthProviderResponse, error) {
				return c.GetAuthProviderByName(ctx, "Example IdP")
			},
		} {
			t.Run(name+"/"+method, func(t *testing.T) {
				t.Parallel()
				c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.body))
				})
				lookup(get, tc.want)(t, c)
			})
		}
	}
}
