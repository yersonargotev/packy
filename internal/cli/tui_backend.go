package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/tui"
	packyversion "github.com/yersonargotev/packy/internal/version"
	"github.com/yersonargotev/packy/internal/workstation"
)

// RunTUI composes the terminal application from Packy's production owners.
func RunTUI(ctx context.Context, opts Options, input io.Reader, output io.Writer) error {
	opts = opts.withDefaults()
	resolver := newWorkstationResolver(opts)
	return tui.Run(ctx, newTUIBackend(opts, resolver), input, output)
}

type tuiBackend struct {
	opts          Options
	resolver      *workstation.Resolver
	repositoryURL string
	repositoryRef string
}

func newTUIBackend(opts Options, resolver *workstation.Resolver) *tuiBackend {
	return &tuiBackend{
		opts:          opts,
		resolver:      resolver,
		repositoryURL: bootstrap.DefaultRepositoryURL,
		repositoryRef: defaultInitRepositoryRef("", packyversion.Value),
	}
}

func (b *tuiBackend) Load(ctx context.Context) (tui.Dashboard, error) {
	health, err := diagnoseSetupHealth(ctx, b.opts, b.resolver)
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("diagnose Packy health: %w", err)
	}
	dashboard := tui.Dashboard{
		Health: healthForTUI(health),
		Global: tui.Scope{Available: true},
	}
	catalog, err := discoverPackCatalog(b.opts, b.resolver)
	if err != nil {
		dashboard.Setup = tui.Setup{
			InitializationAvailable: strings.TrimSpace(b.opts.Env.Getenv("PACKY_SKILLS_SOURCE")) == "",
			Blockers: []tui.SetupBlocker{{
				Cause:           fmt.Sprintf("discover reviewed Pack catalog: %v", err),
				AffectedActions: []string{"Pack catalog inspection", "Pack lifecycle actions"},
			}},
		}
		return dashboard, nil
	}
	details, err := catalog.ListDetails()
	if err != nil {
		dashboard.Setup = tui.Setup{
			InitializationAvailable: strings.TrimSpace(b.opts.Env.Getenv("PACKY_SKILLS_SOURCE")) == "",
			Blockers: []tui.SetupBlocker{{
				Cause:           fmt.Sprintf("load reviewed Pack catalog: %v", err),
				AffectedActions: []string{"Pack catalog inspection", "Pack lifecycle actions"},
			}},
		}
		return dashboard, nil
	}
	dashboard.Global.Packs = catalogPacksForTUI(details, nil)
	facade, err := activationFacade(b.opts, b.resolver)
	if err != nil {
		dashboard.Setup.Blockers = append(dashboard.Setup.Blockers, tui.SetupBlocker{
			Cause:           fmt.Sprintf("compose global Pack status: %v", err),
			AffectedActions: []string{"Global Pack status", "Pack lifecycle actions"},
		})
	} else {
		globalStatus, statusErr := facade.Status(ctx, capabilitypack.StatusRequest{})
		if statusErr != nil {
			dashboard.Setup.Blockers = append(dashboard.Setup.Blockers, tui.SetupBlocker{
				Cause:           fmt.Sprintf("inspect global Pack status: %v", statusErr),
				AffectedActions: []string{"Global Pack status", "Pack lifecycle actions"},
			})
		} else {
			dashboard.Global.Packs = catalogPacksForTUI(details, globalStatusesForTUI(globalStatus))
		}
	}
	snapshot, err := b.resolver.Resolve(workstation.Options{})
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("resolve workstation context: %w", err)
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		dashboard.Setup.Blockers = append(dashboard.Setup.Blockers, tui.SetupBlocker{
			Cause:           fmt.Sprintf("resolve current directory: %v", err),
			AffectedActions: []string{"Current-project inspection", "Project Pack lifecycle actions"},
		})
		return dashboard, nil
	}
	projectRoot, err := capabilitypack.DiscoverProjectRoot(currentDirectory)
	if err != nil {
		var absent capabilitypack.ProjectNotFoundError
		if errors.As(err, &absent) {
			return dashboard, nil
		}
		dashboard.Setup.Blockers = append(dashboard.Setup.Blockers, tui.SetupBlocker{
			Cause:           fmt.Sprintf("discover current project: %v", err),
			AffectedActions: []string{"Current-project inspection", "Project Pack lifecycle actions"},
		})
		return dashboard, nil
	}

	request := capabilitypack.ProjectStatusRequest{
		ProjectRoot: projectRoot,
		PackyHome:   snapshot.PackyHome(),
		Adapters:    projectStatusAdapters(b.opts, snapshot),
	}
	status, err := capabilitypack.InspectProjectStatus(ctx, request)
	if err != nil {
		dashboard.Setup.Blockers = append(dashboard.Setup.Blockers, tui.SetupBlocker{
			Cause:           fmt.Sprintf("inspect current project: %v", err),
			AffectedActions: []string{"Current-project status", "Project Pack lifecycle actions"},
		})
		return dashboard, nil
	}
	dashboard.Project = tui.Scope{Available: true, Root: projectRoot, Packs: catalogPacksForTUI(details, projectStatusesForTUI(status))}
	return dashboard, nil
}

func (b *tuiBackend) Initialize(ctx context.Context, progress func(string)) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return initializeInstalledSource(b.resolver, initializationRequest{
		RepositoryURL: b.repositoryURL,
		RepositoryRef: b.repositoryRef,
		ReportProgress: func(detail string) error {
			progress(detail)
			return nil
		},
	})
}

func (b *tuiBackend) Preview(ctx context.Context, request tui.PreviewRequest) (tui.Preview, error) {
	operation := request.Operation
	surface := capabilitypack.Surface(request.Surface)
	if request.Scope == "project" {
		if request.ProjectRoot == "" {
			return tui.Preview{}, errors.New("project preview requires the current project root")
		}
		snapshot, projectRoot, err := b.requireCurrentProject(request.ProjectRoot)
		if err != nil {
			return tui.Preview{}, err
		}
		composition, err := resolvePackComposition(b.opts, b.resolver)
		if err != nil {
			return tui.Preview{}, err
		}
		facade := capabilitypack.NewFacade(composition.catalog, capabilitypack.WithActivation(capabilitypack.NewFileActivationStore(composition.state.File()), nil))
		switch operation {
		case "install":
			selection, selectionErr := selectionForTUI(request.Selection)
			if selectionErr != nil {
				return tui.Preview{}, selectionErr
			}
			adapter := projectInstallAdapter(surface, composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
			preview, previewErr := facade.PreviewProjectInstall(ctx, capabilitypack.ProjectInstallRequest{
				PackID: request.PackID, Surface: surface, ProjectRoot: projectRoot, Selection: selection,
			}, adapter)
			if previewErr != nil {
				return tui.Preview{}, previewErr
			}
			return projectPreviewForTUI(preview, projectRoot, "install"), nil
		case "update":
			adapter := projectInstallAdapter("", composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
			preview, previewErr := facade.PreviewProjectUpdate(ctx, capabilitypack.ProjectUpdateRequest{PackID: request.PackID, ProjectRoot: projectRoot}, adapter)
			if previewErr != nil {
				return tui.Preview{}, previewErr
			}
			return projectPreviewForTUI(preview, projectRoot, "update"), nil
		case "activate":
			preview, previewErr := facade.PreviewProjectActivation(ctx, capabilitypack.ProjectActivationRequest{
				PackID: request.PackID, Surface: surface, ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: projectRuntimeAdapter(b.opts, surface, snapshot),
			})
			if previewErr != nil {
				return tui.Preview{}, previewErr
			}
			return projectActivationPreviewForTUI(preview, projectRoot), nil
		case "deactivate":
			preview, previewErr := capabilitypack.PreviewProjectDeactivation(ctx, capabilitypack.ProjectDeactivationRequest{
				PackID: request.PackID, Surface: surface, ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: projectRuntimeAdapter(b.opts, surface, snapshot),
			})
			if previewErr != nil {
				return tui.Preview{}, previewErr
			}
			return projectDeactivationPreviewForTUI(preview, projectRoot), nil
		default:
			return tui.Preview{}, fmt.Errorf("project preview operation %q is unsupported", operation)
		}
	}
	if request.Scope != "global" {
		return tui.Preview{}, fmt.Errorf("preview scope %q is unsupported", request.Scope)
	}
	facade, err := activationFacade(b.opts, b.resolver)
	if err != nil {
		return tui.Preview{}, err
	}
	plan, err := globalPlanForTUI(ctx, facade, operation, request.PackID, surface, request.Selection)
	if err != nil {
		return tui.Preview{}, err
	}
	preview := globalPreviewForTUI(plan.JSONReport(true))
	if operation == string(capabilitypack.OperationDeactivate) {
		preview.Selection = request.Selection
	}
	return preview, nil
}

func (b *tuiBackend) Apply(ctx context.Context, request tui.ApplyRequest, progress func(tui.ApplyProgress)) (tui.ApplyResult, error) {
	if request.Preview.Scope == "project" {
		return b.applyProject(ctx, request, progress)
	}
	if request.Preview.Scope != "global" {
		return tui.ApplyResult{Stage: "revalidation"}, fmt.Errorf("TUI Apply scope %q is unsupported", request.Preview.Scope)
	}
	progress(tui.ApplyProgress{Phase: "revalidation"})
	facade, err := activationFacade(b.opts, b.resolver)
	if err != nil {
		return tui.ApplyResult{Stage: "revalidation"}, err
	}
	surface := capabilitypack.Surface(request.Preview.Surface)
	plan, err := globalPlanForTUI(ctx, facade, request.Preview.Operation, request.Preview.PackID, surface, request.Preview.Selection)
	if err != nil {
		return tui.ApplyResult{Stage: "revalidation"}, err
	}
	if plan.ID() != request.Preview.ID || plan.Digest() != request.Preview.Digest {
		return tui.ApplyResult{Stage: "revalidation"}, errors.New("approved preview is stale; create a fresh preview before Apply")
	}
	required := make([]string, 0)
	receipts := make([]capabilitypack.ApprovalReceipt, 0)
	for _, phase := range plan.Phases() {
		if !phase.ApprovalRequired {
			continue
		}
		required = append(required, string(phase.Kind))
		receipts = append(receipts, facade.Approve(plan, phase.Kind))
	}
	if !slices.Equal(request.ApprovedPhases, required) {
		return tui.ApplyResult{Stage: "approval"}, fmt.Errorf("approved effect classes %v do not match required classes %v", request.ApprovedPhases, required)
	}
	progress(tui.ApplyProgress{Phase: "apply"})
	applied, err := facade.Apply(ctx, capabilitypack.ApplyRequest{Plan: plan, Approvals: receipts, Interactive: true})
	if err != nil {
		return tui.ApplyResult{Stage: "apply", Summary: lifecyclePastTense(request.Preview.Operation) + " stopped before verification"}, err
	}
	progress(tui.ApplyProgress{Phase: "verification"})
	return tui.ApplyResult{
		Stage: "verification", Verified: applied.Verified,
		Summary:        fmt.Sprintf("%s %s on %s", lifecyclePastTense(request.Preview.Operation), request.Preview.PackID, request.Preview.Surface),
		Details:        []string{fmt.Sprintf("%d projections owned", applied.Projections)},
		PendingActions: append([]string(nil), applied.PendingHumanActions...),
	}, nil
}

func (b *tuiBackend) applyProject(ctx context.Context, request tui.ApplyRequest, progress func(tui.ApplyProgress)) (tui.ApplyResult, error) {
	progress(tui.ApplyProgress{Phase: "revalidation"})
	snapshot, projectRoot, err := b.requireCurrentProject(request.Preview.ProjectRoot)
	if err != nil {
		return tui.ApplyResult{Stage: "revalidation"}, err
	}
	composition, err := resolvePackComposition(b.opts, b.resolver)
	if err != nil {
		return tui.ApplyResult{Stage: "revalidation"}, err
	}
	facade := capabilitypack.NewFacade(composition.catalog, capabilitypack.WithActivation(capabilitypack.NewFileActivationStore(composition.state.File()), nil))
	surface := capabilitypack.Surface(request.Preview.Surface)
	switch request.Preview.Operation {
	case "install":
		selection, selectionErr := selectionForTUI(request.Preview.Selection)
		if selectionErr != nil {
			return tui.ApplyResult{Stage: "revalidation"}, selectionErr
		}
		adapter := projectInstallAdapter(surface, composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
		fresh, previewErr := facade.PreviewProjectInstall(ctx, capabilitypack.ProjectInstallRequest{PackID: request.Preview.PackID, Surface: surface, ProjectRoot: projectRoot, Selection: selection}, adapter)
		if previewErr != nil {
			return tui.ApplyResult{Stage: "revalidation"}, previewErr
		}
		freshView := projectPreviewForTUI(fresh, projectRoot, "install")
		if freshView.ID != request.Preview.ID || freshView.Digest != request.Preview.Digest {
			return tui.ApplyResult{Stage: "revalidation"}, errors.New("approved preview is stale; create a fresh preview before Apply")
		}
		if err := validateTUIApprovals(request.ApprovedPhases, freshView); err != nil {
			return tui.ApplyResult{Stage: "approval"}, err
		}
		progress(tui.ApplyProgress{Phase: "apply"})
		applied, applyErr := facade.ApplyProjectInstall(ctx, capabilitypack.ProjectInstallApplyRequest{Preview: fresh, PackyHome: snapshot.PackyHome(), Adapter: adapter})
		if applyErr != nil {
			return tui.ApplyResult{Stage: "apply", Summary: "Project installation stopped before verification"}, applyErr
		}
		progress(tui.ApplyProgress{Phase: "verification"})
		result := tui.ApplyResult{Stage: "verification", Verified: applied.Status == "verified" || applied.Status == "no-op", Summary: fmt.Sprintf("Installed %s in the current project", request.Preview.PackID), Details: []string{"Project installation verified separately from personal runtime activation"}, RuntimeActivation: "not required"}
		if capabilitypack.ProjectPackRequiresActivation(fresh.Lock, request.Preview.PackID, surface) {
			activation, activationErr := facade.PreviewProjectActivation(ctx, capabilitypack.ProjectActivationRequest{PackID: request.Preview.PackID, Surface: surface, ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: projectRuntimeAdapter(b.opts, surface, snapshot)})
			if activationErr != nil {
				result.RuntimeActivation = "unknown"
				result.PendingActions = append(result.PendingActions, "Reload project status before personal activation: "+activationErr.Error())
			} else if activation.Disposition == capabilitypack.ProjectActivationPreviewable {
				result.RuntimeActivation = "not yet activated"
				result.FollowUpOperation = "activate"
			} else if activation.Disposition == capabilitypack.ProjectActivationBlocked {
				result.RuntimeActivation = "blocked"
				result.PendingActions = append(result.PendingActions, "Personal project activation is blocked")
			} else if activation.Disposition == capabilitypack.ProjectActivationInheritedGlobal {
				result.RuntimeActivation = "inherited from global activation"
			} else if activation.Disposition == capabilitypack.ProjectActivationConverged {
				result.RuntimeActivation = "active"
			}
		}
		return result, nil
	case "update":
		adapter := projectInstallAdapter("", composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
		fresh, previewErr := facade.PreviewProjectUpdate(ctx, capabilitypack.ProjectUpdateRequest{PackID: request.Preview.PackID, ProjectRoot: projectRoot}, adapter)
		if previewErr != nil {
			return tui.ApplyResult{Stage: "revalidation"}, previewErr
		}
		freshView := projectPreviewForTUI(fresh, projectRoot, "update")
		if freshView.ID != request.Preview.ID || freshView.Digest != request.Preview.Digest {
			return tui.ApplyResult{Stage: "revalidation"}, errors.New("approved preview is stale; create a fresh preview before Apply")
		}
		if err := validateTUIApprovals(request.ApprovedPhases, freshView); err != nil {
			return tui.ApplyResult{Stage: "approval"}, err
		}
		progress(tui.ApplyProgress{Phase: "apply"})
		applied, applyErr := facade.ApplyProjectInstall(ctx, capabilitypack.ProjectInstallApplyRequest{
			Preview: fresh, PackyHome: snapshot.PackyHome(), Adapter: adapter,
			DestructiveCleanupApproved: slices.Contains(request.ApprovedPhases, "destructive-cleanup"),
		})
		if applyErr != nil {
			return tui.ApplyResult{Stage: "apply", Summary: "Project update stopped before verification"}, applyErr
		}
		progress(tui.ApplyProgress{Phase: "verification"})
		return tui.ApplyResult{Stage: "verification", Verified: applied.Status == "verified" || applied.Status == "no-op", Summary: fmt.Sprintf("Updated %s in the current project", request.Preview.PackID), Details: []string{"Reviewed project intent and personal runtime activation remain separate"}}, nil
	case "activate":
		adapter := projectRuntimeAdapter(b.opts, surface, snapshot)
		fresh, previewErr := facade.PreviewProjectActivation(ctx, capabilitypack.ProjectActivationRequest{PackID: request.Preview.PackID, Surface: surface, ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: adapter})
		if previewErr != nil {
			return tui.ApplyResult{Stage: "revalidation"}, previewErr
		}
		freshView := projectActivationPreviewForTUI(fresh, projectRoot)
		if freshView.ID != request.Preview.ID || freshView.Digest != request.Preview.Digest {
			return tui.ApplyResult{Stage: "revalidation"}, errors.New("approved preview is stale; create a fresh preview before Apply")
		}
		if err := validateTUIApprovals(request.ApprovedPhases, freshView); err != nil {
			return tui.ApplyResult{Stage: "approval"}, err
		}
		approvals := make([]capabilitypack.ProjectActivationApproval, 0, len(fresh.Categories))
		for _, category := range fresh.Categories {
			if category.ApprovalRequired {
				approvals = append(approvals, facade.ApproveProjectActivation(fresh, category.Kind))
			}
		}
		progress(tui.ApplyProgress{Phase: "apply"})
		applied, applyErr := facade.ApplyProjectActivation(ctx, capabilitypack.ProjectActivationApplyRequest{Preview: fresh, Approvals: approvals, Adapter: adapter, Interactive: true})
		if applyErr != nil {
			return tui.ApplyResult{Stage: "apply", Summary: "Personal project activation stopped before verification"}, applyErr
		}
		progress(tui.ApplyProgress{Phase: "verification"})
		return tui.ApplyResult{Stage: "verification", Verified: applied.Status == "active", Summary: fmt.Sprintf("Personally activated %s for the current project", request.Preview.PackID), Details: []string{"Project installation remains independently installed"}}, nil
	case "deactivate":
		adapter := projectRuntimeAdapter(b.opts, surface, snapshot)
		fresh, previewErr := capabilitypack.PreviewProjectDeactivation(ctx, capabilitypack.ProjectDeactivationRequest{
			PackID: request.Preview.PackID, Surface: surface, ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: adapter,
		})
		if previewErr != nil {
			return tui.ApplyResult{Stage: "revalidation"}, previewErr
		}
		freshView := projectDeactivationPreviewForTUI(fresh, projectRoot)
		if freshView.ID != request.Preview.ID || freshView.Digest != request.Preview.Digest {
			return tui.ApplyResult{Stage: "revalidation"}, errors.New("approved preview is stale; create a fresh preview before Apply")
		}
		if err := validateTUIApprovals(request.ApprovedPhases, freshView); err != nil {
			return tui.ApplyResult{Stage: "approval"}, err
		}
		progress(tui.ApplyProgress{Phase: "apply"})
		applied, applyErr := capabilitypack.ApplyProjectDeactivation(ctx, capabilitypack.ProjectDeactivationApplyRequest{Preview: fresh, Adapter: adapter, DestructiveCleanupApproved: true})
		if applyErr != nil {
			return tui.ApplyResult{Stage: "apply", Summary: "Personal project deactivation stopped before verification"}, applyErr
		}
		progress(tui.ApplyProgress{Phase: "verification"})
		return tui.ApplyResult{Stage: "verification", Verified: applied.Status == "inactive", Summary: fmt.Sprintf("Personally deactivated %s for the current project", request.Preview.PackID), Details: []string{"Project installation remains independently installed"}}, nil
	default:
		return tui.ApplyResult{Stage: "revalidation"}, fmt.Errorf("project Apply operation %q is unsupported", request.Preview.Operation)
	}
}

func (b *tuiBackend) requireCurrentProject(expectedRoot string) (workstation.Snapshot, string, error) {
	snapshot, err := b.resolver.Resolve(workstation.Options{})
	if err != nil {
		return workstation.Snapshot{}, "", err
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return workstation.Snapshot{}, "", fmt.Errorf("resolve the current Git project: %w", err)
	}
	projectRoot, err := capabilitypack.DiscoverProjectRoot(currentDirectory)
	if err != nil {
		return workstation.Snapshot{}, "", fmt.Errorf("project actions are unavailable outside the current Git project: %w", err)
	}
	currentIdentity, currentErr := filepath.EvalSymlinks(projectRoot)
	expectedIdentity, expectedErr := filepath.EvalSymlinks(expectedRoot)
	if currentErr != nil || expectedErr != nil || filepath.Clean(currentIdentity) != filepath.Clean(expectedIdentity) {
		return workstation.Snapshot{}, "", fmt.Errorf("project action target %q is not the current Git project %q", expectedRoot, projectRoot)
	}
	return snapshot, projectRoot, nil
}

func globalPlanForTUI(ctx context.Context, facade capabilitypack.Facade, operation, packID string, surface capabilitypack.Surface, selection tui.Selection) (capabilitypack.ReconciliationPlan, error) {
	switch operation {
	case string(capabilitypack.OperationActivate):
		selected, err := selectionForTUI(selection)
		if err != nil {
			return capabilitypack.ReconciliationPlan{}, err
		}
		return facade.Preview(ctx, capabilitypack.ActivationRequest{PackID: packID, Surface: surface, Selection: selected})
	case string(capabilitypack.OperationUpdate):
		return facade.PreviewUpdate(ctx, capabilitypack.UpdateRequest{PackID: packID, Surface: surface})
	case string(capabilitypack.OperationDeactivate):
		resources, err := resourceIdentitiesFromTUI(selection.Roots)
		if err != nil {
			return capabilitypack.ReconciliationPlan{}, err
		}
		return facade.PreviewDeactivate(ctx, capabilitypack.DeactivationRequest{PackID: packID, Surface: surface, Resources: resources})
	default:
		return capabilitypack.ReconciliationPlan{}, fmt.Errorf("preview operation %q is unsupported", operation)
	}
}

func lifecyclePastTense(operation string) string {
	switch operation {
	case string(capabilitypack.OperationUpdate):
		return "Updated"
	case string(capabilitypack.OperationDeactivate):
		return "Deactivated"
	default:
		return "Activated"
	}
}

func selectionForTUI(selection tui.Selection) (capabilitypack.ResourceSelection, error) {
	result := capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionMode(selection.Mode), Roots: []capabilitypack.ResourceIdentity{}}
	for _, value := range selection.Roots {
		identity, err := capabilitypack.ParseResourceIdentity(value)
		if err != nil {
			return capabilitypack.ResourceSelection{}, err
		}
		result.Roots = append(result.Roots, identity)
	}
	return result, nil
}

func resourceIdentitiesFromTUI(values []string) ([]capabilitypack.ResourceIdentity, error) {
	result := make([]capabilitypack.ResourceIdentity, 0, len(values))
	for _, value := range values {
		identity, err := capabilitypack.ParseResourceIdentity(value)
		if err != nil {
			return nil, err
		}
		result = append(result, identity)
	}
	return result, nil
}

func globalPreviewForTUI(report capabilitypack.JSONLifecyclePlan) tui.Preview {
	preview := tui.Preview{
		ID: report.PlanID, Digest: report.Digest, Operation: string(report.Operation), Disposition: string(report.Disposition),
		PackID: report.Pack, PackVersion: report.PackVersion, Surface: string(report.Surface), Scope: "global",
		Selection:      tui.Selection{Mode: string(report.Selection.Mode), Roots: resourceIdentitiesForTUI(report.Selection.Roots)},
		Diff:           tui.PreviewDiff{Added: report.ContractDiff.Added, Changed: report.ContractDiff.Changed, Removed: report.ContractDiff.Removed, Retained: report.ContractDiff.Retained},
		PendingActions: append([]string(nil), report.PendingHumanActions...),
	}
	for _, resource := range report.ResourceGraph.Resources {
		preview.Resources = append(preview.Resources, tui.PreviewResource{
			Identity: resource.Resource.String(), Role: string(resource.Role), DependencyChain: resourceIdentitiesForTUI(resource.DependencyChain),
		})
	}
	for _, origin := range report.SensitiveEffects {
		for _, authority := range origin.PromptAuthorities {
			preview.Authorities = append(preview.Authorities, tui.PreviewAuthority{Resource: origin.Resource.String(), Detail: "prompt authority " + authority})
		}
		for _, authority := range origin.RuntimeAuthorities {
			preview.Authorities = append(preview.Authorities, tui.PreviewAuthority{Resource: origin.Resource.String(), Detail: fmt.Sprintf("runtime authority %s (%s)", authority.Kind, authority.Scope)})
		}
		for _, effect := range origin.RuntimeEffects {
			preview.Authorities = append(preview.Authorities, tui.PreviewAuthority{Resource: origin.Resource.String(), Detail: fmt.Sprintf("runtime effect %s (%s)", effect.Kind, effect.Scope)})
		}
	}
	for _, action := range report.MandatoryActions {
		preview.Effects = append(preview.Effects, tui.PreviewEffect{Kind: string(action.Kind), Target: action.Target, Description: action.Description})
	}
	for _, blocker := range report.Blockers {
		preview.Blockers = append(preview.Blockers, tui.PreviewBlocker{Kind: string(blocker.Kind), Subject: blocker.Subject, Detail: blocker.Detail})
	}
	for _, phase := range report.Phases {
		view := tui.PreviewPhase{Kind: string(phase.Kind), ApprovalRequired: phase.ApprovalRequired}
		for _, action := range phase.Actions {
			view.Actions = append(view.Actions, strings.TrimSpace(string(action.Kind)+" "+action.Target+" "+action.Description))
		}
		preview.Phases = append(preview.Phases, view)
	}
	return preview
}

func projectPreviewForTUI(report capabilitypack.JSONProjectInstallPreview, projectRoot, operation string) tui.Preview {
	surface := string(report.Surface)
	if operation == "update" {
		surface = "all installed surfaces"
	}
	preview := tui.Preview{
		ID: report.Observation, Digest: report.Observation, Operation: operation, Disposition: string(report.Disposition),
		PackID: report.Pack.ID, PackVersion: report.Pack.Version, Surface: surface, Scope: "project",
		ProjectRoot:    projectRoot,
		Selection:      tui.Selection{Mode: string(report.Selection.Mode)},
		PendingActions: append([]string(nil), report.Requirements...),
	}
	coreEffects := []tui.PreviewEffect{
		{Kind: "project-manifest", Target: report.Manifest.Path, Description: fmt.Sprintf("write project Pack intent schema %d", report.Manifest.SchemaVersion)},
		{Kind: "project-lock", Target: report.Lock.Path, Description: fmt.Sprintf("write installed Pack receipts schema %d", report.Lock.SchemaVersion)},
		{Kind: "project-notices", Target: report.Notices.Path, Description: fmt.Sprintf("write %d legal notice contributions", len(report.Notices.Contributions))},
	}
	for _, effect := range coreEffects {
		if effect.Target == "" {
			continue
		}
		preview.Effects = append(preview.Effects, effect)
		if report.Disposition == capabilitypack.ProjectInstallConverged {
			preview.Diff.Retained = append(preview.Diff.Retained, effect.Target)
		} else {
			preview.Diff.Changed = append(preview.Diff.Changed, effect.Target)
		}
	}
	for _, resource := range report.Selection.Resources {
		preview.Resources = append(preview.Resources, tui.PreviewResource{
			Identity: resource.Resource.String(), Role: string(resource.Role), DependencyChain: resourceIdentitiesForTUI(resource.DependencyChain),
		})
		if report.Selection.Mode == capabilitypack.SelectionCustom && resource.Role == capabilitypack.ResourceRoleRoot {
			preview.Selection.Roots = append(preview.Selection.Roots, resource.Resource.String())
		}
	}
	for _, change := range report.SensitiveChanges {
		preview.Authorities = append(preview.Authorities, tui.PreviewAuthority{Resource: change.Resource.String(), Detail: string(change.Category) + " — " + change.Detail})
	}
	for _, projection := range report.Projections {
		kind := projection.Mode
		if kind == "" {
			kind = "project-projection"
		}
		preview.Effects = append(preview.Effects, tui.PreviewEffect{Kind: kind, Target: projection.Target, Description: "project projection for " + projection.Resource.String()})
		switch projection.ObservedState {
		case "missing":
			preview.Diff.Added = append(preview.Diff.Added, projection.Target)
		case "owned", "installed":
			preview.Diff.Retained = append(preview.Diff.Retained, projection.Target)
		default:
			preview.Diff.Changed = append(preview.Diff.Changed, projection.Target)
		}
	}
	for _, retirement := range report.Retirements {
		preview.Effects = append(preview.Effects, tui.PreviewEffect{Kind: retirement.Mode, Target: retirement.Target, Description: "retire project projection for " + retirement.Resource.String()})
		preview.Diff.Removed = append(preview.Diff.Removed, retirement.Target)
	}
	for _, blocker := range report.Blockers {
		preview.Blockers = append(preview.Blockers, tui.PreviewBlocker{Kind: blocker.Code, Subject: blocker.Resource.String(), Detail: blocker.Detail + "; " + blocker.Remediation})
	}
	actions := make([]string, 0, len(preview.Effects))
	destructiveActions := make([]string, 0, len(report.Retirements))
	for _, effect := range preview.Effects {
		action := strings.TrimSpace(effect.Kind + " " + effect.Target + " " + effect.Description)
		if strings.HasPrefix(effect.Description, "retire project projection") {
			destructiveActions = append(destructiveActions, action)
		} else {
			actions = append(actions, action)
		}
	}
	preview.Phases = []tui.PreviewPhase{{Kind: "project-" + operation, ApprovalRequired: report.Disposition == capabilitypack.ProjectInstallPreviewable, Actions: actions}}
	if len(destructiveActions) > 0 {
		preview.Phases = append(preview.Phases, tui.PreviewPhase{Kind: "destructive-cleanup", ApprovalRequired: report.Disposition == capabilitypack.ProjectInstallPreviewable, Actions: destructiveActions})
	}
	return preview
}

func projectDeactivationPreviewForTUI(report capabilitypack.JSONProjectDeactivationPreview, projectRoot string) tui.Preview {
	preview := tui.Preview{
		ID: report.Digest, Digest: report.Digest, Operation: "deactivate", Disposition: string(report.Disposition),
		PackID: report.Pack.ID, PackVersion: report.Pack.Version, Surface: string(report.Surface), Scope: "project", ProjectRoot: projectRoot,
		Selection: tui.Selection{Mode: "all"},
	}
	actions := make([]string, 0, len(report.Effects))
	for _, effect := range report.Effects {
		description := string(effect.Action) + " " + effect.Identity
		preview.Effects = append(preview.Effects, tui.PreviewEffect{Kind: "personal-runtime", Target: effect.Target, Description: description})
		preview.Diff.Removed = append(preview.Diff.Removed, effect.Target)
		actions = append(actions, strings.TrimSpace("personal-runtime "+effect.Target+" "+description))
	}
	for _, blocker := range report.Blockers {
		preview.Blockers = append(preview.Blockers, tui.PreviewBlocker{Kind: blocker.Code, Subject: blocker.Resource.String(), Detail: blocker.Detail + "; " + blocker.Remediation})
	}
	preview.Phases = []tui.PreviewPhase{{Kind: "destructive-cleanup", ApprovalRequired: report.Disposition == capabilitypack.ProjectDeactivationPreviewable, Actions: actions}}
	return preview
}

func projectActivationPreviewForTUI(report capabilitypack.JSONProjectActivationPreview, projectRoot string) tui.Preview {
	preview := tui.Preview{
		ID: report.Digest, Digest: report.Digest, Operation: "activate", Disposition: string(report.Disposition),
		PackID: report.Pack.ID, PackVersion: report.Pack.Version, Surface: string(report.Surface), Scope: "project", ProjectRoot: projectRoot,
		Selection: tui.Selection{Mode: "all"},
	}
	for _, category := range report.Categories {
		phase := tui.PreviewPhase{Kind: string(category.Kind), ApprovalRequired: category.ApprovalRequired}
		for _, detail := range category.Details {
			action := detail.Resource.String() + " — " + detail.Detail
			phase.Actions = append(phase.Actions, action)
			preview.Authorities = append(preview.Authorities, tui.PreviewAuthority{Resource: detail.Resource.String(), Detail: string(category.Kind) + " — " + detail.Detail})
		}
		preview.Phases = append(preview.Phases, phase)
	}
	for _, effect := range report.Effects {
		preview.Effects = append(preview.Effects, tui.PreviewEffect{Kind: string(effect.Category), Target: effect.Target, Description: string(effect.Action) + " " + effect.Identity})
	}
	for _, effect := range report.RuntimeEffects {
		if effect.Conflict != "" {
			preview.Blockers = append(preview.Blockers, tui.PreviewBlocker{Kind: "runtime-conflict", Subject: effect.Resource.String(), Detail: effect.Conflict})
		}
	}
	return preview
}

func validateTUIApprovals(approved []string, preview tui.Preview) error {
	required := make([]string, 0)
	for _, phase := range preview.Phases {
		if phase.ApprovalRequired {
			required = append(required, phase.Kind)
		}
	}
	if !slices.Equal(approved, required) {
		return fmt.Errorf("approved effect classes %v do not match required classes %v", approved, required)
	}
	return nil
}

func resourceIdentitiesForTUI(resources []capabilitypack.ResourceIdentity) []string {
	result := make([]string, 0, len(resources))
	for _, resource := range resources {
		result = append(result, resource.String())
	}
	return result
}

func healthForTUI(report setuphealth.Report) tui.Health {
	health := tui.Health{
		Status: report.Summary.Status, Passes: report.Summary.Passes,
		Warnings: report.Summary.Warnings, Failures: report.Summary.Failures,
		Checks: make([]tui.HealthCheck, 0, len(report.Checks)),
	}
	for _, check := range report.Checks {
		health.Checks = append(health.Checks, tui.HealthCheck{Name: check.Name, Severity: string(check.Severity), Detail: check.Detail})
	}
	return health
}

func catalogPacksForTUI(details []capabilitypack.CatalogDetail, statuses map[string]map[string]tui.SurfaceStatus) []tui.Pack {
	result := make([]tui.Pack, 0, len(details))
	for _, detail := range details {
		pack := detail.Pack
		view := tui.Pack{
			ID: pack.ID, Version: pack.Version, Description: pack.Description,
			Requirements: append([]string(nil), pack.Requires.Tools...),
			Resources:    resourcesForTUI(detail),
			Exclusions:   exclusionsForTUI(pack),
		}
		for _, surface := range capabilitypack.SupportedSurfaces() {
			supported := slices.Contains(pack.Surfaces, surface)
			status := tui.SurfaceStatus{Name: string(surface), Supported: supported}
			if supported {
				status.Configured, status.Authorized, status.Usable = "no", "no", "no"
				if observed, ok := statuses[pack.ID][string(surface)]; ok {
					status = observed
					status.Supported = true
					if status.InstalledVersion != "" {
						status.UpdateAvailable = status.Installation == string(capabilitypack.ProjectInstallationInstalled) && status.InstalledVersion != pack.Version
					}
				}
				view.Surfaces = append(view.Surfaces, string(surface))
			}
			view.SurfaceStatuses = append(view.SurfaceStatuses, status)
		}
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func resourcesForTUI(detail capabilitypack.CatalogDetail) []tui.Resource {
	raw := make(map[string]capabilitypack.Resource, len(detail.Pack.Resources))
	for _, resource := range detail.Pack.Resources {
		raw[resource.Kind+":"+resource.ID] = resource
	}
	result := make([]tui.Resource, 0, len(detail.ResourceInventory))
	for _, resource := range detail.ResourceInventory {
		identity := resource.Resource.String()
		requirements := make([]string, 0, len(resource.Dependencies)+len(resource.Notices))
		for _, dependency := range resource.Dependencies {
			requirements = append(requirements, dependency.String())
		}
		for _, notice := range resource.Notices {
			requirements = append(requirements, notice.String())
		}
		manifestResource := raw[identity]
		requirements = append(requirements, manifestResource.RequiresTools...)
		result = append(result, tui.Resource{
			Identity: identity, Description: resource.Description, Role: string(resource.Role),
			Requirements: requirements, Conflicts: append([]string(nil), manifestResource.Conflicts...),
		})
	}
	return result
}

func exclusionsForTUI(pack capabilitypack.Pack) []tui.Exclusion {
	result := make([]tui.Exclusion, 0, len(pack.Contract.Exclusions))
	for _, exclusion := range pack.Contract.Exclusions {
		result = append(result, tui.Exclusion{ID: exclusion.ID, Reason: exclusion.Reason})
	}
	for _, resource := range pack.Resources {
		for _, exclusion := range resource.SurfaceExclusions {
			result = append(result, tui.Exclusion{
				ID: resource.Kind + ":" + resource.ID, Surface: string(exclusion.Surface),
				Mode: exclusion.Mode, Code: exclusion.Code, Reason: exclusion.Reason,
			})
		}
	}
	return result
}

func globalStatusesForTUI(report capabilitypack.StatusReport) map[string]map[string]tui.SurfaceStatus {
	result := make(map[string]map[string]tui.SurfaceStatus)
	for _, entry := range report.Entries {
		status := tui.SurfaceStatus{
			Name: string(entry.Surface), Supported: true,
			Active: entry.IntentPresent && entry.Intent.Active, UpdateAvailable: entry.UpdateActionAvailable,
			Configured: readinessForTUI(entry.ReadinessObserved.Configured, entry.Readiness.Configured),
			Authorized: readinessForTUI(entry.ReadinessObserved.Authorization, entry.Readiness.Authorized),
			Usable:     readinessForTUI(entry.ReadinessObserved.Usability, entry.Readiness.Usable),
			Blockers:   append([]string(nil), entry.Blockers...), PendingActions: append([]string(nil), entry.PendingHumanActions...), Evidence: append([]string(nil), entry.Evidence...),
		}
		for _, projection := range entry.ProjectionDetails {
			if projection.Owner == "packy" {
				status.Ownership++
				if projection.Health != capabilitypack.ProjectionVerified {
					status.Drift++
				}
			}
		}
		if result[entry.Pack.ID] == nil {
			result[entry.Pack.ID] = make(map[string]tui.SurfaceStatus)
		}
		result[entry.Pack.ID][string(entry.Surface)] = status
	}
	return result
}

func projectStatusesForTUI(report capabilitypack.JSONProjectStatusReport) map[string]map[string]tui.SurfaceStatus {
	result := make(map[string]map[string]tui.SurfaceStatus)
	for _, entry := range report.Packs {
		status := tui.SurfaceStatus{
			Name: string(entry.Surface), Supported: true,
			Installation: string(entry.Installation), Runtime: string(entry.Runtime), Active: entry.Runtime == capabilitypack.ProjectRuntimeActive || entry.Runtime == capabilitypack.ProjectRuntimeInheritedGlobal,
			InstalledVersion: entry.Pack.Version,
			Configured:       yesNo(entry.Readiness.Configured), Authorized: yesNo(entry.Readiness.Authorized), Usable: yesNo(entry.Readiness.Usable),
			Ownership: len(entry.Projections), PendingActions: append([]string(nil), entry.PendingHumanActions...), Evidence: append([]string(nil), entry.Evidence...),
		}
		for _, blocker := range entry.Blockers {
			status.Blockers = append(status.Blockers, blocker.Code+": "+blocker.Detail)
		}
		for _, projection := range entry.Projections {
			if projection.Health != "verified" {
				status.Drift++
			}
		}
		if result[entry.Pack.ID] == nil {
			result[entry.Pack.ID] = make(map[string]tui.SurfaceStatus)
		}
		result[entry.Pack.ID][string(entry.Surface)] = status
	}
	return result
}

func readinessForTUI(observed, value bool) string {
	if !observed {
		return "unknown"
	}
	return yesNo(value)
}
