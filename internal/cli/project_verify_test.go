package cli

import (
	"encoding/json"
	"os"
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
