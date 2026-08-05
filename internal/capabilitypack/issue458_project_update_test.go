package capabilitypack

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue458ClassifiesSensitiveVersionChangesForFreshPersonalActivation(t *testing.T) {
	oldHook := ProjectSensitiveDisclosure{Category: ProjectActivationHooks, Surface: SurfaceCodex, Resource: ResourceIdentity{Kind: "lifecycle", ID: "memory"}, Detail: "hook:memory"}
	newMCP := ProjectSensitiveDisclosure{Category: ProjectActivationMCP, Surface: SurfaceOpenCode, Resource: ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Detail: "mcp:memory"}
	prior := ProjectLockProposal{Source: ProjectPackSourceIdentity{PackID: "memory", PackVersion: "1.0.0"}, Sensitive: []ProjectSensitiveDisclosure{oldHook}}
	desired := ProjectLockProposal{Source: ProjectPackSourceIdentity{PackID: "memory", PackVersion: "2.0.0"}, Sensitive: []ProjectSensitiveDisclosure{oldHook, newMCP}}

	changes := projectSensitiveChanges(prior, desired)
	if len(changes) != 2 || changes[0].Change != "changed" || changes[0].Resource != oldHook.Resource || changes[1].Change != "added" || changes[1].Resource != newMCP.Resource {
		t.Fatalf("sensitive changes = %#v", changes)
	}
	for _, change := range changes {
		if !strings.Contains(change.Detail, "fresh personal project activation") {
			t.Fatalf("sensitive change omitted reactivation disclosure: %#v", change)
		}
	}
}

func TestIssue458ExactProjectUpdateIgnoresMachineSelectedCurrentBytes(t *testing.T) {
	catalog, err := DiscoverForDurableIntents(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range catalog.packs {
		if catalog.packs[i].ID == "matty" {
			catalog.packs[i].Resources[0].Source = "machine-selected-content"
		}
	}
	pack, err := (Facade{catalog: catalog}).resolveExactProjectUpdatePackUnlocked("matty", "4.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Resources) == 0 || pack.Resources[0].Source == "machine-selected-content" {
		t.Fatalf("exact update used machine-selected current bytes: %#v", pack.Resources)
	}
}

func TestIssue458RetirementRequiresSeparateDestructiveCleanupApproval(t *testing.T) {
	preview := JSONProjectInstallPreview{
		Disposition: ProjectInstallPreviewable,
		Retirements: []ProjectProjectionPlan{{Resource: ResourceIdentity{Kind: "instruction", ID: "old"}}},
		projectRoot: t.TempDir(),
	}
	_, err := (Facade{}).ApplyProjectInstall(context.Background(), ProjectInstallApplyRequest{Preview: preview, PackyHome: t.TempDir(), Adapter: &fakeSurfaceAdapter{}})
	if err == nil || !strings.Contains(err.Error(), "destructive-cleanup") {
		t.Fatalf("retirement approval error = %v", err)
	}
}
