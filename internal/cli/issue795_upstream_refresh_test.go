package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
)

func TestCatalogUpstreamRefreshUpdatesExactCopiesAndProvenance(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipExactCopy)

	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver}),
		"catalog", "upstream-refresh", "seed",
		"--project", fixture.project,
		"--origin-id", "upstream",
		"--commit", fixture.newCommit,
		"--version", "1.1.0",
	)
	if err != nil {
		t.Fatalf("catalog upstream-refresh: %v\n%s", err, out)
	}
	for _, want := range []string{
		"refreshed 1 exact-copy resource",
		fixture.oldCommit + " -> " + fixture.newCommit,
		"validated Catalog Project packs=1",
		"publication not performed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("refresh output missing %q: %s", want, out)
		}
	}
	data, err := os.ReadFile(filepath.Join(fixture.project, "bundle", "skills", "seed", "SKILL.md"))
	if err != nil || string(data) != "# New upstream\n" {
		t.Fatalf("refreshed resource = %q, %v", data, err)
	}
	manifest := readUpstreamRefreshManifest(t, fixture.project)
	if manifest.Version != "1.1.0" || manifest.Origins[0].Commit != fixture.newCommit {
		t.Fatalf("refreshed manifest = version %q origins %#v", manifest.Version, manifest.Origins)
	}
}

func TestCatalogUpstreamRefreshRejectsUnexpectedExactCopyChanges(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipExactCopy)
	writeAuthoringFile(t, filepath.Join(fixture.project, "bundle", "skills", "seed", "SKILL.md"), "# Local edit\n")
	before := snapshotTree(t, filepath.Join(fixture.project, "bundle"))

	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver}),
		"catalog", "upstream-refresh", "seed",
		"--project", fixture.project,
		"--origin-id", "upstream",
		"--commit", fixture.newCommit,
		"--version", "1.1.0",
	)
	if err == nil || !strings.Contains(err.Error(), "exact-copy") {
		t.Fatalf("locally modified refresh = error %v, output %s", err, out)
	}
	if after := snapshotTree(t, filepath.Join(fixture.project, "bundle")); after != before {
		t.Fatalf("rejected refresh changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestCatalogUpstreamRefreshLeavesProjectUnchangedOnAcquisitionFailure(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipExactCopy)
	resolver := failingRefreshResolver{delegate: fixture.resolver, commit: fixture.newCommit}
	before := snapshotTree(t, filepath.Join(fixture.project, "bundle"))

	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: resolver}),
		"catalog", "upstream-refresh", "seed",
		"--project", fixture.project,
		"--origin-id", "upstream",
		"--commit", fixture.newCommit,
		"--version", "1.1.0",
	)
	if err == nil || !strings.Contains(err.Error(), "resolve selected upstream commit") {
		t.Fatalf("failed acquisition = error %v, output %s", err, out)
	}
	if after := snapshotTree(t, filepath.Join(fixture.project, "bundle")); after != before {
		t.Fatalf("acquisition failure changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestCatalogUpstreamRefreshLeavesProjectUnchangedOnValidationFailure(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipExactCopy)
	if err := os.Remove(filepath.Join(fixture.newRoot, "skills", "seed", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	writeAuthoringFile(t, filepath.Join(fixture.newRoot, "skills", "seed", "README.md"), "missing skill entrypoint\n")
	before := snapshotTree(t, filepath.Join(fixture.project, "bundle"))

	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver}),
		"catalog", "upstream-refresh", "seed",
		"--project", fixture.project,
		"--origin-id", "upstream",
		"--commit", fixture.newCommit,
		"--version", "1.1.0",
	)
	if err == nil || !strings.Contains(err.Error(), "validate prepared Catalog Project") || !strings.Contains(err.Error(), "missing SKILL.md") {
		t.Fatalf("failed validation = error %v, output %s", err, out)
	}
	if after := snapshotTree(t, filepath.Join(fixture.project, "bundle")); after != before {
		t.Fatalf("validation failure changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestCatalogUpstreamRefreshRejectsAdaptedResourcesWithoutChanges(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipAdapted)
	before := snapshotTree(t, filepath.Join(fixture.project, "bundle"))

	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver}),
		"catalog", "upstream-refresh", "seed",
		"--project", fixture.project,
		"--origin-id", "upstream",
		"--commit", fixture.newCommit,
		"--version", "1.1.0",
	)
	if err == nil || !strings.Contains(err.Error(), "requires explicit reconciliation") {
		t.Fatalf("adapted refresh = error %v, output %s", err, out)
	}
	for _, want := range []string{"adapted resource skill:seed upstream changes", "-# Old upstream", "+# New upstream"} {
		if !strings.Contains(out, want) {
			t.Fatalf("unresolved adaptation output missing %q: %s", want, out)
		}
	}
	if after := snapshotTree(t, filepath.Join(fixture.project, "bundle")); after != before {
		t.Fatalf("adapted rejection changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestCatalogUpstreamRefreshPreservesConcurrentCatalogChange(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipExactCopy)
	manifestPath := filepath.Join(fixture.project, "bundle", "packs", "seed", "pack.json")
	resolver := &concurrentRefreshResolver{delegate: fixture.resolver, commit: fixture.newCommit}
	resolver.mutate = func() {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: resolver}),
		"catalog", "upstream-refresh", "seed",
		"--project", fixture.project,
		"--origin-id", "upstream",
		"--commit", fixture.newCommit,
		"--version", "1.1.0",
	)
	if err == nil || !strings.Contains(err.Error(), "Catalog Project changed during preparation") {
		t.Fatalf("concurrent refresh = error %v, output %s", err, out)
	}
	data, readErr := os.ReadFile(manifestPath)
	if readErr != nil || !strings.HasSuffix(string(data), "\n\n") {
		t.Fatalf("concurrent manifest change was not preserved: %q, %v", data, readErr)
	}
	resource, readErr := os.ReadFile(filepath.Join(fixture.project, "bundle", "skills", "seed", "SKILL.md"))
	if readErr != nil || string(resource) != "# Old upstream\n" {
		t.Fatalf("rejected refresh changed resource: %q, %v", resource, readErr)
	}
}

type upstreamRefreshFixture struct {
	project, oldRoot, newRoot string
	oldCommit, newCommit      string
	resolver                  authoringOriginResolver
}

func writeUpstreamRefreshFixture(t *testing.T, relationship managedpack.Relationship) upstreamRefreshFixture {
	t.Helper()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	oldCommit, newCommit := strings.Repeat("d", 40), strings.Repeat("e", 40)
	writeAuthoringFile(t, filepath.Join(oldRoot, "skills", "seed", "SKILL.md"), "# Old upstream\n")
	writeAuthoringFile(t, filepath.Join(newRoot, "skills", "seed", "SKILL.md"), "# New upstream\n")
	writeAuthoringFile(t, filepath.Join(project, "bundle", "skills", "seed", "SKILL.md"), "# Old upstream\n")
	writeAuthoringFile(t, filepath.Join(project, "bundle", "notices", "seed"), "Catalog-authored notice\n")
	manifest := `{
  "schema_version": 1,
  "id": "seed",
  "version": "1.0.0",
  "description": "Seed Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": [],
  "external_requirements": [],
  "origins": [{"id":"upstream","repository":"example/upstream","commit":"` + oldCommit + `"}],
  "resources": [{
    "kind":"notice",
    "id":"seed",
    "source":"notices/seed",
    "description":"Catalog-authored notice",
    "license":"MIT",
    "attribution":"Example",
    "requires":[],
    "conflicts":[],
    "notices":[],
    "bindings":[],
    "surface_exclusions":[]
  },{
    "kind":"skill",
    "id":"seed",
    "source":"skills/seed",
    "description":"Seed workflow",
    "requires":[],
    "conflicts":[],
    "notices":["notice:seed"],
    "origin":{"id":"upstream","path":"skills/seed","relationship":"` + string(relationship) + `"},
    "bindings":[{"surface":"codex","projection":"skill","name":"seed","invocation":"$seed","mode":"native","sharing":"exclusive","capabilities":[]}],
    "surface_exclusions":[]
  }]
}
`
	writeAuthoringFile(t, filepath.Join(project, "bundle", "packs", "seed", "pack.json"), manifest)
	return upstreamRefreshFixture{
		project: project, oldRoot: oldRoot, newRoot: newRoot,
		oldCommit: oldCommit, newCommit: newCommit,
		resolver: authoringOriginResolver{
			"example/upstream@" + oldCommit: oldRoot,
			"example/upstream@" + newCommit: newRoot,
		},
	}
}

func readUpstreamRefreshManifest(t *testing.T, project string) managedpack.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, "bundle", "packs", "seed", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest managedpack.Manifest
	err = json.Unmarshal(data, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

type failingRefreshResolver struct {
	delegate authoringOriginResolver
	commit   string
}

func (r failingRefreshResolver) Resolve(ctx context.Context, origin managedpack.Origin) (string, error) {
	if origin.Commit == r.commit {
		return "", errors.New("upstream unavailable")
	}
	return r.delegate.Resolve(ctx, origin)
}

type concurrentRefreshResolver struct {
	delegate authoringOriginResolver
	commit   string
	mutate   func()
	mutated  bool
}

func (r *concurrentRefreshResolver) Resolve(ctx context.Context, origin managedpack.Origin) (string, error) {
	if origin.Commit == r.commit && !r.mutated {
		r.mutated = true
		r.mutate()
	}
	return r.delegate.Resolve(ctx, origin)
}
