// Package mock provides test fixtures for the Rauthy API client: the vendored
// OpenAPI specification and a request/response validator built from it.
package mock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
)

// SpecPath returns the path of the vendored spec. The filename carries the
// Rauthy version it was generated from (rauthy-openapi-<version>.json) so that
// a spec left behind by a version bump is visible rather than silently stale.
func SpecPath() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the mock package on disk")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "testdata")

	matches, err := filepath.Glob(filepath.Join(dir, "rauthy-openapi-*.json"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		// The spec is committed, so its absence is a broken checkout or a
		// build that moved this file (-trimpath), never a state to skip on.
		return "", fmt.Errorf("no vendored Rauthy OpenAPI spec (rauthy-openapi-*.json) in %s", dir)
	}
	if len(matches) > 1 {
		sort.Strings(matches)
		return "", fmt.Errorf(
			"several vendored specs found (%v); keep exactly one so it is unambiguous which Rauthy version is the contract",
			matches,
		)
	}
	return matches[0], nil
}

// NewValidator builds a request/response validator from the vendored spec. This
// is the authoritative contract: drift between what the client sends and what
// Rauthy accepts shows up here without a live instance. Any error building the
// model fails the build: the pinned libopenapi-validator returns no validator
// alongside one.
func NewValidator() (validator.Validator, error) {
	path, err := SpecPath()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	doc, err := libopenapi.NewDocument(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	v, errs := validator.NewValidator(doc)
	if v == nil || len(errs) > 0 {
		return nil, fmt.Errorf("build validator from %s: %w", path, errors.Join(errs...))
	}
	return v, nil
}
