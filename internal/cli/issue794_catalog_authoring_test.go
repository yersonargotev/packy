package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
)

type authoringOriginResolver map[string]string

func (r authoringOriginResolver) Resolve(_ context.Context, origin managedpack.Origin) (string, error) {
	return r[origin.Repository+"@"+origin.Commit], nil
}

func TestCatalogCreateAndImportPrepareReviewableValidatedContent(t *testing.T) {
	project := writeAuthoringCatalog(t)
	origin := t.TempDir()
	writeAuthoringFile(t, filepath.Join(origin, "LICENSE"), "MIT License\n")
	writeAuthoringFile(t, filepath.Join(origin, "skills", "focus", "SKILL.md"), "# Focus\n")
	commit := strings.Repeat("a", 40)
	opts := Options{
		CatalogOriginResolver: authoringOriginResolver{"example/upstream@" + commit: origin},
	}

	out, err := executeCommand(t, NewRootCommand(opts),
		"catalog", "create", "focus-pack",
		"--project", project,
		"--template", "empty",
		"--version", "0.1.0",
		"--description", "Focused maintainer workflows",
		"--surface", "codex",
	)
	if err != nil {
		t.Fatalf("catalog create: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created focus-pack@0.1.0 from template empty") {
		t.Fatalf("create output = %q", out)
	}

	out, err = executeCommand(t, NewRootCommand(opts),
		"catalog", "import", "focus-pack",
		"--project", project,
		"--repository", "example/upstream",
		"--commit", commit,
		"--origin-id", "upstream",
		"--origin-path", "LICENSE",
		"--destination", "notices/focus-mit",
		"--relationship", "exact-copy",
		"--kind", "notice",
		"--resource-id", "focus-mit",
		"--description", "Preserves the upstream MIT notice",
		"--license", "MIT",
		"--attribution", "Copyright (c) Example",
	)
	if err != nil {
		t.Fatalf("catalog import notice: %v\n%s", err, out)
	}

	out, err = executeCommand(t, NewRootCommand(opts),
		"catalog", "import", "focus-pack",
		"--project", project,
		"--repository", "example/upstream",
		"--commit", commit,
		"--origin-id", "upstream",
		"--origin-path", "skills/focus",
		"--destination", "skills/focus-pack/focus",
		"--relationship", "adapted",
		"--kind", "skill",
		"--resource-id", "focus",
		"--description", "Keeps work focused",
		"--host", "codex",
		"--notice", "notice:focus-mit",
	)
	if err != nil {
		t.Fatalf("catalog import skill: %v\n%s", err, out)
	}
	for _, want := range []string{
		"imported skill:focus",
		"example/upstream@" + commit,
		"adapted",
		"validated Catalog Project packs=2",
		"publication not performed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("import output missing %q: %s", want, out)
		}
	}

	validation, err := managedpack.ValidateCatalogProject(context.Background(), project, "", opts.CatalogOriginResolver)
	if err != nil {
		t.Fatalf("resulting Catalog Project: %v", err)
	}
	if len(validation.Packs) != 2 {
		t.Fatalf("validated Packs = %d, want 2", len(validation.Packs))
	}
	var imported managedpack.Manifest
	for _, pack := range validation.Packs {
		if pack.Manifest.ID == "focus-pack" {
			imported = pack.Manifest
		}
	}
	if len(imported.Origins) != 1 || imported.Origins[0].Commit != commit || len(imported.Resources) != 2 {
		t.Fatalf("imported provenance = origins %#v resources %#v", imported.Origins, imported.Resources)
	}
	if got := imported.Resources[1]; got.Origin == nil || got.Origin.Relationship != managedpack.RelationshipAdapted || len(got.Notices) != 1 || got.Notices[0] != "notice:focus-mit" {
		t.Fatalf("imported skill provenance = %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(project, "bundle", "skills", "focus-pack", "focus", "SKILL.md"))
	if err != nil || string(data) != "# Focus\n" {
		t.Fatalf("imported resource = %q, %v", data, err)
	}
}

func TestCatalogImportDiagnosesMissingInformationWithoutPartialChanges(t *testing.T) {
	project := writeAuthoringCatalog(t)
	origin := t.TempDir()
	writeAuthoringFile(t, filepath.Join(origin, "LICENSE"), "MIT License\n")
	writeAuthoringFile(t, filepath.Join(origin, "guide.md"), "guidance\n")
	commit := strings.Repeat("b", 40)
	opts := Options{
		CatalogOriginResolver: authoringOriginResolver{"example/guide@" + commit: origin},
	}

	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts),
		"catalog", "import", "seed",
		"--project", project,
		"--repository", "example/guide",
		"--commit", commit,
		"--origin-id", "guide",
		"--origin-path", "guide.md",
		"--destination", "instructions/guide.md",
		"--relationship", "exact-copy",
		"--kind", "instruction",
		"--resource-id", "guide",
		"--description", "Imported guidance",
		"--host", "codex",
	)
	if err == nil {
		t.Fatalf("missing notice unexpectedly succeeded: %s", out)
	}
	for _, want := range []string{"resource notice is required", `detected upstream notice "LICENSE"`, "--notice"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("diagnostic missing %q: %v", want, err)
		}
	}
	if after := snapshotTree(t, project); after != before {
		t.Fatalf("failed import changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}

	out, err = executeCommand(t, NewRootCommand(opts),
		"catalog", "import", "seed",
		"--project", project,
		"--repository", "example/guide",
		"--commit", commit,
		"--origin-id", "guide",
		"--origin-path", "guide.md",
		"--destination", "instructions/guide.md",
		"--relationship", "exact-copy",
		"--kind", "instruction",
		"--resource-id", "guide",
		"--description", "Imported guidance",
		"--host", "codex",
		"--notice", "notice:missing",
	)
	if err == nil || !strings.Contains(err.Error(), `notice "notice:missing" does not exist`) {
		t.Fatalf("invalid prepared import = error %v, output %s", err, out)
	}
	if after := snapshotTree(t, project); after != before {
		t.Fatalf("validation failure changed Catalog Project\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func writeAuthoringCatalog(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	manifest := `{
  "schema_version": 1,
  "id": "seed",
  "version": "1.0.0",
  "description": "Seed Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": [],
  "external_requirements": [],
  "origins": [],
  "resources": []
}
`
	writeAuthoringFile(t, filepath.Join(project, "bundle", "packs", "seed", "pack.json"), manifest)
	return project
}

func writeAuthoringFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
