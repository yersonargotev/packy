package cataloglayout

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCatalogAndSchemaRejectRetiredOpenCodeContracts(t *testing.T) {
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "pack", "v3", "pack.schema.json"), "pack.schema.json")
	for _, test := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"surface", func(m map[string]any) { m["surfaces"] = []any{"agents", "opencode"} }},
		{"binding", func(m map[string]any) {
			m["resources"].([]any)[0].(map[string]any)["bindings"].([]any)[0].(map[string]any)["surface"] = "opencode"
		}},
		{"variant", func(m map[string]any) {
			m["resources"].([]any)[0].(map[string]any)["variants"] = []any{map[string]any{"surface": "opencode", "description": "Retired body"}}
		}},
		{"exclusion", func(m map[string]any) {
			m["resources"].([]any)[0].(map[string]any)["surface_exclusions"] = []any{map[string]any{"surface": "opencode", "reason": "Retired host"}}
		}},
		{"primary prompt capability", func(m map[string]any) {
			m["resources"].([]any)[0].(map[string]any)["bindings"].([]any)[0].(map[string]any)["capabilities"] = []any{map[string]any{"type": "opencode-primary-prompt"}}
		}},
		{"primary prompt payload", func(m map[string]any) {
			m["resources"].([]any)[0].(map[string]any)["bindings"].([]any)[0].(map[string]any)["capabilities"] = []any{map[string]any{
				"type": "project-instruction", "primary_prompt": map[string]any{"id": "guide", "source": "instructions/guide.md"},
			}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeCatalogPack(t, root, "current", "1.0.0", "Current guidance\n", "skills/current")
			writeCatalogPack(t, root, "retired", "1.0.0", "Retired guidance\n", "skills/retired")
			path := filepath.Join(root, "packs", "retired", "pack.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			test.edit(manifest)
			data, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(instance); err == nil {
				t.Fatal("authoritative schema accepted a retired contract")
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ValidateCatalogProject(context.Background(), root, "", nil); err == nil || !strings.Contains(err.Error(), "retired") {
				t.Fatalf("complete catalog accepted a retired declaration in an unrelated Pack: %v", err)
			}
		})
	}
}
