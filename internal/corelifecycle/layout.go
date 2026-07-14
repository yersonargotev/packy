package corelifecycle

import "path/filepath"

// Layout is the classic lifecycle-owned state beneath Matty Home.
type Layout struct {
	mattyHome string
	stateFile string
}

func NewLayout(mattyHome string) Layout {
	return Layout{mattyHome: mattyHome, stateFile: filepath.Join(mattyHome, "config.json")}
}

func (l Layout) MattyHome() string { return l.mattyHome }
func (l Layout) StateFile() string { return l.stateFile }
