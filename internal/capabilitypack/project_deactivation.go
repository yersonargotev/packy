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
)

const ProjectDeactivationPreviewSchemaVersion = 3

type ProjectDeactivationDisposition string

const (
	ProjectDeactivationPreviewable ProjectDeactivationDisposition = "previewable"
	ProjectDeactivationConverged   ProjectDeactivationDisposition = "converged"
	ProjectDeactivationBlocked     ProjectDeactivationDisposition = "blocked"
)

type ProjectDeactivationRequest struct {
	PackID      string
	Surface     Surface
	ProjectRoot string
	PackyHome   string
	Adapter     SurfaceAdapter
}

type JSONProjectDeactivationPreview struct {
	SensitiveLockIdentity string                         `json:"sensitive_lock_identity,omitempty"`
	SchemaVersion         int                            `json:"schema_version"`
	Report                string                         `json:"report"`
	DryRun                bool                           `json:"dry_run"`
	ProjectRoot           string                         `json:"project_root"`
	Pack                  ProjectManifestPack            `json:"pack"`
	Surface               Surface                        `json:"surface"`
	Runtime               ProjectRuntimeState            `json:"runtime"`
	Blockers              []ProjectInstallBlocker        `json:"blockers"`
	Disposition           ProjectDeactivationDisposition `json:"disposition"`
	Digest                string                         `json:"digest"`
	projectRoot           string
	packyHome             string
	request               ProjectDeactivationRequest
}

type ProjectDeactivationApplyRequest struct {
	Preview                    JSONProjectDeactivationPreview
	Adapter                    SurfaceAdapter
	DestructiveCleanupApproved bool
}

type ProjectDeactivationApplyResult struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Status        string `json:"status"`
	Digest        string `json:"digest"`
}

func PreviewProjectDeactivation(ctx context.Context, request ProjectDeactivationRequest) (JSONProjectDeactivationPreview, error) {
	report := JSONProjectDeactivationPreview{
		SchemaVersion: ProjectDeactivationPreviewSchemaVersion, Report: "project-deactivation-preview", DryRun: true,
		ProjectRoot: "<project-root>", Surface: request.Surface, Blockers: []ProjectInstallBlocker{},
		projectRoot: request.ProjectRoot, packyHome: request.PackyHome, request: request,
	}
	if err := ValidateSurface(request.Surface); err != nil {
		return report, err
	}
	if request.ProjectRoot == "" || request.PackyHome == "" || request.PackID == "" || request.Adapter == nil {
		return report, errors.New("project deactivation preview requires the project root, Packy Home, pack, and surface adapter")
	}
	document, exists, err := loadProjectActivationDocumentForSurface(request.PackyHome, request.ProjectRoot, request.PackID, request.Surface)
	if err != nil {
		return report, err
	}
	if !exists {
		report.Pack = ProjectManifestPack{ID: request.PackID, Surfaces: []Surface{request.Surface}, Selection: ResourceSelection{Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}}
		report.Runtime, report.Disposition = ProjectRuntimePending, ProjectDeactivationConverged
		report.Digest = sealProjectDeactivationPreview(report)
		return report, nil
	}
	if document.State.PackID != request.PackID {
		return report, fmt.Errorf("personal project activation belongs to capability pack %q, not %q", document.State.PackID, request.PackID)
	}
	report.Pack = ProjectManifestPack{ID: document.State.PackID, Version: document.State.Version, Surfaces: []Surface{request.Surface}, Selection: ResourceSelection{Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}}
	report.SensitiveLockIdentity = document.SensitiveLockIdentity
	report.Runtime = ProjectRuntimeActive
	manifestMissing, manifestErr := projectPathMissing(filepath.Join(request.ProjectRoot, "packy.json"))
	lockMissing, lockErr := projectPathMissing(filepath.Join(request.ProjectRoot, "packy.lock.json"))
	if manifestErr != nil || lockErr != nil {
		return report, errors.Join(manifestErr, lockErr)
	}
	if manifestMissing && lockMissing {
		report.Runtime = ProjectRuntimeOrphaned
	} else if manifestMissing != lockMissing {
		report.Runtime = ProjectRuntimeBlocked
		report.Blockers = append(report.Blockers, ProjectInstallBlocker{Code: "project_contract_incomplete", Detail: "packy.json and packy.lock.json are not consistently present", Remediation: "restore or remove the shared project contract before retrying personal deactivation"})
	}
	if len(report.Blockers) > 0 {
		report.Disposition = ProjectDeactivationBlocked
	} else {
		report.Disposition = ProjectDeactivationPreviewable
	}
	report.Digest = sealProjectDeactivationPreview(report)
	return report, nil
}

func ApplyProjectDeactivation(ctx context.Context, request ProjectDeactivationApplyRequest) (ProjectDeactivationApplyResult, error) {
	preview := request.Preview
	if !request.DestructiveCleanupApproved {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation requires approval of the exact destructive-cleanup phase")
	}
	if preview.projectRoot == "" || preview.packyHome == "" || request.Adapter == nil {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation Apply requires the exact preview and surface adapter")
	}
	if preview.Disposition == ProjectDeactivationBlocked {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation preview is blocked")
	}
	if preview.Disposition == ProjectDeactivationConverged {
		return ProjectDeactivationApplyResult{SchemaVersion: 2, Report: "project-deactivation-apply", Status: "inactive", Digest: preview.Digest}, nil
	}
	fresh, err := PreviewProjectDeactivation(ctx, preview.request)
	if err != nil {
		return ProjectDeactivationApplyResult{}, err
	}
	if fresh.Digest != preview.Digest {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation preview is stale; run the deactivation preview again")
	}
	if err := removeProjectActivationRecord(preview.packyHome, preview.projectRoot, preview.Pack.ID, preview.Surface); err != nil {
		return ProjectDeactivationApplyResult{}, err
	}
	return ProjectDeactivationApplyResult{SchemaVersion: 2, Report: "project-deactivation-apply", Status: "inactive", Digest: preview.Digest}, nil
}

func removeProjectActivationRecord(packyHome, projectRoot, packID string, surface Surface) error {
	directory, err := projectActivationDirectory(packyHome, projectRoot)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(directory, projectActivationStateFile(packID, surface))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_ = os.Remove(directory)
	_ = os.Remove(filepath.Dir(directory))
	_ = os.Remove(packyHome)
	return nil
}

func sealProjectDeactivationPreview(preview JSONProjectDeactivationPreview) string {
	preview.ProjectRoot, preview.Digest, preview.projectRoot, preview.packyHome, preview.request = "", "", "", "", ProjectDeactivationRequest{}
	data, _ := json.Marshal(preview)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
