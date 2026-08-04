package capabilitypack_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestProjectActivationPreviewsAndPersistsSeparateCodexConsent(t *testing.T) {
	facade, adapter, project, packyHome := projectInstallFixture(t)
	install, err := facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{
		PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project,
	}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: packyHome, Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	// The portable lock is the only preview input for sensitive behavior. This
	// simulates a lock whose installed declarative pack includes MCP and hooks.
	lockPath := filepath.Join(project, "packy.lock.json")
	var lock capabilitypack.ProjectLockProposal
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	lock.Sensitive = []capabilitypack.ProjectSensitiveDisclosure{
		{Category: capabilitypack.ProjectActivationMCP, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "mcp_server"},
		{Category: capabilitypack.ProjectActivationHooks, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "lifecycle"},
		{Category: capabilitypack.ProjectActivationPlugins, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "plugin"},
		{Category: capabilitypack.ProjectActivationExternalRequirements, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "tool:node"},
		{Category: capabilitypack.ProjectActivationTrust, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "project-trust"},
		{Category: capabilitypack.ProjectActivationAuthentication, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "environment-reference:PACKY_TEST_TOKEN"},
	}
	updatedLock, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, append(updatedLock, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := facade.PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{
		PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.ProjectRoot != "<project-root>" || preview.Disposition != capabilitypack.ProjectActivationPreviewable || !preview.RuntimeRequired || preview.Digest == "" {
		t.Fatalf("preview = %+v", preview)
	}
	wantCategories := []capabilitypack.ProjectActivationCategory{
		capabilitypack.ProjectActivationMCP, capabilitypack.ProjectActivationHooks, capabilitypack.ProjectActivationPlugins,
		capabilitypack.ProjectActivationExternalRequirements, capabilitypack.ProjectActivationTrust, capabilitypack.ProjectActivationAuthentication,
	}
	if got := preview.Categories; len(got) != len(wantCategories) {
		t.Fatalf("categories = %+v", got)
	} else {
		for i, want := range wantCategories {
			if got[i].Kind != want || !got[i].ApprovalRequired {
				t.Fatalf("category %d = %+v, want %s requiring approval", i, got[i], want)
			}
		}
	}
	if _, err := facade.ApplyProjectActivation(context.Background(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Adapter: adapter}); err == nil || !strings.Contains(err.Error(), "interactive") {
		t.Fatalf("non-interactive activation error = %v", err)
	}
	approvals := make([]capabilitypack.ProjectActivationApproval, 0, len(preview.Categories))
	for _, category := range preview.Categories {
		approvals = append(approvals, facade.ApproveProjectActivation(preview, category.Kind))
	}
	result, err := facade.ApplyProjectActivation(context.Background(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Approvals: approvals, Adapter: adapter, Interactive: true})
	if err != nil || result.Status != "active" {
		t.Fatalf("apply = %+v, %v", result, err)
	}
	status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: project, PackID: "matty", Surface: capabilitypack.SurfaceCodex, PackyHome: packyHome, RequireUsable: true, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Runtime != capabilitypack.ProjectRuntimeActive {
		t.Fatalf("status = %+v, %v", status, err)
	}
	for i := range lock.Projections {
		if lock.Projections[i].Resource == (capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}) {
			lock.Projections[i].Command = "changed-sensitive-command"
		}
	}
	changedLock, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, append(changedLock, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: project, PackID: "matty", Surface: capabilitypack.SurfaceCodex, PackyHome: packyHome, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Runtime != capabilitypack.ProjectRuntimeStale {
		t.Fatalf("changed sensitive lock status = %+v, %v", status, err)
	}
	statePath := filepath.Join(packyHome, "projects", projectActivationDigest(t, project), "state.json")
	info, err := os.Stat(statePath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("personal state mode = %v, %v", info.Mode(), err)
	}
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"approvals"`, `"receipts"`, `"sensitive_lock_identity"`, `"recovery"`} {
		if !strings.Contains(string(state), field) {
			t.Fatalf("personal state omitted %s: %s", field, state)
		}
	}
	if strings.Contains(string(state), "TOKEN=") {
		t.Fatalf("personal state must be secret-safe: %q", state)
	}
}

func TestProjectActivationRequiresAProjectInstallation(t *testing.T) {
	_, adapter, project, packyHome := projectInstallFixture(t)
	_, err := capabilitypack.NewFacade(capabilitypack.Catalog{}).PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err == nil || !strings.Contains(err.Error(), "project installation") {
		t.Fatalf("missing installation error = %v", err)
	}
}

func projectActivationDigest(t *testing.T, root string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(filepath.Clean(canonical)))
	return hex.EncodeToString(sum[:])
}

func TestProjectActivationIsNotRequiredForDeclarativeOnlyInstallation(t *testing.T) {
	facade, adapter, project, packyHome := projectInstallFixture(t)
	install, err := facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: packyHome, Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	preview, err := facade.PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition != capabilitypack.ProjectActivationNotRequired || preview.RuntimeRequired || len(preview.Categories) != 0 {
		t.Fatalf("declarative preview = %+v", preview)
	}
	status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: project, PackID: "matty", Surface: capabilitypack.SurfaceCodex, PackyHome: packyHome, RequireUsable: true, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Runtime != capabilitypack.ProjectRuntimeNotRequired || !status.Packs[0].RequirementSatisfied {
		t.Fatalf("declarative status = %+v, %v", status, err)
	}
	if _, err := facade.ApplyProjectActivation(context.Background(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Adapter: adapter, Interactive: true}); err == nil || !strings.Contains(err.Error(), "not-required") {
		t.Fatalf("empty activation error = %v", err)
	}
}
