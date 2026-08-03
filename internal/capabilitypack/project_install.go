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
)

type ProjectInstallRequest struct {
	PackID      string
	Surface     Surface
	ProjectRoot string
}

type ProjectSurfaceAdapter interface {
	InspectProject(context.Context, Pack, string) (ProjectSurfaceObservation, error)
}

type ProjectSurfaceObservation struct {
	Revision    string
	Projections []ProjectProjectionObservation
}

type ProjectProjectionObservation struct {
	Resource            ResourceIdentity
	Target              string
	Mode                string
	DesiredFingerprint  string
	ObservedFingerprint string
	Exists              bool
	Representable       bool
	Reason              string
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
	ID       string    `json:"id"`
	Version  string    `json:"version"`
	Surfaces []Surface `json:"surfaces"`
}

type ProjectLockProposal struct {
	Path                   string                  `json:"path"`
	SchemaVersion          int                     `json:"schema_version"`
	MinimumPackyCapability string                  `json:"minimum_packy_capability"`
	Source                 PackSourceIdentity      `json:"source"`
	ResourceGraph          ResourceGraph           `json:"resource_graph"`
	Projections            []ProjectProjectionPlan `json:"projections"`
}

type ProjectNoticesProposal struct {
	Path          string                      `json:"path"`
	Contributions []ProjectNoticeContribution `json:"contributions"`
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
}

type ProjectInstallNotActionableError struct{ Disposition ProjectInstallDisposition }

func (e ProjectInstallNotActionableError) Error() string {
	return fmt.Sprintf("project install preview is not actionable: %s", e.Disposition)
}

type ProjectInstallFreshness struct {
	Disposition ProjectInstallDisposition `json:"disposition"`
	Blockers    []ProjectInstallBlocker   `json:"blockers"`
}

func (f Facade) CheckProjectInstallFreshness(ctx context.Context, preview JSONProjectInstallPreview, adapter ProjectSurfaceAdapter) (ProjectInstallFreshness, error) {
	if preview.projectRoot == "" {
		return ProjectInstallFreshness{}, errors.New("project install preview no longer carries its sealed project root")
	}
	pack, err := f.catalog.ResolveIntentPack(preview.Pack.ID, preview.Pack.Version)
	if err != nil {
		return ProjectInstallFreshness{}, err
	}
	observation, err := adapter.InspectProject(ctx, pack, preview.projectRoot)
	if err != nil {
		return ProjectInstallFreshness{}, err
	}
	if observation.Revision != preview.Observation {
		return ProjectInstallFreshness{Disposition: ProjectInstallBlocked, Blockers: []ProjectInstallBlocker{{Code: "stale_observation", Detail: "project targets changed after the preview was created", Remediation: "run the install dry-run again to obtain a fresh preview"}}}, nil
	}
	return ProjectInstallFreshness{Disposition: preview.Disposition, Blockers: append([]ProjectInstallBlocker(nil), preview.Blockers...)}, nil
}

func (f Facade) PreviewProjectInstall(ctx context.Context, request ProjectInstallRequest, adapter ProjectSurfaceAdapter) (JSONProjectInstallPreview, error) {
	if request.Surface != SurfaceCodex {
		return JSONProjectInstallPreview{}, fmt.Errorf("project installation preview does not support CLI surface %q", request.Surface)
	}
	if request.ProjectRoot == "" {
		return JSONProjectInstallPreview{}, errors.New("project root is required")
	}
	pack, err := f.catalog.Show(request.PackID)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if !projectSupportsSurface(pack.Surfaces, request.Surface) {
		return JSONProjectInstallPreview{}, fmt.Errorf("capability pack %q does not support CLI surface %q", request.PackID, request.Surface)
	}
	observation, err := adapter.InspectProject(ctx, pack, request.ProjectRoot)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	graph := ResourceGraphFor(pack, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, false)
	projections := make([]ProjectProjectionPlan, 0, len(observation.Projections))
	blockers := make([]ProjectInstallBlocker, 0)
	for _, projection := range observation.Projections {
		if !projection.Representable {
			blockers = append(blockers, ProjectInstallBlocker{Code: "unrepresentable_resource", Resource: projection.Resource, Detail: projection.Reason, Remediation: "choose a surface with a declared project-native representation"})
			continue
		}
		state := "missing"
		if projection.Exists {
			state = "foreign"
			blockers = append(blockers, ProjectInstallBlocker{Code: "foreign_target", Resource: projection.Resource, Target: projection.Target, Detail: "project target already exists without portable Packy ownership", Remediation: "move or remove the foreign target before installation"})
		}
		projections = append(projections, ProjectProjectionPlan{Resource: projection.Resource, Target: projection.Target, Mode: projection.Mode, DesiredFingerprint: projection.DesiredFingerprint, ObservedState: state, Contributor: "surface:" + string(request.Surface) + ":pack:" + pack.ID})
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
	manifestPack := ProjectManifestPack{ID: pack.ID, Version: pack.Version, Surfaces: []Surface{request.Surface}}
	disposition := ProjectInstallPreviewable
	if len(blockers) > 0 {
		disposition = ProjectInstallBlocked
	}
	report := JSONProjectInstallPreview{
		SchemaVersion: ProjectInstallPreviewSchemaVersion, Report: "project-install-preview", DryRun: true,
		ProjectRoot: "<project-root>", Pack: manifestPack, Surface: request.Surface, projectRoot: request.ProjectRoot,
		Selection:   ProjectSelectionPreview{Mode: SelectionAll, Resources: graph.Resources},
		Manifest:    ProjectContractProposal{Path: "packy.json", SchemaVersion: 1, Packs: []ProjectManifestPack{manifestPack}},
		Lock:        ProjectLockProposal{Path: "packy.lock.json", SchemaVersion: 1, MinimumPackyCapability: "project-installation-v1", Source: PackSourceIdentity{PackID: pack.ID, Version: pack.Version, SchemaVersion: pack.manifestVersion, Limitation: packSourceIdentityLimitation}, ResourceGraph: graph, Projections: append([]ProjectProjectionPlan(nil), projections...)},
		Notices:     ProjectNoticesProposal{Path: "PACKY-NOTICES.md", Contributions: notices},
		Projections: projections, Requirements: requirements, Blockers: blockers, Disposition: disposition, Observation: observation.Revision,
	}
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

func ProjectObservationRevision(value any) string {
	data, _ := json.Marshal(value)
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
	return "", fmt.Errorf("current directory %q is outside a Git worktree", currentDirectory)
}

func validGitWorktreeMarker(root, marker string, info os.FileInfo) (bool, error) {
	if info.IsDir() {
		return true, nil
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
	gitInfo, err := os.Stat(gitDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect linked Git worktree directory: %w", err)
	}
	return gitInfo.IsDir(), nil
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
