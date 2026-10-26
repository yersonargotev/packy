package capabilitypack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestClaudeRetiredResourcesFailCatalogAdmission(t *testing.T) {
	for _, kind := range []string{"mcp_server", "lifecycle", "command"} {
		t.Run(kind, func(t *testing.T) {
			fixture := testsupport.CapabilityRich("retired-contract")
			root := t.TempDir()
			if err := fixture.WriteCatalog(root); err != nil {
				t.Fatal(err)
			}
			packRoot := filepath.Join(root, "packs", fixture.ID())
			path := filepath.Join(packRoot, "pack.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			resource := manifest["resources"].([]any)[0].(map[string]any)
			resource["kind"] = kind
			data, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadCurrentManifest(path, packRoot, false); err == nil || !strings.Contains(err.Error(), "unsupported resource kind") {
				t.Fatalf("retired %s admitted: %v", kind, err)
			}
		})
	}
}

func TestRequiredClaudeRoleScopeFailsBeforeObservation(t *testing.T) {
	pack := Pack{ID: "workflow", Version: "1.0.0", Surfaces: []Surface{SurfaceClaude}, Resources: []Resource{
		{Kind: "agent", ID: "reviewer", Bindings: []Binding{{Surface: SurfaceClaude, Projection: "agent", Name: "reviewer", ClientProjection: &ClientProjection{Client: "claude", Scopes: []string{"global"}}}}},
		{Kind: "skill", ID: "review", Requires: []string{"agent:reviewer"}, Bindings: []Binding{{Surface: SurfaceClaude, Projection: "skill", Name: "review"}}},
	}}
	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceClaude: adapter}))
	project, home := t.TempDir(), t.TempDir()
	_, err := facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceClaude, ProjectRoot: project, PackyHome: home, Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "review"}}}}, adapter)
	if err == nil || !strings.Contains(err.Error(), "does not support project scope") {
		t.Fatalf("unsupported scope accepted: %v", err)
	}
	if adapter.inspectCalls != 0 || len(adapter.applied) != 0 || len(store.saves) != 0 {
		t.Fatal("unsupported role reached adapter or state mutation")
	}
	for _, root := range []string{project, home} {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("unsupported scope wrote files: %v %v", entries, err)
		}
	}
}
