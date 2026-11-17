package rauthy

import (
	"fmt"
	"strings"
)

const redactedCredential = "[REDACTED]"

// formatRedacted keeps the normal struct representation while replacing credentials.
// JSON encoding does not use fmt.Formatter, so wire values remain unchanged.
func formatRedacted(state fmt.State, verb rune, value any) {
	var format strings.Builder
	format.WriteByte('%')
	for _, flag := range "+#" {
		if state.Flag(int(flag)) {
			format.WriteRune(flag)
		}
	}
	format.WriteRune(verb)
	_, _ = fmt.Fprintf(state, format.String(), value)
}

// Format hides the API key and HTTP client authentication state.
func (c Client) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprint(state, "rauthy.Client{[REDACTED]}")
}

// Format hides credentials from diagnostic output.
func (v ScimClientRequestResponse) Format(state fmt.State, verb rune) {
	type plain ScimClientRequestResponse
	v.BearerToken = redactedCredential
	formatRedacted(state, verb, plain(v))
}

// Format hides credentials from diagnostic output.
func (v ClientSecretResponse) Format(state fmt.State, verb rune) {
	type plain ClientSecretResponse
	if v.Secret != nil {
		redacted := redactedCredential
		v.Secret = &redacted
	}
	formatRedacted(state, verb, plain(v))
}

// Format hides credentials from diagnostic output.
func (v AuthProviderRequest) Format(state fmt.State, verb rune) {
	type plain AuthProviderRequest
	if v.ClientSecret != nil {
		redacted := redactedCredential
		v.ClientSecret = &redacted
	}
	formatRedacted(state, verb, plain(v))
}

// Format hides credentials from diagnostic output.
func (v AuthProviderResponse) Format(state fmt.State, verb rune) {
	type plain AuthProviderResponse
	if v.ClientSecret != nil {
		redacted := redactedCredential
		v.ClientSecret = &redacted
	}
	formatRedacted(state, verb, plain(v))
}

// Format hides credentials from diagnostic output.
func (v PamHostSecretResponse) Format(state fmt.State, verb rune) {
	type plain PamHostSecretResponse
	v.Secret = redactedCredential
	formatRedacted(state, verb, plain(v))
}

// Format hides credentials from diagnostic output.
func (v UpdateUserRequest) Format(state fmt.State, verb rune) {
	type plain UpdateUserRequest
	if v.Password != nil {
		redacted := redactedCredential
		v.Password = &redacted
	}
	formatRedacted(state, verb, plain(v))
}
