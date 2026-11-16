package rauthy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// APIError is a non-2xx response from the Rauthy API.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	// Message preserves server diagnostics, except input-reflecting errors are sanitized.
	// Non-JSON responses retain their raw body (truncated).
	Message string
	// ErrorCode is Rauthy's `error` field, e.g. "NotFound", when present.
	ErrorCode string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		if e.ErrorCode != "" {
			return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.ErrorCode)
		}
		return fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.StatusCode)
	}
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Message)
}

const maxErrBody = 512

func newAPIError(method, path string, status int, raw []byte) *APIError {
	e := &APIError{Method: method, Path: path, StatusCode: status}

	var envelope struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && (envelope.Message != "" || envelope.Error != "") {
		e.ErrorCode = envelope.Error
		e.Message = safeAPIMessage(status, envelope.Error, envelope.Message)
		return e
	}

	// A truncated JSON envelope must not fall back to printing its secret values.
	trimmed := strings.TrimSpace(string(raw))
	if json.Valid(raw) || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") ||
		strings.HasPrefix(trimmed, "\"") {
		return e
	}

	msg := strings.TrimSpace(string(raw))
	// Actix answers a body its extractor rejects with plain text such as
	// "Json deserialize error: unknown variant `…`", which quotes the input.
	if status == http.StatusBadRequest {
		msg, _, _ = strings.Cut(msg, ":")
	}
	if len(msg) > maxErrBody {
		cut := maxErrBody
		for cut > 0 && !utf8.RuneStart(msg[cut]) {
			cut--
		}
		msg = msg[:cut] + "…"
	}
	e.Message = msg
	return e
}

// ErrAmbiguousName is matched (errors.Is) by the error of a lookup by name that
// found more than one object under that name. It is deliberately not a 404:
// IsNotFound is false for it, so a caller that drops a resource it cannot find
// does not mistake a duplicate for an absence.
var ErrAmbiguousName = errors.New("name is ambiguous")

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsForbidden reports whether err is a 403 from the API. For this client a
// 403 almost always means the API key is missing an access-group right rather
// than that the object is off limits.
func IsForbidden(err error) bool { return hasStatus(err, http.StatusForbidden) }

// IsUnauthorized reports whether err is a 401 from the API.
func IsUnauthorized(err error) bool { return hasStatus(err, http.StatusUnauthorized) }

func hasStatus(err error, status int) bool {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr.StatusCode == status
	}
	return false
}

// notFoundError builds a synthetic 404 for endpoints Rauthy does not offer
// per-id: roles and groups can only be listed, so "not there" is something the
// client concludes rather than something the server says.
// The endpoints in question are list endpoints, so the method is always GET.
func notFoundError(path, message string) *APIError {
	return &APIError{
		Method:     http.MethodGet,
		Path:       path,
		StatusCode: http.StatusNotFound,
		Message:    message,
		ErrorCode:  "NotFound",
	}
}

// safeAPIMessage sanitizes classes that can echo rejected values, upstream bodies,
// or request URLs (which may contain credentials). Other diagnostics are retained
// regardless of status so operators can diagnose permissions and resource links.
// Sources in both Rauthy v0.36.2 and v0.37.2:
//   - src/error/src/error_impls.rs: validator/serde conversions and From<reqwest::Error>
//     (v0.36.2 lines 366-414, v0.37.2 lines 367-415).
//   - src/data/src/entity/auth_providers.rs: lookup (lines 522-538) and token
//     responses (v0.36.2 lines 756-792, v0.37.2 lines 754-790).
//   - Errors of type Scim (clients_scim.rs, scim_types.rs) and Connection (jwk.rs,
//     auth_provider_cust_impls.rs, http_client.rs, error_impls.rs and others) report
//     a failed call from Rauthy to another system and may carry its URL or response
//     body, so none of their text is kept.
func safeAPIMessage(status int, code, message string) string {
	switch code {
	case "Scim":
		return "SCIM request to the client's endpoint failed; the response is in the Rauthy log"
	case "Connection":
		return "Rauthy could not reach an external system; the details are in the Rauthy log"
	}
	if body, ok := strings.CutPrefix(message, "Payload validation error: "); ok {
		fields, valid := validationFields(body, "")
		if !valid {
			return ""
		}
		sort.Strings(fields)
		return strings.Join(fields, "; ")
	}
	switch {
	case strings.HasPrefix(message, "Payload deserialization error:"):
		return "Payload deserialization error"
	case strings.HasPrefix(message, "HTTP ") && strings.Contains(message, " when trying provider config lookup to "):
		return "Provider config lookup failed"
	case strings.HasPrefix(message, "Cannot send out HTTP request:"):
		return "Cannot send out HTTP request"
	case strings.HasPrefix(message, "The provider does not support the mandatory openid-configuration:"):
		return "The provider does not support the mandatory openid-configuration"
	case strings.HasPrefix(message, "HTTP ") && strings.Contains(message, " for upstream auth provider '"):
		return "Upstream auth provider token request failed"
	case strings.HasPrefix(message, "Deserializing /token response from auth provider "):
		return "Deserializing /token response from auth provider failed"
	case strings.HasPrefix(message, "/token request error:"):
		return "/token request error"
	case status == http.StatusBadRequest:
		// A rejected request is described by what was wrong with it, and the
		// detail after the colon is where Rauthy quotes it (parser errors of an
		// uploaded image, a remote client document and the like).
		class, _, _ := strings.Cut(message, ":")
		return urlPattern.ReplaceAllString(class, "<url>")
	default:
		return urlPattern.ReplaceAllString(message, "<url>")
	}
}

// urlPattern matches a URL inside a diagnostic. Rauthy names remote documents
// by URL (ephemeral clients, discovery), and a URL can carry credentials in its
// query, so diagnostics keep the rest of the text and drop the URL.
var urlPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://[^\s"<>]+`)

func validationFields(body, prefix string) ([]string, bool) {
	body, ok := debugWrapped(body, "ValidationErrors({", "})")
	if !ok {
		return nil, false
	}
	entries, ok := splitDebug(body)
	if !ok {
		return nil, false
	}
	var fields []string
	for _, entry := range entries {
		key, value, found := strings.Cut(entry, ": ")
		name, err := strconv.Unquote(key)
		if !found || err != nil || !debugIdentifier(name) {
			return nil, false
		}
		path := prefix + name
		if nested, isStruct := debugWrapped(value, "Struct(", ")"); isStruct {
			child, valid := validationFields(nested, path+".")
			if !valid {
				return nil, false
			}
			fields = append(fields, child...)
		} else {
			rules, valid := validationRules(value)
			if !valid {
				return nil, false
			}
			for _, rule := range rules {
				fields = append(fields, path+": "+rule)
			}
		}
	}
	return fields, len(fields) > 0
}

func validationRules(body string) ([]string, bool) {
	body, ok := debugWrapped(body, "Field([", "])")
	if !ok {
		return nil, false
	}
	entries, ok := splitDebug(body)
	if !ok {
		return nil, false
	}
	var rules []string
	for _, entry := range entries {
		value, wrapped := debugWrapped(entry, "ValidationError { ", " }")
		if !wrapped {
			return nil, false
		}
		parts, valid := splitDebug(value)
		if !valid || len(parts) != 3 || !strings.HasPrefix(parts[1], "message: ") ||
			!strings.HasPrefix(parts[2], "params: {") {
			return nil, false
		}
		code, found := strings.CutPrefix(parts[0], "code: ")
		rule, err := strconv.Unquote(code)
		if !found || err != nil || rule == "" {
			return nil, false
		}
		// Rauthy's custom regex validator uses the pattern itself as the code.
		if !debugIdentifier(rule) {
			rule = "regex"
		}
		rules = append(rules, rule)
	}
	return rules, len(rules) > 0
}

func debugWrapped(value, prefix, suffix string) (string, bool) {
	value, ok := strings.CutPrefix(value, prefix)
	if !ok {
		return "", false
	}
	return strings.CutSuffix(value, suffix)
}

func debugIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// splitDebug splits only at top-level commas. Quoted Rust debug strings and
// nested params are opaque, including escaped quotes and Unicode escapes.
func splitDebug(value string) ([]string, bool) {
	const maxDebugDepth = 64
	var parts []string
	var stack []byte
	start := 0
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch c {
		case '"':
			end, ok := debugStringEnd(value, i+1)
			if !ok {
				return nil, false
			}
			i = end
		case '{', '[', '(':
			stack = append(stack, c)
			if len(stack) > maxDebugDepth {
				return nil, false
			}
		case '}', ']', ')':
			if len(stack) == 0 || stack[len(stack)-1] != "{[("[strings.IndexByte("}])", c)] {
				return nil, false
			}
			stack = stack[:len(stack)-1]
		case ',':
			if len(stack) == 0 {
				parts = append(parts, strings.TrimSpace(value[start:i]))
				start = i + 1
			}
		}
	}
	if len(stack) != 0 {
		return nil, false
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts, true
}

func debugStringEnd(value string, start int) (int, bool) {
	for i := start; i < len(value); i++ {
		switch value[i] {
		case '\\':
			i++
		case '"':
			return i, true
		}
	}
	return 0, false
}
