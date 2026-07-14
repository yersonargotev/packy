package skillbundle

import (
	"path/filepath"
	"testing"
)

func TestGlobalLayoutOwnsSkillInstallationPaths(t *testing.T) {
	home := t.TempDir()
	layout := NewGlobalLayout(home)

	wantRoot := filepath.Join(home, ".agents", "skills")
	if layout.Root() != wantRoot {
		t.Fatalf("Root = %q, want %q", layout.Root(), wantRoot)
	}
	if layout.Skill("ask-matt") != filepath.Join(wantRoot, "ask-matt") {
		t.Fatalf("Skill = %q", layout.Skill("ask-matt"))
	}
}
