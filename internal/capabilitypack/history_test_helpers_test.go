package capabilitypack

import (
	"os"
	"path/filepath"
	"testing"
)

func mustDecodeHistoricalManifest(t *testing.T, root string) Pack {
	t.Helper()
	pack, err := decodeManifest(filepath.Join(root, "pack.json"), root)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func writeHistoricalArtifact(t *testing.T, root string, artifact historicalArtifact) {
	t.Helper()
	data, err := canonicalHistoricalArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "artifact.json"), data, 0o644)
}

func mustWrite(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
