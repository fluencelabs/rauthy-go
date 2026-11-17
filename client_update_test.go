package rauthy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

// fullClientJSON is a GET /clients/{id} body with every property the spec
// declares on ClientResponse set to a non-zero value.
const fullClientJSON = `{
	"id": "example-app",
	"name": "Example App",
	"enabled": true,
	"confidential": true,
	"redirect_uris": ["https://app.example.com/callback"],
	"post_logout_redirect_uris": ["https://app.example.com/"],
	"allowed_origins": ["https://app.example.com"],
	"allowed_resources": ["https://api.example.com"],
	"default_aud": ["https://api.example.com/v1"],
	"flows_enabled": ["authorization_code", "refresh_token"],
	"access_token_alg": "EdDSA",
	"id_token_alg": "RS256",
	"auth_code_lifetime": 60,
	"access_token_lifetime": 600,
	"scopes": ["openid", "profile"],
	"default_scopes": ["openid"],
	"challenges": ["S256"],
	"force_mfa": true,
	"client_uri": "https://app.example.com",
	"contacts": ["ops@example.com"],
	"backchannel_logout_uri": "https://app.example.com/logout",
	"restrict_group_prefix": "app:",
	"scim": {
		"bearer_token": "scim-token",
		"base_uri": "https://app.example.com/scim/v2",
		"sync_groups": true,
		"group_sync_prefix": "app:"
	},
	"claims": {"tenant": "acme", "tier": 2},
	"claims_at_root": true
}`

func decodeFullClient(t *testing.T) (rauthy.ClientResponse, map[string]json.RawMessage) {
	t.Helper()

	var typed rauthy.ClientResponse
	if err := json.Unmarshal([]byte(fullClientJSON), &typed); err != nil {
		t.Fatalf("decode into ClientResponse: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(fullClientJSON), &raw); err != nil {
		t.Fatalf("decode into a map: %v", err)
	}
	return typed, raw
}

// The fixture is the whole response, so the round trip below covers every
// field: a property added to the spec has to be added here too.
func TestContract_FullClientResponseFixtureIsComplete(t *testing.T) {
	schemas := loadSpecSchemas(t)
	_, raw := decodeFullClient(t)

	for name := range schemas["ClientResponse"].Properties {
		if _, ok := raw[name]; !ok {
			t.Errorf("fullClientJSON lacks spec property %q", name)
		}
	}

	v := newContractValidator(t)
	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/clients/example-app"), http.StatusOK, fullClientJSON)
	if !ok {
		t.Errorf("GET /clients/{id} response rejected by the spec: %s", msg)
	}
}

func TestClientResponse_UpdateRequestSetsEveryField(t *testing.T) {
	t.Parallel()

	got, _ := decodeFullClient(t)
	req := reflect.ValueOf(got.UpdateRequest())
	for i := range req.NumField() {
		if req.Field(i).IsZero() {
			t.Errorf("UpdateRequest leaves %s unset although the response carries it", req.Type().Field(i).Name)
		}
	}
}

// GET, build the update from the response, PUT: the body on the wire carries
// every value the server returned, byte for byte as JSON.
func TestUpdateClient_RoundTripPreservesEveryField(t *testing.T) {
	t.Parallel()

	puts := make(chan []byte, 1)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/clients/example-app" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			t.Errorf("method = %s", r.Method)
		}
		if r.Method == http.MethodPut {
			b, _ := io.ReadAll(r.Body)
			puts <- b
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fullClientJSON)
	})

	ctx := context.Background()
	got, err := c.GetClient(ctx, "example-app")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if _, err = c.UpdateClient(ctx, got.ID, got.UpdateRequest()); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}

	var body map[string]json.RawMessage
	if err = json.Unmarshal(<-puts, &body); err != nil {
		t.Fatalf("decode PUT body: %v", err)
	}
	_, want := decodeFullClient(t)
	for name, w := range want {
		if !jsonEqual(t, body[name], w) {
			t.Errorf("PUT body %s = %s, want %s", name, body[name], w)
		}
	}
	for name := range body {
		if _, ok := want[name]; !ok {
			t.Errorf("PUT body carries %q, which the response did not", name)
		}
	}
}

func TestContract_UpdateRequestFromFullResponse(t *testing.T) {
	v := newContractValidator(t)
	got, _ := decodeFullClient(t)

	ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/clients/example-app"), got.UpdateRequest())
	if !ok {
		t.Errorf("PUT /clients/{id} body built by UpdateRequest rejected by the spec: %s", msg)
	}
}

func TestClientResponse_UpdateRequestSharesNoMemory(t *testing.T) {
	t.Parallel()

	got, _ := decodeFullClient(t)
	req := got.UpdateRequest()

	req.RedirectURIs[0] = "changed"
	req.AllowedResources[0] = "changed"
	req.Claims[0] = ' '
	*req.Name = "changed"
	req.Scim.BearerToken = rauthy.NewSecret("changed")
	*req.Scim.GroupSyncPrefix = "changed"

	again, _ := decodeFullClient(t)
	if !reflect.DeepEqual(got, again) {
		t.Errorf("mutating the update request changed the response it was built from:\n%+v", got)
	}
}

func TestClientResponse_UpdateRequestKeepsAbsentFieldsAbsent(t *testing.T) {
	t.Parallel()

	req := (&rauthy.ClientResponse{ID: "bare"}).UpdateRequest()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body map[string]json.RawMessage
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, name := range []string{"allowed_resources", "default_aud", "claims", "scim"} {
		if _, ok := body[name]; ok {
			t.Errorf("an unset %s is sent as %s; it should be left out", name, body[name])
		}
	}
	if !slices.Equal([]byte(body["claims_at_root"]), []byte("false")) {
		t.Errorf("claims_at_root = %s, want false", body["claims_at_root"])
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()

	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return reflect.DeepEqual(x, y)
}

// An empty optional list read from GET goes back as absent, not as []: the
// optional list fields carry omitempty. Required lists keep [].
func TestClientResponse_UpdateRequestSendsEmptyOptionalListsAsAbsent(t *testing.T) {
	t.Parallel()

	var got rauthy.ClientResponse
	err := json.Unmarshal([]byte(`{
		"id": "example-app", "enabled": true, "confidential": false,
		"redirect_uris": [], "flows_enabled": [], "scopes": [], "default_scopes": [],
		"post_logout_redirect_uris": [], "allowed_origins": [], "challenges": [],
		"contacts": [], "allowed_resources": [], "default_aud": [],
		"access_token_alg": "EdDSA", "id_token_alg": "EdDSA",
		"auth_code_lifetime": 60, "access_token_lifetime": 600,
		"force_mfa": false, "claims_at_root": false
	}`), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw, err := json.Marshal(got.UpdateRequest())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body map[string]json.RawMessage
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, name := range []string{
		"post_logout_redirect_uris", "allowed_origins", "challenges", "contacts", "allowed_resources", "default_aud",
	} {
		if v, ok := body[name]; ok {
			t.Errorf("empty %s is sent as %s; want it absent", name, v)
		}
	}
	for _, name := range []string{"redirect_uris", "flows_enabled", "scopes", "default_scopes"} {
		if string(body[name]) != "[]" {
			t.Errorf("required %s = %s, want []", name, body[name])
		}
	}
}
