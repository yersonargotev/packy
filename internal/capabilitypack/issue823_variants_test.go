package capabilitypack

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue823AllKindVariantRestrictionsAndNonTargetGraphs(t *testing.T) {
	fixture := testsupport.AllKindVariants("all-kind-validation")
	root := t.TempDir()
	if err := fixture.WriteCatalog(root); err != nil {
		t.Fatal(err)
	}
	packRoot := filepath.Join(root, "packs", fixture.ID())
	pack, err := LoadCurrentManifest(filepath.Join(packRoot, "pack.json"), packRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, kind, want string
		mutate           func(*ResourceVariant)
	}{
		{"agent authority", "agent", "permissions", func(v *ResourceVariant) { permissions := []string{"arbitrary-authority"}; v.Permissions = &permissions }},
		{"command arguments", "command", "arguments", func(v *ResourceVariant) { v.Arguments = &CommandArguments{Mode: "execute-shell"} }},
		{"MCP file source", "mcp_server", "cannot have source", func(v *ResourceVariant) { source := "instructions/opencode.md"; v.Source = &source }},
		{"lifecycle file source", "lifecycle", "cannot have source", func(v *ResourceVariant) { source := "instructions/opencode.md"; v.Source = &source }},
		{"lifecycle executable override", "lifecycle", "only applicable", func(v *ResourceVariant) { command := "unreviewed"; v.Command = &command }},
		{"non-target dependency cycle", "command", "cycle", func(v *ResourceVariant) { requires := []string{"command:sample-command"}; v.Requires = &requires }},
		{"non-target missing asset", "instruction", "does not exist", func(v *ResourceVariant) { requires := []string{"asset:absent"}; v.Requires = &requires }},
		{"non-target unknown conflict", "agent", "conflict", func(v *ResourceVariant) { conflicts := []string{"agent:absent"}; v.Conflicts = &conflicts }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := clonePack(pack)
			for i := range candidate.Resources {
				resource := &candidate.Resources[i]
				if resource.Kind == test.kind {
					test.mutate(&resource.Variants[2])
				}
			}
			// Catalog admission checks OpenCode even when a caller would install Codex.
			if err := validateCurrentPack(candidate); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid OpenCode declaration accepted: %v", err)
			}
		})
	}
}
