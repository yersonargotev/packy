package agents

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
	projections := make([]capabilitypack.ObservedProjection, 0, len(pack.Resources))
	unrepresentable := make([]capabilitypack.UnrepresentableResource, 0)
	instructionTarget := filepath.Join(projectRoot, "AGENTS.md")
	instructionBytes, instructionErr := os.ReadFile(instructionTarget)
	if instructionErr != nil && !os.IsNotExist(instructionErr) {
		return capabilitypack.SurfaceInspection{}, instructionErr
	}
	instructionDocument := string(instructionBytes)
	instructionPrecondition := "missing"
	templateNames := capabilitypack.SkillTemplateNames(pack, capabilitypack.SurfaceAgents)
	if instructionErr == nil {
		instructionPrecondition = localprojection.FingerprintBytes(instructionBytes)
	}
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
			continue
		}
		projectInstruction, hasProjectInstruction := resource.SurfaceCapability(capabilitypack.SurfaceAgents, capabilitypack.SurfaceCapabilityProjectInstruction)
		if hasProjectInstruction {
			data := projectInstruction.ProjectInstruction
			content, err := os.ReadFile(filepath.Join(resource.CatalogRootOr(a.catalogRoot), data.Source))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read project instruction capability %q source: %w", data.ID, err)
			}
			instructionIdentity := capabilitypack.ResourceIdentity{Kind: "instruction", ID: data.ID}
			projection := agentsProjectInstructionProjection(instructionIdentity, instructionTarget, string(content), instructionDocument, instructionPrecondition)
			projections = append(projections, projection)
			if projection.Action.Content != "" {
				instructionDocument = projection.Action.Content
			}
		}
		if resource.Kind == "instruction" {
			if !hasProjectInstruction {
				content, err := os.ReadFile(filepath.Join(resource.CatalogRootOr(a.catalogRoot), resource.Source))
				if err != nil {
					return capabilitypack.SurfaceInspection{}, fmt.Errorf("read project instruction %q: %w", resource.ID, err)
				}
				projection := agentsProjectInstructionProjection(identity, instructionTarget, string(content), instructionDocument, instructionPrecondition)
				projections = append(projections, projection)
				if projection.Action.Content != "" {
					instructionDocument = projection.Action.Content
				}
			}
			continue
		}
		bindingName, bound := agentsProjectBinding(resource)
		if !bound || resource.Kind != "skill" || resource.Source == "" {
			unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no Agents project-native representation in this installation preview", identity)})
			continue
		}
		target := filepath.Join(projectRoot, ".agents", "skills", bindingName)
		source := filepath.Join(resource.CatalogRootOr(a.catalogRoot), resource.Source)
		if resource.RequestsSurfaceCapability(capabilitypack.SurfaceAgents, capabilitypack.SurfaceCapabilitySkillTemplate) {
			files, desired, err := localprojection.RenderSkillTemplate(source, resource.ID, templateNames, 0o755)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("render %s source: %w", identity, err)
			}
			observed, exists, err := projectTreeFingerprint(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("inspect %s target: %w", identity, err)
			}
			provenance := skilltemplate.RendererVersion + "/agents/project"
			projections = append(projections, capabilitypack.ObservedProjection{
				ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists,
				ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: provenance,
				ProjectionKey: "path:" + filepath.Clean(target), Shared: true,
				Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceAgents, Kind: capabilitypack.ActionAgentsProjectSkillTree, FileMode: 0o755, Target: target, Version: desired, Precondition: observed, AdapterProvenance: provenance, TreeFiles: files, Description: fmt.Sprintf("render %s in the Agents project skill tree", identity), PreviewOnly: true},
			})
			continue
		}
		desired, err := localprojection.FingerprintCopiedTree(source, 0o755)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("fingerprint %s source: %w", identity, err)
		}
		observed, exists, err := projectTreeFingerprint(target)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("inspect %s target: %w", identity, err)
		}
		projections = append(projections, capabilitypack.ObservedProjection{
			ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists,
			ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "agents-project/v1/copied-skill-tree",
			ProjectionKey: "path:" + filepath.Clean(target), Shared: true,
			Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceAgents, Kind: capabilitypack.ActionAgentsProjectSkillTree, FileMode: 0o755, Source: source, Target: target, Version: desired, Precondition: observed, Description: fmt.Sprintf("copy %s to the Agents project skill tree", identity), PreviewOnly: true},
		})
	}
	// Every contribution writes the complete composed document, independently of
	// projection sorting. Malformed or ambiguous contributions keep no write plan.
	for i := range projections {
		if projections[i].Action.Kind == capabilitypack.ActionInstructionFile && projections[i].Action.Content != "" {
			projections[i].Action.Content = instructionDocument
		}
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].ID < projections[j].ID })
	evidence, err := capabilitypack.UnverifiedRuntimeModeEvidence(pack, time.Unix(0, 0).UTC(), "project-install-preview")
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	return capabilitypack.SurfaceInspection{
		Revision: localprojection.FingerprintBytes([]byte(projectRoot)), Projections: projections, Unrepresentable: unrepresentable,
		Readiness: capabilitypack.ReadinessObservation{OptionalAuthorities: capabilitypack.UnknownOptionalAuthorities(pack)}, RuntimeModeEvidence: evidence,
	}, nil
}

func agentsProjectInstructionProjection(identity capabilitypack.ResourceIdentity, target, source, document, precondition string) capabilitypack.ObservedProjection {
	start, end := projectInstructionMarkers(identity.ID)
	block := start + "\n" + strings.TrimSpace(source) + "\n" + end
	state, fragment := projectInstructionMarkerState(document, start, end)
	desired := localprojection.FingerprintBytes([]byte(block))
	observed := "missing"
	exists := state != "missing"
	if fragment != "" {
		observed = localprojection.FingerprintBytes([]byte(fragment))
	} else if exists {
		observed = state + ":" + localprojection.FingerprintBytes([]byte(document))
	}
	replaceable := state == "intact"
	if state == "intact" && observed != desired {
		state = "changed"
	}
	content := ""
	if state == "missing" {
		content = mergeBlock(document, block, start, end)
	} else if replaceable {
		content = strings.Replace(document, fragment, block, 1)
	}
	fileMode := uint32(0o644)
	if info, err := os.Lstat(target); err == nil && info.Mode().IsRegular() {
		fileMode = uint32(info.Mode().Perm())
	}
	key := "path:" + filepath.Clean(target) + "#packy-project-instruction:" + identity.ID
	return capabilitypack.ObservedProjection{
		ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed,
		DesiredFingerprint: desired, AdapterProvenance: "agents-project/v1/shared-composable-instruction/" + state,
		ProjectionKey: key, Shared: true,
		Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceAgents, Kind: capabilitypack.ActionInstructionFile, Target: target, Content: content, FileMode: fileMode, Precondition: precondition, ProjectionKey: key, Shared: true, Description: fmt.Sprintf("merge %s into the shared project AGENTS.md", identity), PreviewOnly: true},
	}
}

func projectInstructionMarkers(id string) (string, string) {
	return "<!-- packy:project:instruction:" + id + ":start -->", "<!-- packy:project:instruction:" + id + ":end -->"
}

// inspectLockedProject validates and translates a supported committed project
// lock without consulting Packy's catalog or original source trees.
func (a *SurfaceAdapter) inspectLockedProject(projectRoot string, pack capabilitypack.ProjectManifestPack, lock capabilitypack.ProjectLockProposal, goal capabilitypack.ProjectionGoal, inspectionScope capabilitypack.ProjectInspectionScope) (capabilitypack.SurfaceInspection, error) {
	if goal == "" {
		goal = capabilitypack.ProjectionPresent
	}
	bindings := make(map[capabilitypack.ResourceIdentity]string, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		if binding.Surface != "" && binding.Surface != capabilitypack.SurfaceAgents {
			continue
		}
		bindings[capabilitypack.ResourceIdentity{Kind: binding.Kind, ID: binding.ID}] = binding.Name
	}
	projections := make([]capabilitypack.ObservedProjection, 0, len(lock.Projections))
	var revision []string
	for _, projection := range lock.Projections {
		if projection.OwnerPack != pack.ID || projection.Surface != capabilitypack.SurfaceAgents {
			continue
		}
		expected := ""
		switch projection.Mode {
		case "copy_tree":
			name := bindings[projection.Resource]
			if projection.Resource.Kind != "skill" || name == "" {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has no Agents project binding", projection.Resource)
			}
			expected = filepath.Join(".agents", "skills", name)
		case "merge_marked_file":
			if projection.Resource.Kind != "instruction" {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has no supported Agents marked-file target", projection.Resource)
			}
			expected = "AGENTS.md"
		default:
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has unsupported Agents mode %q", projection.Resource, projection.Mode)
		}
		if filepath.Clean(filepath.FromSlash(projection.Target)) != filepath.Clean(expected) {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s target %q does not match the re-derived Agents target %q", projection.Resource, projection.Target, filepath.ToSlash(expected))
		}
		target := filepath.Join(projectRoot, expected)
		if _, err := capabilitypack.RelativeProjectTarget(projectRoot, target); err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has unsafe Agents target: %w", projection.Resource, err)
		}
		observed, exists := "missing", false
		action := capabilitypack.ProjectionAction{ID: projection.Resource.String(), Surface: capabilitypack.SurfaceAgents, Target: target, PreviewOnly: true}
		if projection.Mode == "copy_tree" {
			fingerprint, found, err := projectTreeFingerprint(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			observed, exists = fingerprint, found
			action.Kind = capabilitypack.ActionAgentsProjectSkillTree
			action.Precondition = projection.DesiredFingerprint

		} else {
			start, end := projectInstructionMarkers(projection.Resource.ID)
			current, readErr := os.ReadFile(target)
			if readErr != nil && !os.IsNotExist(readErr) {
				return capabilitypack.SurfaceInspection{}, readErr
			}
			fragment, found := extractBlock(string(current), start, end)
			exists = found
			if found {
				observed = localprojection.FingerprintBytes([]byte(fragment))
			}
			action.Kind = capabilitypack.ActionInstructionFile
			if goal == capabilitypack.ProjectionAbsent && exists && observed == projection.DesiredFingerprint {
				remaining := strings.Replace(string(current), fragment, "", 1)
				info, statErr := os.Lstat(target)
				if statErr != nil || !info.Mode().IsRegular() {
					return capabilitypack.SurfaceInspection{}, fmt.Errorf("Agents project instruction target is not a regular file")
				}
				action.Content, action.FileMode, action.Precondition = remaining, uint32(info.Mode().Perm()), localprojection.FingerprintBytes(current)
				if strings.TrimSpace(remaining) == "" {
					action.Content, action.Mode = "", capabilitypack.ProjectionDeleteTarget
				} else {
					action.Mode = capabilitypack.ProjectionRemoveContent
				}
			}
		}
		item := capabilitypack.ObservedProjection{
			ID: projection.Resource.String(), Goal: goal, Exists: exists, ObservedFingerprint: observed,
			AdapterProvenance: "agents-project/v1/locked-" + projection.Mode, Action: action,
		}
		if goal == capabilitypack.ProjectionPresent {
			item.DesiredFingerprint = projection.DesiredFingerprint
		} else {
			if action.Mode == "" {
				action.Mode = capabilitypack.ProjectionDeleteTarget
				item.Action = action
			}
			item.Action.Description = fmt.Sprintf("remove exact Agents project projection %s", projection.Resource)
		}
		projections = append(projections, item)
		revision = append(revision, item.ID+"="+observed+"\x00"+action.Precondition)
	}
	sort.Strings(revision)
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revision, "\n"))), Projections: projections}, nil
}

func projectTreeFingerprint(target string) (string, bool, error) {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return localprojection.FingerprintPath(target)
	}
	fingerprint, err := localprojection.FingerprintExactTree(target)
	return fingerprint, true, err
}

func projectInstructionMarkerState(content, start, end string) (string, string) {
	starts, ends := strings.Count(content, start), strings.Count(content, end)
	if starts == 0 && ends == 0 {
		return "missing", ""
	}
	if starts > 1 || ends > 1 {
		return "ambiguous", ""
	}
	if starts != 1 || ends != 1 {
		return "malformed", ""
	}
	fragment, found := extractBlock(content, start, end)
	if !found {
		return "malformed", ""
	}
	return "intact", fragment
}

func agentsProjectBinding(resource capabilitypack.Resource) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceAgents && binding.Name != "" {
			return binding.Name, true
		}
	}
	return "", false
}
