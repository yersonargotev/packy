package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const ProjectActivationPreviewSchemaVersion = 2

// ProjectActivationCategory separates consent for each class of sensitive
// project runtime behavior. Its values are portable lock identities.
type ProjectActivationCategory string

const (
	ProjectActivationMCP                  ProjectActivationCategory = "mcp"
	ProjectActivationHooks                ProjectActivationCategory = "hooks"
	ProjectActivationPlugins              ProjectActivationCategory = "plugins"
	ProjectActivationExternalRequirements ProjectActivationCategory = "external-requirements"
	ProjectActivationTrust                ProjectActivationCategory = "trust"
	ProjectActivationAuthentication       ProjectActivationCategory = "authentication"
)

type ProjectActivationDisposition string

const (
	ProjectActivationPreviewable     ProjectActivationDisposition = "previewable"
	ProjectActivationNotRequired     ProjectActivationDisposition = "not-required"
	ProjectActivationConverged       ProjectActivationDisposition = "converged"
	ProjectActivationInheritedGlobal ProjectActivationDisposition = "inherited-global"
	ProjectActivationBlocked         ProjectActivationDisposition = "blocked"
)

// ProjectSensitiveDisclosure is carried in the portable lock. Detail is a
// declared identity, never an observed credential, command argument, or host
// path.
type ProjectSensitiveDisclosure struct {
	Category ProjectActivationCategory `json:"category"`
	Surface  Surface                   `json:"surface"`
	Resource ResourceIdentity          `json:"resource"`
	Detail   string                    `json:"detail"`
}

type ProjectActivationCategoryPreview struct {
	Kind             ProjectActivationCategory    `json:"kind"`
	Details          []ProjectSensitiveDisclosure `json:"details"`
	ApprovalRequired bool                         `json:"approval_required"`
}

type ProjectActivationEffectPreview struct {
	Category          ProjectActivationCategory `json:"category"`
	Action            ProjectionActionKind      `json:"action"`
	Target            string                    `json:"target"`
	Identity          string                    `json:"identity"`
	Observation       string                    `json:"observation"`
	AdapterProvenance string                    `json:"adapter_provenance,omitempty"`
	Consent           ConsentKind               `json:"consent,omitempty"`
}

type ProjectActivationRequest struct {
	PackID      string
	Surface     Surface
	ProjectRoot string
	PackyHome   string
	Adapter     SurfaceAdapter
}

type JSONProjectActivationPreview struct {
	SchemaVersion         int                                `json:"schema_version"`
	Report                string                             `json:"report"`
	DryRun                bool                               `json:"dry_run"`
	ProjectRoot           string                             `json:"project_root"`
	Pack                  ProjectManifestPack                `json:"pack"`
	Surface               Surface                            `json:"surface"`
	RuntimeRequired       bool                               `json:"runtime_required"`
	Disposition           ProjectActivationDisposition       `json:"disposition"`
	Categories            []ProjectActivationCategoryPreview `json:"categories"`
	Effects               []ProjectActivationEffectPreview   `json:"effects"`
	SensitiveLockIdentity string                             `json:"sensitive_lock_identity"`
	RuntimeEffects        []ProjectRuntimeEffectStatus       `json:"runtime_effects"`
	Digest                string                             `json:"digest"`
	projectRoot           string
	packyHome             string
	request               ProjectActivationRequest
	actions               []ProjectionAction
}

type ProjectActivationApproval struct {
	Category ProjectActivationCategory `json:"category"`
	Digest   string                    `json:"digest"`
}

type ProjectActivationApplyRequest struct {
	Preview     JSONProjectActivationPreview
	Approvals   []ProjectActivationApproval
	Adapter     SurfaceAdapter
	Interactive bool
}

type ProjectActivationApplyResult struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Status        string `json:"status"`
	Digest        string `json:"digest"`
}

const projectActivationDocumentSchemaVersion = 1

type projectActivationState struct {
	SchemaVersion         int     `json:"schema_version"`
	PackID                string  `json:"pack_id"`
	Version               string  `json:"version"`
	Surface               Surface `json:"surface"`
	ProjectRootDigest     string  `json:"project_root_digest"`
	SensitiveLockIdentity string  `json:"sensitive_lock_identity"`
	Active                bool    `json:"active"`
}

type projectActivationReceipt struct {
	Category ProjectActivationCategory    `json:"category"`
	Digest   string                       `json:"digest"`
	Details  []ProjectSensitiveDisclosure `json:"details"`
}

type ProjectActivationEffectReceipt struct {
	Action               ProjectionActionKind `json:"action"`
	Surface              Surface              `json:"surface"`
	Target               string               `json:"target"`
	ContributionIdentity string               `json:"contribution_identity"`
	AdapterProvenance    string               `json:"adapter_provenance"`
	StartMarker          string               `json:"start_marker"`
	EndMarker            string               `json:"end_marker"`
	PriorState           string               `json:"prior_state"`
}

type projectActivationDocument struct {
	SchemaVersion         int                              `json:"schema_version"`
	State                 projectActivationState           `json:"state"`
	Approvals             []ProjectActivationApproval      `json:"approvals"`
	Receipts              []projectActivationReceipt       `json:"receipts"`
	Effects               []ProjectActivationEffectReceipt `json:"effects"`
	SensitiveLockIdentity string                           `json:"sensitive_lock_identity"`
}

func (f Facade) PreviewProjectActivation(ctx context.Context, request ProjectActivationRequest) (JSONProjectActivationPreview, error) {
	report := JSONProjectActivationPreview{SchemaVersion: ProjectActivationPreviewSchemaVersion, Report: "project-activation-preview", DryRun: true, ProjectRoot: "<project-root>", Categories: []ProjectActivationCategoryPreview{}, Effects: []ProjectActivationEffectPreview{}, RuntimeEffects: []ProjectRuntimeEffectStatus{}, projectRoot: request.ProjectRoot, packyHome: request.PackyHome, request: request}
	if request.Surface != SurfaceCodex && request.Surface != SurfaceOpenCode && request.Surface != SurfaceClaude {
		return report, fmt.Errorf("project activation preview does not support CLI surface %q", request.Surface)
	}
	if request.ProjectRoot == "" || request.PackyHome == "" || request.PackID == "" || request.Adapter == nil {
		return report, errors.New("project activation preview requires the project root, Packy Home, pack, and surface adapter")
	}
	manifestMissing, err := projectPathMissing(filepath.Join(request.ProjectRoot, "packy.json"))
	if err != nil {
		return report, err
	}
	lockMissing, err := projectPathMissing(filepath.Join(request.ProjectRoot, "packy.lock.json"))
	if err != nil {
		return report, err
	}
	if manifestMissing || lockMissing {
		return report, errors.New("project activation requires a valid project installation")
	}
	installation, err := LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return report, err
	}
	pack, installed := findProjectManifestPack(installation.Manifest.Packs, request.PackID)
	if !installed || !projectSupportsSurface(pack.Surfaces, request.Surface) {
		return report, fmt.Errorf("capability pack %q is not installed for CLI surface %q", request.PackID, request.Surface)
	}
	status, err := f.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: request.ProjectRoot, PackID: request.PackID, Surface: request.Surface, PackyHome: request.PackyHome, RequireInstalled: true, Adapters: map[Surface]SurfaceAdapter{request.Surface: request.Adapter}})
	if err != nil {
		return report, err
	}
	if len(status.Packs) != 1 || !status.Packs[0].RequirementSatisfied {
		return report, errors.New("project activation requires a healthy installed project projection")
	}
	document, exists, err := loadProjectActivationDocumentForSurface(request.PackyHome, request.ProjectRoot, request.PackID, request.Surface)
	if err != nil {
		return report, err
	}
	scopedInstallation := projectInstallationForPack(installation, pack.ID)
	observation, err := inspectSurface(ctx, request.Adapter, SurfaceTransition{ProjectRoot: request.ProjectRoot, ProjectInstallation: &scopedInstallation, ProjectGoal: ProjectionPresent})
	if err != nil {
		return report, err
	}
	report.actions = append([]ProjectionAction(nil), observation.ProjectActivationActions...)
	for _, action := range report.actions {
		if effect, ok := projectActivationEffectPreview(action); ok {
			report.Effects = append(report.Effects, effect)
		}
	}
	categories := projectActivationCategories(scopedInstallation.Lock, request.Surface)
	report.Pack, report.Surface, report.RuntimeEffects = pack, request.Surface, status.Packs[0].RuntimeEffects
	if status.Packs[0].Runtime == ProjectRuntimeBlocked {
		report.Categories, report.RuntimeRequired, report.Disposition = categories, len(categories) != 0, ProjectActivationBlocked
		report.SensitiveLockIdentity = projectSensitiveLockIdentity(scopedInstallation.Lock, categories)
		report.Digest = sealProjectActivationPreview(report)
		return report, nil
	}
	report.Categories = projectRuntimePendingCategories(categories, report.RuntimeEffects)
	report.RuntimeRequired = len(categories) != 0
	report.SensitiveLockIdentity = projectSensitiveLockIdentity(scopedInstallation.Lock, categories)
	if !report.RuntimeRequired {
		report.Disposition, report.Digest = ProjectActivationNotRequired, sealProjectActivationPreview(report)
		return report, nil
	}
	if status.Packs[0].Runtime == ProjectRuntimeInheritedGlobal {
		report.Disposition, report.Digest = ProjectActivationInheritedGlobal, sealProjectActivationPreview(report)
		return report, nil
	}
	state := document.State
	if exists && projectActivationEffectsConverged(request.Surface, observation) && status.Packs[0].Runtime == ProjectRuntimeActive && state.Active && state.PackID == pack.ID && state.Version == pack.Version && state.Surface == request.Surface && state.SensitiveLockIdentity == report.SensitiveLockIdentity {
		report.Disposition = ProjectActivationConverged
	} else {
		report.Disposition = ProjectActivationPreviewable
	}
	report.Digest = sealProjectActivationPreview(report)
	return report, nil
}

// ProjectPackRequiresActivation reports whether one installed Pack receipt
// carries personal runtime effects for the selected surface.
func ProjectPackRequiresActivation(lock ProjectLockProposal, packID string, surface Surface) bool {
	if hydrated, err := hydrateProjectLock(lock); err == nil {
		lock = hydrated
	}
	return len(projectActivationCategories(projectLockForPack(lock, packID), surface)) > 0
}

func (f Facade) ApproveProjectActivation(preview JSONProjectActivationPreview, category ProjectActivationCategory) ProjectActivationApproval {
	return ProjectActivationApproval{Category: category, Digest: preview.Digest}
}

func projectActivationEffectPreview(action ProjectionAction) (ProjectActivationEffectPreview, bool) {
	effect := ProjectActivationEffectPreview{Action: action.Kind, Identity: action.Version, Observation: projectActivationActionObservation(action), AdapterProvenance: action.AdapterProvenance, Consent: action.Consent}
	switch action.Kind {
	case ActionCodexProjectTrust:
		effect.Category, effect.Target = ProjectActivationTrust, "<codex-home>/config.toml"
	default:
		return ProjectActivationEffectPreview{}, false
	}
	return effect, true
}

func projectActivationEffectsConverged(surface Surface, observation SurfaceInspection) bool {
	if len(observation.ProjectActivationActions) != 0 {
		return false
	}
	if surface == SurfaceCodex {
		return observation.Readiness.AuthorizationObserved && observation.Readiness.Authorized
	}
	return true
}

func (f Facade) ApplyProjectActivation(ctx context.Context, request ProjectActivationApplyRequest) (ProjectActivationApplyResult, error) {
	preview := request.Preview
	if !request.Interactive {
		return ProjectActivationApplyResult{}, errors.New("project activation requires interactive explicit approvals")
	}
	if preview.projectRoot == "" || preview.packyHome == "" || request.Adapter == nil {
		return ProjectActivationApplyResult{}, errors.New("project activation Apply requires the exact preview and surface adapter")
	}
	if preview.Disposition == ProjectActivationNotRequired {
		return ProjectActivationApplyResult{}, errors.New("project activation is not-required and cannot create empty personal state")
	}
	if preview.Disposition == ProjectActivationInheritedGlobal {
		return ProjectActivationApplyResult{}, errors.New("project activation is inherited-global and cannot create redundant personal state")
	}
	if preview.Disposition == ProjectActivationBlocked {
		return ProjectActivationApplyResult{}, errors.New("project activation preview is blocked")
	}
	fresh, err := f.PreviewProjectActivation(ctx, ProjectActivationRequest{PackID: preview.Pack.ID, Surface: preview.Surface, ProjectRoot: preview.projectRoot, PackyHome: preview.packyHome, Adapter: request.Adapter})
	if err != nil {
		return ProjectActivationApplyResult{}, err
	}
	if fresh.Digest != preview.Digest {
		return ProjectActivationApplyResult{}, errors.New("project activation preview is stale; run the activation preview again")
	}
	if err := validateProjectActivationApprovals(preview, request.Approvals); err != nil {
		return ProjectActivationApplyResult{}, err
	}
	rootDigest, err := projectActivationRootDigest(preview.projectRoot)
	if err != nil {
		return ProjectActivationApplyResult{}, err
	}
	state := projectActivationState{SchemaVersion: projectActivationDocumentSchemaVersion, PackID: preview.Pack.ID, Version: preview.Pack.Version, Surface: preview.Surface, ProjectRootDigest: rootDigest, SensitiveLockIdentity: preview.SensitiveLockIdentity}
	receipts := make([]projectActivationReceipt, 0, len(preview.Categories))
	for _, category := range preview.Categories {
		receipts = append(receipts, projectActivationReceipt{Category: category.Kind, Digest: preview.Digest, Details: append([]ProjectSensitiveDisclosure(nil), category.Details...)})
	}
	effectReceipts := projectActivationEffectReceipts(fresh.actions)
	if existing, exists, loadErr := loadProjectActivationDocumentForSurface(preview.packyHome, preview.projectRoot, preview.Pack.ID, preview.Surface); loadErr != nil {
		return ProjectActivationApplyResult{}, loadErr
	} else if exists {
		effectReceipts = mergeProjectActivationEffectReceipts(existing.Effects, effectReceipts)
	}
	actions := append([]ProjectionAction(nil), fresh.actions...)
	for i := range actions {
		actions[i].PreviewOnly = false
	}
	if actionErr := request.Adapter.ApplyProjections(ctx, actions); actionErr != nil {
		return ProjectActivationApplyResult{}, actionErr
	}
	installation, err := LoadProjectInstallation(preview.projectRoot)
	if err != nil {
		return ProjectActivationApplyResult{}, err
	}
	scopedInstallation := projectInstallationForPack(installation, preview.Pack.ID)
	verified, err := inspectSurface(ctx, request.Adapter, SurfaceTransition{ProjectRoot: preview.projectRoot, ProjectInstallation: &scopedInstallation, ProjectGoal: ProjectionPresent})
	if err != nil || !projectActivationEffectsConverged(preview.Surface, verified) {
		if err != nil {
			return ProjectActivationApplyResult{}, err
		}
		return ProjectActivationApplyResult{}, fmt.Errorf("%s project runtime effects were not verified after activation", preview.Surface)
	}
	state.Active = true
	if err := saveProjectActivationRecords(preview.packyHome, preview.projectRoot, state, request.Approvals, receipts, effectReceipts); err != nil {
		return ProjectActivationApplyResult{}, err
	}
	return ProjectActivationApplyResult{SchemaVersion: 1, Report: "project-activation-apply", Status: "active", Digest: preview.Digest}, nil
}

func projectActivationCategories(lock ProjectLockProposal, surface Surface) []ProjectActivationCategoryPreview {
	byCategory := map[ProjectActivationCategory][]ProjectSensitiveDisclosure{}
	for _, disclosure := range lock.Sensitive {
		if disclosure.Surface == surface {
			byCategory[disclosure.Category] = append(byCategory[disclosure.Category], disclosure)
		}
	}
	for _, binding := range lock.Bindings {
		if binding.Surface != surface {
			continue
		}
		category := ProjectActivationCategory("")
		switch {
		case binding.Kind == "mcp_server" || binding.Projection == "mcp_server":
			category = ProjectActivationMCP
		case binding.Kind == "lifecycle" || binding.Projection == "lifecycle":
			category = ProjectActivationHooks
		case binding.Projection == "plugin":
			category = ProjectActivationPlugins
		}
		if category != "" {
			byCategory[category] = append(byCategory[category], ProjectSensitiveDisclosure{Category: category, Surface: surface, Resource: ResourceIdentity{Kind: binding.Kind, ID: binding.ID}, Detail: binding.Projection})
		}
	}
	order := []ProjectActivationCategory{ProjectActivationMCP, ProjectActivationHooks, ProjectActivationPlugins, ProjectActivationExternalRequirements, ProjectActivationTrust, ProjectActivationAuthentication}
	result := make([]ProjectActivationCategoryPreview, 0, len(order))
	for _, kind := range order {
		details := deduplicateProjectSensitiveDisclosures(byCategory[kind])
		if len(details) != 0 {
			result = append(result, ProjectActivationCategoryPreview{Kind: kind, Details: details, ApprovalRequired: true})
		}
	}
	return result
}

// projectSensitiveDisclosures records every non-declarative concern in the
// portable project lock while the full selected pack is still available.
func projectSensitiveDisclosures(pack Pack, surface Surface) []ProjectSensitiveDisclosure {
	values := []ProjectSensitiveDisclosure{}
	for _, resource := range pack.Resources {
		identity := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		for _, tool := range resource.RequiresTools {
			values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationExternalRequirements, Surface: surface, Resource: identity, Detail: "tool:" + tool})
		}
		for _, binding := range resource.Bindings {
			if binding.Surface == surface && (resource.Kind == "mcp_server" || binding.Projection == "mcp_server") {
				values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationTrust, Surface: surface, Resource: identity, Detail: "project-trust"})
			}
		}
		for _, permission := range resource.Permissions {
			values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationTrust, Surface: surface, Resource: identity, Detail: permission})
		}
		for _, mode := range resource.RuntimeModes {
			for _, requirement := range mode.Requirements {
				category := ProjectActivationExternalRequirements
				if requirement.Kind == RuntimeRequirementAuthentication {
					category = ProjectActivationAuthentication
				}
				values = append(values, ProjectSensitiveDisclosure{Category: category, Surface: surface, Resource: identity, Detail: "runtime:" + mode.ID + ":" + string(requirement.Kind) + ":" + requirement.ID})
			}
			for _, authority := range mode.Authorities {
				values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationTrust, Surface: surface, Resource: identity, Detail: "runtime:" + mode.ID + ":" + string(authority.Kind) + ":" + string(authority.Scope)})
			}
			for _, effect := range mode.Effects {
				if effect.Kind == RuntimeEffectAuthenticationStateChange {
					values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationAuthentication, Surface: surface, Resource: identity, Detail: "runtime:" + mode.ID + ":" + string(effect.Kind) + ":" + string(effect.Scope)})
				}
			}
		}
	}
	for _, tool := range pack.Requires.Tools {
		matchedResource := false
		for _, resource := range pack.Resources {
			if resource.Kind == "mcp_server" && resource.Command == tool {
				values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationExternalRequirements, Surface: surface, Resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, Detail: "tool:" + tool})
				matchedResource = true
				break
			}
		}
		if !matchedResource {
			values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationExternalRequirements, Surface: surface, Resource: ResourceIdentity{Kind: "pack", ID: pack.ID}, Detail: "tool:" + tool})
		}
	}
	return deduplicateProjectSensitiveDisclosures(values)
}

func deduplicateProjectSensitiveDisclosures(values []ProjectSensitiveDisclosure) []ProjectSensitiveDisclosure {
	seen := map[string]bool{}
	result := make([]ProjectSensitiveDisclosure, 0, len(values))
	for _, value := range values {
		key := string(value.Category) + "\x00" + string(value.Surface) + "\x00" + value.Resource.String() + "\x00" + value.Detail
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := fmt.Sprintf("%s\x00%d\x00%s\x00%s", result[i].Surface, projectActivationCategoryIndex(result[i].Category), result[i].Resource.String(), result[i].Detail)
		right := fmt.Sprintf("%s\x00%d\x00%s\x00%s", result[j].Surface, projectActivationCategoryIndex(result[j].Category), result[j].Resource.String(), result[j].Detail)
		return left < right
	})
	return result
}

func projectActivationCategoryIndex(category ProjectActivationCategory) int {
	switch category {
	case ProjectActivationMCP:
		return 0
	case ProjectActivationHooks:
		return 1
	case ProjectActivationPlugins:
		return 2
	case ProjectActivationExternalRequirements:
		return 3
	case ProjectActivationTrust:
		return 4
	case ProjectActivationAuthentication:
		return 5
	default:
		return 99
	}
}

func projectSensitiveLockIdentity(lock ProjectLockProposal, categories []ProjectActivationCategoryPreview) string {
	surface := Surface("")
	if len(categories) > 0 && len(categories[0].Details) > 0 {
		surface = categories[0].Details[0].Surface
	}
	resources := map[ResourceIdentity]bool{}
	for _, category := range categories {
		for _, detail := range category.Details {
			resources[detail.Resource] = true
		}
	}
	projections := make([]ProjectProjectionPlan, 0)
	for _, projection := range lock.Projections {
		if resources[projection.Resource] && projectProjectionOwnedBySurface(projection, surface) {
			projections = append(projections, projection)
		}
	}
	sort.Slice(projections, func(i, j int) bool {
		return projections[i].Resource.String()+"\x00"+projections[i].Target < projections[j].Resource.String()+"\x00"+projections[j].Target
	})
	sensitive := make([]ProjectSensitiveDisclosure, 0)
	for _, disclosure := range lock.Sensitive {
		if disclosure.Surface == surface {
			sensitive = append(sensitive, disclosure)
		}
	}
	bindings := make([]LifecycleBinding, 0)
	for _, binding := range lock.Bindings {
		if binding.Surface == surface {
			bindings = append(bindings, binding)
		}
	}
	data, _ := json.Marshal(struct {
		Sensitive   []ProjectSensitiveDisclosure       `json:"sensitive"`
		Bindings    []LifecycleBinding                 `json:"bindings"`
		Modes       []OptionalMode                     `json:"modes"`
		Projections []ProjectProjectionPlan            `json:"sensitive_projections"`
		Categories  []ProjectActivationCategoryPreview `json:"categories"`
	}{sensitive, bindings, lock.Modes, projections, categories})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectProjectionOwnedBySurface(projection ProjectProjectionPlan, surface Surface) bool {
	return projection.Surface == surface
}

func sealProjectActivationPreview(preview JSONProjectActivationPreview) string {
	preview.ProjectRoot, preview.Digest, preview.projectRoot, preview.packyHome, preview.request, preview.actions = "", "", "", "", ProjectActivationRequest{}, nil
	data, _ := json.Marshal(preview)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectActivationActionObservation(action ProjectionAction) string {
	data, _ := json.Marshal(struct {
		ID, Target, Version, Precondition, AdapterProvenance string
		Kind                                                 ProjectionActionKind
		Consent                                              ConsentKind
		FileMode                                             uint32
	}{action.ID, action.Target, action.Version, action.Precondition, action.AdapterProvenance, action.Kind, action.Consent, action.FileMode})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectActivationEffectReceipts(actions []ProjectionAction) []ProjectActivationEffectReceipt {
	receipts := make([]ProjectActivationEffectReceipt, 0, len(actions))
	for _, action := range actions {
		receipts = append(receipts, ProjectActivationEffectReceipt{Action: action.Kind, Surface: action.Surface, Target: action.Target, ContributionIdentity: action.Version, AdapterProvenance: action.AdapterProvenance, StartMarker: action.ContributionStartMarker, EndMarker: action.ContributionEndMarker, PriorState: "absent"})
	}
	return receipts
}

func mergeProjectActivationEffectReceipts(left, right []ProjectActivationEffectReceipt) []ProjectActivationEffectReceipt {
	byTarget := map[string]ProjectActivationEffectReceipt{}
	for _, receipt := range append(append([]ProjectActivationEffectReceipt(nil), left...), right...) {
		byTarget[string(receipt.Action)+"\x00"+receipt.Target] = receipt
	}
	result := make([]ProjectActivationEffectReceipt, 0, len(byTarget))
	for _, receipt := range byTarget {
		result = append(result, receipt)
	}
	sort.Slice(result, func(i, j int) bool {
		return string(result[i].Action)+"\x00"+result[i].Target < string(result[j].Action)+"\x00"+result[j].Target
	})
	return result
}

func validateProjectActivationApprovals(preview JSONProjectActivationPreview, approvals []ProjectActivationApproval) error {
	if len(approvals) != len(preview.Categories) {
		return errors.New("project activation requires one exact approval for every disclosed category")
	}
	want := map[ProjectActivationCategory]bool{}
	for _, category := range preview.Categories {
		want[category.Kind] = true
	}
	for _, approval := range approvals {
		if !want[approval.Category] || approval.Digest != preview.Digest {
			return errors.New("project activation approval does not match the exact preview")
		}
		delete(want, approval.Category)
	}
	if len(want) != 0 {
		return errors.New("project activation is missing a disclosed category approval")
	}
	return nil
}

func projectActivationRootDigest(projectRoot string) (string, error) {
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(canonical)))
	return hex.EncodeToString(sum[:]), nil
}

func projectActivationDirectory(packyHome, projectRoot string) (string, error) {
	digest, err := projectActivationRootDigest(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(packyHome, "projects", digest), nil
}

func loadProjectActivationDocumentForSurface(packyHome, projectRoot, packID string, surface Surface) (projectActivationDocument, bool, error) {
	directory, err := projectActivationDirectory(packyHome, projectRoot)
	if err != nil {
		return projectActivationDocument{}, false, err
	}
	data, err := os.ReadFile(filepath.Join(directory, projectActivationStateFile(packID, surface)))
	if errors.Is(err, fs.ErrNotExist) {
		return projectActivationDocument{}, false, nil
	}
	if err != nil {
		return projectActivationDocument{}, false, err
	}
	var document projectActivationDocument
	if err := strictDecode(data, &document); err != nil {
		return projectActivationDocument{}, false, fmt.Errorf("decode project activation state: %w", err)
	}
	rootDigest, err := projectActivationRootDigest(projectRoot)
	if err != nil {
		return projectActivationDocument{}, false, err
	}
	if document.SchemaVersion != projectActivationDocumentSchemaVersion || document.State.SchemaVersion != projectActivationDocumentSchemaVersion || document.State.Surface != surface || document.State.ProjectRootDigest != rootDigest || document.SensitiveLockIdentity == "" || document.SensitiveLockIdentity != document.State.SensitiveLockIdentity || len(document.Approvals) == 0 || len(document.Approvals) != len(document.Receipts) {
		return projectActivationDocument{}, false, errors.New("project activation state is incomplete or unsupported")
	}
	approvalDigests := map[ProjectActivationCategory]string{}
	for _, approval := range document.Approvals {
		if !validProjectActivationCategory(approval.Category) || approval.Digest == "" || approvalDigests[approval.Category] != "" {
			return projectActivationDocument{}, false, errors.New("project activation approvals are incomplete or duplicated")
		}
		approvalDigests[approval.Category] = approval.Digest
	}
	receipts := map[ProjectActivationCategory]bool{}
	for _, receipt := range document.Receipts {
		canonicalDetails := deduplicateProjectSensitiveDisclosures(receipt.Details)
		validDetails := len(receipt.Details) > 0 && digestJSON(canonicalDetails) == digestJSON(receipt.Details)
		for _, detail := range receipt.Details {
			validDetails = validDetails && detail.Category == receipt.Category && detail.Surface == surface && detail.Detail != ""
		}
		if receipts[receipt.Category] || approvalDigests[receipt.Category] == "" || receipt.Digest != approvalDigests[receipt.Category] || !validDetails {
			return projectActivationDocument{}, false, errors.New("project activation receipts do not match exact approvals")
		}
		receipts[receipt.Category] = true
	}
	effects := map[string]bool{}
	for _, effect := range document.Effects {
		key := string(effect.Action) + "\x00" + effect.Target
		if effects[key] || !validProjectActivationEffectReceipt(effect) {
			return projectActivationDocument{}, false, errors.New("project activation effect receipts are malformed or duplicated")
		}
		effects[key] = true
	}
	return document, true, nil
}

func validProjectActivationEffectReceipt(effect ProjectActivationEffectReceipt) bool {
	if (effect.Surface != SurfaceCodex && effect.Surface != SurfaceOpenCode && effect.Surface != SurfaceClaude) || !filepath.IsAbs(effect.Target) || effect.ContributionIdentity == "" || effect.AdapterProvenance == "" || effect.PriorState != "absent" {
		return false
	}
	switch effect.Action {
	case ActionCodexProjectTrust:
		return effect.StartMarker != "" && effect.EndMarker != ""
	default:
		return false
	}
}

func saveProjectActivationRecords(packyHome, projectRoot string, state projectActivationState, approvals []ProjectActivationApproval, receipts []projectActivationReceipt, effects []ProjectActivationEffectReceipt) error {
	directory, err := projectActivationDirectory(packyHome, projectRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	document := projectActivationDocument{
		SchemaVersion: projectActivationDocumentSchemaVersion, State: state, Approvals: approvals, Receipts: receipts, Effects: effects,
		SensitiveLockIdentity: state.SensitiveLockIdentity,
	}
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	return writeProjectActivationRecord(filepath.Join(directory, projectActivationStateFile(state.PackID, state.Surface)), data)
}

func projectActivationStateFile(packID string, surface Surface) string {
	return "state-" + packID + "-" + string(surface) + ".json"
}

func writeProjectActivationRecord(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("project activation record %s is unsafe", filepath.Base(path))
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return atomicWriteState(path, append(data, '\n'))
}
