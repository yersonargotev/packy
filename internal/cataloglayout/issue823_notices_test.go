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

func TestIssue823NoticeVariantPreservesOriginalLegalTextAndAttribution(t *testing.T) {
	for _, failure := range []string{"", "text", "attribution", "license"} {
		t.Run("preserve-"+failure, func(t *testing.T) {
			fixture := testsupport.AllKindVariants("legal")
			root := t.TempDir()
			if err := fixture.WriteCatalog(root); err != nil {
				t.Fatal(err)
			}
			packRoot := filepath.Join(root, "packs/legal")
			if failure == "text" {
				if err := os.WriteFile(filepath.Join(packRoot, "notices/opencode.md"), []byte("Replacement omits original terms\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "attribution" || failure == "license" {
				path := filepath.Join(packRoot, "pack.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var manifest Manifest
				if err := json.Unmarshal(data, &manifest); err != nil {
					t.Fatal(err)
				}
				for i := range manifest.Resources {
					r := &manifest.Resources[i]
					if r.Kind != "notice" {
						continue
					}
					changed := "Replacement"
					if failure == "attribution" {
						r.Variants[2].Attribution = &changed
					} else {
						r.Variants[2].License = &changed
					}
				}
				data, err = json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := ValidateCatalogProject(context.Background(), root, "", nil)
			if failure == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "preserve") {
				t.Fatalf("invalid non-target notice variant accepted: %v", err)
			}
		})
	}
}
