package capabilitypack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasProjectContractTreatsIncompleteContractAsPresent(t *testing.T) {
	root := t.TempDir()
	present, err := HasProjectContract(root)
	if err != nil || present {
		t.Fatalf("empty project presence = %t, err=%v", present, err)
	}

	if err := os.WriteFile(filepath.Join(root, "packy.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	present, err = HasProjectContract(root)
	if err != nil || !present {
		t.Fatalf("incomplete project presence = %t, err=%v", present, err)
	}
}
