package rauthy_test

import (
	"io"
	"net/http"
	"strconv"
	"testing"

	rauthy "github.com/fluencelabs/rauthy-go"
)

func TestLinkedUsers406(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusNotAcceptable, http.StatusForbidden} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/auth/v1/providers/provider1/delete_safe" {
					t.Errorf("request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `[{"id":"u1","email":"synthetic@example.com"}]`)
			})
			users, err := c.AuthProviderLinkedUsers(t.Context(), "provider1")
			if status == http.StatusForbidden {
				if !rauthy.IsForbidden(err) {
					t.Errorf("err = %v", err)
				}
				return
			}
			if err != nil || len(users) != 1 || users[0].ID != "u1" || users[0].Email != "synthetic@example.com" {
				t.Errorf("users=%v err=%v", users, err)
			}
		})
	}
}

func TestDeleteCanonicalIP(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"2001:0DB8:0000:0000:0000:0000:0000:0002": "2001:db8::2",
		"192.0.2.1": "192.0.2.1",
		"not-an-ip": "not-an-ip",
	} {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete || r.URL.Path != "/auth/v1/blacklist/"+want {
				t.Errorf("delete = %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		})
		if err := c.DeleteBlacklistedIP(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	}
}
