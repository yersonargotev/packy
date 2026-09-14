package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

func TestCatalogUpstreamRefreshReconcilesAdaptedResourceExplicitly(t *testing.T) {
	fixture := writeMixedUpstreamRefreshFixture(t, true)
	writeAuthoringFile(t, filepath.Join(fixture.oldRoot, "skills", "seed", "SKILL.md"), "shared\nold upstream\nretained\n")
	writeAuthoringFile(t, filepath.Join(fixture.newRoot, "skills", "seed", "SKILL.md"), "shared\nnew upstream\nretained\n")
	maintained := "# Maintained adaptation\n"
	writeAuthoringFile(t, filepath.Join(fixture.project, "bundle", "skills", "seed", "SKILL.md"), maintained)
	terminal := &fakeTerminal{interactive: true, approve: true}

	out, err := executeCommand(t, NewRootCommand(Options{
		CatalogOriginResolver: fixture.resolver,
		Terminal:              terminal,
	}),
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
		"adapted resource skill:seed upstream changes",
		"-old upstream",
		"+new upstream",
		"refreshed 1 exact-copy resource",
		"reconciled 1 adapted resource",
		"publication not performed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("refresh output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "shared") || strings.Contains(out, "retained") {
		t.Fatalf("upstream diff reported unchanged lines: %s", out)
	}
	if terminal.calls != 1 || !strings.Contains(terminal.prompts[0], "skill:seed") {
		t.Fatalf("reconciliation prompts = %#v", terminal.prompts)
	}
	data, readErr := os.ReadFile(filepath.Join(fixture.project, "bundle", "skills", "seed", "SKILL.md"))
	if readErr != nil || string(data) != maintained {
		t.Fatalf("maintained adaptation = %q, %v", data, readErr)
	}
	exact, readErr := os.ReadFile(filepath.Join(fixture.project, "bundle", "instructions", "guide.md"))
	if readErr != nil || string(exact) != "new exact copy\n" {
		t.Fatalf("refreshed exact copy = %q, %v", exact, readErr)
	}
	manifest := readUpstreamRefreshManifest(t, fixture.project)
	if manifest.Version != "1.1.0" || manifest.Origins[0].Commit != fixture.newCommit {
		t.Fatalf("refreshed manifest = version %q origins %#v", manifest.Version, manifest.Origins)
	}
	var adapted managedpack.Resource
	for _, resource := range manifest.Resources {
		if resource.Kind == "skill" && resource.ID == "seed" {
			adapted = resource
		}
	}
	if got := adapted.Notices; len(got) != 1 || got[0] != "notice:seed" {
		t.Fatalf("adapted resource notices = %#v", got)
	}
}

func TestCatalogUpstreamRefreshDoesNotPromptForIncompleteAdaptationDiff(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipAdapted)
	writeAuthoringFile(t, filepath.Join(fixture.oldRoot, "skills", "seed", "SKILL.md"), strings.Repeat("old upstream line\n", 5000))
	writeAuthoringFile(t, filepath.Join(fixture.newRoot, "skills", "seed", "SKILL.md"), strings.Repeat("new upstream line\n", 5000))
	before := snapshotTree(t, filepath.Join(fixture.project, "bundle"))
	terminal := &fakeTerminal{interactive: true, approve: true}

	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver, Terminal: terminal}),
		"catalog", "upstream-refresh", "seed", "--project", fixture.project,
		"--origin-id", "upstream", "--commit", fixture.newCommit, "--version", "1.1.0")
	if err == nil || !strings.Contains(err.Error(), "differences exceed") {
		t.Fatalf("oversized adaptation diff = %v, output %s", err, out)
	}
	if terminal.calls != 0 {
		t.Fatalf("incomplete diff prompted %d times", terminal.calls)
	}
	if after := snapshotTree(t, filepath.Join(fixture.project, "bundle")); after != before {
		t.Fatalf("incomplete diff changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestCatalogUpstreamRefreshLeavesMixedRequestUnappliedOnFailure(t *testing.T) {
	fixture := writeMixedUpstreamRefreshFixture(t, false)
	before := snapshotTree(t, filepath.Join(fixture.project, "bundle"))
	terminal := &fakeTerminal{interactive: true, approve: true}

	out, err := executeCommand(t, NewRootCommand(Options{
		CatalogOriginResolver: fixture.resolver,
		Terminal:              terminal,
	}),
		"catalog", "upstream-refresh", "seed",
		"--project", fixture.project,
		"--origin-id", "upstream",
		"--commit", fixture.newCommit,
		"--version", "1.1.0",
	)
	if err == nil || !strings.Contains(err.Error(), "prepare exact-copy resource") {
		t.Fatalf("mixed refresh failure = %v, output %s", err, out)
	}
	if terminal.calls != 1 {
		t.Fatalf("reconciliation prompt calls = %d", terminal.calls)
	}
	if after := snapshotTree(t, filepath.Join(fixture.project, "bundle")); after != before {
		t.Fatalf("failed mixed refresh changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func writeMixedUpstreamRefreshFixture(t *testing.T, includeNewExactCopy bool) upstreamRefreshFixture {
	t.Helper()
	fixture := writeUpstreamRefreshFixture(t, managedpack.RelationshipAdapted)
	writeAuthoringFile(t, filepath.Join(fixture.oldRoot, "guide.md"), "old exact copy\n")
	writeAuthoringFile(t, filepath.Join(fixture.project, "bundle", "instructions", "guide.md"), "old exact copy\n")
	if includeNewExactCopy {
		writeAuthoringFile(t, filepath.Join(fixture.newRoot, "guide.md"), "new exact copy\n")
	}
	manifest := readUpstreamRefreshManifest(t, fixture.project)
	manifest.Resources = append([]managedpack.Resource{{
		Kind: "instruction", ID: "guide", Source: "instructions/guide.md", Description: "Exact guidance",
		Requires: []string{}, Conflicts: []string{}, Notices: []string{"notice:seed"},
		Origin:            &managedpack.ResourceOrigin{ID: "upstream", Path: "guide.md", Relationship: managedpack.RelationshipExactCopy},
		Bindings:          []capabilitypack.Binding{{Surface: "codex", Projection: "instruction", Name: "guide", Invocation: "guide", Mode: "native", Sharing: "shared", Capabilities: []capabilitypack.SurfaceCapability{}}},
		SurfaceExclusions: []capabilitypack.SurfaceExclusion{},
	}}, manifest.Resources...)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeAuthoringFile(t, filepath.Join(fixture.project, "bundle", "packs", "seed", "pack.json"), string(data)+"\n")
	return fixture
}
