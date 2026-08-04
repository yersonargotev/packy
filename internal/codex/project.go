package codex

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
)

func (a *SurfaceAdapter) inspectProject(_ context.Context, pack capabilitypack.Pack, projectRoot string) (capabilitypack.SurfaceInspection, error) {
	projections := make([]capabilitypack.ObservedProjection, 0, len(pack.Resources))
	unrepresentable := make([]capabilitypack.UnrepresentableResource, 0)
	for _, resource := range pack.Resources {
		identity := capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		if resource.Kind == "notice" {
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
			Action: capabilitypack.ProjectionAction{ID: identity.String(), Kind: capabilitypack.ActionCodexProjectSkillTree, Source: source, Target: target, Version: desired, Precondition: observed, Description: fmt.Sprintf("copy %s to the Codex project skill tree", identity), PreviewOnly: true},
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

// InspectProjectInstallation validates a supported committed project lock
// without consulting Packy's catalog or the source trees that produced it.
func (a *SurfaceAdapter) InspectProjectInstallation(_ context.Context, projectRoot string, pack capabilitypack.ProjectManifestPack, lock capabilitypack.ProjectLockProposal) ([]capabilitypack.ProjectProjectionStatus, error) {
	bindings := make(map[capabilitypack.ResourceIdentity]string, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		bindings[capabilitypack.ResourceIdentity{Kind: binding.Kind, ID: binding.ID}] = binding.Name
	}
	statuses := make([]capabilitypack.ProjectProjectionStatus, 0, len(lock.Projections))
	for _, projection := range lock.Projections {
		expected := ""
		switch projection.Mode {
		case "copy_tree":
			name := bindings[projection.Resource]
			if projection.Resource.Kind != "skill" || name == "" {
				return nil, fmt.Errorf("project lock projection %s has no Codex project binding", projection.Resource)
			}
			expected = filepath.Join(".agents", "skills", name)
		case "merge_marked_file":
			if pack.ID != "matty" || projection.Resource.String() != projectMattyInstructionID {
				return nil, fmt.Errorf("project lock projection %s has no supported Codex marked-file target", projection.Resource)
			}
			expected = "AGENTS.md"
		default:
			return nil, fmt.Errorf("project lock projection %s has unsupported Codex mode %q", projection.Resource, projection.Mode)
		}
		if filepath.Clean(filepath.FromSlash(projection.Target)) != filepath.Clean(expected) {
			return nil, fmt.Errorf("project lock projection %s target %q does not match the re-derived Codex target %q", projection.Resource, projection.Target, filepath.ToSlash(expected))
		}
		target := filepath.Join(projectRoot, expected)
		if _, err := capabilitypack.RelativeProjectTarget(projectRoot, target); err != nil {
			return nil, fmt.Errorf("project lock projection %s has unsafe Codex target: %w", projection.Resource, err)
		}
		observed, health := "missing", "missing"
		if projection.Mode == "copy_tree" {
			fingerprint, exists, err := projectTreeFingerprint(target)
			if err != nil {
				return nil, err
			}
			observed = fingerprint
			if exists {
				health = "drifted"
				if fingerprint == projection.DesiredFingerprint {
					health = "verified"
				}
			}
		} else {
			inspection, err := projectMattyInstruction(projectRoot)
			if err != nil {
				return nil, err
			}
			observed = inspection.ObservedFingerprint
			if inspection.Exists {
				health = "drifted"
				if observed == projection.DesiredFingerprint {
					health = "verified"
				}
			}
		}
		statuses = append(statuses, capabilitypack.ProjectProjectionStatus{
			Resource: projection.Resource, Target: projection.Target, Mode: projection.Mode, Health: health,
			ObservedFingerprint: observed, DesiredFingerprint: projection.DesiredFingerprint, Contributor: projection.Contributor,
		})
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Target != statuses[j].Target {
			return statuses[i].Target < statuses[j].Target
		}
		return statuses[i].Resource.String() < statuses[j].Resource.String()
	})
	return statuses, nil
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
		Action: capabilitypack.ProjectionAction{ID: projectMattyInstructionID, Kind: capabilitypack.ActionInstructionFile, Target: target, FileMode: 0o644, Description: "merge Packy Matty Codex instructions into the project AGENTS.md", PreviewOnly: true},
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
