package opencode

import (
	"path/filepath"
	"testing"
)

func TestCanonicalLayoutOwnsOpenCodePaths(t *testing.T) {
	configHome := t.TempDir()
	layout := NewCanonicalLayout(configHome)

	if layout.ConfigurationHome() != configHome {
		t.Fatalf("ConfigurationHome = %q, want %q", layout.ConfigurationHome(), configHome)
	}
	if layout.ConfigFile() != filepath.Join(configHome, "opencode", "opencode.json") {
		t.Fatalf("ConfigFile = %q", layout.ConfigFile())
	}
	if layout.PromptFile() != filepath.Join(configHome, "opencode", "matty.md") {
		t.Fatalf("PromptFile = %q", layout.PromptFile())
	}
}
