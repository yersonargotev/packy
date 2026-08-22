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

func TestPackSchemaRejectsDocumentsRejectedByTheContractValidator(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "schemas", "managed-pack", "v1", "pack.schema.json")
	compiled := compileSchema(t, schemaPath, "pack.schema.json")
	tests := []struct {
		name string
		edit func(map[string]any)
	}{
		{"retired field", func(manifest map[string]any) { manifest["exclusions"] = []any{} }},
		{"traversal", func(manifest map[string]any) { resource(manifest)["source"] = "../guide" }},
		{"skill without source", func(manifest map[string]any) { delete(resource(manifest), "source") }},
		{"capability without typed payload", func(manifest map[string]any) {
			binding := resource(manifest)["bindings"].([]any)[1].(map[string]any)
			binding["capabilities"] = []any{map[string]any{"type": "project-instruction"}}
		}},
		{"capability with wrong payload", func(manifest map[string]any) {
			binding := resource(manifest)["bindings"].([]any)[1].(map[string]any)
			binding["capabilities"] = []any{map[string]any{
				"type":           "project-instruction",
				"primary_prompt": map[string]any{"id": "guide", "source": "instructions/guide.md"},
			}}
		}},
		{"derived notice without coverage", func(manifest map[string]any) {
			notice := manifest["resources"].([]any)[0].(map[string]any)
			notice["origin"] = map[string]any{"id": "upstream", "path": "LICENSE", "relationship": "exact-copy"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var manifest map[string]any
			if err := json.Unmarshal([]byte(validManifest), &manifest); err != nil {
				t.Fatal(err)
			}
			test.edit(manifest)
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(instance); err == nil {
				t.Fatal("schema accepted an invalid Managed Pack document")
			}
		})
	}
}

func TestPackSchemaAcceptsOriginRootPath(t *testing.T) {
	var manifest map[string]any
	if err := json.Unmarshal([]byte(validManifest), &manifest); err != nil {
		t.Fatal(err)
	}
	resource(manifest)["origin"].(map[string]any)["path"] = "."
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiled := compileSchema(t, filepath.Join("..", "..", "schemas", "managed-pack", "v1", "pack.schema.json"), "pack.schema.json")
	if err := compiled.Validate(instance); err != nil {
		t.Fatal(err)
	}
}

func compileSchema(t *testing.T, path, resource string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(mustReadSchemaFixture(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource(resource, document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
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
