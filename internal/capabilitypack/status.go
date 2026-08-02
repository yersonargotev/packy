package capabilitypack

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
)

type StatusRequest struct {
	PackID        string
	Surface       Surface
	Resource      string
	RequireUsable bool
}

type IntentStatus struct {
	Active          bool
	Revision        int
	Version         string
	Selection       ResourceSelection
	ProviderChoices []ProviderChoice
}

type PackLifecycleState string

const (
	PackLifecycleActive                PackLifecycleState = "active"
	PackLifecycleInactiveClean         PackLifecycleState = "inactive-clean"
	PackLifecycleInactiveWithResiduals PackLifecycleState = "inactive-with-residuals"
	PackLifecycleRecoveryRequired      PackLifecycleState = "recovery-required"
)

type CapabilityConsumerFact struct {
	ConsumerPack     string
	ConsumerResource *ResourceIdentity
	Capability       string
}

type AttemptStatus struct {
	Outcome string
	PlanID  string
}

type ReadinessStatus struct {
	Configured bool
	Authorized bool
	Usable     bool
}

type ProjectionHealth string

const (
	ProjectionVerified  ProjectionHealth = "verified"
	ProjectionMissing   ProjectionHealth = "missing"
	ProjectionDrifted   ProjectionHealth = "drifted"
	ProjectionAmbiguous ProjectionHealth = "ambiguous"
	ProjectionUnmanaged ProjectionHealth = "unmanaged"
)

type ProjectionStatus struct {
	ID, Target, ObservedFingerprint, DesiredFingerprint string
	Health                                              ProjectionHealth
	Contributors                                        []string
	Owner                                               string
	Shared                                              bool
	DiscoverableBy                                      []Surface
	DiscoveryNotice                                     string
}

const sharedProjectionDiscoveryNotice = "shared standard targets may be discovered by compatible surfaces; discovery does not create activation intent"

type ProjectionSummary struct {
	Verified, Missing, Drifted, Ambiguous, Unmanaged int
}

type ResourceSelectionStatus struct {
	Resource        ResourceIdentity
	Selected        bool
	Role            ResourceRole
	DependencyChain []ResourceIdentity
}

type ResourceStatus struct {
	Resource          ResourceIdentity
	Role              ResourceRole
	DependencyChain   []ResourceIdentity
	Readiness         ReadinessStatus
	ReadinessObserved ReadinessObservationStatus
	Projections       ProjectionSummary
	Blockers          []string
}

// ReadinessObservation is fresh host-owned evidence. Observed distinguishes a
// negative observation from an adapter that cannot inspect that dimension.
type ReadinessObservation struct {
	AuthorizationObserved bool
	Authorized            bool
	UsabilityObserved     bool
	Usable                bool
	OptionalAuthorities   []OptionalAuthorityObservation
	PendingHumanActions   []string
	Evidence              []string
}

type OptionalAuthorityState string

const (
	OptionalAuthorityAvailable   OptionalAuthorityState = "available"
	OptionalAuthorityUnavailable OptionalAuthorityState = "unavailable"
	OptionalAuthorityUnknown     OptionalAuthorityState = "unknown"
)

// OptionalAuthorityObservation reports invocation-time authority separately
// from the required configured/authorized/usable readiness dimensions.
type OptionalAuthorityObservation struct {
	ModeID    string
	Authority string
	State     OptionalAuthorityState
	Fallback  string
}

// UnknownOptionalAuthorities returns one explicit unknown observation for every
// optional authority declared by the Pack contract.
func UnknownOptionalAuthorities(pack Pack) []OptionalAuthorityObservation {
	result := []OptionalAuthorityObservation{}
	for _, mode := range pack.Contract.OptionalModes {
		for _, authority := range mode.Authorities {
			result = append(result, OptionalAuthorityObservation{
				ModeID:    mode.ID,
				Authority: authority,
				State:     OptionalAuthorityUnknown,
				Fallback:  mode.Fallback,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ModeID != result[j].ModeID {
			return result[i].ModeID < result[j].ModeID
		}
		return result[i].Authority < result[j].Authority
	})
	return result
}

type StatusEntry struct {
	Pack                Pack
	Surface             Surface
	Intent              IntentStatus
	IntentPresent       bool
	UpdateAvailable     bool
	LatestAttempt       *AttemptStatus
	Readiness           ReadinessStatus
	ReadinessObserved   ReadinessObservationStatus
	OptionalAuthorities []OptionalAuthorityObservation
	RuntimeModes        []RuntimeModeResult
	Projections         ProjectionSummary
	ProjectionDetails   []ProjectionStatus
	ResourceSelections  []ResourceSelectionStatus
	Resources           []ResourceStatus
	Blockers            []string
	MissingRequirements []string
	InspectionFailed    bool
	PendingHumanActions []string
	Evidence            []string
	Contract            LifecycleContract
	ActivationRole      ActivationRole
	Consumers           []CapabilityConsumerFact
	LifecycleState      PackLifecycleState
}

type ReadinessObservationStatus struct {
	Configured    bool
	Authorization bool
	Usability     bool
}

type StatusRequirement struct {
	Resource  ResourceIdentity
	Readiness string
	Satisfied bool
}

type StatusReport struct {
	Entries             []StatusEntry
	ObservationFailures []Surface
	Focused             *ResourceStatus
	Requirement         *StatusRequirement
}

type ActiveIntentObservation struct {
	Intents        []ActivationIntent
	FailedSurfaces []Surface
}

// Facade is the single capability-pack use-case boundary consumed by the CLI.
type Facade struct {
	catalog    Catalog
	activation *activationDependencies
}

func NewFacade(catalog Catalog, options ...FacadeOption) Facade {
	// Package tests use in-memory catalogs to isolate lifecycle policy from
	// filesystem provenance. Discover always supplies a bundle root in runtime.
	if catalog.bundleRoot == "" && len(catalog.packs) > 0 {
		catalog.allowSyntheticHistory = true
	}
	facade := Facade{catalog: catalog}
	for _, option := range options {
		option(&facade)
	}
	return facade
}

func (f Facade) Status(ctx context.Context, request StatusRequest) (StatusReport, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (StatusReport, error) {
		return locked.status(ctx, request)
	})
}

// ActiveStatus reports fresh status only for explicitly active pack intents.
// It is the read-only summary seam used by Doctor; inactive catalog entries and
// residual ownership are deliberately not inspected.
func (f Facade) ActiveStatus(ctx context.Context) (StatusReport, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (StatusReport, error) {
		return locked.activeStatus(ctx)
	})
}

// ObserveActiveIntents reads durable activation intent without resolving or
// inspecting catalog packs. Per-surface failures remain detached health facts
// so Doctor can still render the most complete report possible.
func ObserveActiveIntents(ctx context.Context, store ActivationStore) ActiveIntentObservation {
	var observation ActiveIntentObservation
	for _, surface := range statusSurfaces() {
		state, err := store.Load(ctx, surface)
		if err != nil {
			observation.FailedSurfaces = append(observation.FailedSurfaces, surface)
			continue
		}
		for _, intent := range activeIntents(state) {
			if intent.Active && intent.Surface == surface {
				observation.Intents = append(observation.Intents, intent)
			}
		}
	}
	sort.Slice(observation.Intents, func(i, j int) bool {
		if observation.Intents[i].PackID != observation.Intents[j].PackID {
			return observation.Intents[i].PackID < observation.Intents[j].PackID
		}
		return observation.Intents[i].Surface < observation.Intents[j].Surface
	})
	return observation
}

func statusSurfaces() []Surface {
	return []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}
}

func (f Facade) activeStatus(ctx context.Context) (StatusReport, error) {
	if f.activation == nil || f.activation.store == nil {
		return StatusReport{}, fmt.Errorf("surface inspection is not configured")
	}
	type target struct {
		intent  ActivationIntent
		surface Surface
		state   ActivationState
	}
	var targets []target
	var report StatusReport
	for _, surface := range statusSurfaces() {
		state, err := f.activation.store.Load(ctx, surface)
		if err != nil {
			report.ObservationFailures = append(report.ObservationFailures, surface)
			continue
		}
		for _, intent := range activeIntents(state) {
			if !intent.Active || intent.Surface != surface {
				continue
			}
			targets = append(targets, target{intent: intent, surface: surface, state: state})
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].intent.PackID != targets[j].intent.PackID {
			return targets[i].intent.PackID < targets[j].intent.PackID
		}
		return targets[i].surface < targets[j].surface
	})

	for _, target := range targets {
		pack, err := f.catalog.catalogMetadata(target.intent.PackID)
		if err != nil {
			report.Entries = append(report.Entries, failedActiveStatusEntry(target.intent, target.surface))
			continue
		}
		entry, err := f.statusEntryWithState(ctx, pack, target.surface, activeOnlyStatusState(target.state))
		if err != nil {
			report.Entries = append(report.Entries, failedActiveStatusEntry(target.intent, target.surface))
			continue
		}
		report.Entries = append(report.Entries, entry)
	}
	return report, nil
}

func activeOnlyStatusState(state ActivationState) ActivationState {
	filtered := cloneActivationState(state)
	activePackIDs := map[string]bool{}
	filtered.Intents = filtered.Intents[:0]
	for _, intent := range activeIntents(state) {
		if !intent.Active {
			continue
		}
		activePackIDs[intent.PackID] = true
		filtered.Intents = append(filtered.Intents, intent)
	}
	if !filtered.Intent.Active {
		filtered.Intent = ActivationIntent{}
	}
	filtered.Ownership = filtered.Ownership[:0]
	for _, owner := range state.Ownership {
		copy := owner
		copy.Contributors = make([]string, 0, len(owner.Contributors))
		for _, contributor := range owner.Contributors {
			for packID := range activePackIDs {
				if contributor == packID || contributorBelongsToPack(contributor, packID) {
					copy.Contributors = append(copy.Contributors, contributor)
					break
				}
			}
		}
		if len(copy.Contributors) > 0 {
			filtered.Ownership = append(filtered.Ownership, copy)
		}
	}
	return filtered
}

func failedActiveStatusEntry(intent ActivationIntent, surface Surface) StatusEntry {
	return StatusEntry{
		Pack:             Pack{ID: intent.PackID, Version: intent.Version},
		Surface:          surface,
		Intent:           IntentStatus{Active: true, Revision: intent.Revision, Version: intent.Version},
		IntentPresent:    true,
		InspectionFailed: true,
		LifecycleState:   PackLifecycleActive,
	}
}

func (f Facade) status(ctx context.Context, request StatusRequest) (StatusReport, error) {
	if request.Resource != "" && (request.PackID == "" || request.Surface == "") {
		return StatusReport{}, fmt.Errorf("a pack and --surface are required when a resource is specified")
	}
	if request.RequireUsable && (request.PackID == "" || request.Surface == "") {
		return StatusReport{}, fmt.Errorf("a pack and --surface are required for require-usable")
	}
	var focused ResourceIdentity
	if request.Resource != "" {
		var err error
		focused, err = ParseResourceIdentity(request.Resource)
		if err != nil {
			return StatusReport{}, fmt.Errorf("invalid resource %q: %w", request.Resource, err)
		}
	}
	packs := f.catalog.List()
	if request.PackID != "" {
		if request.Surface == "" {
			return StatusReport{}, fmt.Errorf("--surface is required when a pack is specified")
		}
		pack, err := f.catalog.catalogMetadata(request.PackID)
		if err != nil {
			return StatusReport{}, err
		}
		packs = []Pack{pack}
	} else if request.Surface != "" {
		return StatusReport{}, fmt.Errorf("a pack is required when --surface is specified")
	}
	var report StatusReport
	for _, pack := range packs {
		for _, surface := range pack.Surfaces {
			if request.Surface != "" && request.Surface != surface {
				continue
			}
			entry, err := f.statusEntry(ctx, pack, surface)
			if err != nil {
				return StatusReport{}, fmt.Errorf("inspect pack %q on %s: %w", pack.ID, surface, err)
			}
			report.Entries = append(report.Entries, entry)
		}
	}
	if request.Surface != "" && len(report.Entries) == 0 {
		return StatusReport{}, fmt.Errorf("pack %q does not support CLI surface %q", request.PackID, request.Surface)
	}
	if request.Resource != "" {
		for i := range report.Entries[0].Resources {
			resource := &report.Entries[0].Resources[i]
			if resource.Resource == focused {
				report.Focused = resource
				if request.RequireUsable {
					report.Requirement = &StatusRequirement{Resource: focused, Readiness: "usable", Satisfied: resource.Readiness.Usable}
				}
				return report, nil
			}
		}
		return StatusReport{}, fmt.Errorf("resource %q is unknown or unselected for capability pack %q on %s", focused.String(), request.PackID, request.Surface)
	}
	if request.RequireUsable {
		report.Requirement = &StatusRequirement{Readiness: "usable", Satisfied: report.Entries[0].Readiness.Usable}
	}
	return report, nil
}

func (f Facade) statusEntry(ctx context.Context, pack Pack, surface Surface) (StatusEntry, error) {
	if f.activation == nil || f.activation.store == nil {
		return StatusEntry{}, fmt.Errorf("surface inspection is not configured")
	}
	state, err := f.activation.store.Load(ctx, surface)
	if err != nil {
		return StatusEntry{}, err
	}
	return f.statusEntryWithState(ctx, pack, surface, state)
}

func (f Facade) statusEntryWithState(ctx context.Context, pack Pack, surface Surface, state ActivationState) (StatusEntry, error) {
	adapter := f.activation.adapters[surface]
	if adapter == nil {
		return StatusEntry{}, fmt.Errorf("no activation adapter configured for CLI surface %q", surface)
	}
	entry := StatusEntry{Pack: pack, Surface: surface}
	var err error
	evidencePack := pack
	ownedResidual := hasContributor(state.Ownership, pack.ID)
	selection := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
	if intent, ok := intentForPack(state, pack.ID, surface); ok {
		selection, err = canonicalSelection(intent.Selection)
		if err != nil {
			return StatusEntry{}, err
		}
		entry.Contract = LifecycleContractFor(pack, surface, intent.Aliases)
		entry.Intent = IntentStatus{Active: intent.Active, Revision: intent.Revision, Version: intent.Version, Selection: selection, ProviderChoices: cloneProviderChoices(intent.ProviderChoices)}
		explicit, required := intentIsExplicit(intent), false
		for _, consumer := range activeIntents(state) {
			if !consumer.Active {
				continue
			}
			consumerPack, resolveErr := f.catalog.resolveIntentPack(consumer.PackID, consumer.Version)
			if resolveErr != nil {
				return StatusEntry{}, fmt.Errorf("resolve capability consumer %q: %w", consumer.PackID, resolveErr)
			}
			consumerPack, resolveErr = selectPackResources(consumerPack, consumer.Selection)
			if resolveErr != nil {
				return StatusEntry{}, fmt.Errorf("active consumer pack %q has invalid resource selection: %w", consumer.PackID, resolveErr)
			}
			if resolveErr = f.validatePersistedProviderChoices(consumerPack, consumer, state, surface); resolveErr != nil {
				return StatusEntry{}, resolveErr
			}
			for _, choice := range consumer.ProviderChoices {
				if choice.ProviderPack != pack.ID {
					continue
				}
				required = true
				for _, requirement := range capabilityRequirements(consumerPack) {
					if requirement.capability == choice.Capability {
						entry.Consumers = append(entry.Consumers, CapabilityConsumerFact{ConsumerPack: consumer.PackID, ConsumerResource: optionalResourceIdentity(requirement.resource), Capability: choice.Capability})
					}
				}
			}
		}
		switch {
		case !intent.Active:
			entry.ActivationRole = ActivationInactive
		case explicit && required:
			entry.ActivationRole = ActivationExplicitRequired
		case required:
			entry.ActivationRole = ActivationRequired
		default:
			entry.ActivationRole = ActivationExplicit
		}
		sort.Slice(entry.Consumers, func(i, j int) bool {
			if entry.Consumers[i].ConsumerPack != entry.Consumers[j].ConsumerPack {
				return entry.Consumers[i].ConsumerPack < entry.Consumers[j].ConsumerPack
			}
			if entry.Consumers[i].Capability != entry.Consumers[j].Capability {
				return entry.Consumers[i].Capability < entry.Consumers[j].Capability
			}
			left, right := "", ""
			if entry.Consumers[i].ConsumerResource != nil {
				left = entry.Consumers[i].ConsumerResource.String()
			}
			if entry.Consumers[j].ConsumerResource != nil {
				right = entry.Consumers[j].ConsumerResource.String()
			}
			return left < right
		})
		entry.IntentPresent = true
		entry.UpdateAvailable = intent.Active && intent.Version != pack.Version
		if intent.Active || ownedResidual {
			evidencePack, err = f.catalog.resolveIntentPack(intent.PackID, intent.Version)
			if err != nil {
				return StatusEntry{}, err
			}
		} else if evidencePack, err = f.catalog.Show(pack.ID); err != nil {
			return StatusEntry{}, err
		}
	} else if evidencePack, err = f.catalog.Show(pack.ID); err != nil {
		return StatusEntry{}, err
	}
	if entry.Contract.DependencyClosure == nil {
		entry.Contract = LifecycleContractFor(pack, surface, nil)
	}
	entry.ResourceSelections = resourceSelectionFacts(evidencePack, selection, entry.Intent.Active || ownedResidual)
	graph := ResourceGraphFor(evidencePack, selection, true)
	facts := make(map[string]ResourceClosureFact, len(graph.Resources))
	for _, fact := range graph.Resources {
		facts[fact.Resource.String()] = fact
	}
	for i := range entry.ResourceSelections {
		fact := facts[entry.ResourceSelections[i].Resource.String()]
		entry.ResourceSelections[i].Role = fact.Role
		entry.ResourceSelections[i].DependencyChain = append([]ResourceIdentity{}, fact.DependencyChain...)
		if !entry.Intent.Active && !ownedResidual {
			entry.ResourceSelections[i].Role = ResourceRoleUnselected
			entry.ResourceSelections[i].DependencyChain = []ResourceIdentity{}
		}
	}
	entry.LatestAttempt = latestAttemptStatus(state, pack.ID, surface)
	surfaceComposition, err := f.compose(evidencePack, state, surface, true)
	if err != nil {
		return StatusEntry{}, err
	}
	selectedEvidencePack, err := selectPackResources(evidencePack, selection)
	if err != nil {
		return StatusEntry{}, err
	}
	relevantPack, err := f.statusEvidencePack(selectedEvidencePack, surface)
	if err != nil {
		return StatusEntry{}, err
	}
	resolutions, err := f.resolveExecutables(ctx, relevantPack)
	if err != nil {
		entry.Blockers = append(entry.Blockers, err.Error())
		resolutions = nil
	}
	for _, resolution := range resolutions {
		if !resolution.Available {
			entry.MissingRequirements = append(entry.MissingRequirements, resolution.Tool)
			entry.Blockers = append(entry.Blockers, fmt.Sprintf("required executable %s is missing", resolution.Tool))
			if entry.Intent.Active {
				entry.PendingHumanActions = append(entry.PendingHumanActions, fmt.Sprintf("install %s and rerun status; Packy will not install it during Status", resolution.Tool))
			}
		}
	}
	observation, inspectErr := inspectSurface(ctx, adapter, SurfaceTransition{Desired: relevantPack, CurrentOwnership: state.Ownership, ResolvedExecutables: resolutions})
	if inspectErr != nil {
		return StatusEntry{}, inspectErr
	}
	entry.LifecycleState = lifecycleStateForStatus(entry, state, pack.ID, observation.Projections)
	entry.ProjectionDetails, entry.Projections = deriveProjectionStatus(pack.ID, observation.Projections, state.Ownership, surfaceComposition)
	entry.RuntimeModes = cloneRuntimeModeResults(observation.RuntimeModeResults)
	entry.Readiness.Configured = entry.Projections.Verified == len(observation.Projections) && len(observation.Projections) > 0
	entry.ReadinessObserved.Configured = true
	for _, detail := range entry.ProjectionDetails {
		entry.Evidence = append(entry.Evidence, fmt.Sprintf("%s: %s observed=%s desired=%s target=%s", detail.ID, detail.Health, detail.ObservedFingerprint, detail.DesiredFingerprint, detail.Target))
		if detail.Health != ProjectionVerified {
			entry.Blockers = append(entry.Blockers, fmt.Sprintf("%s is %s", detail.ID, detail.Health))
		}
	}
	fresh := observation.Readiness
	if entry.Readiness.Configured {
		entry.PendingHumanActions = append(entry.PendingHumanActions, fresh.PendingHumanActions...)
	}
	entry.Evidence = append(entry.Evidence, fresh.Evidence...)
	entry.ReadinessObserved.Authorization = fresh.AuthorizationObserved
	entry.ReadinessObserved.Usability = fresh.UsabilityObserved
	entry.OptionalAuthorities = cloneOptionalAuthorities(fresh.OptionalAuthorities)
	entry.Readiness.Authorized = entry.Readiness.Configured && fresh.AuthorizationObserved && fresh.Authorized
	entry.Readiness.Usable = entry.Readiness.Authorized && fresh.UsabilityObserved && fresh.Usable
	if entry.Intent.Active {
		entry.Resources = deriveResourceStatuses(pack.ID, graph, entry.ProjectionDetails, fresh)
	}
	if len(entry.Resources) > 0 {
		entry.Readiness = entry.Resources[0].Readiness
		for _, resource := range entry.Resources[1:] {
			entry.Readiness.Configured = entry.Readiness.Configured && resource.Readiness.Configured
			entry.Readiness.Authorized = entry.Readiness.Authorized && resource.Readiness.Authorized
			entry.Readiness.Usable = entry.Readiness.Usable && resource.Readiness.Usable
		}
	}
	if entry.Readiness.Configured && len(fresh.PendingHumanActions) == 0 {
		entry.PendingHumanActions = append(entry.PendingHumanActions, observation.PendingHumanActions...)
	}
	if entry.Readiness.Configured && !entry.Readiness.Authorized {
		entry.Blockers = append(entry.Blockers, "authorization/trust is not freshly demonstrated")
	}
	if entry.Readiness.Authorized && !entry.Readiness.Usable {
		entry.Blockers = append(entry.Blockers, "runtime usability is not freshly demonstrated")
	}
	sort.Strings(entry.Blockers)
	sort.Strings(entry.MissingRequirements)
	sort.Strings(entry.PendingHumanActions)
	sort.Strings(entry.Evidence)
	return entry, nil
}

func lifecycleStateForStatus(entry StatusEntry, state ActivationState, packID string, projections []ObservedProjection) PackLifecycleState {
	if entry.LatestAttempt != nil && AttemptOutcome(entry.LatestAttempt.Outcome) == AttemptRecoveryRequired {
		return PackLifecycleRecoveryRequired
	}
	if entry.IntentPresent && entry.Intent.Active {
		return PackLifecycleActive
	}
	if hasContributor(state.Ownership, packID) {
		observed := make(map[string]string, len(projections))
		for _, projection := range projections {
			provenance := projection.AdapterProvenance
			if provenance == "" {
				provenance = projection.Action.AdapterProvenance
			}
			observed[projection.ID] = provenance
		}
		for _, owner := range state.Ownership {
			relevant := false
			for _, contributor := range owner.Contributors {
				if contributorBelongsToPack(contributor, packID) {
					relevant = true
					break
				}
			}
			fresh, inspectable := observed[owner.ID]
			if relevant && (owner.AdapterProvenance == "" || !inspectable || fresh == "" || owner.AdapterProvenance != fresh) {
				return PackLifecycleRecoveryRequired
			}
		}
		return PackLifecycleInactiveWithResiduals
	}
	return PackLifecycleInactiveClean
}

func (f Facade) validatePersistedProviderChoices(consumerPack Pack, intent ActivationIntent, state ActivationState, surface Surface) error {
	choices, err := canonicalProviderChoices(intent.ProviderChoices)
	if err != nil {
		return fmt.Errorf("active consumer pack %q has invalid provider choices: %w", intent.PackID, err)
	}
	required := map[string]bool{}
	for _, requirement := range capabilityRequirements(consumerPack) {
		required[requirement.capability] = true
	}
	for _, choice := range choices {
		if !required[choice.Capability] {
			return fmt.Errorf("active consumer pack %q has stale provider choice for unselected capability %q", intent.PackID, choice.Capability)
		}
		providerIntent, ok := intentForPack(state, choice.ProviderPack, surface)
		if !ok || !providerIntent.Active {
			return fmt.Errorf("active consumer pack %q has invalid persisted provider choice: provider pack %q is not active", intent.PackID, choice.ProviderPack)
		}
		providerPack, err := f.catalog.resolveIntentPack(providerIntent.PackID, providerIntent.Version)
		if err == nil {
			providerPack, err = selectPackResources(providerPack, providerIntent.Selection)
		}
		if err != nil {
			return fmt.Errorf("active consumer pack %q has invalid persisted provider choice: provider pack %q selection is invalid: %w", intent.PackID, choice.ProviderPack, err)
		}
		eligible := providersInPack(providerPack, choice.Capability)
		if _, err := selectChosenProvider(choice, eligible); err != nil {
			return fmt.Errorf("active consumer pack %q has invalid persisted provider choice: %w", intent.PackID, err)
		}
	}
	return nil
}

func providersInPack(pack Pack, capability string) []capabilityProvider {
	if pack.manifestVersion != manifestSchemaV4 {
		if containsString(pack.Provides, capability) {
			return []capabilityProvider{{pack: pack}}
		}
		return nil
	}
	var result []capabilityProvider
	for _, resource := range pack.Resources {
		if resource.Kind != "notice" && containsString(resource.ProvidesCapabilities, capability) {
			result = append(result, capabilityProvider{pack: pack, resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}})
		}
	}
	return result
}

func deriveResourceStatuses(packID string, graph ResourceGraph, projections []ProjectionStatus, fresh ReadinessObservation) []ResourceStatus {
	result := make([]ResourceStatus, 0, len(graph.Resources))
	allConfigured := len(projections) > 0
	for _, projection := range projections {
		allConfigured = allConfigured && projection.Health == ProjectionVerified
	}
	for _, fact := range graph.Resources {
		if fact.Role == ResourceRoleUnselected {
			continue
		}
		contributor := resourceContributor(packID, fact.Resource)
		status := ResourceStatus{Resource: fact.Resource, Role: fact.Role, DependencyChain: append([]ResourceIdentity{}, fact.DependencyChain...)}
		for _, projection := range projections {
			if containsString(projection.Contributors, contributor) {
				addProjectionHealth(&status.Projections, projection.Health)
				if projection.Health != ProjectionVerified {
					status.Blockers = append(status.Blockers, fmt.Sprintf("%s is %s", projection.ID, projection.Health))
				}
			}
		}
		covered := status.Projections.Verified+status.Projections.Missing+status.Projections.Drifted+status.Projections.Ambiguous+status.Projections.Unmanaged > 0
		status.Readiness.Configured = allConfigured
		if covered {
			status.Readiness.Configured = status.Projections.Verified > 0 &&
				status.Projections.Verified == status.Projections.Verified+status.Projections.Missing+status.Projections.Drifted+status.Projections.Ambiguous+status.Projections.Unmanaged
		}
		status.ReadinessObserved = ReadinessObservationStatus{Configured: true, Authorization: fresh.AuthorizationObserved, Usability: fresh.UsabilityObserved}
		status.Readiness.Authorized = status.Readiness.Configured && fresh.AuthorizationObserved && fresh.Authorized
		status.Readiness.Usable = status.Readiness.Authorized && fresh.UsabilityObserved && fresh.Usable
		sort.Strings(status.Blockers)
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Resource.String() < result[j].Resource.String() })
	return result
}

func addProjectionHealth(summary *ProjectionSummary, health ProjectionHealth) {
	switch health {
	case ProjectionVerified:
		summary.Verified++
	case ProjectionMissing:
		summary.Missing++
	case ProjectionDrifted:
		summary.Drifted++
	case ProjectionAmbiguous:
		summary.Ambiguous++
	case ProjectionUnmanaged:
		summary.Unmanaged++
	}
}

// statusEvidencePack excludes unrelated active packs while retaining the
// requested pack's dependency closure.
func (f Facade) statusEvidencePack(pack Pack, surface Surface) (Pack, error) {
	composition, err := f.compose(pack, ActivationState{}, surface, false)
	if err != nil {
		return Pack{}, err
	}
	return composition.combinedPack(), nil
}

func deriveProjectionStatus(packID string, observed []ObservedProjection, ownership []ProjectionOwnership, c composition) ([]ProjectionStatus, ProjectionSummary) {
	result := make([]ProjectionStatus, 0, len(observed))
	var summary ProjectionSummary
	for _, p := range observed {
		status := ProjectionStatus{ID: p.ID, Target: portableProjectionTarget(p.Action.Target), ObservedFingerprint: p.ObservedFingerprint, DesiredFingerprint: p.DesiredFingerprint, Contributors: c.contributorSet(p.ID), Shared: p.Shared || p.Action.Shared, DiscoverableBy: append([]Surface(nil), p.DiscoverableBy...)}
		if len(status.DiscoverableBy) == 0 {
			status.DiscoverableBy = append([]Surface(nil), p.Action.DiscoverableBy...)
		}
		if status.Shared {
			status.DiscoveryNotice = sharedProjectionDiscoveryNotice
		}
		owner, owned := ownershipByID(ownership, physicalProjectionID(c.surface, p))
		if !owned {
			owner, owned = ownershipByID(ownership, projectionOwnershipID(p))
		}
		if p.ExternallyManaged {
			status.Owner = "external"
		} else if owned {
			status.Owner = "packy"
		} else {
			status.Owner = "unmanaged"
		}
		switch {
		case !p.Exists:
			status.Health = ProjectionMissing
			summary.Missing++
		case p.ExternallyManaged && p.ObservedFingerprint == p.DesiredFingerprint:
			status.Health = ProjectionVerified
			summary.Verified++
		case p.ExternallyManaged:
			status.Health = ProjectionDrifted
			summary.Drifted++
		case p.ObservedFingerprint != p.DesiredFingerprint && owned:
			status.Health = ProjectionDrifted
			summary.Drifted++
		case p.ObservedFingerprint != p.DesiredFingerprint:
			status.Health = ProjectionUnmanaged
			summary.Unmanaged++
		case !owned:
			status.Health = ProjectionUnmanaged
			summary.Unmanaged++
		case owner.Fingerprint != p.DesiredFingerprint || !contributorsMatchForSurface(owner.Contributors, c.surface, status.Contributors):
			status.Health = ProjectionAmbiguous
			summary.Ambiguous++
		default:
			status.Health = ProjectionVerified
			summary.Verified++
		}
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, summary
}

func portableProjectionTarget(target string) string {
	if target == "" || !filepath.IsAbs(target) {
		return target
	}
	return filepath.Join("<host-path>", filepath.Base(filepath.Clean(target)))
}

func latestAttemptStatus(state ActivationState, packID string, surface Surface) *AttemptStatus {
	var candidate *ApplyingJournal
	for i := range state.History {
		if state.History[i].PackID == packID && state.History[i].Surface == surface {
			candidate = &state.History[i]
		}
	}
	for i := range state.LastAttempts {
		if state.LastAttempts[i].PackID == packID && state.LastAttempts[i].Surface == surface {
			candidate = &state.LastAttempts[i]
		}
	}
	if state.Journal != nil && state.Journal.PackID == packID && state.Journal.Surface == surface {
		candidate = state.Journal
	}
	if candidate == nil {
		return nil
	}
	return &AttemptStatus{Outcome: string(candidate.Outcome), PlanID: candidate.PlanID}
}
