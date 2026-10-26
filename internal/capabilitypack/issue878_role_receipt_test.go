package capabilitypack_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/claudecode"
)

func TestClaudeRoleProjectReceiptPreservesReviewedContractAcrossGlobalComposition(t *testing.T) {
	ctx := context.Background()
	bundle := t.TempDir()
	writeClaudeCompositionPack(t, bundle, "role-composition", "1.0.0", "Reviewed workflow.\n")
	path := filepath.Join(bundle, "packs", "role-composition", "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		if resource["kind"] != "agent" {
			continue
		}
		resource["permissions"] = []string{"filesystem"}
		binding := resource["bindings"].([]any)[0].(map[string]any)
		binding["name"] = "native-reviewer"
		binding["invocation"] = "@native-reviewer"
		capability := binding["capabilities"].([]any)[0].(map[string]any)["claude_agent_document"].(map[string]any)
		capability["authority"] = map[string]any{"permission_mode": "default", "authorities": []any{map[string]any{"portable": "filesystem", "declarations": []string{"permission:filesystem"}, "outcome": "native", "claude_tools": []string{"Read"}, "fallback": "Ask the user when unavailable"}}}
	}
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := capabilitypack.DiscoverForDurableIntents(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	pack := catalog.List()[0]
	home, project := t.TempDir(), t.TempDir()
	layout := claudecode.NewCanonicalLayout(home)
	store := &memoryActivationStore{}
	ownership := claudecode.NewCapabilityPackOwnershipProvider(store, claudePackResolver(map[string]capabilitypack.Pack{pack.ID: pack}), layout, bundle)
	adapter := claudecode.NewSurfaceAdapter(bundle, layout, "", "", nil, ownership)
	facade := capabilitypack.NewFacade(catalog, capabilitypack.WithActivation(store, map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceClaude: adapter}))
	global, err := facade.Preview(ctx, capabilitypack.ActivationRequest{PackID: pack.ID, Surface: capabilitypack.SurfaceClaude})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := facade.Apply(ctx, capabilitypack.ApplyRequest{Plan: global, Approvals: approvalsFor(t, facade, global), Interactive: true}); err != nil || !result.Verified {
		t.Fatalf("global activation: %+v %v", result, err)
	}
	install, err := facade.PreviewProjectInstall(ctx, capabilitypack.ProjectInstallRequest{PackID: pack.ID, Surface: capabilitypack.SurfaceClaude, ProjectRoot: project, PackyHome: home}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.ApplyProjectInstall(ctx, capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: home, Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	preview, err := facade.PreviewProjectActivation(ctx, capabilitypack.ProjectActivationRequest{PackID: pack.ID, Surface: capabilitypack.SurfaceClaude, ProjectRoot: project, PackyHome: home, Adapter: adapter})
	if err != nil || preview.Disposition != capabilitypack.ProjectActivationInheritedGlobal {
		t.Fatalf("identical reviewed global/project role conflicts: %+v %v", preview, err)
	}
	lockPath := filepath.Join(project, "packy.lock.json")
	data, err = os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	receipt := lock["receipts"].([]any)[0].(map[string]any)
	role := receipt["role_bindings"].([]any)[0].(map[string]any)
	if role["name"] != "native-reviewer" || role["invocation"] != "@native-reviewer" {
		t.Fatalf("reviewed role binding lost: %#v", role)
	}
	role["client_projection"].(map[string]any)["scopes"] = []string{"project"}
	data, err = json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	changed, err := facade.PreviewProjectActivation(ctx, capabilitypack.ProjectActivationRequest{PackID: pack.ID, Surface: capabilitypack.SurfaceClaude, ProjectRoot: project, PackyHome: home, Adapter: adapter})
	if err == nil && changed.Disposition != capabilitypack.ProjectActivationBlocked {
		t.Fatalf("different reviewed scopes inherited global consent: %+v", changed)
	}

}
