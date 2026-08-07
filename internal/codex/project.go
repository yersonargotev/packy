package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

const codexProjectTrustProvenance = "codex-project/v1/project-trust"

func (a *SurfaceAdapter) inspectProjectDeactivation(projectRoot string, receipts []capabilitypack.ProjectActivationEffectReceipt) (capabilitypack.SurfaceInspection, error) {
	start, end, _, err := codexProjectTrustMarkers(projectRoot)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	effects := make([]capabilitypack.ObservedProjectEffect, 0, len(receipts))
	for _, receipt := range receipts {
		effect := capabilitypack.ObservedProjectEffect{Kind: receipt.Action, Target: receipt.Target, State: capabilitypack.ProjectEffectDrifted, AdapterProvenance: codexProjectTrustProvenance}
		if receipt.Action != capabilitypack.ActionCodexProjectTrust || receipt.Surface != capabilitypack.SurfaceCodex || receipt.AdapterProvenance != codexProjectTrustProvenance || filepath.Clean(receipt.Target) != filepath.Clean(a.configFile) || receipt.StartMarker != start || receipt.EndMarker != end {
			effects = append(effects, effect)
			continue
		}
		info, statErr := os.Lstat(a.configFile)
		if os.IsNotExist(statErr) {
			effect.State = capabilitypack.ProjectEffectAbsent
			effects = append(effects, effect)
			continue
		}
		if statErr != nil {
			return capabilitypack.SurfaceInspection{}, statErr
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			effects = append(effects, effect)
			continue
		}
		data, readErr := os.ReadFile(a.configFile)
		if readErr != nil {
			return capabilitypack.SurfaceInspection{}, readErr
		}
		effect.ObservedFingerprint = localprojection.FingerprintBytes(data)
		fragment, found := extractBlock(string(data), start, end)
		if !found {
			if !strings.Contains(string(data), start) && !strings.Contains(string(data), end) {
				effect.State = capabilitypack.ProjectEffectAbsent
			}
			effects = append(effects, effect)
			continue
		}
		if localprojection.FingerprintBytes([]byte(fragment)) != receipt.ContributionIdentity || strings.Count(string(data), start) != 1 || strings.Count(string(data), end) != 1 {
			effects = append(effects, effect)
			continue
		}
		effect.State = capabilitypack.ProjectEffectExact
		effect.Action = capabilitypack.ProjectionAction{
			ID: "deactivate:codex-project-trust", Surface: capabilitypack.SurfaceCodex, Kind: capabilitypack.ActionCodexProjectTrust,
			Target: a.configFile, Content: strings.Replace(string(data), fragment, "", 1), FileMode: uint32(info.Mode().Perm()), Precondition: effect.ObservedFingerprint,
			AdapterProvenance: codexProjectTrustProvenance, Consent: capabilitypack.ConsentDestructiveCleanup, PreviewOnly: true,
			Description: "remove the exact receipted Codex project trust contribution",
		}
		effects = append(effects, effect)
	}
	return capabilitypack.SurfaceInspection{ProjectDeactivationEffects: effects}, nil
}

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
	if instructionErr == nil {
		instructionPrecondition = localprojection.FingerprintBytes(instructionBytes)
	}
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
			continue
		}
		if resource.Kind == "mcp_server" {
			bindingName, bound := codexBindingName(resource, "mcp_server")
			if !bound || resource.Command == "" {
				unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no Codex project-native representation in this installation preview", identity)})
				continue
			}
			configFile := filepath.Join(projectRoot, ".codex", "config.toml")
			current, err := readOptionalFile(configFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projectResource := resource
			projectResource.ID = bindingName
			desiredBlock := mcpBlock(projectResource, resource.Command)
			start, end := mcpMarkers(bindingName)
			fragment, exists := extractBlock(current, start, end)
			observed := "missing"
			if exists {
				observed = localprojection.FingerprintBytes([]byte(fragment))
			} else if codexMCPTableExists(current, bindingName) {
				exists = true
				observed = localprojection.FingerprintBytes([]byte("unmanaged:" + bindingName))
			}
			desired := localprojection.FingerprintBytes([]byte(desiredBlock))
			projections = append(projections, capabilitypack.ObservedProjection{
				ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists,
				ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "codex-project/v1/marked-mcp",
				Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceCodex, Kind: capabilitypack.ActionCodexMCPConfig, Target: configFile, Content: mergeBlock(current, desiredBlock, start, end), FileMode: 0o644, Precondition: localprojection.FingerprintBytes([]byte(current)), Command: resource.Command, Args: append([]string(nil), resource.Args...), Description: fmt.Sprintf("configure %s in the Codex project", identity), PreviewOnly: true},
			})
			continue
		}
		if resource.Kind == "instruction" {
			if _, bound := codexProjectBinding(resource); !bound || resource.Source == "" {
				unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no Codex project-native representation in this installation preview", identity)})
				continue
			}
			content, err := os.ReadFile(filepath.Join(a.bundleRoot, resource.Source))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read %s source: %w", identity, err)
			}
			projection := codexProjectInstructionProjection(identity, instructionTarget, string(content), instructionDocument, instructionPrecondition)
			projections = append(projections, projection)
			if projection.Action.Content != "" {
				instructionDocument = projection.Action.Content
			}
			continue
		}
		bindingName, bound := codexProjectBinding(resource)
		if !bound || resource.Kind != "skill" || resource.Source == "" {
			unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: identity, Reason: fmt.Sprintf("%s has no Codex project-native representation in this installation preview", identity)})
			continue
		}
		target := filepath.Join(projectRoot, ".agents", "skills", bindingName)
		source := filepath.Join(a.bundleRoot, resource.Source)
		desired, err := localprojection.FingerprintCopiedTree(source)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("fingerprint %s source: %w", identity, err)
		}
		observed, exists, err := projectTreeFingerprint(target)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("inspect %s target: %w", identity, err)
		}
		projections = append(projections, capabilitypack.ObservedProjection{
			ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists,
			ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: "codex-project/v1/copied-skill-tree",
			ProjectionKey: "path:" + filepath.Clean(target), Shared: true, DiscoverableBy: []capabilitypack.Surface{capabilitypack.SurfaceOpenCode},
			Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceCodex, Kind: capabilitypack.ActionCodexProjectSkillTree, Source: source, Target: target, Version: desired, Precondition: observed, Description: fmt.Sprintf("copy %s to the Codex project skill tree", identity), PreviewOnly: true},
		})
	}
	if pack.ID == "matty" {
		instruction, err := projectMattyInstruction(projectRoot)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		projections = append(projections, instruction)
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

func codexProjectInstructionProjection(identity capabilitypack.ResourceIdentity, target, source, document, precondition string) capabilitypack.ObservedProjection {
	start, end := projectInstructionMarkers(identity.ID)
	block := start + "\n" + strings.TrimSpace(source) + "\n" + end
	state, fragment := projectInstructionMarkerState(document, start, end)
	observed := "missing"
	exists := state != "missing"
	if fragment != "" {
		observed = localprojection.FingerprintBytes([]byte(fragment))
	} else if exists {
		observed = state + ":" + localprojection.FingerprintBytes([]byte(document))
	}
	content := ""
	if state == "missing" {
		content = mergeBlock(document, block, start, end)
	} else if state == "intact" {
		content = strings.Replace(document, fragment, block, 1)
	}
	key := "path:" + filepath.Clean(target) + "#packy-project-instruction:" + identity.ID
	return capabilitypack.ObservedProjection{
		ID: identity.String(), Goal: capabilitypack.ProjectionPresent, Exists: exists, ObservedFingerprint: observed,
		DesiredFingerprint: localprojection.FingerprintBytes([]byte(block)), AdapterProvenance: "codex-project/v1/shared-composable-instruction/" + state,
		ProjectionKey: key, Shared: true, DiscoverableBy: []capabilitypack.Surface{capabilitypack.SurfaceOpenCode},
		Action: capabilitypack.ProjectionAction{ID: identity.String(), Surface: capabilitypack.SurfaceCodex, Kind: capabilitypack.ActionInstructionFile, Target: target, Content: content, FileMode: 0o644, Precondition: precondition, ProjectionKey: key, Shared: true, DiscoverableBy: []capabilitypack.Surface{capabilitypack.SurfaceOpenCode}, Description: fmt.Sprintf("merge %s into the shared project AGENTS.md", identity), PreviewOnly: true},
	}
}

func projectInstructionMarkers(id string) (string, string) {
	return "<!-- packy:project:instruction:" + id + ":start -->", "<!-- packy:project:instruction:" + id + ":end -->"
}

// inspectLockedProject validates and translates a supported committed project
// lock without consulting Packy's catalog or original source trees.
func (a *SurfaceAdapter) inspectLockedProject(projectRoot string, pack capabilitypack.ProjectManifestPack, lock capabilitypack.ProjectLockProposal, goal capabilitypack.ProjectionGoal) (capabilitypack.SurfaceInspection, error) {
	if goal == "" {
		goal = capabilitypack.ProjectionPresent
	}
	bindings := make(map[capabilitypack.ResourceIdentity]string, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		if binding.Surface != "" && binding.Surface != capabilitypack.SurfaceCodex {
			continue
		}
		bindings[capabilitypack.ResourceIdentity{Kind: binding.Kind, ID: binding.ID}] = binding.Name
	}
	projections := make([]capabilitypack.ObservedProjection, 0, len(lock.Projections))
	var revision []string
	for _, projection := range lock.Projections {
		if projection.OwnerPack != pack.ID || projection.Surface != capabilitypack.SurfaceCodex {
			continue
		}
		expected := ""
		switch projection.Mode {
		case "copy_tree":
			name := bindings[projection.Resource]
			if projection.Resource.Kind != "skill" || name == "" {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has no Codex project binding", projection.Resource)
			}
			expected = filepath.Join(".agents", "skills", name)
		case "merge_marked_file":
			if projection.Resource.Kind == "mcp_server" {
				name := bindings[projection.Resource]
				if name == "" {
					return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has no Codex project binding", projection.Resource)
				}
				expected = filepath.Join(".codex", "config.toml")
			} else if projection.Resource.Kind == "instruction" {
				expected = "AGENTS.md"
			} else {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has no supported Codex marked-file target", projection.Resource)
			}
		default:
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has unsupported Codex mode %q", projection.Resource, projection.Mode)
		}
		if filepath.Clean(filepath.FromSlash(projection.Target)) != filepath.Clean(expected) {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s target %q does not match the re-derived Codex target %q", projection.Resource, projection.Target, filepath.ToSlash(expected))
		}
		target := filepath.Join(projectRoot, expected)
		if _, err := capabilitypack.RelativeProjectTarget(projectRoot, target); err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("project lock projection %s has unsafe Codex target: %w", projection.Resource, err)
		}
		observed, exists := "missing", false
		action := capabilitypack.ProjectionAction{ID: projection.Resource.String(), Surface: capabilitypack.SurfaceCodex, Target: target, PreviewOnly: true}
		if projection.Mode == "copy_tree" {
			fingerprint, found, err := projectTreeFingerprint(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			observed, exists = fingerprint, found
			action.Kind = capabilitypack.ActionCodexProjectSkillTree
			action.Precondition = projection.DesiredFingerprint
		} else if projection.Resource.Kind == "mcp_server" {
			current, err := readOptionalFile(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			name := bindings[projection.Resource]
			start, end := mcpMarkers(name)
			fragment, found := extractBlock(current, start, end)
			if found {
				observed, exists = localprojection.FingerprintBytes([]byte(fragment)), true
			} else if codexMCPTableExists(current, name) {
				observed, exists = localprojection.FingerprintBytes([]byte("unmanaged:"+name)), true
			}
			action.Kind = capabilitypack.ActionCodexMCPConfig
			if goal == capabilitypack.ProjectionAbsent && exists && observed == projection.DesiredFingerprint {
				remaining := removeBlock(current, start, end)
				info, statErr := os.Lstat(target)
				if statErr != nil || !info.Mode().IsRegular() {
					return capabilitypack.SurfaceInspection{}, fmt.Errorf("Codex project MCP target is not a regular file")
				}
				action.Content, action.FileMode, action.Precondition = remaining, uint32(info.Mode().Perm()), localprojection.FingerprintBytes([]byte(current))
				if strings.TrimSpace(remaining) == "" {
					action.Content, action.Mode = "", capabilitypack.ProjectionDeleteTarget
				} else {
					action.Mode = capabilitypack.ProjectionRemoveContent
				}
			}
		} else {
			start, end := projectInstructionMarkers(projection.Resource.ID)
			if pack.ID == "matty" && projection.Resource.String() == projectMattyInstructionID {
				start, end = projectMattyInstructionStart, projectMattyInstructionEnd
			}
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
					return capabilitypack.SurfaceInspection{}, fmt.Errorf("Codex project instruction target is not a regular file")
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
			AdapterProvenance: "codex-project/v1/locked-" + projection.Mode, Action: action,
		}
		if goal == capabilitypack.ProjectionPresent {
			item.DesiredFingerprint = projection.DesiredFingerprint
		} else {
			if action.Mode == "" {
				action.Mode = capabilitypack.ProjectionDeleteTarget
				item.Action = action
			}
			item.Action.Description = fmt.Sprintf("remove exact Codex project projection %s", projection.Resource)
		}
		projections = append(projections, item)
		revision = append(revision, item.ID+"="+observed+"\x00"+action.Precondition)
	}
	sort.Strings(revision)
	readiness := capabilitypack.ReadinessObservation{}
	activationActions := []capabilitypack.ProjectionAction(nil)
	if goal == capabilitypack.ProjectionPresent {
		var err error
		readiness, activationActions, err = a.inspectProjectRuntime(projectRoot, lock)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
	}
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revision, "\n"))), Projections: projections, Readiness: readiness, ProjectActivationActions: activationActions}, nil
}

func (a *SurfaceAdapter) inspectProjectRuntime(projectRoot string, lock capabilitypack.ProjectLockProposal) (capabilitypack.ReadinessObservation, []capabilitypack.ProjectionAction, error) {
	sensitive := false
	additionalTrustReady := true
	authenticationReady := true
	externalCommands := map[string]bool{}
	for _, disclosure := range lock.Sensitive {
		if disclosure.Surface == capabilitypack.SurfaceCodex {
			sensitive = true
			switch disclosure.Category {
			case capabilitypack.ProjectActivationTrust:
				if disclosure.Detail != "project-trust" {
					additionalTrustReady = false
				}
			case capabilitypack.ProjectActivationAuthentication:
				authenticationReady = false
			case capabilitypack.ProjectActivationExternalRequirements:
				parts := strings.Split(disclosure.Detail, ":")
				if len(parts) >= 2 {
					externalCommands[parts[len(parts)-1]] = true
				}
			}
		}
	}
	if !sensitive {
		for _, binding := range lock.Bindings {
			if binding.Surface == capabilitypack.SurfaceCodex && (binding.Kind == "mcp_server" || binding.Kind == "lifecycle" || binding.Projection == "plugin") {
				sensitive = true
				break
			}
		}
	}
	if !sensitive {
		return capabilitypack.ReadinessObservation{}, nil, nil
	}
	if a.configFile == "" {
		return capabilitypack.ReadinessObservation{AuthorizationObserved: true, UsabilityObserved: true, PendingHumanActions: []string{"configure a writable Codex home to activate project trust"}}, nil, nil
	}
	start, end, canonical, err := codexProjectTrustMarkers(projectRoot)
	if err != nil {
		return capabilitypack.ReadinessObservation{}, nil, err
	}
	section := "projects." + strconv.Quote(canonical)
	desiredBlock := start + "\n[" + section + "]\ntrust_level = \"trusted\"\n" + end
	current, err := readOptionalFile(a.configFile)
	if err != nil {
		return capabilitypack.ReadinessObservation{}, nil, err
	}
	fragment, owned := extractBlock(current, start, end)
	trusted := owned && fragment == desiredBlock
	if !owned {
		trusted = tomlSectionHas(current, section, map[string]string{"trust_level": "trusted"})
	}
	actions := []capabilitypack.ProjectionAction{}
	pending := []string{}
	if !trusted {
		if tomlSectionExists(current, section) {
			pending = append(pending, "the existing Codex project trust entry is not trusted; review it in Codex before retrying activation")
		} else {
			mode := uint32(0o600)
			precondition := "missing"
			if info, statErr := os.Stat(a.configFile); statErr == nil {
				mode = uint32(info.Mode().Perm())
				precondition = localprojection.FingerprintBytes([]byte(current))
			} else if !os.IsNotExist(statErr) {
				return capabilitypack.ReadinessObservation{}, nil, statErr
			}
			actions = append(actions, capabilitypack.ProjectionAction{ID: "project_trust:codex", Surface: capabilitypack.SurfaceCodex, Kind: capabilitypack.ActionCodexProjectTrust, Target: a.configFile, Content: mergeBlock(current, desiredBlock, start, end), FileMode: mode, Precondition: precondition, Version: localprojection.FingerprintBytes([]byte(desiredBlock)), AdapterProvenance: codexProjectTrustProvenance, Description: "trust the exact Codex project so its installed runtime definitions can load", ContributionStartMarker: start, ContributionEndMarker: end, PreviewOnly: true})
		}
	}
	if !additionalTrustReady {
		pending = append(pending, "approve the remaining host-owned Codex authority in a fresh runtime session")
	}
	if !authenticationReady {
		pending = append(pending, "complete the declared host-owned authentication requirement in Codex")
	}
	for _, projection := range lock.Projections {
		if projection.Resource.Kind != "mcp_server" || projection.Surface != capabilitypack.SurfaceCodex || projection.OwnerPack == "" || projection.Command == "" {
			continue
		}
		delete(externalCommands, filepath.Base(projection.Command))
		externalCommands[projection.Command] = true
	}
	externalReady := true
	commands := make([]string, 0, len(externalCommands))
	for command := range externalCommands {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	for _, command := range commands {
		if _, lookErr := exec.LookPath(command); lookErr != nil {
			externalReady = false
			pending = append(pending, "install external requirement "+command)
		}
	}
	authorized := trusted && additionalTrustReady
	return capabilitypack.ReadinessObservation{
		AuthorizationObserved: true, Authorized: authorized,
		UsabilityObserved: true, Usable: authorized && externalReady && authenticationReady,
		PendingHumanActions: pending,
		Evidence:            []string{"Codex project trust, locked runtime definitions, and external commands inspected"},
	}, actions, nil
}

func codexProjectTrustMarkers(projectRoot string) (string, string, string, error) {
	canonical, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", "", "", err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", "", "", err
	}
	canonical = filepath.Clean(canonical)
	identity := localprojection.FingerprintBytes([]byte(canonical))[:16]
	return "# packy:project-trust:" + identity + ":start", "# packy:project-trust:" + identity + ":end", canonical, nil
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

const (
	projectMattyInstructionID    = "instruction:matty-codex-project"
	projectMattyInstructionStart = "<!-- packy:project:matty:codex:start -->"
	projectMattyInstructionEnd   = "<!-- packy:project:matty:codex:end -->"
)

func projectMattyInstruction(projectRoot string) (capabilitypack.ObservedProjection, error) {
	target := filepath.Join(projectRoot, "AGENTS.md")
	current, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return capabilitypack.ObservedProjection{}, fmt.Errorf("read Codex project instructions: %w", err)
	}
	desiredBlock := projectMattyInstructionStart + "\n" +
		"Packy manages the Matty Codex skill trees in .agents/skills.\n" +
		projectMattyInstructionEnd
	desired := localprojection.FingerprintBytes([]byte(desiredBlock))
	content := string(current)
	state, fragment := projectInstructionMarkerState(content, projectMattyInstructionStart, projectMattyInstructionEnd)
	projection := capabilitypack.ObservedProjection{
		ID: projectMattyInstructionID, Goal: capabilitypack.ProjectionPresent,
		DesiredFingerprint: desired, AdapterProvenance: "codex-project/v1/composable-instruction/" + state,
		Action: capabilitypack.ProjectionAction{ID: projectMattyInstructionID, Surface: capabilitypack.SurfaceCodex, Kind: capabilitypack.ActionInstructionFile, Target: target, FileMode: 0o644, Description: "merge Packy Matty Codex instructions into the project AGENTS.md", PreviewOnly: true},
	}
	projection.Action.Precondition = "missing"
	if err == nil {
		projection.Action.Precondition = localprojection.FingerprintBytes(current)
	}
	if info, statErr := os.Lstat(target); statErr == nil && info.Mode().IsRegular() {
		projection.Action.FileMode = uint32(info.Mode().Perm())
	}
	switch state {
	case "missing":
		projection.ObservedFingerprint = "missing"
		projection.Action.Content = mergeBlock(content, desiredBlock, projectMattyInstructionStart, projectMattyInstructionEnd)
	case "intact", "changed":
		projection.Exists = true
		projection.ObservedFingerprint = localprojection.FingerprintBytes([]byte(fragment))
		if state == "intact" {
			projection.Action.Content = mergeBlock(content, desiredBlock, projectMattyInstructionStart, projectMattyInstructionEnd)
		}
	case "malformed", "ambiguous":
		projection.Exists = true
		projection.ObservedFingerprint = state + ":" + localprojection.FingerprintBytes([]byte(content))
	default:
		return capabilitypack.ObservedProjection{}, fmt.Errorf("unknown Codex project instruction marker state %q", state)
	}
	if state == "intact" && projection.ObservedFingerprint != desired {
		projection.AdapterProvenance = "codex-project/v1/composable-instruction/changed"
		projection.Action.Content = ""
	}
	return projection, nil
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

func codexProjectBinding(resource capabilitypack.Resource) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceCodex && binding.Name != "" {
			return binding.Name, true
		}
	}
	return "", false
}
