package codex

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

type ProjectSurfaceAdapter struct{ bundleRoot string }

func NewProjectSurfaceAdapter(bundleRoot string) *ProjectSurfaceAdapter {
	return &ProjectSurfaceAdapter{bundleRoot: bundleRoot}
}

func (a *ProjectSurfaceAdapter) InspectProject(_ context.Context, pack capabilitypack.Pack, projectRoot string) (capabilitypack.ProjectSurfaceObservation, error) {
	projections := make([]capabilitypack.ProjectProjectionObservation, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		bindingName, bound := codexProjectBinding(resource)
		if !bound || resource.Kind != "skill" || resource.Source == "" {
			projections = append(projections, capabilitypack.ProjectProjectionObservation{Resource: identity, Representable: false, Reason: fmt.Sprintf("%s has no Codex project-native representation in this installation preview", identity)})
			continue
		}
		target := filepath.Join(projectRoot, ".agents", "skills", bindingName)
		relative, err := capabilitypack.RelativeProjectTarget(projectRoot, target)
		if err != nil {
			return capabilitypack.ProjectSurfaceObservation{}, err
		}
		desired, err := localprojection.FingerprintTree(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ProjectSurfaceObservation{}, fmt.Errorf("fingerprint %s source: %w", identity, err)
		}
		observed, exists, err := localprojection.FingerprintPath(target)
		if err != nil {
			return capabilitypack.ProjectSurfaceObservation{}, fmt.Errorf("inspect %s target: %w", identity, err)
		}
		projections = append(projections, capabilitypack.ProjectProjectionObservation{Resource: identity, Target: relative, Mode: "copy_tree", DesiredFingerprint: desired, ObservedFingerprint: observed, Exists: exists, Representable: true})
	}
	return capabilitypack.ProjectSurfaceObservation{Revision: capabilitypack.ProjectObservationRevision(projections), Projections: projections}, nil
}

func codexProjectBinding(resource capabilitypack.Resource) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceCodex && binding.Name != "" {
			return binding.Name, true
		}
	}
	return "", false
}
