package agents

import (
	"path/filepath"
	"testing"
)

func TestCanonicalLayoutSeparatesExplicitCodexRoles(t *testing.T) {
	home := t.TempDir()
	if got := NewCanonicalLayout(home).RolesDir(); got != filepath.Join(home, ".codex", "agents") {
		t.Fatal(got)
	}
}
