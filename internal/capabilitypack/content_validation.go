package capabilitypack

import (
	"fmt"
	"os"
	"path/filepath"
)

// ValidatePackContent validates one named Pack or Pack directory through the
// current authoring contract and verifies every referenced reviewed resource.
func ValidatePackContent(bundleRoot, pack string) (Pack, error) {
	manifestPath, packDir, err := currentManifestPath(bundleRoot, pack)
	if err != nil {
		return Pack{}, err
	}
	loaded, err := LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err != nil {
		return Pack{}, err
	}
	if filepath.Clean(filepath.Dir(packDir)) == filepath.Clean(filepath.Join(bundleRoot, "packs")) && loaded.ID != filepath.Base(packDir) {
		return Pack{}, fmt.Errorf("Pack directory %q contains manifest id %q", filepath.Base(packDir), loaded.ID)
	}
	return loaded, nil
}

// ValidatePortableContent validates every portable Pack manifest and each inert
// bundle resource it references. It parses declarations only; it never invokes
// a resource or an upstream tool.
func ValidatePortableContent(bundleRoot string) error {
	packsRoot := filepath.Join(bundleRoot, "packs")
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		return fmt.Errorf("read portable Pack manifests: %w", err)
	}
	validated := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			return fmt.Errorf("unexpected portable Pack manifest entry %q", entry.Name())
		}
		if _, err := ValidatePackContent(bundleRoot, entry.Name()); err != nil {
			return err
		}
		validated++
	}
	if validated == 0 {
		return fmt.Errorf("current Pack manifest directory is empty")
	}
	return nil
}
