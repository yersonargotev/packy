package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

type projectSurfaceAdapterSet struct {
	adapters       map[Surface]SurfaceAdapter
	contractWriter Surface
}

// NewProjectSurfaceAdapterSet composes installed project hosts while keeping
// each projection action owned by the adapter that produced it.
func NewProjectSurfaceAdapterSet(adapters map[Surface]SurfaceAdapter, contractWriter Surface) SurfaceAdapter {
	cloned := make(map[Surface]SurfaceAdapter, len(adapters))
	for surface, adapter := range adapters {
		cloned[surface] = adapter
	}
	return projectSurfaceAdapterSet{adapters: cloned, contractWriter: contractWriter}
}

func (a projectSurfaceAdapterSet) InspectSurface(ctx context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	if transition.ProjectInstallation == nil || len(transition.ProjectInstallation.Manifest.Packs) != 1 {
		return SurfaceInspection{}, errors.New("mixed project surface inspection requires one locked project installation")
	}
	var combined SurfaceInspection
	seen := map[string]bool{}
	var revisions []string
	for _, surface := range transition.ProjectInstallation.Manifest.Packs[0].Surfaces {
		adapter := a.adapters[surface]
		if adapter == nil {
			return SurfaceInspection{}, errors.New("mixed project surface inspection is missing an installed surface adapter")
		}
		inspection, err := inspectSurface(ctx, adapter, transition)
		if err != nil {
			return SurfaceInspection{}, err
		}
		revisions = append(revisions, string(surface)+"="+inspection.Revision)
		for _, projection := range inspection.Projections {
			key := projection.ID + "\x00" + filepath.Clean(projection.Action.Target)
			if !seen[key] {
				combined.Projections = append(combined.Projections, projection)
				seen[key] = true
			}
		}
	}
	sort.Strings(revisions)
	combined.Revision = strings.Join(revisions, "\n")
	return combined, nil
}

func (a projectSurfaceAdapterSet) ApplyProjections(ctx context.Context, actions []ProjectionAction) *ProjectionActionError {
	grouped := map[Surface][]ProjectionAction{}
	for _, action := range actions {
		surface := action.Surface
		if surface == "" {
			surface = a.contractWriter
		}
		grouped[surface] = append(grouped[surface], action)
	}
	surfaces := make([]Surface, 0, len(grouped))
	for surface := range grouped {
		surfaces = append(surfaces, surface)
	}
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i] < surfaces[j] })
	for _, surface := range surfaces {
		if len(grouped[surface]) == 0 {
			continue
		}
		adapter := a.adapters[surface]
		if adapter == nil {
			return &ProjectionActionError{ID: grouped[surface][0].ID, Err: errors.New("mixed project projection apply is missing its surface adapter")}
		}
		if err := adapter.ApplyProjections(ctx, grouped[surface]); err != nil {
			return err
		}
	}
	return nil
}
