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

const ProjectUninstallPreviewSchemaVersion = 3

type ProjectUninstallRequest struct {
	PackID      string
	Surface     Surface
	ProjectRoot string
}

type ProjectUninstallScope string

const (
	ProjectUninstallSurface ProjectUninstallScope = "surface"
	ProjectUninstallPack    ProjectUninstallScope = "pack"
)

type JSONProjectUninstallPreview struct {
	SchemaVersion int                       `json:"schema_version"`
	Report        string                    `json:"report"`
	DryRun        bool                      `json:"dry_run"`
	ProjectRoot   string                    `json:"project_root"`
	Pack          ProjectManifestPack       `json:"pack"`
	Surface       Surface                   `json:"surface,omitempty"`
	Scope         ProjectUninstallScope     `json:"scope"`
	Projections   []ProjectProjectionStatus `json:"projections"`
	Contracts     []string                  `json:"contracts"`
	Blockers      []ProjectInstallBlocker   `json:"blockers"`
	Disposition   ProjectInstallDisposition `json:"disposition"`
	Observation   string                    `json:"observation"`
	projectRoot   string
	request       ProjectUninstallRequest
	actions       []ProjectionAction
}

type ProjectUninstallApplyRequest struct {
	Preview   JSONProjectUninstallPreview
	PackyHome string
	Adapter   SurfaceAdapter
}

type ProjectUninstallApplyResult struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Status        string `json:"status"`
	Observation   string `json:"observation"`
}

type ProjectUninstallNotActionableError struct{ Disposition ProjectInstallDisposition }

func (e ProjectUninstallNotActionableError) Error() string {
	return fmt.Sprintf("project uninstall preview is not actionable: %s", e.Disposition)
}

func PreviewProjectUninstall(ctx context.Context, request ProjectUninstallRequest, adapter SurfaceAdapter) (JSONProjectUninstallPreview, error) {
	report := JSONProjectUninstallPreview{
		SchemaVersion: ProjectUninstallPreviewSchemaVersion, Report: "project-uninstall-preview", DryRun: true,
		ProjectRoot: "<project-root>", projectRoot: request.ProjectRoot, request: request,
		Contracts: []string{"PACKY-NOTICES.md", "packy.json", "packy.lock.json"}, Blockers: []ProjectInstallBlocker{},
	}
	if request.ProjectRoot == "" || request.PackID == "" || adapter == nil {
		return report, errors.New("project uninstall preview requires the project root, pack, and adapter")
	}
	installation, err := LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return report, err
	}
	pack, installed := findProjectManifestPack(installation.Manifest.Packs, request.PackID)
	if !installed {
		return report, fmt.Errorf("capability pack %q is not declared by this project installation", request.PackID)
	}
	report.Pack = pack
	if request.Surface == "" {
		report.Scope = ProjectUninstallPack
	} else {
		if !projectSupportsSurface(pack.Surfaces, request.Surface) {
			return report, fmt.Errorf("capability pack %q is not installed on CLI surface %q", request.PackID, request.Surface)
		}
		report.Scope, report.Surface = ProjectUninstallSurface, request.Surface
	}
	surface := request.Surface
	if surface == "" {
		surface = pack.Surfaces[0]
	}
	status, err := InspectProjectStatus(ctx, ProjectStatusRequest{
		ProjectRoot: request.ProjectRoot, PackID: pack.ID, Surface: surface, RequireInstalled: true,
		Adapters: map[Surface]SurfaceAdapter{surface: adapter},
	})
	if err != nil {
		return report, err
	}
	if len(status.Packs) != 1 {
		return report, errors.New("project uninstall could not identify one exact installed surface")
	}
	report.Projections = append([]ProjectProjectionStatus(nil), status.Packs[0].Projections...)
	report.Blockers = append(report.Blockers, status.Packs[0].Blockers...)
	scopedInstallation := projectInstallationForPack(installation, pack.ID)
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{
		ProjectRoot: request.ProjectRoot, ProjectInstallation: &scopedInstallation, ProjectGoal: ProjectionAbsent,
	})
	if err != nil {
		return report, err
	}
	uninstallStatuses, err := projectProjectionStatusesFromObservation(request.ProjectRoot, scopedInstallation.Lock, observation, surface, pack.ID)
	if err != nil {
		return report, err
	}
	if !sameProjectProjectionStatuses(report.Projections, uninstallStatuses) {
		return report, errors.New("project uninstall adapter returned inconsistent portable ownership evidence")
	}
	actions := make([]ProjectionAction, 0, len(observation.Projections)+3)
	remainingSurfaces := removeProjectSurface(pack.Surfaces, surface)
	retainedLock := installation.Lock
	retainedLock.Projections = withoutProjectSurface(retainedLock.Projections, surface)
	retainedTargets := map[string]bool{}
	for _, projection := range retainedLock.Projections {
		retainedTargets[projection.Resource.String()+"\x00"+filepath.Clean(projection.Target)] = true
	}
	for _, projection := range observation.Projections {
		target, targetErr := RelativeProjectTarget(request.ProjectRoot, projection.Action.Target)
		if targetErr != nil {
			return report, targetErr
		}
		if !retainedTargets[projection.ID+"\x00"+filepath.Clean(target)] {
			actions = append(actions, projection.Action)
		}
	}
	actions, err = coalesceProjectRemovalActions(actions)
	if err != nil {
		return report, err
	}
	if report.Scope == ProjectUninstallSurface && len(remainingSurfaces) > 0 {
		pack = withoutProjectSurfaceIntent(pack, surface)
		noticeAction, noticeBlockers, noticeErr := planProjectNoticeRemoval(request.ProjectRoot, pack, request.Surface, installation.Lock)
		if noticeErr != nil {
			return report, noticeErr
		}
		report.Blockers = append(report.Blockers, noticeBlockers...)
		if len(noticeBlockers) == 0 {
			actions = append(actions, noticeAction)
		}
		retainedLock.Receipts = removeProjectReceipt(retainedLock.Receipts, pack.ID, surface)
		retainedLock, err = hydrateProjectLock(retainedLock)
		if err != nil {
			return report, err
		}
		retainedLock = filterProjectLockMetadataToReceipts(retainedLock)
		report.Pack = pack
		manifest := installation.Manifest
		manifest.Packs = replaceProjectManifestPack(manifest.Packs, pack)
		manifestData, marshalErr := marshalProjectManifest(manifest)
		if marshalErr != nil {
			return report, marshalErr
		}
		retainedLock.ManifestSHA256 = fingerprintProjectBytes(manifestData)
		lockData, marshalErr := marshalProjectLock(retainedLock)
		if marshalErr != nil {
			return report, marshalErr
		}
		manifestPath := filepath.Join(request.ProjectRoot, "packy.json")
		lockPath := filepath.Join(request.ProjectRoot, "packy.lock.json")
		actions = append(actions,
			ProjectionAction{ID: "project-contract:manifest", Kind: ActionProjectManifestFile, Target: manifestPath, Content: string(manifestData), FileMode: 0o644, Precondition: projectTargetFingerprint(manifestPath), Description: "remove the selected project surface intent"},
			ProjectionAction{ID: "project-contract:lock", Kind: ActionProjectLockFile, Target: lockPath, Content: string(lockData), FileMode: 0o644, Precondition: projectTargetFingerprint(lockPath), Description: "publish the remaining project receipts"},
		)
		report.Contracts = []string{"PACKY-NOTICES.md", "packy.json", "packy.lock.json"}
		for _, projection := range report.Projections {
			if projection.Health != "verified" {
				report.Blockers = append(report.Blockers, ProjectInstallBlocker{Code: "project_drift", Resource: projection.Resource, Target: projection.Target, Detail: "the installed project projection is " + projection.Health, Remediation: "restore the exact locked projection before uninstalling"})
			}
		}
		report.Blockers = deduplicateProjectUninstallBlockers(report.Blockers)
		report.Disposition = ProjectInstallPreviewable
		if len(report.Blockers) > 0 || !status.Packs[0].RequirementSatisfied {
			report.Disposition = ProjectInstallBlocked
		}
		report.actions = actions
		report.Observation = sealProjectUninstallPreview(report)
		return report, nil
	}
	noticeAction, noticeBlockers, err := planProjectNoticeRemoval(request.ProjectRoot, pack, request.Surface, installation.Lock)
	if err != nil {
		return report, err
	}
	report.Blockers = append(report.Blockers, noticeBlockers...)
	if len(noticeBlockers) == 0 {
		actions = append(actions, noticeAction)
	}
	manifestPath := filepath.Join(request.ProjectRoot, "packy.json")
	lockPath := filepath.Join(request.ProjectRoot, "packy.lock.json")
	if len(installation.Manifest.Packs) > 1 {
		manifest := installation.Manifest
		manifest.Packs = removeProjectManifestPack(manifest.Packs, pack.ID)
		manifestData, marshalErr := marshalProjectManifest(manifest)
		if marshalErr != nil {
			return report, marshalErr
		}
		retainedLock, retainErr := projectLockWithoutPack(installation.Lock, pack.ID)
		if retainErr != nil {
			return report, retainErr
		}
		retainedLock.ManifestSHA256 = fingerprintProjectBytes(manifestData)
		lockData, marshalErr := marshalProjectLock(retainedLock)
		if marshalErr != nil {
			return report, marshalErr
		}
		actions = append(actions,
			ProjectionAction{ID: "project-contract:manifest", Kind: ActionProjectManifestFile, Target: manifestPath, Content: string(manifestData), FileMode: 0o644, Precondition: projectTargetFingerprint(manifestPath), Description: "remove one direct Pack from the project manifest"},
			ProjectionAction{ID: "project-contract:lock", Kind: ActionProjectLockFile, Target: lockPath, Content: string(lockData), FileMode: 0o644, Precondition: projectTargetFingerprint(lockPath), Description: "publish the remaining independent Pack receipts"},
		)
		report.Contracts = []string{"PACKY-NOTICES.md", "packy.json", "packy.lock.json"}
		for _, projection := range report.Projections {
			if projection.Health != "verified" {
				report.Blockers = append(report.Blockers, ProjectInstallBlocker{Code: "project_drift", Resource: projection.Resource, Target: projection.Target, Detail: "the installed project projection is " + projection.Health, Remediation: "restore the exact locked projection before uninstalling"})
			}
		}
		report.Blockers = deduplicateProjectUninstallBlockers(report.Blockers)
		report.Disposition = ProjectInstallPreviewable
		if len(report.Blockers) > 0 || !status.Packs[0].RequirementSatisfied {
			report.Disposition = ProjectInstallBlocked
		}
		report.actions = actions
		report.Observation = sealProjectUninstallPreview(report)
		return report, nil
	}
	actions = append(actions,
		ProjectionAction{ID: "project-contract:manifest", Kind: ActionProjectManifestFile, Target: manifestPath, Mode: ProjectionDeleteTarget, Precondition: projectTargetFingerprint(manifestPath), Description: "remove the project pack manifest"},
		ProjectionAction{ID: "project-contract:lock", Kind: ActionProjectLockFile, Target: lockPath, Mode: ProjectionDeleteTarget, Precondition: projectTargetFingerprint(lockPath), Description: "remove the project pack lock after all owned projections verify absent"},
	)
	for _, projection := range report.Projections {
		if projection.Health != "verified" {
			report.Blockers = append(report.Blockers, ProjectInstallBlocker{Code: "project_drift", Resource: projection.Resource, Target: projection.Target, Detail: "the installed project projection is " + projection.Health, Remediation: "restore the exact locked projection before uninstalling"})
		}
	}
	report.Blockers = deduplicateProjectUninstallBlockers(report.Blockers)
	report.Disposition = ProjectInstallPreviewable
	if len(report.Blockers) > 0 || !status.Packs[0].RequirementSatisfied {
		report.Disposition = ProjectInstallBlocked
	}
	report.actions = actions
	report.Observation = sealProjectUninstallPreview(report)
	return report, nil
}

func removeProjectReceipt(receipts []installedPackReceipt, packID string, surface Surface) []installedPackReceipt {
	result := make([]installedPackReceipt, 0, len(receipts)-1)
	for _, receipt := range receipts {
		if receipt.Pack.ID != packID || receipt.Surface != surface {
			result = append(result, receipt)
		}
	}
	return result
}

func filterProjectLockMetadataToReceipts(lock ProjectLockProposal) ProjectLockProposal {
	owns := func(resource ResourceIdentity, surface Surface) bool {
		for _, receipt := range lock.Receipts {
			if receipt.Surface != surface {
				continue
			}
			for _, candidate := range receipt.Resources {
				if candidate == resource {
					return true
				}
			}
		}
		return false
	}
	bindings := make([]LifecycleBinding, 0, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		if owns(ResourceIdentity{Kind: binding.Kind, ID: binding.ID}, binding.Surface) {
			bindings = append(bindings, binding)
		}
	}
	lock.Bindings = bindings
	degradations := make([]LifecycleExclusion, 0, len(lock.Degradations))
	for _, degradation := range lock.Degradations {
		resource, err := ParseResourceIdentity(degradation.ID)
		if degradation.ResourceKind == "" || err == nil && owns(resource, degradation.Surface) {
			degradations = append(degradations, degradation)
		}
	}
	lock.Degradations = degradations
	sensitive := make([]ProjectSensitiveDisclosure, 0, len(lock.Sensitive))
	for _, disclosure := range lock.Sensitive {
		keep := owns(disclosure.Resource, disclosure.Surface)
		if disclosure.Resource.Kind == "pack" {
			for _, receipt := range lock.Receipts {
				keep = keep || receipt.Pack.ID == disclosure.Resource.ID && receipt.Surface == disclosure.Surface
			}
		}
		if keep {
			sensitive = append(sensitive, disclosure)
		}
	}
	lock.Sensitive = sensitive
	return lock
}

func coalesceProjectRemovalActions(actions []ProjectionAction) ([]ProjectionAction, error) {
	result := make([]ProjectionAction, 0, len(actions))
	groups := map[string]int{}
	originals := map[string]string{}
	for _, action := range actions {
		composable := action.Kind == ActionInstructionFile || action.Kind == ActionOpenCodeInstructionFile || action.Kind == ActionClaudeProjectInstruction || action.Kind == ActionClaudeProjectMCP
		if !composable {
			result = append(result, action)
			continue
		}
		key := string(action.Surface) + "\x00" + filepath.Clean(action.Target)
		index, grouped := groups[key]
		if !grouped {
			data, err := os.ReadFile(action.Target)
			if err != nil {
				return nil, err
			}
			originals[key] = string(data)
			groups[key] = len(result)
			result = append(result, action)
			index = len(result) - 1
		}
		removed, ok := removedProjectContribution(originals[key], action.Content)
		if !ok {
			return nil, fmt.Errorf("project removal action %s does not describe one exact contribution", action.ID)
		}
		combined := strings.Replace(result[index].Content, removed, "", 1)
		if !grouped {
			combined = strings.Replace(originals[key], removed, "", 1)
		}
		result[index].Content = combined
		result[index].Mode = ProjectionRemoveContent
		if strings.TrimSpace(combined) == "" {
			result[index].Content, result[index].Mode = "", ProjectionDeleteTarget
		}
	}
	return result, nil
}

func removedProjectContribution(original, remaining string) (string, bool) {
	prefix := 0
	for prefix < len(original) && prefix < len(remaining) && original[prefix] == remaining[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(original)-prefix && suffix < len(remaining)-prefix && original[len(original)-1-suffix] == remaining[len(remaining)-1-suffix] {
		suffix++
	}
	if len(original)-prefix-suffix <= 0 || original[:prefix]+original[len(original)-suffix:] != remaining {
		return "", false
	}
	return original[prefix : len(original)-suffix], true
}

func removeProjectManifestPack(packs []ProjectManifestPack, packID string) []ProjectManifestPack {
	result := make([]ProjectManifestPack, 0, len(packs)-1)
	for _, pack := range packs {
		if pack.ID != packID {
			result = append(result, pack)
		}
	}
	return result
}

func projectLockWithoutPack(lock ProjectLockProposal, packID string) (ProjectLockProposal, error) {
	receipts := make([]installedPackReceipt, 0, len(lock.Receipts))
	for _, receipt := range lock.Receipts {
		if receipt.Pack.ID != packID {
			receipts = append(receipts, receipt)
		}
	}
	lock.Receipts = receipts
	hydrated, err := hydrateProjectLock(lock)
	if err != nil {
		return ProjectLockProposal{}, err
	}
	return filterProjectLockMetadataToReceipts(hydrated), nil
}

func removeProjectSurface(surfaces []Surface, removed Surface) []Surface {
	result := make([]Surface, 0, len(surfaces))
	for _, surface := range surfaces {
		if surface != removed {
			result = append(result, surface)
		}
	}
	return sortedProjectSurfaces(result)
}

func withoutProjectSurface(projections []ProjectProjectionPlan, surface Surface) []ProjectProjectionPlan {
	result := make([]ProjectProjectionPlan, 0, len(projections))
	for _, projection := range projections {
		if projection.Surface == surface {
			continue
		}
		result = append(result, projection)
	}
	return result
}

func ApplyProjectUninstall(ctx context.Context, request ProjectUninstallApplyRequest) (ProjectUninstallApplyResult, error) {
	preview := request.Preview
	if request.Adapter == nil || request.PackyHome == "" || preview.projectRoot == "" {
		return ProjectUninstallApplyResult{}, errors.New("project uninstall Apply requires the exact preview, adapter, and Packy Home")
	}
	if preview.Disposition != ProjectInstallPreviewable {
		return ProjectUninstallApplyResult{}, ProjectUninstallNotActionableError{Disposition: preview.Disposition}
	}
	guard, err := acquireProjectInstallGuard(ctx, preview.projectRoot)
	if err != nil {
		return ProjectUninstallApplyResult{}, err
	}
	defer guard.Close()
	fresh, err := PreviewProjectUninstall(ctx, preview.request, request.Adapter)
	if err != nil {
		return ProjectUninstallApplyResult{}, err
	}
	if fresh.Observation != preview.Observation {
		return ProjectUninstallApplyResult{}, StalePlanError{Precondition: "project targets changed after preview"}
	}
	if fresh.Disposition != ProjectInstallPreviewable {
		return ProjectUninstallApplyResult{}, ProjectUninstallNotActionableError{Disposition: fresh.Disposition}
	}
	lockIndex := -1
	for i, action := range fresh.actions {
		if action.Kind == ActionProjectLockFile {
			lockIndex = i
			break
		}
	}
	if lockIndex < 0 {
		return ProjectUninstallApplyResult{}, errors.New("project uninstall plan omitted the final lock removal")
	}
	nonLock := append([]ProjectionAction(nil), fresh.actions[:lockIndex]...)
	lockAction := fresh.actions[lockIndex]
	forward := append(append([]ProjectionAction(nil), nonLock...), lockAction)
	backupRoot, err := os.MkdirTemp("", "packy-project-rollback-")
	if err != nil {
		return ProjectUninstallApplyResult{}, err
	}
	defer os.RemoveAll(backupRoot)
	reverse, err := captureProjectReverseActions(forward, backupRoot)
	if err != nil {
		return ProjectUninstallApplyResult{}, err
	}
	fail := func(cause error) (ProjectUninstallApplyResult, error) {
		return ProjectUninstallApplyResult{}, rollbackProjectMutation(ctx, request.Adapter, reverse, cause)
	}
	if actionErr := request.Adapter.ApplyProjections(ctx, nonLock); actionErr != nil {
		return fail(actionErr)
	}
	if err := verifyProjectUninstallActions(nonLock); err != nil {
		return fail(err)
	}
	if actionErr := request.Adapter.ApplyProjections(ctx, []ProjectionAction{lockAction}); actionErr != nil {
		return fail(actionErr)
	}
	if lockAction.Mode == ProjectionDeleteTarget {
		if missing, err := projectPathMissing(lockAction.Target); err != nil || !missing {
			if err == nil {
				err = ErrVerificationFailed
			}
			return fail(err)
		}
	} else if err := verifyProjectRegularFile(lockAction.Target, []byte(lockAction.Content), fs.FileMode(lockAction.FileMode)); err != nil {
		return fail(err)
	}
	return ProjectUninstallApplyResult{SchemaVersion: 2, Report: "project-uninstall-apply", Status: "verified", Observation: fresh.Observation}, nil
}

func planProjectNoticeRemoval(projectRoot string, pack ProjectManifestPack, surface Surface, lock ProjectLockProposal) (ProjectionAction, []ProjectInstallBlocker, error) {
	path := filepath.Join(projectRoot, "PACKY-NOTICES.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ProjectionAction{}, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notice contribution is missing", Remediation: "restore the exact locked notice contribution before uninstalling"}}, nil
		}
		return ProjectionAction{}, nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return ProjectionAction{}, nil, err
	}
	if !info.Mode().IsRegular() {
		return ProjectionAction{}, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the project notices target or mode differs from the lock", Remediation: "restore the exact locked notice contribution before uninstalling"}}, nil
	}
	surfaces := []Surface{surface}
	if surface == "" {
		surfaces = append([]Surface(nil), pack.Surfaces...)
	}
	remaining := string(data)
	for _, target := range surfaces {
		start, end := projectNoticeMarkers(pack.ID, target)
		fragment, found := extractProjectContribution(remaining, start, end)
		lockedNotice, receiptFound := projectNoticeReceiptProjection(lock.Receipts, pack.ID, target)
		if !found || !receiptFound || fingerprintProjectBytes([]byte(fragment)) != lockedNotice.Digest || strings.Count(remaining, start) != 1 || strings.Count(remaining, end) != 1 {
			return ProjectionAction{}, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notice contribution differs from the lock", Remediation: "restore the exact locked notice contribution before uninstalling"}}, nil
		}
		remaining = strings.Replace(remaining, fragment, "", 1)
	}
	action := ProjectionAction{ID: "project-contract:notices", Kind: ActionProjectNoticesFile, Target: path, Content: remaining, FileMode: uint32(info.Mode().Perm()), Precondition: fingerprintProjectBytes(data), Description: "remove the exact project Packy notice contribution"}
	if strings.TrimSpace(remaining) == "" {
		action.Content, action.Mode = "", ProjectionDeleteTarget
	}
	return action, nil, nil
}

func sealProjectUninstallPreview(report JSONProjectUninstallPreview) string {
	report.Observation = ""
	data, _ := json.Marshal(struct {
		Report  JSONProjectUninstallPreview
		Actions []ProjectionAction
	}{Report: report, Actions: report.actions})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sameProjectProjectionStatuses(left, right []ProjectProjectionStatus) bool {
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return string(l) == string(r)
}

func deduplicateProjectUninstallBlockers(blockers []ProjectInstallBlocker) []ProjectInstallBlocker {
	seen := map[string]bool{}
	result := make([]ProjectInstallBlocker, 0, len(blockers))
	for _, blocker := range blockers {
		key := blocker.Code + "\x00" + blocker.Resource.String() + "\x00" + blocker.Target + "\x00" + blocker.Detail
		if !seen[key] {
			seen[key] = true
			result = append(result, blocker)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Code != result[j].Code {
			return result[i].Code < result[j].Code
		}
		if result[i].Target != result[j].Target {
			return result[i].Target < result[j].Target
		}
		return result[i].Resource.String() < result[j].Resource.String()
	})
	return result
}

func verifyProjectUninstallActions(actions []ProjectionAction) error {
	for _, action := range actions {
		if action.Mode == ProjectionDeleteTarget {
			if _, err := os.Lstat(action.Target); errors.Is(err, fs.ErrNotExist) {
				continue
			} else if err != nil {
				return err
			}
			return fmt.Errorf("verify project removal %s: %w", action.ID, ErrVerificationFailed)
		}
		if err := verifyProjectRegularFile(action.Target, []byte(action.Content), fs.FileMode(action.FileMode)); err != nil {
			return err
		}
	}
	return nil
}
