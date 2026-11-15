package rauthy

import (
	"bytes"
	"encoding/json"
	"slices"
)

// Wire types mirroring Rauthy's `rauthy_api_types::clients` at v0.36.2.
//
// Field sets and JSON names are load-bearing: the API silently ignores unknown
// fields on POST /clients and rejects a PUT that omits a required one. The
// contract tests in this package validate both directions against the vendored
// OpenAPI spec.

// NewClientRequest is the body of POST /clients.
//
// This is deliberately tiny: Rauthy's NewClientRequest carries only these
// fields and silently drops everything else. The full configuration has to
// follow in a PUT.
type NewClientRequest struct {
	ID                     string   `json:"id"`
	Name                   *string  `json:"name,omitempty"`
	Confidential           bool     `json:"confidential"`
	RedirectURIs           []string `json:"redirect_uris"`
	PostLogoutRedirectURIs []string `json:"post_logout_redirect_uris,omitempty"`
}

// ScimClientRequestResponse is the SCIM sub-object, shared by the update
// request and the client response.
type ScimClientRequestResponse struct {
	BearerToken     string  `json:"bearer_token"`
	BaseURI         string  `json:"base_uri"`
	SyncGroups      bool    `json:"sync_groups"`
	GroupSyncPrefix *string `json:"group_sync_prefix,omitempty"`
}

// UpdateClientRequest is the body of PUT /clients/{id}.
//
// PUT is a full replacement: every non-pointer field below is required by
// Rauthy's deserializer, so the caller must supply a value even for settings it
// does not care about. Omitting Enabled or FlowsEnabled is a 400, not a no-op.
type UpdateClientRequest struct {
	ID                     string                     `json:"id"`
	Name                   *string                    `json:"name,omitempty"`
	Confidential           bool                       `json:"confidential"`
	RedirectURIs           []string                   `json:"redirect_uris"`
	PostLogoutRedirectURIs []string                   `json:"post_logout_redirect_uris,omitempty"`
	AllowedOrigins         []string                   `json:"allowed_origins,omitempty"`
	Enabled                bool                       `json:"enabled"`
	FlowsEnabled           []string                   `json:"flows_enabled"`
	AccessTokenAlg         string                     `json:"access_token_alg"`
	IDTokenAlg             string                     `json:"id_token_alg"`
	AuthCodeLifetime       int64                      `json:"auth_code_lifetime"`
	AccessTokenLifetime    int64                      `json:"access_token_lifetime"`
	Scopes                 []string                   `json:"scopes"`
	DefaultScopes          []string                   `json:"default_scopes"`
	Challenges             []string                   `json:"challenges,omitempty"`
	ForceMFA               bool                       `json:"force_mfa"`
	ClientURI              *string                    `json:"client_uri,omitempty"`
	Contacts               []string                   `json:"contacts,omitempty"`
	BackchannelLogoutURI   *string                    `json:"backchannel_logout_uri,omitempty"`
	RestrictGroupPrefix    *string                    `json:"restrict_group_prefix,omitempty"`
	Scim                   *ScimClientRequestResponse `json:"scim,omitempty"`
	// AllowedResources is the RFC 8707 allow-list of resource indicators the
	// client may request. Empty rejects any `resource` parameter.
	AllowedResources []string `json:"allowed_resources,omitempty"`
	// DefaultAud lists audiences always added to the client's tokens,
	// whatever `resource` the request carries.
	DefaultAud []string `json:"default_aud,omitempty"`
	// Claims holds admin-defined custom claims emitted into client_credentials
	// tokens: a JSON object of at most 1024 serialized characters, passed
	// through verbatim.
	Claims json.RawMessage `json:"claims,omitempty"`
	// ClaimsAtRoot emits Claims at the token root instead of nested under
	// `custom`.
	ClaimsAtRoot bool `json:"claims_at_root"`
}

// ClientResponse is the body returned by GET/POST/PUT on /clients[/{id}].
// The fields mean what they mean on UpdateClientRequest. Scim is absent from
// the GET /clients listing; only GET /clients/{id} returns it.
type ClientResponse struct {
	ID                     string                     `json:"id"`
	Name                   *string                    `json:"name,omitempty"`
	Enabled                bool                       `json:"enabled"`
	Confidential           bool                       `json:"confidential"`
	RedirectURIs           []string                   `json:"redirect_uris"`
	PostLogoutRedirectURIs []string                   `json:"post_logout_redirect_uris,omitempty"`
	AllowedOrigins         []string                   `json:"allowed_origins,omitempty"`
	FlowsEnabled           []string                   `json:"flows_enabled"`
	AccessTokenAlg         string                     `json:"access_token_alg"`
	IDTokenAlg             string                     `json:"id_token_alg"`
	AuthCodeLifetime       int64                      `json:"auth_code_lifetime"`
	AccessTokenLifetime    int64                      `json:"access_token_lifetime"`
	Scopes                 []string                   `json:"scopes"`
	DefaultScopes          []string                   `json:"default_scopes"`
	Challenges             []string                   `json:"challenges,omitempty"`
	ForceMFA               bool                       `json:"force_mfa"`
	ClientURI              *string                    `json:"client_uri,omitempty"`
	Contacts               []string                   `json:"contacts,omitempty"`
	BackchannelLogoutURI   *string                    `json:"backchannel_logout_uri,omitempty"`
	RestrictGroupPrefix    *string                    `json:"restrict_group_prefix,omitempty"`
	Scim                   *ScimClientRequestResponse `json:"scim,omitempty"`
	AllowedResources       []string                   `json:"allowed_resources,omitempty"`
	DefaultAud             []string                   `json:"default_aud,omitempty"`
	Claims                 json.RawMessage            `json:"claims,omitempty"`
	ClaimsAtRoot           bool                       `json:"claims_at_root"`
}

// UpdateRequest returns a PUT /clients/{id} body that keeps every setting of
// c. Because the PUT replaces the client wholesale, start from this and change
// only what the update is about: a body assembled field by field resets
// whatever it leaves out. Build it from GetClient, not from a listing entry,
// which carries no SCIM block. The result shares no memory with c.
func (c *ClientResponse) UpdateRequest() UpdateClientRequest {
	return UpdateClientRequest{
		ID:                     c.ID,
		Name:                   clonePtr(c.Name),
		Confidential:           c.Confidential,
		RedirectURIs:           slices.Clone(c.RedirectURIs),
		PostLogoutRedirectURIs: slices.Clone(c.PostLogoutRedirectURIs),
		AllowedOrigins:         slices.Clone(c.AllowedOrigins),
		Enabled:                c.Enabled,
		FlowsEnabled:           slices.Clone(c.FlowsEnabled),
		AccessTokenAlg:         c.AccessTokenAlg,
		IDTokenAlg:             c.IDTokenAlg,
		AuthCodeLifetime:       c.AuthCodeLifetime,
		AccessTokenLifetime:    c.AccessTokenLifetime,
		Scopes:                 slices.Clone(c.Scopes),
		DefaultScopes:          slices.Clone(c.DefaultScopes),
		Challenges:             slices.Clone(c.Challenges),
		ForceMFA:               c.ForceMFA,
		ClientURI:              clonePtr(c.ClientURI),
		Contacts:               slices.Clone(c.Contacts),
		BackchannelLogoutURI:   clonePtr(c.BackchannelLogoutURI),
		RestrictGroupPrefix:    clonePtr(c.RestrictGroupPrefix),
		Scim:                   c.Scim.clone(),
		AllowedResources:       slices.Clone(c.AllowedResources),
		DefaultAud:             slices.Clone(c.DefaultAud),
		Claims:                 bytes.Clone(c.Claims),
		ClaimsAtRoot:           c.ClaimsAtRoot,
	}
}

func (s *ScimClientRequestResponse) clone() *ScimClientRequestResponse {
	if s == nil {
		return nil
	}
	out := *s
	out.GroupSyncPrefix = clonePtr(s.GroupSyncPrefix)
	return &out
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// ClientSecretRequest is the body of PUT /clients/{id}/secret. A nil
// CacheCurrentHours means the previous secret stops being accepted immediately.
type ClientSecretRequest struct {
	CacheCurrentHours *int64 `json:"cache_current_hours,omitempty"`
}

// ClientSecretResponse is returned by POST and PUT on /clients/{id}/secret.
// Secret is nil for a public client.
type ClientSecretResponse struct {
	ID           string  `json:"id"`
	Confidential bool    `json:"confidential"`
	Secret       *string `json:"secret,omitempty"`
}
