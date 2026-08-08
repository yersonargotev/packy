package ci_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	catalogSnapshot   = regexp.MustCompile(`(?im)^The selectable pack catalog currently contains |^\| Pack \| Version \| Purpose \|$`)
	staleCatalogClaim = regexp.MustCompile("(?i)only\\s+`?matty`?\\s+and\\s+`?engram`?")
)

func TestPublicDocumentationUsesCanonicalPackDiscovery(t *testing.T) {
	root := repositoryRoot(t)
	documents := []string{"README.md", "docs/capability-packs.md"}
	for _, path := range documents {
		text := readFile(t, filepath.Join(root, filepath.FromSlash(path)))
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
