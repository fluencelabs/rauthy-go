package rauthy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

func TestWritePayloadsPreserveEveryField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, method, path, want string
		call                     func(*rauthy.Client) error
	}{
		{
			name: "provider", method: http.MethodPut, path: "/providers/provider-1",
			want: `{"name":"Example IdP","typ":"oidc","enabled":true,"issuer":"https://idp.example.com","authorization_endpoint":"https://idp.example.com/authorize","token_endpoint":"https://idp.example.com/token","userinfo_endpoint":"https://idp.example.com/userinfo","jwks_endpoint":"https://idp.example.com/jwks","client_id":"upstream-client","client_secret":"synthetic-provider-secret","scope":"openid email","admin_claim_path":"$.roles.*","admin_claim_value":"administrator","mfa_claim_path":"$.amr.*","mfa_claim_value":"mfa","use_pkce":false,"client_secret_basic":true,"client_secret_post":false,"auto_onboarding":true,"auto_link":false}`,
			call: func(c *rauthy.Client) error {
				return c.UpdateAuthProvider(context.Background(), "provider-1", rauthy.AuthProviderRequest{
					Name: "Example IdP", Typ: "oidc", Enabled: true, Issuer: "https://idp.example.com",
					AuthorizationEndpoint: "https://idp.example.com/authorize", TokenEndpoint: "https://idp.example.com/token",
					UserinfoEndpoint: "https://idp.example.com/userinfo", JwksEndpoint: new("https://idp.example.com/jwks"),
					ClientID: "upstream-client", ClientSecret: new(rauthy.NewSecret("synthetic-provider-secret")), Scope: "openid email",
					AdminClaimPath: new("$.roles.*"), AdminClaimValue: new("administrator"), MfaClaimPath: new("$.amr.*"), MfaClaimValue: new("mfa"),
					UsePKCE: false, ClientSecretBasic: true, ClientSecretPost: false, AutoOnboarding: true, AutoLink: false,
				})
			},
		},
		{
			name: "api key", method: http.MethodPut, path: "/api_keys/deploy",
			want: `{"name":"deploy","exp":1900000000,"access":[{"group":"Users","access_rights":["read","update"]},{"group":"Clients","access_rights":["create"]}]}`,
			call: func(c *rauthy.Client) error {
				return c.UpdateAPIKey(context.Background(), "deploy", rauthy.APIKeyRequest{
					Name: "deploy", Exp: new(int64(1900000000)), Access: []rauthy.APIKeyAccess{
						{Group: "Users", AccessRights: []string{"read", "update"}}, {Group: "Clients", AccessRights: []string{"create"}},
					},
				})
			},
		},
		{
			name: "user", method: http.MethodPut, path: "/users/u-1",
			want: `{"email":"ada@example.com","enabled":true,"email_verified":false,"roles":["reader"],"groups":["engineering"],"given_name":"Ada","family_name":"Lovelace","language":"en","password":"synthetic-user-password","user_expires":1900000123,"user_values":{"birthdate":"1815-12-10","city":"London","country":"GB","phone":"+441234567890","street":"Example Street","zip":"12345","tz":"Europe/London"}}`,
			call: func(c *rauthy.Client) error {
				_, err := c.UpdateUser(context.Background(), "u-1", rauthy.UpdateUserRequest{
					Email: "ada@example.com", Enabled: true, EmailVerified: false, Roles: []string{"reader"}, Groups: []string{"engineering"},
					GivenName: new("Ada"), FamilyName: new("Lovelace"), Language: new("en"), Password: new(rauthy.NewSecret("synthetic-user-password")), UserExpires: new(int64(1900000123)),
					UserValues: &rauthy.UserValues{Birthdate: new("1815-12-10"), City: new("London"), Country: new("GB"), Phone: new("+441234567890"), Street: new("Example Street"), Zip: new("12345"), TZ: new("Europe/London")},
				})
				return err
			},
		},
		{
			name: "pam user", method: http.MethodPut, path: "/pam/users/100000",
			want: `{"shell":"/bin/bash","home_dir":"/home/ada","groups":[{"uid":100000,"gid":100001,"wheel":true},{"uid":100000,"gid":100002,"wheel":false}]}`,
			call: func(c *rauthy.Client) error {
				return c.UpdatePamUser(context.Background(), 100000, rauthy.PamUserUpdateRequest{
					Shell: "/bin/bash", HomeDir: new("/home/ada"), Groups: []rauthy.PamGroupUserLink{{UID: 100000, GID: 100001, Wheel: true}, {UID: 100000, GID: 100002, Wheel: false}},
				})
			},
		},
		{
			name: "pam host", method: http.MethodPost, path: "/pam/hosts",
			want: `{"hostname":"build01","gid":100001,"force_mfa":true,"local_password_only":false}`,
			call: func(c *rauthy.Client) error {
				_, err := c.CreatePamHost(context.Background(), rauthy.PamHostCreateRequest{Hostname: "build01", GID: 100001, ForceMfa: true, LocalPasswordOnly: false})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != tc.method || r.URL.Path != "/auth/v1"+tc.path {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				if !jsonEqual(t, body, []byte(tc.want)) {
					t.Errorf("body = %s, want %s", body, tc.want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			})
			if err := tc.call(c); err != nil {
				t.Fatalf("write: %v", err)
			}
			if calls != 1 {
				t.Errorf("calls = %d, want 1", calls)
			}
		})
	}
}

// Every boolean is true on its own, so copying a neighboring flag fails.
func TestUpdateClient_RoundTripIndependentBooleans(t *testing.T) {
	t.Parallel()
	fields := []string{"enabled", "confidential", "force_mfa", "claims_at_root"}
	for _, enabled := range fields {
		t.Run(enabled, func(t *testing.T) {
			t.Parallel()
			var fixture map[string]any
			if err := json.Unmarshal([]byte(fullClientJSON), &fixture); err != nil {
				t.Fatal(err)
			}
			for _, field := range fields {
				fixture[field] = field == enabled
			}
			want, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			assertClientRoundTrip(t, want)
		})
	}
}

func assertClientRoundTrip(t *testing.T, want []byte) {
	t.Helper()
	puts := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/clients/example-app" {
			t.Errorf("path = %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			puts++
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Errorf("read body: %v", readErr)
			}
			if !jsonEqual(t, body, want) {
				t.Errorf("body = %s, want %s", body, want)
			}
		case http.MethodGet:
		default:
			t.Errorf("method = %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(want)
	})
	got, err := c.GetClient(context.Background(), "example-app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.UpdateClient(context.Background(), got.ID, got.UpdateRequest()); err != nil {
		t.Fatal(err)
	}
	if puts != 1 {
		t.Errorf("PUT calls = %d, want 1", puts)
	}
}
