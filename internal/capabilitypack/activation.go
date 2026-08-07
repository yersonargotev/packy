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
	OperationReconcile             Operation            = "reconcile"
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
	PackID          string
	Surface         Surface
	Aliases         []SurfaceAlias
	Selection       ResourceSelection
	ProviderChoices []ProviderChoice
}

type UpdateRequest struct {
	PackID          string
	Surface         Surface
	Aliases         []SurfaceAlias
	ProviderChoices []ProviderChoice
	Force           bool
}

type DeactivationRequest struct {
	PackID    string
	Surface   Surface
	Resources []ResourceIdentity
	Force     bool
}

type ReconcileScope string

const (
	ReconcileTargeted    ReconcileScope = "targeted"
	ReconcileSurfaceWide ReconcileScope = "surface-wide"
)

type ReconcileRequest struct {
	PackID          string
	Surface         Surface
	Aliases         []SurfaceAlias
	ProviderChoices []ProviderChoice
}

// ProviderChoice is the durable, consumer-owned selection of one provider for
// a required capability. Resource is empty for Pack-level (schema v1-v3)
// providers.
type ProviderChoice struct {
	Capability       string            `json:"capability"`
	ProviderPack     string            `json:"provider_pack"`
	ProviderResource *ResourceIdentity `json:"provider_resource,omitempty"`
}

// ExecutableResolution is the immutable fact set used to choose an external
// command. It intentionally contains no credentials or tool-owned data.
type ExecutableResolution struct {
	Tool                 string   `json:"tool"`
	Available            bool     `json:"available"`
	Path                 string   `json:"path"`
	ResolvedPath         string   `json:"resolved_path"`
	Origin               string   `json:"origin"`
	Version              string   `json:"version,omitempty"`
	AcquisitionSupported bool     `json:"acquisition_supported"`
	AcquisitionCommand   string   `json:"acquisition_command,omitempty"`
	AcquisitionArgs      []string `json:"acquisition_args,omitempty"`
	AcquisitionSource    string   `json:"acquisition_source,omitempty"`
	AcquisitionVersion   string   `json:"acquisition_version,omitempty"`
	Precondition         string   `json:"precondition"`
}

// ExecutableResolver is owned by capabilitypack; the concrete Engram
// resolver is composed by the CLI at the edge of the application.
type ExecutableResolver interface {
	Resolve(context.Context, string) (ExecutableResolution, error)
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
	Revision                   string
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

type ActivationIntent struct {
	PackID          string             `json:"pack_id"`
	Surface         Surface            `json:"surface"`
	Version         string             `json:"version"`
	Active          bool               `json:"active"`
	Revision        int                `json:"revision"`
	Aliases         []SurfaceAlias     `json:"aliases"`
	Selection       ResourceSelection  `json:"selection"`
	Resources       []ResourceIdentity `json:"resources,omitempty"`
	ProviderChoices []ProviderChoice   `json:"provider_choices,omitempty"`
	// Explicit distinguishes direct activation intent from an activation kept
	// solely to satisfy consumers. Absence is conservatively treated as direct
	// intent for state written before provider roles were persisted.
	Explicit *bool `json:"explicit,omitempty"`
}

type SurfaceAlias struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProjectionOwnership struct {
	ID                string                `json:"id"`
	ProjectionID      string                `json:"projection_id,omitempty"`
	PhysicalID        string                `json:"physical_id,omitempty"`
	Target            string                `json:"target,omitempty"`
	Contributors      []string              `json:"contributors"`
	Fingerprint       string                `json:"fingerprint"`
	AdapterProvenance string                `json:"adapter_provenance,omitempty"`
	Authorities       []ProjectionAuthority `json:"authorities,omitempty"`
}

// ProjectionAuthority records which adapter observation may authorize
// destructive work for one surface. Discoverability is intentionally absent:
// seeing a shared target never creates activation authority.
type ProjectionAuthority struct {
	Surface           Surface `json:"surface"`
	AdapterProvenance string  `json:"adapter_provenance"`
}

// DeletionAuthorized is lifecycle-owned policy: only the last contributor may
// authorize destructive cleanup of a shared projection.
func (o ProjectionOwnership) DeletionAuthorized() bool { return len(o.Contributors) == 1 }

type ApplyingJournal struct {
	PlanID            string                     `json:"plan_id"`
	PlanDigest        string                     `json:"plan_digest,omitempty"`
	Operation         Operation                  `json:"operation,omitempty"`
	Surface           Surface                    `json:"surface,omitempty"`
	PackID            string                     `json:"pack_id,omitempty"`
	Outcome           AttemptOutcome             `json:"outcome,omitempty"`
	Actions           []string                   `json:"actions"`
	Completed         []string                   `json:"completed,omitempty"`
	FailedAction      string                     `json:"failed_action,omitempty"`
	FailureDetail     string                     `json:"failure_detail,omitempty"`
	AffectedResources []RecoveryAffectedResource `json:"affected_resources,omitempty"`
	Consumers         []RecoveryConsumer         `json:"consumers,omitempty"`
	ReconcileScope    ReconcileScope             `json:"reconcile_scope,omitempty"`
}

type RecoveryAffectedResource struct {
	Pack     string           `json:"pack"`
	Resource ResourceIdentity `json:"resource"`
}

type RecoveryConsumer struct {
	Pack       string            `json:"pack"`
	Resource   *ResourceIdentity `json:"resource,omitempty"`
	Capability string            `json:"capability"`
}

type AttemptOutcome string

const (
	AttemptApplying         AttemptOutcome = "applying"
	AttemptVerified         AttemptOutcome = "verified"
	AttemptRecoveryRequired AttemptOutcome = "recovery-required"
)

type ProjectionActionError struct {
	ID  string
	Err error
}

func (e ProjectionActionError) Error() string {
	return fmt.Sprintf("apply projection %s: %v", e.ID, e.Err)
}
func (e ProjectionActionError) Unwrap() error { return e.Err }

func (j ApplyingJournal) NotStarted() []string {
	completed := map[string]bool{}
	for _, id := range j.Completed {
		completed[id] = true
	}
	result := make([]string, 0, len(j.Actions))
	for _, id := range j.Actions {
		if !completed[id] && id != j.FailedAction {
			result = append(result, id)
		}
	}
	return result
}

func (j *ApplyingJournal) recordFailure(action string, err error) {
	j.FailedAction = action
	j.Outcome = AttemptRecoveryRequired
	j.FailureDetail = err.Error()
}

func requiredFailedActionID(err error, phase string) string {
	var actionErr *ProjectionActionError
	if errors.As(err, &actionErr) && actionErr.ID != "" {
		return actionErr.ID
	}
	panic("surface adapter violated its action-specific error contract: " + phase)
}

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
	Contributors      []string                 `json:"contributors"`
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
	SchemaVersion    int                   `json:"schema_version"`
	Intent           ActivationIntent      `json:"intent"`
	Intents          []ActivationIntent    `json:"intents,omitempty"`
	Journal          *ApplyingJournal      `json:"applying_journal,omitempty"`
	LastAttempts     []ApplyingJournal     `json:"last_attempts,omitempty"`
	History          []ApplyingJournal     `json:"attempt_history,omitempty"`
	Ownership        []ProjectionOwnership `json:"ownership,omitempty"`
	External         []ExternalEffect      `json:"external_effects,omitempty"`
	documentRevision int
	snapshotManaged  bool
}

type ActivationStore interface {
	Load(context.Context, Surface) (ActivationState, error)
	Save(context.Context, Surface, int, ActivationState) error
}

// activationSnapshotStore is implemented by stores that own the complete
// cross-surface state document. The legacy surface store interface remains a
// narrow compatibility seam for embedders and test fakes.
type activationSnapshotStore interface {
	LoadSnapshot(context.Context, Surface) (ActivationState, error)
	SaveSnapshot(context.Context, Surface, int, ActivationState) (int, error)
}

func loadActivationState(ctx context.Context, store ActivationStore, surface Surface) (ActivationState, error) {
	if snapshots, ok := store.(activationSnapshotStore); ok {
		return snapshots.LoadSnapshot(ctx, surface)
	}
	return store.Load(ctx, surface)
}

func saveActivationState(ctx context.Context, store ActivationStore, surface Surface, expectedIntentRevision int, state *ActivationState) error {
	if snapshots, ok := store.(activationSnapshotStore); ok {
		revision, err := snapshots.SaveSnapshot(ctx, surface, state.documentRevision, *state)
		if err == nil {
			state.documentRevision = revision
		}
		return err
	}
	return store.Save(ctx, surface, expectedIntentRevision, *state)
}

type activationDependencies struct {
	store    ActivationStore
	adapters map[Surface]SurfaceAdapter
	resolver ExecutableResolver
	executor ExternalExecutor
}

type FacadeOption func(*Facade)

func WithActivation(store ActivationStore, adapters map[Surface]SurfaceAdapter) FacadeOption {
	return func(f *Facade) {
		var resolver ExecutableResolver
		var executor ExternalExecutor
		if f.activation != nil {
			resolver = f.activation.resolver
			executor = f.activation.executor
		}
		f.activation = &activationDependencies{store: store, adapters: adapters, resolver: resolver, executor: executor}
	}
}

func WithExternalEffects(resolver ExecutableResolver, executor ExternalExecutor) FacadeOption {
	return func(f *Facade) {
		if f.activation == nil {
			f.activation = &activationDependencies{}
		}
		f.activation.resolver = resolver
		f.activation.executor = executor
	}
}

type PlanPhase struct {
	Kind             ConsentKind
	Digest           string
	ApprovalRequired bool
	Actions          []ProjectionAction
}

type ReconciliationPlan struct {
	id                      string
	digest                  string
	pack                    Pack
	operation               Operation
	surface                 Surface
	intentRevision          int
	documentRevision        int
	oldVersion              string
	observationFingerprint  string
	phases                  []PlanPhase
	desired                 []projectionExpectation
	portable                []PortableOutcome
	resolutions             []ExecutableResolution
	runtimeModeResults      []RuntimeModeResult
	sensitiveEffects        []SensitiveEffectOrigin
	readiness               ReadinessStatus
	readinessObserved       ReadinessObservationStatus
	observedEvidence        []string
	pendingEvidence         []string
	pendingHumanActions     []string
	noOp                    bool
	activations             []PlannedActivation
	contributors            map[string][]string
	retained                []RetainedProjection
	sharedProjections       []SharedProjectionVisibility
	blockers                []PlanBlocker
	compositionFacts        []Pack
	intentFacts             []ActivationIntent
	beforeIntentFacts       []ActivationIntent
	ownershipFacts          []ProjectionOwnership
	activeDependents        []ActiveDependent
	capabilityFacts         []CapabilityRequirementFact
	beforeCompositionFacts  []Pack
	removedContributors     map[string]string
	reconcileScope          ReconcileScope
	aliases                 []SurfaceAlias
	previousAliases         []SurfaceAlias
	selection               ResourceSelection
	previousSelection       ResourceSelection
	partialSelection        bool
	selectionValidity       SelectionValidity
	providerChoices         []ProviderChoice
	previousProviderChoices []ProviderChoice
	rootMigrations          []RootMigration
	allModeContractChanges  []string
	recovery                bool
	force                   bool
	historicalAttempt       *ApplyingJournal
}

type RetainedProjection struct {
	ID           string
	Contributors []string
}

type SharedProjectionVisibility struct {
	ID              string    `json:"id"`
	ProjectionKey   string    `json:"projection_key"`
	DiscoverableBy  []Surface `json:"discoverable_by"`
	DiscoveryNotice string    `json:"discovery_notice"`
}

func (p *ReconciliationPlan) recordSharedProjection(projection ObservedProjection) {
	if !projection.Shared && !projection.Action.Shared {
		return
	}
	discoverable := append([]Surface(nil), projection.DiscoverableBy...)
	if len(discoverable) == 0 {
		discoverable = append(discoverable, projection.Action.DiscoverableBy...)
	}
	p.sharedProjections = append(p.sharedProjections, SharedProjectionVisibility{ID: projection.ID, ProjectionKey: projectionOwnershipID(projection), DiscoverableBy: discoverable, DiscoveryNotice: sharedProjectionDiscoveryNotice})
}

type projectionExpectation struct {
	ID, Fingerprint   string
	ExternallyManaged bool
}
type PortableOutcome struct{ Kind, ID string }

func (p ReconciliationPlan) ID() string                     { return p.id }
func (p ReconciliationPlan) Digest() string                 { return p.digest }
func (p ReconciliationPlan) Pack() Pack                     { return clonePack(p.pack) }
func (p ReconciliationPlan) Surface() Surface               { return p.surface }
func (p ReconciliationPlan) Operation() Operation           { return p.operation }
func (p ReconciliationPlan) ReconcileScope() ReconcileScope { return p.reconcileScope }
func (p ReconciliationPlan) Aliases() []SurfaceAlias        { return cloneAliases(p.aliases) }
func (p ReconciliationPlan) Selection() ResourceSelection   { return cloneSelection(p.selection) }
func (p ReconciliationPlan) ProviderChoices() []ProviderChoice {
	return cloneProviderChoices(p.providerChoices)
}
func (p ReconciliationPlan) RootMigrations() []RootMigration {
	return append([]RootMigration(nil), p.rootMigrations...)
}
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
func (p ReconciliationPlan) CapabilityRequirements() []CapabilityRequirementFact {
	result := make([]CapabilityRequirementFact, len(p.capabilityFacts))
	copy(result, p.capabilityFacts)
	for i := range result {
		result[i].RequiredTools = append([]string(nil), result[i].RequiredTools...)
		result[i].RequiredAuthority = append([]string(nil), result[i].RequiredAuthority...)
		if result[i].ConsumerResource != nil {
			resource := *result[i].ConsumerResource
			result[i].ConsumerResource = &resource
		}
		if result[i].ProviderResource != nil {
			resource := *result[i].ProviderResource
			result[i].ProviderResource = &resource
		}
	}
	return result
}
func (p ReconciliationPlan) Blockers() []PlanBlocker {
	return append([]PlanBlocker(nil), p.blockers...)
}
func (p ReconciliationPlan) RetainedProjections() []RetainedProjection {
	result := append([]RetainedProjection(nil), p.retained...)
	prefix := "surface:" + string(p.surface) + ":"
	for i := range result {
		result[i].Contributors = append([]string(nil), result[i].Contributors...)
		for j := range result[i].Contributors {
			result[i].Contributors[j] = strings.TrimPrefix(result[i].Contributors[j], prefix)
		}
	}
	return result
}
func (p ReconciliationPlan) Contributors() map[string][]string {
	result := make(map[string][]string, len(p.contributors))
	for id, contributors := range p.contributors {
		result[id] = append([]string(nil), contributors...)
	}
	return result
}
func (p ReconciliationPlan) RemovedContributors() map[string]string {
	result := make(map[string]string, len(p.removedContributors))
	for id, contributor := range p.removedContributors {
		result[id] = contributor
	}
	return result
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
func (p ReconciliationPlan) ReadinessObserved() ReadinessObservationStatus {
	return p.readinessObserved
}
func (p ReconciliationPlan) Evidence() []string { return append([]string(nil), p.observedEvidence...) }
func (p ReconciliationPlan) PendingEvidence() []string {
	return append([]string(nil), p.pendingEvidence...)
}
func (p ReconciliationPlan) Recovery() bool { return p.recovery }
func (p ReconciliationPlan) HistoricalAttempt() *ApplyingJournal {
	if p.historicalAttempt == nil {
		return nil
	}
	copy := cloneJournal(*p.historicalAttempt)
	return &copy
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
	ReadinessObserved   ReadinessObservationStatus
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
	activation := ActivationRequest{PackID: request.PackID, Surface: request.Surface, Aliases: request.Aliases, ProviderChoices: request.ProviderChoices}
	_, _, state, err := f.activationInputsForOperation(ctx, activation, OperationUpdate)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	if f.catalog.withdrawn(request.PackID) && !recoveryAttempt(state, OperationUpdate, request.PackID, request.Surface) {
		return ReconciliationPlan{}, fmt.Errorf("capability pack %q is withdrawn and cannot be updated", request.PackID)
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
	activation.Selection, activation.Aliases, err = migrateUpdateIntent(current, intent)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	if request.Aliases != nil {
		activation.Aliases = request.Aliases
	}
	if request.ProviderChoices == nil {
		activation.ProviderChoices = cloneProviderChoices(intent.ProviderChoices)
	}
	allModeChanges := []string{}
	if activation.Selection.Mode == SelectionAll && hasTrustedHistoricalArtifact(request.PackID, intent.Version) {
		previous, resolveErr := f.catalog.resolveIntentPack(request.PackID, intent.Version)
		if resolveErr != nil {
			return ReconciliationPlan{}, resolveErr
		}
		allModeChanges, err = allModeOperationalContractChanges(previous, current, request.Surface)
		if err != nil {
			return ReconciliationPlan{}, err
		}
	}
	plan, err := f.preview(ctx, activation, OperationUpdate, intent.Version, request.Force)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	plan.rootMigrations = appliedRootMigrations(current, intent.Selection, intent.Aliases)
	plan.allModeContractChanges = allModeChanges
	if (len(plan.rootMigrations) > 0 || len(plan.allModeContractChanges) > 0) && !planHasApprovalPhase(plan, ConsentReversibleLocal) {
		plan.phases = append([]PlanPhase{{Kind: ConsentReversibleLocal, ApprovalRequired: true, Actions: []ProjectionAction{}}}, plan.phases...)
	}
	plan.seal()
	return plan, nil
}

func planHasApprovalPhase(plan ReconciliationPlan, kind ConsentKind) bool {
	for _, phase := range plan.phases {
		if phase.Kind == kind && phase.ApprovalRequired {
			return true
		}
	}
	return false
}

func migrateUpdateIntent(target Pack, intent ActivationIntent) (ResourceSelection, []SurfaceAlias, error) {
	selection, err := canonicalSelection(intent.Selection)
	if err != nil {
		return ResourceSelection{}, nil, err
	}
	aliases := cloneAliases(intent.Aliases)
	if target.manifestVersion != manifestSchemaV4 {
		if selection.Mode != SelectionCustom {
			return selection, aliases, nil
		}
		return ResourceSelection{}, nil, fmt.Errorf("custom resource selection update requires target manifest schema_version 4")
	}
	if target.RootMigrations != nil {
		if err := validateRootMigrations(target); err != nil {
			return ResourceSelection{}, nil, fmt.Errorf("invalid target root migrations: %w", err)
		}
	}
	resources := make(map[string]Resource, len(target.Resources))
	for _, resource := range target.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	migrations := make(map[string]ResourceIdentity, len(target.RootMigrations))
	for _, migration := range target.RootMigrations {
		migrations[migration.From.String()] = migration.To
	}
	for i := range aliases {
		if targetAlias, exists := migrations[(ResourceIdentity{Kind: aliases[i].Kind, ID: aliases[i].ID}).String()]; exists {
			aliases[i].Kind, aliases[i].ID = targetAlias.Kind, targetAlias.ID
		}
	}
	if selection.Mode != SelectionCustom {
		if err := canonicalizeAliases(&aliases); err != nil {
			return ResourceSelection{}, nil, err
		}
		return selection, aliases, nil
	}
	rewritten := make(map[string]ResourceIdentity, len(selection.Roots))
	for _, root := range selection.Roots {
		if resource, exists := resources[root.String()]; exists && resource.Kind != "asset" && resource.Kind != "notice" {
			rewritten[root.String()] = root
			continue
		}
		targetRoot, exists := migrations[root.String()]
		if !exists {
			return ResourceSelection{}, nil, fmt.Errorf("selected canonical root %q is unavailable in target pack %q version %s and has no valid root migration", root.String(), target.ID, target.Version)
		}
		if _, duplicate := rewritten[targetRoot.String()]; duplicate {
			return ResourceSelection{}, nil, fmt.Errorf("root migration to %q makes the selected roots ambiguous", targetRoot.String())
		}
		rewritten[targetRoot.String()] = targetRoot
	}
	roots := make([]ResourceIdentity, 0, len(rewritten))
	for _, root := range rewritten {
		roots = append(roots, root)
	}
	next, err := canonicalSelection(ResourceSelection{Mode: SelectionCustom, Roots: roots})
	if err != nil {
		return ResourceSelection{}, nil, err
	}
	if err := canonicalizeAliases(&aliases); err != nil {
		return ResourceSelection{}, nil, err
	}
	return next, aliases, nil
}

func appliedRootMigrations(target Pack, before ResourceSelection, aliases []SurfaceAlias) []RootMigration {
	before, beforeErr := canonicalSelection(before)
	if beforeErr != nil {
		return []RootMigration{}
	}
	resources := make(map[string]bool, len(target.Resources))
	for _, resource := range target.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = true
	}
	declared := make(map[string]RootMigration, len(target.RootMigrations))
	for _, migration := range target.RootMigrations {
		declared[migration.From.String()] = migration
	}
	applied := map[string]bool{}
	if before.Mode == SelectionCustom {
		for _, from := range before.Roots {
			if resources[from.String()] {
				continue
			}
			applied[from.String()] = true
		}
	}
	for _, alias := range aliases {
		from := (ResourceIdentity{Kind: alias.Kind, ID: alias.ID}).String()
		if _, exists := declared[from]; exists {
			applied[from] = true
		}
	}
	result := []RootMigration{}
	for _, migration := range target.RootMigrations {
		if applied[migration.From.String()] {
			result = append(result, migration)
		}
	}
	return result
}

func allModeOperationalContractChanges(before, target Pack, surface Surface) ([]string, error) {
	selection := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
	before, err := selectPackResourcesForSurface(before, selection, surface)
	if err != nil {
		return nil, fmt.Errorf("resolve previous all-mode contract: %w", err)
	}
	target, err = selectPackResourcesForSurface(target, selection, surface)
	if err != nil {
		return nil, fmt.Errorf("resolve target all-mode contract: %w", err)
	}
	operational := func(pack Pack) map[string]bool {
		result := map[string]bool{}
		for _, resource := range pack.Resources {
			if resource.Kind != "asset" && resource.Kind != "notice" {
				result[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = true
			}
		}
		return result
	}
	previous, next := operational(before), operational(target)
	changes := []string{}
	for identity := range next {
		if !previous[identity] {
			changes = append(changes, "all selection adds operational resource "+identity)
		}
	}
	for identity := range previous {
		if !next[identity] {
			changes = append(changes, "all selection removes operational resource "+identity)
		}
	}
	sort.Strings(changes)
	return changes, nil
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
	recovery := recoveryAttempt(state, OperationDeactivate, request.PackID, request.Surface)
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
	target, dependents, err := f.composeWithout(requested, state, request.Surface)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	combined := target.combinedPack()
	resolutions, err := f.resolveExecutables(ctx, before.combinedPack())
	if err != nil {
		return ReconciliationPlan{}, err
	}
	observation, err := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Surface, OperationDeactivate, before.combinedPack(), combined, state.Ownership, resolutions))
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("inspect deactivation of pack %q on %s: %w", requested.ID, request.Surface, err)
	}
	targetCollisionBlockers := distinctResourceTargetCollisions(observation.Projections)
	previousProviderChoices := []ProviderChoice{}
	if active {
		previousProviderChoices = cloneProviderChoices(intent.ProviderChoices)
	}
	plan := ReconciliationPlan{pack: currentRequested, operation: OperationDeactivate, surface: request.Surface, intentRevision: state.Intent.Revision, documentRevision: state.documentRevision, oldVersion: oldVersion, previousAliases: cloneAliases(intent.Aliases), selection: selection, previousSelection: selection, previousProviderChoices: previousProviderChoices, observationFingerprint: observationDigest(observation), resolutions: resolutions, runtimeModeResults: cloneRuntimeModeResults(observation.RuntimeModeResults), activations: target.activations, contributors: target.contributors, compositionFacts: target.packs, beforeCompositionFacts: before.packs, intentFacts: target.intentFacts, beforeIntentFacts: before.intentFacts, ownershipFacts: cloneOwnership(state.Ownership), activeDependents: dependents, removedContributors: removedContributorSet(before, target), force: request.Force}
	for _, dependent := range dependents {
		plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerActiveDependent, Subject: requested.ID, Detail: fmt.Sprintf("cannot deactivate requested pack %s: active pack %s still requires capability/dependency %s; no automatic cascade will occur", requested.ID, dependent.PackID, dependent.Dependency)})
	}
	plan.blockers = append(plan.blockers, target.blockers...)
	plan.blockers = append(plan.blockers, targetCollisionBlockers...)
	sortBlockers(plan.blockers)
	for _, resource := range combined.Resources {
		plan.portable = append(plan.portable, PortableOutcome{Kind: resource.Kind, ID: resource.ID})
	}
	for _, projection := range observation.Projections {
		plan.recordSharedProjection(projection)
		contributors := target.contributorSet(projection.ID)
		if state.snapshotManaged {
			if owner, ok := ownershipByID(state.Ownership, ownershipIDForState(state, request.Surface, projection)); ok {
				contributors = mergedProjectionContributors(owner, request.Surface, contributors)
			}
		}
		if projection.DesiredFingerprint != "" {
			plan.desired = append(plan.desired, projectionExpectation{ID: projection.ID, Fingerprint: projection.DesiredFingerprint, ExternallyManaged: projection.ExternallyManaged})
			if projection.Exists && projection.ObservedFingerprint == projection.DesiredFingerprint {
				plan.retained = append(plan.retained, RetainedProjection{ID: projection.ID, Contributors: contributors})
			} else {
				detail := fmt.Sprintf("preserved shared projection %s because it is missing, drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
				plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
				plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
			}
			continue
		}
		if receipt, authorized := receiptForExternalProjection(state.External, request.Surface, projection, observation.Projections, completedJournalActions(state)); authorized {
			if projection.Exists && !externalReceiptHasRemainingContributor(receipt, target.packs) {
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
		removedContributor, removed := uniqueRemovedContributor(projection.ID, before, target)
		if projection.Exists && owned && ownershipHasOtherSurfaceContributor(owner, request.Surface) {
			plan.retained = append(plan.retained, RetainedProjection{ID: projection.ID, Contributors: append([]string(nil), owner.Contributors...)})
			continue
		}
		residual := active && !intent.Active && hasContributor(state.Ownership, requested.ID)
		residualLifecycle := residual || recovery && !intent.Active
		observedProvenance := projectionProvenance(request.Surface, projection)
		authority, hasAuthority := authorityForSurface(owner, request.Surface)
		residualAuthorized := residualLifecycle && hasAuthority && authority.AdapterProvenance != "" && authority.AdapterProvenance == observedProvenance
		activeLifecycle := active && intent.Active || recovery && intent.Active
		if activeLifecycle && owned && state.snapshotManaged {
			authority, hasAuthority = authorityForSurface(owner, request.Surface)
			residualAuthorized = hasAuthority && authority.AdapterProvenance != "" && authority.AdapterProvenance == observedProvenance
		} else if activeLifecycle && owned {
			residualAuthorized = true
		}
		if (activeLifecycle || residualAuthorized) && residualAuthorized && projection.Exists && owned && len(owner.Contributors) == 1 && removed && removedContributorMatches(state, request.Surface, owner.Contributors[0], removedContributor) && receiptPermitsRemoval(owner, projection, request.Force) {
			plan.phases = appendPhaseAction(plan.phases, ConsentDestructiveCleanup, projection.Action)
			continue
		}
		if projection.Exists {
			detail := fmt.Sprintf("preserved %s because it is drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
			plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
			plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
		}
	}
	if (!active || !intent.Active) && !recovery {
		plan.noOp = len(plan.phases) == 0 && len(plan.pendingHumanActions) == 0 && !hasContributor(state.Ownership, requested.ID)
		if len(plan.phases) == 0 && !plan.noOp {
			plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: requested.ID, Detail: fmt.Sprintf("inactive pack %s has partial, drifted, or residual state; preserved it without starting general reconcile", requested.ID)})
		}
	}
	sortBlockers(plan.blockers)
	if len(plan.blockers) > 0 {
		plan.noOp = false
	}
	if len(targetCollisionBlockers) > 0 {
		plan.phases = nil
	}
	sort.Slice(plan.retained, func(i, j int) bool { return plan.retained[i].ID < plan.retained[j].ID })
	sort.Strings(plan.pendingHumanActions)
	plan.attachRecovery(state, recovery)
	plan.requireRecoveryApproval()
	plan.captureSensitiveEffects()
	plan.seal()
	return plan, nil
}

func hasContributor(values []ProjectionOwnership, packID string) bool {
	for _, value := range values {
		for _, contributor := range value.Contributors {
			if contributorBelongsToPack(contributor, packID) {
				return true
			}
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
	resolutions, err := f.resolveExecutables(ctx, combinedBefore)
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
		runtimeModeResults: cloneRuntimeModeResults(observation.RuntimeModeResults), activations: target.activations, contributors: target.contributors,
		compositionFacts: target.packs, beforeCompositionFacts: before.packs, intentFacts: target.intentFacts, beforeIntentFacts: before.intentFacts,
		ownershipFacts: cloneOwnership(state.Ownership), removedContributors: removedContributorSet(before, target),
	}
	if persisted, ok := intentForPack(state, request.PackID, request.Surface); ok {
		plan.providerChoices = cloneProviderChoices(persisted.ProviderChoices)
		plan.previousProviderChoices = cloneProviderChoices(persisted.ProviderChoices)
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
		plan.recordSharedProjection(projection)
		contributors := append([]string(nil), target.contributors[projection.ID]...)
		if state.snapshotManaged {
			if owner, ok := ownershipByID(state.Ownership, ownershipIDForState(state, request.Surface, projection)); ok {
				contributors = mergedProjectionContributors(owner, request.Surface, contributors)
			}
		}
		if projection.DesiredFingerprint != "" {
			plan.desired = append(plan.desired, projectionExpectation{ID: projection.ID, Fingerprint: projection.DesiredFingerprint, ExternallyManaged: projection.ExternallyManaged})
			if projection.Exists && projection.ObservedFingerprint == projection.DesiredFingerprint {
				plan.retained = append(plan.retained, RetainedProjection{ID: projection.ID, Contributors: contributors})
			} else {
				detail := fmt.Sprintf("preserved shared projection %s because it is missing, drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
				plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
				plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
			}
			continue
		}
		if receipt, authorized := receiptForExternalProjection(state.External, request.Surface, projection, observation.Projections, completedJournalActions(state)); authorized {
			if projection.Exists && !externalReceiptHasRemainingContributor(receipt, target.packs) {
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
		removedContributor, removed := uniqueRemovedContributor(projection.ID, before, target)
		if projection.Exists && owned && ownershipHasOtherSurfaceContributor(owner, request.Surface) {
			plan.retained = append(plan.retained, RetainedProjection{ID: projection.ID, Contributors: append([]string(nil), owner.Contributors...)})
			continue
		}
		authority, authorized := authorityForSurface(owner, request.Surface)
		observedProvenance := projectionProvenance(request.Surface, projection)
		ownershipAuthorized := !state.snapshotManaged || authorized && authority.AdapterProvenance == observedProvenance
		if projection.Exists && owned && ownershipAuthorized && len(owner.Contributors) == 1 && removed && removedContributorMatches(state, request.Surface, owner.Contributors[0], removedContributor) && receiptPermitsRemoval(owner, projection, request.Force) {
			plan.phases = appendPhaseAction(plan.phases, ConsentDestructiveCleanup, projection.Action)
			continue
		}
		if projection.Exists {
			detail := fmt.Sprintf("preserved %s because it is drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
			plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
			plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
		}
	}
	readiness := ReadinessStatus{Configured: len(plan.blockers) == 0}
	readiness.Authorized = readiness.Configured && observation.Readiness.AuthorizationObserved && observation.Readiness.Authorized
	readiness.Usable = readiness.Authorized && observation.Readiness.UsabilityObserved && observation.Readiness.Usable
	plan.readiness = readiness
	plan.readinessObserved = ReadinessObservationStatus{Configured: true, Authorization: observation.Readiness.AuthorizationObserved, Usability: observation.Readiness.UsabilityObserved}
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
	sort.Slice(plan.retained, func(i, j int) bool { return plan.retained[i].ID < plan.retained[j].ID })
	if len(plan.blockers) > 0 {
		plan.noOp = false
	}
	if len(targetCollisionBlockers) > 0 {
		plan.phases = nil
	}
	plan.attachRecovery(state, recoveryAttempt(state, OperationDeactivate, request.PackID, request.Surface))
	plan.requireRecoveryApproval()
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
	previousProviderChoices := []ProviderChoice{}
	if intent, ok := intentForPack(state, requested.ID, request.Surface); ok {
		previousAliases = cloneAliases(intent.Aliases)
		previousProviderChoices = cloneProviderChoices(intent.ProviderChoices)
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
	providerChoices, err := canonicalProviderChoices(request.ProviderChoices)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	if request.ProviderChoices == nil {
		providerChoices = cloneProviderChoices(previousProviderChoices)
	}
	if operation == OperationUpdate && digestJSON(providerChoices) != digestJSON(previousProviderChoices) {
		return ReconciliationPlan{}, fmt.Errorf("update preserves the persisted provider choice; changing it requires an explicit lifecycle transition")
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
	state = stateWithProviderChoices(state, requested.ID, request.Surface, providerChoices)
	useRequestedIntent := operation == OperationReconcile
	composition, err := f.compose(requested, state, request.Surface, useRequestedIntent)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	requiredCapabilities := map[string]bool{}
	for _, requirement := range capabilityRequirements(requested) {
		requiredCapabilities[requirement.capability] = true
	}
	staleChoiceBlockers := []PlanBlocker{}
	for _, choice := range providerChoices {
		if !requiredCapabilities[choice.Capability] {
			staleChoiceBlockers = append(staleChoiceBlockers, PlanBlocker{
				Kind: BlockerDependency, Subject: choice.Capability,
				Detail: fmt.Sprintf("persisted provider choice is invalid for consumer pack %q; approve a provider migration or replacement choice", requested.ID),
			})
		}
	}
	if request.ProviderChoices == nil && len(previousProviderChoices) == 0 {
		for _, fact := range composition.capabilityFacts {
			if fact.ConsumerPack != requested.ID {
				continue
			}
			providerChoices = append(providerChoices, ProviderChoice{Capability: fact.Capability, ProviderPack: fact.ProviderPack, ProviderResource: fact.ProviderResource})
		}
		providerChoices, err = canonicalProviderChoices(providerChoices)
		if err != nil {
			return ReconciliationPlan{}, err
		}
		state = stateWithProviderChoices(state, requested.ID, request.Surface, providerChoices)
		composition, err = f.compose(requested, state, request.Surface, useRequestedIntent)
		if err != nil {
			return ReconciliationPlan{}, err
		}
	}
	composition.blockers = append(composition.blockers, selectionValidityBlockers...)
	composition.blockers = append(composition.blockers, staleChoiceBlockers...)
	sortBlockers(composition.blockers)
	var beforeCompositionFacts []Pack
	var beforeIntentFacts []ActivationIntent
	var previousPack Pack
	var ownedBeforeUpdate func(ObservedProjection, string) bool
	if operation == OperationUpdate && hasTrustedHistoricalArtifact(requested.ID, oldVersion) {
		before, err := f.compose(requested, state, request.Surface, true)
		if err != nil {
			return ReconciliationPlan{}, err
		}
		beforeCompositionFacts = before.packs
		beforeIntentFacts = before.intentFacts
		previousPack = before.combinedPack()
		ownedBeforeUpdate = func(projection ObservedProjection, fingerprint string) bool {
			return ownedAtComposition(state.Ownership, projection, fingerprint, before)
		}
	}
	pack := composition.combinedPack()
	resolutions, err := f.resolveExecutables(ctx, pack)
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
			managedDrift := operation == OperationReconcile && projection.Exists && repairEligible(state.Ownership, projection, composition)
			if force && projection.Exists && forceRepairEligible(state.Ownership, projection, composition) {
				managedDrift = true
			}
			if operation == OperationReconcile && removal {
				owner, ok := ownershipByID(state.Ownership, ownershipIDForState(state, request.Surface, projection))
				owned = ok && owner.Fingerprint == projection.ObservedFingerprint
			}
			if projection.Exists && !owned && !managedDrift {
				composition.blockers = append(composition.blockers, PlanBlocker{BlockerOwnership, projection.ID, fmt.Sprintf("projection is unmanaged or drifted; preserving existing %s content", request.Surface)})
				continue
			}
			if managedDrift {
				projection.Action.Description = "restore drifted Packy-managed projection " + projection.ID + " to intent-selected content: " + projection.Action.Description
			}
			if (operation == OperationReconcile || operation == OperationUpdate) && removal {
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
	noOp := compositionActive(state, composition.packs, request.Surface) && ownershipMatchesContributors(state.Ownership, observation.Projections, composition) && len(actions) == 0 && len(externalActions) == 0
	if current, ok := intentForPack(state, request.PackID, request.Surface); ok && digestJSON(current.Aliases) != digestJSON(aliases) {
		noOp = false
	}
	if digestJSON(previousSelection) != digestJSON(selection) {
		noOp = false
	}
	if digestJSON(previousProviderChoices) != digestJSON(providerChoices) {
		noOp = false
	}
	if operation == OperationActivate {
		if current, ok := intentForPack(state, request.PackID, request.Surface); ok && current.Active && !intentIsExplicit(current) {
			noOp = false
		}
	}
	readiness := ReadinessStatus{Configured: operation != OperationDeactivate && len(composition.blockers) == 0}
	readiness.Authorized = readiness.Configured && observation.Readiness.AuthorizationObserved && observation.Readiness.Authorized
	readiness.Usable = readiness.Authorized && observation.Readiness.UsabilityObserved && observation.Readiness.Usable
	readinessObserved := ReadinessObservationStatus{Configured: true, Authorization: observation.Readiness.AuthorizationObserved, Usability: observation.Readiness.UsabilityObserved}
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
	capabilityFacts := append([]CapabilityRequirementFact(nil), composition.capabilityFacts...)
	for i := range capabilityFacts {
		capabilityFacts[i].ResultingReadiness = readiness
	}
	plan := ReconciliationPlan{pack: requested, operation: operation, surface: request.Surface, intentRevision: state.Intent.Revision, documentRevision: state.documentRevision, oldVersion: oldVersion, aliases: cloneAliases(aliases), previousAliases: previousAliases, selection: selection, previousSelection: previousSelection, providerChoices: providerChoices, previousProviderChoices: previousProviderChoices, selectionValidity: selectionValidity, observationFingerprint: observationDigest(observation), resolutions: resolutions, runtimeModeResults: cloneRuntimeModeResults(observation.RuntimeModeResults), readiness: readiness, readinessObserved: readinessObserved, observedEvidence: observedEvidence, pendingEvidence: pendingEvidence, pendingHumanActions: pendingHumanActions, noOp: noOp, activations: composition.activations, contributors: composition.contributors, blockers: composition.blockers, compositionFacts: composition.packs, intentFacts: composition.intentFacts, beforeIntentFacts: beforeIntentFacts, ownershipFacts: cloneOwnership(state.Ownership), beforeCompositionFacts: beforeCompositionFacts, capabilityFacts: capabilityFacts, force: force}
	recovery := recoveryAttempt(state, operation, request.PackID, request.Surface)
	plan.attachRecovery(state, recovery)
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
		plan.recordSharedProjection(projection)
		plan.desired = append(plan.desired, projectionExpectation{projection.ID, projection.DesiredFingerprint, projection.ExternallyManaged})
		contributors := composition.contributorSet(projection.ID)
		if projection.ObservedFingerprint == projection.DesiredFingerprint && len(contributors) > 1 {
			plan.retained = append(plan.retained, RetainedProjection{ID: projection.ID, Contributors: contributors})
		}
	}
	sort.Slice(plan.desired, func(i, j int) bool { return plan.desired[i].ID < plan.desired[j].ID })
	sort.Slice(plan.retained, func(i, j int) bool { return plan.retained[i].ID < plan.retained[j].ID })
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
	plan.requireRecoveryApproval()
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

	actions := flattenActions(request.Plan.phases)
	state.SchemaVersion = 3
	if request.Plan.operation != OperationReconcile && !request.Plan.recovery {
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
		state.Intent = ActivationIntent{PackID: pack.ID, Surface: request.Plan.surface, Version: targetVersion, Active: activeTarget, Revision: state.Intent.Revision + 1, Aliases: cloneAliases(request.Plan.aliases), Selection: request.Plan.selection, Resources: packResourceIdentities(pack), ProviderChoices: cloneProviderChoices(request.Plan.providerChoices), Explicit: &explicit}
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
			providerChoices := cloneProviderChoices(activation.ProviderChoices)
			explicitFact := &explicitIntent
			if previouslyActive {
				explicitFact = previous.Explicit
			}
			byID[activation.Pack.ID] = ActivationIntent{PackID: activation.Pack.ID, Surface: request.Plan.surface, Version: activation.Pack.Version, Active: true, Revision: state.Intent.Revision, Aliases: cloneAliases(aliases), Selection: activationSelection, Resources: packResourceIdentities(activation.Pack), ProviderChoices: providerChoices, Explicit: explicitFact}
			if activation.Pack.ID == pack.ID {
				byID[activation.Pack.ID] = state.Intent
			}
		}
		if request.Plan.operation == OperationDeactivate {
			byID[pack.ID] = state.Intent
			for id, candidate := range byID {
				if id == pack.ID || !candidate.Active || intentIsExplicit(candidate) || providerHasConsumer(byID, id) {
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
	if request.Plan.recovery && state.Journal != nil {
		state.History = append(state.History, cloneJournal(*request.Plan.historicalAttempt))
	}
	affectedResources, consumers := request.Plan.recoverySubjects()
	state.Journal = &ApplyingJournal{
		PlanID: request.Plan.id, PlanDigest: request.Plan.digest, Operation: request.Plan.operation,
		Surface: request.Plan.surface, PackID: request.Plan.pack.ID, Outcome: AttemptApplying,
		AffectedResources: affectedResources, Consumers: consumers, ReconcileScope: request.Plan.reconcileScope,
	}
	if request.Plan.recovery && request.Plan.historicalAttempt != nil {
		state.Journal.Actions = append([]string(nil), request.Plan.historicalAttempt.Actions...)
		state.Journal.Completed = append([]string(nil), request.Plan.historicalAttempt.Completed...)
	}
	for _, action := range actions {
		if action.Kind != ActionHostFollowUp {
			state.Journal.Actions = appendCompleted(state.Journal.Actions, action.ID)
		}
	}
	if err := saveActivationState(ctx, f.activation.store, request.Plan.surface, request.Plan.intentRevision, &state); err != nil {
		return ApplyResult{}, err
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
			safeErr := ReportSafeError(err, &request.Plan)
			state.Journal.recordFailure(requiredFailedActionID(err, "reversible-local"), safeErr)
			if saveErr := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("apply reversible local projections: %v; could not persist recovery facts: %w", safeErr, saveErr)
			}
			return ApplyResult{}, safeErr
		}
	}
	destructiveActions := phaseActions(request.Plan.phases, ConsentDestructiveCleanup)
	prior := priorCombinedPack(request.Plan, pack)
	verified, err := inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Plan.surface, request.Plan.operation, prior, combined, state.Ownership, resolutions))
	if err != nil {
		err = ReportSafeError(err, &request.Plan)
		state.Journal.recordFailure("verify-reversible-local", err)
		if saveErr := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); saveErr != nil {
			return ApplyResult{}, fmt.Errorf("verify reversible local projections: %v; could not persist recovery facts: %w", err, saveErr)
		}
		return ApplyResult{}, err
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
	if request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted {
		verifiedMatches = verificationMatchesSubset(verificationDesired, verified.Projections)
	}
	state.External = refreshExternalReceiptContributors(state.External, currentComposition.packs, request.Plan.surface)
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
		state.Journal.recordFailure("verify-reversible-local", errors.New(verificationMismatch(request.Plan.desired, verified.Projections)))
		if saveErr := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); saveErr != nil {
			return ApplyResult{}, fmt.Errorf("%w: %s; could not persist recovery facts: %v", ErrVerificationFailed, state.Journal.FailureDetail, saveErr)
		}
		return ApplyResult{}, fmt.Errorf("%w: %s", ErrVerificationFailed, verificationMismatch(request.Plan.desired, verified.Projections))
	}
	beforeExternal := cloneSurfaceInspection(verified)
	for _, action := range localActions {
		state.Journal.Completed = appendCompleted(state.Journal.Completed, action.ID)
	}
	if len(externalActions) > 0 {
		if err := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); err != nil {
			return ApplyResult{}, fmt.Errorf("persist verified local recovery facts: %w", err)
		}
	}
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
			state.Journal.recordFailure(action.ID, actionErr)
			if saveErr := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("external action %s failed: %v; could not persist recovery facts: %w", action.ID, actionErr, saveErr)
			}
			return ApplyResult{}, fmt.Errorf("external action %s failed; later actions stopped and recovery is required: %w", action.ID, actionErr)
		}
		state.Journal.Completed = append(state.Journal.Completed, action.ID)
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
		if err := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); err != nil {
			return ApplyResult{}, fmt.Errorf("external action %s completed but recovery facts could not be persisted: %w", action.ID, err)
		}
	}
	for _, action := range destructiveActions {
		if err := adapter.ApplyProjections(ctx, []ProjectionAction{action}); err != nil {
			safeErr := ReportSafeError(err, &request.Plan)
			state.Journal.recordFailure(requiredFailedActionID(err, "destructive-cleanup"), safeErr)
			_ = saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state)
			return ApplyResult{}, safeErr
		}
		state.Journal.Completed = appendCompleted(state.Journal.Completed, action.ID)
		if err := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); err != nil {
			return ApplyResult{}, fmt.Errorf("destructive action %s completed but recovery facts could not be persisted: %w", action.ID, err)
		}
	}
	if len(externalActions) > 0 || len(destructiveActions) > 0 {
		verified, err = inspectSurface(ctx, adapter, surfaceTransitionFacts(request.Plan.surface, request.Plan.operation, prior, combined, state.Ownership, resolutions))
		if err != nil {
			err = ReportSafeError(err, &request.Plan)
			state.Journal.recordFailure("verify-after-external", err)
			if saveErr := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("verify after external effects: %v; could not persist recovery facts: %w", err, saveErr)
			}
			return ApplyResult{}, err
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
		if request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted {
			matches = verificationMatchesSubset(request.Plan.desired, verified.Projections)
		}
		if request.Plan.operation == OperationUpdate && len(destructiveActions) > 0 {
			matches = verificationMatchesAfterCleanup(request.Plan.desired, verified.Projections, destructiveActions)
		}
		if request.Plan.operation == OperationDeactivate {
			matches = verificationMatchesDeactivation(request.Plan.desired, verificationProjections)
		}
		if !matches {
			state.Journal.recordFailure("verify-after-external", errors.New(verificationMismatch(request.Plan.desired, verified.Projections)))
			if saveErr := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("%w: %s; could not persist recovery facts: %v", ErrVerificationFailed, state.Journal.FailureDetail, saveErr)
			}
			return ApplyResult{}, fmt.Errorf("%w: %s", ErrVerificationFailed, verificationMismatch(request.Plan.desired, verified.Projections))
		}
	}
	if request.Plan.operation == OperationDeactivate && len(destructiveActions) > 0 {
		state.External = retireExternalReceipts(state.External, state.Journal.Completed)
	}
	verifiedAttempt := cloneJournal(*state.Journal)
	verifiedAttempt.Outcome = AttemptVerified
	verifiedAttempt.AffectedResources = nil
	verifiedAttempt.Consumers = nil
	verifiedAttempt.ReconcileScope = ""
	state.LastAttempts = recordLatestAttempt(state.LastAttempts, verifiedAttempt)
	state.Journal = nil
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
	if request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted {
		desiredIDs := map[string]bool{}
		for _, expectation := range request.Plan.desired {
			desiredIDs[expectation.ID] = true
		}
		for _, owner := range previousOwnership {
			if !desiredIDs[owner.ID] {
				if _, exists := ownershipByID(state.Ownership, owner.ID); !exists {
					state.Ownership = append(state.Ownership, owner)
				}
			}
		}
	}
	for _, projection := range verified.Projections {
		if projection.ExternallyManaged || hasPhaseActionID(request.Plan.phases, ConsentDestructiveCleanup, projection.ID) || (request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted && !hasExpectation(request.Plan.desired, projection.ID)) {
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
		provenance := ""
		previous, previouslyOwned := ownershipByID(previousOwnership, ownershipIDForState(state, request.Plan.surface, projection))
		if previouslyOwned {
			provenance = previous.AdapterProvenance
		}
		if action, ok := phaseActionByID(request.Plan.phases, projection.ID); ok {
			if action.AdapterProvenance != "" {
				provenance = action.AdapterProvenance
			} else if action.Consent == ConsentExecutableExternal && action.Source != "" {
				provenance = action.Source
			} else if projection.AdapterProvenance != "" {
				provenance = projection.AdapterProvenance
			}
		}
		if state.snapshotManaged {
			provenance = projectionProvenance(request.Plan.surface, projection)
		}
		owner := ProjectionOwnership{ID: ownershipIDForState(state, request.Plan.surface, projection), ProjectionID: projection.ID, Target: projection.Action.Target, Fingerprint: projection.DesiredFingerprint, AdapterProvenance: provenance}
		if previouslyOwned {
			owner = previous
			owner.Fingerprint = projection.DesiredFingerprint
			owner.Target = projection.Action.Target
		}
		if state.snapshotManaged {
			owner.Contributors = mergedProjectionContributors(owner, request.Plan.surface, currentComposition.contributorSet(projection.ID))
			owner = withSurfaceAuthority(owner, request.Plan.surface, provenance)
		} else {
			owner.Contributors = currentComposition.contributorSet(projection.ID)
		}
		state.Ownership = append(state.Ownership, owner)
	}
	sort.Slice(state.Ownership, func(i, j int) bool { return state.Ownership[i].ID < state.Ownership[j].ID })
	if err := saveActivationState(ctx, f.activation.store, request.Plan.surface, state.Intent.Revision, &state); err != nil {
		return ApplyResult{}, err
	}
	fresh := verified.Readiness
	readiness := ReadinessStatus{Configured: true, Authorized: fresh.AuthorizationObserved && fresh.Authorized}
	readiness.Usable = readiness.Authorized && fresh.UsabilityObserved && fresh.Usable
	pendingHumanActions := append([]string(nil), fresh.PendingHumanActions...)
	if len(pendingHumanActions) == 0 {
		pendingHumanActions = append(pendingHumanActions, verified.PendingHumanActions...)
	}
	return ApplyResult{Verified: true, PlanID: request.Plan.id, Projections: len(state.Ownership), Readiness: readiness, ReadinessObserved: ReadinessObservationStatus{Configured: true, Authorization: fresh.AuthorizationObserved, Usability: fresh.UsabilityObserved}, PendingHumanActions: pendingHumanActions}, nil
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

func hasExpectation(values []projectionExpectation, id string) bool {
	for _, value := range values {
		if value.ID == id {
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
	currentChoices := []ProviderChoice{}
	if intent, ok := intentForPack(state, plan.pack.ID, plan.surface); ok {
		currentChoices, err = canonicalProviderChoices(intent.ProviderChoices)
	}
	if err != nil || digestJSON(currentChoices) != digestJSON(plan.previousProviderChoices) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("provider choice changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
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
	if plan.operation == OperationReconcile && state.Intent.Revision != plan.intentRevision {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d; rerun %s to preview a fresh plan", plan.intentRevision, state.Intent.Revision, plan.operation)}
	}
	if plan.recovery {
		currentHistory := normalizedRecoveryJournal(state.Journal)
		if currentHistory == nil || digestJSON(currentHistory) != digestJSON(plan.historicalAttempt) {
			return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("recovery attempt history changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
		}
	}
	if plan.operation != OperationDeactivate {
		state = stateWithSelection(stateWithAliases(state, plan.pack.ID, plan.surface, plan.pack.Version, plan.aliases), plan.pack.ID, plan.surface, plan.pack.Version, plan.selection)
		state = stateWithProviderChoices(state, plan.pack.ID, plan.surface, plan.providerChoices)
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
		useRequestedIntent := plan.operation == OperationReconcile || plan.operation == OperationDeactivate
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
			target, dependents, targetErr := f.composeWithout(pack, state, plan.surface)
			if targetErr != nil {
				return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("historical deactivation comparison changed after Preview: %v; rerun deactivate to preview a fresh plan", targetErr)}
			}
			if digestJSON(before.packs) != digestJSON(plan.beforeCompositionFacts) || digestJSON(dependents) != digestJSON(plan.activeDependents) {
				return planPreflight{}, StalePlanError{Precondition: "dependency closure or active dependents changed after Preview; rerun deactivate to preview a fresh plan"}
			}
			current = target
		}
	}
	if plan.operation == OperationReconcile && digestJSON(current.intentFacts) != digestJSON(plan.intentFacts) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("active intents or intent revisions changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	planned := composition{packs: plan.compositionFacts, activations: plan.activations, contributors: plan.contributors, blockers: plan.blockers, intentFacts: plan.intentFacts}
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
	resolutions, err := f.resolveExecutables(ctx, resolutionPack)
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

func appendCompleted(completed []string, id string) []string {
	for _, existing := range completed {
		if existing == id {
			return completed
		}
	}
	return append(completed, id)
}

func (f Facade) activationInputs(ctx context.Context, request ActivationRequest) (Pack, SurfaceAdapter, ActivationState, error) {
	return f.activationInputsForOperation(ctx, request, OperationActivate)
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
	activationRecovery := operation == OperationActivate && recoveryAttempt(state, OperationActivate, request.PackID, request.Surface)
	if operation == OperationActivate && hasIntent && intent.Active && intent.Version != pack.Version && !activationRecovery {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("capability pack %q is active at %s on %s; use explicit pack update to target catalog current %s", request.PackID, intent.Version, request.Surface, pack.Version)
	}
	if operation == OperationActivate && f.catalog.withdrawn(request.PackID) && (!hasIntent || !intent.Active) {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("capability pack %q is withdrawn and cannot be freshly activated", request.PackID)
	}
	historicalActivationRecovery := activationRecovery && intent.Version != pack.Version
	historicalManagement := (operation == OperationReconcile || operation == OperationDeactivate) && hasIntent && intent.Active
	usesHistory := (historicalManagement || historicalActivationRecovery) && hasTrustedHistoricalArtifact(intent.PackID, intent.Version)
	if historicalActivationRecovery && !usesHistory {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("capability pack %q recovery cannot resolve exact intended version %s", request.PackID, intent.Version)
	}
	if usesHistory {
		pack, err = f.catalog.resolveIntentPack(request.PackID, intent.Version)
		if err != nil {
			return Pack{}, nil, ActivationState{}, err
		}
	}
	if !usesHistory {
		pack, err = f.catalog.Show(request.PackID)
		if err != nil {
			return Pack{}, nil, ActivationState{}, err
		}
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

func recoveryAttempt(state ActivationState, operation Operation, packID string, surface Surface) bool {
	journal := state.Journal
	if journal == nil || (journal.Outcome != AttemptRecoveryRequired && journal.Outcome != AttemptApplying) || journal.Operation != operation || journal.PackID != packID || journal.Surface != surface {
		return false
	}
	intent, ok := intentForPack(state, packID, surface)
	switch operation {
	case OperationActivate, OperationUpdate, OperationReconcile:
		return ok && intent.Active
	case OperationDeactivate:
		return !ok || !intent.Active
	default:
		return false
	}
}

func (p *ReconciliationPlan) attachRecovery(state ActivationState, recovery bool) {
	if !recovery || state.Journal == nil {
		return
	}
	p.recovery = true
	p.historicalAttempt = normalizedRecoveryJournal(state.Journal)
}

func (p ReconciliationPlan) recoverySubjects() ([]RecoveryAffectedResource, []RecoveryConsumer) {
	resources := map[string]RecoveryAffectedResource{}
	for _, phase := range p.phases {
		for _, action := range phase.Actions {
			if action.Kind == ActionHostFollowUp {
				continue
			}
			for _, contributor := range p.actionContributors(action.ID) {
				parts := strings.SplitN(strings.TrimPrefix(contributor, "pack:"), ":", 3)
				if len(parts) != 3 {
					continue
				}
				resource, err := ParseResourceIdentity(parts[1] + ":" + parts[2])
				if err != nil {
					continue
				}
				value := RecoveryAffectedResource{Pack: parts[0], Resource: resource}
				resources[value.Pack+"/"+value.Resource.String()] = value
			}
		}
	}
	affected := make([]RecoveryAffectedResource, 0, len(resources))
	for _, value := range resources {
		affected = append(affected, value)
	}
	sort.Slice(affected, func(i, j int) bool {
		if affected[i].Pack != affected[j].Pack {
			return affected[i].Pack < affected[j].Pack
		}
		return affected[i].Resource.String() < affected[j].Resource.String()
	})

	consumerSet := map[string]RecoveryConsumer{}
	for _, fact := range p.capabilityFacts {
		resourceKey := ""
		if fact.ConsumerResource != nil {
			resourceKey = fact.ConsumerResource.String()
		}
		key := fact.ConsumerPack + "/" + resourceKey + "/" + fact.Capability
		consumer := RecoveryConsumer{Pack: fact.ConsumerPack, Capability: fact.Capability}
		if fact.ConsumerResource != nil {
			resource := *fact.ConsumerResource
			consumer.Resource = &resource
		}
		consumerSet[key] = consumer
	}
	consumers := make([]RecoveryConsumer, 0, len(consumerSet))
	for _, value := range consumerSet {
		consumers = append(consumers, value)
	}
	sort.Slice(consumers, func(i, j int) bool {
		if consumers[i].Pack != consumers[j].Pack {
			return consumers[i].Pack < consumers[j].Pack
		}
		left, right := "", ""
		if consumers[i].Resource != nil {
			left = consumers[i].Resource.String()
		}
		if consumers[j].Resource != nil {
			right = consumers[j].Resource.String()
		}
		if left != right {
			return left < right
		}
		return consumers[i].Capability < consumers[j].Capability
	})
	return affected, consumers
}

func (p ReconciliationPlan) nextLifecycleCommand() string {
	return lifecycleCommand(p.operation, p.pack.ID, p.surface, p.reconcileScope)
}

func lifecycleCommand(operation Operation, packID string, surface Surface, reconcileScope ReconcileScope) string {
	if operation == OperationReconcile && reconcileScope == ReconcileSurfaceWide {
		return fmt.Sprintf("packy pack reconcile --surface %s", surface)
	}
	return fmt.Sprintf("packy pack %s %s --surface %s", operation, packID, surface)
}

func normalizedRecoveryJournal(value *ApplyingJournal) *ApplyingJournal {
	if value == nil {
		return nil
	}
	journal := cloneJournal(*value)
	if journal.Outcome == AttemptApplying {
		journal.Outcome = AttemptRecoveryRequired
		if journal.FailedAction == "" {
			journal.FailedAction = "interrupted"
		}
		if journal.FailureDetail == "" {
			journal.FailureDetail = "attempt was interrupted before a terminal outcome was durably recorded"
		}
	}
	return &journal
}

func (p *ReconciliationPlan) requireRecoveryApproval() {
	if !p.recovery || len(p.blockers) > 0 {
		return
	}
	p.noOp = false
	for _, phase := range p.phases {
		if phase.ApprovalRequired {
			return
		}
	}
	p.phases = append([]PlanPhase{{Kind: ConsentReversibleLocal, ApprovalRequired: true}}, p.phases...)
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
		PackID, Version         string
		Operation               Operation
		Surface                 Surface
		IntentRevision          int
		DocumentRevision        int
		OldVersion              string
		Observation             string
		Phases                  []PlanPhase
		Desired                 []projectionExpectation
		Portable                []PortableOutcome
		Resolutions             []ExecutableResolution
		RuntimeModes            []RuntimeModeResult
		SensitiveEffects        []SensitiveEffectOrigin
		Readiness               ReadinessStatus
		Pending                 []string
		NoOp                    bool
		Activations             []PlannedActivation
		Contributors            map[string][]string
		Retained                []RetainedProjection
		SharedProjections       []SharedProjectionVisibility
		Blockers                []PlanBlocker
		Composition             []Pack
		IntentFacts             []ActivationIntent
		BeforeIntentFacts       []ActivationIntent
		OwnershipFacts          []ProjectionOwnership
		Dependents              []ActiveDependent
		CapabilityFacts         []CapabilityRequirementFact
		Before                  []Pack
		Removed                 map[string]string
		ReconcileScope          ReconcileScope
		Aliases                 []SurfaceAlias
		PreviousAliases         []SurfaceAlias
		Selection               ResourceSelection
		PreviousSelection       ResourceSelection
		ProviderChoices         []ProviderChoice
		PreviousProviderChoices []ProviderChoice
		RootMigrations          []RootMigration
		AllModeContractChanges  []string
		PartialSelection        bool
		SelectionValidity       SelectionValidity
		Recovery                bool
		Force                   bool
		Historical              *ApplyingJournal
	}{p.pack.ID, p.pack.Version, p.operation, p.surface, p.intentRevision, p.documentRevision, p.oldVersion, p.observationFingerprint, p.phases, p.desired, p.portable, p.resolutions, p.runtimeModeResults, p.sensitiveEffects, p.readiness, p.pendingHumanActions, p.noOp, p.activations, p.contributors, p.retained, p.sharedProjections, p.blockers, p.compositionFacts, p.intentFacts, p.beforeIntentFacts, p.ownershipFacts, p.activeDependents, p.capabilityFacts, p.beforeCompositionFacts, p.removedContributors, p.reconcileScope, p.aliases, p.previousAliases, p.selection, p.previousSelection, p.providerChoices, p.previousProviderChoices, p.rootMigrations, p.allModeContractChanges, p.partialSelection, p.selectionValidity, p.recovery, p.force, p.historicalAttempt}
}

func providerHasConsumer(intents map[string]ActivationIntent, providerID string) bool {
	for _, intent := range intents {
		if !intent.Active {
			continue
		}
		for _, choice := range intent.ProviderChoices {
			if choice.ProviderPack == providerID {
				return true
			}
		}
	}
	return false
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

func projectionProvenance(surface Surface, projection ObservedProjection) string {
	if projection.AdapterProvenance != "" {
		return projection.AdapterProvenance
	}
	if projection.Action.AdapterProvenance != "" {
		return projection.Action.AdapterProvenance
	}
	// Older adapters did not expose a typed provenance. Keep their authority
	// surface-scoped; a future explicit provenance will no longer match this
	// conservative compatibility value.
	return "surface:" + string(surface) + "/unspecified-adapter"
}

func authorityForSurface(owner ProjectionOwnership, surface Surface) (ProjectionAuthority, bool) {
	for _, authority := range owner.Authorities {
		if authority.Surface == surface {
			return authority, true
		}
	}
	if len(owner.Authorities) == 0 && owner.AdapterProvenance != "" {
		return ProjectionAuthority{Surface: surface, AdapterProvenance: owner.AdapterProvenance}, true
	}
	return ProjectionAuthority{}, false
}

func withSurfaceAuthority(owner ProjectionOwnership, surface Surface, provenance string) ProjectionOwnership {
	var authorities []ProjectionAuthority
	for _, authority := range owner.Authorities {
		if authority.Surface != surface {
			authorities = append(authorities, authority)
		}
	}
	if provenance != "" {
		authorities = append(authorities, ProjectionAuthority{Surface: surface, AdapterProvenance: provenance})
	}
	sort.Slice(authorities, func(i, j int) bool { return authorities[i].Surface < authorities[j].Surface })
	owner.Authorities = authorities
	return owner
}

func removedContributorMatches(state ActivationState, surface Surface, recorded, removed string) bool {
	if !state.snapshotManaged {
		return recorded == removed
	}
	return recorded == qualifyContributor(surface, removed)
}

func phaseActionByID(phases []PlanPhase, id string) (ProjectionAction, bool) {
	for _, phase := range phases {
		for _, action := range phase.Actions {
			if action.ID == id {
				return action, true
			}
		}
	}
	return ProjectionAction{}, false
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
	for i := range result {
		result[i].Contributors = append([]string(nil), result[i].Contributors...)
		result[i].Authorities = append([]ProjectionAuthority(nil), result[i].Authorities...)
	}
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

func ownershipMatchesContributors(owners []ProjectionOwnership, projections []ObservedProjection, c composition) bool {
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
		if !ok || owner.Fingerprint != projection.DesiredFingerprint || !contributorsMatchForSurface(owner.Contributors, c.surface, c.contributorSet(projection.ID)) {
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
	// Preserve the pre-SurfaceAdapter plan fingerprint payload. Goal makes
	// destructive intent explicit and readiness now travels with inspection,
	// but neither changes the host revision/projection facts that made an
	// existing plan stale before this refactor.
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
		Revision                        string
		Projections                     []fingerprintProjection
		RuntimeModes                    []runtimeModeFingerprint `json:",omitempty"`
		Readiness                       ReadinessStatus
		PendingHumanActions             []string
		LegacyEmptyProjectionDigestSlot []fingerprintProjection `json:"RemovalCandidates"`
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
func flattenActions(phases []PlanPhase) []ProjectionAction {
	var actions []ProjectionAction
	for _, phase := range phases {
		actions = append(actions, phase.Actions...)
	}
	return actions
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
func ownershipMatches(owners []ProjectionOwnership, projections []ObservedProjection, packID string) bool {
	if len(owners) != len(projections) {
		return false
	}
	byID := map[string]ProjectionOwnership{}
	for _, owner := range owners {
		byID[owner.ID] = owner
	}
	for _, projection := range projections {
		owner, ok := byID[projection.ID]
		if !ok || owner.Fingerprint != projection.DesiredFingerprint || len(owner.Contributors) != 1 || owner.Contributors[0] != packID {
			return false
		}
	}
	return true
}
func ownedAtFingerprint(owners []ProjectionOwnership, id, fingerprint, packID string) bool {
	for _, owner := range owners {
		if owner.ID == id && owner.Fingerprint == fingerprint && len(owner.Contributors) == 1 && owner.Contributors[0] == packID {
			return true
		}
	}
	return false
}
func ownedAtComposition(owners []ProjectionOwnership, projection ObservedProjection, fingerprint string, c composition) bool {
	for _, owner := range owners {
		identityMatches := owner.ID == physicalProjectionID(c.surface, projection) || owner.ID == projectionOwnershipID(projection)
		if identityMatches && owner.Fingerprint == fingerprint && contributorsMatchForSurface(owner.Contributors, c.surface, c.contributorSet(projection.ID)) {
			return true
		}
	}
	return false
}

func repairEligible(owners []ProjectionOwnership, projection ObservedProjection, c composition) bool {
	if projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent {
		return false
	}
	var matched []ProjectionOwnership
	for _, owner := range owners {
		if owner.ID == physicalProjectionID(c.surface, projection) || owner.ID == projectionOwnershipID(projection) {
			matched = append(matched, owner)
		}
	}
	if len(matched) != 1 {
		return false
	}
	owner := matched[0]
	return owner.Fingerprint == projection.DesiredFingerprint && contributorsMatchForSurface(owner.Contributors, c.surface, c.contributorSet(projection.ID))
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
	return ok && owner.Target != "" && filepath.Clean(owner.Target) == filepath.Clean(projection.Action.Target) && contributorsMatchForSurface(owner.Contributors, c.surface, c.contributorSet(projection.ID))
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

func ownershipHasOtherSurfaceContributor(owner ProjectionOwnership, surface Surface) bool {
	for _, contributor := range owner.Contributors {
		contributorSurfaceValue, qualified := contributorSurface(contributor)
		if qualified && contributorSurfaceValue != surface {
			return true
		}
	}
	return false
}
func cloneActivationState(state ActivationState) ActivationState {
	state.Intent.Aliases = cloneAliases(state.Intent.Aliases)
	state.Intent.Selection = cloneSelection(state.Intent.Selection)
	state.Intent.Resources = append([]ResourceIdentity(nil), state.Intent.Resources...)
	state.Intent.ProviderChoices = cloneProviderChoices(state.Intent.ProviderChoices)
	state.Ownership = append([]ProjectionOwnership(nil), state.Ownership...)
	state.Intents = append([]ActivationIntent(nil), state.Intents...)
	for i := range state.Intents {
		state.Intents[i].Aliases = cloneAliases(state.Intents[i].Aliases)
		state.Intents[i].Selection = cloneSelection(state.Intents[i].Selection)
		state.Intents[i].Resources = append([]ResourceIdentity(nil), state.Intents[i].Resources...)
		state.Intents[i].ProviderChoices = cloneProviderChoices(state.Intents[i].ProviderChoices)
	}
	for i := range state.Ownership {
		state.Ownership[i].Contributors = append([]string(nil), state.Ownership[i].Contributors...)
		state.Ownership[i].Authorities = append([]ProjectionAuthority(nil), state.Ownership[i].Authorities...)
	}
	if state.Journal != nil {
		journal := cloneJournal(*state.Journal)
		state.Journal = &journal
	}
	state.LastAttempts = append([]ApplyingJournal(nil), state.LastAttempts...)
	for i := range state.LastAttempts {
		state.LastAttempts[i] = cloneJournal(state.LastAttempts[i])
	}
	state.History = append([]ApplyingJournal(nil), state.History...)
	for i := range state.History {
		state.History[i] = cloneJournal(state.History[i])
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

func stateWithProviderChoices(state ActivationState, packID string, surface Surface, choices []ProviderChoice) ActivationState {
	choices = cloneProviderChoices(choices)
	for i := range state.Intents {
		if state.Intents[i].PackID == packID && state.Intents[i].Surface == surface {
			state.Intents[i].ProviderChoices = cloneProviderChoices(choices)
		}
	}
	if state.Intent.PackID == packID && state.Intent.Surface == surface {
		state.Intent.ProviderChoices = cloneProviderChoices(choices)
	}
	return state
}

func cloneProviderChoices(values []ProviderChoice) []ProviderChoice {
	if values == nil {
		return nil
	}
	result := append([]ProviderChoice{}, values...)
	for i := range result {
		if result[i].ProviderResource != nil {
			resource := *result[i].ProviderResource
			result[i].ProviderResource = &resource
		}
	}
	return result
}

func canonicalProviderChoices(values []ProviderChoice) ([]ProviderChoice, error) {
	if values == nil {
		return nil, nil
	}
	result := cloneProviderChoices(values)
	sort.Slice(result, func(i, j int) bool { return result[i].Capability < result[j].Capability })
	for i, choice := range result {
		if choice.Capability == "" || strings.TrimSpace(choice.Capability) != choice.Capability || !idPattern.MatchString(choice.ProviderPack) {
			return nil, fmt.Errorf("provider choice requires canonical capability and provider pack identities")
		}
		if i > 0 && result[i-1].Capability == choice.Capability {
			return nil, fmt.Errorf("duplicate provider choice for capability %q", choice.Capability)
		}
		if choice.ProviderResource != nil {
			if _, err := ParseResourceIdentity(choice.ProviderResource.String()); err != nil {
				return nil, fmt.Errorf("provider choice for capability %q has an invalid provider resource: %w", choice.Capability, err)
			}
		}
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func cloneJournal(journal ApplyingJournal) ApplyingJournal {
	journal.Actions = append([]string(nil), journal.Actions...)
	journal.Completed = append([]string(nil), journal.Completed...)
	journal.AffectedResources = append([]RecoveryAffectedResource(nil), journal.AffectedResources...)
	journal.Consumers = append([]RecoveryConsumer(nil), journal.Consumers...)
	for i := range journal.Consumers {
		if journal.Consumers[i].Resource != nil {
			resource := *journal.Consumers[i].Resource
			journal.Consumers[i].Resource = &resource
		}
	}
	return journal
}

func recordLatestAttempt(attempts []ApplyingJournal, attempt ApplyingJournal) []ApplyingJournal {
	result := append([]ApplyingJournal(nil), attempts...)
	for i := range result {
		if result[i].PackID == attempt.PackID && result[i].Surface == attempt.Surface {
			result[i] = cloneJournal(attempt)
			return result
		}
	}
	return append(result, cloneJournal(attempt))
}

func (f Facade) externalPlan(operation Operation, pack Pack, surface Surface, state ActivationState, resolutions []ExecutableResolution) ([]ProjectionAction, []PlanBlocker) {
	var actions []ProjectionAction
	var blockers []PlanBlocker
	for _, resolution := range resolutions {
		if !resolution.Available {
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
		if externalEffectCompleted(state.External, setup) && !externalActionNeedsRetry(state, setup, surface) && !externalVerificationNeedsRetry(state, setup, surface) {
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

func externalVerificationNeedsRetry(state ActivationState, setup ProjectionAction, surface Surface) bool {
	if state.Journal == nil || state.Journal.Outcome != AttemptRecoveryRequired || state.Journal.FailedAction != "verify-after-external" || !slices.Contains(state.Journal.Completed, setup.ID) {
		return false
	}
	return state.Journal.Surface == surface
}

func externalActionNeedsRetry(state ActivationState, setup ProjectionAction, surface Surface) bool {
	return state.Journal != nil && state.Journal.Outcome == AttemptRecoveryRequired && state.Journal.FailedAction == setup.ID && state.Journal.Surface == surface
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
	case OperationReconcile:
		transition.ResidualOwnership = adapterOwnership
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
		value.ProjectInstallation = &installation
	}
	return value
}

func cloneSurfaceInspection(value SurfaceInspection) SurfaceInspection {
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

func (f Facade) resolveExecutables(ctx context.Context, pack Pack) ([]ExecutableResolution, error) {
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
	}{resolution.Tool, resolution.Path, resolution.ResolvedPath, resolution.Origin, resolution.Version, "", resolution.Available, resolution.AcquisitionSupported, resolution.AcquisitionCommand, resolution.AcquisitionArgs})
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
			Contributors: externalReceiptContributors(packs, tool, surface), Contributions: contributions,
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

func externalReceiptContributors(packs []Pack, tool string, surface Surface) []string {
	var result []string
	for _, pack := range packs {
		if slices.Contains(pack.Requires.Tools, tool) {
			result = append(result, qualifyContributor(surface, "pack:"+pack.ID+":external:"+tool))
		}
	}
	return sortedUnique(result)
}

func cloneExternalEffects(values []ExternalEffect) []ExternalEffect {
	result := append([]ExternalEffect(nil), values...)
	for i := range result {
		if result[i].Receipt == nil {
			continue
		}
		receipt := *result[i].Receipt
		receipt.Contributors = append([]string(nil), receipt.Contributors...)
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
			receipt.EffectID != effect.ID || receipt.EffectFingerprint != effect.Fingerprint || receipt.Surface != surface || len(receipt.Contributors) == 0 {
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

func externalReceiptHasRemainingContributor(receipt *ExternalEffectReceipt, packs []Pack) bool {
	if receipt == nil {
		return false
	}
	current := externalReceiptContributors(packs, externalSetupTool(receipt.EffectID), receipt.Surface)
	for _, contributor := range current {
		if slices.Contains(receipt.Contributors, contributor) {
			return true
		}
	}
	return false
}

func externalReceiptReversalAction(action ProjectionAction, receipt *ExternalEffectReceipt) ProjectionAction {
	action.Consent = ConsentDestructiveCleanup
	action.Consequences = "removes only the exact external configuration contribution sealed by receipt " + receipt.EffectID
	action.RollbackLimits = strings.Join(receipt.Reversal.AuthorityLimits, "; ")
	return action
}

func refreshExternalReceiptContributors(effects []ExternalEffect, packs []Pack, surface Surface) []ExternalEffect {
	result := cloneExternalEffects(effects)
	for i := range result {
		if result[i].Receipt == nil || result[i].Receipt.Surface != surface {
			continue
		}
		contributors := externalReceiptContributors(packs, externalSetupTool(result[i].Receipt.EffectID), surface)
		if len(contributors) > 0 {
			result[i].Receipt.Contributors = contributors
		}
	}
	return result
}

func completedJournalActions(state ActivationState) []string {
	if state.Journal == nil {
		return nil
	}
	return state.Journal.Completed
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

func hasPhaseActions(phases []PlanPhase, kind ConsentKind) bool {
	return len(phaseActions(phases, kind)) > 0
}
