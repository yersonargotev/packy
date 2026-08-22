package managedpack

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestManagedPackSchemasAcceptCanonicalDocuments(t *testing.T) {
	root := filepath.Join("..", "..", "schemas", "managed-pack", "v1")
	for _, test := range []struct {
		name     string
		schema   string
		document []byte
	}{
		{"pack", "pack.schema.json", []byte(validManifest)},
		{"registry", "registry.schema.json", mustReadSchemaFixture(t, filepath.Join("..", "..", "managed-packs", "registry.json"))},
		{"admission record", "admission-record.schema.json", mustMarshalAdmission(t, validAdmissionRecord())},
	} {
		t.Run(test.name, func(t *testing.T) {
			compiler := jsonschema.NewCompiler()
			schemaPath := filepath.Join(root, test.schema)
			schemaData := mustReadSchemaFixture(t, schemaPath)
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
			if err != nil {
				t.Fatal(err)
			}
			if err := compiler.AddResource(test.schema, document); err != nil {
				t.Fatal(err)
			}
			compiled, err := compiler.Compile(test.schema)
			if err != nil {
				t.Fatal(err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(test.document))
			if err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(instance); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mustReadSchemaFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustMarshalAdmission(t *testing.T, record AdmissionRecord) []byte {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
