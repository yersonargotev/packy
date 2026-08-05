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
	"strings"
)

const ProjectDeactivationPreviewSchemaVersion = 1

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
	SchemaVersion int                              `json:"schema_version"`
	Report        string                           `json:"report"`
	DryRun        bool                             `json:"dry_run"`
	ProjectRoot   string                           `json:"project_root"`
	Pack          ProjectManifestPack              `json:"pack"`
	Surface       Surface                          `json:"surface"`
	Runtime       ProjectRuntimeState              `json:"runtime"`
	Effects       []ProjectActivationEffectPreview `json:"effects"`
	Blockers      []ProjectInstallBlocker          `json:"blockers"`
	Disposition   ProjectDeactivationDisposition   `json:"disposition"`
	Digest        string                           `json:"digest"`
	projectRoot   string
	packyHome     string
	request       ProjectDeactivationRequest
	actions       []ProjectionAction
}

type ProjectDeactivationApplyRequest struct {
	Preview     JSONProjectDeactivationPreview
	Adapter     SurfaceAdapter
	Interactive bool
}

type ProjectDeactivationApplyResult struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Status        string `json:"status"`
	Digest        string `json:"digest"`
}

func PreviewProjectDeactivation(_ context.Context, request ProjectDeactivationRequest) (JSONProjectDeactivationPreview, error) {
	report := JSONProjectDeactivationPreview{
		SchemaVersion: ProjectDeactivationPreviewSchemaVersion, Report: "project-deactivation-preview", DryRun: true,
		ProjectRoot: "<project-root>", Surface: request.Surface, Effects: []ProjectActivationEffectPreview{}, Blockers: []ProjectInstallBlocker{},
		projectRoot: request.ProjectRoot, packyHome: request.PackyHome, request: request,
	}
	if request.ProjectRoot == "" || request.PackyHome == "" || request.PackID == "" || request.Adapter == nil {
		return report, errors.New("project deactivation preview requires the project root, Packy Home, pack, and surface adapter")
	}
	document, exists, err := loadProjectActivationDocumentForSurface(request.PackyHome, request.ProjectRoot, request.Surface)
	if err != nil {
		return report, err
	}
	if !exists {
		report.Pack = ProjectManifestPack{ID: request.PackID, Surfaces: []Surface{request.Surface}, Selection: ResourceSelection{Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
		report.Runtime, report.Disposition = ProjectRuntimePending, ProjectDeactivationConverged
		report.Digest = sealProjectDeactivationPreview(report)
		return report, nil
	}
	if document.State.PackID != request.PackID {
		return report, fmt.Errorf("personal project activation belongs to capability pack %q, not %q", document.State.PackID, request.PackID)
	}
	report.Pack = ProjectManifestPack{ID: document.State.PackID, Version: document.State.Version, Surfaces: []Surface{request.Surface}, Selection: ResourceSelection{Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
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
	if document.Recovery.Status != "clean" {
		report.Runtime = ProjectRuntimeRecoveryRequired
	}
	for _, receipt := range document.Effects {
		effect := ProjectActivationEffectPreview{Category: ProjectActivationTrust, Action: receipt.Action, Target: "<personal-host-path>", Identity: receipt.ContributionIdentity}
		if receipt.Action == ActionCodexProjectTrust {
			effect.Target = "<codex-home>/config.toml"
		}
		action, absent, blocker, inspectErr := projectDeactivationAction(receipt)
		if inspectErr != nil {
			return report, inspectErr
		}
		if absent {
			effect.Observation = digestJSON(struct {
				Receipt projectActivationEffectReceipt
				State   string
			}{Receipt: receipt, State: "absent"})
		} else if blocker == nil {
			effect.Observation = projectActivationActionObservation(action)
		}
		report.Effects = append(report.Effects, effect)
		if blocker != nil {
			report.Blockers = append(report.Blockers, *blocker)
			continue
		}
		if !absent {
			report.actions = append(report.actions, action)
		}
	}
	if len(report.Blockers) > 0 {
		report.Disposition = ProjectDeactivationBlocked
	} else {
		report.Disposition = ProjectDeactivationPreviewable
	}
	report.Digest = sealProjectDeactivationPreview(report)
	return report, nil
}

func projectDeactivationAction(receipt projectActivationEffectReceipt) (ProjectionAction, bool, *ProjectInstallBlocker, error) {
	info, err := os.Lstat(receipt.Target)
	if errors.Is(err, fs.ErrNotExist) {
		return ProjectionAction{}, true, nil, nil
	}
	if err != nil {
		return ProjectionAction{}, false, nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ProjectionAction{}, false, &ProjectInstallBlocker{Code: "personal_effect_drift", Target: receipt.Target, Detail: "the receipted personal target is not a regular file", Remediation: "restore the exact receipted contribution before retrying project deactivation"}, nil
	}
	data, err := os.ReadFile(receipt.Target)
	if err != nil {
		return ProjectionAction{}, false, nil, err
	}
	fragment, found := extractProjectContribution(string(data), receipt.StartMarker, receipt.EndMarker)
	if !found {
		if !strings.Contains(string(data), receipt.StartMarker) && !strings.Contains(string(data), receipt.EndMarker) {
			return ProjectionAction{}, true, nil, nil
		}
		return ProjectionAction{}, false, &ProjectInstallBlocker{Code: "personal_effect_drift", Target: receipt.Target, Detail: "the receipted personal contribution markers are incomplete", Remediation: "restore the exact receipted contribution before retrying project deactivation"}, nil
	}
	if fingerprintProjectBytes([]byte(fragment)) != receipt.ContributionIdentity || strings.Count(string(data), receipt.StartMarker) != 1 || strings.Count(string(data), receipt.EndMarker) != 1 {
		return ProjectionAction{}, false, &ProjectInstallBlocker{Code: "personal_effect_drift", Target: receipt.Target, Detail: "the receipted personal contribution differs from its exact activation evidence", Remediation: "restore the exact receipted contribution before retrying project deactivation"}, nil
	}
	return ProjectionAction{
		ID: "deactivate:" + string(receipt.Action), Kind: receipt.Action, Target: receipt.Target,
		Content: strings.Replace(string(data), fragment, "", 1), FileMode: uint32(info.Mode().Perm()), Precondition: fingerprintProjectBytes(data),
		Description: "remove the exact receipted personal project contribution",
	}, false, nil, nil
}

func ApplyProjectDeactivation(ctx context.Context, request ProjectDeactivationApplyRequest) (ProjectDeactivationApplyResult, error) {
	preview := request.Preview
	if !request.Interactive {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation requires interactive destructive consent")
	}
	if preview.projectRoot == "" || preview.packyHome == "" || request.Adapter == nil {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation Apply requires the exact preview and surface adapter")
	}
	if preview.Disposition == ProjectDeactivationBlocked {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation preview is blocked")
	}
	if preview.Disposition == ProjectDeactivationConverged {
		return ProjectDeactivationApplyResult{SchemaVersion: 1, Report: "project-deactivation-apply", Status: "inactive", Digest: preview.Digest}, nil
	}
	fresh, err := PreviewProjectDeactivation(ctx, preview.request)
	if err != nil {
		return ProjectDeactivationApplyResult{}, err
	}
	if fresh.Digest != preview.Digest {
		return ProjectDeactivationApplyResult{}, errors.New("project deactivation preview is stale; run the deactivation preview again")
	}
	document, _, err := loadProjectActivationDocumentForSurface(preview.packyHome, preview.projectRoot, preview.Surface)
	if err != nil {
		return ProjectDeactivationApplyResult{}, err
	}
	if err := saveProjectActivationRecords(preview.packyHome, preview.projectRoot, document.State, document.Approvals, document.Receipts, document.Effects, "applying"); err != nil {
		return ProjectDeactivationApplyResult{}, err
	}
	if actionErr := request.Adapter.ApplyProjections(ctx, fresh.actions); actionErr != nil {
		_ = saveProjectActivationRecords(preview.packyHome, preview.projectRoot, document.State, document.Approvals, document.Receipts, document.Effects, "required")
		return ProjectDeactivationApplyResult{}, actionErr
	}
	for _, receipt := range document.Effects {
		_, absent, blocker, inspectErr := projectDeactivationAction(receipt)
		if inspectErr != nil || !absent || blocker != nil {
			_ = saveProjectActivationRecords(preview.packyHome, preview.projectRoot, document.State, document.Approvals, document.Receipts, document.Effects, "required")
			if inspectErr != nil {
				return ProjectDeactivationApplyResult{}, inspectErr
			}
			return ProjectDeactivationApplyResult{}, errors.New("personal project contribution was not verified absent after deactivation")
		}
	}
	if err := removeProjectActivationRecord(preview.packyHome, preview.projectRoot, preview.Surface); err != nil {
		return ProjectDeactivationApplyResult{}, err
	}
	return ProjectDeactivationApplyResult{SchemaVersion: 1, Report: "project-deactivation-apply", Status: "inactive", Digest: preview.Digest}, nil
}

func removeProjectActivationRecord(packyHome, projectRoot string, surface Surface) error {
	directory, err := projectActivationDirectory(packyHome, projectRoot)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(directory, projectActivationStateFile(surface))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_ = os.Remove(directory)
	_ = os.Remove(filepath.Dir(directory))
	_ = os.Remove(packyHome)
	return nil
}

func sealProjectDeactivationPreview(preview JSONProjectDeactivationPreview) string {
	preview.ProjectRoot, preview.Digest, preview.projectRoot, preview.packyHome, preview.request, preview.actions = "", "", "", "", ProjectDeactivationRequest{}, nil
	data, _ := json.Marshal(preview)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
