package codex

import (
	"path/filepath"
	"testing"
)

func TestCanonicalLayoutOwnsCodexPaths(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)

	if layout.ConfigFile() != filepath.Join(home, ".codex", "config.toml") {
		t.Fatalf("ConfigFile = %q", layout.ConfigFile())
	}
	if layout.PromptFile() != filepath.Join(home, ".codex", "AGENTS.md") {
		t.Fatalf("PromptFile = %q", layout.PromptFile())
	}
}
