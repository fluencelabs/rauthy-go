package rauthy

import (
	"errors"
	"strings"
)

// checkPath refuses a request path, relative to the API root, that has an
// empty, "." or ".." segment. Every such segment comes from a caller-supplied
// identifier: url.PathEscape leaves "." and ".." as they are, and an empty
// identifier collapses the path onto its parent, so any hop that normalises
// the path would send the request to an endpoint other than the one the
// method names. Checking the finished path covers every method that puts an
// identifier into it, before anything is sent.
func checkPath(path string) error {
	for seg := range strings.SplitSeq(strings.TrimPrefix(path, "/"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return errors.New(`an identifier in the request path must not be empty, "." or ".."`)
		}
	}
	return nil
}
