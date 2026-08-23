package repositorycandidate

import (
	"context"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestPrepareRendersTheSemanticReportInsteadOfCandidateInventories(t *testing.T) {
	repositoryRoot, _, _ := packyRepository(t, map[string]string{
		"bundle/packs/example/pack.json": managedManifest("1.0.0", "old guidance\n", nil),
		"bundle/skills/guide/SKILL.md":   "old guidance\n",
		"docs/packs/example.md":          "old pack docs\n",
		"docs/packs/index.md":            "old index\n",
	})
	projectRoot, validation := managedProject(t, "1.0.1", "new guidance\n", nil)
	gates := &fakeGates{t: t, generateDocs: func(root string) error {
		writeTestFile(t, root+"/docs/packs/example.md", "new pack docs\n", 0o644)
		writeTestFile(t, root+"/docs/packs/index.md", "new index\n", 0o644)
		return nil
	}}

	prepared, err := newWithGates(gates).Prepare(context.Background(), repositoryRoot, acquisitionFor("1.0.1", projectRoot), validation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Cleanup() })
	summary := prepared.Candidate.Summary
	for _, want := range []string{
		"## Semantic changes",
		"Resource `skill:guide` content changed.",
		"### Compatibility",
		"### Human judgment",
		"review it without inferring behavioral compatibility",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("candidate summary lacks %q:\n%s", want, summary)
		}
	}
	for _, retired := range []string{"- Origins:", "- Adaptations:", "- Notice coverage:"} {
		if strings.Contains(summary, retired) {
			t.Fatalf("candidate summary retained candidate inventory %q:\n%s", retired, summary)
		}
	}
}

func TestCandidateIDSealsTheRenderedSemanticReport(t *testing.T) {
	current := semanticManifest("1.0.0")
	candidate := semanticManifest("1.0.1")
	currentFiles := []managedpack.FileRecord{{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "old"}}
	first := compareSemanticChanges(current, currentFiles, candidate, []managedpack.FileRecord{{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "first"}})
	secondCandidate := candidate
	secondCandidate.Description = "Changed review evidence"
	second := compareSemanticChanges(current, currentFiles, secondCandidate, []managedpack.FileRecord{{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "first"}})
	if first.renderMarkdown() == second.renderMarkdown() {
		t.Fatal("semantic report mutation did not change rendered evidence")
	}
	coordinate, err := managedpackpromotion.ParseCoordinate("example@1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	firstID := candidateID(coordinate, "owner/example", "base", "head", "tree", first.renderMarkdown())
	secondID := candidateID(coordinate, "owner/example", "base", "head", "tree", second.renderMarkdown())
	if firstID == secondID {
		t.Fatal("Candidate.ID did not seal the rendered semantic report")
	}
}
