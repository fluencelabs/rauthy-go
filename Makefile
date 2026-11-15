.PHONY: test lint openapi-refresh

# Rauthy version the vendored OpenAPI spec was generated from. Bump together
# with the spec file (see openapi-refresh).
RAUTHY_VERSION ?= 0.36.2
SPEC := mock/testdata/rauthy-openapi-$(RAUTHY_VERSION).json

test:
	go test ./... -count=1

lint:
	golangci-lint run

# Regenerate the vendored OpenAPI spec by booting the Rauthy container locally
# with the Swagger UI enabled and fetching /auth/v1/docs/openapi.json.
#
# LOCAL ONLY, run by hand when bumping RAUTHY_VERSION. CI never runs this and
# has no Docker; it tests against the spec committed to the repo. Remove the old
# spec file afterwards: the contract tests refuse to pick between two.
openapi-refresh:
	./scripts/openapi-refresh.sh $(RAUTHY_VERSION) $(SPEC)
