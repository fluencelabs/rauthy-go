package rauthy

import (
	"context"
	"net/http"
	"net/url"
)

// UserValues is the optional profile block carried by a user, shared by the
// update request and the user response. Every field is a pointer because
// Rauthy distinguishes "not set" from "set to empty", and clearing a value
// means sending an explicit null.
//
// preferred_username appears only on the response side; Rauthy has a dedicated
// endpoint for changing it, so it is read-only here.
type UserValues struct {
	Birthdate *string `json:"birthdate"`
	City      *string `json:"city"`
	Country   *string `json:"country"`
	Phone     *string `json:"phone"`
	Street    *string `json:"street"`
	Zip       *string `json:"zip"`
	TZ        *string `json:"tz"`
}

// UserValuesResponse adds the fields Rauthy returns but does not accept back.
type UserValuesResponse struct {
	UserValues

	PreferredUsername *string `json:"preferred_username"`
}

// NewUserRequest is the body of POST /users.
//
// As with clients, this is deliberately smaller than the full user: Rauthy's
// NewUserRequest carries no `enabled`, `email_verified` or profile values, so
// anything beyond these fields has to follow in a PUT.
type NewUserRequest struct {
	Email      string   `json:"email"`
	Language   string   `json:"language"`
	Roles      []string `json:"roles"`
	Groups     []string `json:"groups,omitempty"`
	GivenName  *string  `json:"given_name,omitempty"`
	FamilyName *string  `json:"family_name,omitempty"`
	// PreferredUsername is set at creation only: UpdateUserRequest does not
	// carry it, and the response reports it under user_values.
	PreferredUsername *string `json:"preferred_username,omitempty"`
	TZ                *string `json:"tz,omitempty"`
	// UserExpires is a Unix timestamp in seconds.
	UserExpires *int64 `json:"user_expires,omitempty"`
}

// UpdateUserRequest is the body of PUT /users/{id}.
//
// PUT is a full replacement: Rauthy's deserializer requires email, roles,
// enabled and email_verified, and an omitted `groups` or `user_values` clears
// what was there rather than leaving it alone.
//
// GivenName is required too, though the OpenAPI document calls it nullable —
// a null is answered with "'given_name' is required". That divergence is
// invisible to the contract tests and was found by running against a live
// instance; the resource makes the attribute Required because of it.
// FamilyName is genuinely optional: a PUT carrying only GivenName is accepted.
type UpdateUserRequest struct {
	Email         string      `json:"email"`
	Enabled       bool        `json:"enabled"`
	EmailVerified bool        `json:"email_verified"`
	Roles         []string    `json:"roles"`
	Groups        []string    `json:"groups"`
	GivenName     *string     `json:"given_name"`
	FamilyName    *string     `json:"family_name"`
	Language      *string     `json:"language,omitempty"`
	Password      *string     `json:"password,omitempty"`
	UserExpires   *int64      `json:"user_expires"`
	UserValues    *UserValues `json:"user_values,omitempty"`
}

// UserResponse is a user as Rauthy returns it from /users[/{id}].
type UserResponse struct {
	ID            string             `json:"id"`
	Email         string             `json:"email"`
	Enabled       bool               `json:"enabled"`
	EmailVerified bool               `json:"email_verified"`
	Language      string             `json:"language"`
	Roles         []string           `json:"roles"`
	Groups        []string           `json:"groups,omitempty"`
	GivenName     *string            `json:"given_name,omitempty"`
	FamilyName    *string            `json:"family_name,omitempty"`
	AccountType   string             `json:"account_type"`
	CreatedAt     int64              `json:"created_at"`
	LastLogin     *int64             `json:"last_login,omitempty"`
	UserExpires   *int64             `json:"user_expires,omitempty"`
	UserValues    UserValuesResponse `json:"user_values"`

	// The fields below are reported by Rauthy and not accepted back: no
	// request type carries them, so an update leaves them alone.

	// AuthProviderID names the upstream auth provider of a federated user.
	AuthProviderID *string `json:"auth_provider_id,omitempty"`
	// FederationUID is the user's id at that upstream provider.
	FederationUID *string `json:"federation_uid,omitempty"`
	// FailedLoginAttempts is the count of failed logins Rauthy has recorded.
	FailedLoginAttempts *int64 `json:"failed_login_attempts,omitempty"`
	// LastFailedLogin is a Unix timestamp in seconds.
	LastFailedLogin *int64 `json:"last_failed_login,omitempty"`
	// PasswordExpires is a Unix timestamp in seconds.
	PasswordExpires *int64  `json:"password_expires,omitempty"`
	PictureID       *string `json:"picture_id,omitempty"`
	WebauthnUserID  *string `json:"webauthn_user_id,omitempty"`
}

func userPath(id string) string {
	return "/users/" + url.PathEscape(id)
}

// CreateUser issues POST /users. Requires the Users:Create right.
func (c *Client) CreateUser(ctx context.Context, req NewUserRequest) (*UserResponse, error) {
	var out UserResponse
	if err := c.do(ctx, http.MethodPost, "/users", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUser issues GET /users/{id}. Requires Users:Read.
func (c *Client) GetUser(ctx context.Context, id string) (*UserResponse, error) {
	var out UserResponse
	if err := c.do(ctx, http.MethodGet, userPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUserByEmail issues GET /users/email/{email}. Requires Users:Read.
func (c *Client) GetUserByEmail(ctx context.Context, email string) (*UserResponse, error) {
	var out UserResponse
	path := "/users/email/" + url.PathEscape(email)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateUser issues PUT /users/{id}. Requires Users:Update.
func (c *Client) UpdateUser(ctx context.Context, id string, req UpdateUserRequest) (*UserResponse, error) {
	var out UserResponse
	if err := c.do(ctx, http.MethodPut, userPath(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteUser issues DELETE /users/{id}. Requires Users:Delete.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, userPath(id), nil, nil)
}
