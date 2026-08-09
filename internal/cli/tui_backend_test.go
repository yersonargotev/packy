package cli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestTUIProductionBackendUsesPackyOwnersWithoutMutatingState(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name             string
		currentDirectory string
		projectAvailable bool
	}{
		{name: "Git project", currentDirectory: repositoryRoot, projectAvailable: true},
		{name: "outside Git", currentDirectory: t.TempDir(), projectAvailable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			opts := Options{
				Env: MapEnv{
					"HOME":                home,
					"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
					"PATH":                "",
					"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
				},
				Getwd:  func() (string, error) { return test.currentDirectory, nil },
				Runner: &fakeRunner{},
			}
			opts = opts.withDefaults()
			before := snapshotTree(t, home)

			dashboard, err := newTUIBackend(opts, newWorkstationResolver(opts)).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if dashboard.Health.Status != "healthy" || len(dashboard.Global.Packs) == 0 {
				t.Fatalf("dashboard omitted owner-derived global state: %#v", dashboard)
			}
			orchestrate := findTUIPack(dashboard.Global.Packs, "orchestrate")
			if orchestrate == nil {
				t.Fatalf("dynamic catalog omitted selectable orchestrate Pack: %#v", dashboard.Global.Packs)
			}
			if orchestrate.Description == "" || len(orchestrate.Resources) == 0 {
				t.Fatalf("catalog detail omitted manifest-owned description or resources: %#v", orchestrate)
			}
			if len(orchestrate.SurfaceStatuses) != 3 {
				t.Fatalf("surface matrix has %d entries, want all 3 known CLI surfaces: %#v", len(orchestrate.SurfaceStatuses), orchestrate.SurfaceStatuses)
			}
			for _, unsupported := range []string{"claude", "opencode"} {
				index := slices.IndexFunc(orchestrate.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == unsupported })
				if index < 0 || orchestrate.SurfaceStatuses[index].Supported {
					t.Fatalf("orchestrate did not identify %s as unsupported before lifecycle action: %#v", unsupported, orchestrate.SurfaceStatuses)
				}
			}
			codexIndex := slices.IndexFunc(orchestrate.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
			if codexIndex < 0 || !orchestrate.SurfaceStatuses[codexIndex].Supported || orchestrate.SurfaceStatuses[codexIndex].Configured == "" {
				t.Fatalf("orchestrate omitted supported Codex readiness status: %#v", orchestrate.SurfaceStatuses)
			}
			if dashboard.Project.Available != test.projectAvailable {
				t.Fatalf("project availability = %v, want %v", dashboard.Project.Available, test.projectAvailable)
			}
			if test.projectAvailable && dashboard.Project.Root != repositoryRoot {
				t.Fatalf("project root = %q, want %q", dashboard.Project.Root, repositoryRoot)
			}
			if test.projectAvailable && len(dashboard.Project.Packs) != len(dashboard.Global.Packs) {
				t.Fatalf("project catalog has %d selectable Packs, global has %d", len(dashboard.Project.Packs), len(dashboard.Global.Packs))
			}
			if after := snapshotTree(t, home); after != before {
				t.Fatalf("read-only backend mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

func TestTUIProductionBackendGuidesAndInitializesMissingInstalledSource(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	opts := Options{
		Env: MapEnv{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, "xdg"),
			"PATH":            os.Getenv("PATH"),
		},
		Getwd: func() (string, error) { return t.TempDir(), nil },
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	backend.repositoryURL = repositoryRoot
	before := snapshotTree(t, home)

	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Health.Status != "healthy" || !dashboard.Setup.InitializationAvailable || len(dashboard.Setup.Blockers) != 1 {
		t.Fatalf("missing setup did not retain diagnosis and an initialization route: %#v", dashboard)
	}
	blocker := dashboard.Setup.Blockers[0]
	if !strings.Contains(blocker.Cause, "initialize") || !slices.Equal(blocker.AffectedActions, []string{"Pack catalog inspection", "Pack lifecycle actions"}) {
		t.Fatalf("setup blocker is not decision-ready: %#v", blocker)
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("loading an uninitialized dashboard mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
	}

	var progress []string
	if err := backend.Initialize(context.Background(), func(detail string) { progress = append(progress, detail) }); err != nil {
		t.Fatal(err)
	}
	installedRoot := bootstrap.DefaultInstalledSourceRoot(home)
	if !exists(filepath.Join(installedRoot, "bundle", "packs")) {
		t.Fatalf("TUI initialization did not install the reviewed catalog at %s", installedRoot)
	}
	if !strings.Contains(strings.Join(progress, "\n"), "cloning Installed Source") || !strings.Contains(strings.Join(progress, "\n"), "initialized Installed Source") {
		t.Fatalf("initialization progress was not genuine and complete: %v", progress)
	}

	dashboard, err = backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Setup.InitializationAvailable || len(dashboard.Setup.Blockers) != 0 || len(dashboard.Global.Packs) == 0 {
		t.Fatalf("reloaded dashboard did not reflect initialized state: %#v", dashboard)
	}

	progress = nil
	if err := backend.Initialize(context.Background(), func(detail string) { progress = append(progress, detail) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(progress, "\n"), "already initialized") {
		t.Fatalf("already-initialized result missing from progress: %v", progress)
	}
}

func TestTUIProductionBackendKeepsProjectInspectionWhenGlobalStatusIsBlocked(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	stateFile := filepath.Join(home, ".packy", "packs.json")
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
			"PATH":                "",
			"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
		},
		Getwd:  func() (string, error) { return repositoryRoot, nil },
		Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()

	dashboard, err := newTUIBackend(opts, newWorkstationResolver(opts)).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboard.Setup.Blockers) != 1 || !strings.Contains(dashboard.Setup.Blockers[0].Cause, "global Pack status") {
		t.Fatalf("global status blocker missing or unclear: %#v", dashboard.Setup.Blockers)
	}
	if !dashboard.Project.Available || dashboard.Project.Root != repositoryRoot || len(dashboard.Project.Packs) == 0 {
		t.Fatalf("global status blocker hid unaffected project inspection: %#v", dashboard.Project)
	}
}

func TestTUIProductionBackendPreviewsFullAndPartialSelectionWithoutMutatingState(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	opts := Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
			"PATH":                "",
			"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
		},
		Getwd:  func() (string, error) { return repositoryRoot, nil },
		Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Global.Packs, "argote")
	if pack == nil {
		t.Fatal("argote Pack missing from reviewed catalog")
	}
	var root string
	for _, resource := range pack.Resources {
		if resource.Role == "root" || resource.Role == "operational" {
			root = resource.Identity
			break
		}
	}
	if root == "" {
		t.Fatalf("argote Pack exposes no operational root: %#v", pack.Resources)
	}
	before := snapshotTree(t, home)

	full, err := backend.Preview(context.Background(), tui.PreviewRequest{
		PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	partial, err := backend.Preview(context.Background(), tui.PreviewRequest{
		PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "custom", Roots: []string{root}},
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := backend.Preview(context.Background(), tui.PreviewRequest{
		PackID: "argote", Surface: "codex", Scope: "project", ProjectRoot: repositoryRoot, Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if full.ID == "" || full.Digest == "" || full.PackID != "argote" || full.Surface != "codex" || full.Scope != "global" || full.Selection.Mode != "all" {
		t.Fatalf("full-Pack preview omitted its immutable exact target: %#v", full)
	}
	if partial.ID == "" || partial.Digest == "" || partial.Selection.Mode != "custom" || !slices.Equal(partial.Selection.Roots, []string{root}) {
		t.Fatalf("partial preview omitted its exact selected root: %#v", partial)
	}
	if len(full.Resources) == 0 || len(partial.Resources) == 0 {
		t.Fatalf("preview omitted resource closure: full=%#v partial=%#v", full.Resources, partial.Resources)
	}
	if project.ID == "" || project.PackID != "argote" || project.Surface != "codex" || project.Scope != "project" || project.Operation != "install" {
		t.Fatalf("project preview omitted its immutable exact target: %#v", project)
	}
	effectKinds := make(map[string]bool)
	for _, effect := range project.Effects {
		effectKinds[effect.Kind] = true
	}
	for _, kind := range []string{"project-manifest", "project-lock", "project-notices"} {
		if !effectKinds[kind] {
			t.Fatalf("project preview omitted %s effect: %#v", kind, project.Effects)
		}
	}
	diffFacts := append(append(append(append([]string{}, project.Diff.Added...), project.Diff.Changed...), project.Diff.Removed...), project.Diff.Retained...)
	for _, target := range []string{project.Effects[0].Target, project.Effects[1].Target, project.Effects[2].Target} {
		if target == "" || !slices.Contains(diffFacts, target) {
			t.Fatalf("project preview omitted exact effect target %q from diff: %#v", target, project.Diff)
		}
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("immutable previews mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestTUIProductionBackendRejectsStaleApprovalAndAppliesAnExactGlobalActivation(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	opts := Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
			"PATH":                "",
			"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
		},
		Getwd:  func() (string, error) { return repositoryRoot, nil },
		Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{
		PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	approved := make([]string, 0)
	for _, phase := range preview.Phases {
		if phase.ApprovalRequired {
			approved = append(approved, phase.Kind)
		}
	}
	before := snapshotTree(t, home)
	stale := preview
	stale.Digest = "superseded-digest"
	result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: stale, ApprovedPhases: approved}, func(tui.ApplyProgress) {})
	if err == nil || result.Stage != "revalidation" || !strings.Contains(err.Error(), "fresh preview") {
		t.Fatalf("stale Apply = result %#v, err %v; want revalidation failure", result, err)
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("stale Apply mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
	}

	var progress []string
	result, err = backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: approved}, func(update tui.ApplyProgress) {
		progress = append(progress, update.Phase)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.Stage != "verification" || !strings.Contains(result.Summary, "argote") || !slices.Equal(progress, []string{"revalidation", "apply", "verification"}) {
		t.Fatalf("activation result/progress = %#v / %v", result, progress)
	}
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Global.Packs, "argote")
	if pack == nil {
		t.Fatal("fresh status omitted argote")
	}
	index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || pack.SurfaceStatuses[index].Configured != "yes" {
		t.Fatalf("fresh status did not observe activation: %#v", pack)
	}
}

func findTUIPack(packs []tui.Pack, id string) *tui.Pack {
	for _, pack := range packs {
		if pack.ID == id {
			return &pack
		}
	}
	return nil
}
