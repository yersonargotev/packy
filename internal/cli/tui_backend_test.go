package cli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/capabilitypack"
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

func TestTUICatalogAdapterPreservesSearchableSurfaceCapabilities(t *testing.T) {
	details := []capabilitypack.CatalogDetail{{
		Pack: capabilitypack.Pack{
			ID: "synthetic", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceOpenCode},
			Requires: capabilitypack.Requirements{Tools: []string{"pack-tool"}},
			Resources: []capabilitypack.Resource{{
				Kind: "skill", ID: "coordinate", Description: "Coordinate workers", RequiresTools: []string{"resource-tool"},
				Bindings: []capabilitypack.Binding{{
					Surface: capabilitypack.SurfaceOpenCode,
					Capabilities: []capabilitypack.SurfaceCapability{{
						Type:                          capabilitypack.SurfaceCapabilityExternalExecutableAcquisition,
						ExternalExecutableAcquisition: &capabilitypack.ExternalExecutableAcquisitionCapability{Tool: "capability-tool"},
					}},
				}},
			}},
		},
		ResourceInventory: []capabilitypack.DescriptiveResource{{
			Resource:    capabilitypack.ResourceIdentity{Kind: "skill", ID: "coordinate"},
			Description: "Coordinate workers", Role: capabilitypack.ResourceInventoryRoleOperational,
		}},
	}}

	packs := catalogPacksForTUI(details, nil)
	if len(packs) != 1 || len(packs[0].Resources) != 1 {
		t.Fatalf("adapted catalog = %#v", packs)
	}
	resource := packs[0].Resources[0]
	if !slices.Equal(packs[0].Surfaces, []string{"opencode"}) || !slices.Equal(packs[0].Requirements, []string{"pack-tool"}) {
		t.Fatalf("Pack search metadata = %#v", packs[0])
	}
	if resource.Identity != "skill:coordinate" || resource.Description != "Coordinate workers" || !slices.Contains(resource.Requirements, "resource-tool") {
		t.Fatalf("resource search metadata = %#v", resource)
	}
	if len(resource.SurfaceCapabilities) != 1 {
		t.Fatalf("surface capabilities = %#v", resource.SurfaceCapabilities)
	}
	capability := resource.SurfaceCapabilities[0]
	if capability.Surface != "opencode" || capability.Type != "external-executable-acquisition" || capability.Tool != "capability-tool" {
		t.Fatalf("surface capability = %#v", capability)
	}
}

func TestTUIBackendPreviewsAndRecordsControlledRuntimeCheck(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	if _, err := executeCommand(t, NewRootCommand(opts), "activate", "orchestrate", "--surface", "codex"); err != nil {
		t.Fatal(err)
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	beforeCheck, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	orchestrateBefore := findTUIPack(beforeCheck.Global.Packs, "orchestrate")
	beforeIndex := slices.IndexFunc(orchestrateBefore.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if beforeIndex < 0 || !orchestrateBefore.SurfaceStatuses[beforeIndex].ControlledCheckActionAvailable {
		t.Fatalf("domain-owned controlled check action did not flow to TUI status: %#v", orchestrateBefore)
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "check", PackID: "orchestrate", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Operation != "check" || preview.ValidityIdentity == "" || preview.ProjectionRevision == "" || len(preview.Instructions) == 0 || len(preview.Resources) == 0 {
		t.Fatalf("controlled check preview = %#v", preview)
	}
	result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: []string{"controlled-check"}, ControlledCheckResult: "positive"}, func(tui.ApplyProgress) {})
	if err != nil || !result.Verified {
		t.Fatalf("controlled check apply = %#v, %v", result, err)
	}
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	orchestrate := findTUIPack(dashboard.Global.Packs, "orchestrate")
	index := slices.IndexFunc(orchestrate.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || orchestrate.SurfaceStatuses[index].Usable != "true" || orchestrate.SurfaceStatuses[index].ControlledCheckActionAvailable || orchestrate.SurfaceStatuses[index].ControlledCheckState != "current" || orchestrate.SurfaceStatuses[index].ControlledCheckResult != "true" || orchestrate.SurfaceStatuses[index].ControlledCheckIdentity == "" {
		t.Fatalf("controlled check did not flow to TUI status: %#v", orchestrate)
	}
}

func TestTUIBackendPreservesProjectControlledCheckActionMeaning(t *testing.T) {
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	opts.Getwd = func() (string, error) { return project, nil }
	if output, err := executeCommand(t, NewRootCommand(opts), "install", "orchestrate", "--surface", "codex", "--resource", "skill:orchestrate"); err != nil {
		t.Fatalf("install project orchestrate: %v\n%s", err, output)
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	orchestrate := findTUIPack(dashboard.Project.Packs, "orchestrate")
	index := slices.IndexFunc(orchestrate.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || !orchestrate.SurfaceStatuses[index].ControlledCheckActionAvailable {
		t.Fatalf("project controlled check action did not flow to TUI status: %#v", orchestrate)
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "check", PackID: "orchestrate", Surface: "codex", Scope: "project", ProjectRoot: project, Selection: tui.Selection{Mode: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: []string{"controlled-check"}, ControlledCheckResult: "positive"}, func(tui.ApplyProgress) {}); err != nil || !result.Verified {
		t.Fatalf("record project controlled check = %#v, %v", result, err)
	}
	dashboard, err = backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	orchestrate = findTUIPack(dashboard.Project.Packs, "orchestrate")
	index = slices.IndexFunc(orchestrate.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || orchestrate.SurfaceStatuses[index].ControlledCheckActionAvailable {
		t.Fatalf("current positive project check still offered an action: %#v", orchestrate)
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
	if _, err := backend.Preview(context.Background(), tui.PreviewRequest{PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}}); err == nil || !strings.Contains(err.Error(), "operation") {
		t.Fatalf("missing lifecycle operation was accepted: %v", err)
	}

	full, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "activate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	partial, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "activate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "custom", Roots: []string{root}},
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "install", PackID: "argote", Surface: "codex", Scope: "project", ProjectRoot: repositoryRoot, Selection: tui.Selection{Mode: "all"},
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
		Operation: "activate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
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
	if index < 0 || pack.SurfaceStatuses[index].Configured != "true" {
		t.Fatalf("fresh status did not observe activation: %#v", pack)
	}
}

func TestTUIProductionBackendInstallsThenPersonallyActivatesInTheCurrentProject(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	writeTestGitWorktree(t, project)
	otherProject := filepath.Join(t.TempDir(), "other-project")
	writeTestGitWorktree(t, otherProject)
	currentDirectory := project
	opts := Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
			"PATH":                "",
			"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
		},
		Getwd: func() (string, error) { return currentDirectory, nil }, Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	if _, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "install", PackID: "argote", Surface: "codex", Scope: "project", ProjectRoot: otherProject, Selection: tui.Selection{Mode: "all"},
	}); err == nil || !strings.Contains(err.Error(), "current Git project") {
		t.Fatalf("project preview accepted a different Git repository: %v", err)
	}
	install, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "install", PackID: "matty", Surface: "opencode", Scope: "project", ProjectRoot: project, Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if install.Operation != "install" || install.Disposition != "previewable" || len(install.Resources) == 0 || len(install.Effects) == 0 || len(install.Phases) != 1 {
		t.Fatalf("project install preview = %#v", install)
	}
	for _, effect := range install.Effects {
		if !slices.ContainsFunc(install.Phases[0].Actions, func(action string) bool { return strings.Contains(action, effect.Target) }) {
			t.Fatalf("project consent omitted exact effect target %q: %#v", effect.Target, install.Phases[0])
		}
	}
	beforeProject, beforeHome := snapshotTree(t, project), snapshotTree(t, home)
	wrongTarget := install
	wrongTarget.ProjectRoot = otherProject
	result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: wrongTarget, ApprovedPhases: requiredTUIPhases(install)}, func(tui.ApplyProgress) {})
	if err == nil || result.Stage != "revalidation" || !strings.Contains(err.Error(), "current Git project") {
		t.Fatalf("project Apply accepted a different current repository: %#v, %v", result, err)
	}
	if snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome {
		t.Fatal("wrong-current-project Apply mutated project or personal state")
	}
	stale := install
	stale.Digest = "stale-install"
	result, err = backend.Apply(context.Background(), tui.ApplyRequest{Preview: stale, ApprovedPhases: requiredTUIPhases(install)}, func(tui.ApplyProgress) {})
	if err == nil || result.Stage != "revalidation" || !strings.Contains(err.Error(), "fresh preview") {
		t.Fatalf("stale project install = %#v, %v", result, err)
	}
	if snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome {
		t.Fatal("stale project install mutated project or personal state")
	}

	var progress []string
	result, err = backend.Apply(context.Background(), tui.ApplyRequest{Preview: install, ApprovedPhases: requiredTUIPhases(install)}, func(update tui.ApplyProgress) {
		progress = append(progress, update.Phase)
	})
	if err != nil || !result.Verified || result.FollowUpOperation != "" || result.RuntimeActivation != "not required" || !slices.Equal(progress, []string{"revalidation", "apply", "verification"}) {
		t.Fatalf("project install result/progress = %#v / %v / %v", result, progress, err)
	}
	for _, path := range []string{"packy.json", "packy.lock.json", "PACKY-NOTICES.md"} {
		if _, err := os.Stat(filepath.Join(project, path)); err != nil {
			t.Fatalf("project installation omitted %s: %v", path, err)
		}
	}

	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Project.Packs, "matty")
	index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "opencode" })
	if index < 0 || pack.SurfaceStatuses[index].Installation != "installed" {
		t.Fatalf("fresh project status did not retain project installation: %#v", pack)
	}
}

func TestTUIProductionBackendCoordinatesPersonalDeactivationAndUninstallsAnInstalledProjectPack(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	writeTestGitWorktree(t, project)
	opts := Options{
		Env: MapEnv{
			"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
			"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
		},
		Getwd: func() (string, error) { return project, nil }, Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))

	install, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "install", PackID: "matty", Surface: "opencode", Scope: "project", ProjectRoot: project, Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: install, ApprovedPhases: requiredTUIPhases(install)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "uninstall", PackID: "matty", Surface: "opencode", Scope: "project", ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Operation != "uninstall" || preview.Disposition != "previewable" || preview.Surface != "opencode" || !slices.Equal(requiredTUIPhases(preview), []string{"destructive-cleanup"}) {
		t.Fatalf("project uninstall preview = %#v", preview)
	}
	for _, target := range []string{"packy.json", "packy.lock.json", "PACKY-NOTICES.md"} {
		if !slices.Contains(preview.Diff.Removed, target) {
			t.Fatalf("project uninstall preview omitted contract %q: %#v", target, preview)
		}
	}
	if len(preview.PendingActions) != 0 {
		t.Fatalf("project uninstall preview introduced an unrelated runtime prerequisite: %#v", preview.PendingActions)
	}
	for _, effect := range preview.Effects {
		if effect.Target != "" && !slices.ContainsFunc(preview.Phases[0].Actions, func(action string) bool { return strings.Contains(action, effect.Target) }) {
			t.Fatalf("destructive consent omitted exact effect target %q: %#v", effect.Target, preview.Phases[0])
		}
	}

	before := snapshotTree(t, project)
	stale := preview
	stale.Digest = "stale-project-uninstall"
	result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: stale, ApprovedPhases: requiredTUIPhases(preview)}, func(tui.ApplyProgress) {})
	if err == nil || result.Stage != "revalidation" || !strings.Contains(err.Error(), "fresh preview") {
		t.Fatalf("stale project uninstall = %#v, %v", result, err)
	}
	if after := snapshotTree(t, project); after != before {
		t.Fatalf("stale project uninstall mutated project\nbefore:\n%s\nafter:\n%s", before, after)
	}

	var progress []string
	result, err = backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: requiredTUIPhases(preview)}, func(update tui.ApplyProgress) { progress = append(progress, update.Phase) })
	if err != nil || !result.Verified || !strings.Contains(result.Summary, "Uninstalled matty") || !slices.Equal(progress, []string{"revalidation", "apply", "verification"}) {
		t.Fatalf("project uninstall result = %#v / %v / %v", result, progress, err)
	}
	for _, target := range []string{"packy.json", "packy.lock.json"} {
		if _, err := os.Stat(filepath.Join(project, target)); !os.IsNotExist(err) {
			t.Fatalf("verified uninstall retained %s: %v", target, err)
		}
	}
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Project.Packs, "matty")
	index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "opencode" })
	if index < 0 || (pack.SurfaceStatuses[index].Installation != "" && pack.SurfaceStatuses[index].Installation != "absent") || pack.SurfaceStatuses[index].Runtime == "active" {
		t.Fatalf("fresh status did not verify complete uninstall: %#v", pack)
	}
}

func TestTUIProductionBackendShowsProjectUninstallDriftAndRefusesApply(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	writeTestGitWorktree(t, project)
	opts := Options{
		Env: MapEnv{
			"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
			"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
		},
		Getwd: func() (string, error) { return project, nil }, Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	install, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "install", PackID: "matty", Surface: "opencode", Scope: "project", ProjectRoot: project, Selection: tui.Selection{Mode: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: install, ApprovedPhases: requiredTUIPhases(install)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}
	projectionPath := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if err := os.WriteFile(projectionPath, []byte("operator edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "uninstall", PackID: "matty", Surface: "opencode", Scope: "project", ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition != "blocked" || len(preview.Blockers) == 0 || len(requiredTUIPhases(preview)) != 0 || !slices.ContainsFunc(preview.Blockers, func(blocker tui.PreviewBlocker) bool { return blocker.Kind == "project_drift" }) {
		t.Fatalf("drifted project uninstall did not fail closed: %#v", preview)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview}, func(tui.ApplyProgress) {}); err == nil {
		t.Fatal("blocked project uninstall Apply succeeded")
	}
	if after := snapshotTree(t, project); after != before {
		t.Fatalf("blocked project uninstall mutated the project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestTUIProductionBackendUninstallsOnlyTheSelectedProjectSurface(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	writeTestGitWorktree(t, project)
	opts := Options{
		Env: MapEnv{
			"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
			"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
		},
		Getwd: func() (string, error) { return project, nil }, Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	for _, surface := range []string{"codex", "opencode"} {
		install, previewErr := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "install", PackID: "matty", Surface: surface, Scope: "project", ProjectRoot: project, Selection: tui.Selection{Mode: "all"}})
		if previewErr != nil {
			t.Fatal(previewErr)
		}
		if _, applyErr := backend.Apply(context.Background(), tui.ApplyRequest{Preview: install, ApprovedPhases: requiredTUIPhases(install)}, func(tui.ApplyProgress) {}); applyErr != nil {
			t.Fatal(applyErr)
		}
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "uninstall", PackID: "matty", Surface: "codex", Scope: "project", ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Surface != "codex" || !slices.Contains(preview.Diff.Changed, "packy.json") || !slices.Contains(preview.Diff.Changed, "packy.lock.json") || !slices.Contains(preview.Diff.Removed, "AGENTS.md") {
		t.Fatalf("selected-surface uninstall preview = %#v", preview)
	}
	result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: requiredTUIPhases(preview)}, func(tui.ApplyProgress) {})
	if err != nil || !result.Verified || !strings.Contains(result.Summary, "from codex") || !slices.Contains(result.Details, "Other installed surfaces remain independently installed") {
		t.Fatalf("selected-surface uninstall result = %#v, %v", result, err)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Manifest.Packs) != 1 || !slices.Equal(installation.Manifest.Packs[0].Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceOpenCode}) {
		t.Fatalf("selected-surface uninstall changed retained intent: %#v", installation.Manifest.Packs)
	}
	for _, retained := range []string{"packy.json", "packy.lock.json", filepath.Join(".agents", "skills", "ask-matt", "SKILL.md")} {
		if _, err := os.Stat(filepath.Join(project, retained)); err != nil {
			t.Fatalf("selected-surface uninstall removed retained %s: %v", retained, err)
		}
	}
}

func TestTUIProductionBackendUpdatesAnInstalledProjectPack(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	writeTestGitWorktree(t, project)
	bundle := copyPackBundleForUpdate(t, repositoryRoot)
	opts := Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
			"PATH":                "",
			"PACKY_SKILLS_SOURCE": filepath.Join(bundle, "skills"),
		},
		Getwd:  func() (string, error) { return project, nil },
		Runner: &fakeRunner{},
	}
	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))

	install, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "install", PackID: "matty", Surface: "codex", Scope: "project", ProjectRoot: project, Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: install, ApprovedPhases: requiredTUIPhases(install)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}

	noOp, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "update", PackID: "matty", Surface: "codex", Scope: "project", ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if noOp.Disposition != "converged" || len(requiredTUIPhases(noOp)) != 0 {
		t.Fatalf("catalog-current project update was not an explicit no-op: %#v", noOp)
	}

	manifestPath := filepath.Join(bundle, "packs", "matty", "pack.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedManifest, updatedVersion := bumpManifestPatchVersion(t, string(manifest))
	if err := os.WriteFile(manifestPath, []byte(updatedManifest), 0o600); err != nil {
		t.Fatal(err)
	}

	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Project.Packs, "matty")
	index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || !pack.SurfaceStatuses[index].UpdateAvailable {
		t.Fatalf("updated catalog did not offer a Matty update: %#v", pack)
	}

	update, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "update", PackID: "matty", Surface: "codex", Scope: "project", ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if update.Disposition != "previewable" || update.PackVersion != updatedVersion {
		t.Fatalf("project update preview = %#v", update)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: update, ApprovedPhases: requiredTUIPhases(update)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Manifest.Packs) != 1 || installation.Manifest.Packs[0].Version != updatedVersion {
		t.Fatalf("project update did not record the selected Pack version: %#v", installation.Manifest.Packs)
	}
}
func TestTUIProductionBackendPreviewsNoOpUpdateAndAppliesPartialThenCompleteDeactivation(t *testing.T) {
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

	activate, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "activate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: activate, ApprovedPhases: requiredTUIPhases(activate)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}

	update, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "update", PackID: "argote", Surface: "codex", Scope: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Operation != "update" || update.Disposition != "converged" {
		t.Fatalf("catalog-current update = %#v; want explicit no-op", update)
	}

	partial, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "deactivate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "custom", Roots: []string{"skill:espera-que"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Operation != "deactivate" || partial.Disposition != "applicable" || !slices.Equal(partial.Selection.Roots, []string{"skill:espera-que"}) || len(partial.Diff.Removed) == 0 || len(partial.Diff.Retained) == 0 {
		t.Fatalf("partial deactivation preview = %#v", partial)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: partial, ApprovedPhases: requiredTUIPhases(partial)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}

	complete, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "deactivate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	stale := complete
	stale.Digest = "stale"
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: stale, ApprovedPhases: requiredTUIPhases(complete)}, func(tui.ApplyProgress) {}); err == nil || !strings.Contains(err.Error(), "fresh preview") {
		t.Fatalf("stale deactivation err = %v", err)
	}
	result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: complete, ApprovedPhases: requiredTUIPhases(complete)}, func(tui.ApplyProgress) {})
	if err != nil || !result.Verified || !strings.Contains(result.Summary, "Deactivated") {
		t.Fatalf("complete deactivation = %#v, %v", result, err)
	}
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Global.Packs, "argote")
	index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || pack.SurfaceStatuses[index].Active || pack.SurfaceStatuses[index].Ownership != 0 {
		t.Fatalf("fresh status retained deactivated Pack: %#v", pack)
	}
}

func TestTUIProductionBackendShowsDriftAndFailsDeactivationClosed(t *testing.T) {
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
		Getwd: func() (string, error) { return repositoryRoot, nil }, Runner: &fakeRunner{},
	}
	opts = opts.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	activate, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "activate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: activate, ApprovedPhases: requiredTUIPhases(activate)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".agents", "skills", "espera-que")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("operator edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, home)
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Global.Packs, "argote")
	index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if index < 0 || !pack.SurfaceStatuses[index].UpdateAvailable {
		t.Fatalf("drifted active selection did not offer Update: %#v", pack)
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "deactivate", PackID: "argote", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition == "applicable" || len(preview.Blockers) == 0 {
		t.Fatalf("drifted deactivation did not fail closed: %#v", preview)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: requiredTUIPhases(preview)}, func(tui.ApplyProgress) {}); err == nil {
		t.Fatal("drifted mixed deactivation unexpectedly applied")
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("drift preview mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func requiredTUIPhases(preview tui.Preview) []string {
	result := []string{}
	for _, phase := range preview.Phases {
		if phase.ApprovalRequired {
			result = append(result, phase.Kind)
		}
	}
	return result
}

func findTUIPack(packs []tui.Pack, id string) *tui.Pack {
	for _, pack := range packs {
		if pack.ID == id {
			return &pack
		}
	}
	return nil
}
