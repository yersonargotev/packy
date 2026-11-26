package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestProjectLockRejectionKeepsTheAdoptionGuideWhenThePackIsNamedPacky(t *testing.T) {
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, testsupport.SkillVariants("packy"))
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts := fixture.options
	opts.Getwd = func() (string, error) { return project, nil }
	if err := os.WriteFile(filepath.Join(project, "packy.json"), []byte(`{"schema_version":1,"packs":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), []byte(`{"schema_version":1,"receipts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"status", "packy", "--project", "--surface", "agents"},
		{"status", "packy", "--project", "--surface", "agents", "--json"},
	} {
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err == nil || !strings.Contains(err.Error(), capabilitypack.CatalogAdoptionGuide) || strings.Contains(out+err.Error(), "<redacted>") {
			t.Fatalf("%v: error=%v output=%s", args, err, out)
		}
	}
}
