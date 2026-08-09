package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	ProjectInstallPreviewSchemaVersion = 3
	projectContractSchemaV1            = 1
)

type ProjectInstallDisposition string

const (
	ProjectInstallPreviewable ProjectInstallDisposition = "previewable"
	ProjectInstallBlocked     ProjectInstallDisposition = "blocked"
	ProjectInstallConverged   ProjectInstallDisposition = "converged"
)

type ProjectInstallRequest struct {
	PackID       string
	Surface      Surface
	ProjectRoot  string
	Selection    ResourceSelection
	Aliases      []SurfaceAlias
	manifestPack ProjectManifestPack
	update       bool
	force        bool
}

type ProjectUpdateRequest struct {
	PackID      string
	Surface     Surface
	ProjectRoot string
	Force       bool
}

type ProjectInstallBlocker struct {
	Code        string           `json:"code"`
	Resource    ResourceIdentity `json:"resource,omitempty"`
	Target      string           `json:"target,omitempty"`
	Detail      string           `json:"detail"`
	Remediation string           `json:"remediation"`
}

type ProjectContractProposal struct {
	Path          string                `json:"path"`
	SchemaVersion int                   `json:"schema_version"`
	Packs         []ProjectManifestPack `json:"packs"`
}

type ProjectManifestPack struct {
	ID             string                 `json:"id"`
	Version        string                 `json:"version"`
	Surfaces       []Surface              `json:"surfaces"`
	Selection      ResourceSelection      `json:"selection"`
	Aliases        []SurfaceAlias         `json:"aliases"`
	SurfaceIntents []ProjectSurfaceIntent `json:"surface_intents,omitempty"`
}

type ProjectSurfaceIntent struct {
	Surface   Surface           `json:"surface"`
	Version   string            `json:"version"`
	Selection ResourceSelection `json:"selection"`
	Aliases   []SurfaceAlias    `json:"aliases"`
}

type ProjectLockProposal struct {
	Path                   string                       `json:"-"`
	SchemaVersion          int                          `json:"schema_version"`
	Receipts               []installedPackReceipt       `json:"receipts"`
	MinimumPackyCapability string                       `json:"-"`
	ResourceGraph          ResourceGraph                `json:"-"`
	Bindings               []LifecycleBinding           `json:"-"`
	Degradations           []LifecycleExclusion         `json:"-"`
	Modes                  []OptionalMode               `json:"-"`
	Sensitive              []ProjectSensitiveDisclosure `json:"-"`
	ManifestSHA256         string                       `json:"-"`
	NoticesSHA256          string                       `json:"-"`
	NoticesFileMode        uint32                       `json:"-"`
	Projections            []ProjectProjectionPlan      `json:"-"`
}

type ProjectNoticesProposal struct {
	Path          string                      `json:"path"`
	Contributions []ProjectNoticeContribution `json:"contributions"`
}

type ProjectSensitiveChange struct {
	Change   string                    `json:"change"`
	Category ProjectActivationCategory `json:"category"`
	Surface  Surface                   `json:"surface"`
	Resource ResourceIdentity          `json:"resource"`
	Detail   string                    `json:"detail"`
}

type ProjectNoticeContribution struct {
	Resource    ResourceIdentity `json:"resource"`
	License     string           `json:"license,omitempty"`
	Attribution string           `json:"attribution,omitempty"`
}

type ProjectSelectionPreview struct {
	Mode      SelectionMode         `json:"mode"`
	Resources []ResourceClosureFact `json:"resources"`
}

type ProjectProjectionPlan struct {
	Resource           ResourceIdentity `json:"resource"`
	Target             string           `json:"target"`
	Mode               string           `json:"mode"`
	FileMode           uint32           `json:"file_mode"`
	DesiredFingerprint string           `json:"desired_fingerprint,omitempty"`
	ObservedState      string           `json:"observed_state"`
	OwnerPack          string           `json:"owner_pack"`
	Surface            Surface          `json:"surface"`
	Command            string           `json:"command,omitempty"`
	Args               []string         `json:"args,omitempty"`
}

type JSONProjectInstallPreview struct {
	SchemaVersion     int                       `json:"schema_version"`
	Report            string                    `json:"report"`
	DryRun            bool                      `json:"dry_run"`
	ProjectRoot       string                    `json:"project_root"`
	Pack              ProjectManifestPack       `json:"pack"`
	Surface           Surface                   `json:"surface"`
	Selection         ProjectSelectionPreview   `json:"selection"`
	Manifest          ProjectContractProposal   `json:"manifest"`
	Lock              ProjectLockProposal       `json:"lock"`
	Notices           ProjectNoticesProposal    `json:"notices"`
	Projections       []ProjectProjectionPlan   `json:"projections"`
	Retirements       []ProjectProjectionPlan   `json:"retirements,omitempty"`
	SensitiveChanges  []ProjectSensitiveChange  `json:"sensitive_changes,omitempty"`
	Requirements      []string                  `json:"requirements"`
	Blockers          []ProjectInstallBlocker   `json:"blockers"`
	Disposition       ProjectInstallDisposition `json:"disposition"`
	Observation       string                    `json:"observation"`
	ExpectedReadiness ReadinessStatus           `json:"expected_readiness"`
	Conditions        []ReadinessCondition      `json:"conditions"`
	projectRoot       string
	pack              Pack
	actions           []ProjectionAction
	noticeContent     string
	noticeMode        uint32
	noticeBefore      string
	noticeIntact      bool
	request           ProjectInstallRequest
	updateRequest     ProjectUpdateRequest
}

type ProjectInstallNotActionableError struct{ Disposition ProjectInstallDisposition }

func (e ProjectInstallNotActionableError) Error() string {
	return fmt.Sprintf("project install preview is not actionable: %s", e.Disposition)
}

type ProjectInstallFreshness struct {
	Disposition ProjectInstallDisposition `json:"disposition"`
	Blockers    []ProjectInstallBlocker   `json:"blockers"`
}

func (f Facade) CheckProjectInstallFreshness(ctx context.Context, preview JSONProjectInstallPreview, adapter SurfaceAdapter) (ProjectInstallFreshness, error) {
	if preview.projectRoot == "" {
		return ProjectInstallFreshness{}, errors.New("project install preview no longer carries its sealed project root")
	}
	if preview.updateRequest.PackID != "" {
		fresh, err := f.PreviewProjectUpdate(ctx, preview.updateRequest, adapter)
		if err != nil {
			return ProjectInstallFreshness{}, err
		}
		if fresh.Observation != preview.Observation {
			return ProjectInstallFreshness{Disposition: ProjectInstallBlocked, Blockers: []ProjectInstallBlocker{{Code: "stale_observation", Detail: "project targets changed after the preview was created", Remediation: "run the update dry-run again to obtain a fresh preview"}}}, nil
		}
		return ProjectInstallFreshness{Disposition: preview.Disposition, Blockers: append([]ProjectInstallBlocker(nil), preview.Blockers...)}, nil
	}
	request := preview.request
	if request.ProjectRoot == "" {
		request = ProjectInstallRequest{PackID: preview.Pack.ID, Surface: preview.Surface, ProjectRoot: preview.projectRoot}
	}
	fresh, err := f.PreviewProjectInstall(ctx, request, adapter)
	if err != nil {
		return ProjectInstallFreshness{}, err
	}
	if fresh.Observation != preview.Observation {
		return ProjectInstallFreshness{Disposition: ProjectInstallBlocked, Blockers: []ProjectInstallBlocker{{Code: "stale_observation", Detail: "project targets changed after the preview was created", Remediation: "run the install dry-run again to obtain a fresh preview"}}}, nil
	}
	return ProjectInstallFreshness{Disposition: preview.Disposition, Blockers: append([]ProjectInstallBlocker(nil), preview.Blockers...)}, nil
}

func (f Facade) PreviewProjectInstall(ctx context.Context, request ProjectInstallRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (JSONProjectInstallPreview, error) {
		return locked.previewProjectInstall(ctx, request, adapter)
	})
}

// PreviewProjectUpdate updates one project Pack surface to the current bundled
// version while preserving its selected intent.
func (f Facade) PreviewProjectUpdate(ctx context.Context, request ProjectUpdateRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (JSONProjectInstallPreview, error) {
		return locked.previewProjectUpdate(ctx, request, adapter)
	})
}

func (f Facade) previewProjectUpdate(ctx context.Context, request ProjectUpdateRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	if request.ProjectRoot == "" || request.PackID == "" || request.Surface == "" {
		return JSONProjectInstallPreview{}, errors.New("project update requires the project root, Pack, and surface")
	}
	installation, err := LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	prior, installed := findProjectManifestPack(installation.Manifest.Packs, request.PackID)
	if !installed {
		return JSONProjectInstallPreview{}, fmt.Errorf("project does not declare capability Pack %q", request.PackID)
	}
	targetPack, err := f.resolveProjectPackUnlocked(request.PackID)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if adapter == nil {
		return JSONProjectInstallPreview{}, errors.New("project update requires the selected surface adapter")
	}
	intents := projectSurfaceIntents(prior)
	var report JSONProjectInstallPreview
	foundSurface := false
	selectedVersion := ""
	for _, intent := range intents {
		if intent.Surface != request.Surface {
			continue
		}
		selectedVersion = intent.Version
		intent.Version = targetPack.Version
		target := withProjectSurfaceIntent(prior, intent)
		surfaceReport, previewErr := f.previewProjectInstall(ctx, ProjectInstallRequest{
			PackID: request.PackID, Surface: intent.Surface, ProjectRoot: request.ProjectRoot,
			Selection: intent.Selection, Aliases: intent.Aliases,
			manifestPack: target, update: true, force: request.Force,
		}, adapter)
		if previewErr != nil {
			return JSONProjectInstallPreview{}, previewErr
		}
		report = surfaceReport
		foundSurface = true
	}
	if !foundSurface {
		return JSONProjectInstallPreview{}, fmt.Errorf("project does not install capability Pack %q on %s", request.PackID, request.Surface)
	}
	if selectedVersion == targetPack.Version {
		report.updateRequest = request
		return report, nil
	}
	report.Lock, err = hydrateProjectLock(report.Lock)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	combined, err := finalizeProjectSurfaceUpdate(request.ProjectRoot, report.Pack, report)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	combined.updateRequest = request
	combined.Surface = request.Surface
	return f.addProjectUpdateRetirements(ctx, projectInstallationForPack(installation, prior.ID), combined, adapter)
}

func (f Facade) addProjectUpdateRetirements(ctx context.Context, prior ProjectInstallation, combined JSONProjectInstallPreview, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	desired := map[string]bool{}
	for _, projection := range combined.Lock.Projections {
		desired[string(projection.Surface)+"\x00"+projection.Resource.String()+"\x00"+filepath.Clean(projection.Target)] = true
	}
	var removals []ProjectionAction
	seenRetirement := map[string]bool{}
	surface := combined.Surface
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{ProjectRoot: combined.projectRoot, ProjectInstallation: &prior, ProjectGoal: ProjectionAbsent})
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	for _, observed := range observation.Projections {
		resource, err := ParseResourceIdentity(observed.ID)
		if err != nil {
			return JSONProjectInstallPreview{}, err
		}
		relativeTarget, err := RelativeProjectTarget(combined.projectRoot, observed.Action.Target)
		if err != nil {
			return JSONProjectInstallPreview{}, err
		}
		key := string(surface) + "\x00" + resource.String() + "\x00" + filepath.Clean(relativeTarget)
		if desired[key] || projectProjectionRetainedByOtherSurface(combined.Lock, surface, resource, relativeTarget) {
			continue
		}
		locked, owned := findProjectSurfaceLockProjection(prior.Lock, surface, resource, relativeTarget)
		if !owned || !observed.Exists || normalizeProjectProjectionFingerprint(observed.ObservedFingerprint) != locked.DesiredFingerprint {
			combined.Blockers = append(combined.Blockers, ProjectInstallBlocker{Code: "owned_drift", Resource: resource, Target: relativeTarget, Detail: "retiring Packy-owned project target differs from the locked content", Remediation: "restore the locked content before project update"})
			continue
		}
		retirementKey := resource.String() + "\x00" + filepath.Clean(relativeTarget)
		if !seenRetirement[retirementKey] {
			retired := locked
			retired.ObservedState = "retire"
			combined.Retirements = append(combined.Retirements, retired)
			seenRetirement[retirementKey] = true
		}
		action := observed.Action
		action.PreviewOnly = false
		removals = append(removals, action)
	}
	removals = coalesceProjectComposableActions(removals)
	seenAction := map[string]bool{}
	for _, action := range combined.actions {
		seenAction[action.ID+"\x00"+filepath.Clean(action.Target)] = true
	}
	for _, action := range removals {
		key := action.ID + "\x00" + filepath.Clean(action.Target)
		if !seenAction[key] {
			combined.actions = append(combined.actions, action)
			seenAction[key] = true
		}
	}
	if len(combined.Blockers) > 0 {
		combined.Disposition = ProjectInstallBlocked
	}
	combined.Observation = sealProjectInstallPreview(combined, combined.Observation)
	return combined, nil
}

func projectProjectionRetainedByOtherSurface(lock ProjectLockProposal, selected Surface, resource ResourceIdentity, target string) bool {
	for _, projection := range lock.Projections {
		if projection.Surface != selected && projection.Resource == resource && filepath.Clean(projection.Target) == filepath.Clean(target) {
			return true
		}
	}
	return false
}

func finalizeProjectSurfaceUpdate(projectRoot string, target ProjectManifestPack, report JSONProjectInstallPreview) (JSONProjectInstallPreview, error) {
	installation, err := LoadProjectInstallation(projectRoot)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	combined := report
	combined.Pack = target
	combined.Manifest = installation.Manifest
	combined.Manifest.Packs = replaceProjectManifestPack(combined.Manifest.Packs, target)
	combined.request = ProjectInstallRequest{}
	combined.updateRequest = ProjectUpdateRequest{PackID: target.ID, Surface: report.Surface, ProjectRoot: projectRoot}
	combined.projectRoot = projectRoot
	combined.Requirements = sortedUnique(combined.Requirements)
	manifestBytes, err := marshalProjectManifest(combined.Manifest)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	combined.Lock.ManifestSHA256 = fingerprintProjectBytes(manifestBytes)
	combined.Lock.NoticesSHA256 = fingerprintProjectBytes([]byte(renderProjectNoticeBlock(combined)))
	combined.SensitiveChanges = projectSensitiveChanges(installation.Lock, combined.Lock, combined.Surface)
	combined.noticeContent, combined.noticeMode, combined.noticeBefore, combined.noticeIntact, combined.Blockers, err = planProjectSurfaceUpdateNotices(combined, installation.Lock, combined.Blockers)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	combinedNoticeBlock := renderProjectNoticeBlock(combined)
	combined.Lock.Receipts = replaceProjectNoticeReceiptProjection(combined.Lock.Receipts, target.ID, combined.Surface, combinedNoticeBlock)
	if len(combined.Blockers) > 0 {
		combined.Disposition = ProjectInstallBlocked
	} else {
		converged, contractBlockers, inspectErr := inspectProjectContract(combined, installation.Lock, true, true)
		if inspectErr != nil {
			return JSONProjectInstallPreview{}, inspectErr
		}
		combined.Blockers = append(combined.Blockers, contractBlockers...)
		if len(contractBlockers) > 0 {
			combined.Disposition = ProjectInstallBlocked
		} else if converged {
			combined.Disposition = ProjectInstallConverged
		}
	}
	combined.Observation = sealProjectInstallPreview(combined, report.Observation+"\nnotices="+combined.noticeBefore)
	return combined, nil
}

func projectSensitiveChanges(prior, desired ProjectLockProposal, surface Surface) []ProjectSensitiveChange {
	versionChanged := projectReceiptVersionChanged(prior.Receipts, desired.Receipts, surface)
	key := func(value ProjectSensitiveDisclosure) string {
		return string(value.Surface) + "\x00" + string(value.Category) + "\x00" + value.Resource.String()
	}
	before := map[string]ProjectSensitiveDisclosure{}
	after := map[string]ProjectSensitiveDisclosure{}
	for _, value := range prior.Sensitive {
		if value.Surface == surface {
			before[key(value)] = value
		}
	}
	for _, value := range desired.Sensitive {
		if value.Surface == surface {
			after[key(value)] = value
		}
	}
	keys := map[string]bool{}
	for value := range before {
		keys[value] = true
	}
	for value := range after {
		keys[value] = true
	}
	result := make([]ProjectSensitiveChange, 0, len(keys))
	for value := range keys {
		old, hadOld := before[value]
		current, hasCurrent := after[value]
		change := "changed"
		disclosure := current
		switch {
		case !hadOld:
			change = "added"
		case !hasCurrent:
			change, disclosure = "removed", old
		case digestJSON(old) == digestJSON(current) && !versionChanged:
			continue
		}
		result = append(result, ProjectSensitiveChange{Change: change, Category: disclosure.Category, Surface: disclosure.Surface, Resource: disclosure.Resource, Detail: "requires fresh personal project activation before the changed runtime effect can be used"})
	}
	sort.Slice(result, func(i, j int) bool {
		left := string(result[i].Surface) + "\x00" + string(result[i].Category) + "\x00" + result[i].Resource.String()
		right := string(result[j].Surface) + "\x00" + string(result[j].Category) + "\x00" + result[j].Resource.String()
		return left < right
	})
	return result
}

func projectReceiptVersionChanged(prior, desired []installedPackReceipt, surface Surface) bool {
	versions := make(map[string]string, len(prior))
	for _, receipt := range prior {
		if receipt.Surface == surface {
			versions[receipt.Pack.ID] = receipt.Pack.Version
		}
	}
	for _, receipt := range desired {
		if receipt.Surface == surface {
			if version, ok := versions[receipt.Pack.ID]; ok && version != receipt.Pack.Version {
				return true
			}
		}
	}
	return false
}

func planProjectSurfaceUpdateNotices(preview JSONProjectInstallPreview, prior ProjectLockProposal, blockers []ProjectInstallBlocker) (string, uint32, string, bool, []ProjectInstallBlocker, error) {
	priorDigest := ""
	if projection, found := projectNoticeReceiptProjection(prior.Receipts, preview.Pack.ID, preview.Surface); found {
		priorDigest = projection.Digest
	}
	content, mode, before, intact, noticeBlockers, err := planProjectNotices(preview, true, true, priorDigest)
	return content, mode, before, intact, append(blockers, noticeBlockers...), err
}

func coalesceProjectComposableActions(actions []ProjectionAction) []ProjectionAction {
	result := make([]ProjectionAction, 0, len(actions))
	indices := map[string]int{}
	for _, action := range actions {
		composable := action.Kind == ActionInstructionFile || action.Kind == ActionOpenCodeInstructionFile || action.Kind == ActionClaudeProjectInstruction || action.Kind == ActionClaudeProjectMCP
		if !composable {
			result = append(result, action)
			continue
		}
		key := string(action.Surface) + "\x00" + filepath.Clean(action.Target)
		if index, ok := indices[key]; ok {
			result[index] = action
			continue
		}
		indices[key] = len(result)
		result = append(result, action)
	}
	return result
}

func (f Facade) previewProjectInstall(ctx context.Context, request ProjectInstallRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	if request.Surface != SurfaceCodex && request.Surface != SurfaceOpenCode && request.Surface != SurfaceClaude {
		return JSONProjectInstallPreview{}, fmt.Errorf("project installation preview does not support CLI surface %q", request.Surface)
	}
	if request.ProjectRoot == "" {
		return JSONProjectInstallPreview{}, errors.New("project root is required")
	}
	pack, err := f.resolveProjectPackUnlocked(request.PackID)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if !projectSupportsSurface(pack.Surfaces, request.Surface) {
		return JSONProjectInstallPreview{}, fmt.Errorf("capability pack %q does not support CLI surface %q", request.PackID, request.Surface)
	}
	selection, err := canonicalSelection(request.Selection)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	selectedPack, err := selectProjectPackResources(pack, selection)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	intentPack := selectedPack
	aliases := cloneAliases(request.Aliases)
	if err := canonicalizeAliases(&aliases); err != nil {
		return JSONProjectInstallPreview{}, err
	}
	for _, alias := range aliases {
		if !idPattern.MatchString(alias.Name) {
			return JSONProjectInstallPreview{}, fmt.Errorf("project alias name %q is invalid", alias.Name)
		}
	}
	explicit := true
	intent := ActivationIntent{PackID: pack.ID, Surface: request.Surface, Version: pack.Version, Active: true, Aliases: aliases, Selection: selection, Explicit: &explicit}
	composition, err := f.composeProject(pack, ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}, request.Surface, aliases)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if len(composition.activations) != 1 || composition.activations[0].Pack.ID != pack.ID {
		return JSONProjectInstallPreview{}, errors.New("project installation cannot resolve a cross-Pack dependency graph")
	}
	for _, alias := range aliases {
		matched := false
		for _, composedPack := range composition.packs {
			matched = matched || packHasAliasTarget(composedPack, alias, request.Surface)
		}
		if !matched {
			return JSONProjectInstallPreview{}, fmt.Errorf("project alias %s:%s does not identify a resource in the resulting selected closure bound to %s", alias.Kind, alias.ID, request.Surface)
		}
	}
	selectedPack = composition.combinedPack()
	compositionBlockers := projectCompositionBlockers(composition.blockers)
	graph := mergeProjectResourceGraphs(ResourceGraph{Resources: []ResourceClosureFact{}}, ResourceGraphFor(pack, selection, false))
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{Desired: selectedPack, ProjectRoot: request.ProjectRoot})
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	projections := make([]ProjectProjectionPlan, 0, len(observation.Projections))
	actions := make([]ProjectionAction, 0, len(observation.Projections))
	blockers := append([]ProjectInstallBlocker(nil), compositionBlockers...)
	existingLock, lockExists, lockErr := readExistingProjectLock(request.ProjectRoot)
	var existingInstallation ProjectInstallation
	existingContract := false
	if lockErr != nil {
		blockers = append(blockers, ProjectInstallBlocker{Code: "invalid_project_lock", Target: "packy.lock.json", Detail: lockErr.Error(), Remediation: "restore or remove the invalid project lock before installation"})
	} else if lockExists {
		installation, contractErr := LoadProjectInstallation(request.ProjectRoot)
		if contractErr != nil {
			blockers = append(blockers, ProjectInstallBlocker{Code: "invalid_project_contract", Target: "packy.json", Detail: contractErr.Error(), Remediation: "restore the supported project manifest and lock before installation"})
			lockExists = false
		} else {
			existingLock = installation.Lock
			existingInstallation, existingContract = installation, true
		}
	}
	for _, resource := range observation.Unrepresentable {
		blockers = append(blockers, ProjectInstallBlocker{Code: "unrepresentable_resource", Resource: resource.Resource, Detail: resource.Reason, Remediation: "choose a surface with a declared project-native representation"})
	}
	for _, projection := range observation.Projections {
		projection.ObservedFingerprint = normalizeProjectProjectionFingerprint(projection.ObservedFingerprint)
		projection.DesiredFingerprint = normalizeProjectProjectionFingerprint(projection.DesiredFingerprint)
		resource, err := ParseResourceIdentity(projection.ID)
		if err != nil {
			return JSONProjectInstallPreview{}, fmt.Errorf("project projection identity: %w", err)
		}
		target, err := RelativeProjectTarget(request.ProjectRoot, projection.Action.Target)
		if err != nil {
			return JSONProjectInstallPreview{}, err
		}
		if err := validateProjectTargetPath(request.ProjectRoot, projection.Action.Target); err != nil {
			blockers = append(blockers, ProjectInstallBlocker{Code: "unsafe_path", Resource: resource, Target: target, Detail: err.Error(), Remediation: "remove the unsafe link or non-directory project path before installation"})
		}
		targetKey := filepath.Clean(target)
		for _, locked := range existingLock.Projections {
			if filepath.Clean(locked.Target) == targetKey && (locked.Resource != resource || !projectProjectionOwnedByPack(locked, pack.ID)) {
				blockers = append(blockers, ProjectInstallBlocker{Code: "projection_collision", Resource: resource, Target: target, Detail: "another installed Pack receipt owns the same project path", Remediation: "select a non-colliding resource root or assign an explicit alias"})
				break
			}
			if request.update && locked.Surface != request.Surface && filepath.Clean(locked.Target) == targetKey && locked.Resource == resource && projectProjectionOwnedByPack(locked, pack.ID) && locked.DesiredFingerprint != projection.DesiredFingerprint {
				blockers = append(blockers, ProjectInstallBlocker{Code: "shared_projection_version_conflict", Resource: resource, Target: target, Detail: "the selected update would replace bytes retained by another installed surface", Remediation: "align the shared projection content before updating surfaces independently"})
				break
			}
		}
		state := "missing"
		if projection.Exists {
			state = "foreign"
			lockedProjection, owned := findProjectLockProjection(existingLock, resource, target)
			if request.update {
				lockedProjection, owned = findProjectSurfaceLockProjection(existingLock, request.Surface, resource, target)
			}
			if lockExists && owned && (request.update || lockedProjection.DesiredFingerprint == projection.DesiredFingerprint) {
				state = "owned"
				if projection.ObservedFingerprint != lockedProjection.DesiredFingerprint {
					state = "drifted"
					if request.force {
						state = "outdated"
					} else {
						blockers = append(blockers, ProjectInstallBlocker{Code: "owned_drift", Resource: resource, Target: target, Detail: "Packy-owned project target differs from the locked content", Remediation: "restore it with a forced project update or restore the receipt digest before retrying"})
					}
				} else if lockedProjection.DesiredFingerprint != projection.DesiredFingerprint {
					state = "outdated"
				}
			} else {
				blockers = append(blockers, ProjectInstallBlocker{Code: "foreign_target", Resource: resource, Target: target, Detail: "project target already exists without portable Packy ownership", Remediation: "move or remove the foreign target before installation"})
			}
		}
		mode, fileMode := "copy_file", projection.Action.FileMode
		if projection.Action.Kind == ActionCodexProjectSkillTree || projection.Action.Kind == ActionClaudeProjectSkillTree {
			mode, fileMode = "copy_tree", 0o700
		}
		if projection.Action.Kind == ActionInstructionFile || projection.Action.Kind == ActionCodexMCPConfig || projection.Action.Kind == ActionOpenCodeInstructionFile || projection.Action.Kind == ActionClaudeProjectInstruction {
			mode, fileMode = "merge_marked_file", projection.Action.FileMode
		} else if projection.Action.Kind == ActionOpenCodeMCPConfig || projection.Action.Kind == ActionClaudeProjectMCP {
			mode, fileMode = "merge_structured_file", projection.Action.FileMode
		}
		projections = append(projections, ProjectProjectionPlan{Resource: resource, Target: target, Mode: mode, FileMode: fileMode, DesiredFingerprint: projection.DesiredFingerprint, ObservedState: state, OwnerPack: pack.ID, Surface: request.Surface, Command: projection.Action.Command, Args: append([]string(nil), projection.Action.Args...)})
		if state != "owned" && state != "drifted" {
			action := projection.Action
			action.PreviewOnly = false
			actions = append(actions, action)
		}
	}
	actions = coalesceProjectComposableActions(actions)
	sort.Slice(projections, func(i, j int) bool {
		if projections[i].Target != projections[j].Target {
			return projections[i].Target < projections[j].Target
		}
		return projections[i].Resource.String() < projections[j].Resource.String()
	})
	sort.Slice(blockers, func(i, j int) bool {
		if blockers[i].Code != blockers[j].Code {
			return blockers[i].Code < blockers[j].Code
		}
		return blockers[i].Resource.String() < blockers[j].Resource.String()
	})
	notices := make([]ProjectNoticeContribution, 0)
	for _, resource := range intentPack.Resources {
		if resource.Kind == "notice" {
			notices = append(notices, ProjectNoticeContribution{Resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, License: resource.License, Attribution: resource.Attribution})
		}
	}
	requirements := projectRequirements(selectedPack)
	if aliases == nil {
		aliases = []SurfaceAlias{}
	}
	manifestPack := withProjectSurfaceIntent(ProjectManifestPack{ID: pack.ID}, ProjectSurfaceIntent{Surface: request.Surface, Version: pack.Version, Selection: selection, Aliases: aliases})
	var prior ProjectManifestPack
	priorFound := false
	if request.update {
		manifestPack = request.manifestPack
	} else if existingContract {
		prior, priorFound = findProjectManifestPack(existingInstallation.Manifest.Packs, pack.ID)
		if priorFound {
			manifestPack = prior
			if !projectSupportsSurface(prior.Surfaces, request.Surface) {
				manifestPack = withProjectSurfaceIntent(manifestPack, ProjectSurfaceIntent{Surface: request.Surface, Version: pack.Version, Selection: selection, Aliases: aliases})
			}
		}
	}
	var manifestPacks []ProjectManifestPack
	if existingContract {
		manifestPacks = replaceProjectManifestPack(existingInstallation.Manifest.Packs, manifestPack)
	} else {
		manifestPacks = []ProjectManifestPack{manifestPack}
	}
	disposition := ProjectInstallPreviewable
	if len(blockers) > 0 {
		disposition = ProjectInstallBlocked
	}
	lockProjections := append([]ProjectProjectionPlan(nil), projections...)
	for i := range lockProjections {
		lockProjections[i].ObservedState = "installed"
	}
	contract := LifecycleContractFor(selectedPack, request.Surface, nil)
	lockBindings := append([]LifecycleBinding{}, contract.Bindings...)
	for i := range lockBindings {
		lockBindings[i].Surface = request.Surface
	}
	lockDegradations := append([]LifecycleExclusion{}, contract.Exclusions...)
	lockModes := append([]OptionalMode{}, contract.OptionalModes...)
	lockSensitive := projectSensitiveDisclosures(selectedPack, request.Surface)
	if existingContract {
		graph = mergeProjectResourceGraphs(existingLock.ResourceGraph, graph)
		lockBindings = mergeProjectBindings(existingLock.Bindings, lockBindings)
		lockDegradations = mergeProjectDegradations(existingLock.Degradations, lockDegradations)
		lockModes = mergeProjectModes(existingLock.Modes, lockModes)
		lockSensitive = mergeProjectSensitiveDisclosures(existingLock.Sensitive, lockSensitive)
		lockProjections, projections = mergeProjectProjections(existingLock.Projections, lockProjections, projections)
	}
	resolutions, resolutionErr := f.resolveExecutables(ctx, selectedPack)
	unobservedRequirements := []string{}
	if resolutionErr != nil {
		unobservedRequirements = append(unobservedRequirements, selectedPack.Requires.Tools...)
		resolutions = nil
	}
	expectedReadiness, readinessConditions := evaluateReadiness(readinessEvaluation{
		Pack: selectedPack, Surface: request.Surface, Scope: ReadinessScopeProject,
		Projections: expectedProjectReadinessProjections(projections), Resolutions: resolutions, UnobservedRequirements: unobservedRequirements,
		Observation: observation.Readiness, Revision: observation.Revision,
	})
	report := JSONProjectInstallPreview{
		SchemaVersion: ProjectInstallPreviewSchemaVersion, Report: "project-install-preview", DryRun: true,
		ProjectRoot: "<project-root>", Pack: manifestPack, Surface: request.Surface, projectRoot: request.ProjectRoot, pack: selectedPack, actions: actions, request: request,
		Selection:   ProjectSelectionPreview{Mode: selection.Mode, Resources: graph.Resources},
		Manifest:    ProjectContractProposal{Path: "packy.json", SchemaVersion: projectContractSchemaV1, Packs: manifestPacks},
		Lock:        ProjectLockProposal{Path: "packy.lock.json", SchemaVersion: projectContractSchemaV1, ResourceGraph: graph, Bindings: lockBindings, Degradations: lockDegradations, Modes: lockModes, Sensitive: lockSensitive, Projections: lockProjections},
		Notices:     ProjectNoticesProposal{Path: "PACKY-NOTICES.md", Contributions: notices},
		Projections: projections, Requirements: append([]string{}, requirements...), Blockers: append([]ProjectInstallBlocker{}, blockers...), Disposition: disposition,
		ExpectedReadiness: expectedReadiness, Conditions: readinessConditions,
	}
	report.Lock.Receipts = replaceProjectReceipt(existingLock.Receipts, projectReceipt(selectedPack, request.Surface, selection, aliases, ResourceGraphFor(pack, selection, false), projections))
	manifestBytes, err := marshalProjectManifest(report.Manifest)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	report.Lock.ManifestSHA256 = fingerprintProjectBytes(manifestBytes)
	report.Lock.NoticesSHA256 = fingerprintProjectBytes([]byte(renderProjectNoticeBlock(report)))
	priorNoticeDigest := ""
	if lockedNotice, found := projectNoticeReceiptProjection(existingLock.Receipts, pack.ID, request.Surface); found {
		priorNoticeDigest = lockedNotice.Digest
	}
	noticeReplacementAllowed := request.update || !priorFound || !projectSupportsSurface(prior.Surfaces, request.Surface)
	noticeContent, noticeMode, noticeBefore, noticeIntact, noticeBlockers, err := planProjectNotices(report, lockExists, noticeReplacementAllowed, priorNoticeDigest)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if !lockExists {
		report.Lock.NoticesFileMode = noticeMode
	}
	if report.Lock.NoticesFileMode == 0 {
		report.Lock.NoticesFileMode = noticeMode
	}
	report.Lock.Receipts = replaceProjectNoticeReceiptProjection(report.Lock.Receipts, pack.ID, request.Surface, renderProjectNoticeBlock(report))
	report.noticeContent, report.noticeMode, report.noticeBefore, report.noticeIntact = noticeContent, noticeMode, noticeBefore, noticeIntact
	report.Blockers = append(report.Blockers, noticeBlockers...)
	if len(noticeBlockers) > 0 {
		report.Disposition = ProjectInstallBlocked
	}
	if len(blockers) == 0 {
		allowRefresh := request.update || existingContract && (!priorFound || !projectSupportsSurface(prior.Surfaces, request.Surface))
		converged, contractBlockers, err := inspectProjectContract(report, existingLock, lockExists, allowRefresh)
		if err != nil {
			return JSONProjectInstallPreview{}, err
		}
		report.Blockers = append(report.Blockers, contractBlockers...)
		if len(contractBlockers) > 0 {
			report.Disposition = ProjectInstallBlocked
		} else if converged {
			report.Disposition = ProjectInstallConverged
		}
	}
	sort.Slice(report.Blockers, func(i, j int) bool {
		if report.Blockers[i].Code != report.Blockers[j].Code {
			return report.Blockers[i].Code < report.Blockers[j].Code
		}
		if report.Blockers[i].Target != report.Blockers[j].Target {
			return report.Blockers[i].Target < report.Blockers[j].Target
		}
		return report.Blockers[i].Resource.String() < report.Blockers[j].Resource.String()
	})
	report.Observation = sealProjectInstallPreview(report, observationDigest(observation)+"\nnotices="+noticeBefore)
	return report, nil
}

func expectedProjectReadinessProjections(plans []ProjectProjectionPlan) []ProjectionStatus {
	result := make([]ProjectionStatus, 0, len(plans))
	for _, plan := range plans {
		result = append(result, ProjectionStatus{ID: plan.Resource.String(), Health: ProjectionVerified})
	}
	return result
}

func normalizeProjectProjectionFingerprint(value string) string {
	return strings.TrimPrefix(value, "sha256:")
}

func projectCompositionBlockers(values []PlanBlocker) []ProjectInstallBlocker {
	result := make([]ProjectInstallBlocker, 0, len(values))
	for _, blocker := range values {
		code := "project_dependency"
		remediation := "repair the admitted project pack graph before installation"
		switch {
		case blocker.Kind == BlockerAlias:
			code = "native_name_collision"
			remediation = "supply an explicit valid --alias for one colliding resource"
		case blocker.Kind == BlockerCompatibility:
			code = "unrepresentable_resource"
			remediation = "choose a surface whose native binding or declared degradation represents the complete closure"
		case blocker.Kind == BlockerSharing || blocker.Kind == BlockerIncompatibleContribution || blocker.Kind == BlockerResourceConflict:
			code = "resource_conflict"
			remediation = "repair the conflicting admitted resource contracts before installation"
		}
		result = append(result, ProjectInstallBlocker{Code: code, Detail: blocker.Subject + ": " + blocker.Detail, Remediation: remediation})
	}
	return result
}

func selectProjectPackResources(pack Pack, selection ResourceSelection) (Pack, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return Pack{}, err
	}
	if selection.Mode == SelectionAll {
		return clonePack(pack), nil
	}
	return selectPackResourceClosure(pack, selection)
}

func (f Facade) resolveProjectPackUnlocked(id string) (Pack, error) {
	return f.catalog.showUnlocked(id)
}

func projectRequirements(pack Pack) []string {
	values := make([]string, 0)
	for _, tool := range pack.Requires.Tools {
		values = append(values, "tool:"+tool)
	}
	for _, resource := range pack.Resources {
		for _, tool := range resource.RequiresTools {
			values = append(values, "tool:"+tool)
		}
		for _, mode := range resource.RuntimeModes {
			for _, requirement := range mode.Requirements {
				values = append(values, string(requirement.Kind)+":"+requirement.ID)
			}
		}
	}
	return sortedUnique(values)
}

func projectSupportsSurface(surfaces []Surface, target Surface) bool {
	for _, surface := range surfaces {
		if surface == target {
			return true
		}
	}
	return false
}

func sortedProjectSurfaces(values []Surface) []Surface {
	seen := map[Surface]bool{}
	result := make([]Surface, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func projectSurfaceIntents(pack ProjectManifestPack) []ProjectSurfaceIntent {
	return append([]ProjectSurfaceIntent(nil), pack.SurfaceIntents...)
}

func projectReceipt(pack Pack, surface Surface, selection ResourceSelection, aliases []SurfaceAlias, graph ResourceGraph, projections []ProjectProjectionPlan) installedPackReceipt {
	receipt := installedPackReceipt{
		Pack: installedPackIdentity{ID: pack.ID, Version: pack.Version}, Surface: surface,
		ReadinessObligations: append([]ReadinessObligation(nil), pack.ReadinessObligations...),
		ExternalRequirements: append([]string{}, pack.Requires.Tools...),
		Selection:            cloneSelection(selection), Aliases: cloneAliases(aliases), Resources: []ResourceIdentity{}, Projections: []installedProjection{},
	}
	for _, fact := range graph.Resources {
		receipt.Resources = append(receipt.Resources, fact.Resource)
	}
	selected := make(map[ResourceIdentity]bool, len(receipt.Resources))
	for _, resource := range receipt.Resources {
		selected[resource] = true
	}
	for _, projection := range projections {
		if !projectProjectionOwnedByPack(projection, pack.ID) {
			continue
		}
		if !selected[projection.Resource] {
			receipt.Resources = append(receipt.Resources, projection.Resource)
			selected[projection.Resource] = true
		}
		receipt.Projections = append(receipt.Projections, installedProjection{
			ID: projection.Resource.String(), Target: projection.Target, Digest: projection.DesiredFingerprint,
		})
	}
	sort.Slice(receipt.Resources, func(i, j int) bool { return receipt.Resources[i].String() < receipt.Resources[j].String() })
	for _, disclosure := range projectSensitiveDisclosures(pack, surface) {
		if selected[disclosure.Resource] || disclosure.Resource == (ResourceIdentity{Kind: "pack", ID: pack.ID}) {
			receipt.Sensitive = append(receipt.Sensitive, disclosure)
		}
	}
	return receipt
}

func projectNoticeProjectionID(packID string, surface Surface) string {
	return "notice:pack-" + packID + "-" + string(surface)
}

func replaceProjectNoticeReceiptProjection(receipts []installedPackReceipt, packID string, surface Surface, block string) []installedPackReceipt {
	result := append([]installedPackReceipt(nil), receipts...)
	for i := range result {
		if result[i].Pack.ID != packID || result[i].Surface != surface {
			continue
		}
		projections := make([]installedProjection, 0, len(result[i].Projections)+1)
		for _, projection := range result[i].Projections {
			if projection.ID != projectNoticeProjectionID(packID, surface) {
				projections = append(projections, projection)
			}
		}
		projections = append(projections, installedProjection{
			ID: projectNoticeProjectionID(packID, surface), Target: "PACKY-NOTICES.md", Digest: fingerprintProjectBytes([]byte(block)),
		})
		sort.Slice(projections, func(left, right int) bool {
			leftKey := filepath.Clean(projections[left].Target) + "\x00" + projections[left].ID
			rightKey := filepath.Clean(projections[right].Target) + "\x00" + projections[right].ID
			return leftKey < rightKey
		})
		result[i].Projections = projections
		break
	}
	return result
}

func projectNoticeReceiptProjection(receipts []installedPackReceipt, packID string, surface Surface) (installedProjection, bool) {
	for _, receipt := range receipts {
		if receipt.Pack.ID != packID || receipt.Surface != surface {
			continue
		}
		for _, projection := range receipt.Projections {
			if projection.ID == projectNoticeProjectionID(packID, surface) {
				return projection, true
			}
		}
	}
	return installedProjection{}, false
}

func replaceProjectReceipt(receipts []installedPackReceipt, replacement installedPackReceipt) []installedPackReceipt {
	result := make([]installedPackReceipt, 0, len(receipts)+1)
	for _, receipt := range receipts {
		if receipt.Pack.ID == replacement.Pack.ID && receipt.Surface == replacement.Surface {
			continue
		}
		result = append(result, receipt)
	}
	result = append(result, replacement)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Pack.ID != result[j].Pack.ID {
			return result[i].Pack.ID < result[j].Pack.ID
		}
		return result[i].Surface < result[j].Surface
	})
	return result
}

func projectReceiptProjectionMode(surface Surface, resource ResourceIdentity) (string, uint32) {
	switch resource.Kind {
	case "notice", "instruction":
		return "merge_marked_file", 0o644
	case "mcp_server":
		if surface == SurfaceCodex {
			return "merge_marked_file", 0o644
		}
		return "merge_structured_file", 0o644
	case "skill":
		if surface == SurfaceCodex || surface == SurfaceClaude {
			return "copy_tree", 0o700
		}
	}
	return "copy_file", 0o644
}

func hydrateProjectLock(lock ProjectLockProposal) (ProjectLockProposal, error) {
	lock.Path = "packy.lock.json"
	if len(lock.Receipts) == 0 {
		return ProjectLockProposal{}, errors.New("project lock contains no installed Pack receipts")
	}
	if err := validateProjectReceipts(lock.Receipts); err != nil {
		return ProjectLockProposal{}, err
	}
	resources := map[ResourceIdentity]bool{}
	lock.ResourceGraph = ResourceGraph{Resources: []ResourceClosureFact{}}
	lock.Projections = []ProjectProjectionPlan{}
	lock.Bindings = []LifecycleBinding{}
	lock.Degradations = []LifecycleExclusion{}
	lock.Modes = []OptionalMode{}
	lock.Sensitive = []ProjectSensitiveDisclosure{}
	for _, receipt := range lock.Receipts {
		if receipt.Pack.ID == "" || receipt.Pack.Version == "" || receipt.Surface == "" || receipt.Resources == nil || receipt.Projections == nil {
			return ProjectLockProposal{}, errors.New("project lock contains an incomplete installed Pack receipt")
		}
		lock.Sensitive = mergeProjectSensitiveDisclosures(lock.Sensitive, receipt.Sensitive)
		for _, resource := range receipt.Resources {
			resources[resource] = true
		}
		for _, projection := range receipt.Projections {
			if projection.ID == projectNoticeProjectionID(receipt.Pack.ID, receipt.Surface) {
				if lock.NoticesFileMode == 0 {
					lock.NoticesFileMode = 0o644
					lock.NoticesSHA256 = projection.Digest
				}
				continue
			}
			resource, err := ParseResourceIdentity(projection.ID)
			if err != nil {
				return ProjectLockProposal{}, fmt.Errorf("project receipt projection identity: %w", err)
			}
			mode, fileMode := projectReceiptProjectionMode(receipt.Surface, resource)
			lock.Projections = append(lock.Projections, ProjectProjectionPlan{
				Resource: resource, Target: projection.Target, Mode: mode, FileMode: fileMode,
				DesiredFingerprint: projection.Digest, ObservedState: "installed", OwnerPack: receipt.Pack.ID, Surface: receipt.Surface,
			})
			name := resource.ID
			for _, alias := range receipt.Aliases {
				if alias.Kind == resource.Kind && alias.ID == resource.ID {
					name = alias.Name
					break
				}
			}
			lock.Bindings = append(lock.Bindings, LifecycleBinding{Surface: receipt.Surface, Kind: resource.Kind, ID: resource.ID, Projection: resource.Kind, Name: name, Invocation: name, Mode: "native", Sharing: "exclusive"})
		}
	}
	lock.Projections, _ = mergeProjectProjections(nil, lock.Projections, nil)
	facts := make([]ResourceClosureFact, 0, len(resources))
	for resource := range resources {
		facts = append(facts, ResourceClosureFact{Resource: resource, Role: ResourceRoleRoot, DependencyChain: []ResourceIdentity{resource}, Requires: []ResourceIdentity{}, Notices: []ResourceIdentity{}})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].Resource.String() < facts[j].Resource.String() })
	lock.ResourceGraph = ResourceGraph{Resources: facts}
	sort.Slice(lock.Projections, func(i, j int) bool {
		if lock.Projections[i].Target != lock.Projections[j].Target {
			return lock.Projections[i].Target < lock.Projections[j].Target
		}
		return lock.Projections[i].Resource.String() < lock.Projections[j].Resource.String()
	})
	return lock, nil
}

func validateProjectReceipts(receipts []installedPackReceipt) error {
	seen := map[string]bool{}
	targetOwners := map[string]string{}
	for i, receipt := range receipts {
		key := receipt.Pack.ID + "\x00" + string(receipt.Surface)
		if !idPattern.MatchString(receipt.Pack.ID) || !semverPattern.MatchString(receipt.Pack.Version) || receipt.Surface != SurfaceCodex && receipt.Surface != SurfaceOpenCode && receipt.Surface != SurfaceClaude || seen[key] {
			return errors.New("project lock contains an invalid or duplicate Pack receipt")
		}
		if i > 0 {
			prior := receipts[i-1].Pack.ID + "\x00" + string(receipts[i-1].Surface)
			if prior >= key {
				return errors.New("project lock Pack receipts are not canonically sorted")
			}
		}
		seen[key] = true
		if len(receipt.ExternalEffects) != 0 {
			return fmt.Errorf("project receipt for %s on %s contains personal external effects", receipt.Pack.ID, receipt.Surface)
		}
		if !validReadinessObligations(receipt.ReadinessObligations) {
			return fmt.Errorf("project receipt for %s on %s has invalid readiness obligations", receipt.Pack.ID, receipt.Surface)
		}
		if receipt.ExternalRequirements == nil || !sort.StringsAreSorted(receipt.ExternalRequirements) || hasDuplicateStrings(receipt.ExternalRequirements) {
			return fmt.Errorf("project receipt for %s on %s has invalid external requirements", receipt.Pack.ID, receipt.Surface)
		}
		for _, requirement := range receipt.ExternalRequirements {
			if !idPattern.MatchString(requirement) {
				return fmt.Errorf("project receipt for %s on %s has invalid external requirements", receipt.Pack.ID, receipt.Surface)
			}
		}
		selection, err := canonicalSelection(receipt.Selection)
		aliases := cloneAliases(receipt.Aliases)
		aliasErr := canonicalizeAliases(&aliases)
		if err != nil || receipt.Selection.Roots == nil || digestJSON(selection) != digestJSON(receipt.Selection) || aliasErr != nil || receipt.Aliases != nil && digestJSON(aliases) != digestJSON(receipt.Aliases) || receipt.Resources == nil || receipt.Projections == nil {
			return fmt.Errorf("project receipt for %s on %s is incomplete or non-canonical", receipt.Pack.ID, receipt.Surface)
		}
		resourceSeen := map[ResourceIdentity]bool{}
		for j, resource := range receipt.Resources {
			if resource.Kind == "" || resource.ID == "" || resourceSeen[resource] || j > 0 && receipt.Resources[j-1].String() >= resource.String() {
				return fmt.Errorf("project receipt for %s on %s has invalid resources", receipt.Pack.ID, receipt.Surface)
			}
			resourceSeen[resource] = true
		}
		if receipt.Sensitive != nil && digestJSON(receipt.Sensitive) != digestJSON(deduplicateProjectSensitiveDisclosures(receipt.Sensitive)) {
			return fmt.Errorf("project receipt for %s on %s has non-canonical activation disclosures", receipt.Pack.ID, receipt.Surface)
		}
		for _, disclosure := range receipt.Sensitive {
			packRequirement := disclosure.Resource == (ResourceIdentity{Kind: "pack", ID: receipt.Pack.ID})
			if disclosure.Surface != receipt.Surface || disclosure.Detail == "" || !validProjectActivationCategory(disclosure.Category) || !resourceSeen[disclosure.Resource] && !packRequirement {
				return fmt.Errorf("project receipt for %s on %s has invalid activation disclosure", receipt.Pack.ID, receipt.Surface)
			}
		}
		projectionSeen := map[string]bool{}
		for j, projection := range receipt.Projections {
			projectionKey := filepath.Clean(projection.Target) + "\x00" + projection.ID
			identity, identityErr := ParseResourceIdentity(projection.ID)
			noticeProjection := projection.ID == projectNoticeProjectionID(receipt.Pack.ID, receipt.Surface) && projection.Target == "PACKY-NOTICES.md"
			if identityErr != nil || !noticeProjection && !resourceSeen[identity] || !safeProjectContractTarget(projection.Target) || !projectDigestPattern.MatchString(projection.Digest) || projectionSeen[projectionKey] {
				return fmt.Errorf("project receipt for %s on %s has invalid projection evidence", receipt.Pack.ID, receipt.Surface)
			}
			if j > 0 {
				prior := filepath.Clean(receipt.Projections[j-1].Target) + "\x00" + receipt.Projections[j-1].ID
				if prior >= projectionKey {
					return fmt.Errorf("project receipt for %s on %s has unsorted projections", receipt.Pack.ID, receipt.Surface)
				}
			}
			cleanTarget := filepath.Clean(projection.Target)
			if owner, occupied := targetOwners[cleanTarget]; occupied && owner != receipt.Pack.ID && !noticeProjection {
				return fmt.Errorf("project receipts for Packs %s and %s collide at %s", owner, receipt.Pack.ID, projection.Target)
			}
			targetOwners[cleanTarget] = receipt.Pack.ID
			projectionSeen[projectionKey] = true
		}
	}
	return nil
}

func mergeProjectReceiptSelections(left, right ResourceSelection) ResourceSelection {
	if left.Mode == SelectionAll || right.Mode == SelectionAll {
		return ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
	}
	selection, _ := canonicalSelection(ResourceSelection{Mode: SelectionCustom, Roots: append(append([]ResourceIdentity(nil), left.Roots...), right.Roots...)})
	return selection
}

func findProjectManifestPack(packs []ProjectManifestPack, packID string) (ProjectManifestPack, bool) {
	for _, pack := range packs {
		if pack.ID == packID {
			return pack, true
		}
	}
	return ProjectManifestPack{}, false
}

func replaceProjectManifestPack(packs []ProjectManifestPack, replacement ProjectManifestPack) []ProjectManifestPack {
	result := append([]ProjectManifestPack(nil), packs...)
	replaced := false
	for i := range result {
		if result[i].ID == replacement.ID {
			result[i] = replacement
			replaced = true
			break
		}
	}
	if !replaced {
		result = append(result, replacement)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func withProjectSurfaceIntent(pack ProjectManifestPack, intent ProjectSurfaceIntent) ProjectManifestPack {
	intent.Selection, _ = canonicalSelection(intent.Selection)
	if intent.Aliases == nil {
		intent.Aliases = []SurfaceAlias{}
	}
	intents := projectSurfaceIntents(pack)
	replaced := false
	for i := range intents {
		if intents[i].Surface == intent.Surface {
			intents[i] = intent
			replaced = true
		}
	}
	if !replaced {
		intents = append(intents, intent)
	}
	sort.Slice(intents, func(i, j int) bool { return intents[i].Surface < intents[j].Surface })
	pack.SurfaceIntents = intents
	return deriveProjectManifestPack(pack)
}

func deriveProjectManifestPack(pack ProjectManifestPack) ProjectManifestPack {
	intents := append([]ProjectSurfaceIntent(nil), pack.SurfaceIntents...)
	sort.Slice(intents, func(i, j int) bool { return intents[i].Surface < intents[j].Surface })
	pack.SurfaceIntents = intents
	pack.Version = ""
	pack.Surfaces = make([]Surface, 0, len(intents))
	combined := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{}}
	for _, value := range intents {
		if pack.Version == "" || projectVersionGreater(value.Version, pack.Version) {
			pack.Version = value.Version
		}
		pack.Surfaces = append(pack.Surfaces, value.Surface)
		if value.Selection.Mode == SelectionAll {
			combined = ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
		} else if combined.Mode != SelectionAll {
			combined.Roots = append(combined.Roots, value.Selection.Roots...)
		}
	}
	pack.Selection, _ = canonicalSelection(combined)
	pack.Aliases = []SurfaceAlias{}
	if len(intents) > 0 {
		pack.Aliases = cloneAliases(intents[0].Aliases)
	}
	return pack
}

func projectVersionGreater(left, right string) bool {
	leftVersion, leftErr := semver.StrictNewVersion(left)
	rightVersion, rightErr := semver.StrictNewVersion(right)
	if leftErr != nil || rightErr != nil {
		return left > right
	}
	return leftVersion.GreaterThan(rightVersion)
}

func withoutProjectSurfaceIntent(pack ProjectManifestPack, surface Surface) ProjectManifestPack {
	intents := projectSurfaceIntents(pack)
	kept := intents[:0]
	for _, intent := range intents {
		if intent.Surface != surface {
			kept = append(kept, intent)
		}
	}
	pack.SurfaceIntents = nil
	pack.Surfaces = nil
	if len(kept) == 0 {
		return pack
	}
	pack.SurfaceIntents = append([]ProjectSurfaceIntent(nil), kept...)
	return deriveProjectManifestPack(pack)
}

func mergeProjectResourceGraphs(existing, added ResourceGraph) ResourceGraph {
	byResource := make(map[ResourceIdentity]ResourceClosureFact, len(existing.Resources)+len(added.Resources))
	for _, fact := range existing.Resources {
		byResource[fact.Resource] = fact
	}
	for _, fact := range added.Resources {
		byResource[fact.Resource] = fact
	}
	resources := make([]ResourceClosureFact, 0, len(byResource))
	for _, fact := range byResource {
		resources = append(resources, fact)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Resource.String() < resources[j].Resource.String() })
	return ResourceGraph{Resources: resources}
}

func mergeProjectBindings(existing, added []LifecycleBinding) []LifecycleBinding {
	result := append([]LifecycleBinding{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[digestJSON(value)] = true
	}
	for _, value := range added {
		if !seen[digestJSON(value)] {
			result = append(result, value)
			seen[digestJSON(value)] = true
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		if result[i].Projection != result[j].Projection {
			return result[i].Projection < result[j].Projection
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func mergeProjectDegradations(existing, added []LifecycleExclusion) []LifecycleExclusion {
	result := append([]LifecycleExclusion(nil), existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[digestJSON(value)] = true
	}
	for _, value := range added {
		if !seen[digestJSON(value)] {
			result = append(result, value)
			seen[digestJSON(value)] = true
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Surface < result[j].Surface
	})
	return result
}

func mergeProjectModes(existing, added []OptionalMode) []OptionalMode {
	result := append([]OptionalMode{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[value.ID] = true
	}
	for _, value := range added {
		if !seen[value.ID] {
			result = append(result, value)
			seen[value.ID] = true
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func mergeProjectSensitiveDisclosures(existing, added []ProjectSensitiveDisclosure) []ProjectSensitiveDisclosure {
	return deduplicateProjectSensitiveDisclosures(append(append([]ProjectSensitiveDisclosure{}, existing...), added...))
}

func mergeProjectProjections(existing, added, preview []ProjectProjectionPlan) ([]ProjectProjectionPlan, []ProjectProjectionPlan) {
	result := append([]ProjectProjectionPlan(nil), existing...)
	for i := range added {
		match := -1
		for j := range result {
			if result[j].Surface == added[i].Surface && result[j].Resource == added[i].Resource && filepath.Clean(result[j].Target) == filepath.Clean(added[i].Target) {
				match = j
				break
			}
		}
		if match < 0 {
			result = append(result, added[i])
			continue
		}
		result[match] = added[i]
		for j := range preview {
			if preview[j].Resource == added[i].Resource && filepath.Clean(preview[j].Target) == filepath.Clean(added[i].Target) {
				preview[j].OwnerPack = added[i].OwnerPack
				preview[j].Surface = added[i].Surface
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Target != result[j].Target {
			return result[i].Target < result[j].Target
		}
		if result[i].Resource != result[j].Resource {
			return result[i].Resource.String() < result[j].Resource.String()
		}
		return result[i].Surface < result[j].Surface
	})
	return result, preview
}

func sealProjectInstallPreview(report JSONProjectInstallPreview, surfaceObservation string) string {
	report.Observation = ""
	report.ExpectedReadiness, report.Conditions = ReadinessStatus{}, nil
	data, _ := json.Marshal(struct {
		Report             JSONProjectInstallPreview
		SurfaceObservation string
	}{Report: report, SurfaceObservation: surfaceObservation})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func DiscoverProjectRoot(currentDirectory string) (string, error) {
	if currentDirectory == "" {
		return "", errors.New("current directory is unavailable")
	}
	absolute, err := filepath.Abs(currentDirectory)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	for candidate := filepath.Clean(canonical); ; candidate = filepath.Dir(candidate) {
		marker := filepath.Join(candidate, ".git")
		info, statErr := os.Lstat(marker)
		if statErr == nil {
			valid, markerErr := validGitWorktreeMarker(candidate, marker, info)
			if markerErr != nil {
				return "", markerErr
			}
			if valid {
				return candidate, nil
			}
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect Git worktree marker: %w", statErr)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return "", ProjectNotFoundError{CurrentDirectory: currentDirectory}
}

// ProjectNotFoundError means the current directory has no enclosing Git
// worktree. Callers may treat this expected context absence differently from
// an unreadable or malformed worktree.
type ProjectNotFoundError struct {
	CurrentDirectory string
}

func (e ProjectNotFoundError) Error() string {
	return fmt.Sprintf("current directory %q is outside a Git worktree", e.CurrentDirectory)
}

func validGitWorktreeMarker(root, marker string, info os.FileInfo) (bool, error) {
	if info.IsDir() {
		return validGitDirectory(marker)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return false, fmt.Errorf("read Git worktree marker: %w", err)
	}
	value := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(value, prefix) {
		return false, nil
	}
	gitDirectory := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if !filepath.IsAbs(gitDirectory) {
		gitDirectory = filepath.Join(root, gitDirectory)
	}
	return validGitDirectory(gitDirectory)
}

func validGitDirectory(gitDirectory string) (bool, error) {
	head, err := os.ReadFile(filepath.Join(gitDirectory, "HEAD"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read Git worktree HEAD: %w", err)
	}
	if strings.TrimSpace(string(head)) == "" {
		return false, nil
	}
	commonDirectory := gitDirectory
	commondir, err := os.ReadFile(filepath.Join(gitDirectory, "commondir"))
	if err == nil {
		commonDirectory = strings.TrimSpace(string(commondir))
		if commonDirectory == "" {
			return false, nil
		}
		if !filepath.IsAbs(commonDirectory) {
			commonDirectory = filepath.Join(gitDirectory, commonDirectory)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("read linked Git common directory: %w", err)
	}
	for _, name := range []string{"objects", "refs"} {
		info, err := os.Stat(filepath.Join(commonDirectory, name))
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("inspect Git %s directory: %w", name, err)
		}
		if !info.IsDir() {
			return false, nil
		}
	}
	return true, nil
}

func RelativeProjectTarget(projectRoot, target string) (string, error) {
	relative, err := filepath.Rel(projectRoot, target)
	if err != nil {
		return "", err
	}
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project target %q escapes the project root", target)
	}
	return filepath.Clean(relative), nil
}

func validateProjectTargetPath(projectRoot, target string) error {
	relative, err := RelativeProjectTarget(projectRoot, target)
	if err != nil {
		return err
	}
	current := filepath.Clean(projectRoot)
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("project target ancestor %q is not a real directory", current)
		}
	}
	if info, err := os.Lstat(filepath.Clean(target)); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("project target %q is an unsafe filesystem object", target)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
