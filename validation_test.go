package rauthy_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	rauthy "github.com/fluencelabs/rauthy-go"
)

type constraintCase struct {
	ID        string `json:"id"`
	Value     string `json:"value"`
	Validator string `json:"validator"`
	Expected  bool   `json:"expected"`
	Responses map[string]struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	} `json:"responses"`
}

func TestClientConstraintUnion(t *testing.T) {
	t.Parallel()
	runConstraintCases(t, map[string]func(string) bool{
		"ClientName":  rauthy.ClientNamePattern().MatchString,
		"Contact":     rauthy.ContactPattern().MatchString,
		"URI":         rauthy.URIPattern().MatchString,
		"Group":       rauthy.GroupPattern().MatchString,
		"Scope":       rauthy.ScopePattern().MatchString,
		"Origin":      rauthy.OriginPattern().MatchString,
		"GrantTypes":  func(value string) bool { return slices.Contains(rauthy.GrantTypes(), value) },
		"SigningAlgs": func(value string) bool { return slices.Contains(rauthy.SigningAlgs(), value) },
	})
}

func runConstraintCases(t *testing.T, validators map[string]func(string) bool) {
	t.Helper()
	raw, err := os.ReadFile("testdata/constraints-live.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []constraintCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		validate, ok := validators[tc.Validator]
		if !ok {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			t.Parallel()
			accepted := acceptedByEitherVersion(t, tc)
			if accepted != tc.Expected {
				t.Fatalf("live union = %v, expected %v", accepted, tc.Expected)
			}
			if got := validate(tc.Value); got != tc.Expected {
				t.Errorf("%s(%q) = %v, want %v from live union", tc.Validator, tc.Value, got, tc.Expected)
			}
		})
	}
}

func acceptedByEitherVersion(t *testing.T, tc constraintCase) bool {
	t.Helper()
	accepted := false
	for _, version := range []string{"0.36.2", "0.37.2"} {
		response, present := tc.Responses[version]
		if !present {
			t.Fatalf("missing live response for %s", version)
		}
		success := response.Status >= 200 && response.Status < 300
		if !success && response.Status != 400 && response.Status != 422 {
			t.Fatalf("unexpected live status %d: %s", response.Status, response.Body)
		}
		accepted = accepted || success
	}
	return accepted
}
