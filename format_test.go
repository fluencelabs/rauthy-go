package rauthy_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	rauthy "github.com/fluencelabs/rauthy-go"
)

func TestSecretFormatting(t *testing.T) {
	t.Parallel()
	secret := "SYNTHETIC_CREDENTIAL_42"
	scim := &rauthy.ScimClientRequestResponse{BearerToken: rauthy.NewSecret(secret)}
	client, err := rauthy.New("https://auth.example.com", secret)
	if err != nil {
		t.Fatal(err)
	}
	values := []any{
		reflect.ValueOf(client).Elem().Interface(), client,
		*scim, scim,
		rauthy.ClientResponse{Scim: scim},
		rauthy.UpdateClientRequest{Scim: scim},
		rauthy.ClientSecretResponse{Secret: new(rauthy.NewSecret(secret))},
		rauthy.PamHostSecretResponse{Secret: rauthy.NewSecret(secret)},
		rauthy.AuthProviderRequest{ClientSecret: new(rauthy.NewSecret(secret))},
		rauthy.AuthProviderResponse{ClientSecret: new(rauthy.NewSecret(secret))},
		rauthy.UpdateUserRequest{Password: new(rauthy.NewSecret(secret))},
	}
	for _, value := range values {
		assertSecretFormatting(t, value, secret)
		// Client has no JSON wire representation. Every credential-bearing DTO must
		// still carry the original credential when marshalled.
		if _, ok := value.(rauthy.Client); ok {
			continue
		}
		if _, ok := value.(*rauthy.Client); ok {
			continue
		}
		raw, marshalErr := json.Marshal(value)
		if marshalErr != nil || !strings.Contains(string(raw), secret) {
			t.Errorf("%T lost wire credential: %s (%v)", value, raw, marshalErr)
		}
	}
}

func TestURLConfigurationDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()
	secret := "SYNTHETIC_CREDENTIAL_42"
	for _, raw := range []string{
		"https://auth.example.com/?token=" + secret,
		"https://auth.example.com/#" + secret,
		"ftp://auth.example.com/?token=" + secret,
		"https://auth.example.com/?token=%zz" + secret,
		"https://" + secret + "%zz.example.com",
	} {
		_, err := rauthy.New(raw, "key")
		if err == nil || strings.Contains(fmt.Sprint(err), secret) {
			t.Errorf("unsafe configuration error: %v", err)
		}
	}
}
