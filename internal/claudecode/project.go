package claudecode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
	"github.com/yersonargotev/packy/internal/skilltemplate"
)

func (a *SurfaceAdapter) inspectProject(_ context.Context, pack capabilitypack.Pack, projectRoot string) (capabilitypack.SurfaceInspection, error) {
	var projections []capabilitypack.ObservedProjection
	var unrepresentable []capabilitypack.UnrepresentableResource
	instructionPath := filepath.Join(projectRoot, "CLAUDE.md")
	instructionOriginal, err := readOptional(instructionPath)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	instructionDocument := append([]byte(nil), instructionOriginal...)
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
			continue
		}
		binding, bound := claudeBinding(resource)
		projection, represented, projectErr := a.claudeProjectProjection(pack, resource, binding, bound, projectRoot, instructionPath, &instructionDocument)
		if projectErr != nil {
			return capabilitypack.SurfaceInspection{}, projectErr
		}
		if !represented {
			if resource.Kind != "asset" {
				unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no Claude project-native representation in this installation preview", identity)})
			}
			continue
		}
		projections = append(projections, projection)
	}
	for i := range projections {
		switch projections[i].Action.Kind {
		case capabilitypack.ActionClaudeProjectInstruction:
			projections[i].Action.Content = string(instructionDocument)
		}
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].ID < projections[j].ID })
	evidence, err := capabilitypack.UnverifiedRuntimeModeEvidence(pack, time.Unix(0, 0).UTC(), "project-install-preview")
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	return capabilitypack.SurfaceInspection{
		Revision:    localprojection.FingerprintBytes([]byte(projectRoot + "\x00" + string(instructionOriginal))),
		Projections: projections, Unrepresentable: unrepresentable,
		Readiness: capabilitypack.ReadinessObservation{OptionalAuthorities: capabilitypack.UnknownOptionalAuthorities(pack)}, RuntimeModeEvidence: evidence,
	}, nil
}

func (a *SurfaceAdapter) claudeProjectProjection(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, bound bool, projectRoot, instructionPath string, instructionDocument *[]byte) (capabilitypack.ObservedProjection, bool, error) {
	identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
	regular := func(kind capabilitypack.ProjectionActionKind, target, content string) (capabilitypack.ObservedProjection, bool, error) {
		observed, exists, err := localprojection.FingerprintPath(target)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		desired := localprojection.FingerprintBytes([]byte(content))
		return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "claude-project/v1/copied-file", Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceClaude, Kind: kind, Target: target, Content: content, FileMode: 0o644, Precondition: observed, PreviewOnly: true}}, true, nil
	}
	switch resource.Kind {
	case "skill":
		if !bound || binding.Projection != "skill" || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		if _, composite := resource.SurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilityClaudeCompositeSkill); composite {
			return a.claudeProjectCompositeProjection(pack, resource, binding, projectRoot)
		}
		source := filepath.Join(resource.CatalogRootOr(a.catalogRoot), resource.Source)
		if resource.RequestsSurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilitySkillTemplate) {
			files, desired, err := localprojection.RenderSkillTemplate(source, resource.ID, capabilitypack.SkillTemplateNames(pack, capabilitypack.SurfaceClaude), 0o755)
			if err != nil {
				return capabilitypack.ObservedProjection{}, false, fmt.Errorf("render %s source: %w", identity, err)
			}
			target := filepath.Join(projectRoot, ".claude", "skills", binding.Name)
			observed, exists, err := claudeProjectTreeFingerprint(target)
			if err != nil {
				return capabilitypack.ObservedProjection{}, false, err
			}
			provenance := skilltemplate.RendererVersion + "/claude/project"
			action := capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceClaude, Kind: capabilitypack.ActionClaudeProjectSkillTree, FileMode: 0o755, Target: target, Version: desired, Precondition: observed, AdapterProvenance: provenance, PreviewOnly: true, TreeFiles: files}
			return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: provenance, Action: action}, true, nil
		}
		desired, err := localprojection.FingerprintCopiedTree(source, 0o755)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, fmt.Errorf("fingerprint %s source: %w", identity, err)
		}
		target := filepath.Join(projectRoot, ".claude", "skills", binding.Name)
		observed, exists, err := claudeProjectTreeFingerprint(target)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "claude-project/v1/copied-skill-tree", Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceClaude, Kind: capabilitypack.ActionClaudeProjectSkillTree, FileMode: 0o755, Source: source, Target: target, Version: desired, Precondition: observed, PreviewOnly: true}}, true, nil
	case "agent":
		if !bound || binding.Projection != "agent" || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(resource.CatalogRootOr(a.catalogRoot), resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		if _, hasAgentDocument := resource.SurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilityClaudeAgentDocument); hasAgentDocument {
			content, err = renderClaudeAgentDocument(pack, resource, binding, content)
		} else if len(resource.Tools) > 0 || len(resource.Permissions) > 0 {
			err = fmt.Errorf("Claude agent %s is missing explicit authority translations", resource.ID)
		}
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		content, err = a.embedConsumerAssets(pack, resource, content)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return regular(capabilitypack.ActionClaudeProjectFile, filepath.Join(projectRoot, ".claude", "agents", binding.Name+".md"), string(content))
	case "asset":
		if resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(resource.CatalogRootOr(a.catalogRoot), resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return regular(capabilitypack.ActionClaudeProjectFile, filepath.Join(projectRoot, ".claude", "assets", resource.ID, "RESOURCE"), string(content))
	case "instruction":
		if !bound || binding.Projection != "instruction" || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(resource.CatalogRootOr(a.catalogRoot), resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		contributor := "pack:" + pack.ID + ":" + resource.ID
		merged, err := UpsertInstructionContribution(string(*instructionDocument), InstructionContribution{ContributorID: contributor, Content: string(content)})
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		*instructionDocument = []byte(merged)
		observed, exists := observeInstructionContribution(instructionPath, contributor)
		desired := Fingerprint([]byte(strings.TrimSpace(string(content))))
		return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "claude-project/v1/composable-instruction", Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceClaude, Kind: capabilitypack.ActionClaudeProjectInstruction, Target: instructionPath, FileMode: 0o644, Precondition: projectFileFingerprint(instructionPath), PreviewOnly: true}}, true, nil
	default:
		return capabilitypack.ObservedProjection{}, false, nil
	}
}

func (a *SurfaceAdapter) claudeProjectCompositeProjection(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, projectRoot string) (capabilitypack.ObservedProjection, bool, error) {
	identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
	composite, err := claudeCompositeSkill(pack, resource, binding, a.catalogRoot)
	if err != nil {
		return capabilitypack.ObservedProjection{}, false, err
	}
	target := filepath.Join(projectRoot, ".claude", "skills", binding.Name)
	observed, exists, err := claudeProjectTreeFingerprint(target)
	if err != nil {
		return capabilitypack.ObservedProjection{}, false, err
	}
	files := make([]capabilitypack.ProjectionTreeFile, len(composite.Files))
	for i, file := range composite.Files {
		files[i] = capabilitypack.ProjectionTreeFile{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: file.Mode}
	}
	fingerprint, err := fingerprintCompositeFiles(composite.Files, 0o755)
	if err != nil {
		return capabilitypack.ObservedProjection{}, false, err
	}
	action := capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceClaude, Kind: capabilitypack.ActionClaudeProjectSkillTree, FileMode: 0o755, Target: target, Version: fingerprint, Precondition: observed, PreviewOnly: true, TreeFiles: files}
	return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: fingerprint, AdapterProvenance: "claude-project/v1/composite-skill", Action: action}, true, nil
}

func (a *SurfaceAdapter) inspectLockedProject(ctx context.Context, projectRoot string, pack capabilitypack.ProjectManifestPack, lock capabilitypack.ProjectLockProposal, goal capabilitypack.ProjectionGoal, inspectionScope capabilitypack.ProjectInspectionScope) (capabilitypack.SurfaceInspection, error) {
	if goal == "" {
		goal = capabilitypack.ProjectionPresent
	}
	bindings := make(map[capabilitypack.ResourceIdentity]capabilitypack.LifecycleBinding, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		if binding.Surface != "" && binding.Surface != capabilitypack.SurfaceClaude {
			continue
		}
		bindings[capabilitypack.ResourceIdentity{Kind: binding.Kind, ID: binding.ID}] = binding
	}
	var projections []capabilitypack.ObservedProjection
	var revision []string
	instructionTarget := filepath.Join(projectRoot, "CLAUDE.md")
	instructionOriginal, err := readOptional(instructionTarget)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	instructionDocument := string(instructionOriginal)
	instructionPrecondition := projectFileFingerprint(instructionTarget)
	for _, locked := range lock.Projections {
		if locked.OwnerPack != pack.ID || locked.Surface != capabilitypack.SurfaceClaude {
			continue
		}
		binding := bindings[locked.Resource]
		expected := claudeLockedProjectTarget(locked, binding)
		if expected == "" || filepath.Clean(filepath.FromSlash(locked.Target)) != filepath.Clean(expected) {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s target %q does not match the re-derived Claude target %q", locked.Resource, locked.Target, filepath.ToSlash(expected))
		}
		target := filepath.Join(projectRoot, expected)
		if _, err := capabilitypack.RelativeProjectTarget(projectRoot, target); err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		action := capabilitypack.ProjectionAction{ID: locked.Resource.String(), Surface: capabilitypack.SurfaceClaude, Target: target, Command: locked.Command, Args: append([]string(nil), locked.Args...), PreviewOnly: true}
		var observed string
		var exists bool
		var err error
		switch locked.Resource.Kind {
		case "skill":
			action.Kind = capabilitypack.ActionClaudeProjectSkillTree
			observed, exists, err = claudeProjectTreeFingerprint(target)
		case "instruction":
			action.Kind = capabilitypack.ActionClaudeProjectInstruction
			contribution := "pack:" + pack.ID + ":" + locked.Resource.ID
			observed, exists = observeInstructionContribution(target, contribution)
			if goal == capabilitypack.ProjectionAbsent && exists {
				action.Content, err = RemoveInstructionContribution(instructionDocument, contribution)
				if err == nil {
					instructionDocument = action.Content
					action.Precondition = instructionPrecondition
					action.FileMode = projectFileMode(target)
					action.Mode = capabilitypack.ProjectionRemoveContent
					if strings.TrimSpace(action.Content) == "" {
						action.Mode = capabilitypack.ProjectionDeleteTarget
					}
				}
			}
		default:
			action.Kind = capabilitypack.ActionClaudeProjectFile
			observed, exists, err = localprojection.FingerprintPath(target)
		}
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		observed = strings.TrimPrefix(observed, "sha256:")
		item := capabilitypack.ObservedProjection{ID: locked.Resource.String(), Goal: goal, Exists: exists, ObservedFingerprint: observed, AdapterProvenance: "claude-project/v1/locked-" + locked.Mode, Action: action}
		if goal == capabilitypack.ProjectionPresent {
			item.DesiredFingerprint = locked.DesiredFingerprint
		} else if action.Mode == "" {
			item.Action.Mode = capabilitypack.ProjectionDeleteTarget
		}
		projections = append(projections, item)
		revision = append(revision, item.ID+"="+observed)
	}
	sort.Strings(revision)
	return capabilitypack.SurfaceInspection{Revision: Fingerprint([]byte(strings.Join(revision, "\n"))), Projections: projections}, nil
}

func claudeLockedProjectTarget(locked capabilitypack.ProjectProjectionPlan, binding capabilitypack.LifecycleBinding) string {
	switch locked.Resource.Kind {
	case "skill":
		return filepath.Join(".claude", "skills", binding.Name)
	case "agent":
		return filepath.Join(".claude", "agents", binding.Name+".md")
	case "instruction":
		return "CLAUDE.md"
	case "asset":
		return filepath.Join(".claude", "assets", locked.Resource.ID, "RESOURCE")
	default:
		return ""
	}
}

func claudeProjectTreeFingerprint(target string) (string, bool, error) {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if err != nil {
			return "", false, err
		}
		return localprojection.FingerprintPath(target)
	}
	fingerprint, err := localprojection.FingerprintExactTree(target)
	return fingerprint, true, err
}

func projectFileFingerprint(path string) string {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "unreadable"
	}
	return localprojection.FingerprintBytes(data)
}

func projectFileMode(path string) uint32 {
	info, err := os.Lstat(path)
	if err == nil && info.Mode().IsRegular() {
		return uint32(info.Mode().Perm())
	}
	return 0o644
}

func observeInstructionContribution(path, contributor string) (string, bool) {
	observation := ObserveInstructions(path)
	if observation.Err != nil {
		return "invalid", true
	}
	fingerprint, exists := observation.Contributions[contributor]
	if !exists {
		return "missing", false
	}
	return fingerprint, true
}
