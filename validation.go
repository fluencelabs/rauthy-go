package rauthy

import (
	"net/netip"
	"regexp"
)

// Value constraints accept the union of Rauthy v0.36.2 and v0.37.2.
// A value accepted by either release must pass local checks. Version-specific
// restrictions and configured origin schemes remain the server's responsibility.
// Sources in both tags: src/common/src/regex.rs, src/api_types/src/cust_validation.rs,
// clients.rs, oidc.rs, users.rs, auth_providers.rs and generic.rs.
// The OpenAPI schema does not include the validator crate's constraints.

// Rust's Unicode White_Space class, unlike Go's ASCII-only \s.
const rustWhitespace = `\x{0009}-\x{000D}\x{0085}\p{Zs}\x{2028}\x{2029}`

const legacyNameLetters = `a-zA-Z0-9À-ɏ` +
	`\x{3041}-\x{3096}\x{30A0}-\x{30FF}\x{3400}-\x{4DB5}\x{4E00}-\x{9FCB}` +
	`\x{F900}-\x{FA6A}\x{2E80}-\x{2FD5}\x{FF66}-\x{FF9F}\x{FFA1}-\x{FFDC}\x{31F0}-\x{31FF}`

// Patterns from src/common/src/regex.rs and cust_validation.rs.
var (
	clientIDPattern   = regexp.MustCompile(`^[a-zA-Z0-9._\-]{2,256}$`)
	clientNamePattern = regexp.MustCompile(`^(?:[` + legacyNameLetters + rustWhitespace +
		`()\-]{2,128}|[\p{L}\p{M}\p{N}\p{Zs}()._-]{2,128})$`)
	uriPattern     = regexp.MustCompile(`^[a-zA-Z0-9,.:/_\-&?=~#!$'()*+%@]+$`)
	scopePattern   = regexp.MustCompile(`^[a-zA-Z0-9-_/,:*.]{2,64}$`)
	groupPattern   = regexp.MustCompile(`^[a-zA-Z0-9-_/,:*` + rustWhitespace + `]{2,64}$`)
	originPattern  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*://[a-z0-9.:-]+$`)
	contactPattern = regexp.MustCompile(`^[a-zA-Z0-9+.@/:-]{0,48}$`)
)

// ClientIDPattern returns RE_CLIENT_ID, the pattern of a client id. Rauthy 0.36
// narrowed it: `POST /clients` now rejects everything outside letters, digits,
// dot, underscore and hyphen, so ids that used to be legal (anything with `/`,
// `,`, `:` and friends) are refused by a live server.
func ClientIDPattern() *regexp.Regexp { return clientIDPattern }

// ClientNamePattern returns RE_CLIENT_NAME, the pattern of a client's display
// name, including the legacy CJK ranges and the newer Unicode classes.
func ClientNamePattern() *regexp.Regexp { return clientNamePattern }

// URIPattern returns RE_URI as applied to a client's redirect URIs and other
// URI-valued fields. Rauthy 0.37.2 has stricter redirect URI rules, but
// 0.36.2 accepts RE_URI, so local checks retain that broader pattern.
func URIPattern() *regexp.Regexp { return uriPattern }

// ScopePattern returns RE_ROLES_SCOPES, used for a client's scopes and
// default_scopes. Rauthy applies the same pattern to role and scope names.
func ScopePattern() *regexp.Regexp { return scopePattern }

// GroupPattern returns RE_GROUPS, used for a client's groups prefix. Rauthy
// applies the same pattern to group names.
func GroupPattern() *regexp.Regexp { return groupPattern }

// OriginPattern returns validate_vec_origin, the pattern of each allowed
// origin: scheme and host only, no path. The server checks whether the scheme
// is configured in additional_allowed_origin_schemes.
func OriginPattern() *regexp.Regexp { return originPattern }

// ContactPattern returns validate_vec_contact, the pattern of each client
// contact.
func ContactPattern() *regexp.Regexp { return contactPattern }

// Bounds from the #[validate(range(...))] attributes in
// src/api_types/src/clients.rs, inclusive.
const (
	// AuthCodeLifetimeMin and AuthCodeLifetimeMax bound auth_code_lifetime,
	// in seconds.
	AuthCodeLifetimeMin = 10
	AuthCodeLifetimeMax = 300

	// AccessTokenLifetimeMin and AccessTokenLifetimeMax bound
	// access_token_lifetime, in seconds.
	AccessTokenLifetimeMin = 10
	AccessTokenLifetimeMax = 86400

	// CacheCurrentHoursMin and CacheCurrentHoursMax bound cache_current_hours
	// on a client secret rotation: how long the previous secret stays valid.
	CacheCurrentHoursMin = 1
	CacheCurrentHoursMax = 24
)

// SigningAlgs returns the JWK signing algorithms Rauthy accepts for a client's
// access and id tokens (JwkKeyPairAlg). Each call returns a fresh slice.
func SigningAlgs() []string {
	return []string{"RS256", "RS384", "RS512", "EdDSA", "Ed25519"}
}

// GrantTypes returns the OAuth flows Rauthy accepts in a client's
// flows_enabled (validate_vec_grant_types). Rauthy rejects an empty list. Each
// call returns a fresh slice.
func GrantTypes() []string {
	return []string{
		"authorization_code",
		"client_credentials",
		"urn:ietf:params:oauth:grant-type:device_code",
		"urn:ietf:params:oauth:grant-type:token-exchange",
		"password",
		"refresh_token",
	}
}

// ChallengeMethods returns the PKCE challenge methods Rauthy accepts in a
// client's challenges (RE_CODE_CHALLENGE_METHOD). Rauthy rejects an empty list
// when the field is present. Each call returns a fresh slice.
func ChallengeMethods() []string {
	return []string{"plain", "S256"}
}

var (
	userNamePattern = regexp.MustCompile(`^(?:[` + legacyNameLetters + rustWhitespace +
		`\-']{1,32}|[\p{L}\p{M}\p{N}\p{Zs}'.-]{1,32})$`)
	attributeDescriptionPattern = regexp.MustCompile(`^[a-zA-Z0-9-_/` + rustWhitespace + `]{0,128}$`)
	authProviderScopePattern    = regexp.MustCompile(`^[a-zA-Z0-9\-_/:*.` + rustWhitespace + `]{0,512}$`)
	cityPattern                 = regexp.MustCompile(`^[a-zA-Z0-9À-ÿ\-` + rustWhitespace + `]{0,48}$`)
	streetPattern               = regexp.MustCompile(`^[a-zA-Z0-9À-ÿ.\-` + rustWhitespace + `]{0,48}$`)
	timezonePattern             = regexp.MustCompile(`^[a-zA-Z0-9_+\-/]{1,48}$`)
)

// UserNamePattern returns RE_USER_NAME for given and family names.
func UserNamePattern() *regexp.Regexp { return userNamePattern }

// AttributeDescriptionPattern returns RE_ATTR_DESC for user attribute descriptions.
func AttributeDescriptionPattern() *regexp.Regexp { return attributeDescriptionPattern }

// AuthProviderScopePattern returns RE_SCOPE_SPACE for the complete, space-joined scope string.
func AuthProviderScopePattern() *regexp.Regexp { return authProviderScopePattern }

// CityPattern returns RE_CITY for profile city and country values.
func CityPattern() *regexp.Regexp { return cityPattern }

// StreetPattern returns RE_STREET for profile street values.
func StreetPattern() *regexp.Regexp { return streetPattern }

// TimezonePattern checks timezone name syntax, including IANA aliases without a slash.
// Rauthy resolves the name against its own timezone database.
func TimezonePattern() *regexp.Regexp { return timezonePattern }

// Languages returns the supported user language codes. Each call returns a fresh slice.
func Languages() []string {
	return []string{"de", "en", "es", "fr", "ja", "ko", "nb", "nl", "pt", "ru", "uk", "zhhans"}
}

// ValidIPAddress accepts a bare IPv4 or IPv6 address, as Rust's IpAddr does.
// Scoped IPv6 addresses (zone identifiers) are not accepted by Rauthy.
func ValidIPAddress(value string) bool {
	addr, err := netip.ParseAddr(value)
	return err == nil && addr.Zone() == ""
}
