package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	instructionPath := filepath.Join(projectRoot, "AGENTS.md")
	instructionBytes, err := os.ReadFile(instructionPath)
	if err != nil && !os.IsNotExist(err) {
		return capabilitypack.SurfaceInspection{}, err
	}
	instructionDocument := string(instructionBytes)
	instructionPrecondition := "missing"
	if err == nil {
		instructionPrecondition = localprojection.FingerprintBytes(instructionBytes)
	}
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
			continue
		}
		projection, represented, err := a.openCodeProjectProjection(pack, resource, projectRoot, instructionDocument, instructionPrecondition)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		if !represented {
			unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no OpenCode project-native representation in this installation preview", identity)})
			continue
		}
		projections = append(projections, projection)
		if resource.Kind == "instruction" {
			instructionDocument = projection.Action.Content
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

func (a *SurfaceAdapter) openCodeProjectProjection(pack capabilitypack.Pack, resource capabilitypack.Resource, projectRoot, instructionDocument, instructionPrecondition string) (capabilitypack.ObservedProjection, bool, error) {
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
		projection, represented, err := projectMarkedFileProjectionFromContent(identity.String(), capabilitypack.ActionOpenCodeInstructionFile, target, block, start, end, instructionDocument)
		projection.Action.Precondition = instructionPrecondition
		return projection, represented, err
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
	projection, represented, err := projectMarkedFileProjectionFromContent(id, kind, target, block, start, end, string(current))
	if err == nil && len(current) > 0 {
		projection.Action.Precondition = localprojection.FingerprintBytes(current)
	}
	return projection, represented, err
}

func projectMarkedFileProjectionFromContent(id string, kind capabilitypack.ProjectionActionKind, target, block, start, end, text string) (capabilitypack.ObservedProjection, bool, error) {
	fragment, found := projectExtractBlock(text, start, end)
	observed := "missing"
	if found {
		observed = localprojection.FingerprintBytes([]byte(fragment))
	}
	fileMode := uint32(0o644)
	precondition := "missing"
	if info, statErr := os.Lstat(target); statErr == nil && info.Mode().IsRegular() {
		fileMode = uint32(info.Mode().Perm())
		precondition = localprojection.FingerprintBytes([]byte(text))
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
	instructionTarget := filepath.Join(projectRoot, "AGENTS.md")
	instructionOriginal, instructionReadErr := os.ReadFile(instructionTarget)
	if instructionReadErr != nil && !os.IsNotExist(instructionReadErr) {
		return capabilitypack.SurfaceInspection{}, instructionReadErr
	}
	instructionDocument := string(instructionOriginal)
	instructionPrecondition := "missing"
	if instructionReadErr == nil {
		instructionPrecondition = localprojection.FingerprintBytes(instructionOriginal)
	}
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
			start, end := "<!-- packy:project:opencode:"+projection.Resource.ID+":start -->", "<!-- packy:project:opencode:"+projection.Resource.ID+":end -->"
			fragment, found := projectExtractBlock(string(instructionOriginal), start, end)
			exists = found
			if found {
				observed = localprojection.FingerprintBytes([]byte(fragment))
				if goal == capabilitypack.ProjectionAbsent {
					instructionDocument = strings.TrimSpace(strings.Replace(instructionDocument, fragment, "", 1))
					if instructionDocument != "" {
						instructionDocument += "\n"
					}
					action.Content = instructionDocument
					action.FileMode = 0o644
					action.Precondition = instructionPrecondition
					action.Mode = capabilitypack.ProjectionRemoveContent
					if instructionDocument == "" {
						action.Mode = capabilitypack.ProjectionDeleteTarget
					}
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
			if item.Action.Mode == "" {
				item.Action.Mode = capabilitypack.ProjectionDeleteTarget
			}
			item.Action.Description = fmt.Sprintf("remove exact OpenCode project projection %s", projection.Resource)
		}
		projections = append(projections, item)
		revision = append(revision, item.ID+"="+observed+"\x00"+action.Precondition)
	}
	sort.Strings(revision)
	readiness := capabilitypack.ReadinessObservation{}
	if goal == capabilitypack.ProjectionPresent {
		readiness = inspectOpenCodeProjectRuntime(lock, projections)
	}
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revision, "\n"))), Projections: projections, Readiness: readiness}, nil
}

// inspectOpenCodeProjectRuntime deliberately has no activation actions. OpenCode
// stores project definitions in the checkout, but its runtime permission prompts,
// OAuth state, credentials, and any remembered approval remain host-owned. In
// particular, Packy only materializes lifecycle definitions as inert artifacts;
// it has no hook executor that could run changed bytes.
func inspectOpenCodeProjectRuntime(lock capabilitypack.ProjectLockProposal, projections []capabilitypack.ObservedProjection) capabilitypack.ReadinessObservation {
	var disclosures []capabilitypack.ProjectSensitiveDisclosure
	for _, disclosure := range lock.Sensitive {
		if disclosure.Surface == capabilitypack.SurfaceOpenCode {
			disclosures = append(disclosures, disclosure)
		}
	}
	if len(disclosures) == 0 {
		return capabilitypack.ReadinessObservation{}
	}

	definitionsMatch := true
	for _, projection := range projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			definitionsMatch = false
			break
		}
	}
	pending := make([]string, 0, len(disclosures))
	evidence := []string{"OpenCode project definitions were inspected against the lock; runtime permissions, trust, OAuth, and credentials are host-owned and not observable from project files"}
	if definitionsMatch {
		evidence = append(evidence, "OpenCode project definitions match the lock")
	} else {
		evidence = append(evidence, "OpenCode project definitions differ from the lock")
	}
	externalCommands := map[string]bool{}
	for _, disclosure := range disclosures {
		identity := disclosure.Resource.String()
		evidence = append(evidence, fmt.Sprintf("locked OpenCode %s effect %s (%s)", disclosure.Category, identity, disclosure.Detail))
		switch disclosure.Category {
		case capabilitypack.ProjectActivationMCP:
			pending = append(pending, "approve the locked OpenCode MCP definition "+identity+" in a fresh runtime session")
		case capabilitypack.ProjectActivationHooks:
			pending = append(pending, "approve the locked OpenCode hook "+identity+" in a fresh runtime session after its installed digest is verified")
			evidence = append(evidence, "OpenCode hook artifact remains inert; Packy does not execute hooks or grant runtime authority")
		case capabilitypack.ProjectActivationPlugins:
			pending = append(pending, "approve the locked OpenCode plugin "+identity+" in a fresh runtime session")
		case capabilitypack.ProjectActivationTrust:
			pending = append(pending, "approve the host-owned OpenCode authority for "+identity+" in a fresh runtime session")
		case capabilitypack.ProjectActivationAuthentication:
			pending = append(pending, "complete the host-owned OpenCode authentication for "+identity)
		case capabilitypack.ProjectActivationExternalRequirements:
			parts := strings.Split(disclosure.Detail, ":")
			if len(parts) >= 2 && parts[len(parts)-1] != "" {
				externalCommands[parts[len(parts)-1]] = true
			}
		}
	}
	for command := range externalCommands {
		if _, err := exec.LookPath(command); err != nil {
			pending = append(pending, "install external requirement "+command)
		} else {
			evidence = append(evidence, "external requirement available: "+command)
		}
	}
	for _, projection := range projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			pending = append(pending, "restore the exact locked OpenCode project definition "+projection.ID+" before activating runtime effects")
		}
	}
	return capabilitypack.ReadinessObservation{PendingHumanActions: pending, Evidence: evidence}
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
