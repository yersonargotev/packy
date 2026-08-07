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
	"syscall"
	"time"
)

const projectInstallApplySchemaVersion = 1

type ProjectInstallApplyRequest struct {
	Preview                    JSONProjectInstallPreview
	PackyHome                  string
	Adapter                    SurfaceAdapter
	DestructiveCleanupApproved bool
}

type ProjectInstallApplyResult struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Status        string `json:"status"`
	Observation   string `json:"observation"`
}

func (f Facade) ApplyProjectInstall(ctx context.Context, request ProjectInstallApplyRequest) (ProjectInstallApplyResult, error) {
	preview := request.Preview
	if request.Adapter == nil || preview.projectRoot == "" || request.PackyHome == "" {
		return ProjectInstallApplyResult{}, errors.New("project installation Apply requires the exact preview, adapter, and Packy Home")
	}
	if preview.Disposition == ProjectInstallBlocked {
		return ProjectInstallApplyResult{}, ProjectInstallNotActionableError{Disposition: preview.Disposition}
	}
	if len(preview.Retirements) > 0 && !request.DestructiveCleanupApproved {
		return ProjectInstallApplyResult{}, errors.New("project update retirement requires approval of the exact destructive-cleanup phase")
	}
	guard, err := acquireProjectInstallGuard(ctx, preview.projectRoot)
	if err != nil {
		return ProjectInstallApplyResult{}, err
	}
	defer guard.Close()

	freshRequest := preview.request
	var fresh JSONProjectInstallPreview
	if preview.updateRequest.PackID != "" {
		fresh, err = f.PreviewProjectUpdate(ctx, preview.updateRequest, request.Adapter)
	} else {
		if freshRequest.ProjectRoot == "" {
			freshRequest = ProjectInstallRequest{PackID: preview.Pack.ID, Surface: preview.Surface, ProjectRoot: preview.projectRoot}
		}
		fresh, err = f.PreviewProjectInstall(ctx, freshRequest, request.Adapter)
	}
	if err != nil {
		return ProjectInstallApplyResult{}, err
	}
	if fresh.Observation != preview.Observation {
		return ProjectInstallApplyResult{}, StalePlanError{Precondition: "project targets changed after preview"}
	}
	if len(fresh.Retirements) > 0 && !request.DestructiveCleanupApproved {
		return ProjectInstallApplyResult{}, errors.New("project update retirement requires approval of the exact destructive-cleanup phase")
	}
	if fresh.Disposition == ProjectInstallConverged {
		return ProjectInstallApplyResult{SchemaVersion: projectInstallApplySchemaVersion, Report: "project-install-apply", Status: "no-op", Observation: fresh.Observation}, nil
	}
	if fresh.Disposition != ProjectInstallPreviewable {
		return ProjectInstallApplyResult{}, ProjectInstallNotActionableError{Disposition: fresh.Disposition}
	}

	manifest, err := marshalProjectManifest(fresh.Manifest)
	if err != nil {
		return ProjectInstallApplyResult{}, err
	}
	notices := fresh.noticeContent
	lock, err := marshalProjectLock(fresh.Lock)
	if err != nil {
		return ProjectInstallApplyResult{}, err
	}
	nonLock := make([]ProjectionAction, 0, len(fresh.actions)+2)
	for _, action := range fresh.actions {
		if _, relErr := RelativeProjectTarget(fresh.projectRoot, action.Target); relErr != nil {
			return ProjectInstallApplyResult{}, relErr
		}
		nonLock = append(nonLock, action)
	}
	manifestPath := filepath.Join(fresh.projectRoot, fresh.Manifest.Path)
	if matches, matchErr := projectRegularFileMatches(manifestPath, manifest, 0o644); matchErr != nil {
		return ProjectInstallApplyResult{}, matchErr
	} else if !matches {
		nonLock = append(nonLock, ProjectionAction{ID: "project-contract:manifest", Kind: ActionProjectManifestFile, Target: manifestPath, Content: string(manifest), FileMode: 0o644, Precondition: projectTargetFingerprint(manifestPath), Description: "write the project pack manifest"})
	}
	if !fresh.noticeIntact {
		nonLock = append(nonLock, ProjectionAction{ID: "project-contract:notices", Kind: ActionProjectNoticesFile, Target: filepath.Join(fresh.projectRoot, fresh.Notices.Path), Content: notices, FileMode: fresh.noticeMode, Precondition: fresh.noticeBefore, Description: "merge the project Packy notice contribution"})
	}
	sort.SliceStable(nonLock, func(i, j int) bool { return nonLock[i].Target < nonLock[j].Target })
	lockPath := filepath.Join(fresh.projectRoot, fresh.Lock.Path)
	lockAction := ProjectionAction{ID: "project-contract:lock", Kind: ActionProjectLockFile, Target: lockPath, Content: string(lock), FileMode: 0o644, Precondition: projectTargetFingerprint(lockPath), Description: "publish the verified project pack lock"}
	lockMatches, err := projectRegularFileMatches(lockPath, lock, 0o644)
	if err != nil {
		return ProjectInstallApplyResult{}, err
	}
	writeLock := !lockMatches

	forward := append([]ProjectionAction(nil), nonLock...)
	if writeLock {
		forward = append(forward, lockAction)
	}
	backupRoot, err := os.MkdirTemp("", "packy-project-rollback-")
	if err != nil {
		return ProjectInstallApplyResult{}, err
	}
	defer os.RemoveAll(backupRoot)
	reverse, err := captureProjectReverseActions(forward, backupRoot)
	if err != nil {
		return ProjectInstallApplyResult{}, err
	}
	fail := func(cause error) (ProjectInstallApplyResult, error) {
		return ProjectInstallApplyResult{}, rollbackProjectMutation(ctx, request.Adapter, reverse, cause)
	}
	if actionErr := request.Adapter.ApplyProjections(ctx, nonLock); actionErr != nil {
		return fail(actionErr)
	}
	if err := verifyProjectInstallSurface(ctx, request.Adapter, fresh); err != nil {
		return fail(err)
	}
	if err := verifyProjectRegularFile(filepath.Join(fresh.projectRoot, fresh.Manifest.Path), manifest, 0o644); err != nil {
		return fail(err)
	}
	if err := verifyProjectNoticeContribution(fresh); err != nil {
		return fail(err)
	}
	if writeLock {
		if actionErr := request.Adapter.ApplyProjections(ctx, []ProjectionAction{lockAction}); actionErr != nil {
			return fail(actionErr)
		}
	}
	var verified JSONProjectInstallPreview
	if fresh.updateRequest.PackID != "" {
		verified, err = f.PreviewProjectUpdate(ctx, fresh.updateRequest, request.Adapter)
	} else {
		verified, err = f.PreviewProjectInstall(ctx, freshRequest, request.Adapter)
	}
	if err != nil || verified.Disposition != ProjectInstallConverged {
		if err == nil {
			err = fmt.Errorf("%w: disposition=%s blockers=%v", ErrVerificationFailed, verified.Disposition, verified.Blockers)
		}
		return fail(err)
	}
	return ProjectInstallApplyResult{SchemaVersion: projectInstallApplySchemaVersion, Report: "project-install-apply", Status: "verified", Observation: verified.Observation}, nil
}

func rollbackProjectMutation(ctx context.Context, adapter SurfaceAdapter, reverse []ProjectionAction, cause error) error {
	pending, err := pendingProjectReverse(reverse)
	if err != nil {
		return fmt.Errorf("project mutation failed (%v) and rollback could not be prepared: %w", cause, err)
	}
	if actionErr := adapter.ApplyProjections(ctx, pending); actionErr != nil {
		return fmt.Errorf("project mutation failed (%v) and rollback failed: %w", cause, actionErr)
	}
	if err := verifyProjectReverse(reverse); err != nil {
		return fmt.Errorf("project mutation failed (%v) and rollback verification failed: %w", cause, err)
	}
	return cause
}

func marshalProjectManifest(proposal ProjectContractProposal) ([]byte, error) {
	value := struct {
		SchemaVersion int                   `json:"schema_version"`
		Packs         []ProjectManifestPack `json:"packs"`
	}{SchemaVersion: proposal.SchemaVersion, Packs: proposal.Packs}
	data, err := json.MarshalIndent(value, "", "  ")
	return append(data, '\n'), err
}

func fingerprintProjectBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func marshalProjectLock(proposal ProjectLockProposal) ([]byte, error) {
	data, err := json.MarshalIndent(proposal, "", "  ")
	return append(data, '\n'), err
}

func renderProjectNoticeBlock(preview JSONProjectInstallPreview) string {
	var out strings.Builder
	fmt.Fprintf(&out, "<!-- packy:project:%s:notices:start -->\n", preview.Pack.ID)
	out.WriteString("## Packy project notices\n\n")
	fmt.Fprintf(&out, "Pack: %s %s\n\nReviewed Pack content is authoritative.\n\n", preview.Pack.ID, preview.Pack.Version)
	if len(preview.Notices.Contributions) == 0 {
		out.WriteString("No additional downstream notice text is required by the reviewed Pack contract.\n")
	}
	for _, notice := range preview.Notices.Contributions {
		fmt.Fprintf(&out, "## %s\n\nLicense: %s\n\n%s\n\n", notice.Resource, notice.License, notice.Attribution)
	}
	fmt.Fprintf(&out, "<!-- packy:project:%s:notices:end -->", preview.Pack.ID)
	return out.String()
}

func projectNoticeMarkers(packID string) (string, string) {
	return "<!-- packy:project:" + packID + ":notices:start -->", "<!-- packy:project:" + packID + ":notices:end -->"
}

func planProjectNotices(preview JSONProjectInstallPreview, lockExists, allowMissingRestore bool, priorNoticeSHA256 string) (string, uint32, string, bool, []ProjectInstallBlocker, error) {
	path := filepath.Join(preview.projectRoot, preview.Notices.Path)
	current, err := os.ReadFile(path)
	before, mode := "missing", uint32(0o644)
	if err == nil {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return "", 0, "", false, nil, statErr
		}
		if !info.Mode().IsRegular() {
			return "", 0, "", false, []ProjectInstallBlocker{{Code: "unsafe_path", Target: preview.Notices.Path, Detail: "project notices target is not a regular file", Remediation: "move or remove the unsafe target before installation"}}, nil
		}
		before, mode = fingerprintProjectBytes(current), uint32(info.Mode().Perm())
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", 0, "", false, nil, err
	}
	content := string(current)
	start, end := projectNoticeMarkers(preview.Pack.ID)
	starts, ends := strings.Count(content, start), strings.Count(content, end)
	block := renderProjectNoticeBlock(preview)
	if starts == 0 && ends == 0 {
		if lockExists && !allowMissingRestore {
			return content, mode, before, false, []ProjectInstallBlocker{{Code: "owned_drift", Target: preview.Notices.Path, Detail: "the locked Packy notice contribution is missing", Remediation: "restore the generated notice contribution before installation"}}, nil
		}
		return mergeProjectContribution(content, block, start, end), mode, before, false, nil, nil
	}
	if starts != 1 || ends != 1 {
		return content, mode, before, false, []ProjectInstallBlocker{{Code: "ambiguous_packy_markers", Target: preview.Notices.Path, Detail: "the project notices contain malformed or duplicate Packy markers", Remediation: "restore one exact Packy notice contribution before installation"}}, nil
	}
	fragment, found := extractProjectContribution(content, start, end)
	if !found {
		return content, mode, before, false, []ProjectInstallBlocker{{Code: "ambiguous_packy_markers", Target: preview.Notices.Path, Detail: "the project notices contain malformed Packy markers", Remediation: "restore one exact Packy notice contribution before installation"}}, nil
	}
	if fragment != block {
		if allowMissingRestore && priorNoticeSHA256 != "" && fingerprintProjectBytes([]byte(fragment)) == priorNoticeSHA256 {
			return mergeProjectContribution(content, block, start, end), mode, before, false, nil, nil
		}
		code := "ambiguous_packy_markers"
		if lockExists {
			code = "owned_drift"
		}
		return content, mode, before, false, []ProjectInstallBlocker{{Code: code, Target: preview.Notices.Path, Detail: "the Packy notice contribution differs from the admitted content", Remediation: "restore the exact generated notice contribution before installation"}}, nil
	}
	return content, mode, before, true, nil, nil
}

func mergeProjectContribution(current, block, start, end string) string {
	if fragment, found := extractProjectContribution(current, start, end); found {
		return strings.Replace(current, fragment, block, 1)
	}
	trimmed := strings.TrimRight(current, "\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}

func extractProjectContribution(content, start, end string) (string, bool) {
	startIndex := strings.Index(content, start)
	if startIndex < 0 {
		return "", false
	}
	relativeEnd := strings.Index(content[startIndex+len(start):], end)
	if relativeEnd < 0 {
		return "", false
	}
	endIndex := startIndex + len(start) + relativeEnd + len(end)
	return content[startIndex:endIndex], true
}

func verifyProjectNoticeContribution(preview JSONProjectInstallPreview) error {
	lockedNotice, found := projectNoticeReceiptProjection(preview.Lock.Receipts, preview.Pack.ID)
	if !found {
		return fmt.Errorf("verify project notice contribution: %w", ErrVerificationFailed)
	}
	_, _, _, intact, blockers, err := planProjectNotices(preview, true, false, lockedNotice.Digest)
	if err != nil || !intact || len(blockers) > 0 {
		return fmt.Errorf("verify project notice contribution: %w", ErrVerificationFailed)
	}
	return nil
}

func readExistingProjectLock(projectRoot string) (ProjectLockProposal, bool, error) {
	path := filepath.Join(projectRoot, "packy.lock.json")
	info, statErr := os.Lstat(path)
	if errors.Is(statErr, fs.ErrNotExist) {
		return ProjectLockProposal{}, false, nil
	}
	if statErr != nil {
		return ProjectLockProposal{}, false, statErr
	}
	if !info.Mode().IsRegular() {
		return ProjectLockProposal{}, false, errors.New("project lock is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectLockProposal{}, false, err
	}
	var lock ProjectLockProposal
	if err := strictDecode(data, &lock); err != nil {
		return ProjectLockProposal{}, false, fmt.Errorf("decode project lock: %w", err)
	}
	lock, err = hydrateProjectLock(lock)
	if err != nil {
		return ProjectLockProposal{}, false, err
	}
	if !supportedProjectContractVersion(lock.SchemaVersion, lock.MinimumPackyCapability) {
		return ProjectLockProposal{}, false, errors.New("project lock schema or minimum Packy capability is unsupported")
	}
	return lock, true, nil
}

func supportedProjectContractVersion(schemaVersion int, minimumCapability string) bool {
	return schemaVersion == projectContractSchemaV1 && minimumCapability == ""
}

func projectLockOwnsProjection(lock ProjectLockProposal, resource ResourceIdentity, target, fingerprint string) bool {
	for _, projection := range lock.Projections {
		if projection.Resource == resource && filepath.Clean(projection.Target) == filepath.Clean(target) && projection.DesiredFingerprint == fingerprint && projection.OwnerPack != "" && projection.Surface != "" {
			return true
		}
	}
	return false
}

func findProjectLockProjection(lock ProjectLockProposal, resource ResourceIdentity, target string) (ProjectProjectionPlan, bool) {
	for _, projection := range lock.Projections {
		if projection.Resource == resource && filepath.Clean(projection.Target) == filepath.Clean(target) && projection.OwnerPack != "" && projection.Surface != "" {
			return projection, true
		}
	}
	return ProjectProjectionPlan{}, false
}

func inspectProjectContract(preview JSONProjectInstallPreview, existingLock ProjectLockProposal, lockExists, allowRefresh bool) (bool, []ProjectInstallBlocker, error) {
	manifest, err := marshalProjectManifest(preview.Manifest)
	if err != nil {
		return false, nil, err
	}
	lock, err := marshalProjectLock(preview.Lock)
	if err != nil {
		return false, nil, err
	}
	targets := []struct {
		name    string
		content []byte
	}{{preview.Manifest.Path, manifest}, {preview.Lock.Path, lock}}
	if !lockExists {
		var blockers []ProjectInstallBlocker
		for _, target := range targets {
			path := filepath.Join(preview.projectRoot, target.name)
			if _, err := os.Lstat(path); err == nil {
				blockers = append(blockers, ProjectInstallBlocker{Code: "foreign_contract_target", Target: target.name, Detail: "project contract target exists without a valid Packy lock", Remediation: "move or remove the foreign target before installation"})
			} else if !errors.Is(err, fs.ErrNotExist) {
				return false, nil, err
			}
		}
		return false, blockers, nil
	}
	refreshNeeded := !projectLocksEqual(existingLock, preview.Lock)
	if refreshNeeded && !allowRefresh {
		return false, []ProjectInstallBlocker{{Code: "owned_drift", Target: preview.Lock.Path, Detail: "the project lock does not match the exact admitted installation intent", Remediation: "restore the generated project lock before installation"}}, nil
	}
	var blockers []ProjectInstallBlocker
	for _, target := range targets {
		path := filepath.Join(preview.projectRoot, target.name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			blockers = append(blockers, ProjectInstallBlocker{Code: "owned_drift", Target: target.name, Detail: "a locked project contract target is missing or unsafe", Remediation: "restore the generated project contract before installation"})
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return false, nil, err
		}
		if string(data) != string(target.content) {
			if !allowRefresh {
				blockers = append(blockers, ProjectInstallBlocker{Code: "owned_drift", Target: target.name, Detail: "a locked project contract target has changed", Remediation: "restore the generated project contract before installation"})
			} else {
				refreshNeeded = true
			}
		}
		if info.Mode().Perm() != 0o644 {
			blockers = append(blockers, ProjectInstallBlocker{Code: "owned_drift", Target: target.name, Detail: "a locked project contract target has an unexpected mode", Remediation: "restore the generated project contract mode before installation"})
		}
	}
	if len(blockers) > 0 {
		return false, blockers, nil
	}
	if refreshNeeded {
		return false, nil, nil
	}
	if !preview.noticeIntact {
		return false, nil, nil
	}
	for _, projection := range preview.Projections {
		if projection.ObservedState != "owned" {
			return false, nil, nil
		}
	}
	return true, nil, nil
}

func projectTargetFingerprint(path string) string {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		return "unreadable"
	}
	return fingerprintProjectBytes(data)
}

func projectRegularFileMatches(path string, expected []byte, mode fs.FileMode) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("project contract target %s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return string(data) == string(expected) && info.Mode().Perm() == mode.Perm(), nil
}

func verifyProjectRegularFile(path string, expected []byte, mode fs.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != mode.Perm() {
		return fmt.Errorf("verify project contract %s: %w", filepath.Base(path), ErrVerificationFailed)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(expected) {
		return fmt.Errorf("verify project contract %s: %w", filepath.Base(path), ErrVerificationFailed)
	}
	return nil
}

func projectLocksEqual(left, right ProjectLockProposal) bool {
	leftData, _ := json.Marshal(left)
	rightData, _ := json.Marshal(right)
	return string(leftData) == string(rightData)
}

func verifyProjectInstallSurface(ctx context.Context, adapter SurfaceAdapter, preview JSONProjectInstallPreview) error {
	if preview.updateRequest.PackID != "" {
		resolver, ok := adapter.(projectSurfaceAdapterResolver)
		if !ok {
			return errors.New("complete project update verification requires the installed surface adapter set")
		}
		surfaces := append([]Surface(nil), preview.Pack.Surfaces...)
		for _, surface := range surfaces {
			surfaceAdapter, found := resolver.projectSurfaceAdapter(surface)
			if !found {
				return fmt.Errorf("complete project update verification is missing the %s adapter", surface)
			}
			observation, err := inspectSurface(ctx, surfaceAdapter, SurfaceTransition{Desired: preview.pack, ProjectRoot: preview.projectRoot})
			if err != nil {
				return err
			}
			var expected []ProjectProjectionPlan
			for _, projection := range preview.Projections {
				for _, projectionSurface := range projectProjectionSurfaces(projection) {
					if projectionSurface == surface {
						expected = append(expected, projection)
						break
					}
				}
			}
			if err := verifyProjectProjectionObservation(preview.projectRoot, expected, observation); err != nil {
				return err
			}
		}
		return nil
	}
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{Desired: preview.pack, ProjectRoot: preview.projectRoot})
	if err != nil {
		return err
	}
	return verifyProjectProjectionObservation(preview.projectRoot, preview.Projections, observation)
}

func verifyProjectProjectionObservation(projectRoot string, expected []ProjectProjectionPlan, observation SurfaceInspection) error {
	want := make(map[string]string, len(expected))
	for _, projection := range expected {
		want[projection.Resource.String()+"\x00"+filepath.Clean(filepath.FromSlash(projection.Target))] = projection.DesiredFingerprint
	}
	for _, projection := range observation.Projections {
		relative, relativeErr := RelativeProjectTarget(projectRoot, projection.Action.Target)
		if relativeErr != nil {
			return relativeErr
		}
		key := projection.ID + "\x00" + filepath.Clean(filepath.FromSlash(relative))
		desired, ok := want[key]
		if ok && (!projection.Exists || normalizeProjectProjectionFingerprint(projection.ObservedFingerprint) != desired) {
			return fmt.Errorf("verify project projection %s: %w", projection.ID, ErrVerificationFailed)
		}
		delete(want, key)
	}
	if len(want) > 0 {
		return fmt.Errorf("verify project projections: %w", ErrVerificationFailed)
	}
	return nil
}

func captureProjectReverseActions(actions []ProjectionAction, backupRoot string) ([]ProjectionAction, error) {
	if err := os.RemoveAll(backupRoot); err != nil {
		return nil, err
	}
	reverse := make([]ProjectionAction, 0, len(actions))
	for index, action := range actions {
		info, err := os.Lstat(action.Target)
		if errors.Is(err, fs.ErrNotExist) {
			reverse = append(reverse, ProjectionAction{ID: "restore:" + action.ID, Surface: action.Surface, Kind: action.Kind, Target: action.Target, Mode: ProjectionDeleteTarget, Precondition: projectActionDesiredFingerprint(action)})
			continue
		}
		if err != nil {
			_ = os.RemoveAll(backupRoot)
			return nil, err
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 && projectTreeAction(action.Kind) {
			backup := filepath.Join(backupRoot, fmt.Sprintf("%03d", index))
			if err := copyProjectTreeBackup(action.Target, backup); err != nil {
				_ = os.RemoveAll(backupRoot)
				return nil, err
			}
			reverse = append(reverse, ProjectionAction{ID: "restore:" + action.ID, Surface: action.Surface, Kind: action.Kind, Source: backup, Target: action.Target, Version: action.Precondition, Precondition: "missing"})
			continue
		}
		if !info.Mode().IsRegular() {
			_ = os.RemoveAll(backupRoot)
			return nil, fmt.Errorf("cannot replace existing non-regular project target %s", action.Target)
		}
		data, err := os.ReadFile(action.Target)
		if err != nil {
			_ = os.RemoveAll(backupRoot)
			return nil, err
		}
		reverse = append(reverse, ProjectionAction{ID: "restore:" + action.ID, Surface: action.Surface, Kind: action.Kind, Target: action.Target, Content: string(data), FileMode: uint32(info.Mode().Perm()), Precondition: projectActionDesiredFingerprint(action)})
	}
	for i, j := 0, len(reverse)-1; i < j; i, j = i+1, j-1 {
		reverse[i], reverse[j] = reverse[j], reverse[i]
	}
	return reverse, nil
}

func projectActionDesiredFingerprint(action ProjectionAction) string {
	if action.Mode == ProjectionDeleteTarget {
		return "missing"
	}
	if projectTreeAction(action.Kind) {
		return action.Version
	}
	return fingerprintProjectBytes([]byte(action.Content))
}

func projectTreeAction(kind ProjectionActionKind) bool {
	return kind == ActionCodexProjectSkillTree || kind == ActionClaudeProjectSkillTree
}

func pendingProjectReverse(reverse []ProjectionAction) ([]ProjectionAction, error) {
	pending := make([]ProjectionAction, 0, len(reverse))
	for _, action := range reverse {
		info, err := os.Lstat(action.Target)
		if projectTreeAction(action.Kind) && action.Mode != ProjectionDeleteTarget {
			if errors.Is(err, fs.ErrNotExist) {
				pending = append(pending, action)
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("project rollback target %s changed after mutation", action.Target)
			}
			equal, err := projectTreesEqual(action.Source, action.Target)
			if err != nil {
				return nil, err
			}
			if !equal {
				return nil, fmt.Errorf("project rollback target %s changed after mutation", action.Target)
			}
			continue
		}
		if action.Mode == ProjectionDeleteTarget && errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if action.Mode != ProjectionDeleteTarget && errors.Is(err, fs.ErrNotExist) {
			pending = append(pending, action)
			continue
		}
		if err != nil {
			return nil, err
		}
		if action.Mode != ProjectionDeleteTarget && info.Mode().IsRegular() {
			data, err := os.ReadFile(action.Target)
			if err != nil {
				return nil, err
			}
			if string(data) == action.Content && uint32(info.Mode().Perm()) == action.FileMode {
				continue
			}
		}
		pending = append(pending, action)
	}
	return pending, nil
}

func verifyProjectReverse(reverse []ProjectionAction) error {
	for _, action := range reverse {
		if projectTreeAction(action.Kind) && action.Mode != ProjectionDeleteTarget {
			equal, err := projectTreesEqual(action.Source, action.Target)
			if err != nil || !equal {
				return fmt.Errorf("rollback did not restore project tree %s", action.Target)
			}
			continue
		}
		data, err := os.ReadFile(action.Target)
		if action.Mode == ProjectionDeleteTarget {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err == nil {
				return fmt.Errorf("rollback left project target %s", action.Target)
			}
			return err
		}
		if err != nil || string(data) != action.Content {
			return fmt.Errorf("rollback did not restore project target %s", action.Target)
		}
		info, err := os.Lstat(action.Target)
		if err != nil || !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != action.FileMode {
			return fmt.Errorf("rollback did not restore project target mode %s", action.Target)
		}
	}
	return nil
}

func copyProjectTreeBackup(source, target string) error {
	var directories []string
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("cannot back up non-regular project tree path %s", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
				return err
			}
			if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
				return err
			}
			directories = append(directories, destination)
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err = file.Write(data); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		_ = os.RemoveAll(target)
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err := syncProjectDirectory(directories[i]); err != nil {
			_ = os.RemoveAll(target)
			return err
		}
	}
	return nil
}

func projectTreesEqual(left, right string) (bool, error) {
	type fact struct {
		path string
		mode fs.FileMode
		dir  bool
		data string
	}
	read := func(root string) ([]fact, error) {
		var facts []fact
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return fmt.Errorf("project tree contains non-regular path %s", path)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			item := fact{path: filepath.ToSlash(relative), mode: info.Mode().Perm(), dir: info.IsDir()}
			if info.Mode().IsRegular() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				item.data = string(data)
			}
			facts = append(facts, item)
			return nil
		})
		return facts, err
	}
	leftFacts, err := read(left)
	if err != nil {
		return false, err
	}
	rightFacts, err := read(right)
	if err != nil {
		return false, err
	}
	if len(leftFacts) != len(rightFacts) {
		return false, nil
	}
	for i := range leftFacts {
		if leftFacts[i] != rightFacts[i] {
			return false, nil
		}
	}
	return true, nil
}

func syncProjectDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

type projectInstallGuard struct{ file *os.File }

func acquireProjectInstallGuard(ctx context.Context, root string) (*projectInstallGuard, error) {
	file, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	for {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return &projectInstallGuard{file: file}, nil
		} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (guard *projectInstallGuard) Close() error {
	if guard == nil || guard.file == nil {
		return nil
	}
	file := guard.file
	guard.file = nil
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return file.Close()
}
