package rauthy_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// These names match the credential pattern but describe public metadata.
func publicCredentialMetadata(typeName, fieldName string) bool {
	switch typeName + "." + fieldName {
	case "UpdateClientRequest.AccessTokenAlg", "UpdateClientRequest.IDTokenAlg",
		"ClientResponse.AccessTokenAlg", "ClientResponse.IDTokenAlg",
		"AuthProviderRequest.TokenEndpoint", "AuthProviderResponse.TokenEndpoint",
		"AuthProviderLookupResponse.TokenEndpoint":
		return true
	}
	return false
}

// secretBearingStructs scans every source file in the library package and
// rejects raw credentials. It also finds containers for the formatting probes.
//
//nolint:gocognit // Keep source scanning and container closure together in this test helper.
func secretBearingStructs(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	structs := map[string]*ast.StructType{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if file.Name.Name != "rauthy" {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok || !spec.Name.IsExported() {
				return true
			}
			if typ, isStruct := spec.Type.(*ast.StructType); isStruct {
				structs[spec.Name.Name] = typ
			}
			return true
		})
	}
	pattern := regexp.MustCompile(`(?i)secret|token|password|apikey|api_key|credential`)
	found := map[string]bool{}
	for name, typ := range structs {
		for _, field := range typ.Fields.List {
			fieldType := field.Type
			if pointer, ok := fieldType.(*ast.StarExpr); ok {
				fieldType = pointer.X
			}
			ident, ok := fieldType.(*ast.Ident)
			if !ok {
				continue
			}
			for _, fieldName := range field.Names {
				if ident.Name == "Secret" {
					found[name] = true
				}
				if pattern.MatchString(fieldName.Name) && ident.Name == "string" &&
					!publicCredentialMetadata(name, fieldName.Name) {
					t.Errorf("%s.%s stores a raw credential", name, fieldName.Name)
				}
			}
		}
	}
	// Propagate through container fields until the set is stable.
	for changed := true; changed; {
		changed = false
		for name, typ := range structs {
			if found[name] {
				continue
			}
			for _, field := range typ.Fields.List {
				fieldType := field.Type
				if pointer, ok := fieldType.(*ast.StarExpr); ok {
					fieldType = pointer.X
				}
				if ident, ok := fieldType.(*ast.Ident); ok && found[ident.Name] {
					found[name] = true
					changed = true
				}
			}
		}
	}
	return found
}

func TestSecretFieldsUseProtectedType(t *testing.T) { t.Parallel(); secretBearingStructs(t) }
