package cataloglayout

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestVariantCatalogClosesEveryReviewedBodyAndBuildsDeterministically(t *testing.T) {
	fixture := testsupport.SkillVariants("variants", testsupport.SurfaceClaude, testsupport.SurfaceCodex, testsupport.SurfaceOpenCode)
	root := t.TempDir()
	if err := fixture.WriteCatalog(root); err != nil {
		t.Fatal(err)
	}
	origins, err := fixture.WriteProject(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateCatalogProject(context.Background(), root, "", originResolver(origins))
	if err != nil {
		t.Fatal(err)
	}
	source := CatalogSnapshotSource{Repository: "yersonargotev/packy-catalog", Commit: strings.Repeat("a", 40), Builder: "yersonargotev/packy@" + strings.Repeat("b", 40)}
	first, err := BuildCatalogSnapshot(context.Background(), root, validation, source, filepath.Join(t.TempDir(), "dist"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildCatalogSnapshot(context.Background(), root, validation, source, filepath.Join(t.TempDir(), "dist"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ArchiveSHA256 != second.ArchiveSHA256 {
		t.Fatal("variant snapshot is nondeterministic")
	}
	pack, err := ValidateCatalogPack(context.Background(), root, fixture.ID(), originResolver(origins))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]bool{}
	for _, file := range pack.Files {
		files[file.Path] = true
	}
	for _, body := range []string{"common", "claude", "codex", "opencode"} {
		for _, file := range []string{"SKILL.md", "references/detail.md", "agents/openai.yaml"} {
			if !files["skills/"+body+"/"+file] {
				t.Fatalf("missing reviewed body %s/%s", body, file)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(root, "packs/variants/skills/common/references/detail.md"), []byte("unreviewed original"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCatalogProject(context.Background(), root, "", originResolver(origins)); err == nil || !strings.Contains(err.Error(), "exact-copy") {
		t.Fatalf("unused common original escaped provenance: %v", err)
	}
	if _, err := BuildCatalogSnapshot(context.Background(), root, validation, source, filepath.Join(t.TempDir(), "dist")); err == nil {
		t.Fatal("stale variant snapshot preview accepted")
	}
}

func TestVariantCatalogRejectsInvalidBodiesWithoutFallback(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any, string)
		want   string
	}{
		{"missing variant source", func(v map[string]any, _ string) { v["source"] = "skills/missing" }, "missing"},
		{"escaping variant source", func(v map[string]any, _ string) { v["source"] = "../outside" }, "escapes"},
		{"missing notice", func(v map[string]any, _ string) { v["notices"] = []string{"notice:missing"} }, "does not exist"},
		{"empty notice closure", func(v map[string]any, _ string) { v["notices"] = []string{} }, "at least one notice"},
		{"unknown origin", func(v map[string]any, _ string) { v["origin"].(map[string]any)["id"] = "unknown" }, "unknown origin"},
		{"false exact copy", func(v map[string]any, _ string) { v["origin"].(map[string]any)["relationship"] = "exact-copy" }, "exact-copy mismatch"},
		{"null collection", func(v map[string]any, _ string) { v["notices"] = nil }, "must not be null"},
		{"symlink variant", func(_ map[string]any, root string) {
			target := filepath.Join(root, "skills/codex/references/detail.md")
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "skills/common/references/detail.md"), target); err != nil {
				t.Fatal(err)
			}
		}, "symlink"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := testsupport.SkillVariants("variants", testsupport.SurfaceCodex)
			root := t.TempDir()
			if err := fixture.WriteCatalog(root); err != nil {
				t.Fatal(err)
			}
			origins, err := fixture.WriteProject(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			packRoot := filepath.Join(root, "packs/variants")
			path := filepath.Join(packRoot, "pack.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			variant := manifest["resources"].([]any)[1].(map[string]any)["variants"].([]any)[0].(map[string]any)
			test.mutate(variant, packRoot)
			data, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := ValidateCatalogProject(context.Background(), root, "", originResolver(origins)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %s", err, test.want)
			}
		})
	}
}
