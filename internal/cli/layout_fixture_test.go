package cli

import (
	"testing"

	"github.com/yersonargotev/matty/internal/bootstrap"
	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/codex"
	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/skillbundle"
	"github.com/yersonargotev/matty/internal/workstation"
)

// cliTestFixture gathers owner-derived layout values for CLI integration tests.
// It deliberately contains no path derivation of its own.
type cliTestFixture struct {
	workstation     workstation.Snapshot
	installedSource bootstrap.InstalledSource
	skillSource     skillbundle.Source
	classicState    corelifecycle.Layout
	packState       capabilitypack.StateLayout
	skills          skillbundle.GlobalLayout
	codex           codex.CanonicalLayout
	opencode        opencode.CanonicalLayout
	engram          engrambin.Topology
	engramSetup     engrambin.SetupLayout
}

func newCLITestFixture(t *testing.T, opts Options) cliTestFixture {
	t.Helper()
	opts = opts.withDefaults()
	currentDirectory, currentDirectoryErr := opts.Getwd()
	snapshot, err := workstation.Resolve(workstation.Inputs{
		Home:                 opts.Env.Getenv("HOME"),
		ConfigurationHome:    opts.Env.Getenv("XDG_CONFIG_HOME"),
		ExecutableSearchPath: opts.Env.Getenv("PATH"),
		HomebrewPrefix:       opts.Env.Getenv("HOMEBREW_PREFIX"),
		CurrentDirectory:     currentDirectory,
		CurrentDirectoryErr:  currentDirectoryErr,
	}, workstation.Options{})
	if err != nil {
		t.Fatalf("resolve workstation fixture: %v", err)
	}
	installedSource, err := bootstrap.ResolveInstalledSource(snapshot, "")
	if err != nil {
		t.Fatalf("resolve Installed Source fixture: %v", err)
	}
	skillSource, err := skillbundle.ResolveSource(skillbundle.SourceOptions{
		ExplicitRoot:    opts.Env.Getenv("MATTY_SKILLS_SOURCE"),
		RepositoryStart: currentDirectory,
		InstalledRoot:   installedSource.Root(),
	})
	if err != nil {
		t.Fatalf("resolve Skill Source fixture: %v", err)
	}

	return cliTestFixture{
		workstation:     snapshot,
		installedSource: installedSource,
		skillSource:     skillSource,
		classicState:    corelifecycle.NewLayout(snapshot.MattyHome()),
		packState:       capabilitypack.NewStateLayout(snapshot.MattyHome()),
		skills:          skillbundle.NewGlobalLayout(snapshot.Home()),
		codex:           codex.NewCanonicalLayout(snapshot.Home()),
		opencode:        opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
		engram:          engrambin.NewTopology(snapshot.HomebrewPrefix()),
		engramSetup:     engrambin.NewSetupLayout(snapshot.Home(), snapshot.HomebrewPrefix()),
	}
}
