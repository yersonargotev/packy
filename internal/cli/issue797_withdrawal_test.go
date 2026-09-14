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
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue797WithdrawnPackRemainsInspectableAndDeactivatable(t *testing.T) {
	withdrawn := testsupport.PortableAllSurfaces("withdrawn-runtime")
	blockedActivation := testsupport.PortableAllSurfaces("withdrawn-inactive-surface")
	remaining := testsupport.PortableAllSurfaces("catalog-current")
	retainedSnapshot := strings.Repeat("7", 40)
	currentSnapshot := strings.Repeat("8", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, retainedSnapshot, withdrawn, blockedActivation, remaining)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, withdrawn, blockedActivation, remaining)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env = env
	opts.CatalogSource = source
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(opts), "init"); err != nil {
		t.Fatalf("initialize retained snapshot: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install", withdrawn.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("install Pack in project before withdrawal: %v\n%s", err, out)
	}
	for _, surface := range []string{"claude", "codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "activate", withdrawn.ID(), "--surface", surface); err != nil {
			t.Fatalf("activate Pack on %s before withdrawal: %v\n%s", surface, err, out)
		}
	}
	source.release = catalogReleaseFixture(t, currentSnapshot, remaining)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil {
		t.Fatalf("select snapshot that withdraws Pack: %v\n%s", err, out)
	}

	statePath := filepath.Join(fixture.home, ".packy", "packs.json")
	originalState, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var stateDocument map[string]any
	if err := json.Unmarshal(originalState, &stateDocument); err != nil {
		t.Fatal(err)
	}
	for _, raw := range stateDocument["receipts"].([]any) {
		receipt := raw.(map[string]any)
		if receipt["surface"] == "claude" {
			receipt["pack"].(map[string]any)["catalog_snapshot"] = strings.Repeat("9", 40)
		}
	}
	corruptClaudeState, err := json.MarshalIndent(stateDocument, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(corruptClaudeState, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "status", withdrawn.ID(), "--surface", "codex", "--json"); err != nil {
		t.Fatalf("unresolvable retained Claude receipt blocked Codex status: %v\n%s", err, out)
	}
	if err := os.WriteFile(statePath, originalState, 0o600); err != nil {
		t.Fatal(err)
	}

	beforeRejectedActivation := snapshotTree(t, fixture.home)
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", blockedActivation.ID(), "--surface", "opencode", "--dry-run"); err == nil || !strings.Contains(err.Error(), "current catalog") {
		t.Fatalf("new activation from withdrawn Pack = %v\n%s", err, out)
	}
	if after := snapshotTree(t, fixture.home); after != beforeRejectedActivation {
		t.Fatal("rejected activation mutated Packy Home or host projections")
	}

	for _, surface := range []string{"claude", "codex", "opencode"} {
		statusOutput, err := executeCommand(t, NewRootCommand(opts), "status", withdrawn.ID(), "--surface", surface, "--json")
		if err != nil {
			t.Fatalf("inspect withdrawn Pack status on %s: %v\n%s", surface, err, statusOutput)
		}
		var status capabilitypack.JSONStatusReport
		if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
			t.Fatal(err)
		}
		if len(status.Entries) != 1 || status.Entries[0].PackVersion != withdrawn.CurrentVersion() || status.Entries[0].Intent.Version != withdrawn.CurrentVersion() || !status.Entries[0].HistoricalEvidence.Available || status.Entries[0].UpdateAvailable {
			t.Fatalf("withdrawn status on %s = %#v", surface, status.Entries)
		}
	}

	showOutput, err := executeCommand(t, NewRootCommand(opts), "show", withdrawn.ID(), "--json")
	if err != nil {
		t.Fatalf("show withdrawn Pack: %v\n%s", err, showOutput)
	}
	var show packShowJSON
	if err := json.Unmarshal([]byte(showOutput), &show); err != nil {
		t.Fatal(err)
	}
	if show.CatalogState != "retained" || show.Version != withdrawn.CurrentVersion() || show.LifecycleAvailability.FreshActivationAvailable || show.LifecycleAvailability.CatalogUpdateAvailable || !show.LifecycleAvailability.LifecycleVerbsAvailable || show.LifecycleAvailability.AutomaticDowngrade {
		t.Fatalf("withdrawn show = %#v", show)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "update", withdrawn.ID(), "--surface", "codex", "--dry-run"); err == nil || !strings.Contains(err.Error(), "current catalog") {
		t.Fatalf("withdrawn update = %v\n%s", err, out)
	}

	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	installed := findTUIPack(dashboard.Global.Packs, withdrawn.ID())
	if installed == nil || installed.Version != withdrawn.CurrentVersion() {
		t.Fatalf("TUI omitted retained Pack: %#v", dashboard.Global.Packs)
	}
	var installedStatus *tui.SurfaceStatus
	for index := range installed.SurfaceStatuses {
		if installed.SurfaceStatuses[index].Name == "codex" {
			installedStatus = &installed.SurfaceStatuses[index]
			break
		}
	}
	if installedStatus == nil || installedStatus.Supported || !installedStatus.Active || installedStatus.UpdateAvailable || installedStatus.CatalogUpdateAvailable || installedStatus.InstalledVersion != withdrawn.CurrentVersion() {
		t.Fatalf("TUI retained status = %#v", installedStatus)
	}
	projectPack := findTUIPack(dashboard.Project.Packs, withdrawn.ID())
	if projectPack == nil || projectPack.CatalogState != "retained" {
		t.Fatalf("TUI omitted retained project Pack: %#v", dashboard.Project.Packs)
	}
	projectStatus := slices.IndexFunc(projectPack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if projectStatus < 0 || projectPack.SurfaceStatuses[projectStatus].Installation != "installed" || projectPack.SurfaceStatuses[projectStatus].Supported {
		t.Fatalf("TUI retained project status = %#v", projectPack.SurfaceStatuses)
	}

	for _, surface := range []string{"claude", "codex", "opencode"} {
		preview, err := backend.Preview(context.Background(), tui.PreviewRequest{
			Operation: "deactivate", PackID: withdrawn.ID(), Surface: surface, Scope: "global", Selection: tui.Selection{Mode: "all"},
		})
		if err != nil {
			t.Fatalf("preview retained deactivation on %s: %v", surface, err)
		}
		result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: requiredTUIPhases(preview)}, func(tui.ApplyProgress) {})
		if err != nil || !result.Verified {
			t.Fatalf("apply retained deactivation on %s = %#v, %v", surface, result, err)
		}
	}
	if _, err := os.Stat(filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog", "snapshots", retainedSnapshot)); err != nil {
		t.Fatalf("retained Catalog Snapshot was removed after deactivation: %v", err)
	}
	projectPreview, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "uninstall", PackID: withdrawn.ID(), Surface: "codex", Scope: "project", ProjectRoot: project,
	})
	if err != nil {
		t.Fatalf("preview retained project uninstall: %v", err)
	}
	projectResult, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: projectPreview, ApprovedPhases: requiredTUIPhases(projectPreview)}, func(tui.ApplyProgress) {})
	if err != nil || !projectResult.Verified {
		t.Fatalf("apply retained project uninstall = %#v, %v", projectResult, err)
	}
}

func TestIssue797WithdrawnOrphanedProjectActivationRemainsInTUI(t *testing.T) {
	withdrawn := testsupport.PortableAllSurfaces("withdrawn-project-orphan")
	remaining := testsupport.PortableAllSurfaces("orphan-current")
	retainedSnapshot := strings.Repeat("b", 40)
	currentSnapshot := strings.Repeat("c", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, retainedSnapshot, withdrawn, remaining)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, withdrawn, remaining)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env, opts.CatalogSource = env, source
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	for _, command := range [][]string{{"init"}, {"install", withdrawn.ID(), "--surface", "claude"}} {
		if out, err := executeCommand(t, NewRootCommand(opts), command...); err != nil {
			t.Fatalf("%v: %v\n%s", command, err, out)
		}
	}
	claudeManifest, err := os.ReadFile(filepath.Join(project, "packy.json"))
	if err != nil {
		t.Fatal(err)
	}
	claudeLock, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install", withdrawn.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("install Codex surface before personal activation: %v\n%s", err, out)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	resource := installation.Lock.ResourceGraph.Resources[0].Resource
	receiptIndex := -1
	for index := range installation.Lock.Receipts {
		if installation.Lock.Receipts[index].Surface == capabilitypack.SurfaceCodex {
			receiptIndex = index
			break
		}
	}
	if receiptIndex < 0 {
		t.Fatal("Codex project receipt is missing")
	}
	installation.Lock.Receipts[receiptIndex].Sensitive = []capabilitypack.ProjectSensitiveDisclosure{{
		Category: capabilitypack.ProjectActivationTrust, Surface: capabilitypack.SurfaceCodex, Resource: resource, Detail: "project-trust",
	}}
	lock, err := json.MarshalIndent(installation.Lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), append(lock, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", withdrawn.ID(), "--surface", "codex", "--project"); err != nil {
		t.Fatalf("activate project Pack before orphaning: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.json"), claudeManifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), claudeLock, 0o644); err != nil {
		t.Fatal(err)
	}
	blockedOutput, err := executeCommand(t, NewRootCommand(opts), "install", withdrawn.ID(), "--surface", "opencode", "--dry-run", "--json")
	if err == nil || !strings.Contains(err.Error(), "project install preview is not actionable: blocked") || !strings.Contains(blockedOutput, `"code":"orphaned_personal_activation"`) {
		t.Fatalf("install while orphan remains was not blocked: %v\n%s", err, blockedOutput)
	}
	installationAfter, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil || len(installationAfter.Manifest.Packs) != 1 || !slices.Equal(installationAfter.Manifest.Packs[0].Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceClaude}) {
		t.Fatalf("blocked install changed remaining project contract: %#v, %v", installationAfter.Manifest.Packs, err)
	}
	for _, command := range [][]string{{"update", withdrawn.ID(), "--surface", "claude", "--project", "--dry-run", "--json"}, {"activate", withdrawn.ID(), "--surface", "claude", "--project", "--dry-run"}} {
		out, commandErr := executeCommand(t, NewRootCommand(opts), command...)
		if commandErr == nil || (!strings.Contains(commandErr.Error(), "blocked") && !strings.Contains(commandErr.Error(), "personal project activation on codex remains")) {
			t.Fatalf("%v bypassed orphan guard: %v\n%s", command, commandErr, out)
		}
	}
	source.release = catalogReleaseFixture(t, currentSnapshot, remaining)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil {
		t.Fatalf("withdraw orphaned project Pack: %v\n%s", err, out)
	}

	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pack := findTUIPack(dashboard.Project.Packs, withdrawn.ID())
	if pack == nil || pack.CatalogState != "retained" {
		t.Fatalf("TUI omitted orphaned withdrawn project Pack: %#v", dashboard.Project.Packs)
	}
	statusIndex := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if statusIndex < 0 || pack.SurfaceStatuses[statusIndex].Runtime != "orphaned" || pack.SurfaceStatuses[statusIndex].Installation != "absent" || pack.SurfaceStatuses[statusIndex].Supported {
		t.Fatalf("orphaned withdrawn project status = %#v", pack.SurfaceStatuses)
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "deactivate", PackID: withdrawn.ID(), Surface: "codex", Scope: "project", ProjectRoot: project})
	if err != nil || preview.Operation != "deactivate" || preview.Disposition != "previewable" {
		t.Fatalf("orphaned withdrawn project deactivation preview = %#v, %v", preview, err)
	}
}

func TestIssue797WithdrawnShowKeepsExactContractIdentityPerSurface(t *testing.T) {
	v1 := testsupport.PortableAllSurfaces("withdrawn-multiversion")
	v2 := v1.Candidate()
	remaining := testsupport.PortableAllSurfaces("still-current")
	firstSnapshot := strings.Repeat("1", 40)
	secondSnapshot := strings.Repeat("2", 40)
	withdrawnSnapshot := strings.Repeat("3", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, firstSnapshot, v1, remaining)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, v1, remaining)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env, opts.CatalogSource = env, source

	if out, err := executeCommand(t, NewRootCommand(opts), "init"); err != nil {
		t.Fatalf("initialize first snapshot: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", v1.ID(), "--surface", "claude"); err != nil {
		t.Fatalf("activate v1 on Claude: %v\n%s", err, out)
	}
	source.release = catalogReleaseFixture(t, secondSnapshot, v2, remaining)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil {
		t.Fatalf("select v2 snapshot: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", v2.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("activate v2 on Codex: %v\n%s", err, out)
	}
	currentOutput, err := executeCommand(t, NewRootCommand(opts), "show", v1.ID(), "--json")
	if err != nil {
		t.Fatalf("show mixed retained/current versions: %v\n%s", err, currentOutput)
	}
	var currentDocument packShowJSON
	if err := json.Unmarshal([]byte(currentOutput), &currentDocument); err != nil {
		t.Fatal(err)
	}
	currentVersions := map[capabilitypack.Surface]string{}
	for _, surface := range currentDocument.SurfaceContracts {
		currentVersions[surface.Surface] = surface.CatalogIdentity.Version
	}
	if currentVersions[capabilitypack.SurfaceClaude] != v1.CurrentVersion() || currentVersions[capabilitypack.SurfaceCodex] != v2.CurrentVersion() {
		t.Fatalf("current surface contract identities = %#v", currentVersions)
	}
	statusOutput, err := executeCommand(t, NewRootCommand(opts), "status", v1.ID(), "--surface", "claude", "--json")
	if err != nil {
		t.Fatalf("status retained Claude version against current v2: %v\n%s", err, statusOutput)
	}
	var status capabilitypack.JSONStatusReport
	if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Entries) != 1 || status.Entries[0].PackVersion != v2.CurrentVersion() || status.Entries[0].Intent.Version != v1.CurrentVersion() || !status.Entries[0].UpdateAvailable || !status.Entries[0].HistoricalEvidence.Available {
		t.Fatalf("current catalog status lost retained evidence: %#v", status.Entries)
	}
	source.release = catalogReleaseFixture(t, withdrawnSnapshot, remaining)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil {
		t.Fatalf("select withdrawing snapshot: %v\n%s", err, out)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "show", v1.ID(), "--json")
	if err != nil {
		t.Fatalf("show multi-version retained Pack: %v\n%s", err, out)
	}
	var document packShowJSON
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatal(err)
	}
	versions := map[capabilitypack.Surface]string{}
	for _, surface := range document.SurfaceContracts {
		versions[surface.Surface] = surface.CatalogIdentity.Version
	}
	if versions[capabilitypack.SurfaceClaude] != v1.CurrentVersion() || versions[capabilitypack.SurfaceCodex] != v2.CurrentVersion() {
		t.Fatalf("retained surface contract identities = %#v", versions)
	}
}

func TestIssue797ProjectStatusComposesRetainedGlobalSnapshot(t *testing.T) {
	pack := testsupport.CapabilityRich("withdrawn-composition")
	remaining := testsupport.PortableAllSurfaces("composition-current")
	retainedSnapshot := strings.Repeat("4", 40)
	currentSnapshot := strings.Repeat("5", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, retainedSnapshot, pack, remaining)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack, remaining)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env, opts.CatalogSource = env, source
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	for _, command := range [][]string{{"init"}, {"install", pack.ID(), "--surface", "codex", "--resource", "skill:helper"}} {
		if out, err := executeCommand(t, NewRootCommand(opts), command...); err != nil {
			t.Fatalf("%v: %v\n%s", command, err, out)
		}
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	resource := installation.Lock.ResourceGraph.Resources[0].Resource
	installation.Lock.Receipts[0].Sensitive = []capabilitypack.ProjectSensitiveDisclosure{{
		Category: capabilitypack.ProjectActivationMCP, Surface: capabilitypack.SurfaceCodex, Resource: resource, Detail: "project-only-runtime-consent",
	}}
	lock, err := json.MarshalIndent(installation.Lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), append(lock, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", pack.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("activate global Pack: %v\n%s", err, out)
	}
	source.release = catalogReleaseFixture(t, currentSnapshot, remaining)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil {
		t.Fatalf("withdraw Pack: %v\n%s", err, out)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "status", pack.ID(), "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("compose retained global and project receipts: %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectStatusReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Packs) != 1 || report.Packs[0].Installation != capabilitypack.ProjectInstallationInstalled || len(report.Packs[0].RuntimeEffects) != 1 || report.Packs[0].RuntimeEffects[0].GlobalVersion != pack.CurrentVersion() {
		t.Fatalf("retained project composition = %#v", report.Packs)
	}
}

func TestIssue797RemovedSurfaceKeepsEvidenceWithoutOfferingUpdate(t *testing.T) {
	v1 := testsupport.PortableAllSurfaces("removed-surface-version")
	v2 := v1.Candidate().WithSurfaces(testsupport.SurfaceOpenCode)
	firstSnapshot := strings.Repeat("6", 40)
	secondSnapshot := strings.Repeat("a", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, firstSnapshot, v1)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, v1)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env, opts.CatalogSource = env, source
	if out, err := executeCommand(t, NewRootCommand(opts), "init"); err != nil {
		t.Fatalf("initialize v1: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", v1.ID(), "--surface", "claude"); err != nil {
		t.Fatalf("activate v1 on Claude: %v\n%s", err, out)
	}
	source.release = catalogReleaseFixture(t, secondSnapshot, v2)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil {
		t.Fatalf("select v2 without Claude: %v\n%s", err, out)
	}

	statusOutput, err := executeCommand(t, NewRootCommand(opts), "status", v1.ID(), "--surface", "claude", "--json")
	if err != nil {
		t.Fatalf("status removed Claude surface: %v\n%s", err, statusOutput)
	}
	var status capabilitypack.JSONStatusReport
	if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Entries) != 1 || !status.Entries[0].HistoricalEvidence.Available || !status.Entries[0].UpdateAvailable {
		t.Fatalf("removed surface status = %#v", status.Entries)
	}
	humanStatus, err := executeCommand(t, NewRootCommand(opts), "status")
	if err != nil || !strings.Contains(humanStatus, "update unavailable on this surface") {
		t.Fatalf("removed surface status action: %v\n%s", err, humanStatus)
	}
	showOutput, err := executeCommand(t, NewRootCommand(opts), "show", v1.ID(), "--json")
	if err != nil {
		t.Fatalf("show removed Claude surface: %v\n%s", err, showOutput)
	}
	var document packShowJSON
	if err := json.Unmarshal([]byte(showOutput), &document); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(document.Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceOpenCode}) {
		t.Fatalf("current supported surfaces = %#v", document.Surfaces)
	}
	claudeContract := slices.IndexFunc(document.SurfaceContracts, func(contract packShowSurfaceJSON) bool { return contract.Surface == capabilitypack.SurfaceClaude })
	if claudeContract < 0 || document.SurfaceContracts[claudeContract].CatalogIdentity.Version != v1.CurrentVersion() {
		t.Fatalf("retained Claude contract = %#v", document.SurfaceContracts)
	}
	human, err := executeCommand(t, NewRootCommand(opts), "show", v1.ID())
	if err != nil || strings.Contains(human, "packy update "+v1.ID()+" --surface claude") || !strings.Contains(human, "packy status "+v1.ID()+" --surface claude") {
		t.Fatalf("removed surface decision summary: %v\n%s", err, human)
	}
	dashboard, err := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults())).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	view := findTUIPack(dashboard.Global.Packs, v1.ID())
	if view == nil {
		t.Fatalf("TUI omitted current Pack with retained Claude surface: %#v", dashboard.Global.Packs)
	}
	index := slices.IndexFunc(view.SurfaceStatuses, func(surface tui.SurfaceStatus) bool { return surface.Name == "claude" })
	if index < 0 || view.SurfaceStatuses[index].Supported || view.SurfaceStatuses[index].UpdateAvailable || !view.SurfaceStatuses[index].CatalogUpdateAvailable {
		t.Fatalf("removed surface TUI status = %#v", view)
	}
}
