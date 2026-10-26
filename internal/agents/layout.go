package agents

import "path/filepath"

// CanonicalLayout keeps the explicitly selected Codex role destination separate
// from the shared Agents convention. It does not include client configuration.
type CanonicalLayout struct{ rolesDir string }

func NewCanonicalLayout(home string) CanonicalLayout {
	return CanonicalLayout{rolesDir: filepath.Join(home, ".codex", "agents")}
}

func (l CanonicalLayout) RolesDir() string { return l.rolesDir }
