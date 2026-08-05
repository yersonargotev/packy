package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue462UninstallSeparatelyDeactivatesPersonalProjectEffects(t *testing.T) {
	opts, project, home, trustPath, statePath := installAndActivateIssue462Project(t)
	terminal := opts.Terminal.(*fakeTerminal)
	terminal.calls = 0

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty")
	if err != nil {
		t.Fatalf("uninstall active project: %v\n%s", err, out)
	}
	for _, want := range []string{
		"PERSONAL PROJECT DEACTIVATION PREVIEW",
		"Runtime activation: active",
		"Remove personal effect: codex-project-trust",
		"consent=destructive-cleanup",
		"adapter_provenance=codex-project/v1/project-trust",
		"Approve personal project deactivation",
		"Verified personal project deactivation",
		"COMPLETE PROJECT PACK UNINSTALL PREVIEW",
		"Verified project uninstall",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("guided uninstall missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 2 {
		t.Fatalf("guided uninstall approvals = %d, want separate deactivation and uninstall approvals", terminal.calls)
	}
	if data, err := os.ReadFile(trustPath); err != nil || strings.Contains(string(data), project) {
		t.Fatalf("personal trust remains after guided uninstall: %q, %v", data, err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("personal activation receipt remains after verified deactivation: %v", err)
	}
	for _, target := range []string{"packy.json", "packy.lock.json"} {
		if _, err := os.Stat(filepath.Join(project, target)); !os.IsNotExist(err) {
			t.Fatalf("shared project contract %s remains: %v", target, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".packy", "projects")); err == nil {
		t.Fatal("empty personal project state directory remains")
	}
}

func TestIssue462PersonalDeactivationRejectsConcurrentTargetChange(t *testing.T) {
	opts, project, _, trustPath, statePath := installAndActivateIssue462Project(t)
	terminal := opts.Terminal.(*fakeTerminal)
	terminal.calls = 0
	terminal.onApprove = func() {
		file, err := os.OpenFile(trustPath, os.O_APPEND|os.O_WRONLY, 0)
		if err == nil {
			_, err = file.WriteString("# concurrent foreign content\n")
			_ = file.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty")
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("concurrent personal target change = %v\n%s", err, out)
	}
	if data := readFileString(t, trustPath); !strings.Contains(data, project) || !strings.Contains(data, "concurrent foreign content") {
		t.Fatalf("stale personal cleanup did not preserve complete target: %q", data)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("stale personal cleanup retired receipts: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "packy.lock.json")); err != nil {
		t.Fatalf("stale personal cleanup continued into shared uninstall: %v", err)
	}
}

func TestIssue462OrphanedActivationIsExplicitAndDeactivatesFromReceipts(t *testing.T) {
	opts, project, _, trustPath, statePath := installAndActivateIssue462Project(t)
	trusted := readFileString(t, trustPath)
	removeIssue462SharedProject(t, project)

	beforeTrust := readFileString(t, trustPath)
	beforeState := readFileString(t, statePath)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("orphaned project status: %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectStatusReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Packs) != 1 || report.Packs[0].Installation != capabilitypack.ProjectInstallationAbsent || report.Packs[0].Runtime != capabilitypack.ProjectRuntimeOrphaned {
		t.Fatalf("orphaned structured status = %+v", report)
	}
	if got := strings.Join(report.Packs[0].PendingHumanActions, "\n"); got != "packy pack deactivate matty --surface codex --project" {
		t.Fatalf("orphaned cleanup command = %q", got)
	}
	if readFileString(t, trustPath) != beforeTrust || readFileString(t, statePath) != beforeState {
		t.Fatal("orphan observation mutated personal effects or receipts")
	}
	deactivationJSON, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", "codex", "--project", "--dry-run", "--json")
	var deactivation capabilitypack.JSONProjectDeactivationPreview
	if err != nil || json.Unmarshal([]byte(deactivationJSON), &deactivation) != nil || len(deactivation.Effects) != 1 || deactivation.Effects[0].Consent != capabilitypack.ConsentDestructiveCleanup || deactivation.Effects[0].AdapterProvenance != "codex-project/v1/project-trust" {
		t.Fatalf("typed orphan deactivation preview = %+v, %v\n%s", deactivation, err, deactivationJSON)
	}
	if readFileString(t, trustPath) != beforeTrust || readFileString(t, statePath) != beforeState {
		t.Fatal("orphan deactivation dry-run mutated personal effects or receipts")
	}
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "--project", "--json")
	report = capabilitypack.JSONProjectStatusReport{}
	if err != nil || json.Unmarshal([]byte(out), &report) != nil || len(report.Packs) != 1 || report.Packs[0].Runtime != capabilitypack.ProjectRuntimeOrphaned {
		t.Fatalf("unfocused orphan status = %+v, %v\n%s", report, err, out)
	}

	if err := os.WriteFile(trustPath, []byte(strings.Replace(trusted, "trust_level = \"trusted\"", "trust_level = \"untrusted\"", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	drifted := readFileString(t, trustPath)
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--json")
	if err != nil || json.Unmarshal([]byte(out), &report) != nil || report.Packs[0].Runtime != capabilitypack.ProjectRuntimeBlocked || strings.Join(report.Packs[0].PendingHumanActions, "\n") != "packy pack deactivate matty --surface codex --project" {
		t.Fatalf("drifted personal status = %+v, %v\n%s", report, err, out)
	}
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", "codex", "--project")
	if err == nil || !strings.Contains(out, "personal_effect_drift") {
		t.Fatalf("drifted orphan deactivation = %v\n%s", err, out)
	}
	if readFileString(t, trustPath) != drifted || readFileString(t, statePath) != beforeState {
		t.Fatal("blocked orphan cleanup changed drifted personal state or retired receipts")
	}

	if err := os.WriteFile(trustPath, []byte(trusted), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", "codex", "--project")
	if err != nil {
		t.Fatalf("receipt-backed orphan deactivation: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Runtime activation: orphaned") || !strings.Contains(out, "Verified personal project deactivation") {
		t.Fatalf("orphan deactivation output:\n%s", out)
	}
	if data, err := os.ReadFile(trustPath); err != nil || strings.Contains(string(data), project) {
		t.Fatalf("orphaned personal contribution remains: %q, %v", data, err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("verified-absent orphan receipt remains: %v", err)
	}
}

func TestIssue462StaleAndRecoveryStatusProvideStableExactCommands(t *testing.T) {
	opts, project, _, _, statePath := installAndActivateIssue462Project(t)
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	installation.Lock.Sensitive[0].Detail = "changed-project-trust"
	data, err := json.MarshalIndent(installation.Lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--require", "usable", "--json")
	var report capabilitypack.JSONProjectStatusReport
	if err == nil || json.Unmarshal(firstJSONDocument(t, out), &report) != nil || report.Packs[0].Runtime != capabilitypack.ProjectRuntimeStale || !containsString(report.Packs[0].PendingHumanActions, "packy pack activate matty --surface codex --project") {
		t.Fatalf("stale usable status = %+v, %v\n%s", report, err, out)
	}

	var state map[string]any
	if err := json.Unmarshal([]byte(readFileString(t, statePath)), &state); err != nil {
		t.Fatal(err)
	}
	state["recovery"].(map[string]any)["status"] = "required"
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--require", "usable", "--json")
	report = capabilitypack.JSONProjectStatusReport{}
	if err == nil || json.Unmarshal(firstJSONDocument(t, out), &report) != nil || report.Packs[0].Runtime != capabilitypack.ProjectRuntimeRecoveryRequired || !containsString(report.Packs[0].PendingHumanActions, "packy pack deactivate matty --surface codex --project") {
		t.Fatalf("recovery-required usable status = %+v, %v\n%s", report, err, out)
	}
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex", "--project", "--dry-run", "--json")
	var preview capabilitypack.JSONProjectActivationPreview
	if err != nil || json.Unmarshal([]byte(out), &preview) != nil || preview.Disposition != capabilitypack.ProjectActivationBlocked {
		t.Fatalf("activation over unresolved recovery = %+v, %v\n%s", preview, err, out)
	}
}

func installAndActivateIssue462Project(t *testing.T) (Options, string, string, string, string) {
	t.Helper()
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	installation.Lock.Sensitive = []capabilitypack.ProjectSensitiveDisclosure{
		{Category: capabilitypack.ProjectActivationTrust, Surface: capabilitypack.SurfaceCodex, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "project-trust"},
	}
	data, err := json.MarshalIndent(installation.Lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex", "--project"); err != nil {
		t.Fatalf("activate: %v\n%s", err, out)
	}
	stateMatches, err := filepath.Glob(filepath.Join(home, ".packy", "projects", "*", "state.json"))
	if err != nil || len(stateMatches) != 1 {
		t.Fatalf("project activation state = %v, %v", stateMatches, err)
	}
	return opts, project, home, filepath.Join(home, ".codex", "config.toml"), stateMatches[0]
}

func removeIssue462SharedProject(t *testing.T, project string) {
	t.Helper()
	for _, target := range []string{".agents", "AGENTS.md", "PACKY-NOTICES.md", "packy.json", "packy.lock.json"} {
		if err := os.RemoveAll(filepath.Join(project, target)); err != nil {
			t.Fatal(err)
		}
	}
}
