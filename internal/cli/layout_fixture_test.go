package cli

import (
	"testing"

	"github.com/yersonargotev/packy/internal/agents"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/skilllayout"
	"github.com/yersonargotev/packy/internal/workstation"
)

// cliTestFixture gathers owner-derived layout values for CLI integration tests.
// It deliberately contains no path derivation of its own.
type cliTestFixture struct {
	workstation workstation.Snapshot
	packState   capabilitypack.StateLayout
	skills      skilllayout.GlobalLayout
	agents      agents.CanonicalLayout
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
		skills:      skilllayout.NewGlobalLayout(snapshot.Home()),
		agents:      agents.NewCanonicalLayout(snapshot.Home()),
	}
}
