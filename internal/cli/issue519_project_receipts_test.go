package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue519ProjectPacksUseIndependentReceipts(t *testing.T) {
	first := testsupport.ExternalTool("receipt-alpha")
	second := testsupport.PortableAllSurfaces("receipt-beta")
	firstRoot := first.OperationalResource()
	secondRoot := second.OperationalResource()
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, first, second)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }

	installs := [][]string{
		{"install", first.ID(), "--surface", "codex", "--resource", firstRoot.Kind + ":" + firstRoot.ID},
		{"install", second.ID(), "--surface", "codex", "--resource", secondRoot.Kind + ":" + secondRoot.ID},
	}
	for _, args := range installs {
		if out, err := executeCommand(t, NewRootCommand(fixture.options), args...); err != nil {
			t.Fatalf("install %s: %v\n%s", args[2], err, out)
		}
	}

	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Manifest.Packs) != 2 {
		t.Fatalf("manifest packs = %#v, want two independent direct Packs", installation.Manifest.Packs)
	}
	statusOutput, err := executeCommand(t, NewRootCommand(fixture.options), "status", "--project", "--json")
	if err != nil {
		t.Fatalf("project status: %v\n%s", err, statusOutput)
	}
	var status capabilitypack.JSONProjectStatusReport
	if err := json.Unmarshal([]byte(statusOutput), &status); err != nil || len(status.Packs) != 2 {
		t.Fatalf("independent project status: %v %#v\n%s", err, status, statusOutput)
	}

	lockData, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(lockData, &topLevel); err != nil || len(topLevel) != 2 || topLevel["schema_version"] == nil || topLevel["receipts"] == nil {
		t.Fatalf("project lock is not the minimal receipt document: %v %#v", err, topLevel)
	}
	var lock struct {
		SchemaVersion int `json:"schema_version"`
		Receipts      []struct {
			Pack struct {
				ID      string `json:"id"`
				Version string `json:"version"`
			} `json:"pack"`
			Surface     string `json:"surface"`
			Resources   []any  `json:"resources"`
			Projections []struct {
				Target string `json:"target"`
				Digest string `json:"digest"`
			} `json:"projections"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("decode project lock: %v\n%s", err, lockData)
	}
	if lock.SchemaVersion != 1 || len(lock.Receipts) != 2 {
		t.Fatalf("project receipt document = %#v\n%s", lock, lockData)
	}
	for _, receipt := range lock.Receipts {
		wantVersion := fixture.pack(t, receipt.Pack.ID).CurrentVersion()
		if receipt.Pack.ID == "" || receipt.Pack.Version != wantVersion || receipt.Surface != "codex" || len(receipt.Resources) == 0 || len(receipt.Projections) == 0 {
			t.Fatalf("incomplete project Pack receipt = %#v\n%s", receipt, lockData)
		}
		for _, projection := range receipt.Projections {
			if projection.Target == "" || projection.Digest == "" {
				t.Fatalf("receipt projection omitted target or digest: %#v\n%s", projection, lockData)
			}
		}
	}
	for _, retired := range []string{"source", "sources", "provider_choices", "resource_graph", "role", "compatibility", "history"} {
		if jsonContainsKey(lockData, retired) {
			t.Fatalf("project lock persisted retired field %q:\n%s", retired, lockData)
		}
	}

	beforeFirst := projectReceiptJSON(t, lockData, first.ID())
	beforeSecond := projectReceiptJSON(t, lockData, second.ID())
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", first.ID(), "--project"); err == nil || !strings.Contains(err.Error(), "--surface is required for project update") {
		t.Fatalf("project update without a surface was not rejected: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", first.ID(), "--surface", "codex", "--project"); err != nil {
		t.Fatalf("update first Pack to the current bundled version: %v\n%s", err, out)
	}
	updatedData, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, updatedData, second.ID()); got != beforeSecond {
		t.Fatalf("updating first Pack changed second receipt\nbefore: %s\nafter:  %s", beforeSecond, got)
	}
	firstSkill := projectSkillReceiptTarget(t, project, beforeFirst, firstRoot)
	originalSkill, err := os.ReadFile(firstSkill)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(firstSkill, []byte("user drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", first.ID(), "--surface", "codex", "--project"); err == nil {
		t.Fatalf("ordinary project update overwrote receipt drift:\n%s", out)
	}
	if drifted, _ := os.ReadFile(firstSkill); string(drifted) != "user drift\n" {
		t.Fatalf("blocked update changed drifted receipt target: %q", drifted)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", first.ID(), "--surface", "codex", "--project", "--force"); err != nil {
		t.Fatalf("force receipt-owned first Pack update: %v\n%s", err, out)
	}
	if restored, _ := os.ReadFile(firstSkill); string(restored) != string(originalSkill) {
		t.Fatal("forced update did not restore the exact receipt-owned projection")
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "uninstall", second.ID()); err != nil {
		t.Fatalf("uninstall second Pack: %v\n%s", err, out)
	}
	afterData, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, afterData, first.ID()); got != beforeFirst {
		t.Fatalf("removing second Pack changed first receipt\nbefore: %s\nafter:  %s", beforeFirst, got)
	}
}

func TestIssue519ProjectInstallationAndPersonalActivationStayIndependent(t *testing.T) {
	installed := testsupport.PortableAllSurfaces("scope-installed")
	external := testsupport.ExternalTool("scope-external")
	installedRoot := installed.OperationalResource()
	externalRoot := external.OperationalResource()
	terminal := &fakeTerminal{interactive: true, approve: true, answers: []bool{true, true, false}}
	fixture := newSyntheticCLIFixture(t, terminal, installed, external)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", installed.ID(), "--surface", "codex", "--resource", installedRoot.Kind+":"+installedRoot.ID); err != nil {
		t.Fatalf("install independent Pack: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", external.ID(), "--surface", "codex", "--resource", externalRoot.Kind+":"+externalRoot.ID); err != nil {
		t.Fatalf("install external-tool Pack while declining activation: %v\n%s", err, out)
	}
	lockPath := filepath.Join(project, "packy.lock.json")
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	installedReceipt := projectReceiptJSON(t, before, installed.ID())
	statePattern := filepath.Join(fixture.home, ".packy", "projects", "*", "state-*-*.json")
	if states, _ := filepath.Glob(statePattern); len(states) != 0 {
		t.Fatalf("project installation persisted personal activation: %v", states)
	}
	statusJSON, err := executeCommand(t, NewRootCommand(fixture.options), "status", external.ID(), "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("inspect installed external requirements: %v\n%s", err, statusJSON)
	}
	var status capabilitypack.JSONProjectStatusReport
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil || len(status.Packs) != 1 || status.Packs[0].Readiness.Usable != capabilitypack.ReadinessFalse {
		t.Fatalf("project external requirement readiness = %#v, %v", status, err)
	}
	externalConditions := 0
	for _, condition := range status.Packs[0].Conditions {
		if condition.Type == capabilitypack.ConditionExternalRequirement {
			externalConditions++
			if condition.Value != capabilitypack.ReadinessFalse || condition.Reason != capabilitypack.ReasonRequirementMissing {
				t.Fatalf("project external requirement condition = %#v", condition)
			}
		}
	}
	if externalConditions != 1 {
		t.Fatalf("project external requirement conditions = %d", externalConditions)
	}

	terminal.answers = nil
	terminal.calls = 0
	terminal.approve = true
	runner := fixture.options.Runner.(*fakeRunner)
	runner.path = map[string]string{"engram": filepath.Join(t.TempDir(), "engram")}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", external.ID(), "--surface", "codex", "--project"); err != nil {
		t.Fatalf("activate only external-tool Pack personally: %v\n%s", err, out)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, after, installed.ID()); got != installedReceipt {
		t.Fatalf("personal activation changed independent installation receipt\nbefore: %s\nafter:  %s", installedReceipt, got)
	}
	states, _ := filepath.Glob(statePattern)
	if len(states) != 1 || filepath.Base(states[0]) != "state-"+external.ID()+"-codex.json" {
		t.Fatalf("personal activation receipts = %v, want only external-tool Pack", states)
	}
}

func TestIssue620CrossPackProjectionCollisionRemainsBlocked(t *testing.T) {
	first, second := testsupport.CollisionPair("collision-alpha", "collision-beta")
	firstRoot := first.OperationalResource()
	secondRoot := second.OperationalResource()
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, first, second)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", first.ID(), "--surface", "codex", "--resource", firstRoot.Kind+":"+firstRoot.ID); err != nil {
		t.Fatalf("install first collision Pack: %v\n%s", err, out)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(fixture.options), "install", second.ID(), "--surface", "codex", "--resource", secondRoot.Kind+":"+secondRoot.ID)
	if err == nil || !strings.Contains(out, "projection_collision") {
		t.Fatalf("install second colliding instruction = %v\n%s", err, out)
	}
	after := snapshotTree(t, project)
	if after != before {
		t.Fatalf("colliding install mutated the project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestIssue519MultiSurfaceUpdatePreservesEveryOtherPackReceipt(t *testing.T) {
	multi := testsupport.PortableAllSurfaces("surface-multi")
	other := testsupport.ExternalTool("surface-other")
	multiRoot := multi.OperationalResource()
	otherRoot := other.OperationalResource()
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, multi, other)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }

	commands := [][]string{
		{"install", multi.ID(), "--surface", "codex", "--resource", multiRoot.Kind + ":" + multiRoot.ID},
		{"install", multi.ID(), "--surface", "claude", "--resource", multiRoot.Kind + ":" + multiRoot.ID},
		{"install", other.ID(), "--surface", "codex", "--resource", otherRoot.Kind + ":" + otherRoot.ID},
	}
	for _, args := range commands {
		if out, err := executeCommand(t, NewRootCommand(fixture.options), args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	lockPath := filepath.Join(project, "packy.lock.json")
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	otherReceipt := projectReceiptJSON(t, before, other.ID())
	retainedClaudeReceipt := projectSurfaceReceiptJSON(t, before, multi.ID(), "claude")
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", multi.ID(), "--surface", "codex", "--project"); err != nil {
		t.Fatalf("Codex synthetic Pack update: %v\n%s", err, out)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, after, other.ID()); got != otherReceipt {
		t.Fatalf("surface update changed other Pack receipt\nbefore: %s\nafter:  %s", otherReceipt, got)
	}
	if got := projectSurfaceReceiptJSON(t, after, multi.ID(), "claude"); got != retainedClaudeReceipt {
		t.Fatalf("Codex update changed the retained Claude receipt\nbefore: %s\nafter:  %s", retainedClaudeReceipt, got)
	}
	retainedCodexReceipt := projectSurfaceReceiptJSON(t, after, multi.ID(), "codex")
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", multi.ID(), "--surface", "claude", "--project"); err != nil {
		t.Fatalf("Claude synthetic Pack update: %v\n%s", err, out)
	}
	after, err = os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectSurfaceReceiptJSON(t, after, multi.ID(), "codex"); got != retainedCodexReceipt {
		t.Fatalf("Claude update changed the retained Codex receipt\nbefore: %s\nafter:  %s", retainedCodexReceipt, got)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Manifest.Packs) != 2 {
		t.Fatalf("multi-surface update retained manifest Packs = %#v", installation.Manifest.Packs)
	}
}

func TestIssue626ProjectUpdateRetainsIndependentSurfaceVersionsAndContent(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("independent-update")
	root := pack.OperationalResource()
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }
	packID := pack.ID()
	for _, surface := range []string{"codex", "claude"} {
		if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", packID, "--surface", surface, "--resource", root.Kind+":"+root.ID); err != nil {
			t.Fatalf("install synthetic Pack on %s: %v\n%s", surface, err, out)
		}
	}
	candidate := pack.Candidate()
	if err := candidate.WriteCatalog(fixture.catalogRoot); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", packID, "--surface", "codex", "--project"); err != nil {
		t.Fatalf("compatible Codex update: %v\n%s", err, out)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	versions := map[capabilitypack.Surface]string{}
	for _, receipt := range installation.Lock.Receipts {
		if receipt.Pack.ID == packID {
			versions[receipt.Surface] = receipt.Pack.Version
		}
	}
	if versions[capabilitypack.SurfaceCodex] != candidate.CurrentVersion() || versions[capabilitypack.SurfaceClaude] != pack.CurrentVersion() {
		t.Fatalf("independent update did not retain per-surface versions: %#v", versions)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", packID, "--surface", "claude", "--project"); err != nil {
		t.Fatalf("compatible Claude update: %v\n%s", err, out)
	}
	changed := candidate.Candidate().WithAdaptedBytes(
		root.Kind+":"+root.ID, ".",
		[]byte("# Changed guidance\n\nChanged synthetic guidance.\n"),
	)
	if err := changed.WriteCatalog(fixture.catalogRoot); err != nil {
		t.Fatal(err)
	}
	claudeBefore := snapshotTree(t, filepath.Join(project, "CLAUDE.md"))
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", packID, "--surface", "codex", "--project"); err != nil {
		t.Fatalf("independent Codex update: %v\n%s", err, out)
	}
	if after := snapshotTree(t, filepath.Join(project, "CLAUDE.md")); after != claudeBefore {
		t.Fatal("Codex update changed the retained Claude instructions")
	}
}

func TestIssue626ProjectUpdateRetiresOnlyItsOwnNativeProjection(t *testing.T) {
	workflow := testsupport.ResourceIdentity{Kind: "skill", ID: "workflow"}
	helper := testsupport.ResourceIdentity{Kind: "skill", ID: "helper"}
	pack := testsupport.CapabilityRich("retirement-shared").WithRetainedRequirements(workflow, helper)
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }
	packID := pack.ID()
	for _, surface := range []string{"codex", "claude"} {
		if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", packID, "--surface", surface, "--resource", workflow.String()); err != nil {
			t.Fatalf("install dependency fixture on %s: %v\n%s", surface, err, out)
		}
	}
	retiredPath := filepath.Join(project, ".claude", "skills", "helper", "SKILL.md")
	if _, err := os.Stat(retiredPath); err != nil {
		t.Fatalf("shared dependency was not installed: %v", err)
	}
	candidate := pack.Candidate().WithRetainedRequirements(workflow)
	if err := candidate.WriteCatalog(fixture.catalogRoot); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(project, "packy.lock.json")
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	retainedClaude := projectSurfaceReceiptJSON(t, before, packID, "claude")
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", packID, "--surface", "codex", "--project"); err != nil {
		t.Fatalf("Codex dependency retirement: %v\n%s", err, out)
	}
	if _, err := os.Stat(retiredPath); err != nil {
		t.Fatalf("Codex update removed a projection retained by Claude: %v", err)
	}
	afterCodex, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectSurfaceReceiptJSON(t, afterCodex, packID, "claude"); got != retainedClaude {
		t.Fatalf("Codex retirement changed the Claude receipt\nbefore: %s\nafter:  %s", retainedClaude, got)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", packID, "--surface", "claude", "--project"); err != nil {
		t.Fatalf("Claude dependency retirement: %v\n%s", err, out)
	}
	if _, err := os.Stat(retiredPath); !os.IsNotExist(err) {
		t.Fatalf("last surface update retained the retired projection: %v", err)
	}
}

func TestIssue626ProjectNoticesUseOnlyTheSelectedSurfaceIntent(t *testing.T) {
	pack := testsupport.CapabilityRich("notice-selection")
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	manifest := pack.Manifest()
	manifest.Resources = append(manifest.Resources, testsupport.Resource{
		Kind: "skill", ID: "local", Source: "skills/local", Description: "Authored skill without external notices",
		Requires: []string{}, Conflicts: []string{}, Notices: []string{}, SurfaceExclusions: []testsupport.SurfaceExclusion{},
		Bindings: []testsupport.Binding{
			{Surface: testsupport.SurfaceClaude, Projection: "skill", Name: "local", Invocation: "/local", Mode: "native", Sharing: "exclusive", Capabilities: []testsupport.Capability{}},
			{Surface: testsupport.SurfaceCodex, Projection: "skill", Name: "local", Invocation: "$local", Mode: "native", Sharing: "exclusive", Capabilities: []testsupport.Capability{}},
		},
	})
	slices.SortFunc(manifest.Resources, func(a, b testsupport.Resource) int {
		return strings.Compare(a.Kind+":"+a.ID, b.Kind+":"+b.ID)
	})
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeAuthoringFile(t, filepath.Join(fixture.catalogRoot, "packs", pack.ID(), "pack.json"), string(manifestJSON))
	writeAuthoringFile(t, filepath.Join(fixture.catalogRoot, "packs", pack.ID(), "skills/local/SKILL.md"), "---\nname: local\ndescription: Authored skill.\n---\nLocal guidance.\n")
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }
	packID := pack.ID()
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", packID, "--surface", "codex", "--resource", "skill:helper"); err != nil {
		t.Fatalf("install Codex notice selection: %v\n%s", err, out)
	}
	previewJSON, err := executeCommand(t, NewRootCommand(fixture.options), "install", packID, "--surface", "claude", "--resource", "skill:local", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("preview Claude no-notice selection: %v\n%s", err, previewJSON)
	}
	var preview capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(previewJSON), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Notices.Contributions) != 0 {
		t.Fatalf("Claude preview inherited Codex notices: %#v", preview.Notices.Contributions)
	}
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", packID, "--surface", "claude", "--resource", "skill:local"); err != nil {
		t.Fatalf("install Claude no-notice selection: %v\n%s", err, out)
	}
	notices, err := os.ReadFile(filepath.Join(project, "PACKY-NOTICES.md"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(notices), "<!-- packy:project:"+packID+":claude:notices:start -->")
	end := strings.Index(string(notices), "<!-- packy:project:"+packID+":claude:notices:end -->")
	if start < 0 || end <= start || strings.Contains(string(notices)[start:end], "Packy Fixture Authors") {
		t.Fatalf("Claude notice block inherited the Codex attribution:\n%s", notices)
	}
}

func TestIssue519DriftedNoticeBlocksReceiptRemoval(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("notice-drift")
	root := pack.OperationalResource()
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	fixture.options.Getwd = func() (string, error) { return project, nil }
	packID := pack.ID()
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "install", packID, "--surface", "claude", "--resource", root.Kind+":"+root.ID); err != nil {
		t.Fatalf("install synthetic notices: %v\n%s", err, out)
	}
	noticesPath := filepath.Join(project, "PACKY-NOTICES.md")
	notices, err := os.ReadFile(noticesPath)
	if err != nil {
		t.Fatal(err)
	}
	drifted := strings.Replace(string(notices), "Packy Fixture Authors", "locally changed attribution", 1)
	if drifted == string(notices) {
		t.Fatal("notice fixture did not contain the expected attribution")
	}
	if err := os.WriteFile(noticesPath, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(fixture.options), "uninstall", packID)
	if err == nil || !strings.Contains(out, "project_drift") {
		t.Fatalf("drifted notice uninstall was not blocked: %v\n%s", err, out)
	}
	if after := snapshotTree(t, project); after != before {
		t.Fatal("blocked drifted-notice uninstall mutated the project")
	}
}

func projectReceiptJSON(t *testing.T, data []byte, packID string) string {
	t.Helper()
	var document struct {
		Receipts []json.RawMessage `json:"receipts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range document.Receipts {
		var identity struct {
			Pack struct {
				ID string `json:"id"`
			} `json:"pack"`
		}
		if err := json.Unmarshal(receipt, &identity); err != nil {
			t.Fatal(err)
		}
		if identity.Pack.ID == packID {
			return string(receipt)
		}
	}
	t.Fatalf("receipt for %s not found in %s", packID, data)
	return ""
}

func projectSkillReceiptTarget(t *testing.T, project, receiptJSON string, resource testsupport.ResourceIdentity) string {
	t.Helper()
	var receipt struct {
		Projections []struct {
			ID     string `json:"id"`
			Target string `json:"target"`
		} `json:"projections"`
	}
	if err := json.Unmarshal([]byte(receiptJSON), &receipt); err != nil {
		t.Fatal(err)
	}
	for _, projection := range receipt.Projections {
		if projection.ID == resource.String() && projection.Target != "" {
			return filepath.Join(project, filepath.FromSlash(projection.Target), "SKILL.md")
		}
	}
	t.Fatalf("receipt projections = %#v, want target for %s", receipt.Projections, resource)
	return ""
}

func projectSurfaceReceiptJSON(t *testing.T, data []byte, packID, surface string) string {
	t.Helper()
	var document struct {
		Receipts []json.RawMessage `json:"receipts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range document.Receipts {
		var identity struct {
			Pack struct {
				ID string `json:"id"`
			} `json:"pack"`
			Surface string `json:"surface"`
		}
		if err := json.Unmarshal(receipt, &identity); err != nil {
			t.Fatal(err)
		}
		if identity.Pack.ID == packID && identity.Surface == surface {
			return string(receipt)
		}
	}
	t.Fatalf("receipt for %s on %s not found in %s", packID, surface, data)
	return ""
}
