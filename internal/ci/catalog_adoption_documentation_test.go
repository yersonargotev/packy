package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogAdoptionDocumentsOrderedCleanHandoffAndPreservation(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "catalog-adoption.md"))
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(data)), " ")
	ordered := []string{
		"## 1. Inventory with the previous Packy",
		"packy status --project",
		"## 2. Preview and remove old-model installations",
		"packy deactivate <pack> --surface <surface> --dry-run",
		"packy deactivate <pack> --surface <surface> --project --dry-run",
		"packy uninstall <pack> --surface <surface> --dry-run",
		"## 3. Replace Packy and initialize the current catalog",
		"packy init",
		"## 4. Reinstall and reactivate explicitly",
		"packy install <pack> --surface <surface> --dry-run",
		"packy activate <pack> --surface <surface> --project --dry-run",
	}
	position := -1
	for _, phrase := range ordered {
		next := strings.Index(document[position+1:], phrase)
		if next < 0 {
			t.Fatalf("adoption guide omits ordered step %q", phrase)
		}
		position += next + 1
	}
	for _, phrase := range []string{
		"does not convert or delete source directories",
		"personal files", "credentials", "Engram Memory", "foreign host configuration",
		"There is no automatic converter, downgrade, or cleanup command",
		"withdrawal never triggers an automatic uninstall or downgrade",
	} {
		if !strings.Contains(document, phrase) {
			t.Fatalf("adoption guide omits preservation contract %q", phrase)
		}
	}
}
