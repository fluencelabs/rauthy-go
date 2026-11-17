package rauthy_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	rauthy "github.com/fluencelabs/rauthy-go"
)

func TestSecretJSON(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "SYNTHETIC_CREDENTIAL", "quote\"\n"} {
		secret := rauthy.NewSecret(value)
		raw, err := json.Marshal(secret)
		want, _ := json.Marshal(value)
		if err != nil || string(raw) != string(want) {
			t.Fatalf("wire mismatch: %s (%v)", raw, err)
		}
		var decoded rauthy.Secret
		if decodeErr := json.Unmarshal(raw, &decoded); decodeErr != nil || decoded.Reveal() != value {
			t.Fatalf("round trip failed: %v", decodeErr)
		}
	}
	var zero rauthy.Secret
	raw, err := json.Marshal(zero)
	if err != nil || string(raw) != `""` || zero.Reveal() != "" {
		t.Fatalf("zero secret: %s (%v)", raw, err)
	}
	original := rauthy.NewSecret("SYNTHETIC_ORIGINAL")
	copyOfSecret := original
	if decodeErr := json.Unmarshal([]byte(`"SYNTHETIC_NEW"`), &copyOfSecret); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if original.Reveal() != "SYNTHETIC_ORIGINAL" || copyOfSecret.Reveal() != "SYNTHETIC_NEW" {
		t.Fatal("decoding changed a shared copy")
	}
	if decodeErr := json.Unmarshal(
		[]byte(`123`),
		&copyOfSecret,
	); decodeErr == nil ||
		copyOfSecret.Reveal() != "SYNTHETIC_NEW" {
		t.Fatal("invalid JSON changed the secret")
	}
}

func TestOptionalSecretWireShape(t *testing.T) {
	t.Parallel()
	for _, input := range []string{`{}`, `{"secret":null}`, `{"secret":""}`, `{"secret":"SYNTHETIC"}`} {
		var got rauthy.ClientSecretResponse
		if decodeErr := json.Unmarshal([]byte(input), &got); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if decodeErr := json.Unmarshal(raw, &fields); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if input == `{}` || input == `{"secret":null}` {
			if got.Secret != nil || fields["secret"] != nil {
				t.Fatalf("absent or null secret was not omitted: %s", raw)
			}
		} else {
			var want map[string]json.RawMessage
			_ = json.Unmarshal([]byte(input), &want)
			if got.Secret == nil || string(fields["secret"]) != string(want["secret"]) {
				t.Fatalf("present secret changed: %s", raw)
			}
		}
	}
}

func assertSecretFormatting(t *testing.T, value any, secret string) {
	t.Helper()
	ptr := reflect.New(reflect.TypeOf(value))
	ptr.Elem().Set(reflect.ValueOf(value))
	for _, form := range []any{value, ptr.Interface()} {
		for _, wrapped := range []any{form, struct{ Value any }{form}, struct{ value any }{form}} {
			for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
				if output := fmt.Sprintf(format, wrapped); strings.Contains(output, secret) {
					t.Errorf("%T leaks with %s", value, format)
				}
			}
		}
	}
}

// seedSecrets discovers Secret fields recursively, including container DTOs.
func seedSecrets(value reflect.Value, secret string) int {
	if value.Type() == reflect.TypeFor[rauthy.Secret]() {
		value.Set(reflect.ValueOf(rauthy.NewSecret(secret)))
		return 1
	}
	if value.Kind() == reflect.Pointer {
		child := reflect.New(value.Type().Elem())
		count := seedSecrets(child.Elem(), secret)
		if count > 0 {
			value.Set(child)
		}
		return count
	}
	count := 0
	if value.Kind() == reflect.Struct {
		for _, field := range value.Fields() {
			if field.CanSet() {
				count += seedSecrets(field, secret)
			}
		}
	}
	return count
}

func TestSecretDTOClassFormatting(t *testing.T) {
	t.Parallel()
	const secret = "SYNTHETIC_CLASS_CREDENTIAL"
	tested := map[string]bool{"Client": true, "Secret": true}
	assertSecretFormatting(t, rauthy.NewSecret(secret), secret)
	client, err := rauthy.New("https://user:"+secret+"@auth.example.com", secret)
	if err != nil {
		t.Fatal(err)
	}
	assertSecretFormatting(t, reflect.ValueOf(client).Elem().Interface(), secret)
	for _, entry := range append(coveredTypes(), knownGaps()...) {
		value := reflect.New(reflect.TypeOf(entry.value)).Elem()
		if seedSecrets(value, secret) == 0 {
			continue
		}
		tested[value.Type().Name()] = true
		assertSecretFormatting(t, value.Interface(), secret)
		raw, marshalErr := json.Marshal(value.Interface())
		if marshalErr != nil || !strings.Contains(string(raw), secret) {
			t.Errorf("%s lost its wire credential: %v", value.Type(), marshalErr)
		}
	}
	for name := range secretBearingStructs(t) {
		if !tested[name] {
			t.Errorf("secret-bearing type %s has no formatting probe", name)
		}
	}
}

// A credential in the base URL path must not reach a transport error.
func TestTransportErrorOmitsBaseURL(t *testing.T) {
	t.Parallel()
	const token = "SYNTHETICRESETTOKENSYNTHETICRESETTOKEN"
	c, err := rauthy.New("https://auth.invalid/auth/v1/users/u1/reset/"+token, "key$secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.GetClient(ctx, "app")
	if err == nil {
		t.Fatal("GetClient with a canceled context succeeded")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("transport error repeats the base URL: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error %v lost its cause", err)
	}
}
