package cli

import (
	"context"
	"path/filepath"
	"testing"
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
			if dashboard.Project.Available != test.projectAvailable {
				t.Fatalf("project availability = %v, want %v", dashboard.Project.Available, test.projectAvailable)
			}
			if test.projectAvailable && dashboard.Project.Root != repositoryRoot {
				t.Fatalf("project root = %q, want %q", dashboard.Project.Root, repositoryRoot)
			}
			if after := snapshotTree(t, home); after != before {
				t.Fatalf("read-only backend mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}
