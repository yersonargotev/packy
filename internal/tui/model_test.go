package tui_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/tui"
)

type fakeBackend struct {
	dashboard       tui.Dashboard
	err             error
	load            func() (tui.Dashboard, error)
	loads           int
	initializations int
	initialize      func(func(string)) error
	preview         tui.Preview
	previewErr      error
	previewFor      func(tui.PreviewRequest) (tui.Preview, error)
	previewRequests []tui.PreviewRequest
	applyRequests   []tui.ApplyRequest
	applyResult     tui.ApplyResult
	applyErr        error
	apply           func(func(tui.ApplyProgress)) (tui.ApplyResult, error)
	applyFor        func(tui.ApplyRequest, func(tui.ApplyProgress)) (tui.ApplyResult, error)
}

func (b *fakeBackend) Initialize(_ context.Context, progress func(string)) error {
	b.initializations++
	if b.initialize != nil {
		return b.initialize(progress)
	}
	return nil
}

func (b *fakeBackend) Preview(_ context.Context, request tui.PreviewRequest) (tui.Preview, error) {
	b.previewRequests = append(b.previewRequests, request)
	if b.previewFor != nil {
		return b.previewFor(request)
	}
	return b.preview, b.previewErr
}

func (b *fakeBackend) Apply(_ context.Context, request tui.ApplyRequest, progress func(tui.ApplyProgress)) (tui.ApplyResult, error) {
	b.applyRequests = append(b.applyRequests, request)
	if b.applyFor != nil {
		return b.applyFor(request, progress)
	}
	if b.apply != nil {
		return b.apply(progress)
	}
	return b.applyResult, b.applyErr
}

func TestProjectInstallResultSeparatesAndOffersPersonalActivation(t *testing.T) {
	project := tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
		ID: "argote", Version: "1.0.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}},
		SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
	}}}
	backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: project}}
	backend.previewFor = func(request tui.PreviewRequest) (tui.Preview, error) {
		switch request.Operation {
		case "install":
			return tui.Preview{ID: "install-1", Digest: "install-1", Operation: "install", Disposition: "previewable", PackID: "argote", PackVersion: "1.0.0", Surface: "codex", Scope: "project", Phases: []tui.PreviewPhase{{Kind: "project-install", ApprovalRequired: true}}}, nil
		case "activate":
			return tui.Preview{ID: "activation-1", Digest: "activation-1", Operation: "activate", Disposition: "previewable", PackID: "argote", PackVersion: "1.0.0", Surface: "codex", Scope: "project", Phases: []tui.PreviewPhase{{Kind: "trust", ApprovalRequired: true}}}, nil
		default:
			return tui.Preview{}, errors.New("unexpected operation")
		}
	}
	backend.applyFor = func(request tui.ApplyRequest, _ func(tui.ApplyProgress)) (tui.ApplyResult, error) {
		if request.Preview.Operation == "install" {
			return tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Installed argote in the current project", FollowUpOperation: "activate"}, nil
		}
		return tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Personally activated argote for the current project"}, nil
	}

	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Project installation succeeded", "Installed argote in the current project", "Personal runtime activation: not yet activated", "Enter preview personal project activation"} {
		if !strings.Contains(view, want) {
			t.Fatalf("project installation result missing %q:\n%s", want, view)
		}
	}

	installOnly, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if command != nil || len(backend.previewRequests) != 1 || len(backend.applyRequests) != 1 {
		t.Fatalf("leaving the Pack installed produced another operation: previews=%#v applies=%#v", backend.previewRequests, backend.applyRequests)
	}
	if view := ansi.Strip(installOnly.(tui.Model).View().Content); !strings.Contains(view, "Current project") {
		t.Fatalf("install-only path did not return to fresh project status:\n%s", view)
	}

	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 2 || backend.previewRequests[1].Operation != "activate" || backend.previewRequests[1].Scope != "project" {
		t.Fatalf("personal activation did not create its own preview: %#v", backend.previewRequests)
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "activate argote") || !strings.Contains(view, "activation-1") {
		t.Fatalf("personal activation preview is not distinct:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	view = ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Personal project activation succeeded") || len(backend.applyRequests) != 2 || backend.applyRequests[1].Preview.ID != "activation-1" {
		t.Fatalf("personal activation did not use its own consent and Apply:\n%s\nrequests=%#v", view, backend.applyRequests)
	}
}

func TestExistingProjectInstallationOffersFreshPersonalActivationWithoutReinstalling(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "engram", Version: "1.0.1", SurfaceStatuses: []tui.SurfaceStatus{
				{Name: "codex", Supported: true},
				{Name: "opencode", Supported: true, Installation: "installed", Runtime: "pending"},
			},
		}}}},
		preview: tui.Preview{ID: "activation-fresh", Digest: "activation-fresh", Operation: "activate", Disposition: "previewable", PackID: "engram", PackVersion: "1.0.1", Surface: "opencode", Scope: "project", Phases: []tui.PreviewPhase{{Kind: "mcp", ApprovalRequired: true}}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	for _, want := range []string{"Project installation: installed", "Personal runtime activation: pending", "Enter choose project lifecycle action"} {
		if view := ansi.Strip(model.View().Content); !strings.Contains(view, want) {
			t.Fatalf("existing installation detail missing %q:\n%s", want, view)
		}
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Activate · selected") || !strings.Contains(view, "Uninstall") {
		t.Fatalf("installed pending project actions are incomplete:\n%s", view)
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "activate" || backend.previewRequests[0].Surface != "opencode" || backend.previewRequests[0].Selection.Mode != "" {
		t.Fatalf("existing installation did not request fresh personal activation: %#v", backend.previewRequests)
	}
	if view := ansi.Strip(model.View().Content); strings.Contains(view, "install engram") || !strings.Contains(view, "activate engram") {
		t.Fatalf("existing installation was routed through the wrong lifecycle:\n%s", view)
	}
}

func TestProjectStatusOffersOnlyApplicableUpdatePersonalDeactivationAndUninstall(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{
				Name: "codex", Supported: true, Installation: "installed", Runtime: "active", UpdateAvailable: true,
			}},
		}}}},
		preview: tui.Preview{ID: "project-update", Digest: "project-update", Operation: "update", Disposition: "previewable", PackID: "argote", PackVersion: "2.0.0", Scope: "project", ProjectRoot: "/workspace/project", Phases: []tui.PreviewPhase{{Kind: "project-update", ApprovalRequired: true}}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Choose project lifecycle action", "Update · selected", "Deactivate", "Uninstall"} {
		if !strings.Contains(view, want) {
			t.Fatalf("project action menu missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Activate") || strings.Contains(view, "Install") {
		t.Fatalf("installed active project Pack offered an inapplicable action:\n%s", view)
	}

	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 {
		t.Fatalf("project update preview requests = %d, want 1", len(backend.previewRequests))
	}
	request := backend.previewRequests[0]
	if request.Operation != "update" || request.Surface != "codex" || request.Scope != "project" || request.ProjectRoot != "/workspace/project" || request.Selection.Mode != "" {
		t.Fatalf("project update preview request = %#v", request)
	}
}

func TestProjectStatusNeverOffersUninstallForAnAbsentPack(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Installation: "absent"}},
		}}}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	if strings.Contains(view, "uninstall") || !strings.Contains(view, "Enter select resources for project installation") {
		t.Fatalf("absent project Pack exposed uninstall:\n%s", view)
	}
}

func TestProjectUninstallRequiresFocusedDestructiveApprovalAndCancellationIsSafe(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Installation: "installed", Runtime: "pending"}},
		}}}},
		preview: tui.Preview{
			ID: "uninstall-exact", Digest: "uninstall-exact", Operation: "uninstall", Disposition: "previewable", PackID: "argote", PackVersion: "2.0.0", Surface: "codex", Scope: "project", ProjectRoot: "/workspace/project",
			Effects: []tui.PreviewEffect{{Kind: "owned-projection", Target: ".agents/skills/review/SKILL.md", Description: "remove exact owned projection"}, {Kind: "project-manifest", Target: "packy.json", Description: "remove Pack intent"}, {Kind: "project-lock", Target: "packy.lock.json", Description: "remove installed receipt"}},
			Diff:    tui.PreviewDiff{Removed: []string{".agents/skills/review/SKILL.md", "packy.json", "packy.lock.json"}},
			Phases:  []tui.PreviewPhase{{Kind: "destructive-cleanup", ApprovalRequired: true, Actions: []string{"remove .agents/skills/review/SKILL.md", "remove packy.json", "remove packy.lock.json"}}},
		},
		applyResult: tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Uninstalled argote from the current project"},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "uninstall" || backend.previewRequests[0].Selection.Mode != "" {
		t.Fatalf("project uninstall preview request = %#v", backend.previewRequests)
	}
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"uninstall argote", ".agents/skills/review/SKILL.md", "packy.json", "packy.lock.json"} {
		if !strings.Contains(view, want) {
			t.Fatalf("project uninstall preview missing %q:\n%s", want, view)
		}
	}

	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Text: "u"}))
	if command != nil || len(backend.applyRequests) != 0 {
		t.Fatalf("global one-key shortcut applied uninstall: command=%v applies=%#v", command != nil, backend.applyRequests)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(model.View().Content)
	for _, want := range []string{"Destructive cleanup confirmation", "Exact paths and effects", "Cancel ] · selected"} {
		if !strings.Contains(view, want) {
			t.Fatalf("project uninstall consent missing %q:\n%s", want, view)
		}
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.applyRequests) != 0 || backend.loads != 2 {
		t.Fatalf("cancelled uninstall produced effects or skipped reload: applies=%#v loads=%d", backend.applyRequests, backend.loads)
	}

	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	view = ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Project uninstall succeeded") || !strings.Contains(view, "Fresh Pack status reloaded") || len(backend.applyRequests) != 1 {
		t.Fatalf("approved project uninstall did not report verified completion:\n%s\nrequests=%#v", view, backend.applyRequests)
	}
}

func TestProjectUninstallPartialFailureReportsDriftPendingEffectsAndOnlyVerifiedRollback(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Installation: "installed", Runtime: "pending", Drift: 1}},
		}}}},
		preview:     tui.Preview{ID: "uninstall-partial", Digest: "uninstall-partial", Operation: "uninstall", Disposition: "previewable", PackID: "argote", PackVersion: "2.0.0", Surface: "codex", Scope: "project", ProjectRoot: "/workspace/project", Phases: []tui.PreviewPhase{{Kind: "destructive-cleanup", ApprovalRequired: true, Actions: []string{"remove packy.lock.json"}}}},
		applyResult: tui.ApplyResult{Stage: "verification", Summary: "Project Pack uninstalled with pending personal effects", PendingActions: []string{"Personal runtime still requires deactivation"}, RollbackVerified: true},
		applyErr:    errors.New("remaining owned projection drift requires recovery"),
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Project uninstall failed", "remaining owned projection drift", "Personal runtime still requires deactivation", "Rollback: verified by the domain owner", "Enter create fresh preview"} {
		if !strings.Contains(view, want) {
			t.Fatalf("partial uninstall result missing %q:\n%s", want, view)
		}
	}

	backend.applyResult.RollbackVerified = false
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	if view := strings.ToLower(ansi.Strip(model.View().Content)); strings.Contains(view, "rollback") {
		t.Fatalf("unverified rollback was reported:\n%s", view)
	}
}

func TestProjectStatusOmitsNoOpUpdateAndRequiresDestructiveConsentForPersonalDeactivation(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "engram", Version: "1.0.1", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Installation: "installed", Runtime: "active"}},
		}}}},
		preview: tui.Preview{
			ID: "personal-deactivation", Digest: "personal-deactivation", Operation: "deactivate", Disposition: "previewable", PackID: "engram", PackVersion: "1.0.1", Surface: "codex", Scope: "project", ProjectRoot: "/workspace/project",
			Effects: []tui.PreviewEffect{{Kind: "personal-runtime", Target: "<codex-home>/config.toml", Description: "remove project trust"}}, Diff: tui.PreviewDiff{Removed: []string{"<codex-home>/config.toml"}},
			Phases: []tui.PreviewPhase{{Kind: "destructive-cleanup", ApprovalRequired: true, Actions: []string{"remove exact personal project trust contribution"}}},
		},
		applyResult: tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Personally deactivated engram for the current project"},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	if strings.Contains(view, "preview project update") || !strings.Contains(view, "Enter choose project lifecycle action") {
		t.Fatalf("catalog-current active project status exposed the wrong action:\n%s", view)
	}

	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Deactivate · selected") || !strings.Contains(view, "Uninstall") {
		t.Fatalf("installed active project actions are incomplete:\n%s", view)
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "deactivate" || backend.previewRequests[0].Selection.Mode != "" {
		t.Fatalf("personal project deactivation preview request = %#v", backend.previewRequests)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(model.View().Content)
	for _, want := range []string{"Destructive cleanup confirmation", "remove exact personal project trust contribution", "Cancel ] · selected"} {
		if !strings.Contains(view, want) {
			t.Fatalf("personal project deactivation consent missing %q:\n%s", want, view)
		}
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	view = ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Personal project deactivation succeeded") || len(backend.applyRequests) != 1 || !slices.Equal(backend.applyRequests[0].ApprovedPhases, []string{"destructive-cleanup"}) {
		t.Fatalf("personal project deactivation did not use exact destructive consent:\n%s\nrequests=%#v", view, backend.applyRequests)
	}
}

func TestCancellingProjectConsentReloadsStatusWithoutInferringRollback(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "argote", Version: "1.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview: tui.Preview{ID: "install-cancel", Digest: "install-cancel", Operation: "install", Disposition: "previewable", PackID: "argote", PackVersion: "1.0.0", Surface: "codex", Scope: "project", Phases: []tui.PreviewPhase{{Kind: "project-install", ApprovalRequired: true}}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	view := strings.ToLower(ansi.Strip(model.View().Content))
	if backend.loads != 2 || len(backend.applyRequests) != 0 || !strings.Contains(view, "current project") {
		t.Fatalf("project cancellation did not reload fresh status: loads=%d applies=%#v\n%s", backend.loads, backend.applyRequests, view)
	}
	if strings.Contains(view, "rollback") || strings.Contains(view, "rolled back") {
		t.Fatalf("project cancellation inferred unverified rollback:\n%s", view)
	}
}

func TestFailedProjectApplyReloadsStatusWithoutClaimingRollback(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "argote", Version: "1.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview:     tui.Preview{ID: "install-fail", Digest: "install-fail", Operation: "install", Disposition: "previewable", PackID: "argote", PackVersion: "1.0.0", Surface: "codex", Scope: "project", ProjectRoot: "/workspace/project", Phases: []tui.PreviewPhase{{Kind: "project-install", ApprovalRequired: true}}},
		applyResult: tui.ApplyResult{Stage: "apply", Verified: false, Summary: "Project installation stopped before verification"},
		applyErr:    errors.New("projection write failed"),
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := strings.ToLower(ansi.Strip(model.View().Content))
	for _, want := range []string{"project installation failed", "projection write failed", "fresh pack status reloaded", "enter create fresh preview"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failed project result missing %q:\n%s", want, view)
		}
	}
	if backend.loads != 2 || strings.Contains(view, "rollback") || strings.Contains(view, "rolled back") {
		t.Fatalf("failed project Apply did not rely on fresh status: loads=%d\n%s", backend.loads, view)
	}
}

func TestFailedProjectUpdateReloadsStatusAndRequiresANewPreview(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Installation: "installed", Runtime: "active", UpdateAvailable: true}},
		}}}},
		preview:     tui.Preview{ID: "project-update-fail", Digest: "project-update-fail", Operation: "update", Disposition: "previewable", PackID: "argote", PackVersion: "2.0.0", Scope: "project", ProjectRoot: "/workspace/project", Phases: []tui.PreviewPhase{{Kind: "project-update", ApprovalRequired: true}}},
		applyResult: tui.ApplyResult{Stage: "apply", Verified: false, Summary: "Project update stopped before verification"},
		applyErr:    errors.New("second projection write failed after the first effect"),
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)

	view := strings.ToLower(ansi.Strip(model.View().Content))
	for _, want := range []string{"project update failed", "second projection write failed after the first effect", "fresh pack status reloaded", "enter create fresh preview"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failed project update missing %q:\n%s", want, view)
		}
	}
	if backend.loads != 2 || len(backend.applyRequests) != 1 {
		t.Fatalf("failed project update did not reload exactly once: loads=%d applies=%#v", backend.loads, backend.applyRequests)
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 2 || backend.previewRequests[1].Operation != "update" {
		t.Fatalf("failed project update reused its old preview: %#v", backend.previewRequests)
	}
}

func TestGlobalLifecycleOffersOnlyApplicableActionsAndRequestsTheChosenOperation(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", Resources: []tui.Resource{
				{Identity: "skill:review", Role: "root"},
				{Identity: "skill:tdd", Role: "root"},
			}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, UpdateAvailable: true}},
		}}}},
		preview: tui.Preview{ID: "update-1", Digest: "digest-1", Operation: "update", Disposition: "applicable", PackID: "argote", PackVersion: "2.0.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true}}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Choose lifecycle action", "Update · selected", "Deactivate"} {
		if !strings.Contains(view, want) {
			t.Fatalf("active status action menu missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Activate") {
		t.Fatalf("active Pack offered activation:\n%s", view)
	}

	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "update" || backend.previewRequests[0].Selection.Mode != "" {
		t.Fatalf("update preview request = %#v", backend.previewRequests)
	}

	backend.previewRequests = nil
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(model.View().Content)
	for _, want := range []string{"Deactivate Pack resources", "Complete deactivation · selected", "Selected operational roots"} {
		if !strings.Contains(view, want) {
			t.Fatalf("deactivation selection missing %q:\n%s", want, view)
		}
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "deactivate" || backend.previewRequests[0].Selection.Mode != "all" {
		t.Fatalf("complete deactivation request = %#v", backend.previewRequests)
	}
}

func TestChangingFromAnInactiveToActiveSurfaceRevealsItsLifecycleActions(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
		ID: "argote", Version: "2.0.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{
			{Name: "codex", Supported: true},
			{Name: "opencode", Supported: true, Active: true, UpdateAvailable: true},
		},
	}}}}}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Select Pack resources") || !strings.Contains(view, "codex") {
		t.Fatalf("inactive default surface did not offer activation selection:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Choose lifecycle action", "opencode", "Update · selected", "Deactivate"} {
		if !strings.Contains(view, want) {
			t.Fatalf("active alternate surface missing %q:\n%s", want, view)
		}
	}
}

func TestGlobalStatusOmitsNoOpUpdateAndPartialDeactivationRequestsSelectedRoots(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.0.0", Resources: []tui.Resource{
				{Identity: "skill:review", Role: "root"},
				{Identity: "skill:tdd", Role: "root"},
			}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true}},
		}}}},
		preview: tui.Preview{ID: "deactivate-1", Digest: "digest-1", Operation: "deactivate", Disposition: "applicable", PackID: "argote", PackVersion: "1.0.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "custom", Roots: []string{"skill:review"}}, Phases: []tui.PreviewPhase{{Kind: "destructive-cleanup", ApprovalRequired: true}}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	if strings.Contains(view, "Update") || !strings.Contains(view, "Deactivate · selected") {
		t.Fatalf("no-op status exposed the wrong actions:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: " ", Code: ' '}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	request := backend.previewRequests[0]
	if request.Operation != "deactivate" || request.Selection.Mode != "custom" || !slices.Equal(request.Selection.Roots, []string{"skill:tdd"}) {
		t.Fatalf("partial deactivation request = %#v", request)
	}
}

func TestUpdateUsesSharedConsentApplyResultAndFreshReloadFlow(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, UpdateAvailable: true}},
		}}}},
		preview:     tui.Preview{ID: "update-2", Digest: "digest-2", Operation: "update", Disposition: "applicable", PackID: "argote", PackVersion: "2.0.0", Surface: "codex", Scope: "global", Phases: []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true, Actions: []string{"replace owned projection"}}}, Diff: tui.PreviewDiff{Changed: []string{"$HOME/.codex/AGENTS.md"}}},
		applyResult: tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Updated argote on codex"},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Changed: $HOME/.codex/AGENTS.md") || !strings.Contains(view, "Enter continue to consent") {
		t.Fatalf("update preview did not expose changed projections or continuation:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command == nil {
		t.Fatal("approved update did not enter Apply")
	}
	model = runModelCommand(t, model, command)
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Update succeeded", "Verification: verified", "Updated argote on codex", "Fresh Pack status reloaded"} {
		if !strings.Contains(view, want) {
			t.Fatalf("shared update result missing %q:\n%s", want, view)
		}
	}
	if len(backend.applyRequests) != 1 || backend.applyRequests[0].Preview.Operation != "update" {
		t.Fatalf("update Apply request = %#v", backend.applyRequests)
	}
}

func TestDeactivationRefusalHasNoEffectsAndFailedUpdateRequiresFreshPreview(t *testing.T) {
	deactivate := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true}},
		}}}},
		preview: tui.Preview{ID: "deactivate-refused", Digest: "digest", Operation: "deactivate", Disposition: "applicable", PackID: "argote", PackVersion: "1.0.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{{Kind: "destructive-cleanup", ApprovalRequired: true, Actions: []string{"remove owned projection"}}}},
	}
	model := loadModel(t, deactivate)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command != nil || len(deactivate.applyRequests) != 0 || !strings.Contains(ansi.Strip(model.View().Content), "Immutable lifecycle preview") {
		t.Fatalf("deactivation refusal produced effects: command=%v applies=%d", command != nil, len(deactivate.applyRequests))
	}

	update := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, UpdateAvailable: true}},
		}}}},
		preview:  tui.Preview{ID: "update-failed", Digest: "digest", Operation: "update", Disposition: "applicable", PackID: "argote", PackVersion: "2.0.0", Surface: "codex", Scope: "global", Phases: []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true}}},
		applyErr: errors.New("write failed"), applyResult: tui.ApplyResult{Stage: "apply", Verified: false},
	}
	model = loadModel(t, update)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Update failed") || !strings.Contains(view, "Enter create fresh preview") {
		t.Fatalf("failed update did not require a fresh preview:\n%s", view)
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(update.previewRequests) != 2 || update.previewRequests[1].Operation != "update" {
		t.Fatalf("failed update reused its preview: %#v", update.previewRequests)
	}
}

func TestApplicableActivationRequestsOnlyItsRequiredConsentClassesAndCanCancel(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview: tui.Preview{ID: "plan-1", Digest: "digest-1", Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{
			{Kind: "reversible-local", ApprovalRequired: true, Actions: []string{"write AGENTS.md"}},
			{Kind: "host-follow-up", ApprovalRequired: false, Actions: []string{"restart Codex"}},
			{Kind: "executable-external", ApprovalRequired: true, Actions: []string{"install Engram"}},
		}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command != nil {
		t.Fatal("opening consent unexpectedly scheduled Apply")
	}
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Consent 1 of 2", "reversible-local", "write AGENTS.md", "Approve", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("consent screen missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "host-follow-up") || len(backend.applyRequests) != 0 {
		t.Fatalf("consent included a non-required class or applied early:\n%s", view)
	}

	model, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if command != nil || len(backend.applyRequests) != 0 {
		t.Fatalf("canceling consent produced an effect: command=%v applies=%d", command != nil, len(backend.applyRequests))
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Immutable lifecycle preview") || !strings.Contains(view, "plan-1") {
		t.Fatalf("cancel did not return to the exact preview:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, quit := model.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if quit == nil || len(backend.applyRequests) != 0 {
		t.Fatalf("quit during consent was ignored or applied effects: quit=%v applies=%d", quit != nil, len(backend.applyRequests))
	}
}

func TestDestructiveConsentDefaultsToCancelAndApprovesTheExactRequiredCombination(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview: tui.Preview{ID: "plan-2", Digest: "digest-2", Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{
			{Kind: "reversible-local", ApprovalRequired: true, Actions: []string{"write $HOME/.codex/AGENTS.md"}},
			{Kind: "destructive-cleanup", ApprovalRequired: true, Actions: []string{"remove $HOME/.codex/skills/retired"}},
		}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Destructive cleanup confirmation", "Consent 2 of 2", "destructive-cleanup", "Exact paths and effects", "Removal cannot be interrupted after Apply starts", "remove $HOME/.codex/skills/retired", "Cancel", "selected"} {
		if !strings.Contains(view, want) {
			t.Fatalf("destructive consent missing %q:\n%s", want, view)
		}
	}
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command != nil || len(backend.applyRequests) != 0 || !strings.Contains(ansi.Strip(model.View().Content), "Immutable lifecycle preview") {
		t.Fatalf("default destructive choice did not cancel safely: command=%v applies=%d", command != nil, len(backend.applyRequests))
	}

	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command == nil {
		t.Fatal("approving every required class did not schedule Apply")
	}
	model = runModelCommand(t, model, command)
	if len(backend.applyRequests) != 1 || !slices.Equal(backend.applyRequests[0].ApprovedPhases, []string{"reversible-local", "destructive-cleanup"}) {
		t.Fatalf("Apply approvals = %#v, want exact required combination", backend.applyRequests)
	}
}

func TestApplyShowsKnownProgressAndReloadsFreshStatusIntoItsResult(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview: tui.Preview{ID: "plan-3", Digest: "digest-3", Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true}}},
	}
	backend.apply = func(progress func(tui.ApplyProgress)) (tui.ApplyResult, error) {
		progress(tui.ApplyProgress{Phase: "reversible-local"})
		backend.dashboard.Global.Packs[0].SurfaceStatuses[0].Configured = "yes"
		return tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Activated argote on Codex", Details: []string{"3 projections owned"}}, nil
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, applyCommand := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if applyCommand == nil {
		t.Fatal("approved activation did not schedule Apply")
	}
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Apply in progress", "revalidation", "elapsed 0s"} {
		if !strings.Contains(view, want) {
			t.Fatalf("active Apply missing %q:\n%s", want, view)
		}
	}
	model = runModelCommand(t, model, applyCommand)
	view = ansi.Strip(model.View().Content)
	for _, want := range []string{"Activation succeeded", "Stage: verification", "Verification: verified", "Activated argote on Codex", "Details collapsed · ? expand", "Fresh Pack status reloaded"} {
		if !strings.Contains(view, want) {
			t.Fatalf("activation result missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "3 projections owned") {
		t.Fatalf("result details were not initially collapsed:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Details expanded · ? collapse") || !strings.Contains(view, "3 projections owned") {
		t.Fatalf("result details did not expand:\n%s", view)
	}
	if backend.loads != 2 {
		t.Fatalf("loads = %d, want initial load plus post-result reload", backend.loads)
	}
}

func TestQuitDuringApplyIsVisibleThenCompletesAfterApplyAndFreshStatusReload(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview:     tui.Preview{ID: "plan-exit", Digest: "digest-exit", Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true}}},
		applyResult: tui.ApplyResult{Stage: "verification", Verified: true},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, applyCommand := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, quit := model.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if quit != nil || !strings.Contains(ansi.Strip(model.View().Content), "Exit deferred until Apply returns") {
		t.Fatalf("ordinary quit was not visibly deferred: command=%v\n%s", quit != nil, ansi.Strip(model.View().Content))
	}
	_, exited := runModelCommandTrackingQuit(t, model, applyCommand)
	if !exited || backend.loads != 2 {
		t.Fatalf("deferred quit completed=%v after %d loads; want exit after fresh reload", exited, backend.loads)
	}
}

func TestApplyFailureReloadsStatusAndRetryCreatesANewPreview(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview:     tui.Preview{ID: "plan-failed", Digest: "digest-failed", Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true}}},
		applyResult: tui.ApplyResult{Stage: "apply", Verified: false, Summary: "Activation stopped before verification", Details: []string{"projection write rejected"}},
		applyErr:    errors.New("permission denied"),
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Activation failed", "Stage: apply", "Verification: not verified", "permission denied", "Details collapsed · ? expand", "Fresh Pack status reloaded", "Enter create fresh preview"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failed activation result missing %q:\n%s", want, view)
		}
	}

	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 2 || len(backend.applyRequests) != 1 {
		t.Fatalf("retry reused Apply instead of creating a fresh preview: previews=%d applies=%d", len(backend.previewRequests), len(backend.applyRequests))
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Immutable lifecycle preview") || !strings.Contains(view, "plan-failed") {
		t.Fatalf("fresh retry did not return to preview review:\n%s", view)
	}
}

func TestActivationResultRemainsVisibleWhenFreshStatusReloadFails(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview:     tui.Preview{ID: "plan-4", Digest: "digest-4", Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}, Phases: []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true}}},
		applyResult: tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Activated argote on Codex"},
	}
	backend.apply = func(func(tui.ApplyProgress)) (tui.ApplyResult, error) {
		backend.err = errors.New("status unavailable")
		return backend.applyResult, nil
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Activation succeeded", "Activated argote on Codex", "Fresh Pack status reload failed: status unavailable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("result with reload failure missing %q:\n%s", want, view)
		}
	}
}

func TestRunEntersAndRestoresAlternateScreen(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}}}
	input := bytes.NewBufferString("q")
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := tui.Run(ctx, backend, input, &output); err != nil {
		t.Fatal(err)
	}
	for _, sequence := range []string{"\x1b[?1049h", "\x1b[?1049l"} {
		if !strings.Contains(output.String(), sequence) {
			t.Fatalf("terminal output missing alternate-screen sequence %q: %q", sequence, output.String())
		}
	}
}

func TestGlobalControlledRuntimeCheckRecordsPositiveResultAfterItsOwnPreview(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
		ID: "orchestrate", Version: "1.0.0", Resources: []tui.Resource{{Identity: "skill:orchestrate", Role: "root"}},
		SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Configured: "false", Usable: "true", ControlledCheckActionAvailable: true}},
	}}}}}
	backend.previewFor = func(request tui.PreviewRequest) (tui.Preview, error) {
		if request.Operation != "check" || request.Scope != "global" || request.Selection.Mode != "all" {
			return tui.Preview{}, errors.New("unexpected controlled-check preview request")
		}
		return tui.Preview{
			ID: "runtime-check-global", Digest: "runtime-check-global", Operation: "check", Disposition: "applicable",
			PackID: "orchestrate", PackVersion: "1.0.0", Surface: "codex", Scope: "global", Selection: request.Selection,
			ValidityIdentity: "projection-revision=7; adapter=codex-1; host=codex-2", ProjectionRevision: "revision-7", Instructions: []string{"Run the reviewed delegation workflow in Codex."},
			Phases: []tui.PreviewPhase{{Kind: "record-runtime-evidence", ApprovalRequired: true}},
		}, nil
	}
	backend.applyResult = tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Recorded current positive runtime evidence"}

	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Immutable controlled runtime check", "Check instructions", "Projection revision: revision-7", "projection-revision=7", "Enter continue to consent"} {
		if !strings.Contains(view, want) {
			t.Fatalf("controlled-check preview missing %q:\n%s", want, view)
		}
	}

	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Record controlled runtime check result") || !strings.Contains(view, "[ Positive ]") || !strings.Contains(view, "[ Negative ]") {
		t.Fatalf("controlled-check result choice is not explicit:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	if len(backend.applyRequests) != 1 || backend.applyRequests[0].ControlledCheckResult != "positive" {
		t.Fatalf("controlled check Apply request = %#v, want positive result", backend.applyRequests)
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Controlled runtime check succeeded") {
		t.Fatalf("controlled-check result is not distinct from activation:\n%s", view)
	}
}

func TestProjectControlledRuntimeCheckRecordsNegativeResultAndCanBeCancelled(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{
		ID: "orchestrate", Version: "1.0.0", Resources: []tui.Resource{{Identity: "skill:orchestrate", Role: "root"}},
		SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Installation: "installed", Configured: "false", Usable: "true", ControlledCheckActionAvailable: true}},
	}}}}}
	backend.preview = tui.Preview{ID: "runtime-check-project", Digest: "runtime-check-project", Operation: "check", Disposition: "previewable", PackID: "orchestrate", PackVersion: "1.0.0", Surface: "codex", Scope: "project", ProjectRoot: "/workspace/project", Selection: tui.Selection{Mode: "all"}, Instructions: []string{"Confirm the runtime behavior."}, Phases: []tui.PreviewPhase{{Kind: "record-runtime-evidence", ApprovalRequired: true}}}
	backend.applyResult = tui.ApplyResult{Stage: "verification", Verified: true}

	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "check" || backend.previewRequests[0].Scope != "project" {
		t.Fatalf("project controlled-check preview request = %#v", backend.previewRequests)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if len(backend.applyRequests) != 0 || !strings.Contains(ansi.Strip(model.View().Content), "Immutable controlled runtime check") {
		t.Fatalf("cancelling the unselected check result applied evidence or left the preview")
	}

	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelCommand(t, model, command)
	if len(backend.applyRequests) != 1 || backend.applyRequests[0].ControlledCheckResult != "negative" {
		t.Fatalf("controlled check Apply request = %#v, want negative result", backend.applyRequests)
	}
}

func TestDashboardNavigationKeyMapAndNarrowLayout(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "warnings", Passes: 2, Warnings: 1},
		Global: tui.Scope{Packs: []tui.Pack{
			{ID: "argote", Version: "1.2.0", Description: "Agent guidance"},
			{ID: "matty", Version: "1.0.0", Description: "Product review"},
		}},
		Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.WindowSizeMsg{Width: 64, Height: 30})

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'}))
	view := ansi.Strip(current.View().Content)
	if !strings.Contains(view, "› matty") || strings.Contains(view, "Product review") {
		t.Fatalf("down navigation did not select the next Pack:\n%s", view)
	}
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(current.View().Content)
	for _, want := range []string{"Pack details", "Product review"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Enter did not inspect the selected Pack; missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(current.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 64 {
			t.Fatalf("narrow line width = %d, want <= 64: %q", width, ansi.Strip(line))
		}
	}

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	view = ansi.Strip(current.View().Content)
	if !strings.Contains(view, "Current project · selected") || !strings.Contains(view, "/workspace/project") {
		t.Fatalf("tab did not select project scope:\n%s", view)
	}
	if strings.Contains(lineContaining(view, "Workstation"), "Current project") {
		t.Fatalf("narrow layout did not stack scopes:\n%s", view)
	}

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	view = ansi.Strip(current.View().Content)
	for _, want := range []string{"arrows/j/k", "Tab/Shift+Tab", "Enter", "Esc", "Ctrl+C", "r reload", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded key help missing %q:\n%s", want, view)
		}
	}
}

func TestCatalogCanBeFilteredAndOpensCompletePackDetail(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy"},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{
			{ID: "argote", Version: "1.2.0", Description: "Agent guidance"},
			{
				ID: "orchestrate", Version: "1.0.0", Description: "Coordination workflow",
				Requirements: []string{"git"},
				Resources:    []tui.Resource{{Identity: "skill:orchestrate", Description: "Coordinate agents", Role: "operational", Requirements: []string{"notice:mit"}, Conflicts: []string{"skill:legacy"}}},
				Exclusions: []tui.Exclusion{
					{ID: "windows", Reason: "POSIX shell required"},
					{ID: "skill:orchestrate", Surface: "claude", Mode: "unsupported", Code: "surface-unsupported", Reason: "Codex only"},
				},
				SurfaceStatuses: []tui.SurfaceStatus{
					{Name: "claude", Supported: false},
					{Name: "codex", Supported: true, Configured: "yes", Authorized: "yes", Usable: "no", ControlledCheckState: "stale", ControlledCheckResult: "true", ControlledCheckObserved: "2026-08-09T00:00:00Z", ControlledCheckIdentity: "check-identity", Ownership: 2, Drift: 1, Blockers: []string{"runtime unavailable"}, PendingActions: []string{"install helper"}, Evidence: []string{"projection verified with a deliberately long host-owned fingerprint"}},
					{Name: "opencode", Supported: false},
				},
			},
		}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.WindowSizeMsg{Width: 64, Height: 30})

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	for _, char := range "orch" {
		current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: string(char), Code: char}))
	}
	view := ansi.Strip(current.View().Content)
	if strings.Contains(view, "argote") || !strings.Contains(view, "orchestrate") || !strings.Contains(view, "Filter: orch") {
		t.Fatalf("catalog filter did not narrow the visible list:\n%s", view)
	}

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(current.View().Content)
	initialDetailView := view
	allDetailViews := view
	for range 12 {
		current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'}))
		view = ansi.Strip(current.View().Content)
		if lines := strings.Count(view, "\n") + 1; lines > 30 {
			t.Fatalf("scrollable Pack detail height = %d, want <= 30:\n%s", lines, view)
		}
		allDetailViews += "\n" + view
	}
	if view == initialDetailView || !strings.Contains(view, "Pack details · Workstation · global") || !strings.Contains(view, "Enter select resources") {
		t.Fatalf("Pack detail did not scroll while keeping its header and actions fixed:\n%s", view)
	}
	for _, want := range []string{
		"Pack details · Workstation · global", "orchestrate 1.0.0", "Coordination workflow",
		"skill:orchestrate", "Coordinate agents", "requires notice:mit", "conflicts skill:legacy",
		"Requirements: git", "windows — POSIX shell required",
		"skill:orchestrate (surface=claude", "mode=unsupported", "code=surface-unsupported)", "Codex only",
		"claude: unsupported", "codex: supported", "configured=yes authorized=yes usable=no",
		"Controlled runtime check: state=stale result=true", "identity=check-identity",
		"Ownership: 2 projected paths", "Drift: 1 projections", "runtime unavailable",
		"install helper", "projection verified", "opencode: unsupported",
	} {
		if !strings.Contains(allDetailViews, want) {
			t.Fatalf("Pack detail missing %q after scrolling:\n%s", want, allDetailViews)
		}
	}
	for _, line := range strings.Split(current.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 64 {
			t.Fatalf("detail line width = %d, want <= 64: %q", width, ansi.Strip(line))
		}
	}
}

func TestPackLifecycleSelectionDefaultsToFullPackAndExplainsResourceRoles(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy"},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Description: "Agent guidance",
			Resources: []tui.Resource{
				{Identity: "skill:review", Role: "root", Description: "Review changes"},
				{Identity: "instruction:guidance", Role: "dependency", Description: "Shared guidance"},
				{Identity: "asset:template", Role: "asset", Description: "Report template"},
				{Identity: "notice:mit", Role: "notice", Description: "MIT notice"},
			},
			SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}},
	}}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	view := ansi.Strip(model.View().Content)
	for _, want := range []string{
		"Select Pack resources", "argote · codex · Workstation · global",
		"Full Pack · selected", "Advanced operational roots",
		"skill:review [operational root]", "instruction:guidance [derived dependency · read-only]",
		"asset:template [asset · included by domain role]", "notice:mit [legal notice · included by domain role]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("selection screen missing %q:\n%s", want, view)
		}
	}
}

func TestFullPackSelectionCreatesAndRendersCompleteImmutablePreview(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{
			Health: tui.Health{Status: "healthy"},
			Global: tui.Scope{Available: true, Packs: []tui.Pack{{
				ID: "argote", Version: "1.2.0",
				Resources:       []tui.Resource{{Identity: "skill:review", Role: "root"}},
				SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
			}}},
		},
		preview: tui.Preview{
			ID: "plan-42", Digest: "sha256:exact", Operation: "activate", Disposition: "applicable",
			PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global",
			Selection: tui.Selection{Mode: "all"},
			Resources: []tui.PreviewResource{
				{Identity: "skill:review", Role: "root", DependencyChain: []string{"skill:review"}},
				{Identity: "instruction:guidance", Role: "dependency", DependencyChain: []string{"skill:review", "instruction:guidance"}},
			},
			Authorities:    []tui.PreviewAuthority{{Resource: "skill:review", Detail: "read project files"}},
			Effects:        []tui.PreviewEffect{{Kind: "skill-link", Target: "$HOME/.codex/skills/review", Description: "create reviewed skill link"}},
			Diff:           tui.PreviewDiff{Added: []string{"skill:review"}, Retained: []string{"instruction:guidance"}},
			Blockers:       []tui.PreviewBlocker{},
			Phases:         []tui.PreviewPhase{{Kind: "reversible-local", ApprovalRequired: true, Actions: []string{"skill-link $HOME/.codex/skills/review"}}},
			PendingActions: []string{"restart Codex"},
		},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if len(backend.previewRequests) != 1 {
		t.Fatalf("preview requests = %d, want 1", len(backend.previewRequests))
	}
	request := backend.previewRequests[0]
	if request.PackID != "argote" || request.Surface != "codex" || request.Scope != "global" || request.Selection.Mode != "all" || len(request.Selection.Roots) != 0 {
		t.Fatalf("preview target or default selection is not exact: %#v", request)
	}
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{
		"Immutable lifecycle preview", "activate argote 1.2.0 · codex · global", "plan-42 · sha256:exact",
		"Selection: Full Pack", "skill:review [root]", "skill:review → instruction:guidance",
		"Authorities", "skill:review — read project files", "Effects", "skill-link — $HOME/.codex/skills/review",
		"Diff", "Added: skill:review", "Retained: instruction:guidance", "Blockers: none",
		"Phases", "reversible-local · approval required", "Pending actions: restart Codex", "Enter continue to consent",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("preview screen missing %q:\n%s", want, view)
		}
	}
}

func TestAdvancedSelectionExposesOnlyOperationalRootsAndSendsExactCustomSelection(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{
			Health: tui.Health{Status: "healthy"},
			Global: tui.Scope{Available: true, Packs: []tui.Pack{{
				ID: "argote", Version: "1.2.0",
				Resources: []tui.Resource{
					{Identity: "skill:review", Role: "root"},
					{Identity: "agent:critic", Role: "root"},
					{Identity: "instruction:guidance", Role: "dependency"},
					{Identity: "asset:template", Role: "asset"},
					{Identity: "notice:mit", Role: "notice"},
				},
				SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
			}}},
		},
		preview: tui.Preview{Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "custom", Roots: []string{"agent:critic"}}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	view := ansi.Strip(model.View().Content)
	for _, want := range []string{
		"Advanced operational roots · selected", "[x] skill:review", "[x] agent:critic",
		"instruction:guidance [derived dependency · read-only]",
		"asset:template [asset · included by domain role]", "notice:mit [legal notice · included by domain role]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("advanced selection missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "[x] asset:template") || strings.Contains(view, "[x] notice:mit") || strings.Contains(view, "[x] instruction:guidance") {
		t.Fatalf("non-root resource was independently selectable:\n%s", view)
	}

	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: " ", Code: ' '}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 {
		t.Fatalf("preview requests = %d, want 1", len(backend.previewRequests))
	}
	selection := backend.previewRequests[0].Selection
	if selection.Mode != "custom" || !slices.Equal(selection.Roots, []string{"agent:critic"}) {
		t.Fatalf("advanced selection = %#v, want only retained operational root", selection)
	}
}

func TestBlockedNoOpAndStalePreviewsNeverOfferConsent(t *testing.T) {
	tests := []struct {
		name    string
		preview tui.Preview
		wants   []string
	}{
		{
			name: "blocked",
			preview: tui.Preview{Operation: "activate", Disposition: "blocked", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global",
				Selection: tui.Selection{Mode: "all"}, Blockers: []tui.PreviewBlocker{{Kind: "ownership", Subject: "AGENTS.md", Detail: "owned by another Pack"}}},
			wants: []string{"Disposition: blocked", "Blockers: ownership AGENTS.md — owned by another Pack", "Continue unavailable"},
		},
		{
			name: "no-op",
			preview: tui.Preview{Operation: "activate", Disposition: "converged", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global",
				Selection: tui.Selection{Mode: "all"}},
			wants: []string{"Disposition: converged", "Blockers: none", "Continue unavailable"},
		},
		{
			name: "stale",
			preview: tui.Preview{Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global",
				Selection: tui.Selection{Mode: "all"}, Stale: true, StaleReason: "catalog changed after preview"},
			wants: []string{"Stale preview — catalog changed after preview", "Create a fresh preview before continuing", "Continue unavailable"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
				ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
			}}}}, preview: test.preview}
			model := loadModel(t, backend)
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			view := ansi.Strip(model.View().Content)
			for _, want := range test.wants {
				if !strings.Contains(view, want) {
					t.Fatalf("preview missing %q:\n%s", want, view)
				}
			}
			updated, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if command != nil || len(backend.previewRequests) != 1 || ansi.Strip(updated.View().Content) != view {
				t.Fatalf("preview advanced despite being review-only: command=%v requests=%d", command != nil, len(backend.previewRequests))
			}
		})
	}
}

func TestPreviewWrapsSafetyEvidenceInNarrowTerminalWithoutTruncation(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
		}}}},
		preview: tui.Preview{Operation: "activate", Disposition: "blocked", PackID: "argote", PackVersion: "1.2.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
			Blockers:       []tui.PreviewBlocker{{Kind: "ownership", Subject: "a-very-long-projection-target", Detail: "the complete blocker detail remains visible instead of being shortened"}},
			PendingActions: []string{"a long pending action whose complete meaning must remain visible"}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 48, Height: 80})
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"the complete blocker detail remains visible", "a long pending action whose complete meaning must remain visible"} {
		borderless := strings.NewReplacer("│", " ", "╭", " ", "╮", " ", "╰", " ", "╯", " ", "─", " ").Replace(view)
		if !strings.Contains(strings.Join(strings.Fields(borderless), " "), want) {
			t.Fatalf("narrow preview truncated %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(model.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 48 {
			t.Fatalf("narrow preview line width = %d, want <= 48: %q", width, ansi.Strip(line))
		}
	}

	model, _ = model.Update(tea.WindowSizeMsg{Width: 48, Height: 14})
	seen := ""
	for range 80 {
		view := ansi.Strip(model.View().Content)
		if lines := strings.Count(view, "\n") + 1; lines > 14 {
			t.Fatalf("preview height = %d lines, want <= 14:\n%s", lines, view)
		}
		seen += "\n" + view
		model, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'}))
	}
	borderless := strings.NewReplacer("│", " ", "╭", " ", "╮", " ", "╰", " ", "╯", " ", "─", " ").Replace(seen)
	normalized := strings.Join(strings.Fields(borderless), " ")
	for _, want := range []string{"Immutable lifecycle preview", "Dependency closure", "Authorities", "Effects", "Diff", "Blockers", "Phases", "Pending actions", "complete meaning must remain visible"} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("scrollable preview never exposed %q:\n%s", want, seen)
		}
	}
}

func TestSelectionMakesTheSingleCLISurfaceVisibleAndChangeable(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "argote", Version: "1.2.0", Resources: []tui.Resource{{Identity: "skill:review", Role: "root"}},
			SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}, {Name: "opencode", Supported: true}, {Name: "claude", Supported: false}},
		}}}},
		preview: tui.Preview{Operation: "activate", Disposition: "applicable", PackID: "argote", PackVersion: "1.2.0", Surface: "opencode", Scope: "global", Selection: tui.Selection{Mode: "all"}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "CLI surface: codex · selected") || !strings.Contains(view, "←/→ change surface") {
		t.Fatalf("default exact surface is not reviewable:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	view = ansi.Strip(model.View().Content)
	if !strings.Contains(view, "CLI surface: opencode · selected") || strings.Contains(view, "CLI surface: claude · selected") {
		t.Fatalf("surface navigation did not choose the next supported surface:\n%s", view)
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Surface != "opencode" {
		t.Fatalf("preview did not target the reviewed surface: %#v", backend.previewRequests)
	}
}

func TestSelectionCannotPreviewAnExplicitlyUnsupportedCLISurface(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
		ID: "orchestrate", Version: "1.0.0", Surfaces: []string{"claude"}, Resources: []tui.Resource{{Identity: "skill:orchestrate", Role: "root"}},
		SurfaceStatuses: []tui.SurfaceStatus{{Name: "claude", Supported: false}},
	}}}}}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "CLI surface: unavailable") {
		t.Fatalf("incompatible Pack did not expose unavailable surface:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(updated.View().Content)
	if command != nil || len(backend.previewRequests) != 0 || !strings.Contains(view, "No supported CLI surface is available") {
		t.Fatalf("unsupported surface reached preview: command=%v requests=%#v\n%s", command != nil, backend.previewRequests, view)
	}
}

func TestScopeStatusCannotBeMistakenInPackDetail(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health:  tui.Health{Status: "healthy"},
		Global:  tui.Scope{Available: true, Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
		Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"Pack details · Current project", "/workspace/project"} {
		if !strings.Contains(view, want) {
			t.Fatalf("project detail missing scope marker %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Pack details · Workstation · global") {
		t.Fatalf("project detail was mislabeled as global:\n%s", view)
	}
}

func lineContaining(value, target string) string {
	for _, line := range strings.Split(value, "\n") {
		if strings.Contains(line, target) {
			return line
		}
	}
	return ""
}

func TestDashboardRendersEmptyAndLoadFailureStates(t *testing.T) {
	tests := []struct {
		name    string
		backend *fakeBackend
		want    []string
	}{
		{
			name:    "empty",
			backend: &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}}},
			want:    []string{"No reviewed Packs are available", "No Git project", "Global inspection remains available"},
		},
		{
			name:    "failure",
			backend: &fakeBackend{err: errors.New("catalog is unavailable")},
			want:    []string{"Unable to load dashboard", "catalog is unavailable", "r reload", "q quit"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := tui.NewModel(test.backend)
			updated, _ := model.Update(model.Init()())
			view := updated.View().Content
			for _, want := range test.want {
				if !strings.Contains(view, want) {
					t.Fatalf("view missing %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestDashboardReloadsAfterFailureAndAlwaysRequestsAlternateScreen(t *testing.T) {
	backend := &fakeBackend{err: errors.New("temporary failure")}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	if !current.View().AltScreen {
		t.Fatal("dashboard did not request the alternate screen")
	}

	backend.err = nil
	backend.dashboard = tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Packs: []tui.Pack{{ID: "argote", Version: "1.0.0"}}}}
	current, reload := current.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	if reload == nil || !strings.Contains(current.View().Content, "Loading Packy health") {
		t.Fatalf("reload did not return to loading state:\n%s", current.View().Content)
	}
	current, _ = current.Update(reload())
	if backend.loads != 2 || !strings.Contains(current.View().Content, "argote") {
		t.Fatalf("reload did not replace the failure state: loads=%d\n%s", backend.loads, current.View().Content)
	}
}

func TestReloadReflectsDynamicCatalogChanges(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy"},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{{ID: "first", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())

	backend.dashboard.Global.Packs = []tui.Pack{{ID: "newly-reviewed", Version: "2.0.0"}}
	current, reload := current.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	current, _ = current.Update(reload())
	view := ansi.Strip(current.View().Content)
	if strings.Contains(view, "first") || !strings.Contains(view, "newly-reviewed") {
		t.Fatalf("reload retained a hard-coded catalog instead of backend data:\n%s", view)
	}
}

func (b *fakeBackend) Load(context.Context) (tui.Dashboard, error) {
	b.loads++
	if b.load != nil {
		return b.load()
	}
	return b.dashboard, b.err
}

func TestDashboardLoadsThroughInjectedBackendOutsideUpdate(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy", Passes: 1, Checks: []tui.HealthCheck{{Name: "packy-core", Severity: "PASS", Detail: "Packy core is available"}}},
		Global: tui.Scope{Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)

	if backend.loads != 0 {
		t.Fatal("constructing the model performed I/O")
	}
	if view := model.View().Content; !strings.Contains(view, "Loading Packy health") {
		t.Fatalf("loading view = %q", view)
	}

	load := model.Init()
	if load == nil {
		t.Fatal("Init did not schedule loading")
	}
	message := load()
	if backend.loads != 1 {
		t.Fatalf("backend loads = %d, want 1", backend.loads)
	}
	updated, _ := model.Update(message)
	view := updated.View().Content
	for _, want := range []string{"Packy health", "healthy", "packy-core", "PASS", "Packy core is available", "Workstation", "matty", "1.0.0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("ready view missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardUsesMochaPanelsAndAccessibleStatusLabels(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{
			Status:   "failures",
			Passes:   1,
			Warnings: 1,
			Failures: 1,
			Checks: []tui.HealthCheck{
				{Name: "ready", Severity: "PASS", Detail: "available"},
				{Name: "limited", Severity: "WARN", Detail: "needs attention"},
				{Name: "broken", Severity: "FAIL", Detail: "inspection failed"},
			},
		},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{{ID: "argote", Version: "1.0.0"}}},
	}}
	current := loadModel(t, backend)
	current, _ = current.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := current.View()
	plain := ansi.Strip(view.Content)

	if view.BackgroundColor == nil || view.ForegroundColor == nil {
		t.Fatal("dashboard did not set its terminal-wide Catppuccin foreground and background")
	}
	for _, want := range []string{
		"PACKY", "Packy health · workspace control", "◆ System health",
		"[FAIL] failures", "[OK] PASS", "[WARN] WARN", "[FAIL] FAIL",
		"╭", "Workstation · global · selected", "•",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("themed dashboard missing %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(view.Content, "\x1b[") {
		t.Fatal("themed dashboard did not emit terminal styling")
	}
}

func TestUninitializedDashboardRequiresExplicitFocusedInitialization(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy", Passes: 1},
		Setup: tui.Setup{
			InitializationAvailable: true,
			Blockers: []tui.SetupBlocker{{
				Cause:           "Installed Source is missing",
				AffectedActions: []string{"Pack catalog inspection", "Pack lifecycle actions"},
			}},
		},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"healthy", "Installed Source is missing", "Affected actions: Pack catalog inspection, Pack lifecycle actions", "Initialize Packy", "selected", "Enter initialize"} {
		if !strings.Contains(view, want) {
			t.Fatalf("uninitialized dashboard missing %q:\n%s", want, view)
		}
	}
	if backend.initializations != 0 {
		t.Fatalf("entering the dashboard initialized Packy %d times", backend.initializations)
	}

	current, command := current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command == nil || backend.initializations != 0 {
		t.Fatalf("focused initialization was not scheduled explicitly: command=%v initializations=%d", command != nil, backend.initializations)
	}
	if view := ansi.Strip(current.View().Content); !strings.Contains(view, "Initialization in progress") {
		t.Fatalf("initialization did not enter progress state:\n%s", view)
	}
	current, quit := current.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if quit != nil || !strings.Contains(ansi.Strip(current.View().Content), "Initialization in progress") {
		t.Fatal("active initialization allowed ordinary exit or stopped rendering progress")
	}
	current, _ = current.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	for _, line := range strings.Split(current.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 48 {
			t.Fatalf("active initialization froze responsive rendering: width=%d line=%q", width, ansi.Strip(line))
		}
	}
}

func TestSetupBlockersDisableOnlyAffectedActions(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "warnings", Warnings: 1},
		Setup: tui.Setup{Blockers: []tui.SetupBlocker{{
			Cause:           "project status is unavailable",
			AffectedActions: []string{"Current-project status", "Project Pack lifecycle actions"},
		}}},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{{ID: "argote", Version: "1.0.0", Description: "Agent guidance"}}},
	}}
	current := loadModel(t, backend)
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"project status is unavailable", "Affected actions: Current-project status, Project Pack lifecycle actions", "argote"} {
		if !strings.Contains(view, want) {
			t.Fatalf("blocked dashboard missing %q:\n%s", want, view)
		}
	}
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(current.View().Content); !strings.Contains(view, "Pack details") || !strings.Contains(view, "Agent guidance") {
		t.Fatalf("setup blocker disabled unaffected global inspection:\n%s", view)
	}
}

func TestDashboardShowsMinimumSizeStateAndRecoversIntoOneColumn(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health:  tui.Health{Status: "healthy", Passes: 1},
		Global:  tui.Scope{Available: true, Packs: []tui.Pack{{ID: "argote", Version: "1.0.0"}}},
		Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
	}}
	current := loadModel(t, backend)
	current, _ = current.Update(tea.WindowSizeMsg{Width: 39, Height: 10})
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"Terminal too small", "48 columns", "rows to show safety-critical details", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("minimum-size state missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "argote") || strings.Contains(view, "matty") {
		t.Fatalf("undersized terminal rendered safety-critical dashboard content:\n%s", view)
	}
	for range 4 {
		current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	}
	if len(backend.previewRequests) != 0 || !strings.Contains(ansi.Strip(current.View().Content), "Terminal too small") {
		t.Fatalf("undersized terminal accepted hidden navigation or preview: requests=%#v\n%s", backend.previewRequests, ansi.Strip(current.View().Content))
	}

	current, _ = current.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	view = ansi.Strip(current.View().Content)
	lines := strings.Split(view, "\n")
	globalLine, projectLine := -1, -1
	for index, line := range lines {
		if strings.Contains(line, "Workstation · global") {
			globalLine = index
		}
		if strings.Contains(line, "Current project") {
			projectLine = index
		}
	}
	if globalLine < 0 || projectLine-globalLine < 2 {
		t.Fatalf("narrow dashboard did not recover into one column:\n%s", view)
	}
	for _, line := range strings.Split(current.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 60 {
			t.Fatalf("narrow dashboard line width = %d, want <= 60: %q", width, ansi.Strip(line))
		}
	}
}

func TestInitializationProgressResultReloadAndRetryAreRecoverable(t *testing.T) {
	attempt := 0
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy", Passes: 1},
		Setup:  tui.Setup{InitializationAvailable: true},
	}}
	backend.initialize = func(progress func(string)) error {
		attempt++
		progress("cloning Installed Source")
		if attempt == 1 {
			return errors.New("network unavailable")
		}
		backend.dashboard.Setup = tui.Setup{}
		backend.dashboard.Global = tui.Scope{Available: true, Packs: []tui.Pack{{ID: "argote", Version: "1.0.0"}}}
		return nil
	}

	current := loadModel(t, backend)
	current = runModelMessage(t, current, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"Initialization failed", "network unavailable", "cloning Installed Source", "Enter retry", "Esc dashboard"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failed result missing %q:\n%s", want, view)
		}
	}
	if backend.loads != 2 {
		t.Fatalf("failed initialization loads = %d, want initial load plus fresh diagnosis", backend.loads)
	}

	current = runModelMessage(t, current, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(current.View().Content)
	for _, want := range []string{"Initialization succeeded", "cloning Installed Source", "Enter dashboard"} {
		if !strings.Contains(view, want) {
			t.Fatalf("successful result missing %q:\n%s", want, view)
		}
	}
	if backend.initializations != 2 || backend.loads != 3 {
		t.Fatalf("retry/reload counts = initializations %d, loads %d; want 2 and 3", backend.initializations, backend.loads)
	}

	updated, _ := current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(updated.View().Content)
	if !strings.Contains(view, "argote") || strings.Contains(view, "Initialization succeeded") {
		t.Fatalf("successful result did not continue to reloaded dashboard:\n%s", view)
	}
}

func loadModel(t *testing.T, backend *fakeBackend) tea.Model {
	t.Helper()
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	return current
}

func runModelMessage(t *testing.T, model tea.Model, message tea.Msg) tea.Model {
	t.Helper()
	current, command := model.Update(message)
	return runModelCommand(t, current, command)
}

func runModelCommand(t *testing.T, model tea.Model, command tea.Cmd) tea.Model {
	t.Helper()
	current, _ := runModelCommandTrackingQuit(t, model, command)
	return current
}

func runModelCommandTrackingQuit(t *testing.T, model tea.Model, command tea.Cmd) (tea.Model, bool) {
	t.Helper()
	if command == nil {
		t.Fatal("expected command")
	}
	queue := []tea.Cmd{command}
	current := model
	exited := false
	for len(queue) > 0 {
		command, queue = queue[0], queue[1:]
		message := command()
		if batch, ok := message.(tea.BatchMsg); ok {
			queue = append(queue, []tea.Cmd(batch)...)
			continue
		}
		if _, ok := message.(tea.QuitMsg); ok {
			exited = true
			continue
		}
		if message == nil {
			continue
		}
		var next tea.Cmd
		current, next = current.Update(message)
		if next != nil {
			queue = append(queue, next)
		}
	}
	return current, exited
}
