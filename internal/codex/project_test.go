package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestMattyProjectInspectionBuildsCopiedTreeAndComposableInstructions(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	source := filepath.Join(bundle, "skills", "ask-matt")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("# Ask Matt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Team guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "matty", Resources: []capabilitypack.Resource{{
		Kind: "skill", ID: "ask-matt", Source: "skills/ask-matt",
		Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Name: "ask-matt"}},
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
	instruction := findProjectProjection(t, inspection.Projections, projectMattyInstructionID)
	if instruction.ID != projectMattyInstructionID || instruction.Exists || instruction.AdapterProvenance != "codex-project/v1/composable-instruction/missing" || !strings.Contains(instruction.Action.Content, "# Team guidance") || !strings.Contains(instruction.Action.Content, projectMattyInstructionStart) {
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

func TestMattyProjectInstructionInspectionPreservesOnlyIntactContribution(t *testing.T) {
	project := t.TempDir()
	desired := projectMattyInstructionStart + "\nPacky manages the Matty Codex skill trees in .agents/skills.\n" + projectMattyInstructionEnd
	for _, test := range []struct {
		name, content, provenance string
		exists                    bool
		writable                  bool
	}{
		{name: "missing preserves foreign content", content: "# Team guidance\n", provenance: "missing", writable: true},
		{name: "intact preserves surrounding content", content: "# Before\n" + desired + "\n# After\n", provenance: "intact", exists: true, writable: true},
		{name: "changed marker contribution blocks", content: strings.Replace(desired, "skill trees", "different trees", 1), provenance: "changed", exists: true},
		{name: "orphaned marker blocks", content: projectMattyInstructionStart + "\n# incomplete\n", provenance: "malformed", exists: true},
		{name: "duplicate markers block", content: desired + "\n" + desired, provenance: "ambiguous", exists: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			projection, err := projectMattyInstruction(project)
			if err != nil {
				t.Fatal(err)
			}
			if projection.Exists != test.exists || projection.AdapterProvenance != "codex-project/v1/composable-instruction/"+test.provenance {
				t.Fatalf("projection = %#v", projection)
			}
			if test.writable {
				want := "# Team guidance"
				if test.provenance == "intact" {
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
