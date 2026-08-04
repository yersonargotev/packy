package capabilitypack

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

type ActivationRole string
type BlockerKind string

const (
	ActivationRequested             ActivationRole = "requested"
	ActivationExplicit              ActivationRole = "explicit"
	ActivationRequired              ActivationRole = "required"
	ActivationExplicitRequired      ActivationRole = "explicit-required"
	ActivationInactive              ActivationRole = "inactive"
	BlockerDependency               BlockerKind    = "dependency"
	BlockerCapabilityConflict       BlockerKind    = "capability-conflict"
	BlockerIncompatibleContribution BlockerKind    = "incompatible-contribution"
	BlockerOwnership                BlockerKind    = "ownership"
	BlockerGlobalRequirement        BlockerKind    = "global-requirement"
	BlockerActiveDependent          BlockerKind    = "active-dependent"
	BlockerAlias                    BlockerKind    = "alias"
	BlockerSharing                  BlockerKind    = "sharing"
	BlockerCompatibility            BlockerKind    = "compatibility"
	BlockerResourceConflict         BlockerKind    = "resource-conflict"
	BlockerSelectionUnavailable     BlockerKind    = "selection-unavailable"
)

type PlannedActivation struct {
	Pack            Pack
	Role            ActivationRole
	Selection       ResourceSelection
	ProviderChoices []ProviderChoice
}

// CapabilityRequirementFact explains one resource-scoped provider edge without
// asking a renderer or host adapter to reconstruct portable manifest policy.
type CapabilityRequirementFact struct {
	ConsumerPack       string            `json:"consumer_pack"`
	ConsumerResource   *ResourceIdentity `json:"consumer_resource"`
	Capability         string            `json:"capability"`
	ProviderPack       string            `json:"provider_pack"`
	ProviderResource   *ResourceIdentity `json:"provider_resource"`
	RequiredTools      []string          `json:"required_tools"`
	RequiredAuthority  []string          `json:"required_authority"`
	ResultingReadiness ReadinessStatus   `json:"resulting_readiness"`
}

type ActiveDependent struct {
	PackID     string
	Dependency string
}

type PlanBlocker struct {
	Kind            BlockerKind
	Subject, Detail string
}

type composition struct {
	surface         Surface
	requested       Pack
	packs           []Pack
	activations     []PlannedActivation
	contributors    map[string][]string
	blockers        []PlanBlocker
	intentFacts     []ActivationIntent
	capabilityFacts []CapabilityRequirementFact
	projectAliases  []SurfaceAlias
}

func resourceContributor(packID string, resource ResourceIdentity) string {
	return "pack:" + packID + ":" + resource.String()
}

func contributorBelongsToPack(contributor, packID string) bool {
	return strings.HasPrefix(contributor, "pack:"+packID+":") || strings.Contains(contributor, ":pack:"+packID+":")
}

func qualifyContributor(surface Surface, contributor string) string {
	if strings.HasPrefix(contributor, "surface:") {
		return contributor
	}
	return "surface:" + string(surface) + ":" + contributor
}

func contributorSurface(contributor string) (Surface, bool) {
	if !strings.HasPrefix(contributor, "surface:") {
		return "", false
	}
	rest := strings.TrimPrefix(contributor, "surface:")
	index := strings.Index(rest, ":")
	if index < 1 {
		return "", false
	}
	return Surface(rest[:index]), true
}

func contributorsForSurface(surface Surface, canonical []string) []string {
	result := make([]string, 0, len(canonical))
	for _, contributor := range canonical {
		result = append(result, qualifyContributor(surface, contributor))
	}
	return sortedUnique(result)
}

func mergedProjectionContributors(owner ProjectionOwnership, surface Surface, current []string) []string {
	result := contributorsForSurface(surface, current)
	for _, contributor := range owner.Contributors {
		if contributorSurfaceValue, ok := contributorSurface(contributor); ok && contributorSurfaceValue == surface {
			continue
		}
		result = append(result, contributor)
	}
	return sortedUnique(result)
}

func uniqueRemovedContributor(projectionID string, before, after composition) (string, bool) {
	var removed []string
	remaining := after.contributorSet(projectionID)
	for _, contributor := range before.contributorSet(projectionID) {
		if !slices.Contains(remaining, contributor) {
			removed = append(removed, contributor)
		}
	}
	if len(removed) != 1 {
		return "", false
	}
	return removed[0], true
}

func removedContributorSet(before, after composition) map[string]string {
	result := map[string]string{}
	for projectionID := range before.contributors {
		if contributor, ok := uniqueRemovedContributor(projectionID, before, after); ok {
			result[projectionID] = contributor
		}
	}
	return result
}

// contributorsMatch permits a legacy Pack-only ownership fact to be recognized
// only when a freshly derived projection has one unambiguous resource
// contributor for every recorded Pack. Mutation paths persist the canonical
// form after verification; destructive paths still require an exact canonical
// contributor.
func contributorsMatch(recorded, canonical []string) bool {
	if digestJSON(recorded) == digestJSON(canonical) {
		return true
	}
	if len(recorded) != len(canonical) {
		return false
	}
	for i, packID := range recorded {
		if strings.HasPrefix(packID, "pack:") || !contributorBelongsToPack(canonical[i], packID) {
			return false
		}
	}
	return true
}

func contributorsMatchForSurface(recorded []string, surface Surface, canonical []string) bool {
	var local []string
	for _, contributor := range recorded {
		contributorSurfaceValue, qualified := contributorSurface(contributor)
		if qualified && contributorSurfaceValue != surface {
			continue
		}
		if qualified {
			prefix := "surface:" + string(surface) + ":"
			contributor = strings.TrimPrefix(contributor, prefix)
		}
		local = append(local, contributor)
	}
	return contributorsMatch(local, canonical)
}

func (c composition) combinedPack() Pack {
	p := clonePack(c.requested)
	p.Resources = nil
	p.Requires.Capabilities = nil
	p.Requires.Tools = nil
	resources := map[string]Resource{}
	tools := map[string]bool{}
	preserveResourceOrder := false
	for _, pack := range c.packs {
		preserveResourceOrder = preserveResourceOrder || pack.manifestVersion == manifestSchemaV4
		intent := intentByPackID(c.intentFacts, pack.ID)
		aliases := compositionAliases(intent.Aliases, c.projectAliases)
		for _, tool := range pack.Requires.Tools {
			tools[tool] = true
		}
		for _, r := range pack.Resources {
			if r.Kind != "notice" {
				for _, tool := range r.RequiresTools {
					tools[tool] = true
				}
			}
			r = resourceWithSurfaceAlias(r, aliases, c.surface)
			key := r.Kind + ":" + r.ID
			if _, ok := resources[key]; !ok {
				resources[key] = r
				p.Resources = append(p.Resources, r)
			}
		}
	}
	if !preserveResourceOrder {
		p.Resources = nil
		keys := make([]string, 0, len(resources))
		for key := range resources {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			p.Resources = append(p.Resources, resources[key])
		}
	}
	for tool := range tools {
		p.Requires.Tools = append(p.Requires.Tools, tool)
	}
	sort.Strings(p.Requires.Tools)
	return p
}

func (c composition) contributorSet(projectionID string) []string {
	if values := c.contributors[projectionID]; len(values) > 0 {
		return append([]string(nil), values...)
	}
	matched := map[string]bool{}
	for key, values := range c.contributors {
		resourceID := key
		if colon := strings.IndexByte(key, ':'); colon >= 0 {
			resourceID = key[colon+1:]
		}
		if key == projectionID || strings.HasSuffix(key, ":"+projectionID) || strings.HasSuffix(projectionID, ":"+key) || strings.HasSuffix(projectionID, ":"+resourceID) {
			for _, value := range values {
				matched[value] = true
			}
		}
	}
	if len(matched) > 0 {
		values := make([]string, 0, len(matched))
		for value := range matched {
			values = append(values, value)
		}
		sort.Strings(values)
		return values
	}
	var ids []string
	for _, p := range c.packs {
		for _, resource := range p.Resources {
			ids = append(ids, resourceContributor(p.ID, ResourceIdentity{Kind: resource.Kind, ID: resource.ID}))
		}
	}
	sort.Strings(ids)
	return ids
}

func (f Facade) compose(requested Pack, state ActivationState, surface Surface, useRequestedIntent bool) (composition, error) {
	return f.composeWithPolicy(requested, state, surface, useRequestedIntent, "", nil, false, nil)
}

func (f Facade) composeProject(requested Pack, state ActivationState, surface Surface, aliases []SurfaceAlias) (composition, error) {
	return f.composeWithPolicy(requested, state, surface, true, "", nil, true, aliases)
}

func (f Facade) composeWithPolicy(requested Pack, state ActivationState, surface Surface, useRequestedIntent bool, excludedPackID string, suppressedCapabilities map[string]bool, projectSelection bool, projectAliases []SurfaceAlias) (composition, error) {
	result := composition{requested: requested, surface: surface, contributors: map[string][]string{}, projectAliases: projectAliases}
	selectResources := selectPackResources
	if projectSelection {
		selectResources = selectProjectPackResources
	}
	selected := map[string]Pack{}
	active := activeIntents(state)
	activeIDs := map[string]bool{}
	selections := map[string]ResourceSelection{}
	for _, intent := range active {
		if !intent.Active || intent.Surface != surface {
			continue
		}
		if intent.PackID == requested.ID && !useRequestedIntent {
			continue
		}
		pack, err := f.catalog.resolveIntentPack(intent.PackID, intent.Version)
		if err != nil {
			return composition{}, err
		}
		pack, err = selectResources(pack, intent.Selection)
		if err != nil {
			return composition{}, fmt.Errorf("active pack %q has invalid resource selection: %w", intent.PackID, err)
		}
		selected[pack.ID] = pack
		activeIDs[pack.ID] = true
		selections[pack.ID] = cloneSelection(intent.Selection)
	}
	var visit func(Pack, ActivationRole, ResourceSelection)
	visiting := map[string]bool{}
	expanded := map[string]bool{}
	roles := map[string]ActivationRole{requested.ID: ActivationRequested}
	choicesByConsumer := map[string]map[string]ProviderChoice{}
	for _, intent := range activeIntents(state) {
		choicesByConsumer[intent.PackID] = providerChoicesByCapability(intent.ProviderChoices)
	}
	visit = func(pack Pack, role ActivationRole, selection ResourceSelection) {
		catalogPack := pack
		if existing, ok := selected[pack.ID]; ok {
			pack = existing
			if pack.manifestVersion == manifestSchemaV4 && selection.Mode == SelectionCustom {
				currentSelection, err := canonicalSelection(selections[pack.ID])
				if err != nil {
					result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, pack.ID, err.Error()})
					return
				}
				if currentSelection.Mode == SelectionCustom {
					merged, err := mergeCustomSelections(currentSelection, selection)
					if err != nil {
						result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, pack.ID, err.Error()})
						return
					}
					pack, err = selectResources(catalogPack, merged)
					if err != nil {
						result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, pack.ID, err.Error()})
						return
					}
					selected[pack.ID] = pack
					selections[pack.ID] = merged
					roles[pack.ID] = ActivationRequired
					delete(expanded, pack.ID)
				}
			}
		} else if pack.manifestVersion == manifestSchemaV4 {
			var err error
			pack, err = selectResources(pack, selection)
			if err != nil {
				result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, pack.ID, err.Error()})
				return
			}
		}
		if expanded[pack.ID] {
			return
		}
		if visiting[pack.ID] {
			result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, pack.ID, "dependency cycle prevents a deterministic closure"})
			return
		}
		selected[pack.ID] = pack
		if _, present := selections[pack.ID]; !present {
			selections[pack.ID] = cloneSelection(selection)
		}
		if _, ok := roles[pack.ID]; !ok && !activeIDs[pack.ID] {
			roles[pack.ID] = role
		}
		visiting[pack.ID] = true
		requirements := capabilityRequirements(pack)
		for _, requirement := range requirements {
			capability := requirement.capability
			if suppressedCapabilities[capability] {
				continue
			}
			providers := f.providersExcept(capability, surface, excludedPackID)
			var provider capabilityProvider
			if choice, chosen := choicesByConsumer[pack.ID][capability]; chosen {
				var choiceErr error
				provider, choiceErr = selectChosenProvider(choice, providers)
				if choiceErr != nil {
					result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, capability, choiceErr.Error()})
					continue
				}
			} else if len(providers) == 1 {
				provider = providers[0]
			} else {
				detail := "required capability has no provider"
				if len(providers) > 1 {
					detail = "required capability has multiple eligible providers; an explicit provider choice is required"
				}
				result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, capability, detail})
				continue
			}
			providerSelection := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
			if provider.resource != (ResourceIdentity{}) {
				providerSelection = ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{provider.resource}}
			}
			visit(provider.pack, ActivationRequired, providerSelection)
			requiredAuthority := append([]string(nil), requirement.authority...)
			if provider.resource != (ResourceIdentity{}) {
				providerClosure, err := selectResources(provider.pack, providerSelection)
				if err == nil {
					requiredAuthority = append(requiredAuthority, packAuthorities(providerClosure)...)
				}
			}
			result.capabilityFacts = append(result.capabilityFacts, CapabilityRequirementFact{
				ConsumerPack: pack.ID, ConsumerResource: optionalResourceIdentity(requirement.resource), Capability: capability,
				ProviderPack: provider.pack.ID, ProviderResource: optionalResourceIdentity(provider.resource),
				RequiredTools:     append([]string(nil), requirement.tools...),
				RequiredAuthority: sortedUnique(requiredAuthority),
			})
		}
		delete(visiting, pack.ID)
		expanded[pack.ID] = true
	}
	visit(requested, ActivationRequested, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}})
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result.packs = append(result.packs, selected[id])
		if role, ok := roles[id]; ok {
			if role == ActivationRequired && activeIDs[id] && intentIsExplicit(intentByPackID(active, id)) {
				role = ActivationExplicitRequired
			}
			choices := cloneProviderChoices(intentByPackID(active, id).ProviderChoices)
			for _, fact := range result.capabilityFacts {
				if fact.ConsumerPack == id {
					if _, exists := providerChoicesByCapability(choices)[fact.Capability]; !exists {
						choices = append(choices, ProviderChoice{Capability: fact.Capability, ProviderPack: fact.ProviderPack, ProviderResource: fact.ProviderResource})
					}
				}
			}
			choices, _ = canonicalProviderChoices(choices)
			result.activations = append(result.activations, PlannedActivation{Pack: selected[id], Role: role, Selection: selections[id], ProviderChoices: choices})
		}
	}
	for _, intent := range active {
		if _, ok := selected[intent.PackID]; ok {
			result.intentFacts = append(result.intentFacts, intent)
		}
	}
	sort.Slice(result.intentFacts, func(i, j int) bool { return result.intentFacts[i].PackID < result.intentFacts[j].PackID })
	type capabilityContributor struct {
		packID   string
		resource ResourceIdentity
	}
	provided := map[string]capabilityContributor{}
	for _, pack := range result.packs {
		if pack.manifestVersion != manifestSchemaV4 {
			for _, capability := range pack.Provides {
				provided[capability] = capabilityContributor{packID: pack.ID}
			}
			continue
		}
		for _, resource := range pack.Resources {
			if resource.Kind == "notice" {
				continue
			}
			for _, capability := range resource.ProvidesCapabilities {
				provided[capability] = capabilityContributor{packID: pack.ID, resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}}
			}
		}
	}
	for _, pack := range result.packs {
		if pack.manifestVersion != manifestSchemaV4 {
			for _, conflict := range pack.Conflicts {
				if other, ok := provided[conflict]; ok && other.packID != pack.ID {
					result.blockers = append(result.blockers, PlanBlocker{BlockerCapabilityConflict, conflict, fmt.Sprintf("pack %s conflicts with capability provided by %s", pack.ID, other.packID)})
				}
			}
			continue
		}
		for _, resource := range pack.Resources {
			if resource.Kind == "notice" {
				continue
			}
			identity := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
			for _, conflict := range resource.CapabilityConflicts {
				if other, ok := provided[conflict]; ok && (other.packID != pack.ID || other.resource != identity) {
					result.blockers = append(result.blockers, PlanBlocker{BlockerCapabilityConflict, conflict,
						fmt.Sprintf("resource %s in pack %s conflicts with capability provided by %s in pack %s", identity.String(), pack.ID, other.resource.String(), other.packID)})
				}
			}
		}
	}
	resources := map[string]Resource{}
	projectedNames := map[string]string{}
	for _, pack := range result.packs {
		intent := intentByPackID(active, pack.ID)
		aliases := compositionAliases(intent.Aliases, result.projectAliases)
		if !projectSelection {
			for _, alias := range aliases {
				if !packHasAliasTarget(pack, alias, surface) {
					result.blockers = append(result.blockers, PlanBlocker{BlockerAlias, alias.Kind + ":" + alias.ID, "saved surface alias no longer targets a bound portable resource"})
				}
			}
		}
		for _, resource := range pack.Resources {
			resolved := resourceWithSurfaceAlias(resource, aliases, surface)
			key := resource.Kind + ":" + resource.ID
			if previous, ok := resources[key]; ok {
				if digestJSON(previous) != digestJSON(resource) {
					result.blockers = append(result.blockers, PlanBlocker{BlockerIncompatibleContribution, key, "contributors declare different portable resources"})
					continue
				}
				previousSharing, currentSharing := bindingSharing(previous, surface), bindingSharing(resource, surface)
				if (previousSharing != "" || currentSharing != "") && (previousSharing != "shared" || currentSharing != "shared") {
					result.blockers = append(result.blockers, PlanBlocker{BlockerSharing, key, "every contributor must explicitly declare shared for an overlapping surface binding"})
				}
			}
			resources[key] = resource
			contributor := resourceContributor(pack.ID, ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
			result.contributors[key] = append(result.contributors[key], contributor)
			if projectionID, ok := effectiveProjectionID(resolved, surface); ok && projectionID != key {
				result.contributors[projectionID] = append(result.contributors[projectionID], contributor)
			}
			if namespace, name, ok := projectedNamespace(resolved, surface); ok {
				projection := namespace + ":" + name
				if prior, exists := projectedNames[projection]; exists && prior != key {
					result.blockers = append(result.blockers, PlanBlocker{BlockerAlias, projection, fmt.Sprintf("portable resources %s and %s collide in the %s namespace; declare an explicit alias", prior, key, surface)})
				} else {
					projectedNames[projection] = key
				}
			}
		}
		if compatibilityFor(pack, surface) == CompatibilityBlocked {
			result.blockers = append(result.blockers, PlanBlocker{BlockerCompatibility, pack.ID, "declared surface outcomes do not form a compatible runtime dependency closure"})
		}
	}
	for key := range result.contributors {
		sort.Strings(result.contributors[key])
	}
	sortBlockers(result.blockers)
	sort.Slice(result.capabilityFacts, func(i, j int) bool {
		a, b := result.capabilityFacts[i], result.capabilityFacts[j]
		if a.ConsumerPack != b.ConsumerPack {
			return a.ConsumerPack < b.ConsumerPack
		}
		if resourceIdentityKey(a.ConsumerResource) != resourceIdentityKey(b.ConsumerResource) {
			return resourceIdentityKey(a.ConsumerResource) < resourceIdentityKey(b.ConsumerResource)
		}
		return a.Capability < b.Capability
	})
	return result, nil
}

func compositionAliases(intentAliases, projectAliases []SurfaceAlias) []SurfaceAlias {
	if projectAliases != nil {
		return projectAliases
	}
	return intentAliases
}

func intentIsExplicit(intent ActivationIntent) bool {
	return intent.Explicit == nil || *intent.Explicit
}

func providerChoicesByCapability(values []ProviderChoice) map[string]ProviderChoice {
	result := make(map[string]ProviderChoice, len(values))
	for _, value := range values {
		result[value.Capability] = value
	}
	return result
}

func selectChosenProvider(choice ProviderChoice, eligible []capabilityProvider) (capabilityProvider, error) {
	for _, candidate := range eligible {
		if candidate.pack.ID != choice.ProviderPack {
			continue
		}
		if (choice.ProviderResource == nil && candidate.resource == (ResourceIdentity{})) ||
			(choice.ProviderResource != nil && *choice.ProviderResource == candidate.resource) {
			return candidate, nil
		}
	}
	return capabilityProvider{}, fmt.Errorf("provider choice %s for capability %s is not eligible", choice.ProviderPack, choice.Capability)
}

func resourceIdentityKey(identity *ResourceIdentity) string {
	if identity == nil {
		return ""
	}
	return identity.String()
}

func optionalResourceIdentity(identity ResourceIdentity) *ResourceIdentity {
	if identity == (ResourceIdentity{}) {
		return nil
	}
	copy := identity
	return &copy
}

type capabilityRequirement struct {
	resource   ResourceIdentity
	capability string
	tools      []string
	authority  []string
}

func capabilityRequirements(pack Pack) []capabilityRequirement {
	if pack.manifestVersion != manifestSchemaV4 {
		result := make([]capabilityRequirement, 0, len(pack.Requires.Capabilities))
		for _, capability := range pack.Requires.Capabilities {
			result = append(result, capabilityRequirement{capability: capability, tools: append([]string(nil), pack.Requires.Tools...)})
		}
		return result
	}
	var result []capabilityRequirement
	for _, resource := range pack.Resources {
		if resource.Kind == "notice" {
			continue
		}
		for _, capability := range resource.RequiresCapabilities {
			result = append(result, capabilityRequirement{
				resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, capability: capability,
				tools: append([]string(nil), resource.RequiresTools...), authority: resourceAuthorities(resource),
			})
		}
	}
	return result
}

func providedCapabilities(pack Pack) []string {
	if pack.manifestVersion != manifestSchemaV4 {
		return pack.Provides
	}
	var result []string
	for _, resource := range pack.Resources {
		if resource.Kind == "notice" {
			continue
		}
		result = append(result, resource.ProvidesCapabilities...)
	}
	return sortedUnique(result)
}

func resourceAuthorities(resource Resource) []string {
	result := append([]string(nil), resource.Permissions...)
	for _, mode := range resource.RuntimeModes {
		for _, authority := range mode.Authorities {
			result = append(result, string(authority.Kind)+":"+string(authority.Scope))
		}
	}
	return sortedUnique(result)
}

func packAuthorities(pack Pack) []string {
	var result []string
	for _, resource := range pack.Resources {
		if resource.Kind != "notice" {
			result = append(result, resourceAuthorities(resource)...)
		}
	}
	return sortedUnique(result)
}

func projectedNamespace(resource Resource, surface Surface) (string, string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface != surface {
			continue
		}
		switch binding.Projection {
		case "skill":
			return "personal-skill", binding.Name, true
		case "agent":
			return "agent", binding.Name, true
		case "mcp_server":
			return "mcp", binding.Name, true
		case "command_hook":
			if binding.Hook == nil {
				return "hook", binding.Name, true
			}
			return "hook", binding.Hook.Event + ":" + binding.Hook.Matcher + ":" + binding.Name, true
		}
	}
	return "", "", false
}

func effectiveProjectionID(resource Resource, surface Surface) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == surface && binding.Name != "" {
			return resource.Kind + ":" + binding.Name, true
		}
	}
	return "", false
}

func resourceWithSurfaceAlias(resource Resource, aliases []SurfaceAlias, surface Surface) Resource {
	for _, alias := range aliases {
		if alias.Kind != resource.Kind || alias.ID != resource.ID {
			continue
		}
		resource.Bindings = append([]Binding(nil), resource.Bindings...)
		for i := range resource.Bindings {
			if resource.Bindings[i].Surface != surface {
				continue
			}
			resource.Bindings[i].Name = alias.Name
			switch resource.Bindings[i].Projection {
			case "skill":
				resource.Bindings[i].Invocation = "$" + alias.Name
			case "command":
				resource.Bindings[i].Invocation = "/" + alias.Name
			case "agent":
				resource.Bindings[i].Invocation = "@" + alias.Name
			}
		}
		return resource
	}
	return resource
}

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
		if resource.Kind == alias.Kind && resource.ID == alias.ID && bindingSharing(resource, surface) != "" {
			return true
		}
	}
	return false
}

func bindingSharing(resource Resource, surface Surface) string {
	for _, binding := range resource.Bindings {
		if binding.Surface == surface {
			return binding.Sharing
		}
	}
	return ""
}

// composeWithout builds the complete desired state for a surface after one
// active pack is removed. The requested pack is never reintroduced through a
// dependency: callers must reject the sealed dependent facts instead.
func (f Facade) composeWithout(requested Pack, state ActivationState, surface Surface) (composition, []ActiveDependent, error) {
	targetState := cloneActivationState(state)
	active := activeIntents(state)
	remaining := make([]ActivationIntent, 0, len(active))
	var dependents []ActiveDependent
	suppressed := map[string]bool{}
	for _, intent := range active {
		if intent.Surface != surface || !intent.Active {
			continue
		}
		if intent.PackID == requested.ID {
			continue
		}
		remaining = append(remaining, intent)
		pack, err := f.catalog.resolveIntentPack(intent.PackID, intent.Version)
		if err != nil {
			return composition{}, nil, err
		}
		pack, err = selectPackResources(pack, intent.Selection)
		if err != nil {
			return composition{}, nil, fmt.Errorf("active pack %q has invalid resource selection: %w", intent.PackID, err)
		}
		for _, choice := range intent.ProviderChoices {
			if choice.ProviderPack == requested.ID {
				dependents = append(dependents, ActiveDependent{PackID: pack.ID, Dependency: choice.Capability})
				suppressed[choice.Capability] = true
			}
		}
	}
	sort.Slice(dependents, func(i, j int) bool {
		if dependents[i].PackID == dependents[j].PackID {
			return dependents[i].Dependency < dependents[j].Dependency
		}
		return dependents[i].PackID < dependents[j].PackID
	})
	// Required-only provider intents are lifecycle support, not independent
	// roots. Drop them once no remaining consumer choice references them.
	for {
		byID := make(map[string]ActivationIntent, len(remaining))
		for _, intent := range remaining {
			byID[intent.PackID] = intent
		}
		filtered := remaining[:0]
		removed := false
		for _, intent := range remaining {
			if intentIsExplicit(intent) || providerHasConsumer(byID, intent.PackID) {
				filtered = append(filtered, intent)
				continue
			}
			removed = true
		}
		remaining = filtered
		if !removed {
			break
		}
	}
	targetState.Intents = remaining
	targetState.Intent = ActivationIntent{Surface: surface, Revision: state.Intent.Revision}
	if len(remaining) == 0 {
		return composition{requested: Pack{ID: requested.ID, Surfaces: []Surface{surface}}, contributors: map[string][]string{}}, dependents, nil
	}
	root, err := f.catalog.resolveIntentPack(remaining[0].PackID, remaining[0].Version)
	if err != nil {
		return composition{}, nil, err
	}
	result, err := f.composeWithPolicy(root, targetState, surface, true, requested.ID, suppressed, false, nil)
	if err != nil {
		return composition{}, nil, err
	}
	result.activations = nil
	return result, dependents, nil
}

func (c composition) identityDigest() string {
	return digestJSON(struct {
		Packs          []Pack
		Activations    []PlannedActivation
		Contributors   map[string][]string
		Blockers       []PlanBlocker
		Intents        []ActivationIntent
		ProjectAliases []SurfaceAlias
	}{c.packs, c.activations, c.contributors, c.blockers, c.intentFacts, c.projectAliases})
}

type capabilityProvider struct {
	pack     Pack
	resource ResourceIdentity
}

func (f Facade) providersExcept(capability string, surface Surface, excludedPackID string) []capabilityProvider {
	var result []capabilityProvider
	for _, pack := range f.catalog.List() {
		if pack.ID == excludedPackID || !supportsSurface(pack, surface) {
			continue
		}
		result = append(result, providersInPack(pack, capability)...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].pack.ID != result[j].pack.ID {
			return result[i].pack.ID < result[j].pack.ID
		}
		return result[i].resource.String() < result[j].resource.String()
	})
	return result
}

func supportsSurface(pack Pack, surface Surface) bool {
	for _, item := range pack.Surfaces {
		if item == surface {
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
	if len(state.Intents) > 0 {
		return append([]ActivationIntent(nil), state.Intents...)
	}
	if state.Intent.Active {
		return []ActivationIntent{state.Intent}
	}
	return nil
}
