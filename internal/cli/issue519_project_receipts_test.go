package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue519ProjectPacksUseIndependentReceipts(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	installs := [][]string{
		{"install", "addy", "--surface", "codex", "--resource", "skill:api-and-interface-design"},
		{"install", "argote", "--surface", "codex", "--resource", "instruction:guidance"},
	}
	for _, args := range installs {
		if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
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
	statusOutput, err := executeCommand(t, NewRootCommand(opts), "status", "--project", "--json")
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
		wantVersion := "1.0.1"
		if receipt.Pack.ID == "argote" {
			wantVersion = "1.0.2"
		}
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

	beforeAddy := projectReceiptJSON(t, lockData, "addy")
	beforeArgote := projectReceiptJSON(t, lockData, "argote")
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "addy", "--project"); err == nil || !strings.Contains(err.Error(), "--surface is required for project update") {
		t.Fatalf("project update without a surface was not rejected: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "addy", "--surface", "codex", "--project"); err != nil {
		t.Fatalf("update Addy to the current bundled version: %v\n%s", err, out)
	}
	updatedData, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, updatedData, "argote"); got != beforeArgote {
		t.Fatalf("updating Addy changed Argote receipt\nbefore: %s\nafter:  %s", beforeArgote, got)
	}
	addySkill := filepath.Join(project, ".agents", "skills", "api-and-interface-design", "SKILL.md")
	originalSkill, err := os.ReadFile(addySkill)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(addySkill, []byte("user drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "addy", "--surface", "codex", "--project"); err == nil {
		t.Fatalf("ordinary project update overwrote receipt drift:\n%s", out)
	}
	if drifted, _ := os.ReadFile(addySkill); string(drifted) != "user drift\n" {
		t.Fatalf("blocked update changed drifted receipt target: %q", drifted)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "addy", "--surface", "codex", "--project", "--force"); err != nil {
		t.Fatalf("force receipt-owned Addy update: %v\n%s", err, out)
	}
	if restored, _ := os.ReadFile(addySkill); string(restored) != string(originalSkill) {
		t.Fatal("forced update did not restore the exact receipt-owned Addy projection")
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "uninstall", "argote"); err != nil {
		t.Fatalf("uninstall argote: %v\n%s", err, out)
	}
	afterData, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, afterData, "addy"); got != beforeAddy {
		t.Fatalf("removing Argote changed Addy receipt\nbefore: %s\nafter:  %s", beforeAddy, got)
	}
}

func TestIssue519ProjectInstallationAndPersonalActivationStayIndependent(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true, answers: []bool{true, true, false}}
	opts, home, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(opts), "install", "addy", "--surface", "codex", "--resource", "skill:api-and-interface-design"); err != nil {
		t.Fatalf("install Addy: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install", "engram", "--surface", "codex", "--resource", "instruction:engram-memory"); err != nil {
		t.Fatalf("install Engram while declining activation: %v\n%s", err, out)
	}
	lockPath := filepath.Join(project, "packy.lock.json")
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	addyReceipt := projectReceiptJSON(t, before, "addy")
	statePattern := filepath.Join(home, ".packy", "projects", "*", "state-*-*.json")
	if states, _ := filepath.Glob(statePattern); len(states) != 0 {
		t.Fatalf("project installation persisted personal activation: %v", states)
	}
	statusJSON, err := executeCommand(t, NewRootCommand(opts), "status", "engram", "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("inspect installed Engram requirements: %v\n%s", err, statusJSON)
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
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", "engram", "--surface", "codex", "--project"); err != nil {
		t.Fatalf("activate only Engram personally: %v\n%s", err, out)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, after, "addy"); got != addyReceipt {
		t.Fatalf("personal Engram activation changed Addy installation receipt\nbefore: %s\nafter:  %s", addyReceipt, got)
	}
	states, _ := filepath.Glob(statePattern)
	if len(states) != 1 || filepath.Base(states[0]) != "state-engram-codex.json" {
		t.Fatalf("personal activation receipts = %v, want only Engram", states)
	}
}

func TestIssue620CrossPackProjectionCollisionRemainsBlocked(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex", "--resource", "skill:ask-matt"); err != nil {
		t.Fatalf("install Matty: %v\n%s", err, out)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "install", "argote", "--surface", "codex", "--resource", "instruction:guidance")
	if err == nil || !strings.Contains(out, "projection_collision") {
		t.Fatalf("install colliding Argote instruction = %v\n%s", err, out)
	}
	after := snapshotTree(t, project)
	if after != before {
		t.Fatalf("colliding install mutated the project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestIssue519MultiSurfaceUpdatePreservesEveryOtherPackReceipt(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	commands := [][]string{
		{"install", "matty", "--surface", "codex", "--resource", "skill:ask-matt"},
		{"install", "matty", "--surface", "opencode", "--resource", "skill:ask-matt"},
		{"install", "addy", "--surface", "codex", "--resource", "skill:api-and-interface-design"},
	}
	for _, args := range commands {
		if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	lockPath := filepath.Join(project, "packy.lock.json")
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	addyReceipt := projectReceiptJSON(t, before, "addy")
	mattyOpenCodeReceipt := projectSurfaceReceiptJSON(t, before, "matty", "opencode")
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "codex", "--project"); err != nil {
		t.Fatalf("Codex Matty update: %v\n%s", err, out)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, after, "addy"); got != addyReceipt {
		t.Fatalf("surface update changed Addy receipt\nbefore: %s\nafter:  %s", addyReceipt, got)
	}
	if got := projectSurfaceReceiptJSON(t, after, "matty", "opencode"); got != mattyOpenCodeReceipt {
		t.Fatalf("Codex update changed the retained OpenCode receipt\nbefore: %s\nafter:  %s", mattyOpenCodeReceipt, got)
	}
	mattyCodexReceipt := projectSurfaceReceiptJSON(t, after, "matty", "codex")
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "opencode", "--project"); err != nil {
		t.Fatalf("OpenCode Matty update: %v\n%s", err, out)
	}
	after, err = os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectSurfaceReceiptJSON(t, after, "matty", "codex"); got != mattyCodexReceipt {
		t.Fatalf("OpenCode update changed the retained Codex receipt\nbefore: %s\nafter:  %s", mattyCodexReceipt, got)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Manifest.Packs) != 2 {
		t.Fatalf("multi-surface update retained manifest Packs = %#v", installation.Manifest.Packs)
	}
}

func TestIssue626ProjectUpdateAdvancesCompatibleSharedProjectionThenBlocksIncompatibleContent(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, repoRoot := packActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	for _, surface := range []string{"codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", surface, "--resource", "skill:ask-matt"); err != nil {
			t.Fatalf("install Matty on %s: %v\n%s", surface, err, out)
		}
	}
	manifestPath := filepath.Join(bundle, "packs", "matty", "pack.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedManifest := strings.Replace(string(manifest), `"version": "1.0.3"`, `"version": "1.0.4"`, 1)
	if updatedManifest == string(manifest) {
		t.Fatal("Matty fixture version did not match the expected current version")
	}
	if err := os.WriteFile(manifestPath, []byte(updatedManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "codex", "--project"); err != nil {
		t.Fatalf("compatible Codex shared update: %v\n%s", err, out)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	versions := map[capabilitypack.Surface]string{}
	for _, receipt := range installation.Lock.Receipts {
		if receipt.Pack.ID == "matty" {
			versions[receipt.Surface] = receipt.Pack.Version
		}
	}
	if versions[capabilitypack.SurfaceCodex] != "1.0.4" || versions[capabilitypack.SurfaceOpenCode] != "1.0.3" {
		t.Fatalf("compatible shared update did not retain per-surface versions: %#v", versions)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "opencode", "--project"); err != nil {
		t.Fatalf("compatible OpenCode shared update: %v\n%s", err, out)
	}
	updatedAgain := strings.Replace(updatedManifest, `"version": "1.0.4"`, `"version": "1.0.5"`, 1)
	if updatedAgain == updatedManifest {
		t.Fatal("updated Matty fixture version did not match")
	}
	if err := os.WriteFile(manifestPath, []byte(updatedAgain), 0o600); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(bundle, "skills", "engineering", "ask-matt", "SKILL.md")
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, append(skill, []byte("\nIncompatible shared update.\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "codex", "--project")
	if err == nil || !strings.Contains(out, "shared_projection_version_conflict") {
		t.Fatalf("incompatible shared update was not blocked: %v\n%s", err, out)
	}
	if after := snapshotTree(t, project); after != before {
		t.Fatalf("blocked shared update mutated the project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestIssue626ProjectUpdateRetiresAProjectionOnlyAfterItsLastSurfaceUpdates(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, repoRoot := packActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	manifestPath := filepath.Join(bundle, "packs", "matty", "pack.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	withDependency := strings.Replace(string(manifest), `"requires": [],`, `"requires": ["skill:code-review"],`, 1)
	if withDependency == string(manifest) {
		t.Fatal("Matty fixture omitted the expected empty ask-matt dependency list")
	}
	if err := os.WriteFile(manifestPath, []byte(withDependency), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", surface, "--resource", "skill:ask-matt"); err != nil {
			t.Fatalf("install Matty on %s: %v\n%s", surface, err, out)
		}
	}
	retiredPath := filepath.Join(project, ".agents", "skills", "code-review", "SKILL.md")
	if _, err := os.Stat(retiredPath); err != nil {
		t.Fatalf("shared dependency was not installed: %v", err)
	}
	withoutDependency := strings.Replace(withDependency, `"requires": ["skill:code-review"],`, `"requires": [],`, 1)
	withoutDependency = strings.Replace(withoutDependency, `"version": "1.0.3"`, `"version": "1.0.4"`, 1)
	if err := os.WriteFile(manifestPath, []byte(withoutDependency), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(project, "packy.lock.json")
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	retainedOpenCode := projectSurfaceReceiptJSON(t, before, "matty", "opencode")
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "codex", "--project"); err != nil {
		t.Fatalf("Codex dependency retirement: %v\n%s", err, out)
	}
	if _, err := os.Stat(retiredPath); err != nil {
		t.Fatalf("Codex update removed a projection retained by OpenCode: %v", err)
	}
	afterCodex, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectSurfaceReceiptJSON(t, afterCodex, "matty", "opencode"); got != retainedOpenCode {
		t.Fatalf("Codex retirement changed the OpenCode receipt\nbefore: %s\nafter:  %s", retainedOpenCode, got)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "opencode", "--project"); err != nil {
		t.Fatalf("OpenCode dependency retirement: %v\n%s", err, out)
	}
	if _, err := os.Stat(retiredPath); !os.IsNotExist(err) {
		t.Fatalf("last surface update retained the retired projection: %v", err)
	}
}

func TestIssue626ProjectNoticesUseOnlyTheSelectedSurfaceIntent(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, repoRoot := packActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	manifestPath := filepath.Join(bundle, "packs", "addy", "pack.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	resource := `"id": "api-and-interface-design",
      "kind": "skill",`
	withNotice := strings.Replace(string(manifest), resource, resource+"\n      \"notices\": [\"notice:mit\"],", 1)
	if withNotice == string(manifest) {
		t.Fatal("Addy fixture omitted api-and-interface-design")
	}
	if err := os.WriteFile(manifestPath, []byte(withNotice), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install", "addy", "--surface", "codex", "--resource", "skill:api-and-interface-design"); err != nil {
		t.Fatalf("install Codex notice selection: %v\n%s", err, out)
	}
	previewJSON, err := executeCommand(t, NewRootCommand(opts), "install", "addy", "--surface", "opencode", "--resource", "skill:browser-testing-with-devtools", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("preview OpenCode no-notice selection: %v\n%s", err, previewJSON)
	}
	var preview capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(previewJSON), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Notices.Contributions) != 0 {
		t.Fatalf("OpenCode preview inherited Codex notices: %#v", preview.Notices.Contributions)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install", "addy", "--surface", "opencode", "--resource", "skill:browser-testing-with-devtools"); err != nil {
		t.Fatalf("install OpenCode no-notice selection: %v\n%s", err, out)
	}
	notices, err := os.ReadFile(filepath.Join(project, "PACKY-NOTICES.md"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(notices), "<!-- packy:project:addy:opencode:notices:start -->")
	end := strings.Index(string(notices), "<!-- packy:project:addy:opencode:notices:end -->")
	if start < 0 || end <= start || strings.Contains(string(notices)[start:end], "Copyright 2025 Addy Osmani") {
		t.Fatalf("OpenCode notice block inherited the Codex attribution:\n%s", notices)
	}
}

func TestIssue519DriftedNoticeBlocksReceiptRemoval(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "install", "addy", "--surface", "opencode"); err != nil {
		t.Fatalf("install Addy notices: %v\n%s", err, out)
	}
	noticesPath := filepath.Join(project, "PACKY-NOTICES.md")
	notices, err := os.ReadFile(noticesPath)
	if err != nil {
		t.Fatal(err)
	}
	drifted := strings.Replace(string(notices), "Copyright 2025 Addy Osmani", "locally changed attribution", 1)
	if drifted == string(notices) {
		t.Fatal("notice fixture did not contain the expected attribution")
	}
	if err := os.WriteFile(noticesPath, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "uninstall", "addy")
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
