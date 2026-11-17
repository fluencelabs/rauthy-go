package rauthy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// UserAttrRequest is the body of POST /users/attr and PUT /users/attr/{name}.
//
// DefaultValue is arbitrary JSON upstream (`serde_json::Value`), so it is kept
// as raw bytes rather than being forced into a Go type: Rauthy stores whatever
// it is handed and gives it back unchanged.
//
// `typ` is deliberately not sent. Against a live Rauthy 0.36.2 an attribute
// written with a typ cannot be managed afterwards: one created with
// "typ":"Email" answers PUT and DELETE /users/attr/{name} with 404 "no rows
// returned", whatever the body, and an untyped one PUT with a typ answers 200
// and then drops out of GET /users/attr. UserAttrResponse.Typ reports it.
type UserAttrRequest struct {
	Name         string          `json:"name"`
	Desc         *string         `json:"desc,omitempty"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	UserEditable *bool           `json:"user_editable,omitempty"`
}

// UserAttrResponse is one configured user attribute as Rauthy returns it.
// Unlike the request, user_editable is always present here.
type UserAttrResponse struct {
	Name         string          `json:"name"`
	Desc         *string         `json:"desc,omitempty"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	// Typ is the attribute's value type ("Email" is the only one Rauthy 0.36.2
	// knows), read-only here: see UserAttrRequest for why it is never sent.
	Typ          *string `json:"typ,omitempty"`
	UserEditable bool    `json:"user_editable"`
}

// UpdateRequest returns a PUT /users/attr/{name} body that keeps the
// description, default value and user_editable of a. The PUT replaces the
// attribute wholesale, so start from this and change only what the update is
// about: a body assembled field by field resets whatever it leaves out. Typ is
// not carried (see UserAttrRequest), and an attribute that has one cannot be
// updated on 0.36.2 at all. Keep the name: UpdateUserAttr explains why a PUT
// must not rename. The result shares no memory with a.
func (a *UserAttrResponse) UpdateRequest() UserAttrRequest {
	editable := a.UserEditable
	return UserAttrRequest{
		Name:         a.Name,
		Desc:         clonePtr(a.Desc),
		DefaultValue: bytes.Clone(a.DefaultValue),
		UserEditable: &editable,
	}
}

// userAttrListResponse wraps the list. GET /users/attr does not answer with a
// bare array the way /scopes, /roles and /groups do — it wraps the attributes
// in an object under `values`.
type userAttrListResponse struct {
	Values []UserAttrResponse `json:"values"`
}

const userAttrPathBase = "/users/attr"

// KNOWN SERVER-SIDE RACE, found against a live Rauthy 0.35.2 and not visible in
// the OpenAPI document. Rauthy keeps the whole set of user attribute
// definitions in one cached list and every write is a read-modify-write of it,
// with no locking. Overlapping writes therefore lose one another: three pairs
// of concurrent POSTs left the list holding one attribute twice and two others
// not at all. The rows do reach the database — a later DELETE rebuilds the
// cache and the missing ones reappear — but until something invalidates it,
// GET /users/attr serves the corrupted list, so an attribute created moments
// earlier is invisible.
//
// That is fatal here rather than merely untidy: a scope's attr_include_*
// mapping is filtered against exactly that list, so a scope applied alongside
// the attributes it maps silently loses them. And callers such as Terraform
// apply independent resources in parallel — ten at a time by default — which
// makes overlapping creates the normal case rather than an unlucky one.
//
// Writes are therefore serialised through Client.userAttrMu, and that is all
// the lock does. It belongs to one Client value, not to the process: two
// Clients writing to one instance, in one process or in several, still race
// each other, and so does the Admin UI, so callers that need the ordering share
// one Client. It also orders only the writes this client is waiting on, which
// leaves two overlaps it cannot prevent. A write abandoned on an http.Client
// timeout or a cancelled context releases the lock as soon as the client stops
// waiting, while the server may still be running it, so the next queued write
// can overlap it; the abandoned write itself is not retried. And a PUT or
// DELETE whose connection broke mid-exchange (a reset, an EOF) is retried
// under the lock, so the retry can overlap its own first attempt if that one
// reached the server. A POST is not retried once it may have been sent.
//
// Reads of the attributes are left unserialised: in the Rauthy versions this
// client supports they do not store the attribute list back. A write also
// takes Client.scopeMu, after userAttrMu: Rauthy edits the scopes' attribute
// mappings and empties the scope cache on it, which a concurrent scope call
// would otherwise overwrite with a stale list (see the note above scopePath).
//
// If a Rauthy release makes these writes atomic, the semaphore can go.

func userAttrPath(name string) string {
	return userAttrPathBase + "/" + url.PathEscape(name)
}

// CreateUserAttr issues POST /users/attr. Requires the UserAttributes:Create
// right.
func (c *Client) CreateUserAttr(ctx context.Context, req UserAttrRequest) (*UserAttrResponse, error) {
	if err := c.userAttrMu.lock(ctx); err != nil {
		return nil, err
	}
	defer c.userAttrMu.unlock()
	if err := c.scopeMu.lock(ctx); err != nil {
		return nil, err
	}
	defer c.scopeMu.unlock()

	var out UserAttrResponse
	if err := c.do(ctx, http.MethodPost, userAttrPathBase, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListUserAttrs issues GET /users/attr. Requires UserAttributes:Read.
func (c *Client) ListUserAttrs(ctx context.Context) ([]UserAttrResponse, error) {
	var out userAttrListResponse
	if err := c.do(ctx, http.MethodGet, userAttrPathBase, nil, &out); err != nil {
		return nil, err
	}
	return out.Values, nil
}

// GetUserAttr returns the attribute with the given name. As with roles, groups
// and scopes, Rauthy offers no per-name read, so the list is filtered here and
// a missing attribute yields a synthetic 404.
func (c *Client) GetUserAttr(ctx context.Context, name string) (*UserAttrResponse, error) {
	attrs, err := c.ListUserAttrs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range attrs {
		if attrs[i].Name == name {
			return &attrs[i], nil
		}
	}
	return nil, notFoundError(userAttrPathBase, "no user attribute named "+name)
}

// UpdateUserAttr issues PUT /users/attr/{name}. Requires UserAttributes:Update.
//
// The attribute is addressed by its current name and the body carries the name
// it should have afterwards, which reads like a rename — but is not one, and
// the client never uses it as one. Against a live Rauthy 0.35.2 a PUT that
// changes the name leaves the old name occupied: it disappears from
// GET /users/attr, yet a later POST under it fails with "User attribute config
// does already exist", so the name is unusable until someone deletes it by
// hand. Same root cause as the race above — the cached list and the stored rows
// disagree. rauthy_user_attribute therefore replaces the resource on a rename
// instead, and this method only ever carries the name it was given.
//
// The spec declares no response body for this operation and a live instance
// does send one; it is discarded either way, and the caller reads the attribute
// back rather than trusting it.
func (c *Client) UpdateUserAttr(ctx context.Context, name string, req UserAttrRequest) error {
	if err := c.userAttrMu.lock(ctx); err != nil {
		return err
	}
	defer c.userAttrMu.unlock()
	if err := c.scopeMu.lock(ctx); err != nil {
		return err
	}
	defer c.scopeMu.unlock()

	return c.do(ctx, http.MethodPut, userAttrPath(name), req, nil)
}

// DeleteUserAttr issues DELETE /users/attr/{name}. Requires
// UserAttributes:Delete.
func (c *Client) DeleteUserAttr(ctx context.Context, name string) error {
	if err := c.userAttrMu.lock(ctx); err != nil {
		return err
	}
	defer c.userAttrMu.unlock()
	if err := c.scopeMu.lock(ctx); err != nil {
		return err
	}
	defer c.scopeMu.unlock()

	return c.do(ctx, http.MethodDelete, userAttrPath(name), nil, nil)
}

// writeLock is a context-aware mutex for a list Rauthy keeps as one cached
// value and rewrites without locking. A caller waiting for it gives up when its
// context ends.
type writeLock chan struct{}

func newWriteLock() writeLock { return make(writeLock, 1) }

func (l writeLock) lock(ctx context.Context) error {
	select {
	case l <- struct{}{}:
		if err := ctx.Err(); err != nil {
			l.unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l writeLock) unlock() { <-l }
