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
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/codex"
)

type projectInstallFixture struct {
	facade         capabilitypack.Facade
	adapter        capabilitypack.SurfaceAdapter
	project        string
	packyHome      string
	packID         string
	resource       capabilitypack.ResourceIdentity
	currentVersion string
	candidate      testsupport.Fixture
}

func newProjectInstallFixture(t *testing.T) projectInstallFixture {
	t.Helper()
	fixture := testsupport.CapabilityRich("project-runtime")
	bundle := t.TempDir()
	if err := fixture.WriteBundle(bundle); err != nil {
		t.Fatal(err)
	}
	catalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	resourceIdentity := fixture.OperationalResource()
	resource := capabilitypack.ResourceIdentity{Kind: resourceIdentity.Kind, ID: resourceIdentity.ID}
	project := t.TempDir()
	adapter := codex.NewSurfaceAdapterWithConfig(bundle, filepath.Join(t.TempDir(), "global-skills"), filepath.Join(t.TempDir(), "global-AGENTS.md"), filepath.Join(t.TempDir(), "config.toml"))
	return projectInstallFixture{
		facade: capabilitypack.NewFacade(catalog), adapter: adapter, project: project,
		packyHome: filepath.Join(t.TempDir(), ".packy"),
		packID:    fixture.ID(), resource: resource,
		currentVersion: fixture.CurrentVersion(), candidate: fixture.Candidate(),
	}
}

func TestProjectUpdateFreshnessReplaysTheExactSurfaceUpdate(t *testing.T) {
	fixture := newProjectInstallFixture(t)
	install, err := fixture.facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{
		PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, ProjectRoot: fixture.project,
		Selection: capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{fixture.resource}},
	}, fixture.adapter)
	if err != nil {
		t.Fatal(err)
	}
	if install.Pack.Version != fixture.currentVersion {
		t.Fatalf("installed fixture version = %q, want current %q", install.Pack.Version, fixture.currentVersion)
	}
	if _, err := fixture.facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: fixture.packyHome, Adapter: fixture.adapter}); err != nil {
		t.Fatal(err)
	}
	updatedBundle := t.TempDir()
	if err := fixture.candidate.WriteBundle(updatedBundle); err != nil {
		t.Fatal(err)
	}
	updatedCatalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), updatedBundle)
	if err != nil {
		t.Fatal(err)
	}
	updatedAdapter := codex.NewSurfaceAdapterWithConfig(updatedBundle, filepath.Join(t.TempDir(), "global-skills"), filepath.Join(t.TempDir(), "global-AGENTS.md"), filepath.Join(t.TempDir(), "config.toml"))
	updatedFacade := capabilitypack.NewFacade(updatedCatalog)
	preview, err := updatedFacade.PreviewProjectUpdate(context.Background(), capabilitypack.ProjectUpdateRequest{PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, ProjectRoot: fixture.project}, updatedAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Pack.Version != fixture.candidate.CurrentVersion() {
		t.Fatalf("updated fixture version = %q, want candidate %q", preview.Pack.Version, fixture.candidate.CurrentVersion())
	}
	freshness, err := updatedFacade.CheckProjectInstallFreshness(context.Background(), preview, updatedAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if freshness.Disposition != capabilitypack.ProjectInstallPreviewable || len(freshness.Blockers) != 0 {
		t.Fatalf("unchanged surface update freshness = %#v", freshness)
	}
}

func TestProjectActivationPreviewsAndPersistsSeparateCodexConsent(t *testing.T) {
	fixture := newProjectInstallFixture(t)
	install, err := fixture.facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{
		PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, ProjectRoot: fixture.project,
		Selection: capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{fixture.resource}},
	}, fixture.adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: fixture.packyHome, Adapter: fixture.adapter}); err != nil {
		t.Fatalf("apply synthetic project install: %v; preview=%+v", err, install)
	}
	lockPath := filepath.Join(fixture.project, "packy.lock.json")
	var lock capabilitypack.ProjectLockProposal
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	lock.Receipts[0].Sensitive = []capabilitypack.ProjectSensitiveDisclosure{
		{Category: capabilitypack.ProjectActivationMCP, Surface: capabilitypack.SurfaceCodex, Resource: fixture.resource, Detail: "mcp_server"},
		{Category: capabilitypack.ProjectActivationHooks, Surface: capabilitypack.SurfaceCodex, Resource: fixture.resource, Detail: "lifecycle"},
		{Category: capabilitypack.ProjectActivationPlugins, Surface: capabilitypack.SurfaceCodex, Resource: fixture.resource, Detail: "plugin"},
		{Category: capabilitypack.ProjectActivationExternalRequirements, Surface: capabilitypack.SurfaceCodex, Resource: fixture.resource, Detail: "tool:fixture-tool"},
		{Category: capabilitypack.ProjectActivationTrust, Surface: capabilitypack.SurfaceCodex, Resource: fixture.resource, Detail: "project-trust"},
		{Category: capabilitypack.ProjectActivationAuthentication, Surface: capabilitypack.SurfaceCodex, Resource: fixture.resource, Detail: "environment-reference:PACKY_TEST_TOKEN"},
	}
	updatedLock, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, append(updatedLock, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := fixture.facade.PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{
		PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, ProjectRoot: fixture.project, PackyHome: fixture.packyHome, Adapter: fixture.adapter,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.ProjectRoot != "<project-root>" || preview.Disposition != capabilitypack.ProjectActivationPreviewable || !preview.RuntimeRequired || preview.Digest == "" {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Effects) != 1 || preview.Effects[0].Category != capabilitypack.ProjectActivationTrust || preview.Effects[0].Target != "<codex-home>/config.toml" || preview.Effects[0].Identity == "" || preview.Effects[0].Observation == "" {
		t.Fatalf("personal effect preview = %+v", preview.Effects)
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
	if _, err := fixture.facade.ApplyProjectActivation(context.Background(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Adapter: fixture.adapter}); err == nil || !strings.Contains(err.Error(), "interactive") {
		t.Fatalf("non-interactive activation error = %v", err)
	}
	approvals := make([]capabilitypack.ProjectActivationApproval, 0, len(preview.Categories))
	for _, category := range preview.Categories {
		approvals = append(approvals, fixture.facade.ApproveProjectActivation(preview, category.Kind))
	}
	result, err := fixture.facade.ApplyProjectActivation(context.Background(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Approvals: approvals, Adapter: fixture.adapter, Interactive: true})
	if err != nil || result.Status != "active" {
		t.Fatalf("apply = %+v, %v", result, err)
	}
	status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: fixture.project, PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, PackyHome: fixture.packyHome, RequireUsable: true, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: fixture.adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Runtime != capabilitypack.ProjectRuntimeActive {
		t.Fatalf("status = %+v, %v", status, err)
	}
	statePath := filepath.Join(fixture.packyHome, "projects", projectActivationDigest(t, fixture.project), "state-"+fixture.packID+"-codex.json")
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var incomplete map[string]any
	if err := json.Unmarshal(state, &incomplete); err != nil {
		t.Fatal(err)
	}
	incomplete["receipts"] = []any{}
	incompleteState, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, incompleteState, 0o600); err != nil {
		t.Fatal(err)
	}
	blocked, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: fixture.project, PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, PackyHome: fixture.packyHome, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: fixture.adapter}})
	if err != nil || len(blocked.Packs) != 1 || blocked.Packs[0].Runtime != capabilitypack.ProjectRuntimeBlocked {
		t.Fatalf("incomplete personal state did not fail closed: %+v, %v", blocked, err)
	}
	if err := os.WriteFile(statePath, state, 0o600); err != nil {
		t.Fatal(err)
	}
	if len(lock.Receipts[0].Sensitive) == 0 {
		t.Fatal("installed receipt has no sensitive disclosure to change")
	}
	lock.Receipts[0].Sensitive[0].Detail += "-changed"
	changedLock, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, append(changedLock, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: fixture.project, PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, PackyHome: fixture.packyHome, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: fixture.adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Runtime != capabilitypack.ProjectRuntimeStale {
		t.Fatalf("changed sensitive lock status = %+v, %v", status, err)
	}
	info, err := os.Stat(statePath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("personal state mode = %v, %v", info.Mode(), err)
	}
	state, err = os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"approvals"`, `"receipts"`, `"effects"`, `"sensitive_lock_identity"`} {
		if !strings.Contains(string(state), field) {
			t.Fatalf("personal state omitted %s: %s", field, state)
		}
	}
	if strings.Contains(string(state), `"recovery"`) {
		t.Fatalf("personal state persisted retired recovery data: %s", state)
	}
	if strings.Contains(string(state), "TOKEN=") {
		t.Fatalf("personal state must be secret-safe: %q", state)
	}
	if strings.Contains(string(state), "trust_level") || strings.Contains(string(state), "model =") {
		t.Fatalf("effect receipt persisted Codex config content: %q", state)
	}
}

func TestProjectActivationRequiresAProjectInstallation(t *testing.T) {
	fixture := newProjectInstallFixture(t)
	_, err := capabilitypack.NewFacade(capabilitypack.Catalog{}).PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, ProjectRoot: fixture.project, PackyHome: fixture.packyHome, Adapter: fixture.adapter})
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
	fixture := newProjectInstallFixture(t)
	install, err := fixture.facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{
		PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, ProjectRoot: fixture.project,
		Selection: capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{fixture.resource}},
	}, fixture.adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: fixture.packyHome, Adapter: fixture.adapter}); err != nil {
		t.Fatalf("apply synthetic project install: %v; preview=%+v", err, install)
	}
	preview, err := fixture.facade.PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, ProjectRoot: fixture.project, PackyHome: fixture.packyHome, Adapter: fixture.adapter})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition != capabilitypack.ProjectActivationNotRequired || preview.RuntimeRequired || len(preview.Categories) != 0 {
		t.Fatalf("declarative preview = %+v", preview)
	}
	status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: fixture.project, PackID: fixture.packID, Surface: capabilitypack.SurfaceCodex, PackyHome: fixture.packyHome, RequireUsable: true, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: fixture.adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Runtime != capabilitypack.ProjectRuntimeNotRequired || status.Packs[0].RequirementSatisfied || status.Packs[0].Readiness.Configured != capabilitypack.ReadinessTrue || status.Packs[0].Readiness.Authorized != capabilitypack.ReadinessUnknown || status.Packs[0].Readiness.Usable != capabilitypack.ReadinessUnknown {
		t.Fatalf("declarative status = %+v, %v", status, err)
	}
	if _, err := fixture.facade.ApplyProjectActivation(context.Background(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Adapter: fixture.adapter, Interactive: true}); err == nil || !strings.Contains(err.Error(), "not-required") {
		t.Fatalf("empty activation error = %v", err)
	}
}
