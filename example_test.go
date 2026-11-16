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

// ExampleClient_GetVersion gates a feature on the Rauthy release the server
// runs. GET /version needs no access right, so any API key will do.
func ExampleClient_GetVersion() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "GET /auth/v1/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"current":"0.36.2","last_check":1788076923,"latest":"0.37.2",` +
			`"latest_url":"https://github.com/sebadob/rauthy/releases/tag/v0.37.2","update_available":true}`))
	}))
	defer srv.Close()

	c, err := rauthy.New(srv.URL, "example-api-key")
	if err != nil {
		fmt.Println("new client:", err)
		return
	}

	v, err := c.GetVersion(context.Background())
	if err != nil {
		fmt.Println("read version:", err)
		return
	}
	ok, err := v.AtLeast(0, 37)
	if err != nil {
		fmt.Println("compare version:", err)
		return
	}
	fmt.Println(v.Current, "at least 0.37:", ok)
	// Output: 0.36.2 at least 0.37: false
}
