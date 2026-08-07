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
		{"pack", "install", "addy", "--surface", "codex", "--resource", "skill:api-and-interface-design"},
		{"pack", "install", "argote", "--surface", "codex", "--resource", "instruction:engineering-principles"},
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
	statusOutput, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "--project", "--json")
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
		if receipt.Pack.ID == "" || receipt.Pack.Version != "1.0.0" || receipt.Surface != "codex" || len(receipt.Resources) == 0 || len(receipt.Projections) == 0 {
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
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "addy", "--project"); err != nil {
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
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "addy", "--project"); err == nil {
		t.Fatalf("ordinary project update overwrote receipt drift:\n%s", out)
	}
	if drifted, _ := os.ReadFile(addySkill); string(drifted) != "user drift\n" {
		t.Fatalf("blocked update changed drifted receipt target: %q", drifted)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "addy", "--project", "--force"); err != nil {
		t.Fatalf("force receipt-owned Addy update: %v\n%s", err, out)
	}
	if restored, _ := os.ReadFile(addySkill); string(restored) != string(originalSkill) {
		t.Fatal("forced update did not restore the exact receipt-owned Addy projection")
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "argote"); err != nil {
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

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "addy", "--surface", "codex", "--resource", "skill:api-and-interface-design"); err != nil {
		t.Fatalf("install Addy: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "engram", "--surface", "codex", "--resource", "instruction:engram-memory"); err != nil {
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

	terminal.answers = nil
	terminal.calls = 0
	terminal.approve = true
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex", "--project"); err != nil {
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

func TestIssue519CrossPackProjectionCollisionBlocksWithoutMutation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--resource", "skill:ask-matt"); err != nil {
		t.Fatalf("install Matty: %v\n%s", err, out)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "argote", "--surface", "codex", "--resource", "instruction:engineering-principles")
	if err == nil || !strings.Contains(out, "projection_collision") {
		t.Fatalf("cross-Pack collision was not blocked: %v\n%s", err, out)
	}
	if after := snapshotTree(t, project); after != before {
		t.Fatal("blocked cross-Pack collision mutated the project")
	}
}

func TestIssue519MultiSurfaceUpdatePreservesEveryOtherPackReceipt(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	commands := [][]string{
		{"pack", "install", "matty", "--surface", "codex", "--resource", "skill:ask-matt"},
		{"pack", "install", "matty", "--surface", "opencode", "--resource", "skill:ask-matt"},
		{"pack", "install", "addy", "--surface", "codex", "--resource", "skill:api-and-interface-design"},
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
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--project"); err != nil {
		t.Fatalf("multi-surface Matty update: %v\n%s", err, out)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectReceiptJSON(t, after, "addy"); got != addyReceipt {
		t.Fatalf("multi-surface update changed Addy receipt\nbefore: %s\nafter:  %s", addyReceipt, got)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Manifest.Packs) != 2 {
		t.Fatalf("multi-surface update retained manifest Packs = %#v", installation.Manifest.Packs)
	}
}

func TestIssue519DriftedNoticeBlocksReceiptRemoval(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "addy", "--surface", "opencode"); err != nil {
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
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "addy")
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
