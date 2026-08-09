package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
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
		selection, err := selectionForTUI(request.Selection)
		if err != nil {
			return tui.Preview{}, err
		}
		if request.ProjectRoot == "" {
			return tui.Preview{}, errors.New("project preview requires the current project root")
		}
		composition, err := resolvePackComposition(b.opts, b.resolver)
		if err != nil {
			return tui.Preview{}, err
		}
		facade := capabilitypack.NewFacade(composition.catalog, capabilitypack.WithActivation(capabilitypack.NewFileActivationStore(composition.state.File()), nil))
		adapter := projectInstallAdapter(surface, composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
		preview, err := facade.PreviewProjectInstall(ctx, capabilitypack.ProjectInstallRequest{
			PackID: request.PackID, Surface: surface, ProjectRoot: request.ProjectRoot, Selection: selection,
		}, adapter)
		if err != nil {
			return tui.Preview{}, err
		}
		return projectPreviewForTUI(preview), nil
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
	if request.Preview.Scope != "global" {
		return tui.ApplyResult{Stage: "revalidation"}, errors.New("TUI Apply supports only global Pack lifecycle operations")
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

func projectPreviewForTUI(report capabilitypack.JSONProjectInstallPreview) tui.Preview {
	preview := tui.Preview{
		ID: report.Observation, Digest: report.Observation, Operation: "install", Disposition: string(report.Disposition),
		PackID: report.Pack.ID, PackVersion: report.Pack.Version, Surface: string(report.Surface), Scope: "project",
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
	actions := []string{"project manifest " + report.Manifest.Path, "project lock " + report.Lock.Path, "project notices " + report.Notices.Path}
	preview.Phases = []tui.PreviewPhase{{Kind: "project-install", ApprovalRequired: report.Disposition == capabilitypack.ProjectInstallPreviewable, Actions: actions}}
	return preview
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
			Configured: yesNo(entry.Readiness.Configured), Authorized: yesNo(entry.Readiness.Authorized), Usable: yesNo(entry.Readiness.Usable),
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
