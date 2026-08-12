package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestProjectVerifyPassesOfflineWithoutPersonalState(t *testing.T) {
	opts, project := installIssue453Project(t)
	opts.Env = MapEnv{}
	before := snapshotTree(t, project)

	out, err := executeCommand(t, NewRootCommand(opts), "verify", "--json")
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, out)
	}
	var report capabilitypack.ProjectVerificationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode verification: %v\n%s", err, out)
	}
	if report.Report != "project-verification" || report.Result != capabilitypack.ProjectVerificationPassed || report.ProjectRoot != "<project-root>" || report.Summary.Packs != 1 || report.Summary.Surfaces != 1 || report.Summary.Verified != report.Summary.Projections {
		t.Fatalf("verification report = %#v", report)
	}
	if snapshotTree(t, project) != before {
		t.Fatal("verification mutated the project")
	}
}

func TestProjectVerifyRunsWithNoHomeOrPath(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "install", "engram", "--surface", "codex"); err != nil {
		t.Fatalf("seed external-requirement project: %v\n%s", err, out)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestProjectVerifyEmptyEnvironmentHelper$")
	cmd.Env = []string{"PACKY_VERIFY_HELPER=1", "PACKY_VERIFY_PROJECT=" + project}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify under empty environment: %v\n%s", err, out)
	}
	var report capabilitypack.ProjectVerificationReport
	if json.Unmarshal(firstJSONDocument(t, string(out)), &report) != nil || report.Result != capabilitypack.ProjectVerificationPassed {
		t.Fatalf("empty-environment verification = %#v\n%s", report, out)
	}
}

func TestProjectVerifyEmptyEnvironmentHelper(t *testing.T) {
	if os.Getenv("PACKY_VERIFY_HELPER") != "1" {
		return
	}
	project := os.Getenv("PACKY_VERIFY_PROJECT")
	opts := Options{Env: MapEnv{}, Getwd: func() (string, error) { return project, nil }}
	out, err := executeCommand(t, NewRootCommand(opts), "verify", "--json")
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, out)
	}
	fmt.Print(out)
}

func TestProjectVerifyFailsWithActionableDriftFinding(t *testing.T) {
	opts, project := installIssue453Project(t)
	drift := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if err := os.WriteFile(drift, []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)

	out, err := executeCommand(t, NewRootCommand(opts), "verify", "--json")
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("verify error = %v\n%s", err, out)
	}
	var report capabilitypack.ProjectVerificationReport
	if json.Unmarshal([]byte(out), &report) != nil || report.Result != capabilitypack.ProjectVerificationFailed || report.Summary.Findings == 0 || len(report.Entries) != 1 || len(report.Entries[0].Findings) == 0 {
		t.Fatalf("verification report = %#v\n%s", report, out)
	}
	if snapshotTree(t, project) != before {
		t.Fatal("failed verification mutated the project")
	}
}

func TestProjectVerifyRejectsChangedProjectionMode(t *testing.T) {
	opts, project := installIssue453Project(t)
	target := filepath.Join(project, "AGENTS.md")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "verify", "--json")
	if err == nil || !strings.Contains(out, `"target":"AGENTS.md"`) {
		t.Fatalf("projection mode verification = %v\n%s", err, out)
	}
}

func TestProjectLockRequiresProjectionModeEvidence(t *testing.T) {
	_, project := installIssue453Project(t)
	lockPath := filepath.Join(project, "packy.lock.json")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	receipts := lock["receipts"].([]any)
	projections := receipts[0].(map[string]any)["projections"].([]any)
	for _, value := range projections {
		projection := value.(map[string]any)
		if projection["target"] != "PACKY-NOTICES.md" {
			delete(projection, "file_mode")
			break
		}
	}
	data, err = json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := capabilitypack.LoadProjectInstallation(project); err == nil || !strings.Contains(err.Error(), "invalid projection evidence") {
		t.Fatalf("load project lock without projection mode = %v", err)
	}
}

func TestProjectVerifyFailsCleanlyWhenContractIsAbsent(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	out, err := executeCommand(t, NewRootCommand(opts), "verify")
	if err == nil || !strings.Contains(out, "project_contract_absent") || !strings.Contains(out, "packy install") {
		t.Fatalf("absent verification = %v\n%s", err, out)
	}
}

func TestProjectVerifyRejectsDuplicateNoticeContributions(t *testing.T) {
	opts, project := installIssue453Project(t)
	noticesPath := filepath.Join(project, "PACKY-NOTICES.md")
	notices, err := os.ReadFile(noticesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noticesPath, append(append([]byte(nil), notices...), notices...), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "verify", "--json")
	if err == nil || !strings.Contains(out, `"target":"PACKY-NOTICES.md"`) {
		t.Fatalf("duplicate notices verification = %v\n%s", err, out)
	}
}

func TestProjectVerifyRejectsChangedNoticeMode(t *testing.T) {
	opts, project := installIssue453Project(t)
	noticesPath := filepath.Join(project, "PACKY-NOTICES.md")
	if err := os.Chmod(noticesPath, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "verify", "--json")
	if err == nil || !strings.Contains(out, `"target":"PACKY-NOTICES.md"`) {
		t.Fatalf("notice mode verification = %v\n%s", err, out)
	}
}

func TestProjectVerifyRedactsProjectRootFromInspectionErrors(t *testing.T) {
	opts, project := installIssue453Project(t)
	target := filepath.Join(project, "AGENTS.md")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "verify", "--json")
	var report capabilitypack.ProjectVerificationReport
	decodeErr := json.Unmarshal([]byte(out), &report)
	if err == nil || decodeErr != nil || len(report.Findings) != 1 || strings.Contains(report.Findings[0].Detail, project) || !strings.Contains(report.Findings[0].Detail, "<project-root>") {
		t.Fatalf("redacted verification = %v\n%s", err, out)
	}
}
