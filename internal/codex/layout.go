package codex

import "path/filepath"

// CanonicalLayout contains the Codex-owned global configuration paths.
type CanonicalLayout struct {
	configFile string
	promptFile string
}

func NewCanonicalLayout(home string) CanonicalLayout {
	root := filepath.Join(home, ".codex")
	return CanonicalLayout{
		configFile: filepath.Join(root, "config.toml"),
		promptFile: filepath.Join(root, "AGENTS.md"),
	}
}

func (l CanonicalLayout) ConfigFile() string { return l.configFile }
func (l CanonicalLayout) PromptFile() string { return l.promptFile }
