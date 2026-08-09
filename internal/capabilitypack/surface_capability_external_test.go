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
	"github.com/yersonargotev/packy/internal/opencode"
)

func TestProjectInstructionCapabilityIsPackIdentityIndependent(t *testing.T) {
	for _, surface := range []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode} {
		t.Run(string(surface), func(t *testing.T) {
			bundle := t.TempDir()
			for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
				writeProjectInstructionPack(t, bundle, packID, surface)
			}
			catalog, err := capabilitypack.DiscoverForDurableIntents(bundle)
			if err != nil {
				t.Fatal(err)
			}
			facade := capabilitypack.NewFacade(catalog)
			project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
			if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Foreign guidance\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			adapter := projectInstructionAdapter(t, bundle, surface)

			for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
				preview, err := facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{PackID: packID, Surface: surface, ProjectRoot: project}, adapter)
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
			}

			assertProjectInstructionContributions(t, project, true, true)
			agentsPath := filepath.Join(project, "AGENTS.md")
			installedDocument, err := os.ReadFile(agentsPath)
			if err != nil {
				t.Fatal(err)
			}
			drifted := strings.Replace(string(installedDocument), "Shared project guidance from synthetic-alpha.", "Locally changed alpha guidance.", 1)
			if err := os.WriteFile(agentsPath, []byte(drifted), 0o640); err != nil {
				t.Fatal(err)
			}
			blocked, err := capabilitypack.PreviewProjectUninstall(context.Background(), capabilitypack.ProjectUninstallRequest{PackID: "synthetic-alpha", Surface: surface, ProjectRoot: project}, adapter)
			if err != nil {
				t.Fatal(err)
			}
			if blocked.Disposition != capabilitypack.ProjectInstallBlocked || len(blocked.Blockers) == 0 {
				t.Fatalf("drifted removal = %#v", blocked)
			}
			if err := os.WriteFile(agentsPath, installedDocument, 0o640); err != nil {
				t.Fatal(err)
			}
			uninstallProjectInstructionPack(t, project, packyHome, "synthetic-alpha", surface, adapter)
			assertProjectInstructionContributions(t, project, false, true)
			uninstallProjectInstructionPack(t, project, packyHome, "synthetic-beta", surface, adapter)
			assertProjectInstructionContributions(t, project, false, false)
		})
	}
}

func writeProjectInstructionPack(t *testing.T, bundle, packID string, surface capabilitypack.Surface) {
	t.Helper()
	skill := filepath.Join(bundle, "skills", packID)
	if err := os.MkdirAll(skill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# "+packID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	instruction := filepath.Join(bundle, "instructions", packID+".md")
	if err := os.MkdirAll(filepath.Dir(instruction), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instruction, []byte("Shared project guidance from "+packID+".\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	packDir := filepath.Join(bundle, "packs", packID)
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{
  "id": %q,
  "version": "1.0.0",
  "description": "Synthetic capability tracer",
  "selectable": true,
  "surfaces": [%q],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
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
  }],
  "exclusions": []
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
	case capabilitypack.SurfaceOpenCode:
		return opencode.NewSurfaceAdapter(bundle, filepath.Join(t.TempDir(), "skills"), filepath.Join(t.TempDir(), "opencode.json"), filepath.Join(t.TempDir(), "packy.md"))
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

func assertProjectInstructionContributions(t *testing.T, project string, alpha, beta bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# Foreign guidance") {
		t.Fatalf("foreign guidance was removed: %q", content)
	}
	for id, want := range map[string]bool{"synthetic-alpha-guidance": alpha, "synthetic-beta-guidance": beta} {
		marker := "<!-- packy:project:instruction:" + id + ":start -->"
		if strings.Contains(content, marker) != want {
			t.Fatalf("marker %s presence = %t, want %t: %q", id, strings.Contains(content, marker), want, content)
		}
	}
}
