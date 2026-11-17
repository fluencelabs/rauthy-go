package rauthy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

// The request carries the attribute mappings as arrays; the response returns
// them comma-joined. Both halves of that asymmetry are pinned here.
func TestCreateScope_SendsArraysAndSplitsTheJoinedResponse(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"scope-1","name":"read:billing",` +
			`"attr_include_access":"department,cost_center","attr_include_id":"department"}`))
	})

	got, err := c.CreateScope(context.Background(), rauthy.ScopeRequest{
		Scope:             "read:billing",
		AttrIncludeAccess: []string{"department", "cost_center"},
		AttrIncludeID:     []string{"department"},
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/auth/v1/scopes" {
		t.Errorf("got %s %s, want POST /auth/v1/scopes", gotMethod, gotPath)
	}
	access, ok := gotBody["attr_include_access"].([]any)
	if !ok || len(access) != 2 {
		t.Errorf("attr_include_access = %#v, want a two-element array", gotBody["attr_include_access"])
	}

	if want := (rauthy.AttrList{"department", "cost_center"}); !reflect.DeepEqual(got.AttrIncludeAccess, want) {
		t.Errorf("AttrIncludeAccess = %v, want %v", got.AttrIncludeAccess, want)
	}
	if want := (rauthy.AttrList{"department"}); !reflect.DeepEqual(got.AttrIncludeID, want) {
		t.Errorf("AttrIncludeID = %v, want %v", got.AttrIncludeID, want)
	}
}

// Rauthy answers with two different shapes for the same field: POST /scopes
// returns the stored comma-joined string, while GET /scopes and PUT
// /scopes/{id} return an array. The vendored OpenAPI document describes only
// the string, so this is invisible to the contract tests — it was found by
// running the acceptance suite against a live instance.
func TestAttrList_DecodesBothShapes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		raw  string
		want rauthy.AttrList
	}{
		"null":                    {`null`, nil},
		"joined string":           {`"department,cost_center"`, rauthy.AttrList{"department", "cost_center"}},
		"empty string":            {`""`, nil},
		"string of separators":    {`" , "`, nil},
		"padded joined string":    {`" department , cost_center "`, rauthy.AttrList{"department", "cost_center"}},
		"array":                   {`["department","cost_center"]`, rauthy.AttrList{"department", "cost_center"}},
		"empty array":             {`[]`, nil},
		"array holding one empty": {`[""]`, nil},
	}

	for name, tc := range cases {
		var got rauthy.AttrList
		if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
			t.Errorf("%s: unmarshal %s: %v", name, tc.raw, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: unmarshal %s = %#v, want %#v", name, tc.raw, got, tc.want)
		}
	}
}

// "no attributes" comes back from Rauthy as an empty string or as an array
// holding one empty string; neither may become an attribute named "".
func TestAttrList_RejectsAnEmptyAttributeName(t *testing.T) {
	t.Parallel()

	var s rauthy.ScopeResponse
	body := `{"id":"scope-1","name":"probe","attr_include_access":"","attr_include_id":[""]}`
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.AttrIncludeAccess != nil || s.AttrIncludeID != nil {
		t.Errorf("got access=%#v id=%#v, want both nil", s.AttrIncludeAccess, s.AttrIncludeID)
	}
}

// Unset mappings must be omitted from the body rather than sent as null: the
// field is Option<Vec<String>> upstream, and an explicit null is not the same
// as absent for Rauthy's deserializer.
func TestScopeRequest_OmitsEmptyMappings(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"scope-1","name":"plain"}`))
	})

	got, err := c.CreateScope(context.Background(), rauthy.ScopeRequest{Scope: "plain"})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	for _, field := range []string{"attr_include_access", "attr_include_id"} {
		if _, present := gotBody[field]; present {
			t.Errorf("%s present in the body, want it omitted", field)
		}
	}
	if got.AttrIncludeAccess != nil {
		t.Errorf("AttrIncludeAccess = %v, want nil", got.AttrIncludeAccess)
	}
}

func TestGetScope_MissingScopeIsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/auth/v1/scopes" {
			t.Errorf("got %s %s, want GET /auth/v1/scopes", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"scope-1","name":"openid",` +
			`"attr_include_access":["email","groups"],"attr_include_id":["email"],"claims_at_root":true}]`))
	})

	got, err := c.GetScope(context.Background(), "scope-1")
	if err != nil {
		t.Fatalf("GetScope: %v", err)
	}
	want := rauthy.ScopeResponse{
		ID:                "scope-1",
		Name:              "openid",
		AttrIncludeAccess: rauthy.AttrList{"email", "groups"},
		AttrIncludeID:     rauthy.AttrList{"email"},
		ClaimsAtRoot:      true,
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("GetScope = %+v, want %+v", *got, want)
	}
	if _, err = c.GetScope(context.Background(), "scope-2"); !rauthy.IsNotFound(err) {
		t.Errorf("GetScope(missing) error = %v, want a 404", err)
	}
	if _, err = c.GetScopeByName(context.Background(), "nope"); !rauthy.IsNotFound(err) {
		t.Errorf("GetScopeByName(missing) error = %v, want a 404", err)
	}
}

func TestUpdateScope_PutsToScopePath(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"a/b","name":"renamed"}`))
	})

	if _, err := c.UpdateScope(context.Background(), "a/b", rauthy.ScopeRequest{Scope: "renamed"}); err != nil {
		t.Fatalf("UpdateScope: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/auth/v1/scopes/a%2Fb" {
		t.Errorf("got %s %s, want PUT with the id escaped", gotMethod, gotPath)
	}
}

// PUT /scopes/{id} replaces the scope, so a read-modify-write has to carry
// claims_at_root back or the update resets it to false.
func TestScopeResponse_UpdateRequestKeepsEverySetting(t *testing.T) {
	t.Parallel()

	var got rauthy.ScopeResponse
	if err := json.Unmarshal([]byte(`{"id":"scope-1","name":"read:billing",`+
		`"attr_include_access":["department","cost_center"],"attr_include_id":["department"],`+
		`"claims_at_root":true}`), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	req := got.UpdateRequest()
	want := rauthy.ScopeRequest{
		Scope:             "read:billing",
		AttrIncludeAccess: []string{"department", "cost_center"},
		AttrIncludeID:     []string{"department"},
		ClaimsAtRoot:      true,
	}
	if !reflect.DeepEqual(req, want) {
		t.Errorf("UpdateRequest() = %#v, want %#v", req, want)
	}
	req.AttrIncludeAccess[0] = "changed"
	if got.AttrIncludeAccess[0] != "department" {
		t.Error("UpdateRequest shares its slices with the response")
	}

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"scope-1","name":"read:billing","claims_at_root":true}`))
	})
	out, err := c.UpdateScope(context.Background(), "scope-1", got.UpdateRequest())
	if err != nil {
		t.Fatalf("UpdateScope: %v", err)
	}
	if gotBody["claims_at_root"] != true {
		t.Errorf("claims_at_root = %v, want true on the wire", gotBody["claims_at_root"])
	}
	if !out.ClaimsAtRoot {
		t.Error("ClaimsAtRoot = false, want the response's true")
	}
}
