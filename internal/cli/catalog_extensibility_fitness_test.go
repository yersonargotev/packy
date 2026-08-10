package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/tui"
)

func TestRenamedCatalogPackPreservesReadinessAcrossSurfacesScopesAndPresentations(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, repoRoot := packActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	writeCatalogFitnessPacks(t, bundle)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	globalAuthorization := map[string]string{"codex": "true", "opencode": "true", "claude": "unknown"}
	for _, surface := range []string{"codex", "opencode", "claude"} {
		packID := "renamed-" + surface
		preview, err := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", surface, "--dry-run", "--json")
		if err != nil {
			t.Fatalf("%s preview: %v\n%s", surface, err, preview)
		}
		if !json.Valid([]byte(preview)) {
			t.Fatalf("%s lifecycle preview is not JSON: %s", surface, preview)
		}
		assertStructuredOutput(t, root, "pack-lifecycle.schema.json", preview)
		for _, fact := range []string{`"configured":"true"`, `"authorized":"` + globalAuthorization[surface] + `"`, `"usable":"unknown"`, `"surface-authorization"`, `"runtime-usability"`} {
			if !strings.Contains(preview, fact) {
				t.Fatalf("%s preview omitted %s:\n%s", surface, fact, preview)
			}
		}
		if out, err := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", surface); err != nil || !strings.Contains(out, "Apply result facts: verified=yes") {
			t.Fatalf("%s activation: %v\n%s", surface, err, out)
		}
		status, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", surface, "--json")
		if err != nil {
			t.Fatalf("%s status: %v\n%s", surface, err, status)
		}
		assertStructuredOutput(t, root, "pack-status.schema.json", status)
		for _, fact := range []string{`"configured":"true"`, `"authorized":"` + globalAuthorization[surface] + `"`, `"usable":"unknown"`, `"type":"surface-authorization"`, `"type":"runtime-usability"`} {
			if !strings.Contains(status, fact) {
				t.Fatalf("%s status omitted %s:\n%s", surface, fact, status)
			}
		}
		strict, strictErr := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", surface, "--require", "usable")
		if strictErr == nil || !strings.Contains(strict, "usable=unknown") {
			t.Fatalf("%s strict usability did not fail closed: %v\n%s", surface, strictErr, strict)
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

	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	for _, surface := range []string{"claude", "codex", "opencode"} {
		packID := "renamed-" + surface
		installed, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", surface, "--json")
		if err != nil {
			t.Fatalf("%s project install: %v\n%s", surface, err, installed)
		}
		projectDocuments := strings.Split(strings.TrimSpace(installed), "\n")
		if len(projectDocuments) != 2 {
			t.Fatalf("%s project install JSON documents = %d\n%s", surface, len(projectDocuments), installed)
		}
		assertProjectStructuredOutput(t, root, "project-preview.schema.json", projectDocuments[0])
		assertProjectStructuredOutput(t, root, "project-apply.schema.json", projectDocuments[1])
		status, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", surface, "--project", "--json")
		if err != nil || !strings.Contains(status, `"configured":"true"`) || !strings.Contains(status, `"authorized":"unknown"`) || !strings.Contains(status, `"usable":"unknown"`) {
			t.Fatalf("%s project status: %v\n%s", surface, err, status)
		}
		assertProjectStructuredOutput(t, root, "project-status.schema.json", status)
	}

	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"claude", "codex", "opencode"} {
		pack := findTUIPack(dashboard.Global.Packs, "renamed-"+surface)
		if pack == nil {
			t.Fatalf("TUI omitted renamed %s Pack: %#v", surface, dashboard.Global.Packs)
		}
		index := slices.IndexFunc(pack.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == surface })
		wantPending := 0
		if surface == "claude" {
			wantPending = 3
		}
		if index < 0 || pack.SurfaceStatuses[index].Configured != "true" || pack.SurfaceStatuses[index].Authorized != globalAuthorization[surface] || pack.SurfaceStatuses[index].Usable != "unknown" || len(pack.SurfaceStatuses[index].Conditions) < 3 || len(pack.SurfaceStatuses[index].PendingActions) != wantPending {
			t.Fatalf("TUI %s readiness/actions = %#v", surface, pack.SurfaceStatuses)
		}
		preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "deactivate", PackID: "renamed-" + surface, Surface: surface, Scope: "global"})
		if err != nil || preview.Operation != "deactivate" || len(preview.Phases) == 0 || len(preview.Phases[0].Actions) == 0 {
			t.Fatalf("TUI %s lifecycle preview = %#v, err=%v", surface, preview, err)
		}
	}
}

func writeCatalogFitnessPacks(t *testing.T, bundle string) {
	t.Helper()
	for _, surface := range []string{"claude", "codex", "opencode"} {
		packID, resourceID := "renamed-"+surface, "renamed-"+surface+"-guide"
		skill := filepath.Join(bundle, "skills", resourceID)
		if err := os.MkdirAll(skill, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# "+resourceID+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		pack := filepath.Join(bundle, "packs", packID)
		if err := os.MkdirAll(pack, 0o700); err != nil {
			t.Fatal(err)
		}
		manifest := fmt.Sprintf(`{
  "id": %q,
  "version": "1.0.0",
  "description": "Synthetic catalog fitness Pack",
  "selectable": true,
	  "surfaces": [%q],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "resources": [{
	    "kind": "skill", "id": %q, "source": %q, "description": "Synthetic renamed guide",
    "requires": [], "conflicts": [],
	    "bindings": [{"surface":%q,"projection":"skill","name":%q,"invocation":%q,"mode":"native","sharing":"exclusive","capabilities":[]}],
    "surface_exclusions": []
  }],
  "exclusions": []
}
	`, packID, surface, resourceID, "skills/"+resourceID, surface, resourceID, resourceID)
		if err := os.WriteFile(filepath.Join(pack, "pack.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
