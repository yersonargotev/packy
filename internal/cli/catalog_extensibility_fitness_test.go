package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestUnrelatedSyntheticPacksPreserveReadinessAcrossSurfacesScopesAndPresentations(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	cases := []struct {
		pack                testsupport.Fixture
		surface             string
		globalAuthorization string
		pendingActions      int
		projectScope        bool
	}{
		{pack: testsupport.PortableAllSurfaces("fitness-cedar"), surface: "codex", globalAuthorization: "unknown", projectScope: true},
		{pack: testsupport.PortableAllSurfaces("fitness-orbit"), surface: "opencode", globalAuthorization: "unknown", projectScope: true},
		{pack: testsupport.PortableAllSurfaces("fitness-sable"), surface: "claude", globalAuthorization: "unknown", pendingActions: 3},
	}
	packs := make([]testsupport.Fixture, 0, len(cases))
	for _, test := range cases {
		packs = append(packs, test.pack)
	}
	fixture := newSyntheticCLIFixture(t, terminal, packs...)
	opts := fixture.options

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		manifest := test.pack.Manifest()
		activateArgs := []string{"activate", manifest.ID, "--surface", test.surface}
		preview, err := executeCommand(t, NewRootCommand(opts), append(activateArgs, "--dry-run", "--json")...)
		if err != nil {
			t.Fatalf("%s/%s preview: %v\n%s", manifest.ID, test.surface, err, preview)
		}
		if !json.Valid([]byte(preview)) {
			t.Fatalf("%s/%s lifecycle preview is not JSON: %s", manifest.ID, test.surface, preview)
		}
		assertStructuredOutput(t, root, "pack-lifecycle.schema.json", preview)
		for _, fact := range []string{`"configured":"true"`, `"authorized":"` + test.globalAuthorization + `"`, `"usable":"unknown"`, `"surface-authorization"`, `"runtime-usability"`} {
			if !strings.Contains(preview, fact) {
				t.Fatalf("%s/%s preview omitted %s:\n%s", manifest.ID, test.surface, fact, preview)
			}
		}
		if out, err := executeCommand(t, NewRootCommand(opts), activateArgs...); err != nil || !strings.Contains(out, "Apply result facts: verified=yes") {
			t.Fatalf("%s/%s activation: %v\n%s", manifest.ID, test.surface, err, out)
		}
		status, err := executeCommand(t, NewRootCommand(opts), "status", manifest.ID, "--surface", test.surface, "--json")
		if err != nil {
			t.Fatalf("%s/%s status: %v\n%s", manifest.ID, test.surface, err, status)
		}
		assertStructuredOutput(t, root, "pack-status.schema.json", status)
		for _, fact := range []string{`"configured":"true"`, `"authorized":"` + test.globalAuthorization + `"`, `"usable":"unknown"`, `"type":"surface-authorization"`, `"type":"runtime-usability"`} {
			if !strings.Contains(status, fact) {
				t.Fatalf("%s/%s status omitted %s:\n%s", manifest.ID, test.surface, fact, status)
			}
		}
		strict, strictErr := executeCommand(t, NewRootCommand(opts), "status", manifest.ID, "--surface", test.surface, "--require", "usable")
		if strictErr == nil || !strings.Contains(strict, "usable=unknown") {
			t.Fatalf("%s/%s strict usability did not fail closed: %v\n%s", manifest.ID, test.surface, strictErr, strict)
		}
	}

	doctor, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, doctor)
	}
	assertStructuredOutput(t, root, "doctor.schema.json", doctor)
	if !strings.Contains(doctor, `"status":"warnings"`) || strings.Count(doctor, `"severity":"INFO"`) < 3 || strings.Count(doctor, `"severity":"WARN"`) != 1 || !strings.Contains(doctor, "3 pending human actions") {
		t.Fatalf("Doctor did not preserve informational unknown conditions and actionable Claude work:\n%s", doctor)
	}

	for _, test := range cases {
		if !test.projectScope {
			continue
		}
		project := t.TempDir()
		writeTestGitWorktree(t, project)
		opts.Getwd = func() (string, error) { return project, nil }
		manifest := test.pack.Manifest()
		installArgs := []string{"install", manifest.ID, "--surface", test.surface}
		installed, err := executeCommand(t, NewRootCommand(opts), append(installArgs, "--json")...)
		if err != nil {
			t.Fatalf("%s/%s project install: %v\n%s", manifest.ID, test.surface, err, installed)
		}
		projectDocuments := strings.Split(strings.TrimSpace(installed), "\n")
		if len(projectDocuments) != 2 {
			t.Fatalf("%s/%s project install JSON documents = %d\n%s", manifest.ID, test.surface, len(projectDocuments), installed)
		}
		assertProjectStructuredOutput(t, root, "project-preview.schema.json", projectDocuments[0])
		assertProjectStructuredOutput(t, root, "project-apply.schema.json", projectDocuments[1])
		status, err := executeCommand(t, NewRootCommand(opts), "status", manifest.ID, "--surface", test.surface, "--project", "--json")
		if err != nil || !strings.Contains(status, `"configured":"true"`) || !strings.Contains(status, `"authorized":"unknown"`) || !strings.Contains(status, `"usable":"unknown"`) {
			t.Fatalf("%s/%s project status: %v\n%s", manifest.ID, test.surface, err, status)
		}
		assertProjectStructuredOutput(t, root, "project-status.schema.json", status)
	}

	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		manifest := test.pack.Manifest()
		pack := findTUIPack(dashboard.Global.Packs, manifest.ID)
		if pack == nil {
			t.Fatalf("TUI omitted synthetic Pack %s: %#v", manifest.ID, dashboard.Global.Packs)
		}
		index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == test.surface })
		if index < 0 || pack.SurfaceStatuses[index].Configured != "true" || pack.SurfaceStatuses[index].Authorized != test.globalAuthorization || pack.SurfaceStatuses[index].Usable != "unknown" || len(pack.SurfaceStatuses[index].Conditions) < 3 || len(pack.SurfaceStatuses[index].PendingActions) != test.pendingActions {
			t.Fatalf("TUI %s/%s readiness/actions = %#v", manifest.ID, test.surface, pack.SurfaceStatuses)
		}
		preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "deactivate", PackID: manifest.ID, Surface: test.surface, Scope: "global"})
		if err != nil || preview.Operation != "deactivate" || len(preview.Phases) == 0 || len(preview.Phases[0].Actions) == 0 {
			t.Fatalf("TUI %s/%s lifecycle preview = %#v, err=%v", manifest.ID, test.surface, preview, err)
		}
	}
}
