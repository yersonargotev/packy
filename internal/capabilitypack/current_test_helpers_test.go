package capabilitypack

import (
	"context"
	"errors"
)

type surfaceInspectionCall struct {
	kind        string
	prior       Pack
	desired     Pack
	ownership   []ProjectionOwnership
	resolutions []ExecutableResolution
}

type fakeSurfaceAdapter struct {
	observations []SurfaceInspection
	readiness    []ReadinessObservation
	calls        []surfaceInspectionCall
	applied      [][]ProjectionAction
	inspectCalls int
	actions      []ProjectionAction
	events       *[]string
	applyErr     error
	inspect      func(SurfaceTransition) SurfaceInspection
}

func (f *fakeSurfaceAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	f.inspectCalls++
	f.calls = append(f.calls, surfaceInspectionCall{prior: transition.Prior, desired: transition.Desired, ownership: cloneOwnership(transition.ResidualOwnership), resolutions: cloneResolutions(transition.ResolvedExecutables)})
	var result SurfaceInspection
	if f.inspect != nil {
		result = f.inspect(transition)
	} else if len(f.observations) != 0 {
		index := f.inspectCalls - 1
		if index >= len(f.observations) {
			index = len(f.observations) - 1
		}
		result = f.observations[index]
	}
	if index := f.inspectCalls - 1; len(f.readiness) != 0 {
		if index >= len(f.readiness) {
			index = len(f.readiness) - 1
		}
		result.Readiness = f.readiness[index]
	}
	for i := range result.Projections {
		projection := &result.Projections[i]
		if projection.Action.ID == "" {
			projection.Action.ID = projection.ID
		}
		if projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent || ((transition.Prior.ID != "" || transition.ResidualOwnership != nil) && (projection.DesiredFingerprint == "" || projection.DesiredFingerprint == "missing")) {
			projection.Goal = ProjectionAbsent
			projection.DesiredFingerprint = ""
			if projection.Action.Mode == "" {
				projection.Action.Mode = ProjectionDeleteTarget
			}
		} else if projection.Goal == "" {
			projection.Goal = ProjectionPresent
		}
	}
	return result, nil
}

func (f *fakeSurfaceAdapter) ApplyProjections(_ context.Context, actions []ProjectionAction) *ProjectionActionError {
	f.actions = append(f.actions, actions...)
	f.applied = append(f.applied, append([]ProjectionAction(nil), actions...))
	if f.applyErr == nil {
		return nil
	}
	var actionErr ProjectionActionError
	if errors.As(f.applyErr, &actionErr) {
		return &actionErr
	}
	return &ProjectionActionError{ID: actions[0].ID, Err: f.applyErr}
}

type fakeActivationStore struct {
	state  ActivationState
	events *[]string
	saves  []ActivationState
}

func (f *fakeActivationStore) LoadSnapshot(context.Context, Surface) (ActivationState, error) {
	return cloneActivationState(f.state), nil
}

func (f *fakeActivationStore) SaveSnapshot(_ context.Context, _ Surface, expectedRevision int, state ActivationState) (int, error) {
	if f.state.documentRevision != expectedRevision {
		return f.state.documentRevision, ErrStalePlan
	}
	state.documentRevision = expectedRevision + 1
	f.state = cloneActivationState(state)
	f.saves = append(f.saves, cloneActivationState(state))
	return state.documentRevision, nil
}

func testCapabilityBindings(name string) []Binding {
	return []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: name, Mode: "native", Sharing: "exclusive"}}
}

func updateFixture(packs []Pack, state ActivationState, observations ...SurfaceInspection) (Facade, *fakeSurfaceAdapter, *fakeActivationStore) {
	adapter := &fakeSurfaceAdapter{observations: observations}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	return facade, adapter, store
}

func deactivationFixture(packs []Pack, state ActivationState, observations ...SurfaceInspection) (Facade, *fakeSurfaceAdapter, *fakeActivationStore) {
	adapter := &fakeSurfaceAdapter{observations: observations}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	return facade, adapter, store
}
