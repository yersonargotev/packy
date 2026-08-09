package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// ProjectPackUninstallRequest composes shared project removal with every
// required personal runtime deactivation for one installed Pack.
type ProjectPackUninstallRequest struct {
	PackID           string
	ProjectRoot      string
	PackyHome        string
	UninstallAdapter SurfaceAdapter
	RuntimeAdapters  map[Surface]SurfaceAdapter
}

type ProjectPackUninstallPreview struct {
	ProjectRoot   string                           `json:"project_root"`
	Pack          ProjectManifestPack              `json:"pack"`
	Disposition   ProjectInstallDisposition        `json:"disposition"`
	Uninstall     JSONProjectUninstallPreview      `json:"uninstall"`
	Deactivations []JSONProjectDeactivationPreview `json:"deactivations"`
	Observation   string                           `json:"observation"`
	request       ProjectPackUninstallRequest
}

type ProjectPackUninstallApplyRequest struct {
	Preview                    ProjectPackUninstallPreview
	DestructiveCleanupApproved bool
}

type ProjectPackUninstallApplyResult struct {
	Status           string
	Stage            string
	RollbackVerified bool
	PendingSurfaces  []Surface
}

func PreviewProjectPackUninstall(ctx context.Context, request ProjectPackUninstallRequest) (ProjectPackUninstallPreview, error) {
	preview := ProjectPackUninstallPreview{ProjectRoot: "<project-root>", request: request}
	if request.PackID == "" || request.ProjectRoot == "" || request.PackyHome == "" || request.UninstallAdapter == nil {
		return preview, errors.New("project Pack uninstall preview requires the Pack, project root, Packy Home, and uninstall adapter")
	}
	installation, err := LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return preview, err
	}
	installed := false
	for _, pack := range installation.Manifest.Packs {
		if pack.ID == request.PackID {
			preview.Pack = pack
			installed = true
			break
		}
	}
	if !installed {
		return preview, fmt.Errorf("capability pack %q is not declared by this project installation", request.PackID)
	}
	for _, surface := range preview.Pack.Surfaces {
		adapter := request.RuntimeAdapters[surface]
		if adapter == nil {
			return preview, fmt.Errorf("project Pack uninstall is missing the %s personal runtime adapter", surface)
		}
		deactivation, previewErr := PreviewProjectDeactivation(ctx, ProjectDeactivationRequest{
			PackID: request.PackID, Surface: surface, ProjectRoot: request.ProjectRoot, PackyHome: request.PackyHome, Adapter: adapter,
		})
		if previewErr != nil {
			return preview, previewErr
		}
		preview.Deactivations = append(preview.Deactivations, deactivation)
	}
	preview.Uninstall, err = PreviewProjectUninstall(ctx, ProjectUninstallRequest{PackID: request.PackID, ProjectRoot: request.ProjectRoot}, request.UninstallAdapter)
	if err != nil {
		return preview, err
	}
	preview.Disposition = preview.Uninstall.Disposition
	for _, deactivation := range preview.Deactivations {
		if deactivation.Disposition == ProjectDeactivationBlocked {
			preview.Disposition = ProjectInstallBlocked
		}
	}
	preview.Observation = sealProjectPackUninstallPreview(preview)
	return preview, nil
}

func ApplyProjectPackUninstall(ctx context.Context, request ProjectPackUninstallApplyRequest) (ProjectPackUninstallApplyResult, error) {
	preview := request.Preview
	if !request.DestructiveCleanupApproved {
		return ProjectPackUninstallApplyResult{Stage: "approval"}, errors.New("project Pack uninstall requires approval of the exact destructive-cleanup phase")
	}
	if preview.request.UninstallAdapter == nil || preview.Observation == "" {
		return ProjectPackUninstallApplyResult{Stage: "revalidation"}, errors.New("project Pack uninstall Apply requires the exact preview")
	}
	fresh, err := PreviewProjectPackUninstall(ctx, preview.request)
	if err != nil {
		return ProjectPackUninstallApplyResult{Stage: "revalidation"}, err
	}
	if fresh.Observation != preview.Observation {
		return ProjectPackUninstallApplyResult{Stage: "revalidation"}, errors.New("project Pack uninstall preview is stale; create a fresh preview before Apply")
	}
	if fresh.Disposition != ProjectInstallPreviewable {
		return ProjectPackUninstallApplyResult{Stage: "revalidation"}, ProjectUninstallNotActionableError{Disposition: fresh.Disposition}
	}
	applied, err := ApplyProjectUninstall(ctx, ProjectUninstallApplyRequest{Preview: fresh.Uninstall, PackyHome: fresh.request.PackyHome, Adapter: fresh.request.UninstallAdapter})
	if err != nil {
		result := ProjectPackUninstallApplyResult{Stage: "apply"}
		var mutationErr ProjectMutationError
		if errors.As(err, &mutationErr) {
			result.RollbackVerified = mutationErr.RollbackVerified
		}
		return result, err
	}
	if applied.Status != "verified" {
		return ProjectPackUninstallApplyResult{Stage: "verification"}, errors.New("project uninstall was not verified")
	}
	for _, approved := range fresh.Deactivations {
		if approved.Disposition == ProjectDeactivationConverged {
			continue
		}
		adapter := fresh.request.RuntimeAdapters[approved.Surface]
		current, previewErr := PreviewProjectDeactivation(ctx, ProjectDeactivationRequest{
			PackID: fresh.Pack.ID, Surface: approved.Surface, ProjectRoot: fresh.request.ProjectRoot, PackyHome: fresh.request.PackyHome, Adapter: adapter,
		})
		if previewErr != nil || projectDeactivationEffectsDigest(current) != projectDeactivationEffectsDigest(approved) {
			if previewErr == nil {
				previewErr = errors.New("personal deactivation effects changed after project-owned removal")
			}
			return ProjectPackUninstallApplyResult{Status: "partial", Stage: "verification", PendingSurfaces: []Surface{approved.Surface}}, previewErr
		}
		deactivated, deactivationErr := ApplyProjectDeactivation(ctx, ProjectDeactivationApplyRequest{Preview: current, Adapter: adapter, DestructiveCleanupApproved: true})
		if deactivationErr != nil || deactivated.Status != "inactive" {
			if deactivationErr == nil {
				deactivationErr = errors.New("personal project deactivation was not verified")
			}
			return ProjectPackUninstallApplyResult{Status: "partial", Stage: "verification", PendingSurfaces: []Surface{approved.Surface}}, deactivationErr
		}
	}
	return ProjectPackUninstallApplyResult{Status: "verified", Stage: "verification"}, nil
}

func sealProjectPackUninstallPreview(preview ProjectPackUninstallPreview) string {
	preview.ProjectRoot, preview.Observation, preview.request = "", "", ProjectPackUninstallRequest{}
	data, _ := json.Marshal(preview)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectDeactivationEffectsDigest(preview JSONProjectDeactivationPreview) string {
	data, _ := json.Marshal(preview.Effects)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
