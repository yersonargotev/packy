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

// RunTUI composes the read-only terminal application from Packy's production
// owners. Root-command activation is deliberately owned by a later increment.
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
