package codex

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestProjectInstructionCapabilityBuildsCopiedTreeAndComposableInstructions(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	source := filepath.Join(bundle, "skills", "ask-matt")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("# Ask Matt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	instructionSource := filepath.Join(bundle, "instructions", "project.md")
	if err := os.MkdirAll(filepath.Dir(instructionSource), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instructionSource, []byte("Packy manages the selected Codex skill trees in .agents/skills.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Team guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "synthetic-one", Resources: []capabilitypack.Resource{{
		Kind: "skill", ID: "ask-matt", Source: "skills/ask-matt",
		Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Name: "ask-matt", Capabilities: []capabilitypack.SurfaceCapability{{Type: capabilitypack.SurfaceCapabilityProjectInstruction, ProjectInstruction: &capabilitypack.ProjectInstructionCapability{ID: "project-guidance", Source: "instructions/project.md"}}}}},
	}}}
	adapter := NewSurfaceAdapter(bundle, "", "")
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 2 {
		t.Fatalf("projections = %#v", inspection.Projections)
	}
	skill := findProjectProjection(t, inspection.Projections, "skill:ask-matt")
	if skill.ID != "skill:ask-matt" || skill.Action.Kind != capabilitypack.ActionCodexProjectSkillTree || skill.Action.Source != source || skill.Action.Version != skill.DesiredFingerprint || skill.Action.Target != filepath.Join(project, ".agents", "skills", "ask-matt") {
		t.Fatalf("copied skill projection = %#v", skill)
	}
	instruction := findProjectProjection(t, inspection.Projections, "instruction:project-guidance")
	if instruction.ID != "instruction:project-guidance" || instruction.Exists || instruction.AdapterProvenance != "codex-project/v1/shared-composable-instruction/missing" || !strings.Contains(instruction.Action.Content, "# Team guidance") || !strings.Contains(instruction.Action.Content, "<!-- packy:project:instruction:project-guidance:start -->") {
		t.Fatalf("composable instruction projection = %#v", instruction)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{skill.Action, instruction.Action}); err != nil {
		t.Fatal(err)
	}
	if fingerprint, err := localprojection.FingerprintExactTree(skill.Action.Target); err != nil || fingerprint != skill.DesiredFingerprint {
		t.Fatalf("copied skill fingerprint = %q, want %q: %v", fingerprint, skill.DesiredFingerprint, err)
	}
	content, err := os.ReadFile(instruction.Action.Target)
	if err != nil || string(content) != instruction.Action.Content {
		t.Fatalf("project AGENTS.md = %q, want %q: %v", content, instruction.Action.Content, err)
	}
}

func TestCodexProjectInspectionBuildsComposableMCPDefinition(t *testing.T) {
	project := t.TempDir()
	executable := filepath.Join(project, "bin", "memory")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(project, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("model = \"keep-me\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "engram", Resources: []capabilitypack.Resource{{
		Kind: "mcp_server", ID: "engram", Command: executable, Args: []string{"mcp", "--tools=agent"},
		Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "mcp_server", Name: "engram"}},
	}}}
	userConfig := filepath.Join(project, "home", ".codex", "config.toml")
	adapter := NewSurfaceAdapterWithConfig("", "", "", userConfig)
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	projection := findProjectProjection(t, inspection.Projections, "mcp_server:engram")
	if projection.Action.Kind != capabilitypack.ActionCodexMCPConfig || projection.Action.Target != config || !strings.Contains(projection.Action.Content, "model = \"keep-me\"") || !strings.Contains(projection.Action.Content, "[mcp_servers.engram]") {
		t.Fatalf("Codex project MCP projection = %#v", projection)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{projection.Action}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(config)
	if err != nil || !strings.Contains(string(content), "model = \"keep-me\"") || !strings.Contains(string(content), "[mcp_servers.engram]") {
		t.Fatalf("Codex project config = %q, %v", content, err)
	}
	installation := capabilitypack.ProjectInstallation{
		Manifest: capabilitypack.ProjectContractProposal{Packs: []capabilitypack.ProjectManifestPack{{ID: "engram", Version: "1.0.0", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceCodex}}}},
		Lock: capabilitypack.ProjectLockProposal{
			Sensitive: []capabilitypack.ProjectSensitiveDisclosure{
				{Category: capabilitypack.ProjectActivationMCP, Surface: capabilitypack.SurfaceCodex, Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "engram"}, Detail: "mcp_server"},
				{Category: capabilitypack.ProjectActivationExternalRequirements, Surface: capabilitypack.SurfaceCodex, Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "engram"}, Detail: "tool:memory"},
				{Category: capabilitypack.ProjectActivationTrust, Surface: capabilitypack.SurfaceCodex, Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "engram"}, Detail: "project-trust"},
			},
			Bindings:    []capabilitypack.LifecycleBinding{{Surface: capabilitypack.SurfaceCodex, Kind: "mcp_server", ID: "engram", Projection: "mcp_server", Name: "engram"}},
			Projections: []capabilitypack.ProjectProjectionPlan{{Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "engram"}, Target: ".codex/config.toml", Mode: "merge_marked_file", DesiredFingerprint: projection.DesiredFingerprint, OwnerPack: "engram", Surface: capabilitypack.SurfaceCodex, Command: executable}},
		},
	}
	locked, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionPresent})
	if err != nil || len(locked.Projections) != 1 || locked.Projections[0].ObservedFingerprint != projection.DesiredFingerprint || locked.Readiness.Authorized || len(locked.ProjectActivationActions) != 1 {
		t.Fatalf("locked Codex MCP inspection = %+v, %v", locked, err)
	}
	if err := adapter.ApplyProjections(context.Background(), locked.ProjectActivationActions); err != nil {
		t.Fatal(err)
	}
	locked, err = adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionPresent})
	if err != nil || !locked.Readiness.AuthorizationObserved || !locked.Readiness.Authorized || !locked.Readiness.UsabilityObserved || !locked.Readiness.Usable || len(locked.ProjectActivationActions) != 0 {
		t.Fatalf("trusted Codex MCP readiness = %+v, %v", locked.Readiness, err)
	}
	missingExecutable := executable + ".missing"
	if err := os.Rename(executable, missingExecutable); err != nil {
		t.Fatal(err)
	}
	withoutRequirement, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionPresent})
	if err != nil || !withoutRequirement.Readiness.Authorized || withoutRequirement.Readiness.Usable || len(withoutRequirement.Readiness.PendingHumanActions) == 0 {
		t.Fatalf("missing external requirement readiness = %+v, %v", withoutRequirement.Readiness, err)
	}
	contractOnly, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionPresent, ProjectContractOnly: true})
	if err != nil || len(contractOnly.Projections) != 1 || !reflect.DeepEqual(contractOnly.Readiness, capabilitypack.ReadinessObservation{}) || len(contractOnly.ProjectActivationActions) != 0 {
		t.Fatalf("portable contract inspection included runtime facts = %+v, %v", contractOnly, err)
	}
	if err := os.Rename(missingExecutable, executable); err != nil {
		t.Fatal(err)
	}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionAbsent})
	if err != nil || len(removal.Projections) != 1 || removal.Projections[0].Action.Mode != capabilitypack.ProjectionRemoveContent {
		t.Fatalf("Codex MCP removal inspection = %+v, %v", removal, err)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removal.Projections[0].Action}); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(config)
	if err != nil || strings.Contains(string(content), "mcp_servers.engram") || !strings.Contains(string(content), "model = \"keep-me\"") {
		t.Fatalf("Codex MCP removal did not preserve foreign config = %q, %v", content, err)
	}
}

func findProjectProjection(t *testing.T, projections []capabilitypack.ObservedProjection, id string) capabilitypack.ObservedProjection {
	t.Helper()
	for _, projection := range projections {
		if projection.ID == id {
			return projection
		}
	}
	t.Fatalf("projection %q not found in %#v", id, projections)
	return capabilitypack.ObservedProjection{}
}

func TestProjectInstructionCapabilityPreservesOnlyIntactContribution(t *testing.T) {
	project := t.TempDir()
	start, end := projectInstructionMarkers("project-guidance")
	desired := start + "\nPacky manages the selected Codex skill trees in .agents/skills.\n" + end
	for _, test := range []struct {
		name, content, provenance string
		exists                    bool
		writable                  bool
	}{
		{name: "missing preserves foreign content", content: "# Team guidance\n", provenance: "missing", writable: true},
		{name: "intact preserves surrounding content", content: "# Before\n" + desired + "\n# After\n", provenance: "intact", exists: true, writable: true},
		{name: "changed intact contribution proposes receipt-gated replacement", content: strings.Replace(desired, "skill trees", "different trees", 1), provenance: "changed", exists: true, writable: true},
		{name: "orphaned marker blocks", content: start + "\n# incomplete\n", provenance: "malformed", exists: true},
		{name: "duplicate markers block", content: desired + "\n" + desired, provenance: "ambiguous", exists: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			current, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
			if err != nil {
				t.Fatal(err)
			}
			projection := codexProjectInstructionProjection(capabilitypack.ResourceIdentity{Kind: "instruction", ID: "project-guidance"}, filepath.Join(project, "AGENTS.md"), "Packy manages the selected Codex skill trees in .agents/skills.", string(current), localprojection.FingerprintBytes(current))
			if projection.Exists != test.exists || projection.AdapterProvenance != "codex-project/v1/shared-composable-instruction/"+test.provenance {
				t.Fatalf("projection = %#v", projection)
			}
			if test.writable {
				want := "# Team guidance"
				if test.provenance == "changed" {
					if projection.Action.Content != desired {
						t.Fatalf("changed intact contribution replacement = %q, want %q", projection.Action.Content, desired)
					}
				} else if test.provenance == "intact" {
					if !strings.Contains(projection.Action.Content, "# Before") || !strings.Contains(projection.Action.Content, "# After") {
						t.Fatalf("foreign surrounding content was not preserved: %q", projection.Action.Content)
					}
				} else if !strings.Contains(projection.Action.Content, want) {
					t.Fatalf("foreign surrounding content was not preserved: %q", projection.Action.Content)
				}
			}
			if !test.writable && projection.Action.Content != "" {
				t.Fatalf("blocked marker state carried write content: %q", projection.Action.Content)
			}
		})
	}
}

func TestCodexProjectInspectionBuildsSharedComposableInstructions(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "instructions"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"one.md": "First guidance\n", "two.md": "Second guidance\n"} {
		if err := os.WriteFile(filepath.Join(bundle, "instructions", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(project, "AGENTS.md")
	if err := os.WriteFile(target, []byte("# Team guidance\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "guide", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "one", Source: "instructions/one.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "instruction", Name: "one", Capabilities: []capabilitypack.SurfaceCapability{{Type: capabilitypack.SurfaceCapabilityProjectInstruction, ProjectInstruction: &capabilitypack.ProjectInstructionCapability{ID: "one", Source: "instructions/one.md"}}}}}},
		{Kind: "instruction", ID: "two", Source: "instructions/two.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "instruction", Name: "two", Capabilities: []capabilitypack.SurfaceCapability{}}}},
	}}
	adapter := NewSurfaceAdapter(bundle, "", "")
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Unrepresentable) != 1 || inspection.Unrepresentable[0].Resource.String() != "instruction:two" || len(inspection.Projections) != 1 {
		t.Fatalf("instruction inspection = %#v", inspection)
	}
	one := findProjectProjection(t, inspection.Projections, "instruction:one")
	for _, projection := range []capabilitypack.ObservedProjection{one} {
		if projection.Action.Target != target || projection.Action.Kind != capabilitypack.ActionInstructionFile || !projection.Shared || len(projection.DiscoverableBy) != 1 || projection.DiscoverableBy[0] != capabilitypack.SurfaceOpenCode {
			t.Fatalf("shared instruction projection = %#v", projection)
		}
	}
	if !strings.Contains(one.Action.Content, "# Team guidance") || !strings.Contains(one.Action.Content, "First guidance") || strings.Contains(one.Action.Content, "Second guidance") {
		t.Fatalf("composed AGENTS.md = %q", one.Action.Content)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{one.Action}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != one.Action.Content {
		t.Fatalf("project instructions = %q, want %q: %v", content, one.Action.Content, err)
	}
}
