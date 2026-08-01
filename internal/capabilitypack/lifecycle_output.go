package capabilitypack

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/reportredaction"
)

const LifecycleJSONSchemaVersion = 8

type ResourceRole string

const (
	ResourceRoleRoot       ResourceRole = "root"
	ResourceRoleDependency ResourceRole = "dependency"
	ResourceRoleAsset      ResourceRole = "asset"
	ResourceRoleNotice     ResourceRole = "notice"
	ResourceRoleUnselected ResourceRole = "unselected"
)

// ResourceClosureFact is the canonical portable explanation of why a resource
// participates in an operation. DependencyChain runs from an explicit root to
// Resource and is empty only for unselected inventory.
type ResourceClosureFact struct {
	Resource        ResourceIdentity   `json:"resource"`
	Role            ResourceRole       `json:"role"`
	DependencyChain []ResourceIdentity `json:"dependency_chain"`
	Requires        []ResourceIdentity `json:"requires"`
	Notices         []ResourceIdentity `json:"notices"`
}

// SensitiveEffectOrigin binds manifest-declared authority and effect facts to
// the exact selected resource and root-to-resource dependency chain that
// introduces them.
type SensitiveEffectOrigin struct {
	Pack               string                   `json:"pack"`
	Resource           ResourceIdentity         `json:"resource"`
	Root               ResourceIdentity         `json:"root"`
	DependencyChain    []ResourceIdentity       `json:"dependency_chain"`
	PromptAuthorities  []string                 `json:"prompt_authorities"`
	RuntimeAuthorities []RuntimeAuthorityOrigin `json:"runtime_authorities"`
	RuntimeEffects     []RuntimeEffectOrigin    `json:"runtime_effects"`
}

type RuntimeAuthorityOrigin struct {
	ModeID string               `json:"mode_id"`
	Kind   RuntimeAuthorityKind `json:"kind"`
	Scope  RuntimeScope         `json:"scope,omitempty"`
}

type RuntimeEffectOrigin struct {
	ModeID string            `json:"mode_id"`
	Kind   RuntimeEffectKind `json:"kind"`
	Scope  RuntimeScope      `json:"scope,omitempty"`
}

type ResourceGraph struct {
	Resources []ResourceClosureFact `json:"resources"`
}

func (graph ResourceGraph) MarshalJSON() ([]byte, error) {
	resources := graph.Resources
	if resources == nil {
		resources = []ResourceClosureFact{}
	}
	return json.Marshal(struct {
		Resources []ResourceClosureFact `json:"resources"`
	}{Resources: resources})
}

// LifecycleContract is the canonical, host-neutral description rendered by
// every lifecycle entry point. Renderers must not reconstruct these facts
// from a manifest.
type LifecycleContract struct {
	Compatibility         Compatibility        `json:"compatibility,omitempty"`
	CompatibilityObserved bool                 `json:"-"`
	Counts                ResourceCounts       `json:"logical_resource_counts"`
	DependencyClosure     []string             `json:"dependency_closure"`
	Bindings              []LifecycleBinding   `json:"bindings"`
	Exclusions            []LifecycleExclusion `json:"exclusions"`
	OptionalModes         []OptionalMode       `json:"optional_modes"`
	PromptAuthorities     []string             `json:"prompt_authorities"`
	Aliases               []SurfaceAlias       `json:"aliases"`
	AuthorityDisclosure   string               `json:"authority_disclosure"`
	ResourceGraph         ResourceGraph        `json:"resource_graph"`
	SelectionValidity     SelectionValidity    `json:"selection_validity"`
}

// LifecycleExclusion is the rendered union of portable source exclusions and
// v3 surface outcomes. Surface exclusions retain the resource and stable code
// that explain compatibility without being mistaken for runtime projections.
type LifecycleExclusion struct {
	ID           string   `json:"id"`
	ResourceKind string   `json:"resource_kind,omitempty"`
	Surface      Surface  `json:"surface,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	Code         string   `json:"code,omitempty"`
	SourcePaths  []string `json:"source_paths"`
	Reason       string   `json:"reason"`
}

type Compatibility string

const (
	CompatibilityComplete Compatibility = "complete"
	CompatibilityDegraded Compatibility = "degraded"
	CompatibilityBlocked  Compatibility = "blocked"
)

type LifecycleBinding struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Projection  string `json:"projection"`
	Name        string `json:"name"`
	Invocation  string `json:"invocation"`
	Mode        string `json:"mode"`
	Degradation string `json:"degradation,omitempty"`
	Sharing     string `json:"sharing"`
}

// LifecycleContractFor derives the complete portable contract for one
// surface. Every slice is allocated so JSON preserves [] rather than null.
func LifecycleContractFor(pack Pack, surface Surface, aliases []SurfaceAlias) LifecycleContract {
	contract := LifecycleContract{
		Compatibility: compatibilityFor(pack, surface), CompatibilityObserved: pack.manifestVersion >= manifestSchemaV3,
		Counts: pack.ResourceCounts(), DependencyClosure: []string{}, Bindings: []LifecycleBinding{},
		Exclusions: []LifecycleExclusion{}, OptionalModes: []OptionalMode{}, PromptAuthorities: []string{}, Aliases: []SurfaceAlias{},
		AuthorityDisclosure: "Activation grants only the sealed local projection actions; later workflow effects require host approval.",
		ResourceGraph:       ResourceGraphFor(pack, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, true),
		SelectionValidity:   SelectionValidityFor(pack, surface),
	}
	if !contract.CompatibilityObserved {
		contract.Compatibility = ""
	}
	contract.DependencyClosure = sortedUnique(pack.Requires.Capabilities)
	authorities := []string{}
	for _, resource := range pack.Resources {
		resolved := resourceWithSurfaceAlias(resource, aliases, surface)
		for _, binding := range resolved.Bindings {
			if binding.Surface != surface {
				continue
			}
			contract.Bindings = append(contract.Bindings, LifecycleBinding{
				Kind: resource.Kind, ID: resource.ID, Projection: binding.Projection, Name: binding.Name,
				Invocation: binding.Invocation, Mode: binding.Mode, Degradation: binding.Degradation, Sharing: binding.Sharing,
			})
			authorities = append(authorities, resource.Permissions...)
		}
	}
	sort.Slice(contract.Bindings, func(i, j int) bool {
		a, b := contract.Bindings[i], contract.Bindings[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Projection != b.Projection {
			return a.Projection < b.Projection
		}
		return a.Name < b.Name
	})
	contract.PromptAuthorities = sortedUnique(authorities)
	for _, exclusion := range pack.Contract.Exclusions {
		contract.Exclusions = append(contract.Exclusions, LifecycleExclusion{ID: exclusion.ID, SourcePaths: sortedUnique(exclusion.SourcePaths), Reason: exclusion.Reason})
	}
	for _, resource := range pack.Resources {
		for _, exclusion := range resource.SurfaceExclusions {
			if exclusion.Surface == surface {
				contract.Exclusions = append(contract.Exclusions, LifecycleExclusion{ID: resource.Kind + ":" + resource.ID, ResourceKind: resource.Kind, Surface: surface, Mode: exclusion.Mode, Code: exclusion.Code, SourcePaths: []string{}, Reason: exclusion.Reason})
			}
		}
	}
	for i := range contract.Exclusions {
		contract.Exclusions[i].SourcePaths = sortedUnique(contract.Exclusions[i].SourcePaths)
	}
	sort.Slice(contract.Exclusions, func(i, j int) bool {
		if contract.Exclusions[i].ID != contract.Exclusions[j].ID {
			return contract.Exclusions[i].ID < contract.Exclusions[j].ID
		}
		return contract.Exclusions[i].Code < contract.Exclusions[j].Code
	})
	contract.OptionalModes = append(contract.OptionalModes, pack.Contract.OptionalModes...)
	for i := range contract.OptionalModes {
		contract.OptionalModes[i].Authorities = sortedUnique(contract.OptionalModes[i].Authorities)
		authorities = append(authorities, contract.OptionalModes[i].Authorities...)
	}
	sort.Slice(contract.OptionalModes, func(i, j int) bool { return contract.OptionalModes[i].ID < contract.OptionalModes[j].ID })
	contract.PromptAuthorities = sortedUnique(authorities)
	contract.Aliases = append(contract.Aliases, aliases...)
	sort.Slice(contract.Aliases, func(i, j int) bool {
		if contract.Aliases[i].Kind != contract.Aliases[j].Kind {
			return contract.Aliases[i].Kind < contract.Aliases[j].Kind
		}
		if contract.Aliases[i].ID != contract.Aliases[j].ID {
			return contract.Aliases[i].ID < contract.Aliases[j].ID
		}
		return contract.Aliases[i].Name < contract.Aliases[j].Name
	})
	return contract
}

// ResourceGraphFor returns deterministic graph facts. When inventory is true,
// resources outside a custom closure are retained as unselected facts.
func ResourceGraphFor(pack Pack, selection ResourceSelection, inventory bool) ResourceGraph {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return ResourceGraph{Resources: []ResourceClosureFact{}}
	}
	chains := map[string][]ResourceIdentity{}
	ordered := pack.Resources
	if selection.Mode == SelectionAll && pack.manifestVersion != manifestSchemaV4 {
		for _, resource := range pack.Resources {
			identity := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
			chains[identity.String()] = []ResourceIdentity{identity}
		}
	} else {
		roots, err := resourceSelectionRoots(pack, selection)
		if err != nil {
			return ResourceGraph{Resources: []ResourceClosureFact{}}
		}
		ordered, chains, err = resolveResourceClosure(pack, roots)
		if err != nil {
			return ResourceGraph{Resources: []ResourceClosureFact{}}
		}
	}
	source := ordered
	if inventory {
		source = pack.Resources
	}
	facts := make([]ResourceClosureFact, 0, len(source))
	for _, resource := range source {
		identity := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		chain, selected := chains[identity.String()]
		if !selected && !inventory {
			continue
		}
		role := ResourceRoleDependency
		switch {
		case !selected:
			role = ResourceRoleUnselected
		case resource.Kind == "asset":
			role = ResourceRoleAsset
		case resource.Kind == "notice":
			role = ResourceRoleNotice
		case len(chain) == 1:
			role = ResourceRoleRoot
		}
		facts = append(facts, ResourceClosureFact{
			Resource: identity, Role: role, DependencyChain: append([]ResourceIdentity{}, chain...),
			Requires: resourceIdentities(resource.Requires), Notices: resourceIdentities(resource.Notices),
		})
	}
	if inventory {
		sort.Slice(facts, func(i, j int) bool { return facts[i].Resource.String() < facts[j].Resource.String() })
	}
	return ResourceGraph{Resources: facts}
}

func SensitiveEffectOriginsFor(pack Pack, selection ResourceSelection) []SensitiveEffectOrigin {
	resources := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	selection, err := canonicalSelection(selection)
	if err != nil {
		return []SensitiveEffectOrigin{}
	}
	roots, err := resourceSelectionRoots(pack, selection)
	if err != nil {
		return []SensitiveEffectOrigin{}
	}
	origins := []SensitiveEffectOrigin{}
	for _, root := range roots {
		paths, err := resourceDependencyPaths(pack, root)
		if err != nil {
			return []SensitiveEffectOrigin{}
		}
		identities := make([]string, 0, len(paths))
		for identity := range paths {
			identities = append(identities, identity)
		}
		sort.Strings(identities)
		for _, identity := range identities {
			resource := resources[identity]
			promptAuthorities, runtimeAuthorities, runtimeEffects := resourceSensitiveFacts(resource)
			if len(promptAuthorities) == 0 && len(runtimeAuthorities) == 0 && len(runtimeEffects) == 0 {
				continue
			}
			resourceIdentity := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
			for _, path := range paths[identity] {
				origins = append(origins, SensitiveEffectOrigin{
					Pack: pack.ID, Resource: resourceIdentity, Root: root,
					DependencyChain:   append([]ResourceIdentity{}, path...),
					PromptAuthorities: promptAuthorities, RuntimeAuthorities: runtimeAuthorities, RuntimeEffects: runtimeEffects,
				})
			}
		}
	}
	sort.Slice(origins, func(i, j int) bool {
		return sensitiveEffectOriginKey(origins[i]) < sensitiveEffectOriginKey(origins[j])
	})
	return origins
}

func resourceDependencyPaths(pack Pack, root ResourceIdentity) (map[string][][]ResourceIdentity, error) {
	resources := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	paths := map[string][][]ResourceIdentity{}
	seenPaths := map[string]bool{}
	var visit func(string, []ResourceIdentity) error
	visit = func(identity string, path []ResourceIdentity) error {
		resource, ok := resources[identity]
		if !ok {
			return fmt.Errorf("custom resource selection root or dependency %q does not exist in pack %q", identity, pack.ID)
		}
		for _, member := range path {
			if member.String() == identity {
				return nil
			}
		}
		candidate := append(append([]ResourceIdentity{}, path...), ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
		key := identity + "\x00" + identityChainKey(candidate)
		if !seenPaths[key] {
			paths[identity] = append(paths[identity], candidate)
			seenPaths[key] = true
		}
		dependencies := append([]string(nil), resource.Requires...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency, candidate); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root.String(), nil); err != nil {
		return nil, err
	}
	dependencyPaths := make(map[string][][]ResourceIdentity, len(paths))
	for identity, values := range paths {
		dependencyPaths[identity] = append([][]ResourceIdentity(nil), values...)
	}
	for identity, values := range dependencyPaths {
		notices := append([]string(nil), resources[identity].Notices...)
		sort.Strings(notices)
		for _, notice := range notices {
			resource, ok := resources[notice]
			if !ok {
				return nil, fmt.Errorf("resource %q notice %q does not exist in pack %q", identity, notice, pack.ID)
			}
			for _, path := range values {
				candidate := append(append([]ResourceIdentity{}, path...), ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
				key := notice + "\x00" + identityChainKey(candidate)
				if !seenPaths[key] {
					paths[notice] = append(paths[notice], candidate)
					seenPaths[key] = true
				}
			}
		}
	}
	for identity := range paths {
		sort.Slice(paths[identity], func(i, j int) bool {
			return identityChainLess(paths[identity][i], paths[identity][j])
		})
	}
	return paths, nil
}

func identityChainKey(values []ResourceIdentity) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, value.String())
	}
	return strings.Join(parts, "\x00")
}

func resourceSensitiveFacts(resource Resource) ([]string, []RuntimeAuthorityOrigin, []RuntimeEffectOrigin) {
	promptAuthorities := sortedUnique(resource.Permissions)
	runtimeAuthorities := []RuntimeAuthorityOrigin{}
	runtimeEffects := []RuntimeEffectOrigin{}
	for _, mode := range resource.RuntimeModes {
		for _, authority := range mode.Authorities {
			runtimeAuthorities = append(runtimeAuthorities, RuntimeAuthorityOrigin{ModeID: mode.ID, Kind: authority.Kind, Scope: authority.Scope})
		}
		for _, effect := range mode.Effects {
			runtimeEffects = append(runtimeEffects, RuntimeEffectOrigin{ModeID: mode.ID, Kind: effect.Kind, Scope: effect.Scope})
		}
	}
	sort.Slice(runtimeAuthorities, func(i, j int) bool {
		return runtimeOriginKey(runtimeAuthorities[i].ModeID, string(runtimeAuthorities[i].Kind), string(runtimeAuthorities[i].Scope)) <
			runtimeOriginKey(runtimeAuthorities[j].ModeID, string(runtimeAuthorities[j].Kind), string(runtimeAuthorities[j].Scope))
	})
	sort.Slice(runtimeEffects, func(i, j int) bool {
		return runtimeOriginKey(runtimeEffects[i].ModeID, string(runtimeEffects[i].Kind), string(runtimeEffects[i].Scope)) <
			runtimeOriginKey(runtimeEffects[j].ModeID, string(runtimeEffects[j].Kind), string(runtimeEffects[j].Scope))
	})
	return promptAuthorities, runtimeAuthorities, runtimeEffects
}

func runtimeOriginKey(modeID, kind, scope string) string {
	return modeID + "\x00" + kind + "\x00" + scope
}

func sensitiveEffectOriginKey(origin SensitiveEffectOrigin) string {
	chain := make([]string, 0, len(origin.DependencyChain))
	for _, identity := range origin.DependencyChain {
		chain = append(chain, identity.String())
	}
	return origin.Pack + "\x00" + origin.Root.String() + "\x00" + origin.Resource.String() + "\x00" + strings.Join(chain, "\x00")
}

func cloneSensitiveEffectOrigins(values []SensitiveEffectOrigin) []SensitiveEffectOrigin {
	result := make([]SensitiveEffectOrigin, len(values))
	copy(result, values)
	for i := range result {
		result[i].DependencyChain = append([]ResourceIdentity(nil), result[i].DependencyChain...)
		result[i].PromptAuthorities = append([]string(nil), result[i].PromptAuthorities...)
		result[i].RuntimeAuthorities = append([]RuntimeAuthorityOrigin(nil), result[i].RuntimeAuthorities...)
		result[i].RuntimeEffects = append([]RuntimeEffectOrigin(nil), result[i].RuntimeEffects...)
	}
	return result
}

func sensitiveEffectOriginsForComposition(packs []Pack, activations []PlannedActivation, intents []ActivationIntent, requestedPackID string, requestedSelection ResourceSelection) []SensitiveEffectOrigin {
	selections := make(map[string]ResourceSelection, len(packs))
	for _, intent := range intents {
		selections[intent.PackID] = cloneSelection(intent.Selection)
	}
	for _, activation := range activations {
		selections[activation.Pack.ID] = cloneSelection(activation.Selection)
	}
	if requestedPackID != "" {
		selections[requestedPackID] = cloneSelection(requestedSelection)
	}
	origins := []SensitiveEffectOrigin{}
	for _, pack := range packs {
		selection, ok := selections[pack.ID]
		if !ok {
			selection = ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
		}
		origins = append(origins, SensitiveEffectOriginsFor(pack, selection)...)
	}
	sort.Slice(origins, func(i, j int) bool {
		return sensitiveEffectOriginKey(origins[i]) < sensitiveEffectOriginKey(origins[j])
	})
	return origins
}

func resourceIdentities(values []string) []ResourceIdentity {
	result := make([]ResourceIdentity, 0, len(values))
	for _, value := range values {
		if identity, err := ParseResourceIdentity(value); err == nil {
			result = append(result, identity)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	return result
}

func identityChainLess(a, b []ResourceIdentity) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i].String() != b[i].String() {
			return a[i].String() < b[i].String()
		}
	}
	return len(a) < len(b)
}

func compatibilityFor(pack Pack, surface Surface) Compatibility {
	if pack.manifestVersion < manifestSchemaV3 {
		return CompatibilityComplete
	}
	result := CompatibilityComplete
	resources := make(map[string]Resource, len(pack.Resources))
	included := make(map[string]bool, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[resource.Kind+":"+resource.ID] = resource
		if resource.Kind == "asset" || resource.Kind == "notice" {
			continue
		}
		outcome := false
		for _, binding := range resource.Bindings {
			if binding.Surface != surface {
				continue
			}
			outcome = true
			included[resource.Kind+":"+resource.ID] = true
			if binding.Mode != "native" || binding.Degradation != "" {
				result = CompatibilityDegraded
			}
		}
		for _, exclusion := range resource.SurfaceExclusions {
			if exclusion.Surface != surface {
				continue
			}
			outcome = true
			if exclusion.Mode == "mandatory" {
				return CompatibilityBlocked
			}
			result = CompatibilityDegraded
		}
		if !outcome {
			return CompatibilityBlocked
		}
	}
	// Assets have no standalone surface outcome. They participate only when a
	// compatible runtime consumer reaches them through its declared closure.
	visiting := map[string]bool{}
	var closureCompatible func(string) bool
	closureCompatible = func(identity string) bool {
		if visiting[identity] {
			return true
		}
		resource, ok := resources[identity]
		if !ok || resource.Kind == "notice" {
			return false
		}
		if resource.Kind != "asset" && !included[identity] {
			return false
		}
		visiting[identity] = true
		defer delete(visiting, identity)
		for _, dependency := range resource.Requires {
			if !closureCompatible(dependency) {
				return false
			}
		}
		return true
	}
	for identity := range included {
		if !closureCompatible(identity) {
			return CompatibilityBlocked
		}
	}
	return result
}

func (p ReconciliationPlan) LifecycleContract() LifecycleContract {
	contract := LifecycleContractFor(p.pack, p.surface, p.aliases)
	contract.ResourceGraph = ResourceGraphFor(p.pack, p.selection, false)
	if p.selectionValidity.Roots != nil {
		contract.SelectionValidity = p.selectionValidity
	}
	if contract.CompatibilityObserved && len(p.blockers) > 0 {
		contract.Compatibility = CompatibilityBlocked
	}
	return contract
}

func sortedUnique(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

type JSONLifecyclePhase struct {
	Kind             ConsentKind        `json:"kind"`
	Digest           string             `json:"digest"`
	ApprovalRequired bool               `json:"approval_required"`
	Actions          []ProjectionAction `json:"actions"`
}

type JSONLifecyclePlan struct {
	SchemaVersion          int                          `json:"schema_version"`
	Report                 string                       `json:"report"`
	PlanID                 string                       `json:"plan_id"`
	Operation              Operation                    `json:"operation"`
	Disposition            PlanDisposition              `json:"disposition"`
	Digest                 string                       `json:"digest"`
	Pack                   string                       `json:"pack"`
	PackVersion            string                       `json:"pack_version"`
	Surface                Surface                      `json:"surface"`
	IntentRevision         int                          `json:"intent_revision"`
	DocumentRevision       int                          `json:"document_revision"`
	Selection              ResourceSelection            `json:"selection"`
	ResourceGraph          ResourceGraph                `json:"resource_graph"`
	SensitiveEffects       []SensitiveEffectOrigin      `json:"sensitive_effects"`
	Contract               LifecycleContract            `json:"contract"`
	Aliases                []SurfaceAlias               `json:"aliases"`
	Contributors           map[string][]string          `json:"contributors"`
	Blockers               []PlanBlocker                `json:"blockers"`
	Phases                 []JSONLifecyclePhase         `json:"phases"`
	PendingHumanActions    []string                     `json:"pending_human_actions"`
	ExpectedReadiness      ReadinessStatus              `json:"expected_readiness"`
	ReadinessObserved      ReadinessObservationStatus   `json:"readiness_observed"`
	Evidence               []string                     `json:"evidence"`
	PendingEvidence        []string                     `json:"pending_evidence"`
	RuntimeModes           []RuntimeModeResult          `json:"runtime_modes,omitempty"`
	CapabilityRequirements []CapabilityRequirementFact  `json:"capability_requirements"`
	ProviderChoices        []ProviderChoice             `json:"provider_choices"`
	Recovery               bool                         `json:"recovery"`
	RecoveryGuidance       *RecoveryGuidance            `json:"recovery_guidance,omitempty"`
	MandatoryActions       []ProjectionAction           `json:"mandatory_actions"`
	ContractDiff           JSONContractDiff             `json:"contract_diff"`
	Migrations             []string                     `json:"migrations"`
	RetainedProjections    []RetainedProjection         `json:"retained_projections"`
	SharedProjections      []SharedProjectionVisibility `json:"shared_projections"`
	RemovedContributors    map[string]string            `json:"removed_contributors"`
	DryRun                 bool                         `json:"dry_run"`
}

type RecoveryGuidance struct {
	OriginatingOperation Operation                  `json:"originating_operation"`
	AffectedResources    []RecoveryAffectedResource `json:"affected_resources"`
	Consumers            []RecoveryConsumer         `json:"consumers"`
	Completed            []string                   `json:"completed"`
	FailedAction         string                     `json:"failed_action"`
	FailureDetail        string                     `json:"failure_detail"`
	NotStarted           []string                   `json:"not_started"`
	NextCommand          string                     `json:"next_command"`
}

func (p ReconciliationPlan) RecoveryGuidance() *RecoveryGuidance {
	history := p.HistoricalAttempt()
	if history == nil {
		return nil
	}
	affected := append([]RecoveryAffectedResource(nil), history.AffectedResources...)
	consumers := append([]RecoveryConsumer(nil), history.Consumers...)
	derivedAffected, derivedConsumers := p.recoverySubjects()
	if len(affected) == 0 {
		affected = derivedAffected
	}
	if len(consumers) == 0 {
		consumers = derivedConsumers
	}
	reconcileScope := history.ReconcileScope
	if history.Operation == OperationReconcile && reconcileScope == "" {
		reconcileScope = p.reconcileScope
	}
	return &RecoveryGuidance{
		OriginatingOperation: history.Operation,
		AffectedResources:    nonNilAffectedResources(affected),
		Consumers:            nonNilRecoveryConsumers(consumers),
		Completed:            nonNilStrings(history.Completed),
		FailedAction:         history.FailedAction,
		FailureDetail:        history.FailureDetail,
		NotStarted:           nonNilStrings(history.NotStarted()),
		NextCommand:          lifecycleCommand(history.Operation, history.PackID, history.Surface, reconcileScope),
	}
}

func nonNilAffectedResources(values []RecoveryAffectedResource) []RecoveryAffectedResource {
	if values == nil {
		return []RecoveryAffectedResource{}
	}
	return values
}

func nonNilRecoveryConsumers(values []RecoveryConsumer) []RecoveryConsumer {
	if values == nil {
		return []RecoveryConsumer{}
	}
	return values
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

type JSONContractDiff struct {
	Added    []string `json:"added"`
	Changed  []string `json:"changed"`
	Removed  []string `json:"removed"`
	Retained []string `json:"retained"`
}

func (p ReconciliationPlan) JSONReport(dryRun bool) JSONLifecyclePlan {
	phases := make([]JSONLifecyclePhase, 0, len(p.phases))
	mandatory := []ProjectionAction{}
	for _, phase := range p.Phases() {
		actions := append([]ProjectionAction{}, phase.Actions...)
		for i := range actions {
			actions[i] = actionForReport(actions[i])
		}
		phases = append(phases, JSONLifecyclePhase{Kind: phase.Kind, Digest: phase.Digest, ApprovalRequired: phase.ApprovalRequired, Actions: actions})
		mandatory = append(mandatory, actions...)
	}
	contributors := p.Contributors()
	if contributors == nil {
		contributors = map[string][]string{}
	}
	for id := range contributors {
		contributors[id] = sortedUnique(contributors[id])
	}
	blockers := append([]PlanBlocker{}, p.Blockers()...)
	sort.Slice(blockers, func(i, j int) bool {
		if blockers[i].Kind != blockers[j].Kind {
			return blockers[i].Kind < blockers[j].Kind
		}
		if blockers[i].Subject != blockers[j].Subject {
			return blockers[i].Subject < blockers[j].Subject
		}
		return blockers[i].Detail < blockers[j].Detail
	})
	contract := p.LifecycleContract()
	diff := lifecycleContractDiff(p.beforeCompositionFacts, p.compositionFacts)
	removed := p.RemovedContributors()
	if removed == nil {
		removed = map[string]string{}
	}
	retained := p.RetainedProjections()
	if retained == nil {
		retained = []RetainedProjection{}
	}
	shared := append([]SharedProjectionVisibility(nil), p.sharedProjections...)
	if shared == nil {
		shared = []SharedProjectionVisibility{}
	}
	selection, _ := canonicalSelection(p.selection)
	providerChoices := p.ProviderChoices()
	if providerChoices == nil {
		providerChoices = []ProviderChoice{}
	}
	return JSONLifecyclePlan{SchemaVersion: LifecycleJSONSchemaVersion, Report: "pack-lifecycle-preview", PlanID: p.id,
		Operation: p.operation, Disposition: p.Disposition(), Digest: p.digest, Pack: p.pack.ID, PackVersion: p.pack.Version,
		Surface: p.surface, IntentRevision: p.intentRevision, DocumentRevision: p.documentRevision, Selection: selection, ResourceGraph: ResourceGraphFor(p.pack, selection, false),
		SensitiveEffects: p.SensitiveEffects(), Contract: contract, Aliases: contract.Aliases,
		Contributors: contributors, Blockers: blockers, Phases: phases, PendingHumanActions: sortedCopy(p.pendingHumanActions),
		ExpectedReadiness: p.readiness, ReadinessObserved: p.readinessObserved, Evidence: sortedCopy(p.observedEvidence), PendingEvidence: sortedCopy(p.pendingEvidence),
		RuntimeModes:           sortedRuntimeModeResults(p.runtimeModeResults),
		CapabilityRequirements: p.CapabilityRequirements(),
		ProviderChoices:        providerChoices,
		Recovery:               p.recovery, RecoveryGuidance: p.RecoveryGuidance(), MandatoryActions: mandatory, ContractDiff: diff, Migrations: lifecycleMigrations(p),
		RetainedProjections: retained, SharedProjections: shared, RemovedContributors: removed, DryRun: dryRun}
}

func actionForReport(action ProjectionAction) ProjectionAction {
	// Host effects can carry complete merged documents so an adapter can apply
	// the sealed plan. Structured reports disclose the ordered redacted effect,
	// never raw owned or mixed-store content.
	originalSource, originalTarget, originalCommand := action.Source, action.Target, action.Command
	action.Content = ""
	action.Args = reportredaction.EnvironmentArguments(action.Args)
	action.Source = portableProjectionTarget(action.Source)
	action.Target = portableProjectionTarget(action.Target)
	action.Command = portableProjectionTarget(action.Command)
	replacements := []struct{ original, portable string }{
		{originalSource, action.Source},
		{originalTarget, action.Target},
		{originalCommand, action.Command},
	}
	sort.SliceStable(replacements, func(i, j int) bool {
		if len(replacements[i].original) != len(replacements[j].original) {
			return len(replacements[i].original) > len(replacements[j].original)
		}
		return replacements[i].original < replacements[j].original
	})
	for _, replacement := range replacements {
		if replacement.original != "" && replacement.original != replacement.portable {
			action.Description = strings.ReplaceAll(action.Description, replacement.original, replacement.portable)
		}
	}
	return action
}

func lifecycleContractDiff(before, after []Pack) JSONContractDiff {
	prior, next := map[string]string{}, map[string]string{}
	collect := func(target map[string]string, packs []Pack) {
		for _, pack := range packs {
			for _, resource := range pack.Resources {
				target[resource.Kind+":"+resource.ID] = digestJSON(resource)
			}
		}
	}
	collect(prior, before)
	collect(next, after)
	diff := JSONContractDiff{Added: []string{}, Changed: []string{}, Removed: []string{}, Retained: []string{}}
	for id, digest := range next {
		if old, ok := prior[id]; !ok {
			diff.Added = append(diff.Added, id)
		} else if old != digest {
			diff.Changed = append(diff.Changed, id)
		} else {
			diff.Retained = append(diff.Retained, id)
		}
	}
	for id := range prior {
		if _, ok := next[id]; !ok {
			diff.Removed = append(diff.Removed, id)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Changed)
	sort.Strings(diff.Removed)
	sort.Strings(diff.Retained)
	return diff
}

func lifecycleMigrations(p ReconciliationPlan) []string {
	result := append([]string{}, p.allModeContractChanges...)
	for _, migration := range p.rootMigrations {
		result = append(result, fmt.Sprintf("resource root migrates from %s to %s", migration.From.String(), migration.To.String()))
	}
	if digestJSON(p.previousAliases) != digestJSON(p.aliases) {
		result = append(result, "surface-local aliases change")
	}
	if p.operation == OperationDeactivate && p.previousSelection.Mode == SelectionAll && p.selection.Mode == SelectionCustom {
		result = append(result, "selection changes from all to custom; future resources are not selected automatically")
	}
	for _, blocker := range p.blockers {
		if blocker.Kind == BlockerAlias {
			result = append(result, blocker.Detail)
		}
	}
	return sortedUnique(result)
}

type JSONLifecycleFailure struct {
	SchemaVersion     int                `json:"schema_version"`
	Report            string             `json:"report"`
	Stage             string             `json:"stage"`
	Error             string             `json:"error"`
	Plan              *JSONLifecyclePlan `json:"plan,omitempty"`
	ActionsExecuted   *int               `json:"actions_executed,omitempty"`
	ApprovalRequested *bool              `json:"approval_requested,omitempty"`
}

func JSONFailureFor(stage string, err error, plan *ReconciliationPlan, approvalRequested *bool, actionsExecuted *int) JSONLifecycleFailure {
	err = ReportSafeError(err, plan)
	result := JSONLifecycleFailure{SchemaVersion: LifecycleJSONSchemaVersion, Report: "pack-lifecycle-failure", Stage: stage, Error: err.Error()}
	result.ApprovalRequested, result.ActionsExecuted = approvalRequested, actionsExecuted
	if plan != nil {
		report := plan.JSONReport(false)
		if errors.Is(err, ErrStalePlan) && report.Contract.CompatibilityObserved {
			report.Contract.Compatibility = CompatibilityBlocked
		}
		result.Plan = &report
	}
	return result
}

// ReportSafeError removes sealed action payloads and environment values from
// lifecycle diagnostics without changing their errors.Is/As identity.
func ReportSafeError(err error, plan *ReconciliationPlan) error {
	if plan == nil {
		return err
	}
	argumentSets := make([][]string, 0)
	sealedPayloads := make([]string, 0)
	for _, phase := range plan.phases {
		for _, action := range phase.Actions {
			argumentSets = append(argumentSets, action.Args)
			sealedPayloads = append(sealedPayloads, action.Content)
		}
	}
	return reportredaction.Error(err, argumentSets, sealedPayloads)
}

func reportSafeObservationText(value string, projections []ObservedProjection) string {
	argumentSets := make([][]string, 0, len(projections))
	sealedPayloads := make([]string, 0, len(projections))
	for _, projection := range projections {
		argumentSets = append(argumentSets, projection.Action.Args)
		sealedPayloads = append(sealedPayloads, projection.Action.Content)
	}
	return reportredaction.Text(value, argumentSets, sealedPayloads)
}

type JSONApplyResult struct {
	SchemaVersion       int               `json:"schema_version"`
	Report              string            `json:"report"`
	Plan                JSONLifecyclePlan `json:"plan"`
	Verified            bool              `json:"verified"`
	Projections         int               `json:"projections"`
	Readiness           JSONReadiness     `json:"readiness"`
	PendingHumanActions []string          `json:"pending_human_actions"`
}

func JSONApplyResultFor(plan ReconciliationPlan, applied ApplyResult) JSONApplyResult {
	return JSONApplyResult{SchemaVersion: LifecycleJSONSchemaVersion, Report: "pack-lifecycle-apply", Plan: plan.JSONReport(false),
		Verified: applied.Verified, Projections: applied.Projections,
		Readiness: JSONReadiness{
			Configured: optionalBool(applied.ReadinessObserved.Configured, applied.Readiness.Configured),
			Authorized: optionalBool(applied.ReadinessObserved.Authorization, applied.Readiness.Authorized),
			Usable:     optionalBool(applied.ReadinessObserved.Usability, applied.Readiness.Usable),
		}, PendingHumanActions: sortedCopy(applied.PendingHumanActions)}
}
