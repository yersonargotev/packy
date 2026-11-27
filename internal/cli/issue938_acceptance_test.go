package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/githubfake"
)

func TestIssue938TamperedOfficialSnapshotIsRestoredByTheHintedCommand(t *testing.T) {
	transport := buildIssue929Transport(t, t.TempDir())
	issue929Dists.root = t.TempDir()
	issue929Dists.built.Clear()
	commit := strings.Repeat("a", 40)
	fixtures := t.TempDir()
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, commit, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	environment := startIssue929GitHub(t, fixtures)
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	manifest := filepath.Join(issue929Home(t, environment), ".local", "share", "packy", "catalog", "v2", "yersonargotev", "packy-catalog", "snapshots", commit, "packs", "fixture", "pack.json")
	if err := os.WriteFile(manifest, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runIssue929(t, transport, environment, "list")
	if !issue929Exit(err, 1) || !strings.Contains(out, "run `packy init`") || !strings.Contains(out, "does not match") {
		t.Fatalf("list over a tampered snapshot: %v\n%s", err, out)
	}
	if out, err := runIssue929(t, transport, environment, "init"); err != nil || !strings.Contains(out, "selected official Catalog Snapshot "+commit) {
		t.Fatalf("init over a tampered snapshot: %v\n%s", err, out)
	}
	if out, err := runIssue929(t, transport, environment, "list"); err != nil || !strings.Contains(out, "fixture") {
		t.Fatalf("list after init: %v\n%s", err, out)
	}
}
