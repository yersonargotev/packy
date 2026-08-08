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

const canonicalCatalogAuthority = "The bundled Pack manifests are the canonical selectable catalog."

func TestPublicDocumentationUsesCanonicalPackDiscovery(t *testing.T) {
	root := repositoryRoot(t)
	documents := []string{"README.md", "docs/capability-packs.md"}
	for _, path := range documents {
		text := readFile(t, filepath.Join(root, filepath.FromSlash(path)))
		if !strings.Contains(text, canonicalCatalogAuthority) {
			t.Errorf("%s does not identify bundled Pack manifests as canonical catalog authority", path)
		}
		if !strings.Contains(text, "packy pack list") {
			t.Errorf("%s does not expose canonical runtime Pack discovery", path)
		}
		if catalogSnapshot.MatchString(text) {
			t.Errorf("%s duplicates a selectable Pack catalog snapshot", path)
		}
		if staleCatalogClaim.MatchString(text) {
			t.Errorf("%s retains stale only-Matty-and-Engram catalog claim", path)
		}
	}
}
