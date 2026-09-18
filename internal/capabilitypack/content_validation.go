package capabilitypack

import (
	"fmt"
	"os"
	"path/filepath"
)

// ValidatePackContent validates one named Pack or Pack directory through the
// current authoring contract and verifies every referenced reviewed resource.
func ValidatePackContent(catalogRoot, pack string) (Pack, error) {
	manifestPath, packDir, err := currentManifestPath(catalogRoot, pack)
	if err != nil {
		return Pack{}, err
	}
	loaded, err := LoadCurrentManifest(manifestPath, packDir, true)
	if err != nil {
		return Pack{}, err
	}
	if filepath.Clean(filepath.Dir(packDir)) == filepath.Clean(filepath.Join(catalogRoot, "packs")) && loaded.ID != filepath.Base(packDir) {
		return Pack{}, fmt.Errorf("Pack directory %q contains manifest id %q", filepath.Base(packDir), loaded.ID)
	}
	return loaded, nil
}

// ValidatePortableContent validates every portable Pack manifest and each inert
// Pack resource it references. It parses declarations only; it never invokes
// a resource or an upstream tool.
func ValidatePortableContent(catalogRoot string) error {
	packsRoot := filepath.Join(catalogRoot, "packs")
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		return fmt.Errorf("read portable Pack manifests: %w", err)
	}
	validated := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			return fmt.Errorf("unexpected portable Pack manifest entry %q", entry.Name())
		}
		if _, err := ValidatePackContent(catalogRoot, entry.Name()); err != nil {
			return err
		}
		validated++
	}
	if validated == 0 {
		return fmt.Errorf("current Pack manifest directory is empty")
	}
	return nil
}
