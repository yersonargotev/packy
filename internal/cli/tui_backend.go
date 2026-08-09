package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/tui"
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
	opts     Options
	resolver *workstation.Resolver
}

func newTUIBackend(opts Options, resolver *workstation.Resolver) *tuiBackend {
	return &tuiBackend{opts: opts, resolver: resolver}
}

func (b *tuiBackend) Load(ctx context.Context) (tui.Dashboard, error) {
	health, err := diagnoseSetupHealth(ctx, b.opts, b.resolver)
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("diagnose Packy health: %w", err)
	}
	catalog, err := discoverPackCatalog(b.opts, b.resolver)
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("discover reviewed Pack catalog: %w", err)
	}
	packs, err := catalog.ListCurrent()
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("load reviewed Pack catalog: %w", err)
	}

	dashboard := tui.Dashboard{
		Health: healthForTUI(health),
		Global: tui.Scope{Available: true, Packs: packsForTUI(packs)},
	}
	snapshot, err := b.resolver.Resolve(workstation.Options{})
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("resolve workstation context: %w", err)
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("resolve current directory: %w", err)
	}
	projectRoot, err := capabilitypack.DiscoverProjectRoot(currentDirectory)
	if err != nil {
		var absent capabilitypack.ProjectNotFoundError
		if errors.As(err, &absent) {
			return dashboard, nil
		}
		return tui.Dashboard{}, fmt.Errorf("discover current project: %w", err)
	}

	request := capabilitypack.ProjectStatusRequest{
		ProjectRoot: projectRoot,
		PackyHome:   snapshot.PackyHome(),
		Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
			capabilitypack.SurfaceClaude:   projectRuntimeAdapter(b.opts, capabilitypack.SurfaceClaude, snapshot),
			capabilitypack.SurfaceCodex:    projectRuntimeAdapter(b.opts, capabilitypack.SurfaceCodex, snapshot),
			capabilitypack.SurfaceOpenCode: projectRuntimeAdapter(b.opts, capabilitypack.SurfaceOpenCode, snapshot),
		},
	}
	status, err := capabilitypack.InspectProjectStatus(ctx, request)
	if err != nil {
		return tui.Dashboard{}, fmt.Errorf("inspect current project: %w", err)
	}
	dashboard.Project = tui.Scope{Available: true, Root: projectRoot, Packs: projectPacksForTUI(status)}
	return dashboard, nil
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

func packsForTUI(packs []capabilitypack.Pack) []tui.Pack {
	result := make([]tui.Pack, 0, len(packs))
	for _, pack := range packs {
		surfaces := make([]string, len(pack.Surfaces))
		for index, surface := range pack.Surfaces {
			surfaces[index] = string(surface)
		}
		result = append(result, tui.Pack{ID: pack.ID, Version: pack.Version, Description: pack.Description, Surfaces: surfaces})
	}
	return result
}

func projectPacksForTUI(report capabilitypack.JSONProjectStatusReport) []tui.Pack {
	byID := make(map[string]tui.Pack)
	for _, status := range report.Packs {
		pack := byID[status.Pack.ID]
		pack.ID, pack.Version = status.Pack.ID, status.Pack.Version
		pack.Surfaces = append(pack.Surfaces, string(status.Surface))
		byID[pack.ID] = pack
	}
	result := make([]tui.Pack, 0, len(byID))
	for _, pack := range byID {
		sort.Strings(pack.Surfaces)
		result = append(result, pack)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
