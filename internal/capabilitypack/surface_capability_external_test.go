package capabilitypack_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/opencode"
)

func TestExternalHostSetupCapabilityIsPackAndResourceIdentityIndependent(t *testing.T) {
	for _, surface := range []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode} {
		t.Run(string(surface), func(t *testing.T) {
			bundle := t.TempDir()
			instructions := filepath.Join(bundle, "instructions")
			if err := os.MkdirAll(instructions, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"alpha-notes", "beta-notes"} {
				if err := os.WriteFile(filepath.Join(instructions, id+".md"), []byte("portable memory\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root := t.TempDir()
			var adapter capabilitypack.SurfaceAdapter
			switch surface {
			case capabilitypack.SurfaceCodex:
				adapter = codex.NewSurfaceAdapterWithConfig(bundle, filepath.Join(root, "skills"), filepath.Join(root, "AGENTS.md"), filepath.Join(root, "config.toml"))
			case capabilitypack.SurfaceOpenCode:
				adapter = opencode.NewSurfaceAdapter(bundle, filepath.Join(root, "skills"), filepath.Join(root, "opencode.json"), filepath.Join(root, "primary.md"))
			}
			var translated [][]capabilitypack.ObservedProjection
			for _, named := range []struct{ pack, instruction, mcp string }{{"synthetic-alpha", "alpha-notes", "alpha-memory"}, {"synthetic-beta", "beta-notes", "beta-memory"}} {
				pack := syntheticExternalHostSetupPack(named.pack, named.instruction, named.mcp, surface)
				observation, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ResolvedExecutables: []capabilitypack.ExecutableResolution{{Tool: "engram", Available: true, Path: "/opt/reviewed/engram"}}})
				if err != nil {
					t.Fatal(err)
				}
				var external []capabilitypack.ObservedProjection
				for _, projection := range observation.Projections {
					if projection.ExternallyManaged {
						external = append(external, projection)
					}
				}
				translated = append(translated, external)
			}
			if len(translated[0]) == 0 || !reflect.DeepEqual(translated[0], translated[1]) {
				t.Fatalf("%s host translation depends on Pack or resource identity:\nalpha=%#v\nbeta=%#v", surface, translated[0], translated[1])
			}
		})
	}
}

func syntheticExternalHostSetupPack(packID, instructionID, mcpID string, surface capabilitypack.Surface) capabilitypack.Pack {
	setup := &capabilitypack.ExternalHostSetupCapability{
		Tool: "engram", SetupArgs: []string{"setup", string(surface)},
		ManagedResources: []capabilitypack.ResourceIdentity{{Kind: "instruction", ID: instructionID}, {Kind: "mcp_server", ID: mcpID}},
	}
	if surface == capabilitypack.SurfaceCodex {
		setup.Codex = &capabilitypack.CodexHostSetup{
			MCPArgs: []string{"mcp", "--tools=agent"}, InstructionsFile: "engram-instructions.md", InstructionsFingerprint: "74176fb0847b06fb725ae8992c9a5fa12022ff347ca3ee2ef3e77c6d318d5fb3",
			CompactPromptFile: "engram-compact-prompt.md", CompactPromptFingerprint: "c779d9584c8ca16331ebb31a753f7fbb5bcb8193b229572a54da189ffaa97fd1",
			MarketplaceRepository: "https://github.com/Gentleman-Programming/engram.git", MarketplaceRevision: "main", Plugin: "engram@engram",
		}
	} else {
		setup.OpenCode = &capabilitypack.OpenCodeHostSetup{PluginFile: "plugins/engram.ts", TUIFile: "tui.json", TUIPlugin: "opencode-subagent-statusline"}
	}
	return capabilitypack.Pack{ID: packID, Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: instructionID, Source: "instructions/" + instructionID + ".md"},
		{Kind: "mcp_server", ID: mcpID, Command: "engram", Args: []string{"mcp", "--tools=agent"}, Bindings: []capabilitypack.Binding{{Surface: surface, Capabilities: []capabilitypack.SurfaceCapability{{Type: capabilitypack.SurfaceCapabilityExternalHostSetup, ExternalHostSetup: setup}}}}},
	}}
}

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
			for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
				facade := capabilitypack.NewFacade(catalog)
				project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
				currentGuidance := "Shared project guidance from " + packID + "."
				if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Foreign guidance\n"), 0o640); err != nil {
					t.Fatal(err)
				}
				adapter := projectInstructionAdapter(t, bundle, surface)
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
				currentGuidance = "Updated shared project guidance from " + packID + "."
				if err := os.WriteFile(filepath.Join(updatedBundle, "instructions", packID+".md"), []byte(currentGuidance+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				updatedCatalog, err := capabilitypack.DiscoverForDurableIntents(updatedBundle)
				if err != nil {
					t.Fatal(err)
				}
				adapter = projectInstructionAdapter(t, updatedBundle, surface)
				facade = capabilitypack.NewFacade(updatedCatalog)
				update, err := facade.PreviewProjectUpdate(context.Background(), capabilitypack.ProjectUpdateRequest{PackID: packID, Surface: surface, ProjectRoot: project}, adapter)
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

func TestOpenCodePrimaryPromptCapabilityIsPackIdentityIndependent(t *testing.T) {
	bundle := t.TempDir()
	for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
		writePrimaryPromptPack(t, bundle, packID)
	}
	catalog, err := capabilitypack.DiscoverForDurableIntents(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
		t.Run(packID, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "opencode.json")
			prompt := filepath.Join(root, "packy.md")
			if err := os.WriteFile(config, []byte("// operator setting\n{\n  \"model\": \"test/model\"\n}\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			adapter := opencode.NewSurfaceAdapter(bundle, filepath.Join(root, "skills"), config, prompt)
			store := &memoryActivationStore{}
			facade := capabilitypack.NewFacade(catalog, capabilitypack.WithActivation(store, map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceOpenCode: adapter}))

			plainOnly, err := facade.Preview(context.Background(), capabilitypack.ActivationRequest{PackID: packID, Surface: capabilitypack.SurfaceOpenCode, Selection: capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{{Kind: "skill", ID: "zeta-plain"}}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, phase := range plainOnly.Phases() {
				for _, action := range phase.Actions {
					if action.Target == prompt {
						t.Fatalf("unselected primary prompt owner produced %s", action.ID)
					}
				}
			}

			preview, err := facade.Preview(context.Background(), capabilitypack.ActivationRequest{PackID: packID, Surface: capabilitypack.SurfaceOpenCode, Selection: capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := facade.Apply(context.Background(), capabilitypack.ApplyRequest{Plan: preview, Approvals: approvalsFor(t, facade, preview), Interactive: true})
			if err != nil || !result.Verified {
				t.Fatalf("activate primary prompt: %#v, %v", result, err)
			}

			updatedBundle := t.TempDir()
			if err := os.CopyFS(updatedBundle, os.DirFS(bundle)); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(updatedBundle, "packs", packID, "pack.json")
			manifest, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			updatedManifest := strings.Replace(string(manifest), `"version": "1.0.0"`, `"version": "1.0.1"`, 1)
			if updatedManifest == string(manifest) {
				t.Fatal("synthetic primary prompt version did not advance")
			}
			if err := os.WriteFile(manifestPath, []byte(updatedManifest), 0o600); err != nil {
				t.Fatal(err)
			}
			updatedGuidance := "Updated primary guidance from " + packID + ".\n"
			if err := os.WriteFile(filepath.Join(updatedBundle, "instructions", packID+".md"), []byte(updatedGuidance), 0o600); err != nil {
				t.Fatal(err)
			}
			updatedCatalog, err := capabilitypack.DiscoverForDurableIntents(updatedBundle)
			if err != nil {
				t.Fatal(err)
			}
			adapter = opencode.NewSurfaceAdapter(updatedBundle, filepath.Join(root, "skills"), config, prompt)
			facade = capabilitypack.NewFacade(updatedCatalog, capabilitypack.WithActivation(store, map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceOpenCode: adapter}))
			update, err := facade.PreviewUpdate(context.Background(), capabilitypack.UpdateRequest{PackID: packID, Surface: capabilitypack.SurfaceOpenCode})
			if err != nil {
				t.Fatal(err)
			}
			updated, err := facade.Apply(context.Background(), capabilitypack.ApplyRequest{Plan: update, Approvals: approvalsFor(t, facade, update), Interactive: true})
			if err != nil || !updated.Verified {
				t.Fatalf("update primary prompt: %#v, %v", updated, err)
			}
			status, err := facade.Status(context.Background(), capabilitypack.StatusRequest{PackID: packID, Surface: capabilitypack.SurfaceOpenCode})
			if err != nil || len(status.Entries) != 1 || status.Entries[0].Projections.Verified != 4 {
				t.Fatalf("primary prompt status = %#v, %v", status, err)
			}
			if data, err := os.ReadFile(prompt); err != nil || string(data) != updatedGuidance {
				t.Fatalf("primary prompt = %q, %v", data, err)
			}
			if err := os.WriteFile(prompt, []byte("operator drift\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			drifted, err := facade.Status(context.Background(), capabilitypack.StatusRequest{PackID: packID, Surface: capabilitypack.SurfaceOpenCode})
			if err != nil || drifted.Entries[0].Projections.Drifted != 1 {
				t.Fatalf("primary prompt drift status = %#v, %v", drifted, err)
			}
			if err := os.WriteFile(prompt, []byte(updatedGuidance), 0o640); err != nil {
				t.Fatal(err)
			}
			removal, err := facade.PreviewDeactivate(context.Background(), capabilitypack.DeactivationRequest{PackID: packID, Surface: capabilitypack.SurfaceOpenCode})
			if err != nil {
				t.Fatal(err)
			}
			removed, err := facade.Apply(context.Background(), capabilitypack.ApplyRequest{Plan: removal, Approvals: approvalsFor(t, facade, removal), Interactive: true})
			if err != nil || !removed.Verified {
				t.Fatalf("remove primary prompt: %#v, %v", removed, err)
			}
			if _, err := os.Stat(prompt); !os.IsNotExist(err) {
				t.Fatalf("primary prompt remains: %v", err)
			}
			configData, err := os.ReadFile(config)
			if err != nil || !strings.Contains(string(configData), "operator setting") || strings.Contains(string(configData), prompt) {
				t.Fatalf("safe removal config = %q, %v", configData, err)
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

func writePrimaryPromptPack(t *testing.T, bundle, packID string) {
	t.Helper()
	for _, resource := range []struct{ path, content string }{
		{"skills/" + packID + "/SKILL.md", "# " + packID + "\n"},
		{"skills/plain/SKILL.md", "# Plain\n"},
		{"instructions/" + packID + ".md", "Primary guidance from " + packID + ".\n"},
	} {
		path := filepath.Join(bundle, resource.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(resource.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	packDir := filepath.Join(bundle, "packs", packID)
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{
  "id": %q,
  "version": "1.0.0",
  "description": "Synthetic OpenCode primary prompt tracer",
  "selectable": true,
  "surfaces": ["opencode"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "resources": [
    {
      "kind": "skill", "id": "alpha-owner", "source": %q, "description": "Owns the primary prompt", "requires": [], "conflicts": [],
      "bindings": [{"surface": "opencode", "projection": "skill", "name": %q, "invocation": %q, "mode": "native", "sharing": "exclusive", "capabilities": [{"type": "opencode-primary-prompt", "primary_prompt": {"id": %q, "source": %q}}]}],
      "surface_exclusions": []
    },
    {
      "kind": "skill", "id": "zeta-plain", "source": "skills/plain", "description": "Plain skill", "requires": [], "conflicts": [],
      "bindings": [{"surface": "opencode", "projection": "skill", "name": %q, "invocation": %q, "mode": "native", "sharing": "exclusive", "capabilities": []}],
      "surface_exclusions": []
    }
  ],
  "exclusions": []
}
`, packID, "skills/"+packID, packID+"-owner", packID+"-owner", packID+"-primary", "instructions/"+packID+".md", packID+"-plain", packID+"-plain")
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
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
