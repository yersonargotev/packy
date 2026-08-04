package capabilitypack_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
)

type faultingProjectAdapter struct {
	delegate     capabilitypack.SurfaceAdapter
	failLock     bool
	failRollback bool
}

func (a *faultingProjectAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	return a.delegate.InspectSurface(ctx, transition)
}

func (a *faultingProjectAdapter) ApplyProjections(ctx context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	for _, action := range actions {
		if a.failLock && action.Kind == capabilitypack.ActionProjectLockFile && !strings.HasPrefix(action.ID, "restore:") {
			return &capabilitypack.ProjectionActionError{ID: action.ID, Err: errors.New("injected lock publication failure")}
		}
		if a.failRollback && strings.HasPrefix(action.ID, "restore:") {
			return &capabilitypack.ProjectionActionError{ID: action.ID, Err: errors.New("injected rollback failure")}
		}
	}
	return a.delegate.ApplyProjections(ctx, actions)
}

func TestProjectInstallPublishesLockLastAndRollsBackVerifiedPriorState(t *testing.T) {
	facade, adapter, project, packyHome := projectInstallFixture(t)
	preview, err := facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	fault := &faultingProjectAdapter{delegate: adapter, failLock: true}
	if _, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: preview, PackyHome: packyHome, Adapter: fault}); err == nil || !strings.Contains(err.Error(), "lock publication") {
		t.Fatalf("Apply error = %v", err)
	}
	for _, target := range []string{"packy.json", "packy.lock.json", "PACKY-NOTICES.md", "AGENTS.md", filepath.Join(".agents", "skills", "ask-matt")} {
		if _, err := os.Lstat(filepath.Join(project, target)); !os.IsNotExist(err) {
			t.Fatalf("rollback left %s: %v", target, err)
		}
	}
	if _, err := os.Stat(filepath.Join(packyHome, "projects")); !os.IsNotExist(err) {
		t.Fatalf("successful rollback retained recovery state: %v", err)
	}
}

func TestProjectInstallRetainsAndConsumesRecoveryJournalWhenRollbackCannotBeVerified(t *testing.T) {
	facade, adapter, project, packyHome := projectInstallFixture(t)
	preview, err := facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	fault := &faultingProjectAdapter{delegate: adapter, failLock: true, failRollback: true}
	if _, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: preview, PackyHome: packyHome, Adapter: fault}); err == nil || !strings.Contains(err.Error(), "recovery-required") {
		t.Fatalf("Apply error = %v", err)
	}
	journals, err := filepath.Glob(filepath.Join(packyHome, "projects", "*", "install-journal.json"))
	if err != nil || len(journals) != 1 {
		t.Fatalf("recovery journals = %v, %v", journals, err)
	}
	recovered, err := facade.RecoverProjectInstall(context.Background(), capabilitypack.ProjectInstallRecoveryRequest{ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil || !recovered {
		t.Fatalf("recovery = %t, %v", recovered, err)
	}
	preview, err = facade.PreviewProjectInstall(context.Background(), capabilitypack.ProjectInstallRequest{PackID: "matty", Surface: capabilitypack.SurfaceCodex, ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: preview, PackyHome: packyHome, Adapter: adapter})
	if err != nil || result.Status != "verified" {
		t.Fatalf("recovered Apply = %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(packyHome, "projects")); !os.IsNotExist(err) {
		t.Fatalf("recovery journal was not cleaned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "packy.lock.json")); err != nil {
		t.Fatalf("verified recovery did not publish lock: %v", err)
	}
}

func projectInstallFixture(t *testing.T) (capabilitypack.Facade, capabilitypack.SurfaceAdapter, string, string) {
	t.Helper()
	bundle, err := filepath.Abs(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := capabilitypack.DiscoverForDurableIntents(bundle)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	adapter := codex.NewSurfaceAdapterWithConfig(bundle, filepath.Join(t.TempDir(), "global-skills"), filepath.Join(t.TempDir(), "global-AGENTS.md"), filepath.Join(t.TempDir(), "config.toml"))
	return capabilitypack.NewFacade(catalog), adapter, project, filepath.Join(t.TempDir(), ".packy")
}
