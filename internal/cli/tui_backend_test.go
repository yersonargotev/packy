package cli

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/yersonargotev/packy/internal/tui"
)

func TestTUIProductionBackendUsesPackyOwnersWithoutMutatingState(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name             string
		currentDirectory string
		projectAvailable bool
	}{
		{name: "Git project", currentDirectory: repositoryRoot, projectAvailable: true},
		{name: "outside Git", currentDirectory: t.TempDir(), projectAvailable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			opts := Options{
				Env: MapEnv{
					"HOME":                home,
					"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
					"PATH":                "",
					"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
				},
				Getwd:  func() (string, error) { return test.currentDirectory, nil },
				Runner: &fakeRunner{},
			}
			opts = opts.withDefaults()
			before := snapshotTree(t, home)

			dashboard, err := newTUIBackend(opts, newWorkstationResolver(opts)).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if dashboard.Health.Status != "healthy" || len(dashboard.Global.Packs) == 0 {
				t.Fatalf("dashboard omitted owner-derived global state: %#v", dashboard)
			}
			orchestrate := findTUIPack(dashboard.Global.Packs, "orchestrate")
			if orchestrate == nil {
				t.Fatalf("dynamic catalog omitted selectable orchestrate Pack: %#v", dashboard.Global.Packs)
			}
			if orchestrate.Description == "" || len(orchestrate.Resources) == 0 {
				t.Fatalf("catalog detail omitted manifest-owned description or resources: %#v", orchestrate)
			}
			if len(orchestrate.SurfaceStatuses) != 3 {
				t.Fatalf("surface matrix has %d entries, want all 3 known CLI surfaces: %#v", len(orchestrate.SurfaceStatuses), orchestrate.SurfaceStatuses)
			}
			for _, unsupported := range []string{"claude", "opencode"} {
				index := slices.IndexFunc(orchestrate.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == unsupported })
				if index < 0 || orchestrate.SurfaceStatuses[index].Supported {
					t.Fatalf("orchestrate did not identify %s as unsupported before lifecycle action: %#v", unsupported, orchestrate.SurfaceStatuses)
				}
			}
			codexIndex := slices.IndexFunc(orchestrate.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
			if codexIndex < 0 || !orchestrate.SurfaceStatuses[codexIndex].Supported || orchestrate.SurfaceStatuses[codexIndex].Configured == "" {
				t.Fatalf("orchestrate omitted supported Codex readiness status: %#v", orchestrate.SurfaceStatuses)
			}
			if dashboard.Project.Available != test.projectAvailable {
				t.Fatalf("project availability = %v, want %v", dashboard.Project.Available, test.projectAvailable)
			}
			if test.projectAvailable && dashboard.Project.Root != repositoryRoot {
				t.Fatalf("project root = %q, want %q", dashboard.Project.Root, repositoryRoot)
			}
			if test.projectAvailable && len(dashboard.Project.Packs) != len(dashboard.Global.Packs) {
				t.Fatalf("project catalog has %d selectable Packs, global has %d", len(dashboard.Project.Packs), len(dashboard.Global.Packs))
			}
			if after := snapshotTree(t, home); after != before {
				t.Fatalf("read-only backend mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

func findTUIPack(packs []tui.Pack, id string) *tui.Pack {
	for _, pack := range packs {
		if pack.ID == id {
			return &pack
		}
	}
	return nil
}
