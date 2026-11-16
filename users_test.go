package rauthy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

func TestContract_CreateUserRequest(t *testing.T) {
	v := newContractValidator(t)

	given := "Ada"
	family := "Lovelace"
	preferred := "ada"
	expires := int64(4102444800)
	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/users"), rauthy.NewUserRequest{
		Email:             "ada@example.com",
		Language:          "en",
		Roles:             []string{"admin"},
		Groups:            []string{"engineering"},
		GivenName:         &given,
		FamilyName:        &family,
		PreferredUsername: &preferred,
		UserExpires:       &expires,
	})
	if !ok {
		t.Errorf("POST /users body rejected by the spec: %s", msg)
	}
}

// Roles is required even when empty: a nil slice would marshal to null and
// Rauthy's deserializer rejects that, which is why the client sends [].
func TestContract_CreateUserRequiresRoles(t *testing.T) {
	v := newContractValidator(t)

	partial := map[string]any{"email": "ada@example.com", "language": "en"}
	if ok, _ := validateRequest(t, v, http.MethodPost, apiPath("/users"), partial); ok {
		t.Error("POST /users accepted a body without roles; the spec no longer requires it, " +
			"revisit NewUserRequest")
	}
}

func TestContract_UpdateUserRequest(t *testing.T) {
	v := newContractValidator(t)

	given := "Ada"
	lang := "en"
	city := "London"
	tz := "Europe/London"
	ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/users/u-1"), rauthy.UpdateUserRequest{
		Email:         "ada@example.com",
		Enabled:       true,
		EmailVerified: true,
		Roles:         []string{"admin"},
		Groups:        []string{},
		GivenName:     &given,
		Language:      &lang,
		UserValues:    &rauthy.UserValues{City: &city, TZ: &tz},
	})
	if !ok {
		t.Errorf("PUT /users/{id} body rejected by the spec: %s", msg)
	}
}

// A PUT is a full replacement and Rauthy's deserializer requires these four.
// If a future spec relaxes that, the client may stop sending them.
func TestContract_UpdateUserRequiresEnabledAndVerified(t *testing.T) {
	v := newContractValidator(t)

	partial := map[string]any{"email": "ada@example.com", "roles": []string{}}
	if ok, _ := validateRequest(t, v, http.MethodPut, apiPath("/users/u-1"), partial); ok {
		t.Error("PUT /users/{id} accepted a body without enabled/email_verified; " +
			"the spec no longer requires them, revisit UpdateUserRequest")
	}
}

func TestContract_UserResponse(t *testing.T) {
	v := newContractValidator(t)

	body := `{
		"id": "u-1",
		"email": "ada@example.com",
		"given_name": "Ada",
		"language": "en",
		"roles": ["admin"],
		"groups": ["engineering"],
		"enabled": true,
		"email_verified": true,
		"created_at": 1700000000,
		"account_type": "password",
		"user_values": {"city": "London", "preferred_username": "ada"}
	}`

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/users/u-1"), http.StatusOK, body)
	if !ok {
		t.Errorf("GET /users/{id} response rejected by the spec: %s", msg)
	}

	var decoded rauthy.UserResponse
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode into UserResponse: %v", err)
	}
	if decoded.ID != "u-1" || decoded.AccountType != "password" || !decoded.EmailVerified {
		t.Errorf("UserResponse lost fields on decode: %+v", decoded)
	}
	// preferred_username is promoted from the embedded UserValues, so a
	// mistake in that embedding shows up as a silently empty value.
	if decoded.UserValues.PreferredUsername == nil || *decoded.UserValues.PreferredUsername != "ada" {
		t.Errorf("preferred_username did not decode: %+v", decoded.UserValues)
	}
	if decoded.UserValues.City == nil || *decoded.UserValues.City != "London" {
		t.Errorf("user_values.city did not decode: %+v", decoded.UserValues)
	}
}

// The read-only fields Rauthy reports on a user decode, and an absent or null
// one stays nil.
func TestContract_UserResponseReadOnlyFields(t *testing.T) {
	v := newContractValidator(t)

	body := `{
		"id": "u-1",
		"email": "ada@example.com",
		"language": "en",
		"roles": [],
		"enabled": true,
		"email_verified": true,
		"created_at": 1700000000,
		"account_type": "federated_password",
		"user_values": {},
		"auth_provider_id": "provider-1",
		"failed_login_attempts": 3,
		"federation_uid": "upstream-42",
		"last_failed_login": 1700000100,
		"password_expires": 1800000000,
		"picture_id": "pic-1",
		"webauthn_user_id": "wa-1"
	}`
	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/users/u-1"), http.StatusOK, body)
	if !ok {
		t.Errorf("GET /users/{id} response rejected by the spec: %s", msg)
	}

	var got rauthy.UserResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode into UserResponse: %v", err)
	}
	if deref(got.AuthProviderID) != "provider-1" || deref(got.FailedLoginAttempts) != 3 ||
		deref(got.FederationUID) != "upstream-42" || deref(got.LastFailedLogin) != 1700000100 ||
		deref(got.PasswordExpires) != 1800000000 || deref(got.PictureID) != "pic-1" ||
		deref(got.WebauthnUserID) != "wa-1" {
		t.Errorf("read-only fields did not decode: %+v", got)
	}

	var bare rauthy.UserResponse
	if err := json.Unmarshal([]byte(`{"id":"u-1","failed_login_attempts":null}`), &bare); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if bare.AuthProviderID != nil || bare.FailedLoginAttempts != nil || bare.PasswordExpires != nil {
		t.Errorf("absent read-only fields are not nil: %+v", bare)
	}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// preferred_username is accepted at creation and was dropped before it was
// modelled; it has to reach the wire.
func TestCreateUser_SendsPreferredUsername(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = nil
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u-1","email":"ada@example.com","language":"en","roles":[],` +
			`"enabled":false,"email_verified":false,"created_at":1700000000,"account_type":"new",` +
			`"user_values":{"preferred_username":"ada"}}`))
	})

	preferred := "ada"
	if _, err := c.CreateUser(context.Background(), rauthy.NewUserRequest{
		Email: "ada@example.com", Language: "en", Roles: []string{}, PreferredUsername: &preferred,
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if gotBody["preferred_username"] != "ada" {
		t.Errorf("body = %v, want preferred_username=ada", gotBody)
	}

	// Unset, it stays out of the body.
	if _, err := c.CreateUser(context.Background(), rauthy.NewUserRequest{
		Email: "ada@example.com", Language: "en", Roles: []string{},
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, present := gotBody["preferred_username"]; present {
		t.Errorf("body = %v, want no preferred_username", gotBody)
	}
}
