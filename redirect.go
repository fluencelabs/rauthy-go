package rauthy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// redirectError is a redirect refused before there was a response to report:
// net/http parses the Location header before it asks CheckRedirect, and fails
// the whole exchange when the header does not parse.
type redirectError struct{ method, path string }

func (e *redirectError) Error() string {
	return fmt.Sprintf("%s %s: refused a redirect: its Location header does not parse", e.method, e.path)
}

// isUnparseableLocation recognises that net/http failure. Its error carries no
// type of its own, only this text, and it quotes the raw header, which is
// dropped here.
func isUnparseableLocation(err error) bool {
	return strings.Contains(err.Error(), "failed to parse Location header")
}

// redirectMessage tells the operator where a refused 3xx pointed, as scheme,
// host and path: userinfo and the query may carry credentials and are left
// out. The response body is not used, since it usually repeats the full
// Location. It returns "" for a status that is not a redirect.
func redirectMessage(status int, reqURL *url.URL, location string) string {
	if status < http.StatusMultipleChoices || status >= http.StatusBadRequest {
		return ""
	}
	if location == "" {
		return "redirect refused: the response has no Location header"
	}
	u, err := reqURL.Parse(location)
	if err != nil {
		return "redirect refused: its Location header does not parse"
	}
	target := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	return "redirect refused: the server pointed to " + target.String()
}
