package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredRoleClientAndScopeRejectBeforeObservationOrMutation(t *testing.T) {
	pack := Pack{ID: "workflow", Version: "1.0.0", Surfaces: []Surface{SurfaceAgents}, Resources: []Resource{
		{Kind: "agent", ID: "reviewer", Bindings: []Binding{{Surface: SurfaceAgents, Projection: "agent", Name: "reviewer", ClientProjection: &ClientProjection{Client: "codex", Scopes: []string{"global"}}}}},
		{Kind: "skill", ID: "review", Requires: []string{"agent:reviewer"}, Bindings: []Binding{{Surface: SurfaceAgents, Projection: "skill", Name: "review"}}},
	}}
	for _, client := range []string{"", "claude", "codex"} {
		t.Run(client, func(t *testing.T) {
			adapter := &fakeSurfaceAdapter{}
			store := &fakeActivationStore{}
			facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceAgents: adapter}))
			selection := ResourceSelection{Mode: SelectionCustom, Client: client, Roots: []ResourceIdentity{{Kind: "skill", ID: "review"}}}
			project, home := t.TempDir(), t.TempDir()
			var err error
			if client == "codex" {
				_, err = facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceAgents, ProjectRoot: project, PackyHome: home, Selection: selection}, adapter)
			} else {
				_, err = facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceAgents, Selection: selection})
			}
			if err == nil || (!strings.Contains(err.Error(), "client") && !strings.Contains(err.Error(), "project Codex roles")) {
				t.Fatalf("unsupported client/scope accepted: %v", err)
			}
			if adapter.inspectCalls != 0 || len(adapter.applied) != 0 || len(store.saves) != 0 {
				t.Fatal("unsupported selection reached surface or wrote state")
			}
			for _, root := range []string{project, home} {
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 0 {
					t.Fatalf("unsupported selection wrote %s: %v %v", root, entries, err)
				}
			}
		})
	}
}

func TestOfflineAgentsRoleReceiptRequiresExplicitClient(t *testing.T) {
	facade, store, adapter := receiptStatusFixture(t, func(state *ActivationState) {
		state.Intent.Selection = ResourceSelection{Mode: SelectionAll}
		state.Intent.Resources = []ResourceIdentity{{Kind: "agent", ID: "reviewer"}}
		state.Ownership[0].ProjectionID = "agent:reviewer"
		state.Ownership[0].ID = "surface:agents:agent:reviewer"
	}, nil)
	_, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceAgents})
	if err == nil || !strings.Contains(err.Error(), "invalid identity, selection") {
		t.Fatalf("role receipt without client accepted: %v", err)
	}
	if adapter.inspectCalls != 0 || len(store.saves) != 0 {
		t.Fatal("invalid receipt was observed or rewritten")
	}
	store.state.Intent.Selection.Client = "codex"
	if _, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceAgents}); err != nil {
		t.Fatalf("otherwise identical explicit-client receipt rejected: %v", err)
	}
}

func TestAgentsInstructionSelectionRequiresProjectScope(t *testing.T) {
	pack := Pack{ID: "guidance", Version: "1.0.0", Surfaces: []Surface{SurfaceAgents}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide.md", Bindings: []Binding{{Surface: SurfaceAgents, Projection: "instruction", Name: "guide"}}}}}
	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceAgents: adapter}))
	_, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceAgents, Selection: ResourceSelection{Mode: SelectionAll}})
	if err == nil || !strings.Contains(err.Error(), "project-only") {
		t.Fatalf("global instructions accepted: %v", err)
	}
	if adapter.inspectCalls != 0 || len(store.saves) != 0 {
		t.Fatal("global instructions reached surface or wrote state")
	}
	project := t.TempDir()
	adapter.inspect = func(transition SurfaceTransition) SurfaceInspection {
		if transition.ProjectRoot != project {
			t.Fatal("lost project scope")
		}
		return SurfaceInspection{Revision: "project", Projections: []ObservedProjection{{ID: "instruction:guide", Goal: ProjectionPresent, DesiredFingerprint: "guide", Action: ProjectionAction{ID: "instruction:guide", Kind: ActionInstructionFile, Surface: SurfaceAgents, Target: filepath.Join(project, "AGENTS.md"), Content: "reviewed guidance", FileMode: 0644, PreviewOnly: true}}}}
	}
	preview, err := facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceAgents, ProjectRoot: project, PackyHome: t.TempDir(), Selection: ResourceSelection{Mode: SelectionAll}}, adapter)
	if err != nil || preview.Disposition != ProjectInstallPreviewable || adapter.inspectCalls == 0 {
		t.Fatalf("project instruction selection rejected: %+v %v", preview, err)
	}
}
