package rauthy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// VersionResponse is the body of GET /version: the Rauthy release the server
// runs and what it last learned about newer ones.
type VersionResponse struct {
	// Current is the running release, a semantic version such as "0.36.2".
	Current string `json:"current"`
	// LastCheck is when Rauthy last looked for a newer release, as a Unix
	// timestamp in seconds; nil until it has looked.
	LastCheck *int64 `json:"last_check,omitempty"`
	// Latest is the newest release Rauthy knows of, nil until it has looked.
	Latest *string `json:"latest,omitempty"`
	// LatestURL points at the release notes of Latest.
	LatestURL *string `json:"latest_url,omitempty"`
	// UpdateAvailable reports that Latest is newer than Current.
	UpdateAvailable bool `json:"update_available"`
}

// GetVersion issues GET /version, without the API key. Rauthy serves the
// endpoint to anyone: its OpenAPI document names no access right for it, and a
// live 0.36.2 answers 200 to a request without credentials. Sending the key
// would only add a way to fail: the same server answers 400 to an invalid API
// key, so a revoked or mistyped key would break a call that needs none.
func (c *Client) GetVersion(ctx context.Context) (*VersionResponse, error) {
	resp, err := c.send(ctx, http.MethodGet, "/version", nil, map[string]string{"Authorization": ""})
	if err != nil {
		return nil, err
	}
	var out VersionResponse
	if err = json.Unmarshal(resp.body, &out); err != nil {
		return nil, fmt.Errorf("decode GET /version response: %w", err)
	}
	return &out, nil
}

// AtLeast reports whether Current is release major.minor.0 or later, by
// semantic versioning precedence: a pre-release of major.minor.0 such as
// "0.37.0-rc.1" comes before "0.37.0" and is therefore not yet 0.37, and build
// metadata is ignored. Current has to be a semantic version in full
// (MAJOR.MINOR.PATCH with optional pre-release and build parts, no leading "v");
// anything else is an error rather than a guess in either direction, as is a
// negative major or minor.
func (v *VersionResponse) AtLeast(major, minor int) (bool, error) {
	if major < 0 || minor < 0 {
		return false, fmt.Errorf("version floor %d.%d must not be negative", major, minor)
	}
	sv, err := parseSemver(v.Current)
	if err != nil {
		return false, err
	}
	switch {
	case sv.major != major:
		return sv.major > major, nil
	case sv.minor != minor:
		return sv.minor > minor, nil
	case sv.patch > 0:
		return true, nil
	}
	return !sv.prerelease, nil
}

// semver is the part of a semantic version that AtLeast compares.
type semver struct {
	major, minor, patch int
	prerelease          bool
}

// semverCoreParts is the number of dot-separated numbers in MAJOR.MINOR.PATCH.
const semverCoreParts = 3

// parseSemver parses a version by the Semantic Versioning 2.0.0 grammar.
func parseSemver(s string) (semver, error) {
	fail := func(why string) (semver, error) {
		return semver{}, fmt.Errorf("rauthy version %q is not a semantic version: %s", s, why)
	}

	rest, build, hasBuild := strings.Cut(s, "+")
	if hasBuild {
		if err := checkIdentifiers(build, false); err != nil {
			return fail("build metadata: " + err.Error())
		}
	}
	core, pre, hasPre := strings.Cut(rest, "-")
	if hasPre {
		if err := checkIdentifiers(pre, true); err != nil {
			return fail("pre-release: " + err.Error())
		}
	}

	parts := strings.Split(core, ".")
	if len(parts) != semverCoreParts {
		return fail("want MAJOR.MINOR.PATCH")
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := parseNumericIdentifier(p)
		if err != nil {
			return fail(err.Error())
		}
		nums[i] = n
	}
	return semver{major: nums[0], minor: nums[1], patch: nums[2], prerelease: hasPre}, nil
}

// parseNumericIdentifier parses a version number: ASCII digits, no sign and
// no leading zero.
func parseNumericIdentifier(p string) (int, error) {
	if !isDigits(p) {
		return 0, fmt.Errorf("%q is not a number", p)
	}
	if len(p) > 1 && p[0] == '0' {
		return 0, fmt.Errorf("%q has a leading zero", p)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("%q is out of range", p)
	}
	return n, nil
}

// checkIdentifiers checks a dot-separated pre-release or build part: non-empty
// identifiers of ASCII letters, digits and hyphens. A numeric pre-release
// identifier must not have a leading zero; build identifiers may.
func checkIdentifiers(part string, prerelease bool) error {
	for id := range strings.SplitSeq(part, ".") {
		if id == "" {
			return errors.New("empty identifier")
		}
		for _, r := range id {
			if !isIdentifierRune(r) {
				return fmt.Errorf("identifier %q has a character outside [0-9A-Za-z-]", id)
			}
		}
		if prerelease && len(id) > 1 && id[0] == '0' && isDigits(id) {
			return fmt.Errorf("numeric identifier %q has a leading zero", id)
		}
	}
	return nil
}

func isIdentifierRune(r rune) bool {
	return r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
