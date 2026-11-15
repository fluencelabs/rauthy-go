# rauthy-go

A thin Go client for the [Rauthy](https://github.com/sebadob/rauthy) admin API
(`/auth/v1`): OIDC clients with their secrets and branding images, users, user attributes,
roles, groups, scopes, auth providers, API keys, the IP blacklist, the password
policy and the PAM configuration surface. It authenticates with a Rauthy API
key, retries transient failures, and ships the value constraints Rauthy enforces
on OIDC clients.

It is the client behind
[terraform-provider-rauthy](https://github.com/fluencelabs/terraform-provider-rauthy),
published on its own so other tools can use it. The package itself depends on
the standard library only.

**Tracked Rauthy version: 0.36.2.** The wire types and the contract tests match
that release; `RAUTHY_VERSION` in the `Makefile` is the single place it is
pinned.

## Install

```sh
go get github.com/fluencelabs/rauthy-go
```

The package name is `rauthy`.

## Usage

```go
import "github.com/fluencelabs/rauthy-go"

c, err := rauthy.New("https://auth.example.com", apiKey)
if err != nil {
	return err
}

created, err := c.CreateClient(ctx, rauthy.NewClientRequest{
	ID:           "example-app",
	Confidential: true,
	RedirectURIs: []string{"https://app.example.com/callback"},
})
if err != nil {
	return err
}

secret, err := c.GetClientSecret(ctx, created.ID)
if err != nil {
	return err
}
```

`New` takes the instance root; `/auth/v1` is appended for you. The API key needs
the access-group rights of the calls you make (here `Clients` and `Secrets`); a
403 is reported by `rauthy.IsForbidden` and usually names the missing right.
`WithHTTPClient` sets the transport and timeout, `WithRetryPolicy` the retry
budget. A runnable version of the example is `ExampleNew` in
`example_test.go`.

### Validation

`validation.go` carries the patterns, bounds and enums Rauthy checks on OIDC
clients (`ClientIDPattern()`, `URIPattern()`, `AccessTokenLifetimeMax`,
`GrantTypes()` and so on), so a bad value fails before the request with an error
that names the field. Rauthy's OpenAPI document does not publish these, so they
are transcribed from Rauthy's sources for the tracked version, not derived from
it. One difference is known: Go's `\s` matches ASCII whitespace only, Rust's
matches Unicode whitespace, so a client name or group containing non-ASCII
whitespace is accepted by Rauthy and rejected by `ClientNamePattern()` and
`GroupPattern()`.

## How it is verified

Two layers, in two repositories:

- **Contract tests, here.** `go test ./...` validates the request bodies this
  client sends, and the responses it expects back, against Rauthy's own OpenAPI
  document, vendored under `internal/mock/testdata`. They run in CI on every
  pull request and need no Rauthy instance. They also pin the places where the
  live API and its document disagree (see `pam.go`), so a Rauthy release that
  changes them is noticed.
- **Live acceptance tests, in terraform-provider-rauthy.** The provider's
  acceptance suite boots a real Rauthy and drives this client end to end
  through it. Those tests stay there; this repository has no live tests.

The OpenAPI document cannot express value ranges, so the contract tests check
field sets and types, not the constraints in `validation.go`.

## Refreshing the spec

When moving to a new Rauthy release:

```sh
make openapi-refresh RAUTHY_VERSION=0.37.0
```

This boots `ghcr.io/sebadob/rauthy:<version>` locally with the Swagger UI on and
writes `internal/mock/testdata/rauthy-openapi-<version>.json`. It needs Docker
and is never run in CI. Delete the old spec file (the tests refuse to choose
between two), bump `RAUTHY_VERSION` in the `Makefile`, run `make test`, and fix
whatever drifted.

## Development

```sh
make test   # go test ./... -count=1
make lint   # golangci-lint run (v2.7.1)
```
