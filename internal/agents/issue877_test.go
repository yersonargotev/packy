package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestRequiredCodexRoleNeedsExplicitClientAndSupportedScope(t *testing.T) {
	root := t.TempDir()
	pack := reviewedRolePack(t, root)
	roles := filepath.Join(root, "home", ".codex", "agents")
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "home", ".agents", "skills"), roles)
	for _, client := range []string{"", "claude"} {
		if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, Client: client}); err == nil {
			t.Fatalf("accepted client %q", client)
		}
		if _, err := os.Stat(filepath.Dir(roles)); !os.IsNotExist(err) {
			t.Fatalf("failed preview mutated client state: %v", err)
		}
	}
	project, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, Client: "codex", ProjectRoot: filepath.Join(root, "project")})
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Unrepresentable) != 1 || len(project.Projections) != 0 {
		t.Fatalf("unsupported project role: %+v", project)
	}
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, Client: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 1 {
		t.Fatalf("role projection: %+v", inspection)
	}
	role := inspection.Projections[0]
	if role.Action.Target != filepath.Join(roles, "reviewer.toml") || !strings.Contains(role.Action.Description, "Codex") || !strings.Contains(role.Action.Content, `developer_instructions = `) {
		t.Fatalf("native role: %+v", role)
	}
	if inspection.Readiness.AuthorizationObserved || inspection.Readiness.UsabilityObserved {
		t.Fatal("role file claimed runtime readiness")
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{role.Action}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(roles), "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("role configured runtime: %v", err)
	}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ResidualOwnership: []capabilitypack.ProjectionOwnership{{ID: role.ID, Target: role.Action.Target, Surface: capabilitypack.SurfaceAgents, PackID: pack.ID, Fingerprint: role.DesiredFingerprint}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(removal.Projections) != 1 || removal.Projections[0].Action.Mode != capabilitypack.ProjectionDeleteTarget {
		t.Fatalf("role removal: %+v", removal)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removal.Projections[0].Action}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(role.Action.Target); !os.IsNotExist(err) {
		t.Fatalf("role remains: %v", err)
	}
}

func TestGlobalAgentsInstructionsFailWithoutWritingState(t *testing.T) {
	root := t.TempDir()
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "home", ".agents", "skills"), filepath.Join(root, "home", ".codex", "agents"))
	pack := capabilitypack.Pack{ID: "guide", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md"}}}
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil || len(inspection.Projections) != 0 || len(inspection.Unrepresentable) != 1 || !strings.Contains(inspection.Unrepresentable[0].Reason, "project scope") {
		t.Fatalf("global instructions: %+v %v", inspection, err)
	}
	if _, err := os.Stat(filepath.Join(root, "home")); !os.IsNotExist(err) {
		t.Fatalf("global instruction rejection wrote state: %v", err)
	}
}

func TestSharedAgentsSkillsPreserveMetadataAndExecutableSupportWithoutCodexState(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "guide")
	for name, body := range map[string]string{"SKILL.md": "---\nname: guide\ndescription: Guide\n---\n", "agents/openai.yaml": "interface:\n  display_name: Guide\n", "scripts/run.sh": "#!/bin/sh\nexit 0\n", "LICENSE": "reviewed notice\n"} {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0644)
		if name == "scripts/run.sh" {
			mode = 0755
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	pack := capabilitypack.Pack{ID: "guide", Resources: []capabilitypack.Resource{{Kind: "skill", ID: "guide", Source: "guide", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceAgents, Projection: "skill", Name: "guide"}}}}}
	home := filepath.Join(root, "home")
	adapter := NewSurfaceAdapter(root, filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".codex", "agents"))
	for _, project := range []string{"", filepath.Join(root, "project")} {
		inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
		if err != nil {
			t.Fatal(err)
		}
		if len(inspection.Projections) != 1 {
			t.Fatalf("shared skill: %+v", inspection)
		}
		action := inspection.Projections[0].Action
		if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"SKILL.md", "agents/openai.yaml", "scripts/run.sh", "LICENSE"} {
			got, err := os.ReadFile(filepath.Join(action.Target, name))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(source, name))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("changed %s", name)
			}
		}
		info, err := os.Stat(filepath.Join(action.Target, "scripts/run.sh"))
		if err != nil || info.Mode().Perm() != 0755 {
			t.Fatalf("executable support mode: %v %v", info, err)
		}
	}
	if err := os.Rename(source, filepath.Join(root, "retained-source-moved")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "guide", "agents", "openai.yaml")); err != nil {
		t.Fatalf("installed skill depended on catalog source path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("shared skill wrote Codex state: %v", err)
	}
}

func reviewedRolePack(t *testing.T, root string) capabilitypack.Pack {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "reviewer.md"), []byte("Review the proposed change.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return capabilitypack.Pack{ID: "roles", Resources: []capabilitypack.Resource{{Kind: "agent", ID: "reviewer", Source: "reviewer.md", Description: "Review changes", Mode: "subagent", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceAgents, Projection: "agent", Name: "reviewer", ClientProjection: &capabilitypack.ClientProjection{Client: "codex", Scopes: []string{"global"}}}}}}}
}

func TestProjectInstructionCompositionSurvivesProjectionOrdering(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"one.md": "first guidance", "two.md": "second guidance"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Native contribution names need not have the same ordering as resource IDs.
	pack := capabilitypack.Pack{ID: "guide", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "one", Source: "one.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceAgents, Projection: "instruction", Name: "one", Capabilities: []capabilitypack.SurfaceCapability{{Type: capabilitypack.SurfaceCapabilityProjectInstruction, ProjectInstruction: &capabilitypack.ProjectInstructionCapability{ID: "z-last", Source: "one.md"}}}}}},
		{Kind: "instruction", ID: "two", Source: "two.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceAgents, Projection: "instruction", Name: "two"}}},
	}}
	project := filepath.Join(root, "project")
	adapter := NewSurfaceAdapter(root, "", "")
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	actions := make([]capabilitypack.ProjectionAction, len(inspection.Projections))
	for i, p := range inspection.Projections {
		actions[i] = p.Action
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "first guidance") || !strings.Contains(string(data), "second guidance") {
		t.Fatalf("lost contribution: %s", data)
	}
}

func TestInactiveRoleObservationDisclosesClientRequirementWithoutProjection(t *testing.T) {
	root := t.TempDir()
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "home", ".agents", "skills"), filepath.Join(root, "home", ".codex", "agents"))
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: reviewedRolePack(t, root), ObservationOnly: true})
	if err != nil || len(inspection.Unrepresentable) != 1 || len(inspection.Projections) != 0 {
		t.Fatalf("inactive role observation: %+v %v", inspection, err)
	}
	if inspection.Readiness.AuthorizationObserved || inspection.Readiness.UsabilityObserved {
		t.Fatal("unselected client role claimed readiness")
	}
	if _, err := os.Stat(filepath.Join(root, "home")); !os.IsNotExist(err) {
		t.Fatalf("observation wrote client state: %v", err)
	}
}
