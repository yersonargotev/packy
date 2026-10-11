package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestCandidateTransitionPreservesInstalledBaselineForTargetedUpdate(t *testing.T) {
	pack := testsupport.SkillVariants("transition", testsupport.SurfaceCodex, testsupport.SurfaceClaude)
	baseline, digest := candidateTestAssets(t, strings.Repeat("a", 40), pack)
	candidate, _ := candidateTestAssets(t, strings.Repeat("b", 40), pack.WithVersion("2.0.0").WithVariantBytes(testsupport.SurfaceCodex, "references/detail.md", []byte("updated codex\n")))
	workspace := filepath.Join(t.TempDir(), "workspace")
	opts := Options{Terminal: &fakeTerminal{interactive: true, approve: true}}
	run := func(assets string, args ...string) string {
		t.Helper()
		prefix := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}
		out, err := executeCommand(t, NewRootCommand(opts), append(prefix, args...)...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run(baseline, "install", pack.ID(), "--surface", "codex")
	run(baseline, "install", pack.ID(), "--surface", "claude")
	project := filepath.Join(workspace, "project")
	before := snapshotTree(t, project)
	out := run(candidate, "transition", "--expect-archive-sha256", digest, "--json")
	if !strings.Contains(out, `"result":"transitioned"`) || snapshotTree(t, project) != before {
		t.Fatalf("transition changed installed baseline: %s", out)
	}
	claude := snapshotTree(t, filepath.Join(project, ".claude"))
	run(candidate, "update", pack.ID(), "--surface", "codex", "--project")
	if snapshotTree(t, filepath.Join(project, ".claude")) != claude {
		t.Fatal("targeted update changed Claude")
	}
	run(candidate, "uninstall", pack.ID(), "--surface", "claude")
	run(candidate, "uninstall", pack.ID(), "--surface", "codex")
}

func candidateTestAssets(t *testing.T, commit string, pack testsupport.Fixture) (string, string) {
	t.Helper()
	release := catalogReleaseFixture(t, commit, pack)
	assets := t.TempDir()
	var digest string
	for _, asset := range release.Assets {
		if err := os.WriteFile(filepath.Join(assets, asset.Name), asset.Data, 0o644); err != nil {
			t.Fatal(err)
		}
		if asset.Name == "catalog-snapshot.tar.gz" {
			digest = fmt.Sprintf("%x", sha256.Sum256(asset.Data))
		}
	}
	return assets, digest
}
