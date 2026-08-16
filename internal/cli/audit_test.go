package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/audit"
	"github.com/yersonargotev/packy/internal/setuphealth"
)

func TestAuditReportsUnknownRuntimeWithoutFailingAutomation(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	opts.Getwd = func() (string, error) { return t.TempDir(), nil }
	opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
		return setuphealth.Report{Checks: []setuphealth.Check{
			{Name: "packy-core", Scope: setuphealth.CheckScopeWorkstation, Severity: setuphealth.Pass, Detail: "Packy core is available"},
			{Name: "pack-orchestrate-codex-usable", Scope: setuphealth.CheckScopeGlobal, Severity: setuphealth.Info, Detail: "runtime usability cannot be observed"},
		}}, nil
	}

	output, err := executeCommand(t, NewRootCommand(opts), "audit")
	if err != nil {
		t.Fatalf("audit: %v\n%s", err, output)
	}
	for _, want := range []string{
		"Packy audit: healthy", "Workstation", "Active global Packs", "Current project",
		"runtime usability cannot be observed", "project-unavailable", "warnings=0 failures=0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("audit output missing %q:\n%s", want, output)
		}
	}
}

func TestAuditWarningsAreMachineReadableAndExitZero(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	opts.Getwd = func() (string, error) { return t.TempDir(), nil }
	opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
		return setuphealth.Report{Checks: []setuphealth.Check{{Name: "pack-matty-codex", Scope: setuphealth.CheckScopeGlobal, Severity: setuphealth.Warn, Detail: "one projection finding"}}}, nil
	}

	output, err := executeCommand(t, NewRootCommand(opts), "audit", "--json")
	if err != nil {
		t.Fatalf("warning audit: %v\n%s", err, output)
	}
	var report audit.Report
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatal(err)
	}
	if report.Result != audit.Warnings || report.Summary.Warnings != 1 || report.Summary.Failures != 0 {
		t.Fatalf("warning report = %#v", report)
	}
}

func TestAuditEmitsRedactedFullReportWhenWorkstationInspectionFails(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	opts.Getwd = func() (string, error) { return t.TempDir(), nil }
	opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
		return setuphealth.Report{}, errors.New("cannot inspect /Users/private/workstation")
	}

	output, err := executeCommand(t, NewRootCommand(opts), "audit", "--json")
	if !errors.Is(err, ErrAuditFailures) {
		t.Fatalf("inspection audit error = %v\n%s", err, output)
	}
	var report audit.Report
	if json.Unmarshal([]byte(output), &report) != nil || report.Result != audit.Failures || report.Summary.Failures != 1 || report.Project.State != audit.ProjectUnavailable {
		t.Fatalf("inspection report = %#v\n%s", report, output)
	}
	if !strings.Contains(output, "packy-core-inspection") || strings.Contains(output, "/Users/private") {
		t.Fatalf("inspection report is incomplete or leaked a path:\n%s", output)
	}
}

func TestAuditFailsClosedWhenCurrentDirectoryCannotBeInspected(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	opts.Getwd = func() (string, error) { return "", errors.New("cannot read /Users/private/project") }
	opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
		return setuphealth.Report{Checks: []setuphealth.Check{{Name: "packy-core", Scope: setuphealth.CheckScopeWorkstation, Severity: setuphealth.Pass, Detail: "Packy core is available"}}}, nil
	}

	output, err := executeCommand(t, NewRootCommand(opts), "audit", "--json")
	if !errors.Is(err, ErrAuditFailures) {
		t.Fatalf("current-directory audit error = %v\n%s", err, output)
	}
	var report audit.Report
	if json.Unmarshal([]byte(output), &report) != nil || report.Project.State != audit.ProjectFailed || report.Summary.Failures != 1 {
		t.Fatalf("current-directory report = %#v\n%s", report, output)
	}
	if !strings.Contains(output, "project-context-inspection-failed") || strings.Contains(output, "/Users/private") {
		t.Fatalf("current-directory report is incomplete or leaked a path:\n%s", output)
	}
}

func TestAuditVerifiesProjectContractAndDetectsDriftWithoutMutation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if output, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex", "--resource", "skill:ask-matt"); err != nil {
		t.Fatalf("install: %v\n%s", err, output)
	}

	beforeProject := snapshotTree(t, project)
	beforeHome := snapshotTree(t, home)
	output, err := executeCommand(t, NewRootCommand(opts), "audit", "--json")
	if err != nil {
		t.Fatalf("verified audit: %v\n%s", err, output)
	}
	var verified audit.Report
	if json.Unmarshal([]byte(output), &verified) != nil || verified.Project.State != audit.ProjectVerified || verified.Project.Verified != verified.Project.Projections || verified.Project.Projections == 0 {
		t.Fatalf("verified report = %#v\n%s", verified, output)
	}
	if strings.Contains(output, project) || strings.Contains(output, home) {
		t.Fatalf("audit leaked a local path:\n%s", output)
	}
	if snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome || terminal.calls != 1 {
		t.Fatal("audit mutated state or requested approval")
	}

	drift := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if err := os.WriteFile(drift, []byte("operator drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	driftedProject := snapshotTree(t, project)
	output, err = executeCommand(t, NewRootCommand(opts), "audit", "--json")
	if !errors.Is(err, ErrAuditFailures) {
		t.Fatalf("drift audit error = %v\n%s", err, output)
	}
	var failed audit.Report
	if json.Unmarshal([]byte(output), &failed) != nil || failed.Result != audit.Failures || failed.Project.State != audit.ProjectFailed || failed.Summary.Failures == 0 {
		t.Fatalf("failed report = %#v\n%s", failed, output)
	}
	if !strings.Contains(output, "project_drift") || !strings.Contains(output, "restore the exact locked projection with the named Pack install") {
		t.Fatalf("drift audit lacks actionable finding:\n%s", output)
	}
	if snapshotTree(t, project) != driftedProject || snapshotTree(t, home) != beforeHome || terminal.calls != 1 {
		t.Fatal("failed audit mutated state or requested approval")
	}
}

func TestAuditTreatsIncompleteProjectContractAsFailure(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if err := os.WriteFile(filepath.Join(project, "packy.json"), []byte(`{"schema_version":1,"packs":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := executeCommand(t, NewRootCommand(opts), "audit")
	if !errors.Is(err, ErrAuditFailures) || !strings.Contains(output, "project_contract_invalid") || !strings.Contains(output, "Remediation:") {
		t.Fatalf("incomplete audit: err=%v\n%s", err, output)
	}
}
