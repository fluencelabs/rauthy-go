package rauthy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// RoleResponse is a role as Rauthy returns it from /roles.
type RoleResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Meta is free-form JSON attached to the role, passed through verbatim.
	Meta json.RawMessage `json:"meta,omitempty"`
}

// UpdateRequest returns a PUT /roles/{id} body that keeps every setting of r.
// The PUT replaces the role wholesale, so start from this and change only what
// the update is about: a body assembled field by field resets whatever it
// leaves out, meta included: on a live Rauthy 0.36.2 a role PUT without meta
// clears it, and a PUT built from this keeps it. The result shares no memory
// with r.
func (r *RoleResponse) UpdateRequest() RoleRequest {
	return RoleRequest{Role: r.Name, Meta: bytes.Clone(r.Meta)}
}

// RoleRequest is the body of POST /roles and PUT /roles/{id}.
type RoleRequest struct {
	Role string `json:"role"`
	// Meta is free-form JSON attached to the role. Left empty it is not sent,
	// and on a live 0.36.2 a PUT without it clears what the role carried.
	Meta json.RawMessage `json:"meta,omitempty"`
}

func rolePath(id string) string {
	return "/roles/" + url.PathEscape(id)
}

// CreateRole issues POST /roles. Requires the Roles:Create right.
func (c *Client) CreateRole(ctx context.Context, req RoleRequest) (*RoleResponse, error) {
	var out RoleResponse
	if err := c.do(ctx, http.MethodPost, "/roles", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListRoles issues GET /roles. Requires Roles:Read.
func (c *Client) ListRoles(ctx context.Context) ([]RoleResponse, error) {
	var out []RoleResponse
	if err := c.do(ctx, http.MethodGet, "/roles", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRole returns the role with the given id.
//
// Rauthy has no GET /roles/{id}: the whole list is fetched and filtered here.
// A role that is not in the list yields a synthetic 404, so callers can use
// IsNotFound the same way they do for clients.
func (c *Client) GetRole(ctx context.Context, id string) (*RoleResponse, error) {
	roles, err := c.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].ID == id {
			return &roles[i], nil
		}
	}
	return nil, notFoundError("/roles", "role "+id+" does not exist")
}

// GetRoleByName returns the role with the given name, or a synthetic 404.
func (c *Client) GetRoleByName(ctx context.Context, name string) (*RoleResponse, error) {
	roles, err := c.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].Name == name {
			return &roles[i], nil
		}
	}
	return nil, notFoundError("/roles", "no role named "+name)
}

// UpdateRole issues PUT /roles/{id}, replacing the role wholesale: build req
// with RoleResponse.UpdateRequest from a GetRole result and change only what
// the update is about. Requires Roles:Update.
func (c *Client) UpdateRole(ctx context.Context, id string, req RoleRequest) (*RoleResponse, error) {
	var out RoleResponse
	if err := c.do(ctx, http.MethodPut, rolePath(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole issues DELETE /roles/{id}. Requires Roles:Delete.
func (c *Client) DeleteRole(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, rolePath(id), nil, nil)
}
