package cli

import (
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue877SharedSkillsRejectClientBeforeMutation(t *testing.T) {
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, testsupport.SkillVariants("shared-only"))
	project, err := fixture.options.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	beforeHome, beforeProject := snapshotTree(t, fixture.home), snapshotTree(t, project)
	for _, operation := range []string{"activate", "install"} {
		out, err := executeCommand(t, NewRootCommand(fixture.options), operation, "shared-only", "--surface", "agents", "--client", "codex")
		if err == nil || !strings.Contains(out+err.Error(), "client") {
			t.Fatalf("%s accepted a client without reviewed roles: %v\n%s", operation, err, out)
		}
		if snapshotTree(t, fixture.home) != beforeHome || snapshotTree(t, project) != beforeProject {
			t.Fatalf("%s client rejection mutated state", operation)
		}
	}
	for _, operation := range []string{"activate", "install"} {
		if out, err := executeCommand(t, NewRootCommand(fixture.options), operation, "shared-only", "--surface", "agents"); err != nil {
			t.Fatalf("seed shared skill %s: %v\n%s", operation, err, out)
		}
	}
	beforeHome, beforeProject = snapshotTree(t, fixture.home), snapshotTree(t, project)
	for _, projectCheck := range []bool{false, true} {
		args := []string{"check", "shared-only", "--surface", "agents", "--client", "codex", "--dry-run"}
		if projectCheck {
			args = append(args, "--project")
		}
		out, err := executeCommand(t, NewRootCommand(fixture.options), args...)
		if err == nil || !strings.Contains(out+err.Error(), "client") {
			t.Fatalf("shared-only check accepted client: %v\n%s", err, out)
		}
		if snapshotTree(t, fixture.home) != beforeHome || snapshotTree(t, project) != beforeProject {
			t.Fatal("client-rejected check mutated state")
		}
	}
}
