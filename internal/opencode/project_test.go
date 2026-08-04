package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestProjectInspectionRepresentsEverySupportedOpenCodeResourceKind(t *testing.T) {
	root := t.TempDir()
	bundle, project := filepath.Join(root, "bundle"), filepath.Join(root, "project")
	for path, content := range map[string]string{
		"skills/one/SKILL.md": "# One\n", "instructions/one.md": "Project guidance\n", "agents/one.md": "Agent prompt\n",
		"commands/one.md": "Command prompt\n", "assets/one.md": "Reference\n",
	} {
		full := filepath.Join(bundle, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	binding := func(projection, name string) []capabilitypack.Binding {
		return []capabilitypack.Binding{{Surface: capabilitypack.SurfaceOpenCode, Projection: projection, Name: name, Mode: "native", Sharing: "exclusive"}}
	}
	pack := capabilitypack.Pack{ID: "complete", Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "one", Source: "skills/one", Bindings: binding("skill", "one")},
		{Kind: "instruction", ID: "one", Source: "instructions/one.md", Bindings: binding("instruction", "one")},
		{Kind: "agent", ID: "one", Source: "agents/one.md", Bindings: binding("agent", "one")},
		{Kind: "command", ID: "one", Source: "commands/one.md", Bindings: binding("command", "one")},
		{Kind: "mcp_server", ID: "one", Command: "server", Args: []string{"--stdio"}, Bindings: binding("mcp_server", "one")},
		{Kind: "lifecycle", ID: "one", Bindings: binding("lifecycle", "one")},
		{Kind: "asset", ID: "one", Source: "assets/one.md"},
	}}
	adapter := NewSurfaceAdapter(bundle, "", "", "")
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Unrepresentable) != 0 || len(inspection.Projections) != 7 {
		t.Fatalf("inspection = %#v", inspection)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range inspection.Projections {
		actions = append(actions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".agents/skills/one/SKILL.md", "AGENTS.md", ".opencode/agents/one.md", ".opencode/commands/one.md", "opencode.json", ".opencode/packy-hooks/one.json", ".opencode/assets/one/one.md"} {
		if _, err := os.Stat(filepath.Join(project, relative)); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
}
