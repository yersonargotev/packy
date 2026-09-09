package localprojection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReceiptTargetSafety(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	target := filepath.Join(root, "skill")
	if err := os.WriteFile(filepath.Join(external, "SKILL.md"), []byte("skill"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, target); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptTarget(root, target, true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptTarget(root, target, false); err == nil {
		t.Fatal("file symlink accepted")
	}
	if err := ValidateReceiptTarget(root, filepath.Join(target, "SKILL.md"), false); err == nil {
		t.Fatal("symlink parent accepted")
	}
	if err := ValidateReceiptTarget(root, filepath.Join(external, "SKILL.md"), false); err == nil {
		t.Fatal("outside target accepted")
	}
	if err := os.Symlink(filepath.Join(external, "SKILL.md"), filepath.Join(external, "nested-link")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptTarget(root, target, true); err == nil {
		t.Fatal("nested skill symlink accepted")
	}
}
