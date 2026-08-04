package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
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
	mcpPath := filepath.Join(projectRoot, ".mcp.json")
	mcpOriginal, err := readOptional(mcpPath)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	mcpDocument := append([]byte(nil), mcpOriginal...)
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
			continue
		}
		binding, bound := claudeBinding(resource)
		projection, represented, projectErr := a.claudeProjectProjection(pack, resource, binding, bound, projectRoot, instructionPath, &instructionDocument, mcpPath, &mcpDocument)
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
		case capabilitypack.ActionClaudeProjectMCP:
			projections[i].Action.Content = string(mcpDocument)
		}
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].ID < projections[j].ID })
	evidence, err := capabilitypack.UnverifiedRuntimeModeEvidence(pack, time.Unix(0, 0).UTC(), "project-install-preview")
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	return capabilitypack.SurfaceInspection{
		Revision:    localprojection.FingerprintBytes([]byte(projectRoot + "\x00" + string(instructionOriginal) + "\x00" + string(mcpOriginal))),
		Projections: projections, Unrepresentable: unrepresentable,
		Readiness: capabilitypack.ReadinessObservation{OptionalAuthorities: capabilitypack.UnknownOptionalAuthorities(pack)}, RuntimeModeEvidence: evidence,
	}, nil
}

func (a *SurfaceAdapter) claudeProjectProjection(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, bound bool, projectRoot, instructionPath string, instructionDocument *[]byte, mcpPath string, mcpDocument *[]byte) (capabilitypack.ObservedProjection, bool, error) {
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
		source := filepath.Join(a.bundleRoot, resource.Source)
		desired, err := localprojection.FingerprintCopiedTree(source)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, fmt.Errorf("fingerprint %s source: %w", identity, err)
		}
		target := filepath.Join(projectRoot, ".claude", "skills", binding.Name)
		observed, exists, err := claudeProjectTreeFingerprint(target)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "claude-project/v1/copied-skill-tree", Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceClaude, Kind: capabilitypack.ActionClaudeProjectSkillTree, Source: source, Target: target, Version: desired, Precondition: observed, PreviewOnly: true}}, true, nil
	case "command":
		if !bound || binding.Projection != "skill" || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		prompt, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return regular(capabilitypack.ActionClaudeProjectFile, filepath.Join(projectRoot, ".claude", "skills", binding.Name, "SKILL.md"), claudeCommandSkill(resource, binding.Name, prompt))
	case "agent":
		if !bound || binding.Projection != "agent" || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		if pack.ID == "addy" && pack.Version == "1.1.0" {
			content, err = renderAddyClaudeAgent(pack, resource, binding, content)
		} else {
			content, err = a.embedConsumerAssets(pack, resource, content)
			if err == nil && binding.AgentAuthority != nil {
				content = []byte(claudeAgentDocument(resource, binding.Name, *binding.AgentAuthority, content))
			}
		}
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return regular(capabilitypack.ActionClaudeProjectFile, filepath.Join(projectRoot, ".claude", "agents", binding.Name+".md"), string(content))
	case "asset":
		if resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return regular(capabilitypack.ActionClaudeProjectFile, filepath.Join(projectRoot, ".claude", "assets", resource.ID, "RESOURCE"), string(content))
	case "instruction":
		if !bound || binding.Projection != "instruction" || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
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
	case "mcp_server":
		if !bound || binding.Projection != "mcp_server" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		inspection, err := inspectProjectMCP(*mcpDocument, binding.Name, resource.Command, resource.Args)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		merged, err := mergeProjectMCP(*mcpDocument, binding.Name, resource.Command, resource.Args)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		*mcpDocument = merged
		return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: inspection.Exists, ObservedFingerprint: inspection.ObservedFingerprint, DesiredFingerprint: inspection.DesiredFingerprint, AdapterProvenance: "claude-project/v1/mcp-config", Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceClaude, Kind: capabilitypack.ActionClaudeProjectMCP, Target: mcpPath, Command: resource.Command, Args: append([]string(nil), resource.Args...), FileMode: 0o644, Precondition: projectFileFingerprint(mcpPath), PreviewOnly: true}}, true, nil
	case "lifecycle":
		if !bound || binding.Projection != "command_hook" || binding.Hook == nil {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		definition, err := json.MarshalIndent(binding.Hook, "", "  ")
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return regular(capabilitypack.ActionClaudeProjectFile, filepath.Join(projectRoot, ".claude", "packy-hooks", binding.Name+".json"), string(append(definition, '\n')))
	default:
		return capabilitypack.ObservedProjection{}, false, nil
	}
}

func (a *SurfaceAdapter) inspectLockedProject(projectRoot string, pack capabilitypack.ProjectManifestPack, lock capabilitypack.ProjectLockProposal, goal capabilitypack.ProjectionGoal) (capabilitypack.SurfaceInspection, error) {
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
	contributor := "surface:claude:pack:" + pack.ID
	var projections []capabilitypack.ObservedProjection
	var revision []string
	for _, locked := range lock.Projections {
		if !capabilitypack.ProjectProjectionHasContributor(locked, contributor) {
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
		observed, exists := "missing", false
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
				current, readErr := readOptional(target)
				if readErr != nil {
					err = readErr
				} else {
					action.Content, err = RemoveInstructionContribution(string(current), contribution)
					action.Precondition = localprojection.FingerprintBytes(current)
					action.FileMode = projectFileMode(target)
					action.Mode = capabilitypack.ProjectionRemoveContent
					if strings.TrimSpace(action.Content) == "" {
						action.Mode = capabilitypack.ProjectionDeleteTarget
					}
				}
			}
		case "mcp_server":
			action.Kind = capabilitypack.ActionClaudeProjectMCP
			current, readErr := readOptional(target)
			if readErr != nil {
				err = readErr
				break
			}
			inspection, inspectErr := inspectProjectMCP(current, binding.Name, locked.Command, locked.Args)
			if inspectErr != nil {
				err = inspectErr
				break
			}
			observed, exists = inspection.ObservedFingerprint, inspection.Exists
			if goal == capabilitypack.ProjectionAbsent && exists {
				removed, removeErr := removeProjectMCP(current, binding.Name)
				err = removeErr
				action.Content, action.Precondition, action.FileMode, action.Mode = string(removed), localprojection.FingerprintBytes(current), projectFileMode(target), capabilitypack.ProjectionRemoveContent
			}
		default:
			action.Kind = capabilitypack.ActionClaudeProjectFile
			observed, exists, err = localprojection.FingerprintPath(target)
		}
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
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
	case "command":
		return filepath.Join(".claude", "skills", binding.Name, "SKILL.md")
	case "agent":
		return filepath.Join(".claude", "agents", binding.Name+".md")
	case "instruction":
		return "CLAUDE.md"
	case "mcp_server":
		return ".mcp.json"
	case "lifecycle":
		return filepath.Join(".claude", "packy-hooks", binding.Name+".json")
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

type projectMCPInspection struct {
	Exists              bool
	ObservedFingerprint string
	DesiredFingerprint  string
}

func projectMCPValue(command string, args []string) map[string]any {
	return map[string]any{"type": "stdio", "command": command, "args": append([]string(nil), args...)}
}

func inspectProjectMCP(content []byte, name, command string, args []string) (projectMCPInspection, error) {
	desired := canonicalFingerprint(projectMCPValue(command, args))
	if strings.TrimSpace(string(content)) == "" {
		return projectMCPInspection{ObservedFingerprint: "missing", DesiredFingerprint: desired}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		return projectMCPInspection{}, fmt.Errorf("invalid Claude project MCP JSON: %w", err)
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if root["mcpServers"] != nil && !ok {
		return projectMCPInspection{}, fmt.Errorf("Claude project mcpServers must be an object")
	}
	value, exists := servers[name]
	if !exists {
		return projectMCPInspection{ObservedFingerprint: "missing", DesiredFingerprint: desired}, nil
	}
	return projectMCPInspection{Exists: true, ObservedFingerprint: canonicalFingerprint(value), DesiredFingerprint: desired}, nil
}

func mergeProjectMCP(content []byte, name, command string, args []string) ([]byte, error) {
	inspection, err := inspectProjectMCP(content, name, command, args)
	if err != nil {
		return nil, err
	}
	if inspection.Exists {
		if inspection.ObservedFingerprint != inspection.DesiredFingerprint {
			return nil, fmt.Errorf("Claude project MCP server %q already exists with unmanaged settings", name)
		}
		return append([]byte(nil), content...), nil
	}
	data := content
	if strings.TrimSpace(string(data)) == "" {
		data = []byte("{}\n")
	}
	server, _ := json.Marshal(projectMCPValue(command, args))
	root := JSONSpan{0, len(data)}
	servers, found, err := jsonField(data, root, "mcpServers")
	if err != nil {
		return nil, err
	}
	if !found {
		value, _ := json.Marshal(map[string]json.RawMessage{name: server})
		return insertObjectField(data, root, "mcpServers", value)
	}
	return insertObjectField(data, servers, name, server)
}

func removeProjectMCP(content []byte, name string) ([]byte, error) {
	if strings.TrimSpace(string(content)) == "" {
		return append([]byte(nil), content...), nil
	}
	root := JSONSpan{0, len(content)}
	servers, found, err := jsonField(content, root, "mcpServers")
	if err != nil || !found {
		return append([]byte(nil), content...), err
	}
	return removeObjectField(content, servers, name)
}
