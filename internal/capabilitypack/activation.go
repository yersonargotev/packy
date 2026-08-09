package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

var (
	ErrInteractiveRequired = errors.New("Apply requires an interactive terminal")
	ErrApprovalMismatch    = errors.New("approval does not match the exact plan")
	ErrStalePlan           = errors.New("reconciliation plan is stale")
	ErrVerificationFailed  = errors.New("fresh verification did not match desired state")
	ErrPlanNotActionable   = errors.New("lifecycle plan is not actionable")
)

type PlanDisposition string

const (
	PlanConverged    PlanDisposition = "converged"
	PlanApplicable   PlanDisposition = "applicable"
	PlanMixed        PlanDisposition = "mixed"
	PlanBlocked      PlanDisposition = "blocked"
	PlanPendingHuman PlanDisposition = "pending-human-actions"
)

type PlanNotActionableError struct{ Disposition PlanDisposition }

func (e PlanNotActionableError) Error() string {
	return fmt.Sprintf("%s: %s", ErrPlanNotActionable, e.Disposition)
}
func (e PlanNotActionableError) Unwrap() error { return ErrPlanNotActionable }

type ConsentKind string
type Operation string
type ProjectionActionKind string
type ProjectionActionMode string

const (
	ConsentReversibleLocal         ConsentKind          = "reversible-local"
	ConsentExecutableExternal      ConsentKind          = "executable-external"
	ConsentToolHostSetup           ConsentKind          = "tool-host-setup"
	ConsentHostFollowUp            ConsentKind          = "host-follow-up"
	ConsentDestructiveCleanup      ConsentKind          = "destructive-cleanup"
	OperationActivate              Operation            = "activate"
	OperationUpdate                Operation            = "update"
	OperationDeactivate            Operation            = "deactivate"
	ActionSkillLink                ProjectionActionKind = "skill-link"
	ActionInstructionFile          ProjectionActionKind = "instruction-file"
	ActionOpenCodeSkillLink        ProjectionActionKind = "opencode-skill-link"
	ActionOpenCodeInstructionFile  ProjectionActionKind = "opencode-instruction-file"
	ActionOpenCodeConfigReference  ProjectionActionKind = "opencode-config-reference"
	ActionCodexMCPConfig           ProjectionActionKind = "codex-mcp-config"
	ActionCodexProjectTrust        ProjectionActionKind = "codex-project-trust"
	ActionCodexAgentFile           ProjectionActionKind = "codex-agent-file"
	ActionCodexWorkflowSkill       ProjectionActionKind = "codex-workflow-skill"
	ActionCodexAssetFile           ProjectionActionKind = "codex-asset-file"
	ActionCodexProjectSkillTree    ProjectionActionKind = "codex-project-skill-tree"
	ActionClaudeProjectSkillTree   ProjectionActionKind = "claude-project-skill-tree"
	ActionClaudeProjectFile        ProjectionActionKind = "claude-project-file"
	ActionClaudeProjectInstruction ProjectionActionKind = "claude-project-instruction"
	ActionClaudeProjectMCP         ProjectionActionKind = "claude-project-mcp"
	ActionProjectManifestFile      ProjectionActionKind = "project-manifest-file"
	ActionProjectLockFile          ProjectionActionKind = "project-lock-file"
	ActionProjectNoticesFile       ProjectionActionKind = "project-notices-file"
	ActionOpenCodeMCPConfig        ProjectionActionKind = "opencode-mcp-config"
	ActionOpenCodeAgentFile        ProjectionActionKind = "opencode-agent-file"
	ActionOpenCodeCommandFile      ProjectionActionKind = "opencode-command-file"
	ActionOpenCodeAssetFile        ProjectionActionKind = "opencode-asset-file"
	ActionExternalCommand          ProjectionActionKind = "external-command"
	ActionHostFollowUp             ProjectionActionKind = "host-follow-up"
	ProjectionRemoveContent        ProjectionActionMode = "remove-content"
	ProjectionDeleteTarget         ProjectionActionMode = "delete-target"
)

type StalePlanError struct{ Precondition string }

func (e StalePlanError) Error() string { return fmt.Sprintf("%s: %s", ErrStalePlan, e.Precondition) }
func (e StalePlanError) Unwrap() error { return ErrStalePlan }

type ActivationRequest struct {
	PackID    string
	Surface   Surface
	Aliases   []SurfaceAlias
	Selection ResourceSelection
}

type UpdateRequest struct {
	PackID  string
	Surface Surface
	Aliases []SurfaceAlias
	Force   bool
}

type DeactivationRequest struct {
	PackID    string
	Surface   Surface
	Resources []ResourceIdentity
	Force     bool
}

// ExecutableResolution is the immutable fact set used to choose an external
// command. It intentionally contains no credentials or tool-owned data.
type ExecutableResolution struct {
	Tool                 string                `json:"tool"`
	Available            bool                  `json:"available"`
	Path                 string                `json:"path"`
	ResolvedPath         string                `json:"resolved_path"`
	Origin               string                `json:"origin"`
	Version              string                `json:"version,omitempty"`
	AcquisitionSupported bool                  `json:"acquisition_supported"`
	AcquisitionCommand   string                `json:"acquisition_command,omitempty"`
	AcquisitionArgs      []string              `json:"acquisition_args,omitempty"`
	AcquisitionSource    string                `json:"acquisition_source,omitempty"`
	AcquisitionVersion   string                `json:"acquisition_version,omitempty"`
	Precondition         string                `json:"precondition"`
	Capability           SurfaceCapabilityType `json:"-"`
}

type ExecutableAcquisition struct {
	Path    string
	Command string
	Args    []string
	Source  string
	Version string
}

// ExecutableResolver is owned by capabilitypack; the generic PATH observer is
// composed by the CLI at the edge of the application.
type ExecutableResolver interface {
	Resolve(context.Context, string) (ExecutableResolution, error)
}

// ExecutableAcquirer resolves one explicitly reviewed tool capability. It is
// separate from PATH observation so ordinary requirements never inherit a
// tool-specific installation or setup convention.
type ExecutableAcquirer interface {
	ResolveAcquisition(context.Context) (ExecutableAcquisition, error)
}

// ExternalExecutor is the only side-effect seam for executable/external
// actions. The facade supplies exact sealed actions; it never asks the
// executor to discover or construct a command.
type ExternalExecutor interface {
	Execute(context.Context, ProjectionAction) error
}

// ProjectionAction is an adapter-produced, host-specific local projection.
// Capability-pack policy orders and approves it; only the matching adapter executes it.
type ProjectionAction struct {
	ID                      string               `json:"id"`
	Surface                 Surface              `json:"surface,omitempty"`
	Description             string               `json:"description"`
	Kind                    ProjectionActionKind `json:"kind,omitempty"`
	Consent                 ConsentKind          `json:"consent,omitempty"`
	Source                  string               `json:"source,omitempty"`
	Target                  string               `json:"target,omitempty"`
	Content                 string               `json:"content,omitempty"`
	Command                 string               `json:"command,omitempty"`
	Args                    []string             `json:"args,omitempty"`
	Version                 string               `json:"version,omitempty"`
	FileMode                uint32               `json:"file_mode,omitempty"`
	Precondition            string               `json:"precondition,omitempty"`
	Consequences            string               `json:"consequences,omitempty"`
	RollbackLimits          string               `json:"rollback_limits,omitempty"`
	Mode                    ProjectionActionMode `json:"mode,omitempty"`
	AdapterProvenance       string               `json:"adapter_provenance,omitempty"`
	ContributionStartMarker string               `json:"contribution_start_marker,omitempty"`
	ContributionEndMarker   string               `json:"contribution_end_marker,omitempty"`
	// ProjectionKey identifies the physical target when several surface
	// adapters translate different logical projection IDs to one shared target.
	ProjectionKey  string    `json:"projection_key,omitempty"`
	Shared         bool      `json:"shared,omitempty"`
	DiscoverableBy []Surface `json:"discoverable_by,omitempty"`
	PreviewOnly    bool      `json:"preview_only,omitempty"`
}

func RemovalCandidate(projection ObservedProjection, mode ProjectionActionMode, content, description string) ObservedProjection {
	projection.Goal = ProjectionAbsent
	projection.DesiredFingerprint = ""
	projection.Action.Source = ""
	projection.Action.Content = content
	projection.Action.Mode = mode
	projection.Action.Description = description
	return projection
}

type ObservedProjection struct {
	ID                  string
	Goal                ProjectionGoal
	Exists              bool
	ObservedFingerprint string
	// ExactFingerprint seals the exact externally managed contribution while
	// ObservedFingerprint may represent a normalized semantic contract. Local
	// projections leave it empty because their observed fingerprint is exact.
	ExactFingerprint   string
	DesiredFingerprint string
	AdapterProvenance  string
	ProjectionKey      string
	Shared             bool
	DiscoverableBy     []Surface
	ExternallyManaged  bool
	Action             ProjectionAction
}

type ProjectionGoal string

const (
	ProjectionPresent ProjectionGoal = "present"
	ProjectionAbsent  ProjectionGoal = "absent"
)

// SurfaceTransition is the complete, lifecycle-neutral input to host
// inspection. Capability-pack decides which facts are relevant to each use
// case; adapters only translate those facts into host projections.
type SurfaceTransition struct {
	Prior                 Pack
	Desired               Pack
	CurrentOwnership      []ProjectionOwnership
	ResidualOwnership     []ProjectionOwnership
	ResolvedExecutables   []ExecutableResolution
	ProjectRoot           string
	ProjectInstallation   *ProjectInstallation
	ProjectGoal           ProjectionGoal
	ProjectEffectReceipts []ProjectActivationEffectReceipt
}

type ProjectEffectState string

const (
	ProjectEffectAbsent  ProjectEffectState = "absent"
	ProjectEffectExact   ProjectEffectState = "exact"
	ProjectEffectDrifted ProjectEffectState = "drifted"
)

type ObservedProjectEffect struct {
	Kind                ProjectionActionKind
	Target              string
	State               ProjectEffectState
	ObservedFingerprint string
	AdapterProvenance   string
	Action              ProjectionAction
}

type SurfaceInspection struct {
	Revision string
	// ControlledCheck describes the host facts used to bind an explicit runtime
	// check. Empty fields are normalized by the capability-pack domain so older
	// adapters remain safe: they can never accidentally share evidence across a
	// later adapter or observable host identity.
	ControlledCheck            ControlledCheckDescriptor
	Projections                []ObservedProjection
	OccupiedNames              []OccupiedName
	RuntimeModeEvidence        []RuntimeModeEvidence
	RuntimeModeResults         []RuntimeModeResult
	Readiness                  ReadinessObservation
	PendingHumanActions        []string
	Unrepresentable            []UnrepresentableResource
	ProjectActivationActions   []ProjectionAction
	ProjectDeactivationEffects []ObservedProjectEffect
}

type UnrepresentableResource struct {
	Resource ResourceIdentity
	Reason   string
}

// OccupiedName is one freshly observed host namespace entry relied on by a
// surface plan. OwnerType is reserved, unmanaged, or packy.
type OccupiedName struct {
	Namespace   string
	Name        string
	OwnerType   string
	OwnerID     string
	Fingerprint string
}

type SurfaceAdapter interface {
	InspectSurface(context.Context, SurfaceTransition) (SurfaceInspection, error)
	ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError
}

type controlledCheckSurfaceAdapter struct {
	SurfaceAdapter
	descriptor ControlledCheckDescriptor
}

// WithControlledCheckDescriptor binds observable adapter and host facts to all
// inspection paths without moving readiness policy into a driving adapter.
func WithControlledCheckDescriptor(adapter SurfaceAdapter, descriptor ControlledCheckDescriptor) SurfaceAdapter {
	return controlledCheckSurfaceAdapter{SurfaceAdapter: adapter, descriptor: descriptor}
}

func (a controlledCheckSurfaceAdapter) controlledCheckDescriptor() ControlledCheckDescriptor {
	return a.descriptor
}

type ActivationIntent struct {
	PackID               string                `json:"pack_id"`
	Surface              Surface               `json:"surface"`
	Version              string                `json:"version"`
	Active               bool                  `json:"active"`
	Revision             int                   `json:"revision"`
	ReadinessObligations []ReadinessObligation `json:"readiness_obligations"`
	ExternalRequirements []string              `json:"external_requirements"`
	Aliases              []SurfaceAlias        `json:"aliases"`
	Selection            ResourceSelection     `json:"selection"`
	Resources            []ResourceIdentity    `json:"resources,omitempty"`
	Explicit             *bool                 `json:"explicit,omitempty"`
}

type SurfaceAlias struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProjectionOwnership struct {
	ID           string  `json:"id"`
	ProjectionID string  `json:"projection_id,omitempty"`
	PhysicalID   string  `json:"-"`
	Target       string  `json:"target,omitempty"`
	Fingerprint  string  `json:"fingerprint"`
	PackID       string  `json:"pack_id"`
	Surface      Surface `json:"surface"`
}

type ProjectionActionError struct {
	ID  string
	Err error
}

func (e ProjectionActionError) Error() string {
	return fmt.Sprintf("apply projection %s: %v", e.ID, e.Err)
}
func (e ProjectionActionError) Unwrap() error { return e.Err }

type ExternalEffect struct {
	ID          string                 `json:"id"`
	Fingerprint string                 `json:"fingerprint"`
	Receipt     *ExternalEffectReceipt `json:"receipt,omitempty"`
}

type ExternalEffectReceipt struct {
	SchemaVersion     int                      `json:"schema_version"`
	EffectID          string                   `json:"effect_id"`
	EffectFingerprint string                   `json:"effect_fingerprint"`
	Surface           Surface                  `json:"surface"`
	PackID            string                   `json:"pack_id"`
	Contributions     []ExternalContribution   `json:"contributions"`
	Reversal          ExternalReversalContract `json:"reversal"`
}

type ExternalContribution struct {
	ID                  string `json:"id"`
	ObservedFingerprint string `json:"observed_fingerprint"`
	AdapterProvenance   string `json:"adapter_provenance"`
}

type ExternalReversalContract struct {
	SchemaVersion   int         `json:"schema_version"`
	Consent         ConsentKind `json:"consent"`
	AuthorityLimits []string    `json:"authority_limits"`
}

type ActivationState struct {
	SchemaVersion      int                   `json:"schema_version"`
	Intent             ActivationIntent      `json:"intent"`
	Intents            []ActivationIntent    `json:"intents,omitempty"`
	Ownership          []ProjectionOwnership `json:"ownership,omitempty"`
	External           []ExternalEffect      `json:"external_effects,omitempty"`
	documentRevision   int
	snapshotManaged    bool
	externalCheckpoint bool
}

type ActivationStore interface {
	LoadSnapshot(context.Context, Surface) (ActivationState, error)
	SaveSnapshot(context.Context, Surface, int, ActivationState) (int, error)
}

func loadActivationState(ctx context.Context, store ActivationStore, surface Surface) (ActivationState, error) {
	return store.LoadSnapshot(ctx, surface)
}

func saveActivationState(ctx context.Context, store ActivationStore, surface Surface, state *ActivationState) error {
	revision, err := store.SaveSnapshot(ctx, surface, state.documentRevision, *state)
	if err == nil {
		state.documentRevision = revision
	}
	return err
}

func checkpointExternalEffects(ctx context.Context, store ActivationStore, surface Surface, state *ActivationState) error {
	state.externalCheckpoint = true
	err := saveActivationState(ctx, store, surface, state)
	state.externalCheckpoint = false
	return err
}

type activationDependencies struct {
	store     ActivationStore
	adapters  map[Surface]SurfaceAdapter
	resolver  ExecutableResolver
	acquirers map[SurfaceCapabilityType]ExecutableAcquirer
	executor  ExternalExecutor
}

type FacadeOption func(*Facade)

func WithActivation(store ActivationStore, adapters map[Surface]SurfaceAdapter) FacadeOption {
	return func(f *Facade) {
		var resolver ExecutableResolver
		var acquirers map[SurfaceCapabilityType]ExecutableAcquirer
		var executor ExternalExecutor
		if f.activation != nil {
			resolver = f.activation.resolver
			acquirers = f.activation.acquirers
			executor = f.activation.executor
		}
		f.activation = &activationDependencies{store: store, adapters: adapters, resolver: resolver, acquirers: acquirers, executor: executor}
	}
}

func WithExternalEffects(resolver ExecutableResolver, acquirers map[SurfaceCapabilityType]ExecutableAcquirer, executor ExternalExecutor) FacadeOption {
	return func(f *Facade) {
		if f.activation == nil {
			f.activation = &activationDependencies{}
		}
		f.activation.resolver = resolver
		f.activation.acquirers = acquirers
		f.activation.executor = executor
	}
}

// WithControlledCheckEvidence injects workstation-local controlled runtime
// evidence. It lets all facade reads, including Doctor's ActiveStatus, use
// the same personal store without coupling them to a CLI path convention.
func WithControlledCheckEvidence(store ControlledCheckEvidenceStore) FacadeOption {
	return func(f *Facade) { f.controlledChecks = store }
}

type PlanPhase struct {
	Kind             ConsentKind
	Digest           string
	ApprovalRequired bool
	Actions          []ProjectionAction
}

type ReconciliationPlan struct {
	id                     string
	digest                 string
	pack                   Pack
	operation              Operation
	surface                Surface
	intentRevision         int
	documentRevision       int
	oldVersion             string
	observationFingerprint string
	phases                 []PlanPhase
	desired                []projectionExpectation
	portable               []PortableOutcome
	resolutions            []ExecutableResolution
	runtimeModeResults     []RuntimeModeResult
	sensitiveEffects       []SensitiveEffectOrigin
	readiness              ReadinessStatus
	conditions             []ReadinessCondition
	observedEvidence       []string
	pendingEvidence        []string
	pendingHumanActions    []string
	noOp                   bool
	activations            []PlannedActivation
	blockers               []PlanBlocker
	compositionFacts       []Pack
	intentFacts            []ActivationIntent
	beforeIntentFacts      []ActivationIntent
	ownershipFacts         []ProjectionOwnership
	beforeCompositionFacts []Pack
	aliases                []SurfaceAlias
	previousAliases        []SurfaceAlias
	selection              ResourceSelection
	previousSelection      ResourceSelection
	partialSelection       bool
	selectionValidity      SelectionValidity
	force                  bool
}

type projectionExpectation struct {
	ID, Fingerprint   string
	ExternallyManaged bool
}
type PortableOutcome struct{ Kind, ID string }

func (p ReconciliationPlan) ID() string                   { return p.id }
func (p ReconciliationPlan) Digest() string               { return p.digest }
func (p ReconciliationPlan) Pack() Pack                   { return clonePack(p.pack) }
func (p ReconciliationPlan) Surface() Surface             { return p.surface }
func (p ReconciliationPlan) Operation() Operation         { return p.operation }
func (p ReconciliationPlan) Aliases() []SurfaceAlias      { return cloneAliases(p.aliases) }
func (p ReconciliationPlan) Selection() ResourceSelection { return cloneSelection(p.selection) }
func (p ReconciliationPlan) SensitiveEffects() []SensitiveEffectOrigin {
	return cloneSensitiveEffectOrigins(p.sensitiveEffects)
}
func (p ReconciliationPlan) OldVersion() string  { return p.oldVersion }
func (p ReconciliationPlan) IntentRevision() int { return p.intentRevision }
func (p ReconciliationPlan) NoOp() bool          { return p.noOp }
func (p ReconciliationPlan) Applicable() bool {
	return p.Disposition() == PlanApplicable || p.Disposition() == PlanConverged || p.Disposition() == PlanPendingHuman
}
func (p ReconciliationPlan) Disposition() PlanDisposition {
	actions := false
	for _, phase := range p.phases {
		if phase.ApprovalRequired && len(phase.Actions) > 0 {
			actions = true
			break
		}
	}
	if len(p.blockers) > 0 {
		if actions {
			return PlanMixed
		}
		return PlanBlocked
	}
	if actions {
		return PlanApplicable
	}
	if len(p.pendingHumanActions) > 0 {
		return PlanPendingHuman
	}
	return PlanConverged
}
func (p ReconciliationPlan) Activations() []PlannedActivation {
	result := append([]PlannedActivation(nil), p.activations...)
	for i := range result {
		result[i].Pack = clonePack(result[i].Pack)
	}
	return result
}
func (p ReconciliationPlan) Blockers() []PlanBlocker {
	return append([]PlanBlocker(nil), p.blockers...)
}
func (p ReconciliationPlan) PortableOutcomes() []PortableOutcome {
	return append([]PortableOutcome(nil), p.portable...)
}
func (p ReconciliationPlan) RuntimeModeResults() []RuntimeModeResult {
	return cloneRuntimeModeResults(p.runtimeModeResults)
}
func (p ReconciliationPlan) Phases() []PlanPhase {
	result := make([]PlanPhase, len(p.phases))
	for i, phase := range p.phases {
		result[i] = phase
		result[i].Actions = append([]ProjectionAction(nil), phase.Actions...)
		for j := range result[i].Actions {
			result[i].Actions[j].Args = append([]string(nil), result[i].Actions[j].Args...)
		}
	}
	return result
}

func (p ReconciliationPlan) Resolutions() []ExecutableResolution {
	result := append([]ExecutableResolution(nil), p.resolutions...)
	for i := range result {
		result[i].AcquisitionArgs = append([]string(nil), result[i].AcquisitionArgs...)
	}
	return result
}

func (p ReconciliationPlan) PendingHumanActions() []string {
	return append([]string(nil), p.pendingHumanActions...)
}

func (p ReconciliationPlan) Readiness() ReadinessStatus { return p.readiness }
func (p ReconciliationPlan) Conditions() []ReadinessCondition {
	return cloneReadinessConditions(p.conditions)
}
func (p ReconciliationPlan) Evidence() []string { return append([]string(nil), p.observedEvidence...) }
func (p ReconciliationPlan) PendingEvidence() []string {
	return append([]string(nil), p.pendingEvidence...)
}

type ApprovalReceipt struct {
	planDigest, phaseDigest string
	kind                    ConsentKind
}

type ApplyRequest struct {
	Plan        ReconciliationPlan
	Approvals   []ApprovalReceipt
	Interactive bool
}

type ApplyResult struct {
	Verified            bool
	PlanID              string
	Projections         int
	Readiness           ReadinessStatus
	Conditions          []ReadinessCondition
	PendingHumanActions []string
}

func (f Facade) Preview(ctx context.Context, request ActivationRequest) (ReconciliationPlan, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (ReconciliationPlan, error) {
		return locked.preview(ctx, request, OperationActivate, "", false)
	})
}

func (f Facade) PreviewUpdate(ctx context.Context, request UpdateRequest) (ReconciliationPlan, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (ReconciliationPlan, error) {
		return locked.previewUpdate(ctx, request)
	})
}

func (f Facade) previewUpdate(ctx context.Context, request UpdateRequest) (ReconciliationPlan, error) {
	activation := ActivationRequest{PackID: request.PackID, Surface: request.Surface, Aliases: request.Aliases}
	_, _, state, err := f.activationInputsForOperation(ctx, activation, OperationUpdate)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	intent, ok := intentForPack(state, request.PackID, request.Surface)
	if !ok || !intent.Active {
		return ReconciliationPlan{}, fmt.Errorf("capability pack %q is not active on %s", request.PackID, request.Surface)
	}
	current, err := f.catalog.catalogMetadata(request.PackID)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	if err := f.catalog.validateUpdateRoute(request.PackID, intent.Version, current.Version, current.manifestVersion, request.Surface); err != nil {
		return ReconciliationPlan{}, err
	}
	activation.Selection, err = canonicalSelection(intent.Selection)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	activation.Aliases = cloneAliases(intent.Aliases)
	if err := canonicalizeAliases(&activation.Aliases); err != nil {
		return ReconciliationPlan{}, err
	}
	if request.Aliases != nil {
		activation.Aliases = request.Aliases
	}
	plan, err := f.preview(ctx, activation, OperationUpdate, intent.Version, request.Force)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	plan.seal()
	return plan, nil
}

func (f Facade) PreviewDeactivate(ctx context.Context, request DeactivationRequest) (ReconciliationPlan, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (ReconciliationPlan, error) {
		return locked.previewDeactivate(ctx, request)
	})
}

func (f Facade) previewDeactivate(ctx context.Context, request DeactivationRequest) (ReconciliationPlan, error) {
	activation := ActivationRequest{PackID: request.PackID, Surface: request.Surface}
	requested, adapter, state, err := f.activationInputsForOperation(ctx, activation, OperationDeactivate)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	intent, active := intentForPack(state, request.PackID, request.Surface)
	selection := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
	if active {
		selection, err = canonicalSelection(intent.Selection)
		if err != nil {
			return ReconciliationPlan{}, err
		}
	}
	if len(request.Resources) > 0 {
		if !active || !intent.Active {
			return ReconciliationPlan{}, fmt.Errorf("cannot remove resources from inactive capability pack %q", request.PackID)
		}
		if requested.manifestVersion != manifestSchemaV4 {
			return ReconciliationPlan{}, fmt.Errorf("resource-scoped deactivation requires manifest schema_version 4")
		}
		nextSelection, selectionErr := removeResourceSelectionRoots(requested, selection, request.Resources)
		if selectionErr != nil {
			return ReconciliationPlan{}, selectionErr
		}
		if len(nextSelection.Roots) > 0 {
			return f.previewPartialDeactivate(ctx, request, requested, state, selection, nextSelection)
		}
	}
	currentRequested, err := selectPackResources(requested, selection)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	oldVersion := requested.Version
	if active && intent.Version != "" {
		oldVersion = intent.Version
		requested, err = f.catalog.resolveIntentPack(request.PackID, intent.Version)
		if err != nil {
			return ReconciliationPlan{}, err
		}
	}
	requested, err = selectPackResources(requested, selection)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	before, err := f.compose(requested, state, request.Surface, true)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	target := f.composeWithout(requested, state, request.Surface)
	combined := target.combinedPack()
	resolutions, err := f.resolveExecutables(ctx, before.combinedPack(), request.Surface, false)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	observation, err := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Surface, OperationDeactivate, before.combinedPack(), combined, state.Ownership, resolutions))
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("inspect deactivation of pack %q on %s: %w", requested.ID, request.Surface, err)
	}
	targetCollisionBlockers := distinctResourceTargetCollisions(observation.Projections)
	plan := ReconciliationPlan{pack: currentRequested, operation: OperationDeactivate, surface: request.Surface, intentRevision: state.Intent.Revision, documentRevision: state.documentRevision, oldVersion: oldVersion, previousAliases: cloneAliases(intent.Aliases), selection: selection, previousSelection: selection, observationFingerprint: observationDigest(observation), resolutions: resolutions, runtimeModeResults: cloneRuntimeModeResults(observation.RuntimeModeResults), activations: target.activations, compositionFacts: target.packs, beforeCompositionFacts: before.packs, intentFacts: target.intentFacts, beforeIntentFacts: before.intentFacts, ownershipFacts: cloneOwnership(state.Ownership), force: request.Force}
	plan.blockers = append(plan.blockers, target.blockers...)
	plan.blockers = append(plan.blockers, targetCollisionBlockers...)
	sortBlockers(plan.blockers)
	for _, resource := range combined.Resources {
		plan.portable = append(plan.portable, PortableOutcome{Kind: resource.Kind, ID: resource.ID})
	}
	for _, projection := range observation.Projections {
		if projection.DesiredFingerprint != "" {
			plan.desired = append(plan.desired, projectionExpectation{ID: projection.ID, Fingerprint: projection.DesiredFingerprint, ExternallyManaged: projection.ExternallyManaged})
			if projection.Exists && projection.ObservedFingerprint == projection.DesiredFingerprint {
			} else {
				detail := fmt.Sprintf("preserved projection %s because it is missing, drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
				plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
				plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
			}
			continue
		}
		if receipt, authorized := receiptForExternalProjection(state.External, request.Surface, projection, observation.Projections, nil); authorized {
			if projection.Exists && !externalReceiptOwnerRemains(receipt, target.packs) {
				plan.phases = appendPhaseAction(plan.phases, ConsentDestructiveCleanup, externalReceiptReversalAction(projection.Action, receipt))
			}
			continue
		}
		if projection.ExternallyManaged {
			if projection.Exists {
				plan.pendingHumanActions = append(plan.pendingHumanActions, fmt.Sprintf("preserved %s because no complete, exact, fresh external-effect receipt authorizes reversal; external executable, service, memory, data, sessions, credentials, and unrelated configuration remain untouched", projection.ID))
			}
			continue
		}
		owner, owned := ownershipForDeactivation(state, request.Surface, projection, request.Force)
		residual := active && !intent.Active && hasPackOwnership(state.Ownership, requested.ID)
		residualAuthorized := residual && owned && owner.Surface == request.Surface
		activeLifecycle := active && intent.Active
		if activeLifecycle && owned {
			residualAuthorized = true
		}
		if (activeLifecycle || residualAuthorized) && residualAuthorized && projection.Exists && owned && owner.PackID == requested.ID && owner.Surface == request.Surface && receiptPermitsRemoval(owner, projection, request.Force) {
			plan.phases = appendPhaseAction(plan.phases, ConsentDestructiveCleanup, projection.Action)
			continue
		}
		if projection.Exists {
			detail := fmt.Sprintf("preserved %s because it is drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
			plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
			plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
		}
	}
	if !active || !intent.Active {
		plan.noOp = len(plan.phases) == 0 && len(plan.pendingHumanActions) == 0 && !hasPackOwnership(state.Ownership, requested.ID)
		if len(plan.phases) == 0 && !plan.noOp {
			plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: requested.ID, Detail: fmt.Sprintf("inactive pack %s has partial, drifted, or residual state; preserved it", requested.ID)})
		}
	}
	sortBlockers(plan.blockers)
	if len(plan.blockers) > 0 {
		plan.noOp = false
	}
	if len(targetCollisionBlockers) > 0 {
		plan.phases = nil
	}
	sort.Strings(plan.pendingHumanActions)
	plan.captureSensitiveEffects()
	plan.seal()
	return plan, nil
}

func hasPackOwnership(values []ProjectionOwnership, packID string) bool {
	for _, value := range values {
		if value.PackID == packID {
			return true
		}
	}
	return false
}

func (f Facade) previewPartialDeactivate(ctx context.Context, request DeactivationRequest, requested Pack, state ActivationState, previousSelection, selection ResourceSelection) (ReconciliationPlan, error) {
	previousPack, err := selectPackResources(requested, previousSelection)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	targetPack, err := selectPackResources(requested, selection)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	previousAliases := intentAliases(state, request.PackID, request.Surface)
	aliases := aliasesForPack(targetPack, request.Surface, previousAliases)
	before, err := f.compose(previousPack, state, request.Surface, true)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	targetState := stateWithAliases(cloneActivationState(state), requested.ID, request.Surface, requested.Version, aliases)
	targetState = stateWithSelection(targetState, requested.ID, request.Surface, requested.Version, selection)
	target, err := f.compose(targetPack, targetState, request.Surface, true)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	combinedBefore, combined := before.combinedPack(), target.combinedPack()
	resolutions, err := f.resolveExecutables(ctx, combinedBefore, request.Surface, false)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	adapter := f.activation.adapters[request.Surface]
	observation, err := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Surface, OperationDeactivate, combinedBefore, combined, state.Ownership, resolutions))
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("inspect partial deactivation of pack %q on %s: %w", requested.ID, request.Surface, err)
	}
	targetCollisionBlockers := distinctResourceTargetCollisions(observation.Projections)
	plan := ReconciliationPlan{
		pack: targetPack, operation: OperationDeactivate, surface: request.Surface, intentRevision: state.Intent.Revision, documentRevision: state.documentRevision,
		oldVersion: requested.Version, aliases: cloneAliases(aliases), previousAliases: cloneAliases(previousAliases), selection: selection, previousSelection: previousSelection, partialSelection: true, force: request.Force,
		observationFingerprint: observationDigest(observation), resolutions: resolutions,
		runtimeModeResults: cloneRuntimeModeResults(observation.RuntimeModeResults), activations: target.activations,
		compositionFacts: target.packs, beforeCompositionFacts: before.packs, intentFacts: target.intentFacts, beforeIntentFacts: before.intentFacts,
		ownershipFacts: cloneOwnership(state.Ownership),
	}
	if intent, ok := intentForPack(state, request.PackID, request.Surface); ok && intent.Version != "" {
		plan.oldVersion = intent.Version
	}
	plan.blockers = append(plan.blockers, target.blockers...)
	plan.blockers = append(plan.blockers, targetCollisionBlockers...)
	sortBlockers(plan.blockers)
	for _, resource := range combined.Resources {
		plan.portable = append(plan.portable, PortableOutcome{Kind: resource.Kind, ID: resource.ID})
	}
	for _, projection := range observation.Projections {
		if projection.DesiredFingerprint != "" {
			plan.desired = append(plan.desired, projectionExpectation{ID: projection.ID, Fingerprint: projection.DesiredFingerprint, ExternallyManaged: projection.ExternallyManaged})
			if projection.Exists && projection.ObservedFingerprint == projection.DesiredFingerprint {
			} else {
				detail := fmt.Sprintf("preserved projection %s because it is missing, drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
				plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
				plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
			}
			continue
		}
		if receipt, authorized := receiptForExternalProjection(state.External, request.Surface, projection, observation.Projections, nil); authorized {
			if projection.Exists && !externalReceiptOwnerRemains(receipt, target.packs) {
				plan.phases = appendPhaseAction(plan.phases, ConsentDestructiveCleanup, externalReceiptReversalAction(projection.Action, receipt))
			}
			continue
		}
		if projection.ExternallyManaged {
			if projection.Exists {
				plan.pendingHumanActions = append(plan.pendingHumanActions, fmt.Sprintf("preserved %s because no complete, exact, fresh external-effect receipt authorizes reversal; external executable, service, memory, data, sessions, credentials, and unrelated configuration remain untouched", projection.ID))
			}
			continue
		}
		owner, owned := ownershipForDeactivation(state, request.Surface, projection, request.Force)
		if projection.Exists && owned && owner.PackID == request.PackID && owner.Surface == request.Surface && receiptPermitsRemoval(owner, projection, request.Force) {
			plan.phases = appendPhaseAction(plan.phases, ConsentDestructiveCleanup, projection.Action)
			continue
		}
		if projection.Exists {
			detail := fmt.Sprintf("preserved %s because it is drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
			plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
			plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
		}
	}
	plan.readiness, plan.conditions = evaluateReadiness(readinessEvaluation{
		Pack: targetPack, Surface: request.Surface, Scope: ReadinessScopeGlobal,
		Projections: expectedReadinessProjections(observation.Projections, plan.blockers), Resolutions: resolutions,
		Observation: observation.Readiness, Revision: observation.Revision,
	})
	for _, evidence := range observation.Readiness.Evidence {
		plan.observedEvidence = append(plan.observedEvidence, reportSafeObservationText(evidence, observation.Projections))
	}
	sort.Strings(plan.observedEvidence)
	for _, projection := range observation.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint && !projection.ExternallyManaged {
			plan.pendingEvidence = append(plan.pendingEvidence, projection.ID+": verification pending Apply")
		}
	}
	if !observation.Readiness.AuthorizationObserved {
		plan.pendingEvidence = append(plan.pendingEvidence, "authorization evidence pending a host observation")
	}
	if !observation.Readiness.UsabilityObserved {
		plan.pendingEvidence = append(plan.pendingEvidence, "usability evidence pending a host observation")
	}
	sort.Strings(plan.pendingEvidence)
	sort.Strings(plan.pendingHumanActions)
	if len(plan.blockers) > 0 {
		plan.noOp = false
	}
	if len(targetCollisionBlockers) > 0 {
		plan.phases = nil
	}
	plan.captureSensitiveEffects()
	plan.seal()
	return plan, nil
}

func (f Facade) preview(ctx context.Context, request ActivationRequest, operation Operation, oldVersion string, force bool) (ReconciliationPlan, error) {
	requested, adapter, state, err := f.activationInputsForOperation(ctx, request, operation)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	selection, err := canonicalSelection(request.Selection)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	previousAliases := []SurfaceAlias{}
	previousSelection := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
	if intent, ok := intentForPack(state, requested.ID, request.Surface); ok {
		previousAliases = cloneAliases(intent.Aliases)
		previousSelection, err = canonicalSelection(intent.Selection)
		if err != nil {
			return ReconciliationPlan{}, err
		}
		if operation == OperationActivate && intent.Active {
			switch {
			case selection.Mode == SelectionCustom && previousSelection.Mode == SelectionCustom:
				selection, err = mergeCustomSelections(previousSelection, selection)
				if err != nil {
					return ReconciliationPlan{}, err
				}
			case selection.Mode == SelectionCustom && previousSelection.Mode == SelectionAll:
				if _, err = selectPackResources(requested, selection); err != nil {
					return ReconciliationPlan{}, err
				}
				selection = previousSelection
			case digestJSON(previousSelection) != digestJSON(selection):
				return ReconciliationPlan{}, fmt.Errorf("capability pack %q is already active on %s with a different resource selection; selection changes require an explicit lifecycle transition", requested.ID, request.Surface)
			}
		}
	}
	selectionValidityBlockers, selectionValidity, err := selectionBlockers(requested, selection, request.Surface)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	requested, err = selectPackResourcesForSurface(requested, selection, request.Surface)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	aliases, err := requestedAliases(requested, request.Surface, request.Aliases, state, operation)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	state = stateWithSelection(stateWithAliases(state, requested.ID, request.Surface, requested.Version, aliases), requested.ID, request.Surface, requested.Version, selection)
	composition, err := f.compose(requested, state, request.Surface, false)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	composition.blockers = append(composition.blockers, selectionValidityBlockers...)
	sortBlockers(composition.blockers)
	var beforeCompositionFacts []Pack
	var beforeIntentFacts []ActivationIntent
	var previousPack Pack
	var ownedBeforeUpdate func(ObservedProjection, string) bool
	pack := composition.combinedPack()
	resolutions, err := f.resolveExecutables(ctx, pack, request.Surface, operation == OperationActivate || operation == OperationUpdate)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	observation, err := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Surface, operation, previousPack, pack, state.Ownership, resolutions))
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("inspect activation of pack %q on %s: %w", pack.ID, request.Surface, err)
	}
	targetCollisionBlockers := distinctResourceTargetCollisions(observation.Projections)
	composition.blockers = append(composition.blockers, targetCollisionBlockers...)

	actions := make([]ProjectionAction, 0, len(observation.Projections))
	executableAdapterActions := make([]ProjectionAction, 0)
	destructiveActions := make([]ProjectionAction, 0)
	for _, projection := range observation.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			if projection.ExternallyManaged {
				continue
			}
			owned := ownedAtComposition(state.Ownership, projection, projection.ObservedFingerprint, composition)
			removal := projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent
			if operation == OperationUpdate && removal && !owned && ownedBeforeUpdate != nil {
				owned = ownedBeforeUpdate(projection, projection.ObservedFingerprint)
			}
			managedDrift := false
			if force && projection.Exists && forceRepairEligible(state.Ownership, projection, composition) {
				managedDrift = true
			}
			if projection.Exists && !owned && !managedDrift {
				composition.blockers = append(composition.blockers, PlanBlocker{BlockerOwnership, projection.ID, fmt.Sprintf("projection is unmanaged or drifted; preserving existing %s content", request.Surface)})
				continue
			}
			if managedDrift {
				projection.Action.Description = "restore drifted Packy-managed projection " + projection.ID + " to intent-selected content: " + projection.Action.Description
			}
			if operation == OperationUpdate && removal {
				destructiveActions = append(destructiveActions, projection.Action)
			} else if projection.Action.Consent == ConsentExecutableExternal {
				executableAdapterActions = append(executableAdapterActions, projection.Action)
			} else {
				actions = append(actions, projection.Action)
			}
		}
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	sort.Slice(executableAdapterActions, func(i, j int) bool { return executableAdapterActions[i].ID < executableAdapterActions[j].ID })
	sort.Slice(destructiveActions, func(i, j int) bool { return destructiveActions[i].ID < destructiveActions[j].ID })
	externalActions, externalBlockers := f.externalPlan(operation, pack, request.Surface, state, resolutions)
	externalActions = append(executableAdapterActions, externalActions...)
	composition.blockers = append(composition.blockers, externalBlockers...)
	if len(targetCollisionBlockers) > 0 {
		actions = nil
		externalActions = nil
		destructiveActions = nil
	}
	sortBlockers(composition.blockers)
	noOp := compositionActive(state, composition.packs, request.Surface) && ownershipMatchesPack(state.Ownership, observation.Projections, composition) && len(actions) == 0 && len(externalActions) == 0
	if current, ok := intentForPack(state, request.PackID, request.Surface); ok && digestJSON(current.Aliases) != digestJSON(aliases) {
		noOp = false
	}
	if digestJSON(previousSelection) != digestJSON(selection) {
		noOp = false
	}
	if operation == OperationActivate {
		if current, ok := intentForPack(state, request.PackID, request.Surface); ok && current.Active && !intentIsExplicit(current) {
			noOp = false
		}
	}
	readiness, conditions := evaluateReadiness(readinessEvaluation{
		Pack: requested, Surface: request.Surface, Scope: ReadinessScopeGlobal,
		Projections: expectedReadinessProjections(observation.Projections, composition.blockers), Resolutions: resolutions,
		Observation: observation.Readiness, Revision: observation.Revision,
	})
	observedEvidence := make([]string, 0, len(observation.Readiness.Evidence))
	for _, evidence := range observation.Readiness.Evidence {
		observedEvidence = append(observedEvidence, reportSafeObservationText(evidence, observation.Projections))
	}
	sort.Strings(observedEvidence)
	pendingEvidence := []string{}
	for _, projection := range observation.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint && !projection.ExternallyManaged {
			pendingEvidence = append(pendingEvidence, projection.ID+": verification pending Apply")
		}
	}
	if !observation.Readiness.AuthorizationObserved {
		pendingEvidence = append(pendingEvidence, "authorization evidence pending a host observation")
	}
	if !observation.Readiness.UsabilityObserved {
		pendingEvidence = append(pendingEvidence, "usability evidence pending a host observation")
	}
	sort.Strings(pendingEvidence)
	pendingHumanActions := make([]string, 0, len(observation.PendingHumanActions))
	for _, action := range observation.PendingHumanActions {
		pendingHumanActions = append(pendingHumanActions, reportSafeObservationText(action, observation.Projections))
	}
	sort.Strings(pendingHumanActions)
	plan := ReconciliationPlan{pack: requested, operation: operation, surface: request.Surface, intentRevision: state.Intent.Revision, documentRevision: state.documentRevision, oldVersion: oldVersion, aliases: cloneAliases(aliases), previousAliases: previousAliases, selection: selection, previousSelection: previousSelection, selectionValidity: selectionValidity, observationFingerprint: observationDigest(observation), resolutions: resolutions, runtimeModeResults: cloneRuntimeModeResults(observation.RuntimeModeResults), readiness: readiness, conditions: conditions, observedEvidence: observedEvidence, pendingEvidence: pendingEvidence, pendingHumanActions: pendingHumanActions, noOp: noOp, activations: composition.activations, blockers: composition.blockers, compositionFacts: composition.packs, intentFacts: composition.intentFacts, beforeIntentFacts: beforeIntentFacts, ownershipFacts: cloneOwnership(state.Ownership), beforeCompositionFacts: beforeCompositionFacts, force: force}
	for _, resource := range pack.Resources {
		plan.portable = append(plan.portable, PortableOutcome{Kind: resource.Kind, ID: resource.ID})
	}
	sort.Slice(plan.portable, func(i, j int) bool {
		if plan.portable[i].Kind == plan.portable[j].Kind {
			return plan.portable[i].ID < plan.portable[j].ID
		}
		return plan.portable[i].Kind < plan.portable[j].Kind
	})
	for _, projection := range observation.Projections {
		plan.desired = append(plan.desired, projectionExpectation{projection.ID, projection.DesiredFingerprint, projection.ExternallyManaged})
	}
	sort.Slice(plan.desired, func(i, j int) bool { return plan.desired[i].ID < plan.desired[j].ID })
	if len(actions) > 0 {
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentReversibleLocal, ApprovalRequired: true, Actions: append([]ProjectionAction(nil), actions...)})
	}
	if len(externalActions) > 0 {
		var acquisitionActions, setupActions []ProjectionAction
		for _, action := range externalActions {
			if action.Consent == ConsentToolHostSetup {
				setupActions = append(setupActions, action)
			} else {
				acquisitionActions = append(acquisitionActions, action)
			}
		}
		if len(acquisitionActions) > 0 {
			plan.phases = append(plan.phases, PlanPhase{Kind: ConsentExecutableExternal, ApprovalRequired: true, Actions: acquisitionActions})
		}
		if len(setupActions) > 0 {
			plan.phases = append(plan.phases, PlanPhase{Kind: ConsentToolHostSetup, ApprovalRequired: true, Actions: setupActions})
		}
	}
	if len(destructiveActions) > 0 {
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentDestructiveCleanup, ApprovalRequired: true, Actions: append([]ProjectionAction(nil), destructiveActions...)})
	}
	if len(pendingHumanActions) > 0 {
		hostActions := make([]ProjectionAction, 0, len(pendingHumanActions))
		for i, action := range pendingHumanActions {
			hostActions = append(hostActions, ProjectionAction{ID: fmt.Sprintf("host-follow-up:%s:%d", request.Surface, i), Kind: ActionHostFollowUp, Description: action})
		}
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentHostFollowUp, Actions: hostActions})
	}
	if len(plan.blockers) > 0 {
		plan.noOp = false
	}
	plan.captureSensitiveEffects()
	plan.seal()
	return plan, nil
}

func distinctResourceTargetCollisions(projections []ObservedProjection) []PlanBlocker {
	byTarget := map[string]string{}
	seen := map[string]bool{}
	var blockers []PlanBlocker
	for _, projection := range projections {
		if projection.Action.Target == "" {
			continue
		}
		if !isPackResourceProjection(projection.ID) {
			continue
		}
		target := filepath.Clean(projection.Action.Target)
		prior, exists := byTarget[target]
		if !exists {
			byTarget[target] = projection.ID
			continue
		}
		if prior == projection.ID {
			continue
		}
		pair := prior + "+" + projection.ID
		if seen[pair] {
			continue
		}
		seen[pair] = true
		blockers = append(blockers, PlanBlocker{Kind: BlockerTargetCollision, Subject: pair, Detail: fmt.Sprintf("distinct Pack resources %s and %s target the same path; shared ownership is not supported", prior, projection.ID)})
	}
	sortBlockers(blockers)
	return blockers
}

func isPackResourceProjection(id string) bool {
	kind, _, ok := strings.Cut(id, ":")
	if !ok {
		return false
	}
	switch kind {
	case "skill", "instruction", "mcp_server", "lifecycle", "agent", "command", "asset", "notice":
		return true
	default:
		return false
	}
}

func (f Facade) Approve(plan ReconciliationPlan, kind ConsentKind) ApprovalReceipt {
	for _, phase := range plan.phases {
		if phase.Kind == kind && phase.ApprovalRequired {
			return ApprovalReceipt{planDigest: plan.digest, phaseDigest: phase.Digest, kind: kind}
		}
	}
	return ApprovalReceipt{}
}

func (f Facade) Apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	result, err := withBundleObservation(ctx, f, func(locked Facade) (ApplyResult, error) {
		return locked.apply(ctx, request)
	})
	return result, ReportSafeError(err, &request.Plan)
}

func (f Facade) apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	if !request.Plan.Applicable() {
		return ApplyResult{}, PlanNotActionableError{Disposition: request.Plan.Disposition()}
	}
	if !request.Plan.validSeal() {
		return ApplyResult{}, ErrApprovalMismatch
	}
	if request.Plan.noOp {
		if _, err := f.preflightPlan(ctx, request.Plan); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Verified: true, PlanID: request.Plan.id, Readiness: request.Plan.readiness, PendingHumanActions: request.Plan.PendingHumanActions()}, nil
	}
	if !request.Interactive {
		return ApplyResult{}, ErrInteractiveRequired
	}
	for _, phase := range request.Plan.phases {
		if !phase.ApprovalRequired {
			continue
		}
		approved := false
		for _, receipt := range request.Approvals {
			if receipt.planDigest == request.Plan.digest && receipt.phaseDigest == phase.Digest && receipt.kind == phase.Kind {
				approved = true
				break
			}
		}
		if !approved {
			return ApplyResult{}, ErrApprovalMismatch
		}
	}
	preflight, err := f.preflightPlan(ctx, request.Plan)
	if err != nil {
		return ApplyResult{}, err
	}
	pack, adapter, state := preflight.pack, preflight.adapter, preflight.state
	currentComposition, combined, resolutions := preflight.composition, preflight.combined, preflight.resolutions
	if hasExternalCommand(request.Plan.phases) && f.activation.executor == nil {
		return ApplyResult{}, fmt.Errorf("external effects are not configured")
	}

	state.SchemaVersion = 3
	{
		previousIntents := activeIntents(state)
		previousByID := map[string]ActivationIntent{}
		for _, intent := range previousIntents {
			previousByID[intent.PackID] = intent
		}
		activeTarget := request.Plan.operation != OperationDeactivate || request.Plan.partialSelection
		targetVersion := pack.Version
		if request.Plan.operation == OperationDeactivate && request.Plan.oldVersion != "" {
			targetVersion = request.Plan.oldVersion
		}
		explicit := true
		state.Intent = ActivationIntent{PackID: pack.ID, Surface: request.Plan.surface, Version: targetVersion, Active: activeTarget, Revision: state.Intent.Revision + 1, ReadinessObligations: append([]ReadinessObligation(nil), pack.ReadinessObligations...), ExternalRequirements: append([]string{}, pack.Requires.Tools...), Aliases: cloneAliases(request.Plan.aliases), Selection: request.Plan.selection, Resources: packResourceIdentities(pack), Explicit: &explicit}
		byID := map[string]ActivationIntent{}
		for _, intent := range previousIntents {
			byID[intent.PackID] = intent
		}
		for _, activation := range request.Plan.activations {
			previous, previouslyActive := previousByID[activation.Pack.ID]
			aliases := previous.Aliases
			if activation.Pack.ID == pack.ID {
				aliases = request.Plan.aliases
			}
			activationSelection := activation.Selection
			explicitIntent := activation.Role != ActivationRequired
			explicitFact := &explicitIntent
			if previouslyActive {
				explicitFact = previous.Explicit
			}
			byID[activation.Pack.ID] = ActivationIntent{PackID: activation.Pack.ID, Surface: request.Plan.surface, Version: activation.Pack.Version, Active: true, Revision: state.Intent.Revision, ReadinessObligations: append([]ReadinessObligation(nil), activation.Pack.ReadinessObligations...), ExternalRequirements: append([]string{}, activation.Pack.Requires.Tools...), Aliases: cloneAliases(aliases), Selection: activationSelection, Resources: packResourceIdentities(activation.Pack), Explicit: explicitFact}
			if activation.Pack.ID == pack.ID {
				byID[activation.Pack.ID] = state.Intent
			}
		}
		if request.Plan.operation == OperationDeactivate {
			byID[pack.ID] = state.Intent
			for id, candidate := range byID {
				if id == pack.ID || !candidate.Active || intentIsExplicit(candidate) {
					continue
				}
				candidate.Active = false
				byID[id] = candidate
			}
		}
		state.Intents = nil
		for _, intent := range byID {
			state.Intents = append(state.Intents, intent)
		}
		sort.Slice(state.Intents, func(i, j int) bool { return state.Intents[i].PackID < state.Intents[j].PackID })
	}
	localActions := phaseActions(request.Plan.phases, ConsentReversibleLocal)
	externalActions := externalEffectActions(request.Plan.phases)
	adapterExternalActions := make([]ProjectionAction, 0, len(externalActions))
	for _, action := range externalActions {
		if action.Kind != ActionExternalCommand {
			adapterExternalActions = append(adapterExternalActions, action)
		}
	}
	if len(localActions) > 0 {
		if err := adapter.ApplyProjections(ctx, localActions); err != nil {
			return ApplyResult{}, ReportSafeError(err, &request.Plan)
		}
	}
	destructiveActions := phaseActions(request.Plan.phases, ConsentDestructiveCleanup)
	prior := priorCombinedPack(request.Plan, pack)
	verified, err := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Plan.surface, request.Plan.operation, prior, combined, state.Ownership, resolutions))
	if err != nil {
		return ApplyResult{}, ReportSafeError(err, &request.Plan)
	}
	verificationDesired := withoutExternallyManagedExpectations(request.Plan.desired)
	verificationDesired = withoutActionExpectations(verificationDesired, adapterExternalActions)
	if len(destructiveActions) > 0 {
		verificationDesired = withoutActionExpectations(verificationDesired, destructiveActions)
	}
	verifiedMatches := len(verificationDesired) == 0 || verificationMatches(verificationDesired, verified.Projections)
	if len(verificationDesired) != len(request.Plan.desired) {
		verifiedMatches = verificationMatchesSubset(verificationDesired, verified.Projections)
	}
	state.External = refreshExternalReceiptOwner(state.External, currentComposition.packs, request.Plan.surface)
	if request.Plan.operation == OperationDeactivate && len(destructiveActions) > 0 {
		verifiedMatches = verificationMatchesSubset(verificationDesired, verified.Projections)
	} else if request.Plan.operation == OperationDeactivate {
		present := make([]ObservedProjection, 0, len(verified.Projections))
		for _, projection := range verified.Projections {
			if projection.Goal == ProjectionPresent {
				present = append(present, projection)
			}
		}
		verifiedMatches = verificationMatchesDeactivation(request.Plan.desired, present)
	}
	if !verifiedMatches {
		return ApplyResult{}, fmt.Errorf("%w: %s", ErrVerificationFailed, verificationMismatch(request.Plan.desired, verified.Projections))
	}
	beforeExternal := cloneSurfaceInspection(verified)
	for _, action := range externalActions {
		var actionErr error
		if action.Kind == ActionExternalCommand {
			actionErr = f.activation.executor.Execute(ctx, action)
		} else if err := adapter.ApplyProjections(ctx, []ProjectionAction{action}); err != nil {
			actionErr = err
		}
		if actionErr != nil {
			actionErr = ReportSafeError(actionErr, &request.Plan)
			if action.Kind == ActionExternalCommand && action.Consent == ConsentToolHostSetup {
				state.External = recordExternalEffect(state.External, action)
				partial, inspectErr := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Plan.surface, request.Plan.operation, prior, combined, state.Ownership, resolutions))
				if inspectErr == nil {
					state.External = recordExternalReceipts(state.External, []ProjectionAction{action}, request.Plan.surface, currentComposition.packs, beforeExternal, partial)
				}
			}
			if saveErr := checkpointExternalEffects(ctx, f.activation.store, request.Plan.surface, &state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("external action %s failed: %v; could not persist verified external-effect receipts: %w", action.ID, actionErr, saveErr)
			}
			return ApplyResult{}, fmt.Errorf("external action %s failed; later actions stopped: %w", action.ID, actionErr)
		}
		if action.Kind == ActionExternalCommand {
			state.External = recordExternalEffect(state.External, action)
			if action.Consent == ConsentToolHostSetup {
				partial, inspectErr := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Plan.surface, request.Plan.operation, prior, combined, state.Ownership, resolutions))
				if inspectErr == nil {
					state.External = recordExternalReceipts(state.External, []ProjectionAction{action}, request.Plan.surface, currentComposition.packs, beforeExternal, partial)
					beforeExternal = partial
				}
			}
		}
		if err := checkpointExternalEffects(ctx, f.activation.store, request.Plan.surface, &state); err != nil {
			return ApplyResult{}, fmt.Errorf("external action %s completed but its verified receipt could not be persisted: %w", action.ID, err)
		}
	}
	for _, action := range destructiveActions {
		if err := adapter.ApplyProjections(ctx, []ProjectionAction{action}); err != nil {
			return ApplyResult{}, ReportSafeError(err, &request.Plan)
		}
	}
	if len(externalActions) > 0 || len(destructiveActions) > 0 {
		verified, err = inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Plan.surface, request.Plan.operation, prior, combined, state.Ownership, resolutions))
		if err != nil {
			return ApplyResult{}, ReportSafeError(err, &request.Plan)
		}
		if len(externalActions) > 0 {
			state.External = recordExternalReceipts(state.External, externalActions, request.Plan.surface, currentComposition.packs, beforeExternal, verified)
		}
		verificationProjections := verified.Projections
		if request.Plan.operation == OperationDeactivate {
			actionIDs := make(map[string]bool, len(destructiveActions))
			for _, action := range destructiveActions {
				actionIDs[action.ID] = true
			}
			verificationProjections = make([]ObservedProjection, 0, len(verified.Projections))
			for _, projection := range verified.Projections {
				if projection.Goal == ProjectionPresent || actionIDs[projection.ID] {
					verificationProjections = append(verificationProjections, projection)
				}
			}
		}
		matches := verificationMatches(request.Plan.desired, verificationProjections)
		if request.Plan.operation == OperationUpdate && len(destructiveActions) > 0 {
			matches = verificationMatchesAfterCleanup(request.Plan.desired, verified.Projections, destructiveActions)
		}
		if request.Plan.operation == OperationDeactivate {
			matches = verificationMatchesDeactivation(request.Plan.desired, verificationProjections)
		}
		if !matches {
			if saveErr := checkpointExternalEffects(ctx, f.activation.store, request.Plan.surface, &state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("%w: %s; could not persist verified external-effect receipts: %v", ErrVerificationFailed, verificationMismatch(request.Plan.desired, verified.Projections), saveErr)
			}
			return ApplyResult{}, fmt.Errorf("%w: %s", ErrVerificationFailed, verificationMismatch(request.Plan.desired, verified.Projections))
		}
	}
	if request.Plan.operation == OperationDeactivate && len(destructiveActions) > 0 {
		completed := make([]string, 0, len(destructiveActions))
		for _, action := range destructiveActions {
			completed = append(completed, action.ID)
		}
		state.External = retireExternalReceipts(state.External, completed)
	}
	previousOwnership := cloneOwnership(state.Ownership)
	state.Ownership = make([]ProjectionOwnership, 0, len(verified.Projections))
	observedOwnershipIDs := map[string]bool{}
	for _, projection := range verified.Projections {
		observedOwnershipIDs[ownershipIDForState(state, request.Plan.surface, projection)] = true
	}
	// Ownership is document-wide. Preserve facts for physical projections not
	// inspected by this surface; a surface adapter may update only identities it
	// actually observed in its sealed transition.
	if state.snapshotManaged {
		for _, owner := range previousOwnership {
			if !observedOwnershipIDs[owner.ID] {
				state.Ownership = append(state.Ownership, owner)
			}
		}
	}
	for _, projection := range verified.Projections {
		if projection.ExternallyManaged || hasPhaseActionID(request.Plan.phases, ConsentDestructiveCleanup, projection.ID) {
			continue
		}
		if projection.DesiredFingerprint == "" {
			// A deactivation may intentionally preserve a projection that no longer
			// belongs in the desired composition. Keep only pre-existing ownership:
			// an unmanaged lookalike must never become Packy-owned merely because it
			// matches a catalog resource. The inactive intent retains the exact Pack
			// version and selection needed to inspect this residual again.
			if request.Plan.operation == OperationDeactivate && projection.Exists {
				if previous, ok := ownershipByID(previousOwnership, ownershipIDForState(state, request.Plan.surface, projection)); ok {
					state.Ownership = append(state.Ownership, previous)
				}
			}
			continue
		}
		previous, previouslyOwned := ownershipByID(previousOwnership, ownershipIDForState(state, request.Plan.surface, projection))
		owner := ProjectionOwnership{ID: ownershipIDForState(state, request.Plan.surface, projection), ProjectionID: projection.ID, Target: projection.Action.Target, Fingerprint: projection.DesiredFingerprint, PackID: request.Plan.pack.ID, Surface: request.Plan.surface}
		if previouslyOwned {
			owner = previous
			owner.Fingerprint = projection.DesiredFingerprint
			owner.Target = projection.Action.Target
		}
		owner.PackID = request.Plan.pack.ID
		owner.Surface = request.Plan.surface
		state.Ownership = append(state.Ownership, owner)
	}
	sort.Slice(state.Ownership, func(i, j int) bool { return state.Ownership[i].ID < state.Ownership[j].ID })
	if err := saveActivationState(ctx, f.activation.store, request.Plan.surface, &state); err != nil {
		return ApplyResult{}, err
	}
	fresh := verified.Readiness
	readiness, conditions := evaluateReadiness(readinessEvaluation{
		Pack: request.Plan.pack, Surface: request.Plan.surface, Scope: ReadinessScopeGlobal,
		Projections: observedReadinessProjections(verified.Projections), Resolutions: request.Plan.resolutions,
		Observation: fresh, Revision: verified.Revision,
	})
	pendingHumanActions := append([]string(nil), fresh.PendingHumanActions...)
	if len(pendingHumanActions) == 0 {
		pendingHumanActions = append(pendingHumanActions, verified.PendingHumanActions...)
	}
	return ApplyResult{Verified: true, PlanID: request.Plan.id, Projections: len(state.Ownership), Readiness: readiness, Conditions: conditions, PendingHumanActions: pendingHumanActions}, nil
}

func expectedReadinessProjections(values []ObservedProjection, blockers []PlanBlocker) []ProjectionStatus {
	blocked := make(map[string]bool, len(blockers))
	for _, blocker := range blockers {
		blocked[blocker.Subject] = true
	}
	result := make([]ProjectionStatus, 0, len(values))
	for _, value := range values {
		if value.DesiredFingerprint == "" {
			continue
		}
		health := ProjectionVerified
		if blocked[value.ID] {
			health = ProjectionAmbiguous
		}
		result = append(result, ProjectionStatus{ID: value.ID, Target: value.Action.Target, Health: health, ObservedFingerprint: value.ObservedFingerprint, DesiredFingerprint: value.DesiredFingerprint})
	}
	return result
}

func observedReadinessProjections(values []ObservedProjection) []ProjectionStatus {
	result := make([]ProjectionStatus, 0, len(values))
	for _, value := range values {
		if value.DesiredFingerprint == "" {
			continue
		}
		health := ProjectionMissing
		if value.Exists {
			health = ProjectionDrifted
			if value.ObservedFingerprint == value.DesiredFingerprint {
				health = ProjectionVerified
			}
		}
		result = append(result, ProjectionStatus{ID: value.ID, Target: value.Action.Target, Health: health, ObservedFingerprint: value.ObservedFingerprint, DesiredFingerprint: value.DesiredFingerprint})
	}
	return result
}

func verificationMatchesSubset(desired []projectionExpectation, observed []ObservedProjection) bool {
	byID := make(map[string]ObservedProjection, len(observed))
	for _, projection := range observed {
		byID[projection.ID] = projection
	}
	for _, expectation := range desired {
		projection, ok := byID[expectation.ID]
		if !ok || projection.ObservedFingerprint != expectation.Fingerprint {
			return false
		}
	}
	return true
}

func verificationMatchesAfterCleanup(desired []projectionExpectation, observed []ObservedProjection, actions []ProjectionAction) bool {
	if !verificationMatchesSubset(withoutActionExpectations(desired, actions), observed) {
		return false
	}
	byID := make(map[string]ObservedProjection, len(observed))
	for _, projection := range observed {
		byID[projection.ID] = projection
	}
	for _, action := range actions {
		projection, ok := byID[action.ID]
		if !ok || projection.Exists {
			return false
		}
	}
	return true
}

func withoutActionExpectations(values []projectionExpectation, actions []ProjectionAction) []projectionExpectation {
	ids := map[string]bool{}
	for _, action := range actions {
		ids[action.ID] = true
	}
	result := make([]projectionExpectation, 0, len(values))
	for _, value := range values {
		if !ids[value.ID] {
			result = append(result, value)
		}
	}
	return result
}

func hasPhaseActionID(phases []PlanPhase, kind ConsentKind, id string) bool {
	for _, action := range phaseActions(phases, kind) {
		if action.ID == id {
			return true
		}
	}
	return false
}

type planPreflight struct {
	pack        Pack
	adapter     SurfaceAdapter
	state       ActivationState
	composition composition
	combined    Pack
	resolutions []ExecutableResolution
}

func priorCombinedPack(plan ReconciliationPlan, requested Pack) Pack {
	if plan.operation == OperationUpdate && len(plan.beforeCompositionFacts) == 0 {
		return Pack{}
	}
	return composition{
		requested:   requested,
		packs:       plan.beforeCompositionFacts,
		intentFacts: plan.beforeIntentFacts,
		surface:     plan.surface,
	}.combinedPack()
}

func (f Facade) preflightPlan(ctx context.Context, plan ReconciliationPlan) (planPreflight, error) {
	freshCatalog, err := f.catalog.refreshed()
	if err != nil {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("catalog or manifest changed after Preview: %v; rerun %s to preview a fresh plan", err, plan.operation)}
	}
	f.catalog = freshCatalog
	pack, adapter, state, err := f.activationInputsForOperation(ctx, ActivationRequest{PackID: plan.pack.ID, Surface: plan.surface, Selection: plan.selection}, plan.operation)
	if err != nil {
		return planPreflight{}, err
	}
	if state.documentRevision != plan.documentRevision {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("capability-pack state revision changed from %d to %d after Preview; rerun %s to preview a fresh plan", plan.documentRevision, state.documentRevision, plan.operation)}
	}
	if plan.operation == OperationDeactivate {
		pack, err = selectPackResources(pack, plan.selection)
	} else {
		pack, err = selectPackResourcesForSurface(pack, plan.selection, plan.surface)
	}
	if err != nil {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("resource selection became invalid after Preview: %v; rerun %s to preview a fresh plan", err, plan.operation)}
	}
	if digestJSON(pack) != digestJSON(plan.pack) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("catalog-current pack changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	loadedIntentRevision := state.Intent.Revision
	currentAliases := []SurfaceAlias{}
	if intent, ok := intentForPack(state, plan.pack.ID, plan.surface); ok {
		currentAliases = intent.Aliases
	}
	canonicalCurrentAliases, canonicalPreviousAliases := cloneAliases(currentAliases), cloneAliases(plan.previousAliases)
	_ = canonicalizeAliases(&canonicalCurrentAliases)
	_ = canonicalizeAliases(&canonicalPreviousAliases)
	if digestJSON(canonicalCurrentAliases) != digestJSON(canonicalPreviousAliases) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("activation aliases changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	currentSelection, selectionErr := canonicalSelection(ResourceSelection{})
	if intent, ok := intentForPack(state, plan.pack.ID, plan.surface); ok {
		currentSelection, selectionErr = canonicalSelection(intent.Selection)
	}
	if selectionErr != nil || digestJSON(currentSelection) != digestJSON(plan.previousSelection) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("resource selection changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	if plan.operation == OperationDeactivate {
		intent, ok := intentForPack(state, plan.pack.ID, plan.surface)
		if ok && intent.Version != "" {
			pack, err = f.catalog.resolveIntentPack(intent.PackID, intent.Version)
			if err != nil {
				return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("historical artifact changed after Preview: %v; rerun deactivate to preview a fresh plan", err)}
			}
		}
	}
	if plan.operation != OperationDeactivate {
		state = stateWithSelection(stateWithAliases(state, plan.pack.ID, plan.surface, plan.pack.Version, plan.aliases), plan.pack.ID, plan.surface, plan.pack.Version, plan.selection)
		if state.Intent.PackID == plan.pack.ID && state.Intent.Surface == plan.surface {
			state.Intent.Revision = plan.intentRevision
		}
		for i := range state.Intents {
			if state.Intents[i].PackID == plan.pack.ID && state.Intents[i].Surface == plan.surface {
				state.Intents[i].Revision = plan.intentRevision
			}
		}
	}
	var current composition
	if plan.operation == OperationDeactivate && plan.partialSelection {
		previousPack, previousErr := selectPackResources(pack, plan.previousSelection)
		if previousErr != nil {
			return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("previous resource selection changed after Preview: %v; rerun deactivate to preview a fresh plan", previousErr)}
		}
		before, beforeErr := f.compose(previousPack, state, plan.surface, true)
		if beforeErr != nil || digestJSON(before.packs) != digestJSON(plan.beforeCompositionFacts) {
			return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("previous resource composition changed after Preview; rerun deactivate to preview a fresh plan")}
		}
		state = stateWithSelection(stateWithAliases(cloneActivationState(state), plan.pack.ID, plan.surface, plan.pack.Version, plan.aliases), plan.pack.ID, plan.surface, plan.pack.Version, plan.selection)
		current, err = f.compose(pack, state, plan.surface, true)
	} else {
		useRequestedIntent := plan.operation == OperationDeactivate
		current, err = f.compose(pack, state, plan.surface, useRequestedIntent)
	}
	if err != nil {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("catalog or historical artifact changed after Preview: %v; rerun %s to preview a fresh plan", err, plan.operation)}
	}
	if plan.operation == OperationUpdate && len(plan.beforeCompositionFacts) > 0 {
		before, beforeErr := f.compose(pack, state, plan.surface, true)
		if beforeErr != nil || digestJSON(before.packs) != digestJSON(plan.beforeCompositionFacts) {
			return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("historical update comparison changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
		}
	}
	if plan.operation == OperationDeactivate {
		if plan.partialSelection {
			if digestJSON(current.intentFacts) != digestJSON(plan.intentFacts) {
				return planPreflight{}, StalePlanError{Precondition: "resource selection or active intents changed after Preview; rerun deactivate to preview a fresh plan"}
			}
		} else {
			before := current
			target := f.composeWithout(pack, state, plan.surface)
			if digestJSON(before.packs) != digestJSON(plan.beforeCompositionFacts) {
				return planPreflight{}, StalePlanError{Precondition: "active Pack composition changed after Preview; rerun deactivate to preview a fresh plan"}
			}
			current = target
		}
	}
	planned := composition{requested: plan.pack, surface: plan.surface, packs: plan.compositionFacts, activations: plan.activations, blockers: plan.blockers, intentFacts: plan.intentFacts}
	if current.identityDigest() != planned.identityDigest() {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("dependency or catalog composition changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	if digestJSON(cloneOwnership(state.Ownership)) != digestJSON(plan.ownershipFacts) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("projection ownership changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	combined := current.combinedPack()
	resolutionPack := combined
	if plan.operation == OperationDeactivate {
		resolutionPack = priorCombinedPack(plan, pack)
	}
	resolutions, err := f.resolveExecutables(ctx, resolutionPack, plan.surface, plan.operation == OperationActivate || plan.operation == OperationUpdate)
	if err != nil {
		return planPreflight{}, err
	}
	if !sameResolutions(plan.resolutions, resolutions) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("executable resolution changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	before := priorCombinedPack(plan, pack)
	observation, err := inspectSurface(ctx, adapter, surfaceTransitionFacts(plan.surface, plan.operation, before, combined, state.Ownership, resolutions))
	if err != nil {
		return planPreflight{}, err
	}
	if loadedIntentRevision != plan.intentRevision {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d; rerun %s to preview a fresh plan", plan.intentRevision, loadedIntentRevision, plan.operation)}
	}
	if observationDigest(observation) != plan.observationFingerprint {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("%s projections changed after Preview; rerun %s to preview a fresh plan", plan.surface, plan.operation)}
	}
	return planPreflight{pack: pack, adapter: adapter, state: state, composition: current, combined: combined, resolutions: resolutions}, nil
}

func (f Facade) activationInputsForOperation(ctx context.Context, request ActivationRequest, operation Operation) (Pack, SurfaceAdapter, ActivationState, error) {
	if f.activation == nil || f.activation.store == nil {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("activation is not configured")
	}
	if request.Surface != SurfaceCodex && request.Surface != SurfaceOpenCode && request.Surface != SurfaceClaude {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("activation does not support CLI surface %q", request.Surface)
	}
	pack, err := f.catalog.catalogMetadata(request.PackID)
	if err != nil {
		return Pack{}, nil, ActivationState{}, err
	}
	adapter := f.activation.adapters[request.Surface]
	if adapter == nil {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("no activation adapter configured for CLI surface %q", request.Surface)
	}
	state, err := loadActivationState(ctx, f.activation.store, request.Surface)
	if err != nil {
		return Pack{}, nil, ActivationState{}, err
	}
	intent, hasIntent := intentForPack(state, request.PackID, request.Surface)
	if operation == OperationActivate && hasIntent && intent.Active && intent.Version != pack.Version {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("capability pack %q is active at %s on %s; use explicit pack update to target catalog current %s", request.PackID, intent.Version, request.Surface, pack.Version)
	}
	pack, err = f.catalog.Show(request.PackID)
	if err != nil {
		return Pack{}, nil, ActivationState{}, err
	}
	return pack, adapter, state, nil
}

func (p *ReconciliationPlan) seal() {
	for i := range p.phases {
		p.phases[i].Digest = digestJSON(struct {
			Kind             ConsentKind
			ApprovalRequired bool
			Actions          []ProjectionAction
		}{p.phases[i].Kind, p.phases[i].ApprovalRequired, p.phases[i].Actions})
	}
	p.digest = digestJSON(p.sealPayload())
	p.id = "plan-" + p.digest[:12]
}

func (p *ReconciliationPlan) captureSensitiveEffects() {
	p.sensitiveEffects = sensitiveEffectOriginsForComposition(
		p.compositionFacts,
		p.activations,
		p.intentFacts,
		p.pack.ID,
		p.selection,
	)
}
func (p ReconciliationPlan) validSeal() bool {
	copy := p
	copy.seal()
	return copy.digest == p.digest && copy.id == p.id
}
func (p ReconciliationPlan) sealPayload() any {
	return struct {
		PackID, Version   string
		Operation         Operation
		Surface           Surface
		IntentRevision    int
		DocumentRevision  int
		OldVersion        string
		Observation       string
		Phases            []PlanPhase
		Desired           []projectionExpectation
		Portable          []PortableOutcome
		Resolutions       []ExecutableResolution
		RuntimeModes      []RuntimeModeResult
		SensitiveEffects  []SensitiveEffectOrigin
		Readiness         ReadinessStatus
		Pending           []string
		NoOp              bool
		Activations       []PlannedActivation
		Blockers          []PlanBlocker
		Composition       []Pack
		IntentFacts       []ActivationIntent
		BeforeIntentFacts []ActivationIntent
		OwnershipFacts    []ProjectionOwnership
		Before            []Pack
		Aliases           []SurfaceAlias
		PreviousAliases   []SurfaceAlias
		Selection         ResourceSelection
		PreviousSelection ResourceSelection
		PartialSelection  bool
		SelectionValidity SelectionValidity
		Force             bool
	}{p.pack.ID, p.pack.Version, p.operation, p.surface, p.intentRevision, p.documentRevision, p.oldVersion, p.observationFingerprint, p.phases, p.desired, p.portable, p.resolutions, p.runtimeModeResults, p.sensitiveEffects, p.readiness, p.pendingHumanActions, p.noOp, p.activations, p.blockers, p.compositionFacts, p.intentFacts, p.beforeIntentFacts, p.ownershipFacts, p.beforeCompositionFacts, p.aliases, p.previousAliases, p.selection, p.previousSelection, p.partialSelection, p.selectionValidity, p.force}
}

func ownershipByID(values []ProjectionOwnership, id string) (ProjectionOwnership, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return ProjectionOwnership{}, false
}

func projectionOwnershipID(projection ObservedProjection) string {
	if projection.ProjectionKey != "" {
		return projection.ProjectionKey
	}
	if projection.Action.ProjectionKey != "" {
		return projection.Action.ProjectionKey
	}
	return projection.ID
}

func physicalProjectionID(surface Surface, projection ObservedProjection) string {
	if projection.ProjectionKey != "" || projection.Action.ProjectionKey != "" {
		return projectionOwnershipID(projection)
	}
	return "surface:" + string(surface) + ":" + projection.ID
}

func ownershipIDForState(state ActivationState, surface Surface, projection ObservedProjection) string {
	if state.snapshotManaged {
		return physicalProjectionID(surface, projection)
	}
	return projectionOwnershipID(projection)
}

func appendPhaseAction(phases []PlanPhase, kind ConsentKind, action ProjectionAction) []PlanPhase {
	for i := range phases {
		if phases[i].Kind == kind {
			phases[i].Actions = append(phases[i].Actions, action)
			return phases
		}
	}
	return append(phases, PlanPhase{Kind: kind, ApprovalRequired: true, Actions: []ProjectionAction{action}})
}

func hasExternalCommand(phases []PlanPhase) bool {
	for _, action := range externalEffectActions(phases) {
		if action.Kind == ActionExternalCommand {
			return true
		}
	}
	return false
}

func externalEffectActions(phases []PlanPhase) []ProjectionAction {
	actions := phaseActions(phases, ConsentExecutableExternal)
	return append(actions, phaseActions(phases, ConsentToolHostSetup)...)
}

func cloneOwnership(values []ProjectionOwnership) []ProjectionOwnership {
	result := append([]ProjectionOwnership(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func intentForPack(state ActivationState, packID string, surface Surface) (ActivationIntent, bool) {
	for _, intent := range activeIntents(state) {
		if intent.PackID == packID && intent.Surface == surface {
			return intent, true
		}
	}
	return ActivationIntent{}, false
}

func compositionActive(state ActivationState, packs []Pack, surface Surface) bool {
	active := map[string]ActivationIntent{}
	for _, intent := range activeIntents(state) {
		active[intent.PackID] = intent
	}
	for _, pack := range packs {
		intent, ok := active[pack.ID]
		if !ok || !intent.Active || intent.Surface != surface || intent.Version != pack.Version {
			return false
		}
	}
	return len(packs) > 0
}

func ownershipMatchesPack(owners []ProjectionOwnership, projections []ObservedProjection, c composition) bool {
	for _, projection := range projections {
		if projection.ExternallyManaged {
			if projection.ObservedFingerprint != projection.DesiredFingerprint {
				return false
			}
			continue
		}
		owner, ok := ownershipByID(owners, physicalProjectionID(c.surface, projection))
		if !ok {
			owner, ok = ownershipByID(owners, projectionOwnershipID(projection))
		}
		if !ok || owner.Fingerprint != projection.DesiredFingerprint || owner.PackID != c.requested.ID || owner.Surface != c.surface {
			return false
		}
	}
	return true
}

func withoutExternallyManagedExpectations(values []projectionExpectation) []projectionExpectation {
	result := make([]projectionExpectation, 0, len(values))
	for _, value := range values {
		if !value.ExternallyManaged {
			result = append(result, value)
		}
	}
	return result
}
func digestJSON(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func observationDigest(o SurfaceInspection) string {
	type fingerprintProjection struct {
		ID                  string
		Exists              bool
		ObservedFingerprint string
		ExactFingerprint    string `json:",omitempty"`
		DesiredFingerprint  string
		AdapterProvenance   string `json:",omitempty"`
		ExternallyManaged   bool
		Action              ProjectionAction
	}
	var projections []fingerprintProjection
	for _, projection := range o.Projections {
		projections = append(projections, fingerprintProjection{
			ID: projection.ID, Exists: projection.Exists,
			ObservedFingerprint: projection.ObservedFingerprint,
			ExactFingerprint:    projection.ExactFingerprint,
			DesiredFingerprint:  projection.DesiredFingerprint,
			AdapterProvenance:   projection.AdapterProvenance,
			ExternallyManaged:   projection.ExternallyManaged,
			Action:              projection.Action,
		})
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].ID < projections[j].ID })
	pending := append([]string(nil), o.PendingHumanActions...)
	sort.Strings(pending)
	return digestJSON(struct {
		Revision            string
		Projections         []fingerprintProjection
		RuntimeModes        []runtimeModeFingerprint `json:",omitempty"`
		Readiness           ReadinessStatus
		PendingHumanActions []string
	}{Revision: o.Revision, Projections: projections, RuntimeModes: runtimeModeFingerprints(o.RuntimeModeResults), PendingHumanActions: pending})
}

type runtimeObservationFingerprint struct {
	State            ObservationState
	Reason           ObservationReason
	ObserverRevision string
	RedactedIdentity string
}

type runtimeModeFingerprint struct {
	ResourceID   string
	ModeID       string
	State        RuntimeModeState
	Requirements []runtimeObservationFingerprint
	Authorities  []runtimeObservationFingerprint
}

// runtimeModeFingerprints deliberately omits ObservedAt. A refreshed probe
// with the same semantic facts and observer revision must not stale a sealed
// plan merely because the adapter stamped a later wall-clock instant.
func runtimeModeFingerprints(values []RuntimeModeResult) []runtimeModeFingerprint {
	result := make([]runtimeModeFingerprint, 0, len(values))
	for _, value := range values {
		item := runtimeModeFingerprint{ResourceID: value.ResourceID, ModeID: value.ModeID, State: value.State, Requirements: []runtimeObservationFingerprint{}, Authorities: []runtimeObservationFingerprint{}}
		for _, fact := range value.Evidence.Requirements {
			item.Requirements = append(item.Requirements, runtimeObservationFingerprint{fact.State, fact.Reason, fact.ObserverRevision, fact.RedactedIdentity})
		}
		for _, fact := range value.Evidence.Authorities {
			item.Authorities = append(item.Authorities, runtimeObservationFingerprint{fact.State, fact.Reason, fact.ObserverRevision, fact.RedactedIdentity})
		}
		result = append(result, item)
	}
	return result
}
func verificationMatches(expected []projectionExpectation, values []ObservedProjection) bool {
	if len(values) != len(expected) || len(values) == 0 {
		return false
	}
	byID := map[string]ObservedProjection{}
	for _, value := range values {
		byID[value.ID] = value
	}
	for _, want := range expected {
		value, ok := byID[want.ID]
		if !ok || value.DesiredFingerprint != want.Fingerprint || value.ObservedFingerprint != want.Fingerprint {
			return false
		}
	}
	return true
}

func verificationMatchesDeactivation(expected []projectionExpectation, values []ObservedProjection) bool {
	byID := map[string]ObservedProjection{}
	for _, value := range values {
		byID[value.ID] = value
	}
	for _, want := range expected {
		value, ok := byID[want.ID]
		if !ok || value.ObservedFingerprint != want.Fingerprint || value.DesiredFingerprint != want.Fingerprint {
			return false
		}
		delete(byID, want.ID)
	}
	for _, value := range byID {
		if value.Exists {
			return false
		}
	}
	return true
}

func verificationMismatch(expected []projectionExpectation, values []ObservedProjection) string {
	want := map[string]string{}
	for _, projection := range expected {
		want[projection.ID] = projection.Fingerprint
	}
	got := map[string]string{}
	for _, projection := range values {
		got[projection.ID] = projection.ObservedFingerprint
	}
	var details []string
	ids := make([]string, 0, len(want)+len(got))
	seen := map[string]bool{}
	for id := range want {
		seen[id] = true
		ids = append(ids, id)
	}
	for id := range got {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if want[id] != got[id] {
			details = append(details, fmt.Sprintf("%s expected %s observed %s", id, want[id], got[id]))
		}
	}
	return fmt.Sprintf("expected %d projections, observed %d; %s", len(expected), len(values), strings.Join(details, "; "))
}
func ownedAtComposition(owners []ProjectionOwnership, projection ObservedProjection, fingerprint string, c composition) bool {
	for _, owner := range owners {
		identityMatches := owner.ID == physicalProjectionID(c.surface, projection) || owner.ID == projectionOwnershipID(projection)
		if identityMatches && owner.Fingerprint == fingerprint && owner.PackID == c.requested.ID && owner.Surface == c.surface {
			return true
		}
	}
	return false
}

func forceRepairEligible(owners []ProjectionOwnership, projection ObservedProjection, c composition) bool {
	if projection.Action.Target == "" {
		return false
	}
	owner, ok := ownershipByID(owners, physicalProjectionID(c.surface, projection))
	if !ok {
		owner, ok = ownershipByID(owners, projectionOwnershipID(projection))
	}
	if !ok {
		for _, candidate := range owners {
			if candidate.Target != "" && filepath.Clean(candidate.Target) == filepath.Clean(projection.Action.Target) {
				owner, ok = candidate, true
				break
			}
		}
	}
	return ok && owner.Target != "" && filepath.Clean(owner.Target) == filepath.Clean(projection.Action.Target) && owner.PackID == c.requested.ID && owner.Surface == c.surface
}

func receiptTargetMatches(owner ProjectionOwnership, projection ObservedProjection) bool {
	return owner.Target != "" && projection.Action.Target != "" && filepath.Clean(owner.Target) == filepath.Clean(projection.Action.Target)
}

func ownershipByReceiptTarget(owners []ProjectionOwnership, projection ObservedProjection) (ProjectionOwnership, bool) {
	var matched ProjectionOwnership
	found := false
	for _, owner := range owners {
		if !receiptTargetMatches(owner, projection) {
			continue
		}
		if found {
			return ProjectionOwnership{}, false
		}
		matched, found = owner, true
	}
	return matched, found
}

func ownershipForDeactivation(state ActivationState, surface Surface, projection ObservedProjection, force bool) (ProjectionOwnership, bool) {
	owner, owned := ownershipByID(state.Ownership, ownershipIDForState(state, surface, projection))
	if force && !owned {
		return ownershipByReceiptTarget(state.Ownership, projection)
	}
	return owner, owned
}

func receiptPermitsRemoval(owner ProjectionOwnership, projection ObservedProjection, force bool) bool {
	return owner.Fingerprint == projection.ObservedFingerprint || force && receiptTargetMatches(owner, projection)
}

func cloneActivationState(state ActivationState) ActivationState {
	state.Intent.Aliases = cloneAliases(state.Intent.Aliases)
	state.Intent.Selection = cloneSelection(state.Intent.Selection)
	state.Intent.Resources = append([]ResourceIdentity(nil), state.Intent.Resources...)
	state.Intent.ReadinessObligations = append([]ReadinessObligation(nil), state.Intent.ReadinessObligations...)
	state.Intent.ExternalRequirements = append([]string{}, state.Intent.ExternalRequirements...)
	state.Ownership = append([]ProjectionOwnership(nil), state.Ownership...)
	state.Intents = append([]ActivationIntent(nil), state.Intents...)
	for i := range state.Intents {
		state.Intents[i].Aliases = cloneAliases(state.Intents[i].Aliases)
		state.Intents[i].Selection = cloneSelection(state.Intents[i].Selection)
		state.Intents[i].Resources = append([]ResourceIdentity(nil), state.Intents[i].Resources...)
		state.Intents[i].ReadinessObligations = append([]ReadinessObligation(nil), state.Intents[i].ReadinessObligations...)
		state.Intents[i].ExternalRequirements = append([]string{}, state.Intents[i].ExternalRequirements...)
	}
	state.External = cloneExternalEffects(state.External)
	return state
}

func packResourceIdentities(pack Pack) []ResourceIdentity {
	resources := make([]ResourceIdentity, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources = append(resources, ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].String() < resources[j].String() })
	return resources
}

func cloneAliases(aliases []SurfaceAlias) []SurfaceAlias {
	if aliases == nil {
		return nil
	}
	return append([]SurfaceAlias{}, aliases...)
}

func requestedAliases(pack Pack, surface Surface, supplied []SurfaceAlias, state ActivationState, operation Operation) ([]SurfaceAlias, error) {
	if supplied == nil {
		if intent, ok := intentForPack(state, pack.ID, surface); ok {
			if operation != OperationActivate || intent.Active {
				return cloneAliases(intent.Aliases), nil
			}
		}
	}
	aliases := cloneAliases(supplied)
	if err := canonicalizeAliases(&aliases); err != nil {
		return nil, err
	}
	for _, alias := range aliases {
		if !idPattern.MatchString(alias.Name) {
			return nil, fmt.Errorf("activation alias name %q is invalid", alias.Name)
		}
		if !packHasAliasTarget(pack, alias, surface) {
			return nil, fmt.Errorf("activation alias %s:%s does not identify a resource in the resulting selected closure bound to %s in pack %q", alias.Kind, alias.ID, surface, pack.ID)
		}
	}
	return aliases, nil
}

func intentAliases(state ActivationState, packID string, surface Surface) []SurfaceAlias {
	if intent, ok := intentForPack(state, packID, surface); ok {
		return intent.Aliases
	}
	return nil
}

func aliasesForPack(pack Pack, surface Surface, aliases []SurfaceAlias) []SurfaceAlias {
	if aliases == nil {
		return nil
	}
	result := make([]SurfaceAlias, 0, len(aliases))
	for _, alias := range aliases {
		if packHasAliasTarget(pack, alias, surface) {
			result = append(result, alias)
		}
	}
	_ = canonicalizeAliases(&result)
	return result
}

func stateWithAliases(state ActivationState, packID string, surface Surface, version string, aliases []SurfaceAlias) ActivationState {
	state = cloneActivationState(state)
	intents := activeIntents(state)
	found := false
	for i := range intents {
		if intents[i].PackID == packID && intents[i].Surface == surface {
			intents[i].Aliases = cloneAliases(aliases)
			found = true
		}
	}
	if !found {
		intents = append(intents, ActivationIntent{PackID: packID, Surface: surface, Version: version, Active: true, Revision: state.Intent.Revision, Aliases: cloneAliases(aliases)})
	}
	state.Intents = intents
	for _, intent := range intents {
		if intent.PackID == packID && intent.Surface == surface {
			state.Intent = intent
			break
		}
	}
	return state
}

func stateWithSelection(state ActivationState, packID string, surface Surface, version string, selection ResourceSelection) ActivationState {
	selection, _ = canonicalSelection(selection)
	for i := range state.Intents {
		if state.Intents[i].PackID == packID && state.Intents[i].Surface == surface {
			state.Intents[i].Selection = cloneSelection(selection)
		}
	}
	if state.Intent.PackID == packID && state.Intent.Surface == surface {
		state.Intent.Selection = cloneSelection(selection)
	}
	return state
}

func (f Facade) externalPlan(operation Operation, pack Pack, surface Surface, state ActivationState, resolutions []ExecutableResolution) ([]ProjectionAction, []PlanBlocker) {
	var actions []ProjectionAction
	var blockers []PlanBlocker
	for _, resolution := range resolutions {
		if !resolution.Available {
			if resolution.Capability == "" {
				blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, fmt.Sprintf("required executable %s is missing from PATH; install it and retry", resolution.Tool)})
				continue
			}
			if operation != OperationActivate && operation != OperationUpdate {
				blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, "executable acquisition requires an explicit activation or update; preview one of those operations before retrying"})
				continue
			}
			if !resolution.AcquisitionSupported || strings.TrimSpace(resolution.AcquisitionCommand) == "" {
				blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, "no supported acquisition action is available; configure a supported acquisition or install it before retrying"})
				continue
			}
			if strings.TrimSpace(resolution.AcquisitionSource) == "" || strings.TrimSpace(resolution.AcquisitionVersion) == "" {
				blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, "supported acquisition did not resolve an exact source and version; refresh the acquisition metadata before retrying"})
				continue
			}
			acquisition := ProjectionAction{
				ID: "external:" + resolution.Tool + ":acquire", Kind: ActionExternalCommand, Consent: ConsentExecutableExternal,
				Source: resolution.AcquisitionSource, Version: resolution.AcquisitionVersion,
				Command: resolution.AcquisitionCommand, Args: append([]string(nil), resolution.AcquisitionArgs...),
				Description:    fmt.Sprintf("acquire global tool %s via %s %s", resolution.Tool, resolution.AcquisitionCommand, strings.Join(resolution.AcquisitionArgs, " ")),
				Consequences:   fmt.Sprintf("installs the shared global executable %s at %s for Packy and other workflows", resolution.Tool, resolution.Path),
				RollbackLimits: "pack deactivation does not uninstall the shared executable or delete tool-owned data and credentials",
			}
			if !externalEffectCompleted(state.External, acquisition) {
				actions = append(actions, acquisition)
			}
		}
		if resolution.Capability != SurfaceCapabilityEngramIntegration {
			continue
		}
		if strings.TrimSpace(resolution.Path) == "" {
			blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, "resolved tool has no executable path"})
			continue
		}
		if surface == SurfaceClaude && hasNativeMCPBinding(pack, surface, resolution.Tool) {
			// Claude's official user-scoped MCP projection is the setup effect.
			// Running a tool-owned generic setup would import a second lifecycle.
			continue
		}
		setup := ProjectionAction{
			ID: "external:" + resolution.Tool + ":setup:" + string(surface), Kind: ActionExternalCommand, Consent: ConsentToolHostSetup,
			Source: resolution.Path, Version: resolution.AcquisitionVersion, Command: resolution.Path, Args: []string{"setup", string(surface)},
			Description:    fmt.Sprintf("run %s setup %s", resolution.Path, surface),
			Consequences:   fmt.Sprintf("allows %s to mutate the %s host configuration for its tool-owned setup", resolution.Tool, surfaceDisplayName(surface)),
			RollbackLimits: "pack deactivation removes Packy-owned projections but does not delete tool-owned configuration, data, or credentials",
		}
		if externalEffectCompleted(state.External, setup) {
			continue
		}
		if operation != OperationActivate && operation != OperationUpdate {
			blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, "tool-owned host setup requires an explicit activation or update; preview one of those operations before retrying"})
			continue
		}
		actions = append(actions, setup)
	}
	sortBlockers(blockers)
	return actions, blockers
}

func surfaceDisplayName(surface Surface) string {
	switch surface {
	case SurfaceCodex:
		return "Codex"
	case SurfaceOpenCode:
		return "OpenCode"
	case SurfaceClaude:
		return "Claude Code"
	default:
		return string(surface)
	}
}

func hasNativeMCPBinding(pack Pack, surface Surface, tool string) bool {
	for _, resource := range pack.Resources {
		if resource.Kind != "mcp_server" || resource.Command != tool {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface == surface && binding.Projection == "mcp_server" {
				return true
			}
		}
	}
	return false
}

// inspectSurface is the only gateway from capability-pack policy to host
// observation. It isolates caller and adapter memory, validates the complete
// contract, and canonicalizes facts used by planning and plan sealing.
func inspectSurface(ctx context.Context, adapter SurfaceAdapter, transition SurfaceTransition) (SurfaceInspection, error) {
	transition = cloneSurfaceTransition(transition)
	observation, err := adapter.InspectSurface(ctx, transition)
	if err != nil {
		return SurfaceInspection{}, err
	}
	observation = cloneSurfaceInspection(observation)
	if provider, ok := adapter.(interface {
		controlledCheckDescriptor() ControlledCheckDescriptor
	}); ok {
		descriptor := provider.controlledCheckDescriptor()
		if observation.ControlledCheck.AdapterVersion == "" {
			observation.ControlledCheck.AdapterVersion = descriptor.AdapterVersion
		}
		if observation.ControlledCheck.HostVersion == "" {
			observation.ControlledCheck.HostVersion = descriptor.HostVersion
		}
		if len(observation.ControlledCheck.Instructions) == 0 {
			observation.ControlledCheck.Instructions = append([]string(nil), descriptor.Instructions...)
		}
	}
	seen := make(map[string]struct{}, len(observation.Projections))
	for i := range observation.Projections {
		projection := &observation.Projections[i]
		if projection.ID == "" || projection.Action.ID != projection.ID {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned a malformed projection identity")
		}
		if _, duplicate := seen[projection.ID]; duplicate {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned duplicate projection %q", projection.ID)
		}
		if projection.Action.Consent != "" && projection.Action.Consent != ConsentExecutableExternal {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned unsupported consent %q for projection %q", projection.Action.Consent, projection.ID)
		}
		seen[projection.ID] = struct{}{}
		if transition.ProjectRoot != "" {
			if projection.Action.Target == "" {
				return SurfaceInspection{}, fmt.Errorf("project surface adapter omitted target for projection %q", projection.ID)
			}
			if _, err := RelativeProjectTarget(transition.ProjectRoot, projection.Action.Target); err != nil {
				return SurfaceInspection{}, err
			}
			if !projection.Action.PreviewOnly {
				return SurfaceInspection{}, fmt.Errorf("project preview adapter returned applicable projection %q before project mutation exists", projection.ID)
			}
		} else if projection.Action.PreviewOnly {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned preview-only projection %q outside a project preview", projection.ID)
		}
		switch projection.Goal {
		case ProjectionPresent:
			if projection.DesiredFingerprint == "" || projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent {
				return SurfaceInspection{}, fmt.Errorf("surface adapter returned incompatible present goal for projection %q", projection.ID)
			}
		case ProjectionAbsent:
			if projection.DesiredFingerprint != "" || (projection.Action.Mode != ProjectionDeleteTarget && projection.Action.Mode != ProjectionRemoveContent) {
				return SurfaceInspection{}, fmt.Errorf("surface adapter returned incompatible absent goal for projection %q", projection.ID)
			}
		default:
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned zero goal for projection %q", projection.ID)
		}
	}
	activationActions := make(map[string]struct{}, len(observation.ProjectActivationActions))
	for _, action := range observation.ProjectActivationActions {
		key := action.ID + "\x00" + action.Target
		if transition.ProjectRoot == "" || transition.ProjectInstallation == nil {
			return SurfaceInspection{}, errors.New("surface adapter returned a personal project action outside locked project inspection")
		}
		if err := validatePersonalProjectAction(action); err != nil {
			return SurfaceInspection{}, err
		}
		if _, duplicate := activationActions[key]; duplicate {
			return SurfaceInspection{}, errors.New("surface adapter returned a duplicate personal Codex project action")
		}
		activationActions[key] = struct{}{}
	}
	expectedEffects := make(map[string]ProjectActivationEffectReceipt, len(transition.ProjectEffectReceipts))
	for _, receipt := range transition.ProjectEffectReceipts {
		key := string(receipt.Action) + "\x00" + receipt.Target
		if _, duplicate := expectedEffects[key]; duplicate || !validProjectActivationEffectReceipt(receipt) {
			return SurfaceInspection{}, errors.New("project effect transition contains malformed or duplicate receipts")
		}
		expectedEffects[key] = receipt
	}
	observedEffects := make(map[string]struct{}, len(observation.ProjectDeactivationEffects))
	for _, effect := range observation.ProjectDeactivationEffects {
		key := string(effect.Kind) + "\x00" + effect.Target
		receipt, expected := expectedEffects[key]
		if !expected {
			return SurfaceInspection{}, errors.New("surface adapter returned an unreceipted personal project effect")
		}
		if _, duplicate := observedEffects[key]; duplicate || effect.AdapterProvenance == "" {
			return SurfaceInspection{}, errors.New("surface adapter returned a malformed or duplicate personal project effect")
		}
		observedEffects[key] = struct{}{}
		switch effect.State {
		case ProjectEffectAbsent, ProjectEffectDrifted:
			if effect.Action.Kind != "" {
				return SurfaceInspection{}, errors.New("surface adapter granted a reversal action for a non-exact personal project effect")
			}
		case ProjectEffectExact:
			action := effect.Action
			if effect.AdapterProvenance != receipt.AdapterProvenance || action.Kind != receipt.Action || action.Surface != receipt.Surface || filepath.Clean(action.Target) != filepath.Clean(receipt.Target) || action.AdapterProvenance != receipt.AdapterProvenance || action.Consent != ConsentDestructiveCleanup || !action.PreviewOnly || action.Content == "" || action.Precondition == "" || action.FileMode == 0 || action.Mode != "" {
				return SurfaceInspection{}, errors.New("surface adapter returned a malformed receipted personal project reversal")
			}
		default:
			return SurfaceInspection{}, errors.New("surface adapter returned an unsupported personal project effect state")
		}
	}
	if len(observedEffects) != len(expectedEffects) {
		return SurfaceInspection{}, errors.New("surface adapter omitted a receipted personal project effect")
	}
	unrepresentable := make(map[string]struct{}, len(observation.Unrepresentable))
	for _, resource := range observation.Unrepresentable {
		key := resource.Resource.String()
		if resource.Resource.Kind == "" || resource.Resource.ID == "" || resource.Reason == "" {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned malformed unrepresentable resource %q", key)
		}
		if _, duplicate := unrepresentable[key]; duplicate {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned duplicate unrepresentable resource %q", key)
		}
		unrepresentable[key] = struct{}{}
	}
	occupied := make(map[string]struct{}, len(observation.OccupiedNames))
	for _, name := range observation.OccupiedNames {
		key := name.Namespace + ":" + name.Name
		if name.Namespace == "" || name.Name == "" || name.Fingerprint == "" || (name.OwnerType != "reserved" && name.OwnerType != "unmanaged" && name.OwnerType != "packy") {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned malformed occupied name %q", key)
		}
		if _, duplicate := occupied[key]; duplicate {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned duplicate occupied name %q", key)
		}
		occupied[key] = struct{}{}
	}
	optionalModes := make(map[string]OptionalMode)
	declaredOptionalAuthorities := make(map[string]struct{})
	contract := transition.Desired.Contract
	if transition.Desired.ID == "" {
		contract = transition.Prior.Contract
	}
	for _, mode := range contract.OptionalModes {
		if _, duplicate := optionalModes[mode.ID]; duplicate {
			return SurfaceInspection{}, fmt.Errorf("surface transition declared duplicate optional mode %q", mode.ID)
		}
		optionalModes[mode.ID] = mode
		for _, authority := range mode.Authorities {
			key := mode.ID + ":" + authority
			if _, duplicate := declaredOptionalAuthorities[key]; duplicate {
				return SurfaceInspection{}, fmt.Errorf("surface transition declared duplicate optional authority %q", key)
			}
			declaredOptionalAuthorities[key] = struct{}{}
		}
	}
	optionalAuthorities := make(map[string]struct{}, len(observation.Readiness.OptionalAuthorities))
	for _, authority := range observation.Readiness.OptionalAuthorities {
		key := authority.ModeID + ":" + authority.Authority
		mode, modeExists := optionalModes[authority.ModeID]
		if authority.ModeID == "" || authority.Authority == "" || authority.Fallback == "" || !modeExists || !slices.Contains(mode.Authorities, authority.Authority) || authority.Fallback != mode.Fallback {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned malformed optional authority %q", key)
		}
		switch authority.State {
		case OptionalAuthorityAvailable, OptionalAuthorityUnavailable, OptionalAuthorityUnknown:
		default:
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned unsupported optional authority state %q for %q", authority.State, key)
		}
		if _, duplicate := optionalAuthorities[key]; duplicate {
			return SurfaceInspection{}, fmt.Errorf("surface adapter returned duplicate optional authority %q", key)
		}
		optionalAuthorities[key] = struct{}{}
	}
	for key := range declaredOptionalAuthorities {
		if _, observed := optionalAuthorities[key]; !observed {
			return SurfaceInspection{}, fmt.Errorf("surface adapter omitted optional authority %q", key)
		}
	}
	relevantPack := transition.Desired
	if transition.Prior.ID != "" {
		relevantPack = transition.Prior
	}
	results, err := EvaluateRuntimeModes(relevantPack, observation.RuntimeModeEvidence, time.Now().UTC().Truncate(time.Second), runtimeEvidenceFreshness)
	if err != nil {
		return SurfaceInspection{}, err
	}
	observation.RuntimeModeResults = results
	sort.Slice(observation.Projections, func(i, j int) bool { return observation.Projections[i].ID < observation.Projections[j].ID })
	sort.Slice(observation.Unrepresentable, func(i, j int) bool {
		return observation.Unrepresentable[i].Resource.String() < observation.Unrepresentable[j].Resource.String()
	})
	sort.Slice(observation.OccupiedNames, func(i, j int) bool {
		if observation.OccupiedNames[i].Namespace != observation.OccupiedNames[j].Namespace {
			return observation.OccupiedNames[i].Namespace < observation.OccupiedNames[j].Namespace
		}
		return observation.OccupiedNames[i].Name < observation.OccupiedNames[j].Name
	})
	sort.Slice(observation.ProjectActivationActions, func(i, j int) bool {
		return observation.ProjectActivationActions[i].ID+"\x00"+observation.ProjectActivationActions[i].Target < observation.ProjectActivationActions[j].ID+"\x00"+observation.ProjectActivationActions[j].Target
	})
	sort.Slice(observation.ProjectDeactivationEffects, func(i, j int) bool {
		return string(observation.ProjectDeactivationEffects[i].Kind)+"\x00"+observation.ProjectDeactivationEffects[i].Target < string(observation.ProjectDeactivationEffects[j].Kind)+"\x00"+observation.ProjectDeactivationEffects[j].Target
	})
	sort.Strings(observation.PendingHumanActions)
	sort.Strings(observation.Readiness.PendingHumanActions)
	sort.Strings(observation.Readiness.Evidence)
	sort.Slice(observation.Readiness.OptionalAuthorities, func(i, j int) bool {
		if observation.Readiness.OptionalAuthorities[i].ModeID != observation.Readiness.OptionalAuthorities[j].ModeID {
			return observation.Readiness.OptionalAuthorities[i].ModeID < observation.Readiness.OptionalAuthorities[j].ModeID
		}
		return observation.Readiness.OptionalAuthorities[i].Authority < observation.Readiness.OptionalAuthorities[j].Authority
	})
	sort.Slice(observation.RuntimeModeEvidence, func(i, j int) bool {
		return runtimeModeEvidenceKey(observation.RuntimeModeEvidence[i].ResourceID, observation.RuntimeModeEvidence[i].ModeID) <
			runtimeModeEvidenceKey(observation.RuntimeModeEvidence[j].ResourceID, observation.RuntimeModeEvidence[j].ModeID)
	})
	return observation, nil
}

func validatePersonalProjectAction(action ProjectionAction) error {
	commonMalformed := !action.PreviewOnly || action.Target == "" || !filepath.IsAbs(action.Target) || action.Content == "" || action.Version == "" || action.Precondition == "" || action.FileMode == 0 || action.Mode != "" || action.Consent != "" || action.AdapterProvenance == "" || action.Source != "" || action.Command != "" || len(action.Args) != 0
	switch action.Kind {
	case ActionCodexProjectTrust:
		if commonMalformed || action.ID != "project_trust:codex" || action.Surface != SurfaceCodex || action.ContributionStartMarker == "" || action.ContributionEndMarker == "" {
			return errors.New("surface adapter returned a malformed personal Codex project action")
		}
	default:
		return errors.New("surface adapter returned an unsupported personal project action")
	}
	return nil
}

func surfaceTransitionFacts(surface Surface, operation Operation, prior, desired Pack, ownership []ProjectionOwnership, resolutions []ExecutableResolution) SurfaceTransition {
	adapterOwnership := adapterOwnershipForSurface(ownership, surface)
	transition := SurfaceTransition{Desired: desired, CurrentOwnership: adapterOwnership, ResolvedExecutables: resolutions}
	switch operation {
	case OperationDeactivate, OperationUpdate:
		transition.Prior = prior
	}
	return transition
}

func adapterOwnershipForSurface(ownership []ProjectionOwnership, surface Surface) []ProjectionOwnership {
	var result []ProjectionOwnership
	for _, durable := range cloneOwnership(ownership) {
		if strings.HasPrefix(durable.ID, "surface:") && !strings.HasPrefix(durable.ID, "surface:"+string(surface)+":") {
			continue
		}
		if durable.ProjectionID != "" {
			durable.PhysicalID = durable.ID
			durable.ID = durable.ProjectionID
		}
		result = append(result, durable)
	}
	return result
}

func cloneSurfaceTransition(value SurfaceTransition) SurfaceTransition {
	value.Prior = clonePack(value.Prior)
	value.Desired = clonePack(value.Desired)
	value.CurrentOwnership = cloneOwnership(value.CurrentOwnership)
	value.ResidualOwnership = cloneOwnership(value.ResidualOwnership)
	value.ResolvedExecutables = cloneResolutions(value.ResolvedExecutables)
	value.ProjectEffectReceipts = append([]ProjectActivationEffectReceipt(nil), value.ProjectEffectReceipts...)
	if value.ProjectInstallation != nil {
		data, _ := json.Marshal(value.ProjectInstallation)
		var installation ProjectInstallation
		_ = json.Unmarshal(data, &installation)
		if hydrated, err := hydrateProjectLock(installation.Lock); err == nil {
			installation.Lock = hydrated
		}
		value.ProjectInstallation = &installation
	}
	return value
}

func cloneSurfaceInspection(value SurfaceInspection) SurfaceInspection {
	value.ControlledCheck.Instructions = append([]string(nil), value.ControlledCheck.Instructions...)
	value.Projections = append([]ObservedProjection(nil), value.Projections...)
	value.OccupiedNames = append([]OccupiedName(nil), value.OccupiedNames...)
	value.RuntimeModeEvidence = cloneRuntimeModeEvidence(value.RuntimeModeEvidence)
	value.RuntimeModeResults = cloneRuntimeModeResults(value.RuntimeModeResults)
	for i := range value.Projections {
		value.Projections[i].Action.Args = append([]string(nil), value.Projections[i].Action.Args...)
	}
	value.ProjectActivationActions = append([]ProjectionAction(nil), value.ProjectActivationActions...)
	for i := range value.ProjectActivationActions {
		value.ProjectActivationActions[i].Args = append([]string(nil), value.ProjectActivationActions[i].Args...)
	}
	value.ProjectDeactivationEffects = append([]ObservedProjectEffect(nil), value.ProjectDeactivationEffects...)
	for i := range value.ProjectDeactivationEffects {
		value.ProjectDeactivationEffects[i].Action.Args = append([]string(nil), value.ProjectDeactivationEffects[i].Action.Args...)
	}
	value.PendingHumanActions = append([]string(nil), value.PendingHumanActions...)
	value.Unrepresentable = append([]UnrepresentableResource(nil), value.Unrepresentable...)
	value.Readiness.PendingHumanActions = append([]string(nil), value.Readiness.PendingHumanActions...)
	value.Readiness.Evidence = append([]string(nil), value.Readiness.Evidence...)
	value.Readiness.OptionalAuthorities = cloneOptionalAuthorities(value.Readiness.OptionalAuthorities)
	return value
}

func cloneOptionalAuthorities(values []OptionalAuthorityObservation) []OptionalAuthorityObservation {
	return append([]OptionalAuthorityObservation(nil), values...)
}

func cloneResolutions(values []ExecutableResolution) []ExecutableResolution {
	result := append([]ExecutableResolution(nil), values...)
	for i := range result {
		result[i].AcquisitionArgs = append([]string(nil), result[i].AcquisitionArgs...)
	}
	return result
}

func (f Facade) resolveExecutables(ctx context.Context, pack Pack, surface Surface, includeAcquisition bool) ([]ExecutableResolution, error) {
	if len(pack.Requires.Tools) == 0 {
		return nil, nil
	}
	if f.activation == nil || f.activation.resolver == nil {
		return nil, fmt.Errorf("pack %q requires an executable resolver", pack.ID)
	}
	result := make([]ExecutableResolution, 0, len(pack.Requires.Tools))
	for _, tool := range pack.Requires.Tools {
		resolution, err := f.activation.resolver.Resolve(ctx, tool)
		if err != nil {
			return nil, fmt.Errorf("resolve required executable %q: %w", tool, err)
		}
		resolution.Tool = tool
		if capability, ok := pack.externalToolCapability(surface, tool); ok {
			resolution.Capability = capability
			if includeAcquisition && !resolution.Available && f.activation.acquirers != nil {
				if acquirer := f.activation.acquirers[capability]; acquirer != nil {
					acquisition, acquisitionErr := acquirer.ResolveAcquisition(ctx)
					if acquisitionErr != nil {
						return nil, fmt.Errorf("resolve acquisition for required executable %q: %w", tool, acquisitionErr)
					}
					resolution.Path = acquisition.Path
					resolution.AcquisitionSupported = acquisition.Command != ""
					resolution.AcquisitionCommand = acquisition.Command
					resolution.AcquisitionArgs = append([]string(nil), acquisition.Args...)
					resolution.AcquisitionSource = acquisition.Source
					resolution.AcquisitionVersion = acquisition.Version
				}
			}
		}
		resolution.AcquisitionArgs = append([]string(nil), resolution.AcquisitionArgs...)
		if resolution.Precondition == "" {
			resolution.Precondition = resolutionFingerprint(resolution)
		}
		result = append(result, resolution)
	}
	return result, nil
}

func ResolvedExecutablePath(command string, resolutions []ExecutableResolution) string {
	for _, resolution := range resolutions {
		if resolution.Tool == command && resolution.Path != "" {
			return resolution.Path
		}
	}
	return command
}

func resolutionFingerprint(resolution ExecutableResolution) string {
	return digestJSON(struct {
		Tool, Path, ResolvedPath, Origin, Version, Precondition string
		Available, AcquisitionSupported                         bool
		AcquisitionCommand                                      string
		AcquisitionArgs                                         []string
		Capability                                              SurfaceCapabilityType
	}{resolution.Tool, resolution.Path, resolution.ResolvedPath, resolution.Origin, resolution.Version, "", resolution.Available, resolution.AcquisitionSupported, resolution.AcquisitionCommand, resolution.AcquisitionArgs, resolution.Capability})
}

func sameResolutions(want, got []ExecutableResolution) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if resolutionFingerprint(want[i]) != resolutionFingerprint(got[i]) || want[i].Precondition != got[i].Precondition {
			return false
		}
	}
	return true
}

func externalEffectFingerprint(action ProjectionAction) string {
	return digestJSON(struct {
		ID, Kind, Command, Description string
		Args                           []string
	}{action.ID, string(action.Kind), action.Command, action.Description, action.Args})
}

func externalEffectCompleted(effects []ExternalEffect, action ProjectionAction) bool {
	want := externalEffectFingerprint(action)
	for _, effect := range effects {
		if effect.ID == action.ID && effect.Fingerprint == want {
			return true
		}
	}
	return false
}

func recordExternalEffect(effects []ExternalEffect, action ProjectionAction) []ExternalEffect {
	result := append([]ExternalEffect(nil), effects...)
	want := externalEffectFingerprint(action)
	for i := range result {
		if result[i].ID == action.ID {
			result[i].Fingerprint = want
			return result
		}
	}
	return append(result, ExternalEffect{ID: action.ID, Fingerprint: want})
}

func recordExternalReceipts(effects []ExternalEffect, actions []ProjectionAction, surface Surface, packs []Pack, before, after SurfaceInspection) []ExternalEffect {
	result := cloneExternalEffects(effects)
	for _, action := range actions {
		if action.Kind != ActionExternalCommand || action.Consent != ConsentToolHostSetup {
			continue
		}
		var contributions []ExternalContribution
		for _, effect := range result {
			if effect.ID == action.ID && effect.Receipt != nil {
				contributions = append(contributions, effect.Receipt.Contributions...)
				break
			}
		}
		for _, observed := range after.Projections {
			if !observed.ExternallyManaged || !observed.Exists || observed.ObservedFingerprint != observed.DesiredFingerprint {
				continue
			}
			prior, existed := observedProjectionByID(before.Projections, observed.ID)
			existing := -1
			for i := range contributions {
				if contributions[i].ID == observed.ID {
					existing = i
					break
				}
			}
			if existing < 0 && existed && prior.Exists {
				// A setup command that converged or replaced an existing contribution
				// did not create deletion authority for that contribution.
				continue
			}
			exact := observed.ExactFingerprint
			if exact == "" {
				exact = observed.ObservedFingerprint
			}
			contribution := ExternalContribution{ID: observed.ID, ObservedFingerprint: exact, AdapterProvenance: observed.AdapterProvenance}
			if existing >= 0 {
				contributions[existing] = contribution
			} else {
				contributions = append(contributions, contribution)
			}
		}
		if len(contributions) == 0 {
			continue
		}
		sort.Slice(contributions, func(i, j int) bool { return contributions[i].ID < contributions[j].ID })
		tool := externalSetupTool(action.ID)
		receipt := &ExternalEffectReceipt{
			SchemaVersion: 1, EffectID: action.ID, EffectFingerprint: externalEffectFingerprint(action), Surface: surface,
			PackID: externalReceiptPackID(packs, tool), Contributions: contributions,
			Reversal: ExternalReversalContract{
				SchemaVersion: 1, Consent: ConsentDestructiveCleanup,
				AuthorityLimits: []string{"configuration contributions recorded by this receipt only", "external executable, service, memory, data, sessions, credentials, and unrelated configuration are preserved"},
			},
		}
		for i := range result {
			if result[i].ID == action.ID && result[i].Fingerprint == receipt.EffectFingerprint {
				result[i].Receipt = receipt
			}
		}
	}
	return result
}

func observedProjectionByID(values []ObservedProjection, id string) (ObservedProjection, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return ObservedProjection{}, false
}

func externalSetupTool(effectID string) string {
	const prefix = "external:"
	value := strings.TrimPrefix(effectID, prefix)
	tool, _, _ := strings.Cut(value, ":setup:")
	return tool
}

func externalReceiptPackID(packs []Pack, tool string) string {
	for _, pack := range packs {
		if slices.Contains(pack.Requires.Tools, tool) {
			return pack.ID
		}
	}
	return ""
}

func cloneExternalEffects(values []ExternalEffect) []ExternalEffect {
	result := append([]ExternalEffect(nil), values...)
	for i := range result {
		if result[i].Receipt == nil {
			continue
		}
		receipt := *result[i].Receipt
		receipt.Contributions = append([]ExternalContribution(nil), receipt.Contributions...)
		receipt.Reversal.AuthorityLimits = append([]string(nil), receipt.Reversal.AuthorityLimits...)
		result[i].Receipt = &receipt
	}
	return result
}

func receiptForExternalProjection(effects []ExternalEffect, surface Surface, projection ObservedProjection, observations []ObservedProjection, completed []string) (*ExternalEffectReceipt, bool) {
	exact := projection.ExactFingerprint
	if exact == "" {
		exact = projection.ObservedFingerprint
	}
	for _, effect := range effects {
		receipt := effect.Receipt
		if receipt == nil || receipt.SchemaVersion != 1 || receipt.Reversal.SchemaVersion != 1 || receipt.Reversal.Consent != ConsentDestructiveCleanup ||
			receipt.EffectID != effect.ID || receipt.EffectFingerprint != effect.Fingerprint || receipt.Surface != surface || receipt.PackID == "" {
			continue
		}
		allExact := true
		for _, sealed := range receipt.Contributions {
			fresh, ok := observedProjectionByID(observations, sealed.ID)
			freshExact := fresh.ExactFingerprint
			if freshExact == "" {
				freshExact = fresh.ObservedFingerprint
			}
			completedAndAbsent := ok && !fresh.Exists && slices.Contains(completed, sealed.ID)
			if !completedAndAbsent && (!ok || !fresh.Exists || freshExact != sealed.ObservedFingerprint || fresh.AdapterProvenance != sealed.AdapterProvenance) {
				allExact = false
				break
			}
		}
		if !allExact {
			continue
		}
		for _, contribution := range receipt.Contributions {
			if contribution.ID == projection.ID && contribution.ObservedFingerprint == exact && contribution.AdapterProvenance == projection.AdapterProvenance {
				return receipt, true
			}
		}
	}
	return nil, false
}

func externalReceiptOwnerRemains(receipt *ExternalEffectReceipt, packs []Pack) bool {
	if receipt == nil {
		return false
	}
	return externalReceiptPackID(packs, externalSetupTool(receipt.EffectID)) == receipt.PackID
}

func externalReceiptReversalAction(action ProjectionAction, receipt *ExternalEffectReceipt) ProjectionAction {
	action.Consent = ConsentDestructiveCleanup
	action.Consequences = "removes only the exact external configuration contribution sealed by receipt " + receipt.EffectID
	action.RollbackLimits = strings.Join(receipt.Reversal.AuthorityLimits, "; ")
	return action
}

func refreshExternalReceiptOwner(effects []ExternalEffect, packs []Pack, surface Surface) []ExternalEffect {
	result := cloneExternalEffects(effects)
	for i := range result {
		if result[i].Receipt == nil || result[i].Receipt.Surface != surface {
			continue
		}
		packID := externalReceiptPackID(packs, externalSetupTool(result[i].Receipt.EffectID))
		if packID != "" {
			result[i].Receipt.PackID = packID
		}
	}
	return result
}

func retireExternalReceipts(effects []ExternalEffect, completedActions []string) []ExternalEffect {
	removed := map[string]bool{}
	for _, id := range completedActions {
		removed[id] = true
	}
	result := make([]ExternalEffect, 0, len(effects))
	for _, effect := range cloneExternalEffects(effects) {
		if effect.Receipt == nil {
			result = append(result, effect)
			continue
		}
		complete := len(effect.Receipt.Contributions) > 0
		for _, contribution := range effect.Receipt.Contributions {
			complete = complete && removed[contribution.ID]
		}
		if !complete {
			result = append(result, effect)
		}
	}
	return result
}

func phaseActions(phases []PlanPhase, kind ConsentKind) []ProjectionAction {
	var actions []ProjectionAction
	for _, phase := range phases {
		if phase.Kind == kind {
			for _, action := range phase.Actions {
				action.Args = append([]string(nil), action.Args...)
				actions = append(actions, action)
			}
		}
	}
	return actions
}
