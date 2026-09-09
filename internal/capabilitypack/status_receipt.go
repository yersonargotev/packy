package capabilitypack

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// receiptStatusEntry observes installed evidence without consulting an unavailable
// historical manifest. A current catalog entry describes only the update target.
func (f Facade) receiptStatusEntry(ctx context.Context, entry StatusEntry, intent ActivationIntent, state ActivationState, adapter SurfaceAdapter) (StatusEntry, error) {
	matches := 0
	for _, active := range activeIntents(state) {
		if active.PackID == intent.PackID && active.Surface == intent.Surface {
			matches++
		}
	}
	if matches != 1 {
		return StatusEntry{}, fmt.Errorf("installed receipt identity is duplicated or missing")
	}
	owners, err := statusReceiptOwnership(intent, state.Ownership)
	if err != nil {
		return StatusEntry{}, err
	}
	entry.Contract = LifecycleContract{}
	entry.HistoricalEvidence = HistoricalEvidenceStatus{Message: fmt.Sprintf("Pack %s@%s manifest is unavailable; installed projection and readiness policy evidence is retained, but resource, dependency, contract, and runtime semantics are unavailable", intent.PackID, intent.Version)}
	entry.LifecycleState = PackLifecycleActive
	entry.UpdateActionAvailable = slices.Contains(entry.Pack.Surfaces, intent.Surface)
	entry.ControlledCheck = ControlledCheckStatus{State: ControlledCheckUnknown}
	policy := Pack{ID: intent.PackID, Version: intent.Version, ReadinessObligations: append([]ReadinessObligation{}, intent.ReadinessObligations...), Requires: Requirements{Tools: append([]string{}, intent.ExternalRequirements...)}}
	resolutions, resolveErr := f.resolveExecutables(ctx, policy, intent.Surface, false)
	unobserved := []string{}
	if resolveErr != nil {
		unobserved = append(unobserved, intent.ExternalRequirements...)
		entry.Blockers = append(entry.Blockers, resolveErr.Error())
	}
	for _, resolution := range resolutions {
		if !resolution.Available {
			entry.MissingRequirements = append(entry.MissingRequirements, resolution.Tool)
			entry.Blockers = append(entry.Blockers, fmt.Sprintf("required executable %s is missing", resolution.Tool))
			entry.PendingHumanActions = append(entry.PendingHumanActions, fmt.Sprintf("install %s and rerun status; Packy will not install it during Status", resolution.Tool))
		}
	}
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{ObservationOnly: true, ReceiptOwnership: owners, CurrentOwnership: adapterOwnershipForSurface(state.Ownership, intent.Surface)})
	if err != nil {
		return StatusEntry{}, err
	}
	if len(observation.Projections) != len(owners) {
		return StatusEntry{}, fmt.Errorf("receipt inspection omitted or added an installed projection")
	}
	byID := make(map[string]ProjectionOwnership, len(owners))
	for _, owner := range owners {
		byID[owner.ID] = owner
	}
	for _, p := range observation.Projections {
		owner, ok := byID[p.ID]
		if !ok || p.Action.Target != owner.Target || p.DesiredFingerprint != owner.Fingerprint || p.Goal != ProjectionPresent {
			return StatusEntry{}, fmt.Errorf("receipt inspection changed installed projection evidence for %q", p.ID)
		}
		detail := ProjectionStatus{ID: owner.ID, Target: portableProjectionTarget(owner.Target), DesiredFingerprint: owner.Fingerprint, ObservedFingerprint: p.ObservedFingerprint, Owner: "packy", Health: ProjectionVerified}
		switch {
		case !p.Exists:
			detail.Health = ProjectionMissing
		case p.ObservedFingerprint != owner.Fingerprint:
			detail.Health = ProjectionDrifted
		}
		entry.ProjectionDetails = append(entry.ProjectionDetails, detail)
		addProjectionHealth(&entry.Projections, detail.Health)
		entry.Evidence = append(entry.Evidence, fmt.Sprintf("%s: %s observed=%s desired=%s target=%s", detail.ID, detail.Health, detail.ObservedFingerprint, detail.DesiredFingerprint, detail.Target))
		if detail.Health != ProjectionVerified {
			entry.Blockers = append(entry.Blockers, fmt.Sprintf("%s is %s", detail.ID, detail.Health))
		}
	}
	// Historical host semantics and controlled-check validity cannot be inferred
	// from a receipt. Persisted obligations remain authoritative, with no runtime
	// observations imported from the current manifest or adapter resource graph.
	entry.Readiness, entry.Conditions = evaluateReadiness(readinessEvaluation{Pack: policy, Surface: intent.Surface, Scope: ReadinessScopeGlobal, Projections: entry.ProjectionDetails, Resolutions: resolutions, UnobservedRequirements: unobserved, Revision: observation.Revision, ObservedAt: f.observationTime()})
	sort.Strings(entry.Evidence)
	sort.Strings(entry.Blockers)
	sort.Strings(entry.MissingRequirements)
	sort.Strings(entry.PendingHumanActions)
	return entry, nil
}

func statusReceiptOwnership(intent ActivationIntent, ownership []ProjectionOwnership) ([]ProjectionOwnership, error) {
	invalid := func() ([]ProjectionOwnership, error) {
		return nil, fmt.Errorf("installed receipt for %s on %s contains invalid identity, selection, readiness, or ownership evidence", intent.PackID, intent.Surface)
	}
	if !idPattern.MatchString(intent.PackID) || !validSemver(intent.Version) || intent.Revision < 0 {
		return invalid()
	}
	if len(intent.ReadinessObligations) > 0 && !validReadinessObligations(intent.ReadinessObligations) {
		return invalid()
	}
	if !sort.StringsAreSorted(intent.ExternalRequirements) || hasDuplicateStrings(intent.ExternalRequirements) {
		return invalid()
	}
	for _, requirement := range intent.ExternalRequirements {
		if !idPattern.MatchString(requirement) {
			return invalid()
		}
	}
	resources := map[ResourceIdentity]bool{}
	for _, resource := range intent.Resources {
		if _, err := ParseResourceIdentity(resource.String()); err != nil || resources[resource] {
			return invalid()
		}
		resources[resource] = true
	}
	if len(resources) == 0 {
		return invalid()
	}
	selection, err := canonicalSelection(intent.Selection)
	if err != nil || len(selection.Roots) != len(intent.Selection.Roots) {
		return invalid()
	}
	for _, root := range selection.Roots {
		if !resources[root] {
			return invalid()
		}
	}
	result := []ProjectionOwnership{}
	seen := map[string]bool{}
	physical := map[string]bool{}
	for _, owner := range ownership {
		if owner.PackID != intent.PackID || owner.Surface != intent.Surface {
			continue
		}
		if owner.ProjectionID == "" || owner.Target == "" || !projectDigestPattern.MatchString(strings.TrimPrefix(owner.Fingerprint, "sha256:")) {
			return invalid()
		}
		if owner.ID != receiptProjectionOwnershipID(owner.Surface, installedProjection{ID: owner.ProjectionID, Target: owner.Target}) {
			return invalid()
		}
		if seen[owner.ProjectionID] || physical[owner.ID] || filepath.Clean(owner.Target) != owner.Target {
			return invalid()
		}
		for _, other := range ownership {
			if other.ID == owner.ID && (other.PackID != owner.PackID || other.Surface != owner.Surface) {
				return invalid()
			}
		}
		seen[owner.ProjectionID], physical[owner.ID] = true, true
		owner.PhysicalID, owner.ID = owner.ID, owner.ProjectionID
		result = append(result, owner)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
