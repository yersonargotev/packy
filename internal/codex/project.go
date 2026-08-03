package codex

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func (a *SurfaceAdapter) inspectProject(_ context.Context, pack capabilitypack.Pack, projectRoot string) (capabilitypack.SurfaceInspection, error) {
	projections := make([]capabilitypack.ObservedProjection, 0, len(pack.Resources))
	unrepresentable := make([]capabilitypack.UnrepresentableResource, 0)
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
			continue
		}
		bindingName, bound := codexProjectBinding(resource)
		if !bound || resource.Kind != "skill" || resource.Source == "" {
			unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no Codex project-native representation in this installation preview", identity)})
			continue
		}
		target := filepath.Join(projectRoot, ".agents", "skills", bindingName)
		source := filepath.Join(a.bundleRoot, resource.Source)
		desired, err := localprojection.FingerprintExactTree(source)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("fingerprint %s source: %w", identity, err)
		}
		observed, exists, err := localprojection.FingerprintPath(target)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("inspect %s target: %w", identity, err)
		}
		projections = append(projections, capabilitypack.ObservedProjection{
			ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists,
			ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "codex-project/v1/copied-skill-tree",
			Action: capabilitypack.ProjectionAction{ID: identity.String(), Kind: capabilitypack.ActionCodexProjectSkillTree, Source: source, Target: target, Description: fmt.Sprintf("copy %s to the Codex project skill tree", identity), PreviewOnly: true},
		})
	}
	evidence, err := capabilitypack.UnverifiedRuntimeModeEvidence(pack, time.Unix(0, 0).UTC(), "project-install-preview")
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	return capabilitypack.SurfaceInspection{
		Revision: localprojection.FingerprintBytes([]byte(projectRoot)), Projections: projections, Unrepresentable: unrepresentable,
		Readiness: capabilitypack.ReadinessObservation{OptionalAuthorities: capabilitypack.UnknownOptionalAuthorities(pack)}, RuntimeModeEvidence: evidence,
	}, nil
}

func codexProjectBinding(resource capabilitypack.Resource) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceCodex && binding.Name != "" {
			return binding.Name, true
		}
	}
	return "", false
}
