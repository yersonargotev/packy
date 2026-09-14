package ci_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	catalogSnapshot   = regexp.MustCompile(`(?im)^The selectable pack catalog .*(?:contains|includes)|^\|\s*Pack\s*\|\s*Version\s*\|`)
	staleCatalogClaim = regexp.MustCompile("(?i)only\\s+`?matty`?\\s+and\\s+`?engram`?")
)

const canonicalCatalogAuthority = "Catalog Project is the canonical authoring source"

func TestPublicDocumentationUsesCanonicalPackDiscovery(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{"README.md", "docs/capability-packs.md"} {
		text := readFile(t, filepath.Join(root, filepath.FromSlash(path)))
		if !strings.Contains(text, canonicalCatalogAuthority) {
			t.Errorf("%s does not identify the Catalog Project as canonical authoring authority", path)
		}
		if !strings.Contains(text, "packy list") {
			t.Errorf("%s does not expose canonical runtime Pack discovery", path)
		}
		if catalogSnapshot.MatchString(text) {
			t.Errorf("%s duplicates a selectable Pack catalog snapshot", path)
		}
		if staleCatalogClaim.MatchString(text) {
			t.Errorf("%s retains stale only-Matty-and-Engram catalog claim", path)
		}
		if strings.Contains(text, "docs/packs/") || strings.Contains(text, "packs/index.md") {
			t.Errorf("%s links to removed generated Pack documentation", path)
		}
	}
}

func TestCurrentProductGuidanceUsesManifestBackedPackDiscovery(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{"docs/product/packy-v0.md", "docs/roadmap.md"} {
		text := readFile(t, filepath.Join(root, filepath.FromSlash(path)))
		if !strings.Contains(text, "manifest-backed Pack catalog") {
			t.Errorf("%s does not direct readers to manifest-backed Pack discovery", path)
		}
		if strings.Contains(text, "four reviewed Packs") || strings.Contains(text, "current four") {
			t.Errorf("%s retains a brittle current catalog count", path)
		}
	}
}
