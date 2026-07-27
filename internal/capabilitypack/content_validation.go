package capabilitypack

import (
	"fmt"
	"os"
	"path/filepath"
)

// ValidatePortableContent validates every portable Pack manifest and each inert
// bundle resource it references. It parses declarations only; it never invokes
// a resource or an upstream tool.
func ValidatePortableContent(bundleRoot string) error {
	packsRoot := filepath.Join(bundleRoot, "packs")
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		return fmt.Errorf("read portable Pack manifests: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("portable Pack manifest directory is empty")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return fmt.Errorf("unexpected portable Pack manifest entry %q", entry.Name())
		}
		manifestPath := filepath.Join(packsRoot, entry.Name(), "pack.json")
		pack, err := decodeManifest(manifestPath, bundleRoot)
		if err != nil {
			return err
		}
		if pack.ID != entry.Name() {
			return fmt.Errorf("portable Pack directory %q contains manifest id %q", entry.Name(), pack.ID)
		}
	}
	return nil
}
