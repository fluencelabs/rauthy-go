package rauthy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

// Full response fixtures: every property the spec declares on the response
// schema, each set to a non-zero value, so that a round trip through
// UpdateRequest covers every field.
const (
	fullRoleJSON  = `{"id":"role-1","name":"admin","meta":{"owner":"platform","tier":2,"tags":["a","b"]}}`
	fullGroupJSON = `{"id":"group-1","name":"developers","meta":{"owner":"platform","tier":2,"tags":["a","b"]}}`
	fullAttrJSON  = `{"name":"department","desc":"cost-center","default_value":{"unit":"eng","level":[1,2]},` +
		`"typ":"Email","user_editable":true}`
)

// updateCase is one response type with an UpdateRequest: the spec schemas on
// both sides, its fixture, the request it builds from the fixture, and the
// response property each request property is read from.
type updateCase struct {
	name, respSchema, reqSchema string
	fixture                     string
	getPath, putPath            string
	build                       func(t *testing.T, raw []byte) any
	put                         func(ctx context.Context, c *rauthy.Client, req any) error
	get                         func(ctx context.Context, c *rauthy.Client) (any, error)
	// renamed maps a request property to the response property it carries
	// when the two names differ.
	renamed map[string]string
}

func updateCases() []updateCase {
	return []updateCase{
		{
			name: "role", respSchema: "RoleResponse", reqSchema: "RoleRequest", fixture: fullRoleJSON,
			getPath: "/auth/v1/roles", putPath: "/auth/v1/roles/role-1",
			build: func(t *testing.T, raw []byte) any {
				t.Helper()
				return decodeAs[rauthy.RoleResponse](t, raw).UpdateRequest()
			},
			get: func(ctx context.Context, c *rauthy.Client) (any, error) { return c.GetRole(ctx, "role-1") },
			put: func(ctx context.Context, c *rauthy.Client, req any) error {
				r, ok := req.(*rauthy.RoleResponse)
				if !ok {
					return errUnexpected(req)
				}
				_, err := c.UpdateRole(ctx, r.ID, r.UpdateRequest())
				return err
			},
			renamed: map[string]string{"role": "name"},
		},
		{
			name: "group", respSchema: "GroupResponse", reqSchema: "GroupRequest", fixture: fullGroupJSON,
			getPath: "/auth/v1/groups", putPath: "/auth/v1/groups/group-1",
			build: func(t *testing.T, raw []byte) any {
				t.Helper()
				return decodeAs[rauthy.GroupResponse](t, raw).UpdateRequest()
			},
			get: func(ctx context.Context, c *rauthy.Client) (any, error) { return c.GetGroup(ctx, "group-1") },
			put: func(ctx context.Context, c *rauthy.Client, req any) error {
				g, ok := req.(*rauthy.GroupResponse)
				if !ok {
					return errUnexpected(req)
				}
				_, err := c.UpdateGroup(ctx, g.ID, g.UpdateRequest())
				return err
			},
			renamed: map[string]string{"group": "name"},
		},
		{
			name: "user attribute", respSchema: "UserAttrConfigValueResponse", reqSchema: "UserAttrConfigRequest",
			fixture: fullAttrJSON, getPath: "/auth/v1/users/attr", putPath: "/auth/v1/users/attr/department",
			build: func(t *testing.T, raw []byte) any {
				t.Helper()
				return decodeAs[rauthy.UserAttrResponse](t, raw).UpdateRequest()
			},
			get: func(ctx context.Context, c *rauthy.Client) (any, error) {
				return c.GetUserAttr(ctx, "department")
			},
			put: func(ctx context.Context, c *rauthy.Client, req any) error {
				a, ok := req.(*rauthy.UserAttrResponse)
				if !ok {
					return errUnexpected(req)
				}
				return c.UpdateUserAttr(ctx, a.Name, a.UpdateRequest())
			},
		},
	}
}

func decodeAs[T any](t *testing.T, raw []byte) *T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode into %T: %v", out, err)
	}
	return &out
}

func errUnexpected(got any) error { return fmt.Errorf("unexpected %T", got) }

// listBody wraps a fixture the way the list endpoint answers: GET /users/attr
// in an object under `values`, the others as a bare array.
func listBody(tc updateCase) string {
	if tc.getPath == "/auth/v1/users/attr" {
		return `{"values":[` + tc.fixture + `]}`
	}
	return `[` + tc.fixture + `]`
}

// The fixtures are the whole response and are what the spec accepts, so the
// round trips below cover every field: a property added to the spec has to be
// added to its fixture too.
func TestContract_FullUpdateFixturesAreComplete(t *testing.T) {
	schemas := loadSpecSchemas(t)
	v := newContractValidator(t)

	for _, tc := range updateCases() {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(tc.fixture), &raw); err != nil {
			t.Fatalf("%s: decode fixture: %v", tc.name, err)
		}
		for name := range schemas[tc.respSchema].Properties {
			if _, ok := raw[name]; !ok {
				t.Errorf("%s: fixture lacks spec property %q", tc.name, name)
			}
		}
		ok, msg := validateResponse(t, v, http.MethodGet, tc.getPath, http.StatusOK, listBody(tc))
		if !ok {
			t.Errorf("%s: GET %s response rejected by the spec: %s", tc.name, tc.getPath, msg)
		}

		ok, msg = validateRequest(t, v, http.MethodPut, tc.putPath, tc.build(t, []byte(tc.fixture)))
		if !ok {
			t.Errorf("%s: UpdateRequest body rejected by the spec: %s", tc.name, msg)
		}
	}
}

func TestUpdateRequest_SetsEveryField(t *testing.T) {
	t.Parallel()

	for _, tc := range updateCases() {
		req := reflect.ValueOf(tc.build(t, []byte(tc.fixture)))
		for i := range req.NumField() {
			if req.Field(i).IsZero() {
				t.Errorf("%s: UpdateRequest leaves %s unset although the response carries it",
					tc.name, req.Type().Field(i).Name)
			}
		}
	}
}

// GET, build the update from the response, PUT: the body on the wire carries
// every value the server returned, compared as JSON, and nothing else.
func TestUpdateRequest_RoundTripPreservesEveryField(t *testing.T) {
	t.Parallel()

	for _, tc := range updateCases() {
		puts := make(chan []byte, 1)
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPut {
				if r.URL.Path != tc.putPath {
					t.Errorf("%s: PUT %s, want %s", tc.name, r.URL.Path, tc.putPath)
				}
				b, _ := io.ReadAll(r.Body)
				puts <- b
				_, _ = io.WriteString(w, tc.fixture)
				return
			}
			_, _ = io.WriteString(w, listBody(tc))
		})

		ctx := context.Background()
		got, err := tc.get(ctx, c)
		if err != nil {
			t.Fatalf("%s: read: %v", tc.name, err)
		}
		if err = tc.put(ctx, c, got); err != nil {
			t.Fatalf("%s: update: %v", tc.name, err)
		}

		var sent, read map[string]any
		if err = json.Unmarshal(<-puts, &sent); err != nil {
			t.Fatalf("%s: decode PUT body: %v", tc.name, err)
		}
		if err = json.Unmarshal([]byte(tc.fixture), &read); err != nil {
			t.Fatalf("%s: decode fixture: %v", tc.name, err)
		}
		checkCarried(t, tc, sent, read)
	}
}

// checkCarried compares a PUT body with the response it was built from: every
// request property of the spec is sent with the value read, and nothing the
// spec does not declare is sent.
func checkCarried(t *testing.T, tc updateCase, sent, read map[string]any) {
	t.Helper()

	reqProps := loadSpecSchemas(t)[tc.reqSchema].Properties
	for name := range reqProps {
		from := name
		if r, ok := tc.renamed[name]; ok {
			from = r
		}
		want, ok := read[from]
		if !ok {
			t.Errorf("%s: request property %q has no response counterpart %q", tc.name, name, from)
			continue
		}
		if got, sentIt := sent[name]; !sentIt || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: PUT sent %q = %#v (present %v), want %#v", tc.name, name, got, sentIt, want)
		}
	}
	for name := range sent {
		if _, ok := reqProps[name]; !ok {
			t.Errorf("%s: PUT sent %q, which the request schema does not declare", tc.name, name)
		}
	}
}

// The request shares no memory with the response it was built from: editing
// the meta of one must not edit the other. A field UpdateRequest drops is
// reported by the tests above, so it is skipped here.
func TestUpdateRequest_SharesNoMemory(t *testing.T) {
	t.Parallel()

	role := decodeAs[rauthy.RoleResponse](t, []byte(fullRoleJSON))
	checkRawUnshared(t, "RoleResponse.Meta", role.Meta, role.UpdateRequest().Meta)

	group := decodeAs[rauthy.GroupResponse](t, []byte(fullGroupJSON))
	checkRawUnshared(t, "GroupResponse.Meta", group.Meta, group.UpdateRequest().Meta)

	attr := decodeAs[rauthy.UserAttrResponse](t, []byte(fullAttrJSON))
	attrReq := attr.UpdateRequest()
	checkRawUnshared(t, "UserAttrResponse.DefaultValue", attr.DefaultValue, attrReq.DefaultValue)
	if attrReq.Desc != nil && attrReq.Desc == attr.Desc {
		t.Error("UserAttrResponse.UpdateRequest shares Desc with the response")
	}
	if attrReq.Typ != nil && attrReq.Typ == attr.Typ {
		t.Error("UserAttrResponse.UpdateRequest shares Typ with the response")
	}
}

func checkRawUnshared(t *testing.T, field string, resp, req json.RawMessage) {
	t.Helper()
	if len(req) == 0 || len(resp) == 0 {
		return
	}
	req[0] = '#'
	if resp[0] == '#' {
		t.Errorf("UpdateRequest shares %s with the response", field)
	}
}

// What the server returns as unset goes back as unset, and a false
// user_editable is sent as false rather than dropped.
func TestUpdateRequest_KeepsUnsetAndFalseValues(t *testing.T) {
	t.Parallel()

	role := decodeAs[rauthy.RoleResponse](t, []byte(`{"id":"role-1","name":"admin"}`))
	assertJSON(t, role.UpdateRequest(), `{"role":"admin"}`)

	nullMeta := decodeAs[rauthy.GroupResponse](t, []byte(`{"id":"group-1","name":"developers","meta":null}`))
	assertJSON(t, nullMeta.UpdateRequest(), `{"group":"developers","meta":null}`)

	attr := decodeAs[rauthy.UserAttrResponse](t, []byte(`{"name":"department","typ":null,"user_editable":false}`))
	assertJSON(t, attr.UpdateRequest(), `{"name":"department","user_editable":false}`)
}

func assertJSON(t *testing.T, v any, want string) {
	t.Helper()
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	if string(got) != want {
		t.Errorf("%T marshals to %s, want %s", v, got, want)
	}
}
