package capabilitypack

import (
	"context"
	"strings"
	"testing"
)

func TestProjectUpdateAvailabilityRequiresAnIntactOlderInstallation(t *testing.T) {
	tests := []struct {
		name         string
		installation ProjectInstallationState
		installed    string
		catalog      string
		want         bool
	}{
		{name: "older installed Pack", installation: ProjectInstallationInstalled, installed: "1.0.0", catalog: "1.0.1", want: true},
		{name: "catalog current", installation: ProjectInstallationInstalled, installed: "1.0.1", catalog: "1.0.1"},
		{name: "drifted", installation: ProjectInstallationDrifted, installed: "1.0.0", catalog: "1.0.1"},
		{name: "blocked", installation: ProjectInstallationBlocked, installed: "1.0.0", catalog: "1.0.1"},
		{name: "absent installed version", installation: ProjectInstallationInstalled, catalog: "1.0.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ProjectUpdateAvailable(test.installation, test.installed, test.catalog); got != test.want {
				t.Fatalf("ProjectUpdateAvailable(%q, %q, %q) = %v, want %v", test.installation, test.installed, test.catalog, got, test.want)
			}
		})
	}
}

func TestIssue458ClassifiesSensitiveVersionChangesForFreshPersonalActivation(t *testing.T) {
	oldHook := ProjectSensitiveDisclosure{Category: ProjectActivationHooks, Surface: SurfaceCodex, Resource: ResourceIdentity{Kind: "lifecycle", ID: "memory"}, Detail: "hook:memory"}
	newMCP := ProjectSensitiveDisclosure{Category: ProjectActivationMCP, Surface: SurfaceOpenCode, Resource: ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Detail: "mcp:memory"}
	prior := ProjectLockProposal{Receipts: []installedPackReceipt{{Pack: installedPackIdentity{ID: "memory", Version: "1.0.0"}, Surface: SurfaceCodex}}, Sensitive: []ProjectSensitiveDisclosure{oldHook}}
	desired := ProjectLockProposal{Receipts: []installedPackReceipt{{Pack: installedPackIdentity{ID: "memory", Version: "2.0.0"}, Surface: SurfaceCodex}}, Sensitive: []ProjectSensitiveDisclosure{oldHook, newMCP}}

	changes := projectSensitiveChanges(prior, desired, SurfaceCodex)
	if len(changes) != 1 || changes[0].Change != "changed" || changes[0].Resource != oldHook.Resource {
		t.Fatalf("sensitive changes = %#v", changes)
	}
	for _, change := range changes {
		if !strings.Contains(change.Detail, "fresh personal project activation") {
			t.Fatalf("sensitive change omitted reactivation disclosure: %#v", change)
		}
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

func TestIssue626DerivedProjectPackUsesSemanticVersionPrecedence(t *testing.T) {
	pack := deriveProjectManifestPack(ProjectManifestPack{ID: "app", SurfaceIntents: []ProjectSurfaceIntent{
		{Surface: SurfaceCodex, Version: "1.0.0-alpha.2", Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}},
		{Surface: SurfaceOpenCode, Version: "1.0.0-alpha.10", Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}},
	}})
	if pack.Version != "1.0.0-alpha.10" {
		t.Fatalf("derived version = %q, want semantic maximum", pack.Version)
	}
}
