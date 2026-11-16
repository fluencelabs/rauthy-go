package rauthy_test

import (
	"slices"
	"testing"

	rauthy "github.com/fluencelabs/rauthy-go"
)

func TestAdditionalConstraintUnion(t *testing.T) {
	t.Parallel()
	runConstraintCases(t, map[string]func(string) bool{
		"UserName":             rauthy.UserNamePattern().MatchString,
		"AttributeDescription": rauthy.AttributeDescriptionPattern().MatchString,
		"AuthProviderScope":    rauthy.AuthProviderScopePattern().MatchString,
		"City":                 rauthy.CityPattern().MatchString,
		"Street":               rauthy.StreetPattern().MatchString,
		"Timezone":             rauthy.TimezonePattern().MatchString,
		"IPAddress":            rauthy.ValidIPAddress,
		"Languages":            func(value string) bool { return slices.Contains(rauthy.Languages(), value) },
	})
}
