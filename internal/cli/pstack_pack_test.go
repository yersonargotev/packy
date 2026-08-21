package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPstackActivationPreviewsProjectThroughEverySurfaceAdapter(t *testing.T) {
	tests := []struct {
		surface    string
		actionKind capabilitypack.ProjectionActionKind
	}{
		{surface: "claude", actionKind: "claude-skill-link"},
		{surface: "codex", actionKind: capabilitypack.ActionSkillLink},
		{surface: "opencode", actionKind: capabilitypack.ActionOpenCodeSkillLink},
	}

	for _, test := range tests {
		t.Run(test.surface, func(t *testing.T) {
			opts, home, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
			before := snapshotTree(t, home)

			complete := previewPstackActivation(t, opts, test.surface)
			if complete.Selection.Mode != capabilitypack.SelectionAll || len(complete.MandatoryActions) != 26 || len(complete.ResourceGraph.Resources) != 27 {
				t.Fatalf("complete preview selection=%#v actions=%d resources=%d", complete.Selection, len(complete.MandatoryActions), len(complete.ResourceGraph.Resources))
			}
			assertPstackProjectionActions(t, complete.MandatoryActions, test.actionKind, 26)

			selected := previewPstackActivation(t, opts, test.surface, "--resource", "skill:principle-build-the-lever")
			wantRoot := capabilitypack.ResourceIdentity{Kind: "skill", ID: "principle-build-the-lever"}
			if selected.Selection.Mode != capabilitypack.SelectionCustom || len(selected.Selection.Roots) != 1 || selected.Selection.Roots[0] != wantRoot {
				t.Fatalf("custom preview selection=%#v", selected.Selection)
			}
			if len(selected.MandatoryActions) != 4 || len(selected.ResourceGraph.Resources) != 5 {
				t.Fatalf("dependency-closing preview actions=%d resources=%d", len(selected.MandatoryActions), len(selected.ResourceGraph.Resources))
			}
			assertPstackProjectionActions(t, selected.MandatoryActions, test.actionKind, 4)

			if after := snapshotTree(t, home); after != before {
				t.Fatalf("%s pstack previews mutated sandbox HOME:\n%s", test.surface, after)
			}
		})
	}
}

func previewPstackActivation(t *testing.T, opts Options, surface string, extra ...string) capabilitypack.JSONLifecyclePlan {
	t.Helper()
	args := []string{"activate", "pstack", "--surface", surface, "--dry-run", "--json"}
	args = append(args, extra...)
	out, err := executeCommand(t, NewRootCommand(opts), args...)
	if err != nil {
		t.Fatalf("%s pstack preview: %v\n%s", surface, err, out)
	}
	var report capabilitypack.JSONLifecyclePlan
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s pstack preview: %v\n%s", surface, err, out)
	}
	if report.Report != "pack-lifecycle-preview" || report.Pack != "pstack" || report.PackVersion != "1.0.0" || string(report.Surface) != surface || !report.DryRun || len(report.Blockers) != 0 {
		t.Fatalf("invalid %s pstack preview: %#v", surface, report)
	}
	return report
}

func assertPstackProjectionActions(t *testing.T, actions []capabilitypack.ProjectionAction, wantKind capabilitypack.ProjectionActionKind, wantCount int) {
	t.Helper()
	seen := map[string]bool{}
	for _, action := range actions {
		if action.Kind != wantKind || action.ID == "" || action.Source == "" || action.Target == "" {
			t.Fatalf("invalid pstack projection action: %#v", action)
		}
		if filepath.Base(action.Source) != filepath.Base(action.Target) || seen[action.Target] {
			t.Fatalf("colliding or mismatched pstack projection action: %#v", action)
		}
		seen[action.Target] = true
	}
	if len(seen) != wantCount {
		t.Fatalf("pstack projection targets=%d want=%d", len(seen), wantCount)
	}
}
