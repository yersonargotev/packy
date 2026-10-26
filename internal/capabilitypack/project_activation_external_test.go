package capabilitypack_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/agents"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
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
	if err := fixture.WriteCatalog(bundle); err != nil {
		t.Fatal(err)
	}
	catalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	resourceIdentity := fixture.OperationalResource()
	resource := capabilitypack.ResourceIdentity{Kind: resourceIdentity.Kind, ID: resourceIdentity.ID}
	project := t.TempDir()
	adapter := agents.NewSurfaceAdapter(bundle, filepath.Join(t.TempDir(), "global-skills"), filepath.Join(t.TempDir(), "roles"))
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
		PackID: fixture.packID, Surface: capabilitypack.SurfaceAgents, ProjectRoot: fixture.project, PackyHome: fixture.packyHome,
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
	if err := fixture.candidate.WriteCatalog(updatedBundle); err != nil {
		t.Fatal(err)
	}
	updatedCatalog, err := capabilitypack.DiscoverForDurableIntents(context.Background(), updatedBundle)
	if err != nil {
		t.Fatal(err)
	}
	updatedAdapter := agents.NewSurfaceAdapter(updatedBundle, filepath.Join(t.TempDir(), "global-skills"), filepath.Join(t.TempDir(), "roles"))
	updatedFacade := capabilitypack.NewFacade(updatedCatalog)
	preview, err := updatedFacade.PreviewProjectUpdate(context.Background(), capabilitypack.ProjectUpdateRequest{PackID: fixture.packID, Surface: capabilitypack.SurfaceAgents, ProjectRoot: fixture.project, PackyHome: fixture.packyHome}, updatedAdapter)
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

func TestProjectActivationRequiresAProjectInstallation(t *testing.T) {
	fixture := newProjectInstallFixture(t)
	_, err := capabilitypack.NewFacade(capabilitypack.Catalog{}).PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{PackID: fixture.packID, Surface: capabilitypack.SurfaceAgents, ProjectRoot: fixture.project, PackyHome: fixture.packyHome, Adapter: fixture.adapter})
	if err == nil || !strings.Contains(err.Error(), "project installation") {
		t.Fatalf("missing installation error = %v", err)
	}
}

func TestProjectActivationIsNotRequiredForDeclarativeOnlyInstallation(t *testing.T) {
	fixture := newProjectInstallFixture(t)
	install, err := fixture.facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{
		PackID: fixture.packID, Surface: capabilitypack.SurfaceAgents, ProjectRoot: fixture.project, PackyHome: fixture.packyHome,
		Selection: capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{fixture.resource}},
	}, fixture.adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: install, PackyHome: fixture.packyHome, Adapter: fixture.adapter}); err != nil {
		t.Fatalf("apply synthetic project install: %v; preview=%+v", err, install)
	}
	preview, err := fixture.facade.PreviewProjectActivation(context.Background(), capabilitypack.ProjectActivationRequest{PackID: fixture.packID, Surface: capabilitypack.SurfaceAgents, ProjectRoot: fixture.project, PackyHome: fixture.packyHome, Adapter: fixture.adapter})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition != capabilitypack.ProjectActivationNotRequired || preview.RuntimeRequired || len(preview.Categories) != 0 {
		t.Fatalf("declarative preview = %+v", preview)
	}
	status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{ProjectRoot: fixture.project, PackID: fixture.packID, Surface: capabilitypack.SurfaceAgents, PackyHome: fixture.packyHome, RequireUsable: true, Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceAgents: fixture.adapter}})
	if err != nil || len(status.Packs) != 1 || status.Packs[0].Runtime != capabilitypack.ProjectRuntimeNotRequired || status.Packs[0].RequirementSatisfied || status.Packs[0].Readiness.Configured != capabilitypack.ReadinessTrue || status.Packs[0].Readiness.Authorized != capabilitypack.ReadinessUnknown || status.Packs[0].Readiness.Usable != capabilitypack.ReadinessUnknown {
		t.Fatalf("declarative status = %+v, %v", status, err)
	}
	if _, err := fixture.facade.ApplyProjectActivation(context.Background(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Adapter: fixture.adapter, Interactive: true}); err == nil || !strings.Contains(err.Error(), "not-required") {
		t.Fatalf("empty activation error = %v", err)
	}
}
