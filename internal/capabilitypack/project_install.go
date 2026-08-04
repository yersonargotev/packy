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
)

const ProjectInstallPreviewSchemaVersion = 1

type ProjectInstallDisposition string

const (
	ProjectInstallPreviewable ProjectInstallDisposition = "previewable"
	ProjectInstallBlocked     ProjectInstallDisposition = "blocked"
	ProjectInstallConverged   ProjectInstallDisposition = "converged"
)

type ProjectInstallRequest struct {
	PackID      string
	Surface     Surface
	ProjectRoot string
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
	ID              string            `json:"id"`
	Version         string            `json:"version"`
	Surfaces        []Surface         `json:"surfaces"`
	Selection       ResourceSelection `json:"selection"`
	Aliases         []SurfaceAlias    `json:"aliases"`
	ProviderChoices []ProviderChoice  `json:"provider_choices"`
}

type ProjectLockProposal struct {
	Path                   string                    `json:"path"`
	SchemaVersion          int                       `json:"schema_version"`
	MinimumPackyCapability string                    `json:"minimum_packy_capability"`
	Source                 ProjectPackSourceIdentity `json:"source"`
	ResourceGraph          ResourceGraph             `json:"resource_graph"`
	Bindings               []LifecycleBinding        `json:"bindings"`
	Modes                  []OptionalMode            `json:"modes"`
	ManifestSHA256         string                    `json:"manifest_sha256"`
	NoticesSHA256          string                    `json:"notices_sha256"`
	NoticesFileMode        uint32                    `json:"notices_file_mode"`
	Projections            []ProjectProjectionPlan   `json:"projections"`
}

type ProjectNoticesProposal struct {
	Path          string                      `json:"path"`
	Contributions []ProjectNoticeContribution `json:"contributions"`
}

type ProjectPackSourceIdentity struct {
	PackID           string `json:"pack_id"`
	PackVersion      string `json:"pack_version"`
	ManifestSchema   int    `json:"manifest_schema"`
	SourceID         string `json:"source_id"`
	Provider         string `json:"provider"`
	Repository       string `json:"repository"`
	Commit           string `json:"commit"`
	Tree             string `json:"tree"`
	Reference        string `json:"reference,omitempty"`
	SourceLockSHA256 string `json:"source_lock_sha256"`
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
	Contributor        string           `json:"contributor"`
}

type JSONProjectInstallPreview struct {
	SchemaVersion int                       `json:"schema_version"`
	Report        string                    `json:"report"`
	DryRun        bool                      `json:"dry_run"`
	ProjectRoot   string                    `json:"project_root"`
	Pack          ProjectManifestPack       `json:"pack"`
	Surface       Surface                   `json:"surface"`
	Selection     ProjectSelectionPreview   `json:"selection"`
	Manifest      ProjectContractProposal   `json:"manifest"`
	Lock          ProjectLockProposal       `json:"lock"`
	Notices       ProjectNoticesProposal    `json:"notices"`
	Projections   []ProjectProjectionPlan   `json:"projections"`
	Requirements  []string                  `json:"requirements"`
	Blockers      []ProjectInstallBlocker   `json:"blockers"`
	Disposition   ProjectInstallDisposition `json:"disposition"`
	Observation   string                    `json:"observation"`
	projectRoot   string
	pack          Pack
	actions       []ProjectionAction
	noticeContent string
	noticeMode    uint32
	noticeBefore  string
	noticeIntact  bool
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
	fresh, err := f.PreviewProjectInstall(ctx, ProjectInstallRequest{PackID: preview.Pack.ID, Surface: preview.Surface, ProjectRoot: preview.projectRoot}, adapter)
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

func (f Facade) previewProjectInstall(ctx context.Context, request ProjectInstallRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	if request.Surface != SurfaceCodex {
		return JSONProjectInstallPreview{}, fmt.Errorf("project installation preview does not support CLI surface %q", request.Surface)
	}
	if request.ProjectRoot == "" {
		return JSONProjectInstallPreview{}, errors.New("project root is required")
	}
	pack, err := f.catalog.showUnlocked(request.PackID)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if !projectSupportsSurface(pack.Surfaces, request.Surface) {
		return JSONProjectInstallPreview{}, fmt.Errorf("capability pack %q does not support CLI surface %q", request.PackID, request.Surface)
	}
	source, err := f.catalog.projectPackSourceIdentity(pack)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{Desired: pack, ProjectRoot: request.ProjectRoot})
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	graph := ResourceGraphFor(pack, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, false)
	projections := make([]ProjectProjectionPlan, 0, len(observation.Projections))
	actions := make([]ProjectionAction, 0, len(observation.Projections))
	blockers := make([]ProjectInstallBlocker, 0)
	existingLock, lockExists, lockErr := readExistingProjectLock(request.ProjectRoot)
	if lockErr != nil {
		blockers = append(blockers, ProjectInstallBlocker{Code: "invalid_project_lock", Target: "packy.lock.json", Detail: lockErr.Error(), Remediation: "restore or remove the invalid project lock before installation"})
	}
	for _, resource := range observation.Unrepresentable {
		blockers = append(blockers, ProjectInstallBlocker{Code: "unrepresentable_resource", Resource: resource.Resource, Detail: resource.Reason, Remediation: "choose a surface with a declared project-native representation"})
	}
	for _, projection := range observation.Projections {
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
		state := "missing"
		if projection.Exists {
			state = "foreign"
			if lockExists && projectLockOwnsProjection(existingLock, resource, target, projection.DesiredFingerprint) {
				state = "owned"
				if projection.ObservedFingerprint != projection.DesiredFingerprint {
					state = "drifted"
					blockers = append(blockers, ProjectInstallBlocker{Code: "owned_drift", Resource: resource, Target: target, Detail: "Packy-owned project target differs from the locked content", Remediation: "restore the locked content before installation"})
				}
			} else {
				blockers = append(blockers, ProjectInstallBlocker{Code: "foreign_target", Resource: resource, Target: target, Detail: "project target already exists without portable Packy ownership", Remediation: "move or remove the foreign target before installation"})
			}
		}
		mode, fileMode := "copy_tree", uint32(0o700)
		if projection.Action.Kind == ActionInstructionFile {
			mode, fileMode = "merge_marked_file", projection.Action.FileMode
		}
		projections = append(projections, ProjectProjectionPlan{Resource: resource, Target: target, Mode: mode, FileMode: fileMode, DesiredFingerprint: projection.DesiredFingerprint, ObservedState: state, Contributor: "surface:" + string(request.Surface) + ":pack:" + pack.ID})
		action := projection.Action
		action.PreviewOnly = false
		actions = append(actions, action)
	}
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
	for _, resource := range pack.Resources {
		if resource.Kind == "notice" {
			notices = append(notices, ProjectNoticeContribution{Resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, License: resource.License, Attribution: resource.Attribution})
		}
	}
	requirements := projectRequirements(pack)
	manifestPack := ProjectManifestPack{ID: pack.ID, Version: pack.Version, Surfaces: []Surface{request.Surface}, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
	disposition := ProjectInstallPreviewable
	if len(blockers) > 0 {
		disposition = ProjectInstallBlocked
	}
	lockProjections := append([]ProjectProjectionPlan(nil), projections...)
	for i := range lockProjections {
		lockProjections[i].ObservedState = "installed"
	}
	report := JSONProjectInstallPreview{
		SchemaVersion: ProjectInstallPreviewSchemaVersion, Report: "project-install-preview", DryRun: true,
		ProjectRoot: "<project-root>", Pack: manifestPack, Surface: request.Surface, projectRoot: request.ProjectRoot, pack: pack, actions: actions,
		Selection:   ProjectSelectionPreview{Mode: SelectionAll, Resources: graph.Resources},
		Manifest:    ProjectContractProposal{Path: "packy.json", SchemaVersion: 1, Packs: []ProjectManifestPack{manifestPack}},
		Lock:        ProjectLockProposal{Path: "packy.lock.json", SchemaVersion: 1, MinimumPackyCapability: "project-installation-v1", Source: source, ResourceGraph: graph, Bindings: LifecycleContractFor(pack, request.Surface, nil).Bindings, Modes: LifecycleContractFor(pack, request.Surface, nil).OptionalModes, Projections: lockProjections},
		Notices:     ProjectNoticesProposal{Path: "PACKY-NOTICES.md", Contributions: notices},
		Projections: projections, Requirements: requirements, Blockers: blockers, Disposition: disposition,
	}
	manifestBytes, err := marshalProjectManifest(report.Manifest)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	report.Lock.ManifestSHA256 = fingerprintProjectBytes(manifestBytes)
	report.Lock.NoticesSHA256 = fingerprintProjectBytes([]byte(renderProjectNoticeBlock(report)))
	if lockExists {
		report.Lock.NoticesFileMode = existingLock.NoticesFileMode
	}
	noticeContent, noticeMode, noticeBefore, noticeIntact, noticeBlockers, err := planProjectNotices(report, lockExists)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if !lockExists {
		report.Lock.NoticesFileMode = noticeMode
	}
	report.noticeContent, report.noticeMode, report.noticeBefore, report.noticeIntact = noticeContent, noticeMode, noticeBefore, noticeIntact
	report.Blockers = append(report.Blockers, noticeBlockers...)
	if len(noticeBlockers) > 0 {
		report.Disposition = ProjectInstallBlocked
	}
	if len(blockers) == 0 {
		converged, contractBlockers, err := inspectProjectContract(report, existingLock, lockExists)
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

func projectRequirements(pack Pack) []string {
	values := make([]string, 0)
	for _, capability := range pack.Requires.Capabilities {
		values = append(values, "capability:"+capability)
	}
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

func sealProjectInstallPreview(report JSONProjectInstallPreview, surfaceObservation string) string {
	report.Observation = ""
	data, _ := json.Marshal(struct {
		Report             JSONProjectInstallPreview
		SurfaceObservation string
	}{Report: report, SurfaceObservation: surfaceObservation})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c Catalog) projectPackSourceIdentity(pack Pack) (ProjectPackSourceIdentity, error) {
	registryData, err := os.ReadFile(filepath.Join(c.bundleRoot, "sources.json"))
	if err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("read admitted Pack Source registry: %w", err)
	}
	var registry struct {
		Sources []struct {
			ID         string `json:"id"`
			Provider   string `json:"provider"`
			Repository string `json:"repository"`
			Resources  []struct {
				PackID string `json:"pack_id"`
			} `json:"resources"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(registryData, &registry); err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("decode admitted Pack Source registry: %w", err)
	}
	var matches []struct{ ID, Provider, Repository string }
	for _, source := range registry.Sources {
		for _, resource := range source.Resources {
			if resource.PackID == pack.ID {
				matches = append(matches, struct{ ID, Provider, Repository string }{source.ID, source.Provider, source.Repository})
				break
			}
		}
	}
	if len(matches) != 1 {
		return ProjectPackSourceIdentity{}, fmt.Errorf("pack %q does not resolve to one exact admitted Pack Source", pack.ID)
	}
	source := matches[0]
	lockData, err := os.ReadFile(filepath.Join(c.bundleRoot, "sources", source.ID+".lock.json"))
	if err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("read admitted Pack Source lock %q: %w", source.ID, err)
	}
	var lock struct {
		SourceID   string `json:"source_id"`
		Provider   string `json:"provider"`
		Repository string `json:"repository"`
		Candidate  struct {
			Commit     string `json:"commit"`
			Tree       string `json:"tree"`
			TagRefName string `json:"tag_ref_name"`
		} `json:"candidate"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("decode admitted Pack Source lock %q: %w", source.ID, err)
	}
	if lock.SourceID != source.ID || lock.Provider != source.Provider || lock.Repository != source.Repository || lock.Candidate.Commit == "" || lock.Candidate.Tree == "" {
		return ProjectPackSourceIdentity{}, fmt.Errorf("admitted Pack Source lock %q lacks an exact immutable identity", source.ID)
	}
	digest := sha256.Sum256(lockData)
	return ProjectPackSourceIdentity{
		PackID: pack.ID, PackVersion: pack.Version, ManifestSchema: pack.manifestVersion,
		SourceID: source.ID, Provider: source.Provider, Repository: source.Repository,
		Commit: lock.Candidate.Commit, Tree: lock.Candidate.Tree, Reference: lock.Candidate.TagRefName,
		SourceLockSHA256: hex.EncodeToString(digest[:]),
	}, nil
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
	return "", fmt.Errorf("current directory %q is outside a Git worktree", currentDirectory)
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
