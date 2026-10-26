package capabilitypack_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
)

func TestProjectInstructionCapabilityIsPackIdentityIndependent(t *testing.T) {
	for _, surface := range []capabilitypack.Surface{capabilitypack.SurfaceCodex} {
		t.Run(string(surface), func(t *testing.T) {
			bundle := t.TempDir()
			for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
				writeProjectInstructionPack(t, bundle, packID, surface)
			}
			catalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), bundle)
			if err != nil {
				t.Fatal(err)
			}
			for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
				facade := capabilitypack.NewFacade(catalog)
				project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
				if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Foreign guidance\n"), 0o640); err != nil {
					t.Fatal(err)
				}
				adapter := projectInstructionAdapter(t, bundle, surface)
				preview, err := facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{PackID: packID, Surface: surface, ProjectRoot: project, PackyHome: packyHome}, adapter)
				if err != nil {
					t.Fatal(err)
				}
				if preview.Disposition != capabilitypack.ProjectInstallPreviewable {
					t.Fatalf("%s install disposition = %s, blockers = %#v", packID, preview.Disposition, preview.Blockers)
				}
				if _, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: preview, PackyHome: packyHome, Adapter: adapter}); err != nil {
					t.Fatal(err)
				}
				status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: project, PackID: packID, Surface: surface, RequireInstalled: true, PackyHome: packyHome, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{surface: adapter}})
				if err != nil {
					t.Fatal(err)
				}
				if len(status.Packs) != 1 || status.Packs[0].Installation != capabilitypack.ProjectInstallationInstalled {
					t.Fatalf("%s status = %#v", packID, status)
				}
				for _, projection := range status.Packs[0].Projections {
					if projection.Health != "verified" {
						t.Fatalf("%s projection status = %#v", packID, projection)
					}
				}

				assertProjectInstructionContribution(t, project, packID, true)
				updatedBundle := t.TempDir()
				if err := os.CopyFS(updatedBundle, os.DirFS(bundle)); err != nil {
					t.Fatal(err)
				}
				manifestPath := filepath.Join(updatedBundle, "packs", packID, "pack.json")
				manifest, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				advanced := strings.Replace(string(manifest), `"version": "1.0.0"`, `"version": "1.0.1"`, 1)
				if advanced == string(manifest) {
					t.Fatal("synthetic project-instruction version did not advance")
				}
				if err := os.WriteFile(manifestPath, []byte(advanced), 0o600); err != nil {
					t.Fatal(err)
				}
				currentGuidance := "Updated shared project guidance from " + packID + "."
				if err := os.WriteFile(filepath.Join(updatedBundle, "packs", packID, "instructions", packID+".md"), []byte(currentGuidance+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				updatedCatalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), updatedBundle)
				if err != nil {
					t.Fatal(err)
				}
				adapter = projectInstructionAdapter(t, updatedBundle, surface)
				facade = capabilitypack.NewFacade(updatedCatalog)
				update, err := facade.PreviewProjectUpdate(context.Background(), capabilitypack.ProjectUpdateRequest{PackID: packID, Surface: surface, ProjectRoot: project, PackyHome: packyHome}, adapter)
				if err != nil || update.Disposition != capabilitypack.ProjectInstallPreviewable {
					t.Fatalf("%s update = %#v, err=%v", packID, update, err)
				}
				if _, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: update, PackyHome: packyHome, Adapter: adapter}); err != nil {
					t.Fatal(err)
				}
				updatedDocument, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
				if err != nil || !strings.Contains(string(updatedDocument), currentGuidance) {
					t.Fatalf("%s updated project instruction = %q, err=%v", packID, updatedDocument, err)
				}

				agentsPath := filepath.Join(project, "AGENTS.md")
				installedDocument, err := os.ReadFile(agentsPath)
				if err != nil {
					t.Fatal(err)
				}
				drifted := strings.Replace(string(installedDocument), currentGuidance, "Locally changed guidance.", 1)
				if err := os.WriteFile(agentsPath, []byte(drifted), 0o640); err != nil {
					t.Fatal(err)
				}
				blocked, err := capabilitypack.PreviewProjectUninstall(context.Background(), capabilitypack.ProjectUninstallRequest{PackID: packID, Surface: surface, ProjectRoot: project}, adapter)
				if err != nil {
					t.Fatal(err)
				}
				if blocked.Disposition != capabilitypack.ProjectInstallBlocked || len(blocked.Blockers) == 0 {
					t.Fatalf("drifted removal = %#v", blocked)
				}
				if err := os.WriteFile(agentsPath, installedDocument, 0o640); err != nil {
					t.Fatal(err)
				}
				uninstallProjectInstructionPack(t, project, packyHome, packID, surface, adapter)
				assertProjectInstructionContribution(t, project, packID, false)
			}
		})
	}
}

type memoryActivationStore struct {
	state    capabilitypack.ActivationState
	revision int
}

func (s *memoryActivationStore) LoadSnapshot(context.Context, capabilitypack.Surface) (capabilitypack.ActivationState, error) {
	return s.state, nil
}

func (s *memoryActivationStore) SaveSnapshot(_ context.Context, _ capabilitypack.Surface, _ int, state capabilitypack.ActivationState) (int, error) {
	s.revision++
	s.state = state
	return s.revision, nil
}

func approvalsFor(t *testing.T, facade capabilitypack.Facade, plan capabilitypack.ReconciliationPlan) []capabilitypack.ApprovalReceipt {
	t.Helper()
	var approvals []capabilitypack.ApprovalReceipt
	for _, phase := range plan.Phases() {
		if phase.ApprovalRequired {
			approvals = append(approvals, facade.Approve(plan, phase.Kind))
		}
	}
	return approvals
}

func writeProjectInstructionPack(t *testing.T, bundle, packID string, surface capabilitypack.Surface) {
	t.Helper()
	packDir := filepath.Join(bundle, "packs", packID)
	skill := filepath.Join(packDir, "skills", packID)
	if err := os.MkdirAll(skill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# "+packID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	instruction := filepath.Join(packDir, "instructions", packID+".md")
	if err := os.MkdirAll(filepath.Dir(instruction), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instruction, []byte("Shared project guidance from "+packID+".\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{
  "schema_version": 3,
  "id": %q,
  "version": "1.0.0",
  "description": "Synthetic capability tracer",
  "selectable": true,
  "surfaces": [%q],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [{
    "kind": "skill",
    "id": %q,
    "source": %q,
    "description": "Synthetic skill",
    "requires": [],
    "conflicts": [],
    "bindings": [{
      "surface": %q,
      "projection": "skill",
      "name": %q,
      "invocation": %q,
      "mode": "native",
      "sharing": "exclusive",
      "capabilities": [{
        "type": "project-instruction",
        "project_instruction": {"id": %q, "source": %q}
      }]
    }],
    "surface_exclusions": []
  }]
}
`, packID, surface, packID+"-skill", "skills/"+packID, surface, packID+"-skill", packID+"-skill", packID+"-guidance", "instructions/"+packID+".md")
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

func projectInstructionAdapter(t *testing.T, bundle string, surface capabilitypack.Surface) capabilitypack.SurfaceAdapter {
	t.Helper()
	switch surface {
	case capabilitypack.SurfaceCodex:
		return codex.NewSurfaceAdapterWithConfig(bundle, filepath.Join(t.TempDir(), "skills"), filepath.Join(t.TempDir(), "AGENTS.md"), filepath.Join(t.TempDir(), "config.toml"))
	default:
		t.Fatalf("unsupported test surface %s", surface)
		return nil
	}
}

func uninstallProjectInstructionPack(t *testing.T, project, packyHome, packID string, surface capabilitypack.Surface, adapter capabilitypack.SurfaceAdapter) {
	t.Helper()
	preview, err := capabilitypack.PreviewProjectUninstall(context.Background(), capabilitypack.ProjectUninstallRequest{PackID: packID, Surface: surface, ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition != capabilitypack.ProjectInstallPreviewable {
		t.Fatalf("%s uninstall disposition = %s, blockers = %#v", packID, preview.Disposition, preview.Blockers)
	}
	if _, err := capabilitypack.ApplyProjectUninstall(context.Background(), capabilitypack.ProjectUninstallApplyRequest{Preview: preview, PackyHome: packyHome, Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
}

func assertProjectInstructionContribution(t *testing.T, project, packID string, want bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# Foreign guidance") {
		t.Fatalf("foreign guidance was removed: %q", content)
	}
	id := packID + "-guidance"
	marker := "<!-- packy:project:instruction:" + id + ":start -->"
	if strings.Contains(content, marker) != want {
		t.Fatalf("marker %s presence = %t, want %t: %q", id, strings.Contains(content, marker), want, content)
	}
}
