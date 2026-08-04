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

const ProjectUninstallPreviewSchemaVersion = 1

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
	pack := installation.Manifest.Packs[0]
	if pack.ID != request.PackID {
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
	// Schema v1 has one pack on one surface. Keeping the selected surface
	// explicit here makes the subtraction boundary stable when the manifest
	// grows additional contributors.
	surface := pack.Surfaces[0]
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
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{
		ProjectRoot: request.ProjectRoot, ProjectInstallation: &installation, ProjectGoal: ProjectionAbsent,
	})
	if err != nil {
		return report, err
	}
	uninstallStatuses, err := projectProjectionStatusesFromObservation(request.ProjectRoot, installation.Lock, observation)
	if err != nil {
		return report, err
	}
	if !sameProjectProjectionStatuses(report.Projections, uninstallStatuses) {
		return report, errors.New("project uninstall adapter returned inconsistent portable ownership evidence")
	}
	actions := make([]ProjectionAction, 0, len(observation.Projections)+3)
	for _, projection := range observation.Projections {
		actions = append(actions, projection.Action)
	}
	noticeAction, noticeBlockers, err := planProjectNoticeRemoval(request.ProjectRoot, pack, installation.Lock)
	if err != nil {
		return report, err
	}
	report.Blockers = append(report.Blockers, noticeBlockers...)
	if len(noticeBlockers) == 0 {
		actions = append(actions, noticeAction)
	}
	manifestPath := filepath.Join(request.ProjectRoot, "packy.json")
	lockPath := filepath.Join(request.ProjectRoot, "packy.lock.json")
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
	journalPath := projectInstallJournalPath(request.PackyHome, preview.projectRoot)
	if err := recoverProjectInstall(ctx, request.Adapter, journalPath, preview.projectRoot); err != nil {
		return ProjectUninstallApplyResult{}, err
	}
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
	backupRoot := projectInstallBackupRoot(journalPath)
	reverse, err := captureProjectReverseActions(forward, backupRoot)
	if err != nil {
		return ProjectUninstallApplyResult{}, err
	}
	journal := projectInstallJournal{SchemaVersion: projectInstallApplySchemaVersion, Observation: fresh.Observation, ProjectRoot: fresh.projectRoot, Reverse: reverse}
	journal.Seal = sealProjectInstallJournal(journal)
	if err := writeProjectInstallJournal(journalPath, journal); err != nil {
		_ = os.RemoveAll(backupRoot)
		return ProjectUninstallApplyResult{}, err
	}
	fail := func(cause error) (ProjectUninstallApplyResult, error) {
		return ProjectUninstallApplyResult{}, rollbackProjectMutation(ctx, request.Adapter, reverse, journalPath, cause)
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
	if missing, err := projectPathMissing(lockAction.Target); err != nil || !missing {
		if err == nil {
			err = ErrVerificationFailed
		}
		return fail(err)
	}
	if err := removeProjectInstallJournal(journalPath); err != nil {
		return ProjectUninstallApplyResult{}, err
	}
	return ProjectUninstallApplyResult{SchemaVersion: 1, Report: "project-uninstall-apply", Status: "verified", Observation: fresh.Observation}, nil
}

func planProjectNoticeRemoval(projectRoot string, pack ProjectManifestPack, lock ProjectLockProposal) (ProjectionAction, []ProjectInstallBlocker, error) {
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
	if !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != lock.NoticesFileMode {
		return ProjectionAction{}, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the project notices target or mode differs from the lock", Remediation: "restore the exact locked notice contribution before uninstalling"}}, nil
	}
	start, end := projectNoticeMarkers(pack.ID)
	fragment, found := extractProjectContribution(string(data), start, end)
	if !found || fingerprintProjectBytes([]byte(fragment)) != lock.NoticesSHA256 || strings.Count(string(data), start) != 1 || strings.Count(string(data), end) != 1 {
		return ProjectionAction{}, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notice contribution differs from the lock", Remediation: "restore the exact locked notice contribution before uninstalling"}}, nil
	}
	remaining := strings.Replace(string(data), fragment, "", 1)
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
