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
	"strings"
)

const ProjectActivationPreviewSchemaVersion = 1

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
	ProjectActivationPreviewable ProjectActivationDisposition = "previewable"
	ProjectActivationNotRequired ProjectActivationDisposition = "not-required"
	ProjectActivationConverged   ProjectActivationDisposition = "converged"
	ProjectActivationBlocked     ProjectActivationDisposition = "blocked"
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
	Category    ProjectActivationCategory `json:"category"`
	Action      ProjectionActionKind      `json:"action"`
	Target      string                    `json:"target"`
	Identity    string                    `json:"identity"`
	Observation string                    `json:"observation"`
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
	Category ProjectActivationCategory `json:"category"`
	Digest   string                    `json:"digest"`
}

type projectActivationEffectReceipt struct {
	Action               ProjectionActionKind `json:"action"`
	Target               string               `json:"target"`
	ContributionIdentity string               `json:"contribution_identity"`
	StartMarker          string               `json:"start_marker"`
	EndMarker            string               `json:"end_marker"`
	PriorState           string               `json:"prior_state"`
}

type projectActivationRecovery struct {
	Status string `json:"status"`
}

type projectActivationDocument struct {
	SchemaVersion         int                              `json:"schema_version"`
	State                 projectActivationState           `json:"state"`
	Approvals             []ProjectActivationApproval      `json:"approvals"`
	Receipts              []projectActivationReceipt       `json:"receipts"`
	Effects               []projectActivationEffectReceipt `json:"effects"`
	SensitiveLockIdentity string                           `json:"sensitive_lock_identity"`
	Recovery              projectActivationRecovery        `json:"recovery"`
}

func (f Facade) PreviewProjectActivation(ctx context.Context, request ProjectActivationRequest) (JSONProjectActivationPreview, error) {
	report := JSONProjectActivationPreview{SchemaVersion: ProjectActivationPreviewSchemaVersion, Report: "project-activation-preview", DryRun: true, ProjectRoot: "<project-root>", Categories: []ProjectActivationCategoryPreview{}, Effects: []ProjectActivationEffectPreview{}, projectRoot: request.ProjectRoot, packyHome: request.PackyHome, request: request}
	if request.Surface != SurfaceCodex {
		return report, fmt.Errorf("project activation preview supports only CLI surface %q", SurfaceCodex)
	}
	if request.ProjectRoot == "" || request.PackyHome == "" || request.PackID == "" || request.Adapter == nil {
		return report, errors.New("project activation preview requires the project root, Packy Home, pack, and Codex adapter")
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
	pack := installation.Manifest.Packs[0]
	if pack.ID != request.PackID || !projectSupportsSurface(pack.Surfaces, request.Surface) {
		return report, fmt.Errorf("capability pack %q is not installed for CLI surface %q", request.PackID, request.Surface)
	}
	status, err := InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: request.ProjectRoot, PackID: request.PackID, Surface: request.Surface, RequireInstalled: true, Adapters: map[Surface]SurfaceAdapter{request.Surface: request.Adapter}})
	if err != nil {
		return report, err
	}
	if len(status.Packs) != 1 || !status.Packs[0].RequirementSatisfied {
		return report, errors.New("project activation requires a healthy installed project projection")
	}
	observation, err := inspectSurface(ctx, request.Adapter, SurfaceTransition{ProjectRoot: request.ProjectRoot, ProjectInstallation: &installation, ProjectGoal: ProjectionPresent})
	if err != nil {
		return report, err
	}
	report.actions = append([]ProjectionAction(nil), observation.ProjectActivationActions...)
	for _, action := range report.actions {
		if action.Kind == ActionCodexProjectTrust {
			report.Effects = append(report.Effects, ProjectActivationEffectPreview{Category: ProjectActivationTrust, Action: action.Kind, Target: "<codex-home>/config.toml", Identity: action.Version, Observation: projectActivationActionObservation(action)})
		}
	}
	categories := projectActivationCategories(installation.Lock, request.Surface)
	report.Pack, report.Surface, report.Categories = pack, request.Surface, categories
	report.RuntimeRequired = len(categories) != 0
	report.SensitiveLockIdentity = projectSensitiveLockIdentity(installation.Lock, categories)
	if !report.RuntimeRequired {
		report.Disposition, report.Digest = ProjectActivationNotRequired, sealProjectActivationPreview(report)
		return report, nil
	}
	document, exists, err := loadProjectActivationDocument(request.PackyHome, request.ProjectRoot)
	if err != nil {
		return report, err
	}
	state := document.State
	if exists && document.Recovery.Status == "clean" && len(report.actions) == 0 && observation.Readiness.AuthorizationObserved && observation.Readiness.Authorized && projectActivationDocumentMatches(document, categories) && state.Active && state.PackID == pack.ID && state.Version == pack.Version && state.Surface == request.Surface && state.SensitiveLockIdentity == report.SensitiveLockIdentity {
		report.Disposition = ProjectActivationConverged
	} else {
		report.Disposition = ProjectActivationPreviewable
	}
	report.Digest = sealProjectActivationPreview(report)
	return report, nil
}

func (f Facade) ApproveProjectActivation(preview JSONProjectActivationPreview, category ProjectActivationCategory) ProjectActivationApproval {
	return ProjectActivationApproval{Category: category, Digest: preview.Digest}
}

func (f Facade) ApplyProjectActivation(ctx context.Context, request ProjectActivationApplyRequest) (ProjectActivationApplyResult, error) {
	preview := request.Preview
	if !request.Interactive {
		return ProjectActivationApplyResult{}, errors.New("project activation requires interactive explicit approvals")
	}
	if preview.projectRoot == "" || preview.packyHome == "" || request.Adapter == nil {
		return ProjectActivationApplyResult{}, errors.New("project activation Apply requires the exact preview and Codex adapter")
	}
	if preview.Disposition == ProjectActivationNotRequired {
		return ProjectActivationApplyResult{}, errors.New("project activation is not-required and cannot create empty personal state")
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
	state := projectActivationState{SchemaVersion: 1, PackID: preview.Pack.ID, Version: preview.Pack.Version, Surface: preview.Surface, ProjectRootDigest: rootDigest, SensitiveLockIdentity: preview.SensitiveLockIdentity}
	receipts := make([]projectActivationReceipt, 0, len(preview.Categories))
	for _, category := range preview.Categories {
		receipts = append(receipts, projectActivationReceipt{Category: category.Kind, Digest: preview.Digest})
	}
	effectReceipts := projectActivationEffectReceipts(fresh.actions)
	if existing, exists, loadErr := loadProjectActivationDocument(preview.packyHome, preview.projectRoot); loadErr != nil {
		return ProjectActivationApplyResult{}, loadErr
	} else if exists {
		effectReceipts = mergeProjectActivationEffectReceipts(existing.Effects, effectReceipts)
	}
	if err := saveProjectActivationRecords(preview.packyHome, preview.projectRoot, state, request.Approvals, receipts, effectReceipts, "applying"); err != nil {
		return ProjectActivationApplyResult{}, err
	}
	actions := append([]ProjectionAction(nil), fresh.actions...)
	for i := range actions {
		actions[i].PreviewOnly = false
	}
	if actionErr := request.Adapter.ApplyProjections(ctx, actions); actionErr != nil {
		_ = saveProjectActivationRecords(preview.packyHome, preview.projectRoot, state, request.Approvals, receipts, effectReceipts, "required")
		return ProjectActivationApplyResult{}, actionErr
	}
	installation, err := LoadProjectInstallation(preview.projectRoot)
	if err != nil {
		_ = saveProjectActivationRecords(preview.packyHome, preview.projectRoot, state, request.Approvals, receipts, effectReceipts, "required")
		return ProjectActivationApplyResult{}, err
	}
	verified, err := inspectSurface(ctx, request.Adapter, SurfaceTransition{ProjectRoot: preview.projectRoot, ProjectInstallation: &installation, ProjectGoal: ProjectionPresent})
	if err != nil || !verified.Readiness.AuthorizationObserved || !verified.Readiness.Authorized {
		_ = saveProjectActivationRecords(preview.packyHome, preview.projectRoot, state, request.Approvals, receipts, effectReceipts, "required")
		if err != nil {
			return ProjectActivationApplyResult{}, err
		}
		return ProjectActivationApplyResult{}, errors.New("Codex project runtime authorization was not verified after activation")
	}
	state.Active = true
	if err := saveProjectActivationRecords(preview.packyHome, preview.projectRoot, state, request.Approvals, receipts, effectReceipts, "clean"); err != nil {
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
		for _, resource := range pack.Resources {
			if resource.Kind == "mcp_server" && resource.Command == tool {
				values = append(values, ProjectSensitiveDisclosure{Category: ProjectActivationExternalRequirements, Surface: surface, Resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, Detail: "tool:" + tool})
				break
			}
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
		if resources[projection.Resource] && projectProjectionHasSurfaceContributor(projection, surface) {
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
		Source      ProjectPackSourceIdentity          `json:"source"`
		Sensitive   []ProjectSensitiveDisclosure       `json:"sensitive"`
		Bindings    []LifecycleBinding                 `json:"bindings"`
		Modes       []OptionalMode                     `json:"modes"`
		Projections []ProjectProjectionPlan            `json:"sensitive_projections"`
		Categories  []ProjectActivationCategoryPreview `json:"categories"`
	}{lock.Source, sensitive, bindings, lock.Modes, projections, categories})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectProjectionHasSurfaceContributor(projection ProjectProjectionPlan, surface Surface) bool {
	prefix := "surface:" + string(surface) + ":"
	if strings.HasPrefix(projection.Contributor, prefix) {
		return true
	}
	for _, contributor := range projection.Contributors {
		if strings.HasPrefix(contributor, prefix) {
			return true
		}
	}
	return false
}

func sealProjectActivationPreview(preview JSONProjectActivationPreview) string {
	preview.ProjectRoot, preview.Digest, preview.projectRoot, preview.packyHome, preview.request, preview.actions = "", "", "", "", ProjectActivationRequest{}, nil
	data, _ := json.Marshal(preview)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectActivationActionObservation(action ProjectionAction) string {
	data, _ := json.Marshal(struct {
		ID, Target, Version, Precondition string
		Kind                              ProjectionActionKind
		FileMode                          uint32
	}{action.ID, action.Target, action.Version, action.Precondition, action.Kind, action.FileMode})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectActivationEffectReceipts(actions []ProjectionAction) []projectActivationEffectReceipt {
	receipts := make([]projectActivationEffectReceipt, 0, len(actions))
	for _, action := range actions {
		receipts = append(receipts, projectActivationEffectReceipt{Action: action.Kind, Target: action.Target, ContributionIdentity: action.Version, StartMarker: action.ContributionStartMarker, EndMarker: action.ContributionEndMarker, PriorState: "absent"})
	}
	return receipts
}

func mergeProjectActivationEffectReceipts(left, right []projectActivationEffectReceipt) []projectActivationEffectReceipt {
	byTarget := map[string]projectActivationEffectReceipt{}
	for _, receipt := range append(append([]projectActivationEffectReceipt(nil), left...), right...) {
		byTarget[string(receipt.Action)+"\x00"+receipt.Target] = receipt
	}
	result := make([]projectActivationEffectReceipt, 0, len(byTarget))
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

func loadProjectActivationDocument(packyHome, projectRoot string) (projectActivationDocument, bool, error) {
	directory, err := projectActivationDirectory(packyHome, projectRoot)
	if err != nil {
		return projectActivationDocument{}, false, err
	}
	data, err := os.ReadFile(filepath.Join(directory, "state.json"))
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
	if document.SchemaVersion != 1 || document.State.SchemaVersion != 1 || document.State.ProjectRootDigest != rootDigest || document.SensitiveLockIdentity == "" || document.SensitiveLockIdentity != document.State.SensitiveLockIdentity || !validProjectActivationRecovery(document.Recovery.Status) || len(document.Approvals) == 0 || len(document.Approvals) != len(document.Receipts) {
		return projectActivationDocument{}, false, errors.New("project activation state is incomplete or recovery-required")
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
		if receipts[receipt.Category] || approvalDigests[receipt.Category] == "" || receipt.Digest != approvalDigests[receipt.Category] {
			return projectActivationDocument{}, false, errors.New("project activation receipts do not match exact approvals")
		}
		receipts[receipt.Category] = true
	}
	effects := map[string]bool{}
	for _, effect := range document.Effects {
		key := string(effect.Action) + "\x00" + effect.Target
		if effects[key] || effect.Action != ActionCodexProjectTrust || !filepath.IsAbs(effect.Target) || effect.ContributionIdentity == "" || effect.StartMarker == "" || effect.EndMarker == "" || effect.PriorState != "absent" {
			return projectActivationDocument{}, false, errors.New("project activation effect receipts are malformed or duplicated")
		}
		effects[key] = true
	}
	return document, true, nil
}

func validProjectActivationRecovery(status string) bool {
	return status == "clean" || status == "applying" || status == "required"
}

func projectActivationDocumentMatches(document projectActivationDocument, categories []ProjectActivationCategoryPreview) bool {
	if len(document.Approvals) != len(categories) {
		return false
	}
	want := map[ProjectActivationCategory]bool{}
	for _, category := range categories {
		want[category.Kind] = true
	}
	for _, approval := range document.Approvals {
		if !want[approval.Category] {
			return false
		}
		delete(want, approval.Category)
	}
	return len(want) == 0
}

func saveProjectActivationRecords(packyHome, projectRoot string, state projectActivationState, approvals []ProjectActivationApproval, receipts []projectActivationReceipt, effects []projectActivationEffectReceipt, recovery string) error {
	directory, err := projectActivationDirectory(packyHome, projectRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	document := projectActivationDocument{
		SchemaVersion: 1, State: state, Approvals: approvals, Receipts: receipts, Effects: effects,
		SensitiveLockIdentity: state.SensitiveLockIdentity, Recovery: projectActivationRecovery{Status: recovery},
	}
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	return writeProjectActivationRecord(filepath.Join(directory, "state.json"), data)
}

func writeProjectActivationRecord(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("project activation record %s is unsafe", filepath.Base(path))
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return atomicWriteState(path, append(data, '\n'))
}
