package ci_test

import (
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

var (
	catalogDeclaration = regexp.MustCompile(`(?m)^The selectable pack catalog currently contains ((?:` + "`[^`]+`" + `(?:, |, and )?)+)\.$`)
	catalogID          = regexp.MustCompile("`([^`]+)`")
	staleCatalogClaim  = regexp.MustCompile("(?i)only\\s+`?matty`?\\s+and\\s+`?engram`?")
)

func TestPublicDocumentationNamesExactSelectablePackCatalog(t *testing.T) {
	root := repositoryRoot(t)
	catalog, err := capabilitypack.Discover(filepath.Join(root, "bundle"))
	if err != nil {
		t.Fatalf("discover production catalog: %v", err)
	}
	var want []string
	packs, err := catalog.ListCurrent()
	if err != nil {
		t.Fatalf("list production selectable pack catalog: %v", err)
	}
	for _, pack := range packs {
		want = append(want, pack.ID)
	}
	sort.Strings(want)

	documents := []string{"README.md", "docs/capability-packs.md"}
	for _, path := range documents {
		text := readFile(t, filepath.Join(root, filepath.FromSlash(path)))
		got := documentedCatalogIDs(text)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s selectable pack catalog IDs = %#v, want production selectable pack catalog %#v", path, got, want)
		}
		if staleCatalogClaim.MatchString(text) {
			t.Errorf("%s retains stale only-Matty-and-Engram catalog claim", path)
		}
	}
}

func documentedCatalogIDs(text string) []string {
	declarations := catalogDeclaration.FindAllStringSubmatch(text, -1)
	if len(declarations) != 1 {
		return nil
	}
	matches := catalogID.FindAllStringSubmatch(declarations[0][1], -1)
	ids := make([]string, 0, len(matches))
	for _, item := range matches {
		ids = append(ids, item[1])
	}
	return ids
}
