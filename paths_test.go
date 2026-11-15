package rauthy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

// pathCalls is every method that puts a caller-supplied identifier into the
// request path.
func pathCalls() map[string]func(context.Context, *rauthy.Client, string) error {
	img := rauthy.Image{Data: []byte("<svg/>"), ContentType: rauthy.MimeSVG}
	return map[string]func(context.Context, *rauthy.Client, string) error{
		"GetClient": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.GetClient(ctx, id)
			return err
		},
		"UpdateClient": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.UpdateClient(ctx, id, rauthy.UpdateClientRequest{})
			return err
		},
		"DeleteClient": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteClient(ctx, id)
		},
		"GetClientSecret": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.GetClientSecret(ctx, id)
			return err
		},
		"RotateClientSecret": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.RotateClientSecret(ctx, id, nil)
			return err
		},
		"GetClientImage": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.GetClientImage(ctx, id, rauthy.ImageLogo)
			return err
		},
		"PutClientImage": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.PutClientImage(ctx, id, rauthy.ImageLogo, img)
		},
		"DeleteClientImage": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteClientImage(ctx, id, rauthy.ImageFavicon)
		},
		"UpdateAPIKey": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.UpdateAPIKey(ctx, id, rauthy.APIKeyRequest{Name: id})
		},
		"RotateAPIKeySecret": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.RotateAPIKeySecret(ctx, id)
			return err
		},
		"DeleteAPIKey": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteAPIKey(ctx, id)
		},
		"UpdateAuthProvider": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.UpdateAuthProvider(ctx, id, rauthy.AuthProviderRequest{})
		},
		"DeleteAuthProvider": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteAuthProvider(ctx, id)
		},
		"AuthProviderLinkedUsers": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.AuthProviderLinkedUsers(ctx, id)
			return err
		},
		"UpdateRole": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.UpdateRole(ctx, id, rauthy.RoleRequest{Role: "r"})
			return err
		},
		"DeleteRole": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteRole(ctx, id)
		},
		"UpdateGroup": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.UpdateGroup(ctx, id, rauthy.GroupRequest{Group: "g"})
			return err
		},
		"DeleteGroup": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteGroup(ctx, id)
		},
		"UpdateScope": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.UpdateScope(ctx, id, rauthy.ScopeRequest{Scope: "s"})
			return err
		},
		"DeleteScope": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteScope(ctx, id)
		},
		"UpdateUserAttr": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.UpdateUserAttr(ctx, id, rauthy.UserAttrRequest{Name: id})
		},
		"DeleteUserAttr": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteUserAttr(ctx, id)
		},
		"DeleteBlacklistedIP": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteBlacklistedIP(ctx, id)
		},
		"GetPamHost": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.GetPamHost(ctx, id)
			return err
		},
		"UpdatePamHost": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.UpdatePamHost(ctx, id, rauthy.PamHostUpdateRequest{})
		},
		"DeletePamHost": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeletePamHost(ctx, id)
		},
		"GetPamHostSecret": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.GetPamHostSecret(ctx, id)
			return err
		},
		"GetUser": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.GetUser(ctx, id)
			return err
		},
		"UpdateUser": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.UpdateUser(ctx, id, rauthy.UpdateUserRequest{})
			return err
		},
		"DeleteUser": func(ctx context.Context, c *rauthy.Client, id string) error {
			return c.DeleteUser(ctx, id)
		},
		"GetUserByEmail": func(ctx context.Context, c *rauthy.Client, id string) error {
			_, err := c.GetUserByEmail(ctx, id)
			return err
		},
	}
}

// url.PathEscape leaves "." and ".." as they are, and an empty identifier
// collapses the path onto its parent, so each would address a different
// endpoint than the method names. They are refused before any request.
func TestPathIdentifiers_RejectsDotSegmentsAndEmpty(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	c, err := rauthy.New(srv.URL, "tf$secret")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for name, call := range pathCalls() {
		for _, id := range []string{"", ".", ".."} {
			callErr := call(t.Context(), c, id)
			if callErr == nil || !strings.Contains(callErr.Error(), "identifier") {
				t.Errorf("%s(%q): err = %v, want an invalid identifier error", name, id, callErr)
			}
		}
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("server received %d request(s), want none", got)
	}
}

// An identifier that merely contains dots is an ordinary one.
func TestPathIdentifiers_AllowsDotsInsideAnIdentifier(t *testing.T) {
	t.Parallel()

	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
	})

	if err := c.DeleteBlacklistedIP(t.Context(), "10.0.0.1"); err != nil {
		t.Fatalf("DeleteBlacklistedIP: %v", err)
	}
	if want := "/auth/v1/blacklist/10.0.0.1"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}
