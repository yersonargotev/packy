package capabilitypack

import "sort"

type ActivationRole string
type BlockerKind string

const (
	ActivationRequested        ActivationRole = "requested"
	ActivationExplicit         ActivationRole = "explicit"
	ActivationRequired         ActivationRole = "required"
	ActivationExplicitRequired ActivationRole = "explicit-required"
	ActivationInactive         ActivationRole = "inactive"

	BlockerIncompatibleContribution BlockerKind = "incompatible-contribution"
	BlockerOwnership                BlockerKind = "ownership"
	BlockerGlobalRequirement        BlockerKind = "global-requirement"
	BlockerAlias                    BlockerKind = "alias"
	BlockerSharing                  BlockerKind = "sharing"
	BlockerCompatibility            BlockerKind = "compatibility"
	BlockerResourceConflict         BlockerKind = "resource-conflict"
	BlockerSelectionUnavailable     BlockerKind = "selection-unavailable"
	BlockerTargetCollision          BlockerKind = "target-collision"
)

type PlannedActivation struct {
	Pack      Pack
	Role      ActivationRole
	Selection ResourceSelection
}

type PlanBlocker struct {
	Kind    BlockerKind
	Subject string
	Detail  string
}

type composition struct {
	requested   Pack
	surface     Surface
	packs       []Pack
	activations []PlannedActivation
	blockers    []PlanBlocker
	intentFacts []ActivationIntent
}

func (c composition) combinedPack() Pack {
	if len(c.packs) == 0 {
		return Pack{ID: c.requested.ID, Version: c.requested.Version, Surfaces: []Surface{c.surface}, Resources: []Resource{}, Requires: Requirements{Tools: []string{}}}
	}
	return clonePack(c.packs[0])
}

func (f Facade) compose(requested Pack, state ActivationState, surface Surface, useRequestedIntent bool) (composition, error) {
	return f.composeProject(requested, state, surface, nil)
}

func (f Facade) composeProject(requested Pack, state ActivationState, surface Surface, projectAliases []SurfaceAlias) (composition, error) {
	selection := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
	aliases := cloneAliases(projectAliases)
	var intentFacts []ActivationIntent
	if intent, ok := intentForPack(state, requested.ID, surface); ok {
		selection = cloneSelection(intent.Selection)
		if projectAliases == nil {
			aliases = cloneAliases(intent.Aliases)
		}
		intentFacts = []ActivationIntent{intent}
	}
	selected, err := selectPackResources(requested, selection)
	if err != nil {
		return composition{}, err
	}
	for i := range selected.Resources {
		selected.Resources[i] = resourceWithSurfaceAlias(selected.Resources[i], aliases, surface)
	}
	result := composition{
		requested:   requested,
		surface:     surface,
		packs:       []Pack{selected},
		activations: []PlannedActivation{{Pack: selected, Role: ActivationRequested, Selection: selection}},
		intentFacts: intentFacts,
	}
	return result, nil
}

func (f Facade) composeWithout(requested Pack, state ActivationState, surface Surface) composition {
	return composition{requested: requested, surface: surface, packs: []Pack{}, activations: []PlannedActivation{}, intentFacts: []ActivationIntent{}}
}

func (c composition) identityDigest() string {
	return digestJSON(struct {
		Packs       []Pack
		Activations []PlannedActivation
		Blockers    []PlanBlocker
		Intents     []ActivationIntent
	}{c.packs, c.activations, c.blockers, c.intentFacts})
}

func resourceWithSurfaceAlias(resource Resource, aliases []SurfaceAlias, surface Surface) Resource {
	for i := range resource.Bindings {
		binding := &resource.Bindings[i]
		if binding.Surface != surface {
			continue
		}
		for _, alias := range aliases {
			if alias.Kind == resource.Kind && alias.ID == resource.ID {
				binding.Name = alias.Name
			}
		}
	}
	return resource
}

func intentIsExplicit(ActivationIntent) bool { return true }

func intentByPackID(intents []ActivationIntent, packID string) ActivationIntent {
	for _, intent := range intents {
		if intent.PackID == packID {
			return intent
		}
	}
	return ActivationIntent{}
}

func packHasAliasTarget(pack Pack, alias SurfaceAlias, surface Surface) bool {
	for _, resource := range pack.Resources {
		if resource.Kind != alias.Kind || resource.ID != alias.ID {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface == surface {
				return true
			}
		}
	}
	return false
}

func supportsSurface(pack Pack, surface Surface) bool {
	for _, candidate := range pack.Surfaces {
		if candidate == surface {
			return true
		}
	}
	return false
}

func sortBlockers(values []PlanBlocker) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].Subject != values[j].Subject {
			return values[i].Subject < values[j].Subject
		}
		return values[i].Detail < values[j].Detail
	})
}

func activeIntents(state ActivationState) []ActivationIntent {
	values := append([]ActivationIntent(nil), state.Intents...)
	if len(values) == 0 && state.Intent.PackID != "" {
		values = append(values, state.Intent)
	}
	result := values[:0]
	for _, intent := range values {
		if intent.Active {
			result = append(result, intent)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Surface != result[j].Surface {
			return result[i].Surface < result[j].Surface
		}
		return result[i].PackID < result[j].PackID
	})
	return result
}
