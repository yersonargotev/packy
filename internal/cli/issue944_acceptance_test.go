package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/githubfake"
)

func TestIssue944TamperedOlderSnapshotIsRestoredByTheHintedCommand(t *testing.T) {
	transport := builtIssue929Transport(t)
	issue929Dists.root = t.TempDir()
	issue929Dists.built.Clear()
	older, newer := strings.Repeat("a", 40), strings.Repeat("b", 40)
	fixtures := t.TempDir()
	publishIssue929Portable(t, transport, fixtures, issue929OfficialCatalog, older, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	environment := startIssue929GitHub(t, fixtures)
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh to the older snapshot: %v\n%s", err, out)
	}
	approveIssue929(t, transport, environment, t.TempDir(), "activate", "fixture", "--surface", "agents")
	publishIssue929Portable(t, transport, fixtures, issue929OfficialCatalog, newer, githubfake.Pack{ID: "fixture", Version: "2.0.0"})
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil || !strings.Contains(out, "selected Catalog Snapshot "+newer) {
		t.Fatalf("catalog refresh to the newer snapshot: %v\n%s", err, out)
	}
	snapshots := filepath.Join(issue929CatalogDirectory(t, environment, issue929OfficialCatalog), "snapshots")
	skill := filepath.Join(snapshots, older, "packs", "fixture", "skills", "fixture", "SKILL.md")
	released, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, append(bytes.Clone(released), "tampered\n"...), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := "packy catalog restore " + issue929OfficialCatalog + " " + older
	exported := filepath.Join(t.TempDir(), "plugin")
	readers := [][]string{
		{"get", "skill", "fixture:fixture", "--surface", "agents", "--snapshot", older},
		{"export", "fixture", "--surface", "agents", "--output", exported, "--snapshot", older},
		{"deactivate", "fixture", "--surface", "agents", "--dry-run"},
	}
	for _, args := range readers {
		out, err := runIssue929(t, transport, environment, args...)
		if !issue929Exit(err, 1) || !strings.Contains(out, restore) || strings.Contains(out, "packy init or packy catalog refresh") {
			t.Fatalf("%v over a tampered older snapshot: %v\n%s\nwant the hint %q", args, err, out, restore)
		}
	}

	if out, err := runIssue929(t, transport, environment, "catalog", "restore", issue929OfficialCatalog, older); err != nil || !strings.Contains(out, "restored Catalog Snapshot "+older+" of "+issue929OfficialCatalog) {
		t.Fatalf("catalog restore: %v\n%s", err, out)
	}
	if after, err := os.ReadFile(skill); err != nil || !bytes.Equal(after, released) {
		t.Fatalf("SKILL.md after restore = %q, %v; want the released bytes", after, err)
	}
	if selected := issue929Selected(t, environment, issue929OfficialCatalog); selected != newer {
		t.Fatalf("selection after restore = %s, want %s", selected, newer)
	}
	entries, err := os.ReadDir(snapshots)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{older, newer}) {
		t.Fatalf("snapshots after restore = %v, want only %s and %s", names, older, newer)
	}
	for _, args := range readers {
		if out, err := runIssue929(t, transport, environment, args...); err != nil {
			t.Fatalf("%v after restore: %v\n%s", args, err, out)
		}
	}
	if out, err := runIssue929(t, transport, environment, "catalog", "restore", issue929OfficialCatalog, older); err != nil || !strings.Contains(out, "Catalog Snapshot "+older+" of "+issue929OfficialCatalog+" is intact") {
		t.Fatalf("catalog restore of an intact snapshot: %v\n%s", err, out)
	}
	out, err := runIssue929(t, transport, environment, "catalog", "restore", issue929Acme, older)
	if !issue929Exit(err, 1) || !strings.Contains(out, "catalog acme/packs is not registered; run `packy catalog list` to see registered catalogs, or `packy catalog add acme/packs` to register it") {
		t.Fatalf("catalog restore of an unregistered catalog: %v\n%s", err, out)
	}
}
