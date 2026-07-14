package skillbundle

import "path/filepath"

// GlobalLayout is the canonical global installation surface for skills.
type GlobalLayout struct {
	root string
}

func NewGlobalLayout(home string) GlobalLayout {
	return GlobalLayout{root: filepath.Join(home, ".agents", "skills")}
}

func (l GlobalLayout) Root() string             { return l.root }
func (l GlobalLayout) Skill(name string) string { return filepath.Join(l.root, name) }
