package rauthy_test

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

// lookup compares every field against a literal independent of the wire tags.
func lookup[T any](get func(context.Context, *rauthy.Client) (*T, error), want T) func(*testing.T, *rauthy.Client) {
	return func(t *testing.T, c *rauthy.Client) {
		t.Helper()
		if reflect.ValueOf(want).IsZero() {
			t.Fatal("the wanted record is empty, so it cannot tell a found record from a zero one")
		}
		got, err := get(context.Background(), c)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if !reflect.DeepEqual(*got, want) {
			t.Errorf("lookup = %+v, want %+v", *got, want)
		}
	}
}

// Rauthy has no GET by id or name for these objects, so each lookup filters a
// list. The record asked for is the second of two that differ in every field:
// returning an empty record, or another one, fails.
func TestListLookupsReturnTheFoundRecord(t *testing.T) {
	t.Parallel()

	const (
		group1    = `{"id":"group-1","name":"admins","meta":{"a":1}}`
		group2    = `{"id":"group-2","name":"developers","meta":{"b":2}}`
		role1     = `{"id":"role-1","name":"admin","meta":{"a":1}}`
		role2     = `{"id":"role-2","name":"operator","meta":{"b":2}}`
		scope1    = `{"id":"scope-1","name":"openid","attr_include_access":null,"attr_include_id":null,"claims_at_root":false}`
		scope2    = `{"id":"scope-2","name":"billing","attr_include_access":["email","groups"],"attr_include_id":["email"],"claims_at_root":true}`
		key1      = `{"name":"first","created":1700000000,"access":[{"group":"Clients","access_rights":["read"]}]}`
		key2      = `{"name":"second","created":1700000500,"expires":4102444800,"access":[{"group":"Users","access_rights":["read","update"]}]}`
		ip1       = `{"ip":"203.0.113.1","exp":4102444000}`
		ip2       = `{"ip":"203.0.113.2","exp":4102444800}`
		attr1     = `{"name":"first","desc":"one","default_value":"x","typ":"Email","user_editable":false}`
		attr2     = `{"name":"second","desc":"two","default_value":42,"user_editable":true}`
		pamGroup1 = `{"id":100001,"name":"admins","typ":"host"}`
		pamGroup2 = `{"id":100002,"name":"developers","typ":"generic"}`
	)
	provider2 := strings.TrimSuffix(strings.TrimPrefix(oneProviderJSON, "["), "]")
	provider1 := `{"id":"provider-0","name":"Other IdP","typ":"github","enabled":false,` +
		`"issuer":"https://other.example.com","authorization_endpoint":"https://other.example.com/a",` +
		`"token_endpoint":"https://other.example.com/t","userinfo_endpoint":"https://other.example.com/u",` +
		`"client_id":"other","client_secret":"other-secret","scope":"openid",` +
		`"use_pkce":false,"client_secret_basic":false,"client_secret_post":true,` +
		`"auto_onboarding":true,"auto_link":true}`
	pair := func(a, b string) string { return "[" + a + "," + b + "]" }

	cases := map[string]struct {
		path  string
		list  string
		check func(*testing.T, *rauthy.Client)
	}{
		"GetGroup": {"/auth/v1/groups", pair(group1, group2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.GroupResponse, error) {
				return c.GetGroup(ctx, "group-2")
			}, rauthy.GroupResponse{ID: "group-2", Name: "developers", Meta: []byte(`{"b":2}`)})},
		"GetGroupByName": {"/auth/v1/groups", pair(group1, group2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.GroupResponse, error) {
				return c.GetGroupByName(ctx, "developers")
			}, rauthy.GroupResponse{ID: "group-2", Name: "developers", Meta: []byte(`{"b":2}`)})},
		"GetRole": {"/auth/v1/roles", pair(role1, role2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.RoleResponse, error) {
				return c.GetRole(ctx, "role-2")
			}, rauthy.RoleResponse{ID: "role-2", Name: "operator", Meta: []byte(`{"b":2}`)})},
		"GetRoleByName": {"/auth/v1/roles", pair(role1, role2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.RoleResponse, error) {
				return c.GetRoleByName(ctx, "operator")
			}, rauthy.RoleResponse{ID: "role-2", Name: "operator", Meta: []byte(`{"b":2}`)})},
		"GetScope": {"/auth/v1/scopes", pair(scope1, scope2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.ScopeResponse, error) {
				return c.GetScope(ctx, "scope-2")
			}, rauthy.ScopeResponse{ID: "scope-2", Name: "billing", AttrIncludeAccess: rauthy.AttrList{"email", "groups"}, AttrIncludeID: rauthy.AttrList{"email"}, ClaimsAtRoot: true})},
		"GetScopeByName": {"/auth/v1/scopes", pair(scope1, scope2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.ScopeResponse, error) {
				return c.GetScopeByName(ctx, "billing")
			}, rauthy.ScopeResponse{ID: "scope-2", Name: "billing", AttrIncludeAccess: rauthy.AttrList{"email", "groups"}, AttrIncludeID: rauthy.AttrList{"email"}, ClaimsAtRoot: true})},
		"GetAuthProvider": {"/auth/v1/providers", pair(provider1, provider2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.AuthProviderResponse, error) {
				return c.GetAuthProvider(ctx, "provider-1")
			}, oneProviderResponse())},
		"GetAuthProviderByName": {"/auth/v1/providers", pair(provider1, provider2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.AuthProviderResponse, error) {
				return c.GetAuthProviderByName(ctx, "Example IdP")
			}, oneProviderResponse())},
		"GetAPIKey": {"/auth/v1/api_keys", `{"keys":` + pair(key1, key2) + `}`,
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.APIKeyResponse, error) {
				return c.GetAPIKey(ctx, "second")
			}, rauthy.APIKeyResponse{Name: "second", Created: 1700000500, Expires: new(int64(4102444800)), Access: []rauthy.APIKeyAccess{{Group: "Users", AccessRights: []string{"read", "update"}}}})},
		"GetBlacklistedIP": {"/auth/v1/blacklist", `{"ips":` + pair(ip1, ip2) + `}`,
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.BlacklistedIP, error) {
				return c.GetBlacklistedIP(ctx, "203.0.113.2")
			}, rauthy.BlacklistedIP{IP: "203.0.113.2", Exp: 4102444800})},
		"GetUserAttr": {"/auth/v1/users/attr", `{"values":` + pair(attr1, attr2) + `}`,
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.UserAttrResponse, error) {
				return c.GetUserAttr(ctx, "second")
			}, rauthy.UserAttrResponse{Name: "second", Desc: new("two"), DefaultValue: []byte(`42`), UserEditable: true})},
		"GetPamGroup": {"/auth/v1/pam/groups", pair(pamGroup1, pamGroup2),
			lookup(func(ctx context.Context, c *rauthy.Client) (*rauthy.PamGroupResponse, error) {
				return c.GetPamGroup(ctx, 100002)
			}, rauthy.PamGroupResponse{ID: 100002, Name: "developers", Typ: "generic"})},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("got %s %s, want the list at %s", r.Method, r.URL.Path, tc.path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.list))
			})
			tc.check(t, c)
		})
	}
}

func TestGetGroupByNamePositionsAndAbsence(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		list  string
		found bool
	}{
		"first":                    {`[{"id":"wanted","name":"developers"},{"id":"other","name":"admins"}]`, true},
		"last":                     {`[{"id":"other","name":"admins"},{"id":"wanted","name":"developers"}]`, true},
		"missing in nonempty list": {`[{"id":"other","name":"admins"}]`, false},
		"empty list":               {`[]`, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.list))
			})
			got, err := c.GetGroupByName(t.Context(), "developers")
			if !tc.found {
				if got != nil || !rauthy.IsNotFound(err) {
					t.Fatalf("lookup = %+v, %v, want nil and IsNotFound", got, err)
				}
				return
			}
			if err != nil || got == nil {
				t.Fatalf("lookup = %+v, %v", got, err)
			}
			if got.ID != "wanted" || got.Name != "developers" {
				t.Errorf("lookup = %+v, want the matching group", got)
			}
		})
	}
}
