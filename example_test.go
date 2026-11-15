package rauthy_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/fluencelabs/rauthy-go"
)

// ExampleNew creates a confidential OIDC client and reads back its secret. A
// stand-in server plays Rauthy here; against a real instance pass its root URL
// (https://auth.example.com) and an API key with the Clients and Secrets rights.
func ExampleNew() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /auth/v1/clients":
			_, _ = w.Write([]byte(`{"id":"example-app","enabled":true,"confidential":true,` +
				`"redirect_uris":["https://app.example.com/callback"],"flows_enabled":["authorization_code"]}`))
		case "POST /auth/v1/clients/example-app/secret":
			_, _ = w.Write([]byte(`{"id":"example-app","confidential":true,"secret":"s3cr3t"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := rauthy.New(srv.URL, "example-api-key")
	if err != nil {
		fmt.Println("new client:", err)
		return
	}

	created, err := c.CreateClient(ctx, rauthy.NewClientRequest{
		ID:           "example-app",
		Confidential: true,
		RedirectURIs: []string{"https://app.example.com/callback"},
	})
	if err != nil {
		fmt.Println("create client:", err)
		return
	}

	secret, err := c.GetClientSecret(ctx, created.ID)
	if err != nil {
		fmt.Println("read client secret:", err)
		return
	}
	fmt.Println(created.ID, *secret.Secret)
	// Output: example-app s3cr3t
}
