# rauthy-go

[![ci](https://github.com/fluencelabs/rauthy-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/fluencelabs/rauthy-go/actions/workflows/ci.yml)

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
budget. A 429, a 5xx or a transport failure such as a reset is retried for
GET, PUT and DELETE, except a PUT whose SCIM sync failed after Rauthy stored it
(a 500 with error "Scim"), and for a creating POST, a password change or a
secret rotation only on a 429 or when nothing was sent; a request that timed out is never retried, because it may
still be running on the server. With the defaults the pauses between attempts
add up to at most 1.4 seconds. A runnable version of the example is `ExampleNew` in
`example_test.go`.

`GetVersion` reads the Rauthy release the server runs (`GET /version`, which
needs no access right, so it is sent without the API key), and
`VersionResponse.AtLeast(major, minor)` answers whether that release is at
least `major.minor`, by semantic versioning precedence. A version string that
does not parse is an error, not a guess.

`UpdateClient` is a full replace: a field the body leaves out is reset, so
build the body with `GetClient(...).UpdateRequest()` and change only what the
update is about. Scopes, roles, groups and user attributes work the same way,
each response type carrying an `UpdateRequest()`.

`RoleRequest`, `RoleResponse`, `GroupRequest` and `GroupResponse` carry `Meta`,
a `json.RawMessage`, since the release after v0.1.0. That is a source
incompatibility for some callers: the four structs are no longer comparable
with `==` and cannot be map keys, and a positional (unkeyed) literal of them
no longer compiles. Keyed literals are unaffected.

### Known limits

- **Canceled mutations.** Canceling or timing out an in-flight scope or
  user-attribute write releases the SDK locks while Rauthy may still be
  processing it. Another call on the same `Client` can see stale scope cache
  data. Let those writes finish during an apply. If one is interrupted,
  re-read the affected state after the server has had time to finish.
- **Client PUT and SCIM.** Rauthy can save a client update and then return
  HTTP 500 (`Scim`) if group synchronization fails. A repeat PUT after a
  lost response can succeed without running that sync again. Rauthy's
  scheduler retries recorded tasks up to the configured limit. Check the
  Rauthy logs and `failed_scim_tasks` for the sync outcome.
  [SDK test for one request on HTTP 500](retry_round2_test.go#L189).
- **Scope cache outside the SDK lock.** Rauthy logins, token requests and
  SCIM can refill the scope cache, and separate SDK `Client` instances have
  separate locks. Overlapping calls can leave stale scope or mapping data
  in the cache. Serialize scope and user-attribute changes across applies
  and re-read after suspected overlap.
  [SDK test for serialization on one client](scopes_serial_test.go#L20).
- **Truncated response bodies.** When a body read fails after the headers
  arrive, the SDK keeps the HTTP status for retry decisions. If it returns
  the read error, callers lose the error body and cannot classify the status
  with helpers such as `IsNotFound`. Check server or proxy logs for the
  status and re-read resource state before repeating a write.
  [SDK test for retries on truncated responses](retry_round2_test.go#L302).
- **Concurrent writers.** The Rauthy admin API has no conditional writes
  with ETag / If-Match, so concurrent writers can overwrite each other's
  changes. Assign one writer per resource or coordinate writes across
  controllers and administrators.
- **S10. Client image reads.** Rauthy uses HTTP 404 with no body or
  Content-Type when a client logo or favicon is unavailable. A proxy with a
  missing image route can return the same response. `GetClientImage` reports
  `ErrImageNotFound` in either case, so a provider can drop image state.
  Check that the proxy forwards both image routes before recreating an image.

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
`TestSchemaCoverage` compares the field sets of the wire types it lists with
the spec's schemas; the types that still miss spec properties are recorded in
its `knownGaps` table, property by property, with the reason each is left out.

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
make test   # go test ./... -race -count=1 -timeout 20m
make lint   # golangci-lint run (v2.15.0)
```

## Releases

Commits on `main` follow [Conventional Commits](https://www.conventionalcommits.org/).
release-please reads them and keeps a release pull request open with the next
version and its `CHANGELOG.md` entry; merging that pull request tags `vX.Y.Z`
and publishes the GitHub release, and `go get` picks the tag up from the module
proxy. Before 1.0 a `feat` or a breaking change bumps the minor version and a
`fix` the patch version. Tags are never created by hand.

## License

Apache License 2.0, see [LICENSE](LICENSE).
