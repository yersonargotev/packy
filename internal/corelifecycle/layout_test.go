package corelifecycle

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/bootstrap"
	"github.com/yersonargotev/matty/internal/codex"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/skillbundle"
)

func TestClassicLayoutDerivesStateFromMattyHome(t *testing.T) {
	mattyHome := filepath.Join(t.TempDir(), ".matty")
	layout := NewLayout(mattyHome)

	if layout.MattyHome() != mattyHome {
		t.Fatalf("MattyHome = %q, want %q", layout.MattyHome(), mattyHome)
	}
	if layout.StateFile() != filepath.Join(mattyHome, "config.json") {
		t.Fatalf("StateFile = %q", layout.StateFile())
	}
}

func TestFacadeConfigDerivesInternalPathsFromOwnerValues(t *testing.T) {
	home := t.TempDir()
	mattyHome := filepath.Join(home, ".matty")
	source := skillbundle.Source{Root: filepath.Join(t.TempDir(), "bundle", "skills")}
	installed := bootstrap.InstalledSourceAt(filepath.Join(home, ".local", "share", "matty"))
	facade := NewFacade(FacadeConfig{
		MattyHome:       mattyHome,
		Skills:          skillbundle.NewGlobalLayout(home),
		SkillSource:     source,
		Codex:           codex.NewCanonicalLayout(home),
		OpenCode:        opencode.NewCanonicalLayout(filepath.Join(home, ".config")),
		Engram:          engrambin.NewTopology(filepath.Join(home, "homebrew")),
		InstalledSource: installed,
		RunningVersion:  "v1.2.3",
	}, &installTestCommands{}, time.Now)

	if facade.config.State.StateFile() != filepath.Join(mattyHome, "config.json") {
		t.Fatalf("StateFile = %q", facade.config.State.StateFile())
	}
	if facade.config.Skills.Root() != filepath.Join(home, ".agents", "skills") {
		t.Fatalf("AgentSkillsDir = %q", facade.config.Skills.Root())
	}
	if facade.config.InstalledSource.Root() != installed.Root() {
		t.Fatalf("InstalledSource = %q", facade.config.InstalledSource.Root())
	}
}
