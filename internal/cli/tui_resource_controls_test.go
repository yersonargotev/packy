package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestTUIBackendConfiguresExactResourcesAndClearsPack(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			ctx := context.Background()
			synthetic := testsupport.CapabilityRich("tui-configure-" + scope)
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, synthetic)
			manifest := synthetic.Manifest()
			manifest.Resources = slices.DeleteFunc(manifest.Resources, func(resource testsupport.Resource) bool { return resource.Kind == "lifecycle" })
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(fixture.bundleRoot, "packs", synthetic.ID(), "pack.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}

			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts := fixture.options
			opts.Getwd = func() (string, error) { return project, nil }
			opts = opts.withDefaults()
			backend := newTUIBackend(opts, newWorkstationResolver(opts))
			request := tui.PreviewRequest{Operation: "activate", PackID: synthetic.ID(), Surface: "codex", Scope: scope, Selection: tui.Selection{Mode: "custom", Roots: []string{"skill:helper"}}}
			if scope == "project" {
				request.Operation, request.ProjectRoot = "install", project
			}
			preview := func(request tui.PreviewRequest) tui.Preview {
				t.Helper()
				view, err := backend.Preview(ctx, request)
				if err != nil {
					t.Fatal(err)
				}
				if len(view.Blockers) != 0 {
					t.Fatalf("blocked preview: %#v", view)
				}
				return view
			}
			apply := func(view tui.Preview) {
				t.Helper()
				result, err := backend.Apply(ctx, tui.ApplyRequest{Preview: view, ApprovedPhases: requiredTUIPhases(view)}, func(tui.ApplyProgress) {})
				if err != nil || !result.Verified {
					t.Fatalf("Apply = %#v, %v", result, err)
				}
			}
			status := func(surface string) tui.SurfaceStatus {
				t.Helper()
				dashboard, err := backend.Load(ctx)
				if err != nil {
					t.Fatal(err)
				}
				packs := dashboard.Global.Packs
				if scope == "project" {
					packs = dashboard.Project.Packs
				}
				if len(dashboard.Setup.Blockers) != 0 {
					t.Fatalf("setup blockers: %#v", dashboard.Setup.Blockers)
				}
				pack := findTUIPack(packs, synthetic.ID())
				if pack == nil {
					t.Fatal("Pack missing from dashboard")
				}
				for _, current := range pack.SurfaceStatuses {
					if current.Name == surface {
						return current
					}
				}
				t.Fatal("surface missing from dashboard")
				return tui.SurfaceStatus{}
			}
			apply(preview(request))
			if got := status("codex").Selection; got.Mode != "custom" || !slices.Equal(got.Roots, request.Selection.Roots) {
				t.Fatalf("initial selection = %#v", got)
			}
			if scope == "project" {
				other := request
				other.Surface = "opencode"
				apply(preview(other))
			}

			request.Operation = "configure"
			request.Selection.Roots = []string{"instruction:guidance", "skill:helper"}
			beforeHome, beforeProject := snapshotTree(t, fixture.home), snapshotTree(t, project)
			added := preview(request)
			if added.Operation != "configure" || added.PackVersion != synthetic.Manifest().Version || !slices.Equal(added.Selection.Roots, request.Selection.Roots) {
				t.Fatalf("exact same-version configure lost intent: %#v", added)
			}
			stale := added
			stale.Selection.Roots = []string{"instruction:guidance"}
			result, err := backend.Apply(ctx, tui.ApplyRequest{Preview: stale, ApprovedPhases: requiredTUIPhases(stale)}, func(tui.ApplyProgress) {})
			if err == nil || result.Stage != "revalidation" {
				t.Fatalf("changed selection accepted: %#v, %v", result, err)
			}
			result, err = backend.Apply(ctx, tui.ApplyRequest{Preview: added}, func(tui.ApplyProgress) {})
			if err == nil || result.Stage != "approval" {
				t.Fatalf("missing consent accepted: %#v, %v", result, err)
			}
			if snapshotTree(t, fixture.home) != beforeHome || snapshotTree(t, project) != beforeProject {
				t.Fatal("preview or rejected Apply changed files")
			}
			apply(added)
			if got := status("codex").Selection; !slices.Equal(got.Roots, request.Selection.Roots) {
				t.Fatalf("direct roots were not preserved: %#v", got)
			}
			projectionRoot := fixture.home
			if scope == "project" {
				projectionRoot = project
			}
			helper := filepath.Join(projectionRoot, ".agents", "skills", "helper")
			if _, err := os.Stat(helper); err != nil {
				t.Fatalf("helper was not projected: %v", err)
			}

			request.Selection.Roots = []string{"instruction:guidance"}
			removed := preview(request)
			if scope == "global" && !slices.Contains(requiredTUIPhases(removed), "destructive-cleanup") {
				t.Fatalf("retirement omitted destructive consent: %#v", removed.Phases)
			}
			apply(removed)
			if _, err := os.Lstat(helper); scope == "global" && !os.IsNotExist(err) {
				t.Fatalf("helper projection retained: %v", err)
			}
			if got := status("codex").Selection; !slices.Equal(got.Roots, request.Selection.Roots) {
				t.Fatalf("selection replacement merged old roots: %#v", got)
			}

			request.Selection.Roots = nil
			cleared := preview(request)
			wantOperation := "deactivate"
			if scope == "project" {
				wantOperation = "uninstall"
			}
			if cleared.Operation != wantOperation {
				t.Fatalf("empty selection operation = %s", cleared.Operation)
			}
			apply(cleared)
			if scope == "global" {
				if status("codex").Active {
					t.Fatal("empty selection retained global activation")
				}
			} else {
				installation, err := capabilitypack.LoadProjectInstallation(project)
				if err != nil {
					t.Fatal(err)
				}
				if len(installation.Manifest.Packs) != 1 || !slices.Equal(installation.Manifest.Packs[0].Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceOpenCode}) {
					t.Fatalf("clearing Codex changed other surface: %#v", installation.Manifest.Packs)
				}
				if got := status("opencode").Selection; !slices.Equal(got.Roots, []string{"skill:helper"}) {
					t.Fatalf("other surface selection changed: %#v", got)
				}
				if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "helper")); err != nil {
					t.Fatalf("other surface projection removed: %v", err)
				}
			}
		})
	}
}

func TestTUIBackendResourceClosuresIncludeSupportingContent(t *testing.T) {
	synthetic := testsupport.CapabilityRich("tui-resource-closures")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, synthetic)
	opts := fixture.options.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Global.Packs, synthetic.ID())
	if pack == nil {
		t.Fatal("Pack missing")
	}
	foundWorkflow := false
	for _, resource := range pack.Resources {
		if resource.Identity == "skill:workflow" {
			foundWorkflow = true
			for _, surface := range pack.Surfaces {
				for _, dependency := range []string{"skill:workflow", "skill:helper", "asset:reference", "notice:apache", "notice:mit"} {
					if !slices.Contains(resource.SelectionClosures[surface], dependency) {
						t.Fatalf("%s closure missing %s: %#v", surface, dependency, resource.SelectionClosures)
					}
				}
			}
		}
		if resource.Role == "supporting" || resource.Role == "notice" {
			if len(resource.SelectionClosures) != 0 {
				t.Fatalf("supporting content offers independent closure: %#v", resource)
			}
		}
	}
	if !foundWorkflow {
		t.Fatal("workflow resource missing from catalog")
	}
}

func TestTUIBackendProjectConfigurationRetiresUnsharedResource(t *testing.T) {
	ctx := context.Background()
	synthetic := testsupport.CapabilityRich("tui-project-retirement")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, synthetic)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts := fixture.options
	opts.Getwd = func() (string, error) { return project, nil }
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	request := tui.PreviewRequest{
		Operation: "install", PackID: synthetic.ID(), Surface: "codex", Scope: "project", ProjectRoot: project,
		Selection: tui.Selection{Mode: "custom", Roots: []string{"skill:helper"}},
	}
	initial, err := backend.Preview(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.Apply(ctx, tui.ApplyRequest{Preview: initial, ApprovedPhases: requiredTUIPhases(initial)}, func(tui.ApplyProgress) {})
	if err != nil || !result.Verified {
		t.Fatalf("install = %#v, %v", result, err)
	}
	helper := filepath.Join(project, ".agents", "skills", "helper")
	if _, err := os.Stat(helper); err != nil {
		t.Fatal(err)
	}
	request.Operation = "configure"
	request.Selection.Roots = []string{"instruction:guidance"}
	changed, err := backend.Preview(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if changed.PackVersion != synthetic.CurrentVersion() || len(changed.Blockers) != 0 || !slices.Contains(changed.Diff.Removed, ".agents/skills/helper") || !slices.Contains(requiredTUIPhases(changed), "destructive-cleanup") {
		t.Fatalf("same-version resource replacement preview = %#v", changed)
	}
	before := snapshotTree(t, project)
	approvals := slices.DeleteFunc(requiredTUIPhases(changed), func(phase string) bool { return phase == "destructive-cleanup" })
	result, err = backend.Apply(ctx, tui.ApplyRequest{Preview: changed, ApprovedPhases: approvals}, func(tui.ApplyProgress) {})
	if err == nil || result.Stage != "approval" || snapshotTree(t, project) != before {
		t.Fatalf("retirement without destructive consent = %#v, %v", result, err)
	}
	result, err = backend.Apply(ctx, tui.ApplyRequest{Preview: changed, ApprovedPhases: requiredTUIPhases(changed)}, func(tui.ApplyProgress) {})
	if err != nil || !result.Verified {
		t.Fatalf("configure = %#v, %v", result, err)
	}
	if _, err := os.Lstat(helper); !os.IsNotExist(err) {
		t.Fatalf("retired helper still exists: %v", err)
	}
	guidance, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil || !strings.Contains(string(guidance), "Synthetic capability-rich guidance.") {
		t.Fatalf("new guidance not materialized: %s, %v", guidance, err)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Manifest.Packs) != 1 {
		t.Fatalf("manifest = %#v", installation.Manifest.Packs)
	}
	intent := installation.Manifest.Packs[0]
	if intent.Version != synthetic.CurrentVersion() || intent.Selection.Mode != capabilitypack.SelectionCustom || !slices.Equal(intent.Selection.Roots, []capabilitypack.ResourceIdentity{{Kind: "instruction", ID: "guidance"}}) {
		t.Fatalf("exact replacement not persisted: %#v", intent)
	}
	dashboard, err := backend.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Project.Packs, synthetic.ID())
	if pack == nil {
		t.Fatal("project Pack missing after configuration")
	}
	index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || !slices.Equal(pack.SurfaceStatuses[index].Selection.Roots, []string{"instruction:guidance"}) {
		t.Fatalf("project status omitted new selection: %#v", pack.SurfaceStatuses)
	}
}
