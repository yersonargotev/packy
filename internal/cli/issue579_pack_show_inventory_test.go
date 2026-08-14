package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPackShowHumanRendersDeterministicDescriptiveInventory(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Env: MapEnv{
		"HOME":                t.TempDir(),
		"XDG_CONFIG_HOME":     filepath.Join(t.TempDir(), "xdg"),
		"PATH":                "",
		"PACKY_SKILLS_SOURCE": filepath.Join(root, "bundle", "skills"),
	}}

	first, err := executeCommand(t, NewRootCommand(opts), "show", "engram")
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeCommand(t, NewRootCommand(opts), "show", "engram")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("pack show output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	want := "Resource: skill:engram-memory-cli — Uses Engram project memory safely through the CLI; role=operational dependencies=none notices=notice:mit"
	if !strings.Contains(first, want) {
		t.Fatalf("pack show output lacks descriptive inventory line %q:\n%s", want, first)
	}
	if strings.Contains(first, "Resource: skill:engram-memory-cli role=") {
		t.Fatalf("pack show retained the description-less resource graph line:\n%s", first)
	}
}

func TestPackShowJSONV5IncludesDescriptiveInventory(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Env: MapEnv{
		"HOME":                t.TempDir(),
		"XDG_CONFIG_HOME":     filepath.Join(t.TempDir(), "xdg"),
		"PATH":                "",
		"PACKY_SKILLS_SOURCE": filepath.Join(root, "bundle", "skills"),
	}}

	output, err := executeCommand(t, NewRootCommand(opts), "show", "engram", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		SchemaVersion     int                                  `json:"schema_version"`
		ResourceInventory []capabilitypack.DescriptiveResource `json:"resource_inventory"`
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 5 {
		t.Fatalf("schema version = %d, want 5", document.SchemaVersion)
	}
	if len(document.ResourceInventory) != 2 {
		t.Fatalf("resource inventory = %#v", document.ResourceInventory)
	}
	first := document.ResourceInventory[0]
	if first.Resource.String() != "notice:mit" ||
		first.Description != "Preserve the upstream Engram MIT license and attribution" ||
		first.Role != capabilitypack.ResourceInventoryRoleNotice {
		t.Fatalf("first resource inventory entry = %#v", first)
	}
	second := document.ResourceInventory[1]
	if second.Resource.String() != "skill:engram-memory-cli" ||
		second.Description != "Uses Engram project memory safely through the CLI" ||
		second.Role != capabilitypack.ResourceInventoryRoleOperational {
		t.Fatalf("second resource inventory entry = %#v", second)
	}
}
