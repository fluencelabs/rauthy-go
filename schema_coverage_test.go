package rauthy_test

import (
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/rauthy-go/internal/mock"
)

// The validator-based contract tests check that a body this package builds is
// acceptable to the spec. They cannot notice a property the Go type never
// sends or never reads: the spec's objects are open (no additionalProperties:
// false) and an optional property may always be left out. For a PUT that
// replaces the resource wholesale, such a gap silently resets the value on
// every read-modify-write. These tests compare the field sets themselves.

// specSchema is the part of an OpenAPI component schema the coverage check
// reads.
type specSchema struct {
	Type       any                     `json:"type"`
	Properties map[string]specProperty `json:"properties"`
	Required   []string                `json:"required"`
}

type specProperty struct {
	Type  any               `json:"type"`
	Ref   string            `json:"$ref"`
	OneOf []json.RawMessage `json:"oneOf"`
}

// nullable reports whether the property admits JSON null.
func (p specProperty) nullable() bool {
	switch t := p.Type.(type) {
	case []any:
		return slices.Contains(t, any("null"))
	case nil:
		return len(p.OneOf) > 0 && slices.ContainsFunc(p.OneOf, func(raw json.RawMessage) bool {
			return strings.Contains(string(raw), `"null"`)
		})
	}
	return false
}

// jsonType returns the property's JSON type other than null, following a
// $ref or a nullable oneOf to the schema it names. It is "" for a property
// that admits any JSON value.
func (p specProperty) jsonType(schemas map[string]specSchema) string {
	if t := nonNull(p.Type); t != "" {
		return t
	}
	refs := []string{p.Ref}
	for _, raw := range p.OneOf {
		var alt specProperty
		if json.Unmarshal(raw, &alt) == nil {
			refs = append(refs, alt.Ref)
		}
	}
	for _, ref := range refs {
		if name, ok := strings.CutPrefix(ref, "#/components/schemas/"); ok {
			return nonNull(schemas[name].Type)
		}
	}
	return ""
}

func nonNull(typ any) string {
	switch t := typ.(type) {
	case string:
		return t
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok && s != "null" {
				return s
			}
		}
	}
	return ""
}

// kindMatches reports whether a Go type can carry a value of the JSON type.
func kindMatches(jsonType string, t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	isRaw := t == reflect.TypeFor[json.RawMessage]()
	switch jsonType {
	case "":
		return isRaw || t.Kind() == reflect.Interface
	case "string":
		return t.Kind() == reflect.String || t == reflect.TypeFor[rauthy.Secret]()
	case "boolean":
		return t.Kind() == reflect.Bool
	case "integer":
		return t.Kind() >= reflect.Int && t.Kind() <= reflect.Uint64
	case "number":
		return t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64
	case "array":
		return t.Kind() == reflect.Slice && !isRaw
	case "object":
		return t.Kind() == reflect.Struct || t.Kind() == reflect.Map || isRaw
	}
	return false
}

// coveredType pairs a Go wire type with the component schema it mirrors.
// goOnly names JSON fields the Go type deliberately sends although the schema
// does not declare them; liveShape names fields typed after what a live server
// sends rather than after the spec (the divergences documented in pam.go);
// missing names spec properties the Go type does not model yet (knownGaps).
type coveredType struct {
	value     any
	schema    string
	request   bool
	goOnly    []string
	liveShape []string
	missing   []string
}

// coveredTypes lists the wire types whose field sets match the spec exactly.
func coveredTypes() []coveredType {
	return []coveredType{
		{value: rauthy.NewClientRequest{}, schema: "NewClientRequest", request: true},
		// id is not in the schema: Rauthy takes the id from the path. The
		// package has always sent it and keeps doing so.
		{value: rauthy.UpdateClientRequest{}, schema: "UpdateClientRequest", request: true, goOnly: []string{"id"}},
		{value: rauthy.ClientResponse{}, schema: "ClientResponse"},
		{value: rauthy.ScimClientRequestResponse{}, schema: "ScimClientRequestResponse", request: true},
		{value: rauthy.ScopeRequest{}, schema: "ScopeRequest", request: true},
		{value: rauthy.ScopeResponse{}, schema: "ScopeResponse"},
		{value: rauthy.ClientSecretRequest{}, schema: "ClientSecretRequest", request: true},
		{value: rauthy.ClientSecretResponse{}, schema: "ClientSecretResponse"},
		{value: rauthy.APIKeyRequest{}, schema: "ApiKeyRequest", request: true},
		{value: rauthy.APIKeyResponse{}, schema: "ApiKeyResponse"},
		{value: rauthy.APIKeyAccess{}, schema: "ApiKeyAccess", request: true},
		{value: rauthy.IPBlacklistRequest{}, schema: "IpBlacklistRequest", request: true},
		{value: rauthy.BlacklistedIP{}, schema: "BlacklistedIp"},
		{value: rauthy.AuthProviderRequest{}, schema: "ProviderRequest", request: true},
		{value: rauthy.AuthProviderResponse{}, schema: "ProviderResponse"},
		{value: rauthy.AuthProviderLookupRequest{}, schema: "ProviderLookupRequest", request: true},
		{value: rauthy.AuthProviderLookupResponse{}, schema: "ProviderLookupResponse"},
		{value: rauthy.PasswordPolicy{}, schema: "PasswordPolicyRequest", request: true},
		{value: rauthy.PasswordPolicy{}, schema: "PasswordPolicyResponse"},
		{value: rauthy.AuthProviderLinkedUser{}, schema: "ProviderLinkedUserResponse"},
		{value: rauthy.UpdateUserRequest{}, schema: "UpdateUserRequest", request: true},
		{value: rauthy.UserValues{}, schema: "UserValuesRequest", request: true},
		{value: rauthy.UserValuesResponse{}, schema: "UserValuesResponse"},
		{value: rauthy.PamGroupResponse{}, schema: "PamGroupResponse"},
		{value: rauthy.PamGroupCreateRequest{}, schema: "PamGroupCreateRequest", request: true},
		{value: rauthy.PamGroupUserLink{}, schema: "PamGroupUserLink", request: true},
		{value: rauthy.PamUserCreateRequest{}, schema: "PamUserCreateRequest", request: true},
		{value: rauthy.PamUserUpdateRequest{}, schema: "PamUserUpdateRequest", request: true},
		{value: rauthy.PamHostSimpleResponse{}, schema: "PamHostSimpleResponse", liveShape: []string{"addresses"}},
		{value: rauthy.PamHostDetailsResponse{}, schema: "PamHostDetailsResponse", liveShape: []string{"ips"}},
		{value: rauthy.PamHostCreateRequest{}, schema: "PamHostCreateRequest", request: true},
		{
			value: rauthy.PamHostUpdateRequest{}, schema: "PamHostUpdateRequest", request: true,
			liveShape: []string{"ips"},
		},
		{value: rauthy.PamHostSecretResponse{}, schema: "PamHostSecretResponse"},
		{value: rauthy.RoleRequest{}, schema: "RoleRequest", request: true},
		{value: rauthy.RoleResponse{}, schema: "RoleResponse"},
		{value: rauthy.GroupRequest{}, schema: "GroupRequest", request: true},
		{value: rauthy.GroupResponse{}, schema: "GroupResponse"},
		{value: rauthy.UserAttrResponse{}, schema: "UserAttrConfigValueResponse"},
		{value: rauthy.NewUserRequest{}, schema: "NewUserRequest", request: true},
		{value: rauthy.UserResponse{}, schema: "UserResponse"},
		{value: rauthy.VersionResponse{}, schema: "AppVersionResponse"},
	}
}

// knownGaps lists the wire types that do not yet model every spec property.
// Each entry records exactly which properties are missing: a new gap fails the
// test, and so does a closed one until its entry is removed. Request types in
// this list lose the missing values on every read-modify-write.
func knownGaps() []coveredType {
	// Left out on purpose rather than pending: an SSH key is added by the user
	// through /pam/users/self/authorized_keys, never by an admin, so it is
	// runtime state the admin surface does not manage (see pam.go). Both types
	// are responses, so nothing written back loses the keys.
	authorizedKeys := []string{"authorized_keys"}
	return []coveredType{
		// Left out on purpose: against a live Rauthy 0.36.2, an attribute
		// created with a typ answers PUT and DELETE /users/attr/{name} with 404
		// "no rows returned", with or without typ in the body, and an untyped
		// attribute PUT with a typ answers 200 and then drops out of
		// GET /users/attr. Sending typ makes attributes that cannot be managed.
		// UserAttrResponse still reads it.
		{value: rauthy.UserAttrRequest{}, schema: "UserAttrConfigRequest", request: true, missing: []string{"typ"}},
		{value: rauthy.PamUserResponse{}, schema: "PamUserResponse", missing: authorizedKeys},
		{value: rauthy.PamUserDetailsResponse{}, schema: "PamUserDetailsResponse", missing: authorizedKeys},
	}
}

func loadSpecSchemas(t *testing.T) map[string]specSchema {
	t.Helper()

	path, err := mock.SpecPath()
	if err != nil {
		t.Fatalf("locate spec: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]specSchema `json:"schemas"`
		} `json:"components"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return doc.Components.Schemas
}

// jsonField is one field as encoding/json sees it.
type jsonField struct {
	typ       reflect.Type
	omitEmpty bool
}

// jsonFields returns the JSON fields of struct type t, flattening embedded
// structs the way encoding/json does.
func jsonFields(t reflect.Type) map[string]jsonField {
	out := map[string]jsonField{}
	for f := range t.Fields() {
		tag := f.Tag.Get("json")
		if f.Anonymous && tag == "" {
			maps.Copy(out, jsonFields(f.Type))
			continue
		}
		if !f.IsExported() || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		out[name] = jsonField{typ: f.Type, omitEmpty: slices.Contains(strings.Split(opts, ","), "omitempty")}
	}
	return out
}

// nilable reports whether a Go type can carry JSON null.
func nilable(t reflect.Type) bool {
	switch t.Kind() { //nolint:exhaustive // every other kind cannot be nil
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return true
	}
	return false
}

func TestSchemaCoverage(t *testing.T) {
	schemas := loadSpecSchemas(t)

	for _, ct := range slices.Concat(coveredTypes(), knownGaps()) {
		goType := reflect.TypeOf(ct.value)
		t.Run(goType.Name()+"/"+ct.schema, func(t *testing.T) {
			schema, ok := schemas[ct.schema]
			if !ok {
				t.Fatalf("the spec has no schema %q", ct.schema)
			}
			fields := jsonFields(goType)
			checkSchemaCoverage(t, schemas, schema, fields, ct)
		})
	}
}

func checkSchemaCoverage(
	t *testing.T,
	schemas map[string]specSchema,
	schema specSchema,
	fields map[string]jsonField,
	ct coveredType,
) {
	t.Helper()

	for name, prop := range schema.Properties {
		f, ok := fields[name]
		if !ok {
			if !slices.Contains(ct.missing, name) {
				t.Errorf("spec property %q is not modelled; a read-modify-write would lose it", name)
			}
			continue
		}
		if slices.Contains(ct.missing, name) {
			t.Errorf("spec property %q is modelled now; remove it from knownGaps", name)
		}
		checkPropertyType(t, name, prop, f, schemas, ct, slices.Contains(schema.Required, name))
	}
	for _, name := range ct.missing {
		if _, ok := schema.Properties[name]; !ok {
			t.Errorf("knownGaps lists %q, which the spec no longer declares", name)
		}
	}
	for name := range fields {
		if _, ok := schema.Properties[name]; !ok && !slices.Contains(ct.goOnly, name) {
			t.Errorf("field %q is not in the spec; Rauthy would ignore it", name)
		}
	}
}

// checkPropertyType checks that a modelled field can carry what the spec
// property allows, and, on a request, that it does not drop a required value.
func checkPropertyType(
	t *testing.T,
	name string,
	prop specProperty,
	f jsonField,
	schemas map[string]specSchema,
	ct coveredType,
	required bool,
) {
	t.Helper()

	jt := prop.jsonType(schemas)
	if !kindMatches(jt, f.typ) && !slices.Contains(ct.liveShape, name) {
		t.Errorf("spec property %q is %q but the Go field is %s", name, jt, f.typ)
	}
	if prop.nullable() && ct.request && !nilable(f.typ) {
		t.Errorf("spec property %q is nullable but %s cannot send null", name, f.typ)
	}
	if ct.request && f.omitEmpty && required {
		t.Errorf("spec property %q is required but omitempty drops its zero value", name)
	}
}
