package capabilitypack_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/claudecode"
)

func TestClaudeCompositionCapabilitiesArePackIdentityIndependentThroughProjectLifecycle(t *testing.T) {
	for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
		t.Run(packID, func(t *testing.T) {
			bundle := t.TempDir()
			writeClaudeCompositionPack(t, bundle, packID, "1.0.0", "Original workflow.\n")
			catalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), bundle)
			if err != nil {
				t.Fatal(err)
			}
			project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
			adapter := claudecode.NewSurfaceAdapter(bundle, claudecode.NewCanonicalLayout(""), "", "", nil, nil)
			facade := capabilitypack.NewFacade(catalog)
			install, err := facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude, ProjectRoot: project}, adapter)
			if err != nil || install.Disposition != capabilitypack.ProjectInstallPreviewable {
				t.Fatalf("install = %#v, err=%v", install, err)
			}
			assertClaudeGuideFact(t, install.Selection.Resources)
			assertClaudeGuideFact(t, install.Lock.ResourceGraph.Resources)
			if len(install.Lock.Receipts) != 1 || !containsClaudeGuide(install.Lock.Receipts[0].Resources) {
				t.Fatalf("receipt resources = %#v", install.Lock.Receipts)
			}
			if _, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: packyHome, Adapter: adapter}); err != nil {
				t.Fatal(err)
			}
			assertClaudeCompositionProject(t, project, "Original workflow.\n")
			assertClaudeProjectVerified(t, project, packyHome, packID, adapter)

			updatedBundle := t.TempDir()
			writeClaudeCompositionPack(t, updatedBundle, packID, "1.0.1", "Updated workflow.\n")
			updatedCatalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), updatedBundle)
			if err != nil {
				t.Fatal(err)
			}
			updatedAdapter := claudecode.NewSurfaceAdapter(updatedBundle, claudecode.NewCanonicalLayout(""), "", "", nil, nil)
			updatedFacade := capabilitypack.NewFacade(updatedCatalog)
			update, err := updatedFacade.PreviewProjectUpdate(context.Background(), capabilitypack.ProjectUpdateRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude, ProjectRoot: project}, updatedAdapter)
			if err != nil || update.Disposition != capabilitypack.ProjectInstallPreviewable {
				t.Fatalf("update = %#v, err=%v", update, err)
			}
			if _, err := updatedFacade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: update, PackyHome: packyHome, Adapter: updatedAdapter}); err != nil {
				t.Fatal(err)
			}
			assertClaudeCompositionProject(t, project, "Updated workflow.\n")
			assertClaudeProjectVerified(t, project, packyHome, packID, updatedAdapter)

			agentPath := filepath.Join(project, ".claude", "agents", "reviewer.md")
			agentBytes, err := os.ReadFile(agentPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(agentPath, append(agentBytes, []byte("local drift\n")...), 0o644); err != nil {
				t.Fatal(err)
			}
			blocked, err := capabilitypack.PreviewProjectUninstall(context.Background(), capabilitypack.ProjectUninstallRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude, ProjectRoot: project}, updatedAdapter)
			if err != nil || blocked.Disposition != capabilitypack.ProjectInstallBlocked {
				t.Fatalf("drifted uninstall = %#v, err=%v", blocked, err)
			}
			if err := os.WriteFile(agentPath, agentBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			uninstall, err := capabilitypack.PreviewProjectUninstall(context.Background(), capabilitypack.ProjectUninstallRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude, ProjectRoot: project}, updatedAdapter)
			if err != nil || uninstall.Disposition != capabilitypack.ProjectInstallPreviewable {
				t.Fatalf("uninstall = %#v, err=%v", uninstall, err)
			}
			if _, err := capabilitypack.ApplyProjectUninstall(context.Background(), capabilitypack.ProjectUninstallApplyRequest{Preview: uninstall, PackyHome: packyHome, Adapter: updatedAdapter}); err != nil {
				t.Fatal(err)
			}
			for _, target := range []string{agentPath, filepath.Join(project, ".claude", "skills", "workflow"), filepath.Join(project, ".claude", "assets", "guide", "RESOURCE")} {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("safe removal retained %s: %v", target, err)
				}
			}
		})
	}
}

func TestClaudeCompositionCapabilitiesArePackIdentityIndependentThroughGlobalLifecycle(t *testing.T) {
	for _, packID := range []string{"synthetic-alpha", "synthetic-beta"} {
		t.Run(packID, func(t *testing.T) {
			bundle := t.TempDir()
			writeClaudeCompositionPack(t, bundle, packID, "1.0.0", "Original workflow.\n")
			catalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), bundle)
			if err != nil {
				t.Fatal(err)
			}
			oldPack := catalog.List()[0]
			root := t.TempDir()
			layout := claudecode.NewCanonicalLayout(root)
			store := &memoryActivationStore{}
			ownership := claudecode.NewCapabilityPackOwnershipProvider(store, map[string]capabilitypack.Pack{packID: oldPack, packID + "@1.0.0": oldPack}, layout, bundle)
			adapter := claudecode.NewSurfaceAdapter(bundle, layout, filepath.Join(root, "state"), "claude", claudeCapabilityRunner{}, ownership)
			facade := capabilitypack.NewFacade(catalog, capabilitypack.WithActivation(store, map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceClaude: adapter}))

			activation, err := facade.Preview(context.Background(), capabilitypack.ActivationRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude, Selection: capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll}})
			if err != nil || !activation.Applicable() {
				t.Fatalf("activation = %#v, err=%v", activation.Blockers(), err)
			}
			assertClaudeGuideFact(t, activation.JSONReport(true).ResourceGraph.Resources)
			activated, err := facade.Apply(context.Background(), capabilitypack.ApplyRequest{Plan: activation, Approvals: approvalsFor(t, facade, activation), Interactive: true})
			if err != nil || !activated.Verified {
				t.Fatalf("activation result = %#v, err=%v", activated, err)
			}
			assertClaudeGlobalVerified(t, facade, packID)
			controlled, err := facade.PreviewControlledCheck(context.Background(), capabilitypack.ControlledCheckRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude, PackyHome: t.TempDir()})
			if err != nil || !containsClaudeGuide(controlled.Resources) {
				t.Fatalf("controlled check resources = %#v, err=%v", controlled.Resources, err)
			}

			updatedBundle := t.TempDir()
			writeClaudeCompositionPack(t, updatedBundle, packID, "1.0.1", "Original workflow.\n")
			updatedCatalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), updatedBundle)
			if err != nil {
				t.Fatal(err)
			}
			newPack := updatedCatalog.List()[0]
			updatedOwnership := claudecode.NewCapabilityPackOwnershipProvider(store, map[string]capabilitypack.Pack{packID: newPack, packID + "@1.0.0": oldPack, packID + "@1.0.1": newPack}, layout, updatedBundle)
			updatedAdapter := claudecode.NewSurfaceAdapter(updatedBundle, layout, filepath.Join(root, "state"), "claude", claudeCapabilityRunner{}, updatedOwnership)
			updatedFacade := capabilitypack.NewFacade(updatedCatalog, capabilitypack.WithActivation(store, map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceClaude: updatedAdapter}))
			update, err := updatedFacade.PreviewUpdate(context.Background(), capabilitypack.UpdateRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude})
			if err != nil || !update.Applicable() {
				t.Fatalf("update = %#v, err=%v", update.Blockers(), err)
			}
			updated, err := updatedFacade.Apply(context.Background(), capabilitypack.ApplyRequest{Plan: update, Approvals: approvalsFor(t, updatedFacade, update), Interactive: true})
			if err != nil || !updated.Verified {
				t.Fatalf("update result = %#v, err=%v", updated, err)
			}
			assertClaudeGlobalVerified(t, updatedFacade, packID)

			agentPath := filepath.Join(layout.AgentsDir, "reviewer.md")
			agentBytes, err := os.ReadFile(agentPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(agentPath, append(agentBytes, []byte("local drift\n")...), 0o644); err != nil {
				t.Fatal(err)
			}
			blocked, err := updatedFacade.PreviewDeactivate(context.Background(), capabilitypack.DeactivationRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude})
			if err != nil || blocked.Applicable() || len(blocked.Blockers()) == 0 {
				t.Fatalf("drifted deactivation = %#v, err=%v", blocked.Blockers(), err)
			}
			if err := os.WriteFile(agentPath, agentBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			deactivation, err := updatedFacade.PreviewDeactivate(context.Background(), capabilitypack.DeactivationRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude})
			if err != nil || !deactivation.Applicable() {
				t.Fatalf("deactivation = %#v, err=%v", deactivation.Blockers(), err)
			}
			deactivated, err := updatedFacade.Apply(context.Background(), capabilitypack.ApplyRequest{Plan: deactivation, Approvals: approvalsFor(t, updatedFacade, deactivation), Interactive: true})
			if err != nil || !deactivated.Verified {
				t.Fatalf("deactivation result = %#v, err=%v", deactivated, err)
			}
			for _, target := range []string{agentPath, filepath.Join(layout.SkillsDir, "workflow")} {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("safe deactivation retained %s: %v", target, err)
				}
			}
		})
	}
}

type claudeCapabilityRunner struct{}

func (claudeCapabilityRunner) Run(context.Context, claudecode.Command) claudecode.Result {
	return claudecode.Result{Stdout: claudecode.MinimumSupportedVersion}
}

func assertClaudeGlobalVerified(t *testing.T, facade capabilitypack.Facade, packID string) {
	t.Helper()
	status, err := facade.Status(context.Background(), capabilitypack.StatusRequest{PackID: packID, Surface: capabilitypack.SurfaceClaude})
	if err != nil || len(status.Entries) != 1 || status.Entries[0].Projections.Verified == 0 || status.Entries[0].Projections.Missing != 0 || status.Entries[0].Projections.Drifted != 0 || status.Entries[0].Projections.Ambiguous != 0 || status.Entries[0].Projections.Unmanaged != 0 {
		t.Fatalf("global status = %#v, err=%v", status, err)
	}
	found := false
	for _, resource := range status.Entries[0].ResourceSelections {
		if resource.Resource == (capabilitypack.ResourceIdentity{Kind: "asset", ID: "guide"}) {
			found = resource.Selected && resource.Role == capabilitypack.ResourceRoleAsset && validClaudeGuideChain(resource.DependencyChain)
		}
	}
	if !found {
		t.Fatalf("global resource selections = %#v", status.Entries[0].ResourceSelections)
	}
}

func assertClaudeGuideFact(t *testing.T, facts []capabilitypack.ResourceClosureFact) {
	t.Helper()
	for _, fact := range facts {
		if fact.Resource == (capabilitypack.ResourceIdentity{Kind: "asset", ID: "guide"}) {
			if fact.Role != capabilitypack.ResourceRoleAsset || !validClaudeGuideChain(fact.DependencyChain) {
				t.Fatalf("guide fact = %#v", fact)
			}
			return
		}
	}
	t.Fatalf("guide asset missing from closure: %#v", facts)
}

func containsClaudeGuide(resources []capabilitypack.ResourceIdentity) bool {
	for _, resource := range resources {
		if resource == (capabilitypack.ResourceIdentity{Kind: "asset", ID: "guide"}) {
			return true
		}
	}
	return false
}

func validClaudeGuideChain(chain []capabilitypack.ResourceIdentity) bool {
	return len(chain) == 2 &&
		chain[0] == (capabilitypack.ResourceIdentity{Kind: "skill", ID: "workflow"}) &&
		chain[1] == (capabilitypack.ResourceIdentity{Kind: "asset", ID: "guide"})
}

func assertClaudeProjectVerified(t *testing.T, project, packyHome, packID string, adapter capabilitypack.SurfaceAdapter) {
	t.Helper()
	status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: project, PackID: packID, Surface: capabilitypack.SurfaceClaude, RequireInstalled: true, PackyHome: packyHome, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceClaude: adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Installation != capabilitypack.ProjectInstallationInstalled {
		t.Fatalf("status = %#v, err=%v", status, err)
	}
	for _, projection := range status.Packs[0].Projections {
		if projection.Health != "verified" {
			t.Fatalf("projection = %#v", projection)
		}
	}
}

func assertClaudeCompositionProject(t *testing.T, project, workflow string) {
	t.Helper()
	for path, want := range map[string]string{
		filepath.Join(project, ".claude", "skills", "workflow", "SKILL.md"):               workflow,
		filepath.Join(project, ".claude", "skills", "workflow", "references", "guide.md"): "Reviewed guide.\n",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("%s = %q, err=%v", path, data, err)
		}
	}
	agent, err := os.ReadFile(filepath.Join(project, ".claude", "agents", "reviewer.md"))
	if err != nil || !strings.Contains(string(agent), "name: reviewer\n") || !strings.Contains(string(agent), "skills:\n  - workflow\n") || !strings.Contains(string(agent), "## Packy authority contract") {
		t.Fatalf("agent = %q, err=%v", agent, err)
	}
}

func writeClaudeCompositionPack(t *testing.T, bundle, packID, version, workflow string) {
	t.Helper()
	for path, content := range map[string]string{
		"agents/reviewer.md":       "---\nname: reviewer\ndescription: Review changes\n---\n\n# Reviewer\n",
		"references/guide.md":      "Reviewed guide.\n",
		"skills/workflow/SKILL.md": workflow,
	} {
		target := filepath.Join(bundle, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	packDir := filepath.Join(bundle, "packs", packID)
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{
  "id": %q,
  "version": %q,
  "description": "Synthetic Claude composition",
  "selectable": true,
  "surfaces": ["claude"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "resources": [
    {
      "kind": "agent", "id": "reviewer", "source": "agents/reviewer.md", "description": "Reviews changes", "mode": "subagent",
      "tools": [], "permissions": [], "requires": ["skill:workflow"], "conflicts": [],
      "bindings": [{"surface":"claude","projection":"agent","name":"reviewer","invocation":"@reviewer","mode":"native","sharing":"exclusive","capabilities":[{"type":"claude-agent-document","claude_agent_document":{"skills":[{"kind":"skill","id":"workflow"}],"authority":{"permission_mode":"default","authorities":[]}}}]}],
      "surface_exclusions": []
    },
    {
      "kind": "asset", "id": "guide", "source": "references/guide.md", "description": "Reviewed guide",
      "requires": [], "conflicts": [], "bindings": [], "surface_exclusions": []
    },
    {
      "kind": "skill", "id": "workflow", "source": "skills/workflow", "description": "Workflow",
      "requires": [], "conflicts": [],
      "bindings": [{"surface":"claude","projection":"skill","name":"workflow","invocation":"/workflow","mode":"native","sharing":"exclusive","capabilities":[{"type":"claude-composite-skill","claude_composite_skill":{"dependencies":[],"references":[{"kind":"asset","id":"guide"}]}}]}],
      "surface_exclusions": []
    }
  ],
  "exclusions": []
}`, packID, version)
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}
