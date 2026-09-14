package cli

import (
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/skillbundle"
	"github.com/yersonargotev/packy/internal/workstation"
)

// cliTestFixture gathers owner-derived layout values for CLI integration tests.
// It deliberately contains no path derivation of its own.
type cliTestFixture struct {
	workstation workstation.Snapshot
	packState   capabilitypack.StateLayout
	skills      skillbundle.GlobalLayout
	codex       codex.CanonicalLayout
	opencode    opencode.CanonicalLayout
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
	return cliTestFixture{
		workstation: snapshot,
		packState:   capabilitypack.NewStateLayout(snapshot.PackyHome()),
		skills:      skillbundle.NewGlobalLayout(snapshot.Home()),
		codex:       codex.NewCanonicalLayout(snapshot.Home()),
		opencode:    opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
	}
}
