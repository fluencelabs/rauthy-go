package rauthy_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	rauthy "github.com/fluencelabs/rauthy-go"
)

func TestCreateAuthProviderRejectedSecret(t *testing.T) {
	t.Parallel()
	secret := "SYNTHETIC" + strings.Repeat("x", 316)
	c, calls := newRetryClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/providers/create" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req rauthy.AuthProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ClientSecret == nil || *req.ClientSecret != secret {
			t.Error("request lost client_secret")
		}
		w.WriteHeader(http.StatusBadRequest)
		message := fmt.Sprintf(
			`Payload validation error: ValidationErrors({"client_secret": Field([ValidationError { code: "length", message: None, params: {"value": String(%q), "max": Number(256)} }])})`,
			secret,
		)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "BadRequest", "message": message}); err != nil {
			t.Error(err)
		}
	})
	_, err := c.CreateAuthProvider(t.Context(), rauthy.AuthProviderRequest{ClientSecret: &secret})
	var api *rauthy.APIError
	if !errors.As(err, &api) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if api.StatusCode != http.StatusBadRequest || api.ErrorCode != "BadRequest" || calls.Load() != 1 {
		t.Fatalf("unexpected response or retries: %v, calls %d", api, calls.Load())
	}
	for _, message := range []string{api.Message, api.Error(), fmt.Errorf("wrapped: %w", err).Error()} {
		if strings.Contains(message, secret) || !strings.Contains(message, "client_secret: length") {
			t.Errorf("unsafe or missing diagnostic: %s", message)
		}
	}
}
