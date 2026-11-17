package rauthy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLiveNotFoundResponses(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"0.36.2", "0.37.2"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile("testdata/live-notfound-" + version + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var responses []struct {
				Method       string `json:"method"`
				Path         string `json:"path"`
				Status       int    `json:"status"`
				Body         string `json:"body"`
				WantNotFound bool   `json:"want_not_found"`
			}
			if decodeErr := json.Unmarshal(raw, &responses); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if len(responses) == 0 {
				t.Fatal("empty response fixture")
			}
			for _, response := range responses {
				apiErr := newAPIError(response.Method, response.Path, response.Status, []byte(response.Body))
				if got := IsNotFound(apiErr); got != response.WantNotFound {
					t.Errorf("%s %s: IsNotFound(%v) = %t, want %t",
						response.Method, response.Path, apiErr, got, response.WantNotFound)
				}
			}
		})
	}
}

func TestAPIErrorRejectedValues(t *testing.T) {
	t.Parallel()
	secret := `SYNTHETIC"\\雪 params: {"fake": Field([ValidationError { code: "leak" }])}`
	field := func(name, code, value string) string {
		return fmt.Sprintf(
			`%q: Field([ValidationError { code: %q, message: None, params: {"value": String(%q)} }])`,
			name,
			code,
			value,
		)
	}
	validation := func(fields string) string { return "Payload validation error: ValidationErrors({" + fields + "})" }
	cases := []struct{ name, message, want string }{
		{
			"bearer token",
			validation(
				`"scim": Struct(ValidationErrors({` + field(
					"bearer_token",
					`[a-zA-Z0-9,.:/_-&?=~#!$'()*+%@]+$`,
					"Bearer SYNTHETIC",
				) + `}))`,
			),
			"scim.bearer_token: regex",
		},
		{
			"client secret",
			validation(field("client_secret", "length", strings.Repeat("S", 325))),
			"client_secret: length",
		},
		{
			"multiple nested fields",
			validation(
				field(
					"client_secret",
					"length",
					secret,
				) + `, "scim": Struct(ValidationErrors({` + field(
					"bearer_token",
					"regex",
					secret,
				) + `}))`,
			),
			"client_secret: length; scim.bearer_token: regex",
		},
		{
			"escaped value and params text",
			validation(field("client_secret", "length", secret)),
			"client_secret: length",
		},
		{"truncated validation", validation(field("client_secret", "length", secret))[:90], ""},
		{
			"serde",
			`Payload deserialization error: Error("invalid value: SYNTHETIC", line: 1, column: 2)`,
			"Payload deserialization error",
		},
		{
			"upstream",
			`HTTP https://SYNTHETIC.example when trying provider config lookup to 400 Bad Request: SYNTHETIC`,
			"Provider config lookup failed",
		},
		{
			"transport URL",
			`Cannot send out HTTP request: reqwest::Error { kind: Request, url: "https://example.com/?token=SYNTHETIC" }`,
			"Cannot send out HTTP request",
		},
		{
			"discovery decoding",
			`The provider does not support the mandatory openid-configuration: error decoding response body for url (https://example.com/?token=SYNTHETIC)`,
			"The provider does not support the mandatory openid-configuration",
		},
		{
			"token response body",
			"HTTP 400 during POST https://example.com/token for upstream auth provider 'test'\nSYNTHETIC",
			"Upstream auth provider token request failed",
		},
		{
			"token decoding",
			"Deserializing /token response from auth provider test: SYNTHETIC",
			"Deserializing /token response from auth provider failed",
		},
		{"token error description", "/token request error: invalid_request: SYNTHETIC", "/token request error"},
		{"trailing unparsed value", validation(field("client_secret", "length", secret)) + " SYNTHETIC", ""},
		{
			"invalid delimiters",
			`Payload validation error: ValidationErrors({"client_secret": Field([ValidationError { code: "length", message: None, params: {"value": String("SYNTHETIC")} ])})`,
			"",
		},
		{
			"unclosed string",
			`Payload validation error: ValidationErrors({"client_secret": Field([ValidationError { code: "length", message: None, params: {"value": String("SYNTHETIC)} }])})`,
			"",
		},
		{
			"excessive nesting",
			validation(
				field("client_secret", "length", "SYNTHETIC") + strings.Repeat("(", 65) + strings.Repeat(")", 65),
			),
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(map[string]string{"error": "BadRequest", "message": tc.message})
			if err != nil {
				t.Fatal(err)
			}
			got := newAPIError(http.MethodPut, "/clients/test", http.StatusBadRequest, raw)
			assertSafeAPIError(t, got, tc.want)
		})
	}
}

func TestAPIErrorBodyFallback(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"error":"BadRequest","message":"SYNTHETIC`, `{"unexpected":"SYNTHETIC"}`, `["SYNTHETIC"]`, `"SYNTHETIC"`} {
		got := newAPIError(http.MethodPost, "/clients", http.StatusBadRequest, []byte(raw))
		if got.Message != "" {
			t.Errorf("JSON fallback = %q", got.Message)
		}
	}
	got := newAPIError(http.MethodGet, "/clients", http.StatusBadGateway, []byte("<html>proxy unavailable</html>"))
	if got.Message != "<html>proxy unavailable</html>" {
		t.Fatal(got)
	}
	got = newAPIError(http.MethodGet, "/clients", http.StatusBadGateway, []byte(strings.Repeat("x", 600)))
	if got.Message != strings.Repeat("x", maxErrBody)+"…" {
		t.Fatal(got)
	}
}

func assertSafeAPIError(t *testing.T, got *APIError, want string) {
	t.Helper()
	if got.Message != want {
		t.Errorf("Message = %q, want %q", got.Message, want)
	}
	if got.ErrorCode != "BadRequest" {
		t.Errorf("ErrorCode = %q", got.ErrorCode)
	}
	if want == "" && !strings.Contains(got.Error(), "PUT /clients/test: HTTP 400: BadRequest") {
		t.Errorf("lost context: %v", got)
	}
	for _, output := range []string{got.Message, got.Error(), fmt.Errorf("wrapped: %w", got).Error()} {
		if strings.Contains(output, "SYNTHETIC") || strings.Contains(output, strings.Repeat("S", 325)) {
			t.Errorf("rejected value in %q", output)
		}
		if !strings.Contains(output, want) {
			t.Errorf("missing diagnostic %q in %q", want, output)
		}
	}
}

func TestAPIErrorPreservesOrdinaryMessages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status        int
		code, message string
	}{
		{http.StatusForbidden, "Forbidden", "API key is missing Clients:Delete"},
		{http.StatusNotFound, "NotFound", "Client 'gone' not found"},
		{http.StatusConflict, "Conflict", "A group with this name already exists"},
		{http.StatusBadRequest, "BadRequest", "Cannot delete group with assigned hosts"},
		{http.StatusBadGateway, "Internal", "upstream is down"},
		{http.StatusBadRequest, "BadRequest", "SYNTHETIC ordinary diagnostic"},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(map[string]string{"error": tc.code, "message": tc.message})
			if err != nil {
				t.Fatal(err)
			}
			got := newAPIError(http.MethodDelete, "/resource", tc.status, raw)
			if got.Message != tc.message || got.ErrorCode != tc.code || got.StatusCode != tc.status {
				t.Errorf("unexpected APIError: %+v", got)
			}
			for _, output := range []string{got.Error(), fmt.Errorf("wrapped: %w", got).Error()} {
				if !strings.Contains(output, tc.message) {
					t.Errorf("lost diagnostic in %q", output)
				}
			}
		})
	}
}

func TestAPIErrorWithholdsExternalResponses(t *testing.T) {
	t.Parallel()
	const (
		scim       = "SCIM request to the client's endpoint failed; the response is in the Rauthy log"
		connection = "Rauthy could not reach an external system; the details are in the Rauthy log"
	)
	for _, tc := range []struct{ code, message, want string }{
		{"Scim", `Error during sync groups: ScimError { status: 401, detail: Some("Bearer SYNTHETIC") }`, scim},
		{"Scim", `SCIM GET /Groups error: reqwest::Error { kind: Request, url: "https://scim.example/?token=SYNTHETIC" }`, scim},
		{"Connection", "Error during User E-Mails lookup: SYNTHETIC upstream body", connection},
		{"Connection", "Error connecting to https://jwks.example/?token=SYNTHETIC", connection},
	} {
		raw, err := json.Marshal(map[string]string{"error": tc.code, "message": tc.message})
		if err != nil {
			t.Fatal(err)
		}
		got := newAPIError(http.MethodPut, "/clients/test", http.StatusBadRequest, raw)
		if got.ErrorCode != tc.code {
			t.Errorf("ErrorCode = %q, want %q", got.ErrorCode, tc.code)
		}
		want := tc.want
		for _, output := range []string{got.Message, got.Error(), fmt.Errorf("wrapped: %w", got).Error()} {
			if strings.Contains(output, "SYNTHETIC") || !strings.Contains(output, want) {
				t.Errorf("output %q: want %q without the upstream response", output, want)
			}
		}
	}
}

func TestAPIErrorDropsReflectedText(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(map[string]string{
		"error":   "BadRequest",
		"message": "client_id mismatch: https://app.example/client?token=SYNTHETIC != https://other.example/x?code=SYNTHETIC",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := newAPIError(http.MethodGet, "/clients/test", http.StatusConflict, raw)
	if want := "client_id mismatch: <url> != <url>"; got.Message != want {
		t.Errorf("Message = %q, want %q", got.Message, want)
	}
	raw, err = json.Marshal(
		map[string]string{"error": "NotFound", "message": "no document at https://app.example/a'SYNTHETIC"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got = newAPIError(
		http.MethodGet,
		"/clients/test",
		http.StatusNotFound,
		raw,
	); got.Message != "no document at <url>" {
		t.Errorf("Message = %q, want the URL masked up to its end", got.Message)
	}
	for _, tc := range []struct{ message, want string }{
		{"svg sanitization error: Reader(Error { kind: Syntax(\"Unexpected entity: SYNTHETIC\") })", "svg sanitization error"},
		{"Minimum password length is 12", "Minimum password length is 12"},
	} {
		raw, err = json.Marshal(map[string]string{"error": "BadRequest", "message": tc.message})
		if err != nil {
			t.Fatal(err)
		}
		if got = newAPIError(http.MethodPut, "/clients/test/logo", http.StatusBadRequest, raw); got.Message != tc.want {
			t.Errorf("Message = %q, want %q", got.Message, tc.want)
		}
	}

	body := "Json deserialize error: unknown variant `SYNTHETIC`, expected one of `custom`, `github`"
	got = newAPIError(http.MethodPost, "/providers/create", http.StatusBadRequest, []byte(body))
	if got.Message != "Json deserialize error" {
		t.Errorf("Message = %q, want the actix error class only", got.Message)
	}
	for _, output := range []string{got.Message, got.Error()} {
		if strings.Contains(output, "SYNTHETIC") {
			t.Errorf("rejected value in %q", output)
		}
	}
}

func TestAPIErrorTruncatesOnRuneBoundary(t *testing.T) {
	t.Parallel()
	got := newAPIError(
		http.MethodGet,
		"/clients/test",
		http.StatusForbidden,
		[]byte(strings.Repeat("a", maxErrBody-1)+"雪雪"),
	)
	if !utf8.ValidString(got.Message) {
		t.Errorf("Message is not valid UTF-8: %q", got.Message)
	}
}
