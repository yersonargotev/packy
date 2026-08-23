package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue460ProjectRuntimeUsesOneVocabularyAcrossOpenCodeAndClaude(t *testing.T) {
	for _, surface := range []capabilitypack.Surface{capabilitypack.SurfaceOpenCode, capabilitypack.SurfaceClaude} {
		t.Run(string(surface), func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			pack := testsupport.CapabilityRich("project-runtime-" + string(surface))
			fixture := newSyntheticCLIFixture(t, terminal, pack)
			opts, home := fixture.options, fixture.home
			packID := pack.Manifest().ID
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts.Getwd = func() (string, error) { return project, nil }

			if out, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", string(surface), "--resource", "skill:helper"); err != nil {
				t.Fatalf("install: %v\n%s", err, out)
			}
			installation, err := capabilitypack.LoadProjectInstallation(project)
			if err != nil {
				t.Fatal(err)
			}
			if len(installation.Lock.ResourceGraph.Resources) == 0 {
				t.Fatal("installed project lock has no selected resources")
			}
			resource := installation.Lock.ResourceGraph.Resources[0].Resource
			installation.Lock.Receipts[0].Sensitive = []capabilitypack.ProjectSensitiveDisclosure{
				{Category: capabilitypack.ProjectActivationMCP, Surface: surface, Resource: resource, Detail: "host-owned-runtime-consent"},
			}
			lock, err := json.MarshalIndent(installation.Lock, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), append(lock, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}

			human, err := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", string(surface), "--project", "--dry-run")
			if err != nil || !strings.Contains(human, "Runtime activation: previewable") || !strings.Contains(human, "Approval category: mcp") {
				t.Fatalf("human activation preview: %v\n%s", err, human)
			}
			structured, err := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", string(surface), "--project", "--dry-run", "--json")
			if err != nil {
				t.Fatalf("structured activation preview: %v\n%s", err, structured)
			}
			var preview capabilitypack.JSONProjectActivationPreview
			if err := json.Unmarshal([]byte(structured), &preview); err != nil || preview.Surface != surface || preview.Disposition != capabilitypack.ProjectActivationPreviewable || len(preview.Categories) != 1 || preview.Categories[0].Kind != capabilitypack.ProjectActivationMCP || preview.ExpectedReadiness.Configured != capabilitypack.ReadinessTrue || len(preview.Conditions) == 0 {
				t.Fatalf("structured preview = %+v, %v", preview, err)
			}

			if out, err := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", string(surface), "--project"); err != nil || !strings.Contains(out, "Verified personal project activation") {
				t.Fatalf("activate: %v\n%s", err, out)
			}
			state := "state-" + packID + "-" + string(surface) + ".json"
			matches, err := filepath.Glob(filepath.Join(home, ".packy", "projects", "*", state))
			if err != nil || len(matches) != 1 {
				t.Fatalf("surface-scoped personal state %s = %v, %v", state, matches, err)
			}
			status, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", string(surface), "--project")
			if err != nil || !strings.Contains(status, "Installation: installed") || !strings.Contains(status, "Runtime activation: active") || !strings.Contains(status, "Pending human actions:") || !strings.Contains(status, "Evidence:") {
				t.Fatalf("project status: %v\n%s", err, status)
			}
			statusJSON, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", string(surface), "--project", "--json")
			var report capabilitypack.JSONProjectStatusReport
			if err != nil || json.Unmarshal([]byte(statusJSON), &report) != nil || len(report.Packs) != 1 || len(report.Packs[0].PendingHumanActions) == 0 || len(report.Packs[0].Evidence) == 0 {
				t.Fatalf("structured project status = %+v, err=%v, output=%s", report, err, statusJSON)
			}
		})
	}
}
