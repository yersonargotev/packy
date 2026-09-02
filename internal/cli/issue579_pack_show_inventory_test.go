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
	asset := "Resource: asset:protocol-contract-v1 — Machine-verifiable Engram Protocol contract v1 compatibility metadata; role=supporting dependencies=none notices=none"
	if !strings.Contains(first, asset) {
		t.Fatalf("pack show output lacks descriptive inventory line %q:\n%s", asset, first)
	}
	if strings.Contains(first, "Resource: skill:engram-memory-cli role=") {
		t.Fatalf("pack show retained the description-less resource graph line:\n%s", first)
	}
}

func TestPackShowJSONV6IncludesDescriptiveInventory(t *testing.T) {
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
	if document.SchemaVersion != 6 {
		t.Fatalf("schema version = %d, want 6", document.SchemaVersion)
	}
	type expectedResource struct {
		identity    string
		description string
		role        capabilitypack.ResourceInventoryRole
	}
	expected := []expectedResource{
		{
			identity:    "asset:protocol-contract-v1",
			description: "Machine-verifiable Engram Protocol contract v1 compatibility metadata",
			role:        capabilitypack.ResourceInventoryRoleSupporting,
		},
		{
			identity:    "notice:mit",
			description: "Preserve the upstream Engram MIT license and attribution",
			role:        capabilitypack.ResourceInventoryRoleNotice,
		},
		{
			identity:    "skill:engram-memory-cli",
			description: "Uses Engram project memory safely through the CLI",
			role:        capabilitypack.ResourceInventoryRoleOperational,
		},
	}
	if len(document.ResourceInventory) != len(expected) {
		t.Fatalf("resource inventory = %#v", document.ResourceInventory)
	}
	for i, want := range expected {
		resource := document.ResourceInventory[i]
		if resource.Resource.String() != want.identity || resource.Description != want.description || resource.Role != want.role {
			t.Fatalf("resource inventory entry %d = %#v", i, resource)
		}
	}
}
