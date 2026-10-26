package cataloglayout

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestClaudeContractSchemaPreservesRolesAndRejectsRetiredProjections(t *testing.T) {
	compiled := compileSchema(t, filepath.Join("..", "..", "schemas", "pack", "v3", "pack.schema.json"), "pack.schema.json")
	data, err := json.Marshal(testsupport.CapabilityRich("claude-contract").Manifest())
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		if resource["kind"] != "agent" {
			delete(resource, "tools")
			delete(resource, "permissions")
		}
	}
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := compiled.Validate(manifest); err != nil {
		t.Fatalf("supported Claude role contract: %v", err)
	}
	for _, kind := range []string{"mcp_server", "command", "lifecycle"} {
		t.Run(kind, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(data, &candidate); err != nil {
				t.Fatal(err)
			}
			candidate["resources"].([]any)[0].(map[string]any)["kind"] = kind
			if err := compiled.Validate(candidate); err == nil {
				t.Fatalf("retired %s admitted", kind)
			}
		})
	}
}
