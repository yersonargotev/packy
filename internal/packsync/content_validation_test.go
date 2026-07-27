package packsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateContentAcceptsRepositoryBundle(t *testing.T) {
	if err := ValidateContent(filepath.Join("..", "..", "bundle")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateContentRequiresStrictSourceConfigLockBijection(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "bundle")
	if err := copyTreeError(filepath.Join("..", "..", "bundle"), bundle); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(bundle, "sources", "mattpocock-skills.lock.json")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContent(bundle); err == nil || !strings.Contains(err.Error(), "has no canonical lock") {
		t.Fatalf("missing lock error = %v", err)
	}
}
