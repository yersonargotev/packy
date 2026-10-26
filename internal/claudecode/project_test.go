package claudecode

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestClaudeProjectUsesDeclaredCompositeSkillCapability(t *testing.T) {
	bundle, pack := claudeCompositeFixture(t)
	pack.ID = "synthetic-project"
	project := t.TempDir()
	adapter := NewSurfaceAdapter(bundle, NewCanonicalLayout(""), "", "", nil, nil)

	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	composite, err := claudeCompositeSkill(pack, pack.Resources[0], pack.Resources[0].Bindings[0], bundle)
	if err != nil {
		t.Fatal(err)
	}
	projectFingerprint, err := fingerprintCompositeFiles(composite.Files, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	var action capabilitypack.ProjectionAction
	for _, projection := range inspection.Projections {
		if projection.ID == "skill:example" {
			if projection.DesiredFingerprint != projectFingerprint || projection.Action.FileMode != 0o755 {
				t.Fatalf("project fingerprint = %q, mode = %o, want %q and 0755", projection.DesiredFingerprint, projection.Action.FileMode, projectFingerprint)
			}
			action = projection.Action
		}
	}
	if action.ID == "" {
		t.Fatal("composite skill projection missing")
	}
	action.PreviewOnly = false
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
		t.Fatal(err)
	}
	for _, file := range composite.Files {
		got, err := os.ReadFile(filepath.Join(action.Target, filepath.FromSlash(file.Path)))
		if err != nil || !bytes.Equal(got, file.Content) {
			t.Fatalf("project composite file %s = %q, err=%v", file.Path, got, err)
		}
	}
}

func TestClaudeProjectAdapterPreservesInstructionsAndForeignConfiguration(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(bundle, "instructions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bundle, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "instructions", "memory.md"), []byte("Use durable memory.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "assets", "checklist.md"), []byte("# Checklist\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("# Team instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), []byte("{\n  \"foreign\": true\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "portable", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "memory", Source: "instructions/memory.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "instruction", Name: "memory"}}},
		{Kind: "asset", ID: "checklist", Source: "assets/checklist.md"},
	}}
	adapter := NewSurfaceAdapter(bundle, NewCanonicalLayout(""), "", "", nil, nil)
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	actions := make([]capabilitypack.ProjectionAction, len(inspection.Projections))
	for i := range inspection.Projections {
		actions[i] = inspection.Projections[i].Action
		actions[i].PreviewOnly = false
	}
	if actionErr := adapter.ApplyProjections(context.Background(), actions); actionErr != nil {
		t.Fatal(actionErr)
	}
	instructions, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(instructions), "# Team instructions") || !strings.Contains(string(instructions), "pack:portable:memory") {
		t.Fatalf("instructions = %q, err=%v", instructions, err)
	}
	mcpBytes, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mcpBytes) != "{\n  \"foreign\": true\n}\n" {
		t.Fatalf("foreign config changed: %q", mcpBytes)
	}
	lock := capabilitypack.ProjectLockProposal{Bindings: []capabilitypack.LifecycleBinding{
		{Kind: "instruction", ID: "memory", Projection: "instruction", Name: "memory"},
	}}
	for _, projection := range inspection.Projections {
		relative, relErr := filepath.Rel(project, projection.Action.Target)
		if relErr != nil {
			t.Fatal(relErr)
		}
		lock.Projections = append(lock.Projections, capabilitypack.ProjectProjectionPlan{
			Resource: capabilitypack.ResourceIdentity{Kind: strings.SplitN(projection.ID, ":", 2)[0], ID: strings.SplitN(projection.ID, ":", 2)[1]},
			Target:   filepath.ToSlash(relative), DesiredFingerprint: projection.DesiredFingerprint, Command: projection.Action.Command, Args: projection.Action.Args,
			OwnerPack: "portable", Surface: capabilitypack.SurfaceClaude,
		})
	}
	installation := capabilitypack.ProjectInstallation{Manifest: capabilitypack.ProjectContractProposal{Packs: []capabilitypack.ProjectManifestPack{{ID: "portable", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceClaude}}}}, Lock: lock}
	tampered := installation
	tampered.Lock.Projections = append([]capabilitypack.ProjectProjectionPlan(nil), lock.Projections...)
	for i := range tampered.Lock.Projections {
		if tampered.Lock.Projections[i].Resource.Kind == "asset" {
			tampered.Lock.Projections[i].Target = ".claude/other/checklist.md"
		}
	}
	if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &tampered}); err == nil {
		t.Fatal("Claude locked inspection trusted a tampered asset target")
	}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionAbsent})
	if err != nil {
		t.Fatal(err)
	}
	removeActions := make([]capabilitypack.ProjectionAction, len(removal.Projections))
	for i := range removal.Projections {
		removeActions[i] = removal.Projections[i].Action
	}
	if actionErr := adapter.ApplyProjections(context.Background(), removeActions); actionErr != nil {
		t.Fatal(actionErr)
	}
	instructions, err = os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil || string(instructions) != "# Team instructions\n" {
		t.Fatalf("removed instructions = %q, err=%v", instructions, err)
	}
	mcpBytes, err = os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil || string(mcpBytes) != "{\n  \"foreign\": true\n}\n" {
		t.Fatalf("foreign config changed: %q, %v", mcpBytes, err)
	}

}
