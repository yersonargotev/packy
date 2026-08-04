package opencode

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
	projections := make([]capabilitypack.ObservedProjection, 0, len(pack.Resources))
	unrepresentable := make([]capabilitypack.UnrepresentableResource, 0)
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
			continue
		}
		projection, represented, err := a.openCodeProjectProjection(pack, resource, projectRoot)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		if !represented {
			unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no OpenCode project-native representation in this installation preview", identity)})
			continue
		}
		projections = append(projections, projection)
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

func (a *SurfaceAdapter) openCodeProjectProjection(pack capabilitypack.Pack, resource capabilitypack.Resource, projectRoot string) (capabilitypack.ObservedProjection, bool, error) {
	identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
	name, bound := openCodeBindingName(resource, resource.Kind)
	switch resource.Kind {
	case "skill":
		if !bound || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		target := filepath.Join(projectRoot, ".agents", "skills", name)
		source := filepath.Join(a.bundleRoot, resource.Source)
		desired, err := localprojection.FingerprintCopiedTree(source)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, fmt.Errorf("fingerprint %s source: %w", identity, err)
		}
		observed, exists, err := projectTreeFingerprint(target)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, fmt.Errorf("inspect %s target: %w", identity, err)
		}
		return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "opencode-project/v1/shared-copied-skill-tree", ProjectionKey: "path:" + filepath.Clean(target), Shared: true, DiscoverableBy: []capabilitypack.Surface{capabilitypack.SurfaceCodex}, Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceOpenCode, Kind: capabilitypack.ActionCodexProjectSkillTree, Source: source, Target: target, Version: desired, Precondition: observed, Description: fmt.Sprintf("copy %s to the shared project skill tree", identity), PreviewOnly: true}}, true, nil
	case "instruction":
		if !bound || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		target := filepath.Join(projectRoot, "AGENTS.md")
		start, end := "<!-- packy:project:opencode:"+resource.ID+":start -->", "<!-- packy:project:opencode:"+resource.ID+":end -->"
		block := start + "\n" + strings.TrimSpace(string(content)) + "\n" + end
		return projectMarkedFileProjection(identity.String(), capabilitypack.ActionOpenCodeInstructionFile, target, block, start, end)
	case "agent":
		if !bound || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return projectRegularFileProjection(identity.String(), capabilitypack.ActionOpenCodeAgentFile, filepath.Join(projectRoot, ".opencode", "agents", name+".md"), openCodeAgentMarkdown(pack, resource, content))
	case "command":
		if !bound || resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return projectRegularFileProjection(identity.String(), capabilitypack.ActionOpenCodeCommandFile, filepath.Join(projectRoot, ".opencode", "commands", name+".md"), openCodeCommandMarkdown(pack, resource, content))
	case "asset":
		if resource.Source == "" {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return projectRegularFileProjection(identity.String(), capabilitypack.ActionOpenCodeAssetFile, filepath.Join(projectRoot, ".opencode", "assets", resource.ID, filepath.Base(resource.Source)), string(content))
	case "mcp_server":
		if !bound {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		target := filepath.Join(projectRoot, "opencode.json")
		current, err := readOptionalSurfaceFile(target)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		inspection, err := InspectMCPContent(current, target, resource.ID, resource.Command, resource.Args)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		merged, err := MergeMCPProjection(current, target, resource.ID, resource.Command, resource.Args)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return capabilitypack.ObservedProjection{ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: inspection.Exists, ObservedFingerprint: inspection.ObservedFingerprint, DesiredFingerprint: inspection.DesiredFingerprint, AdapterProvenance: "opencode-project/v1/mcp-config", Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceOpenCode, Kind: capabilitypack.ActionOpenCodeMCPConfig, Target: target, Content: merged, Command: resource.Command, Args: append([]string(nil), resource.Args...), FileMode: 0o644, Description: "merge the OpenCode project MCP definition", PreviewOnly: true}}, true, nil
	case "lifecycle":
		if !bound {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		// Keep the version-controlled hook definition inert. OpenCode project
		// activation remains an explicit personal action; cloning the project
		// must not execute Pack-provided lifecycle behavior.
		definition := struct {
			ID      string                 `json:"id"`
			Binding capabilitypack.Binding `json:"binding"`
		}{resource.ID, projectOpenCodeBinding(resource)}
		data, err := json.MarshalIndent(definition, "", "  ")
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		return projectRegularFileProjection(identity.String(), capabilitypack.ActionOpenCodeAssetFile, filepath.Join(projectRoot, ".opencode", "packy-hooks", name+".json"), string(append(data, '\n')))
	default:
		return capabilitypack.ObservedProjection{}, false, nil
	}
}

func projectOpenCodeBinding(resource capabilitypack.Resource) capabilitypack.Binding {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceOpenCode {
			return binding
		}
	}
	return capabilitypack.Binding{}
}

func projectRegularFileProjection(id string, kind capabilitypack.ProjectionActionKind, target, content string) (capabilitypack.ObservedProjection, bool, error) {
	observed, exists, err := localprojection.FingerprintPath(target)
	if err != nil {
		return capabilitypack.ObservedProjection{}, false, err
	}
	desired := localprojection.FingerprintBytes([]byte(content))
	return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "opencode-project/v1/copied-file", Action: capabilitypack.ProjectionAction{ID: id, Surface: capabilitypack.SurfaceOpenCode, Kind: kind, Target: target, Content: content, FileMode: 0o644, Precondition: observed, Description: "write OpenCode project projection " + id, PreviewOnly: true}}, true, nil
}

func projectMarkedFileProjection(id string, kind capabilitypack.ProjectionActionKind, target, block, start, end string) (capabilitypack.ObservedProjection, bool, error) {
	current, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return capabilitypack.ObservedProjection{}, false, err
	}
	text := string(current)
	fragment, found := projectExtractBlock(text, start, end)
	observed := "missing"
	if found {
		observed = localprojection.FingerprintBytes([]byte(fragment))
	}
	fileMode := uint32(0o644)
	precondition := "missing"
	if info, statErr := os.Lstat(target); statErr == nil && info.Mode().IsRegular() {
		fileMode = uint32(info.Mode().Perm())
		precondition = localprojection.FingerprintBytes(current)
	}
	content := block + "\n"
	if strings.TrimSpace(text) != "" {
		content = strings.TrimRight(text, "\n") + "\n\n" + block + "\n"
	}
	if found {
		content = strings.Replace(text, fragment, block, 1)
	}
	return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionPresent, Exists: found, ObservedFingerprint: observed, DesiredFingerprint: localprojection.FingerprintBytes([]byte(block)), AdapterProvenance: "opencode-project/v1/composable-instruction", Action: capabilitypack.ProjectionAction{ID: id, Surface: capabilitypack.SurfaceOpenCode, Kind: kind, Target: target, Content: content, FileMode: fileMode, Precondition: precondition, PreviewOnly: true}}, true, nil
}

func projectExtractBlock(content, start, end string) (string, bool) {
	startIndex := strings.Index(content, start)
	if startIndex < 0 || strings.Count(content, start) != 1 || strings.Count(content, end) != 1 {
		return "", false
	}
	relativeEnd := strings.Index(content[startIndex+len(start):], end)
	if relativeEnd < 0 {
		return "", false
	}
	return content[startIndex : startIndex+len(start)+relativeEnd+len(end)], true
}

func (a *SurfaceAdapter) inspectLockedProject(projectRoot string, pack capabilitypack.ProjectManifestPack, lock capabilitypack.ProjectLockProposal, goal capabilitypack.ProjectionGoal) (capabilitypack.SurfaceInspection, error) {
	if goal == "" {
		goal = capabilitypack.ProjectionPresent
	}
	bindings := make(map[capabilitypack.ResourceIdentity]string, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		if binding.Surface != "" && binding.Surface != capabilitypack.SurfaceOpenCode {
			continue
		}
		bindings[capabilitypack.ResourceIdentity{Kind: binding.Kind, ID: binding.ID}] = binding.Name
	}
	contributor := "surface:opencode:pack:" + pack.ID
	var projections []capabilitypack.ObservedProjection
	var revision []string
	for _, projection := range lock.Projections {
		if !capabilitypack.ProjectProjectionHasContributor(projection, contributor) {
			continue
		}
		name := bindings[projection.Resource]
		expected := ""
		switch projection.Resource.Kind {
		case "skill":
			expected = filepath.Join(".agents", "skills", name)
		case "instruction":
			expected = "AGENTS.md"
		case "agent":
			expected = filepath.Join(".opencode", "agents", name+".md")
		case "command":
			expected = filepath.Join(".opencode", "commands", name+".md")
		case "mcp_server":
			expected = "opencode.json"
		case "lifecycle":
			expected = filepath.Join(".opencode", "packy-hooks", name+".json")
		case "asset":
			expected = projection.Target
		}
		if expected == "" || (projection.Resource.Kind != "asset" && name == "") {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has no OpenCode project binding", projection.Resource)
		}
		if filepath.Clean(filepath.FromSlash(projection.Target)) != filepath.Clean(expected) {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s target %q does not match the re-derived OpenCode target %q", projection.Resource, projection.Target, filepath.ToSlash(expected))
		}
		target := filepath.Join(projectRoot, expected)
		observed, exists := "missing", false
		action := capabilitypack.ProjectionAction{ID: projection.Resource.String(), Surface: capabilitypack.SurfaceOpenCode, Target: target, PreviewOnly: true, Precondition: projection.DesiredFingerprint, Command: projection.Command, Args: append([]string(nil), projection.Args...)}
		var err error
		switch projection.Resource.Kind {
		case "skill":
			action.Kind = capabilitypack.ActionCodexProjectSkillTree
			observed, exists, err = projectTreeFingerprint(target)
		case "instruction":
			action.Kind = capabilitypack.ActionOpenCodeInstructionFile
			data, readErr := os.ReadFile(target)
			if readErr != nil && !os.IsNotExist(readErr) {
				err = readErr
			} else if readErr == nil {
				fragment, found := projectExtractBlock(string(data), "<!-- packy:project:opencode:"+projection.Resource.ID+":start -->", "<!-- packy:project:opencode:"+projection.Resource.ID+":end -->")
				exists = found
				if found {
					observed = localprojection.FingerprintBytes([]byte(fragment))
				}
			}
		case "mcp_server":
			action.Kind = capabilitypack.ActionOpenCodeMCPConfig
			content, readErr := readOptionalSurfaceFile(target)
			if readErr != nil {
				err = readErr
			} else {
				inspected, inspectErr := InspectMCPContent(content, target, projection.Resource.ID, projection.Command, projection.Args)
				err = inspectErr
				exists, observed = inspected.Exists, inspected.ObservedFingerprint
			}
		case "agent":
			action.Kind = capabilitypack.ActionOpenCodeAgentFile
			observed, exists, err = localprojection.FingerprintPath(target)
		case "command":
			action.Kind = capabilitypack.ActionOpenCodeCommandFile
			observed, exists, err = localprojection.FingerprintPath(target)
		default:
			action.Kind = capabilitypack.ActionOpenCodeAssetFile
			observed, exists, err = localprojection.FingerprintPath(target)
		}
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		item := capabilitypack.ObservedProjection{ID: projection.Resource.String(), Goal: goal, Exists: exists, ObservedFingerprint: observed, AdapterProvenance: "opencode-project/v1/locked-copy_tree", Action: action}
		if goal == capabilitypack.ProjectionPresent {
			item.DesiredFingerprint = projection.DesiredFingerprint
		} else {
			item.Action.Mode = capabilitypack.ProjectionDeleteTarget
			item.Action.Description = fmt.Sprintf("remove exact OpenCode project projection %s", projection.Resource)
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
