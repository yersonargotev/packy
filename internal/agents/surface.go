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

type SurfaceAdapter struct {
	catalogRoot string
	skillsDir   string
	rolesDir    string
}

func NewSurfaceAdapter(catalogRoot, skillsDir, rolesDir string) *SurfaceAdapter {
	return &SurfaceAdapter{catalogRoot: catalogRoot, skillsDir: skillsDir, rolesDir: rolesDir}
}

func (a *SurfaceAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	if transition.ProjectInstallation != nil {
		if transition.ProjectRoot == "" || len(transition.ProjectInstallation.Manifest.Packs) != 1 {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("locked Agents project inspection requires one manifest pack and the project root")
		}
		return a.inspectLockedProject(transition.ProjectRoot, transition.ProjectInstallation.Manifest.Packs[0], transition.ProjectInstallation.Lock, transition.ProjectGoal, transition.ProjectInspectionScope)
	}
	if transition.ProjectRoot != "" {
		return a.inspectProject(ctx, transition.Desired, transition.ProjectRoot)
	}
	originalDesired := transition.Desired
	var unavailable []capabilitypack.UnrepresentableResource
	if transition.ObservationOnly {
		transition.Desired.Resources = nil
	}
	for _, resource := range originalDesired.Resources {
		if resource.Kind == "agent" {
			if transition.Client != "codex" || a.rolesDir == "" {
				reason := fmt.Sprintf("agent %s requires explicit client codex at global scope", resource.ID)
				if !transition.ObservationOnly {
					return capabilitypack.SurfaceInspection{}, fmt.Errorf("%s", reason)
				}
				unavailable = append(unavailable, capabilitypack.UnrepresentableResource{Resource: capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, Reason: reason})
				continue
			}
			if err := validateCodexRole(resource); err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
		}
		if transition.ObservationOnly {
			transition.Desired.Resources = append(transition.Desired.Resources, resource)
		}
	}
	var (
		observation capabilitypack.SurfaceInspection
		err         error
	)
	if transition.ReceiptOwnership != nil {
		return a.inspectReceipt(transition.ReceiptOwnership)
	}
	if len(transition.ResidualOwnership) > 0 {
		observation, err = a.inspectOwnershipResidual(ctx, transition.Desired, transition.ResidualOwnership, transition.ResolvedExecutables)
	} else if transition.Prior.ID != "" {
		observation, err = a.inspectPriorTransition(ctx, transition.Prior, transition.Desired, transition.ResolvedExecutables)
	} else {
		observation, err = a.inspectDesired(ctx, transition.Desired, transition.ResolvedExecutables)
	}
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	observation.Unrepresentable = append(observation.Unrepresentable, unavailable...)
	bindAdapterProvenance(&observation)
	applyRecordedOccupancyOwnership(&observation, transition.CurrentOwnership)
	readinessPack := originalDesired
	if readinessPack.ID == "" {
		readinessPack = transition.Prior
	}
	observation.Readiness, err = a.inspectReadiness(ctx, readinessPack, observation, transition.ResolvedExecutables)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	observation.RuntimeModeEvidence, err = capabilitypack.UnverifiedRuntimeModeEvidence(
		readinessPack, time.Now().UTC(), observation.Revision,
	)
	return observation, err
}

func bindAdapterProvenance(observation *capabilitypack.SurfaceInspection) {
	for i := range observation.Projections {
		projection := &observation.Projections[i]
		provenance := projection.AdapterProvenance
		if provenance == "" {
			provenance = projection.Action.AdapterProvenance
		}
		if provenance == "" {
			provenance = "agents-projection/v1/" + string(projection.Action.Kind)
		}
		projection.AdapterProvenance = provenance
		if projection.Action.Kind == capabilitypack.ActionSkillTree && projection.Action.Target != "" {
			projection.ProjectionKey = "path:" + filepath.Clean(projection.Action.Target)
			projection.Shared = true
			projection.Action.ProjectionKey = projection.ProjectionKey
			projection.Action.Shared = true
		}
	}
}

// inspectReadiness is filesystem-only. Shared discovery paths do not identify
// a runtime client or establish its authorization and execution capabilities.
func (a *SurfaceAdapter) inspectReadiness(_ context.Context, pack capabilitypack.Pack, observation capabilitypack.SurfaceInspection, _ []capabilitypack.ExecutableResolution) (capabilitypack.ReadinessObservation, error) {
	return capabilitypack.ReadinessObservation{OptionalAuthorities: capabilitypack.UnknownOptionalAuthorities(pack), PendingHumanActions: observation.PendingHumanActions, Evidence: []string{"Agents filesystem projections inspected; client authorization and runtime loading are not observed"}}, nil
}

func (a *SurfaceAdapter) inspectDesired(_ context.Context, pack capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	var projections []capabilitypack.ObservedProjection
	var unrepresentable []capabilitypack.UnrepresentableResource
	var revisionParts []string
	templateNames := capabilitypack.SkillTemplateNames(pack, capabilitypack.SurfaceAgents)
	for _, resource := range pack.Resources {
		switch resource.Kind {
		case "skill":
			source := filepath.Join(resource.CatalogRootOr(a.catalogRoot), filepath.Clean(resource.Source))
			name, ok := agentsBindingName(resource, "skill")
			if !ok {
				continue
			}
			target := filepath.Join(a.skillsDir, name)
			id := "skill:" + name
			if resource.RequestsSurfaceCapability(capabilitypack.SurfaceAgents, capabilitypack.SurfaceCapabilitySkillTemplate) {
				files, desired, err := localprojection.RenderSkillTemplate(source, resource.ID, templateNames, 0o700)
				if err != nil {
					return capabilitypack.SurfaceInspection{}, fmt.Errorf("render skill template %q: %w", resource.ID, err)
				}
				observed, exists, err := localprojection.ObserveSkillTarget(target)
				if err != nil {
					return capabilitypack.SurfaceInspection{}, err
				}
				provenance := skilltemplate.RendererVersion + "/agents/global"
				action := capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillTree, Target: target, TreeFiles: files, FileMode: 0o700, Version: desired, Precondition: observed, AdapterProvenance: provenance, Description: fmt.Sprintf("render skill template %s at %s", resource.ID, target)}
				projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: provenance, Action: action})
				revisionParts = append(revisionParts, id+"="+observed)
				continue
			}
			desired, err := localprojection.FingerprintCopiedTree(source, 0o700)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("fingerprint skill %q: %w", resource.ID, err)
			}
			observed, exists, err := localprojection.ObserveSkillTarget(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillTree, Source: source, Target: target, FileMode: 0o700, Version: desired, Precondition: observed, Description: fmt.Sprintf("copy complete skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
		case "instruction":
			unrepresentable = append(unrepresentable, capabilitypack.UnrepresentableResource{Resource: capabilitypack.ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, Reason: "Agents instructions are supported only at project scope"})
		case "mcp_server", "command", "hook":
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("Agents does not support %s projections", resource.Kind)
		case "agent":
			name, ok := agentsBindingName(resource, "agent")
			if !ok {
				continue
			}
			source := filepath.Join(resource.CatalogRootOr(a.catalogRoot), filepath.Clean(resource.Source))
			prompt, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read agent %q: %w", resource.ID, err)
			}
			content := renderCodexRole(resource, name, prompt)
			target := filepath.Join(a.rolesDir, name+".toml")
			projection, revision, err := fileProjection("agent:"+name, capabilitypack.ActionCodexAgentFile, target, content, fmt.Sprintf("write Codex agent %s at global scope in %s", name, target))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projections = append(projections, projection)
			revisionParts = append(revisionParts, revision)

		}
	}
	for i := range projections {
		projections[i].Goal = capabilitypack.ProjectionPresent
	}
	for i, projection := range projections {
		for _, prior := range projections[:i] {
			if !exclusivePathKind(projection.Action.Kind) && !exclusivePathKind(prior.Action.Kind) {
				continue
			}
			if projection.Action.Target != "" && prior.Action.Target != "" && pathsOverlap(prior.Action.Target, projection.Action.Target) && prior.ID != projection.ID {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("Agents projections %s and %s have overlapping targets %s and %s", prior.ID, projection.ID, prior.Action.Target, projection.Action.Target)
			}
		}
	}
	sort.Strings(revisionParts)
	sort.Slice(projections, func(i, j int) bool { return projections[i].ID < projections[j].ID })
	occupied, err := a.inspectOccupiedNames()
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	for _, name := range occupied {
		revisionParts = append(revisionParts, "occupied:"+name.Namespace+":"+name.Name+"="+name.OwnerType+":"+name.OwnerID+":"+name.Fingerprint)
	}
	sort.Strings(revisionParts)
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections, OccupiedNames: occupied, Unrepresentable: unrepresentable}, nil
}

func (a *SurfaceAdapter) inspectOccupiedNames() ([]capabilitypack.OccupiedName, error) {
	var result []capabilitypack.OccupiedName
	for _, namespace := range []struct{ name, dir, suffix string }{{"skill", a.skillsDir, ""}, {"agent", a.rolesDir, ".toml"}} {
		if namespace.dir == "" {
			continue
		}
		entries, err := os.ReadDir(namespace.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect Agents %s namespace: %w", namespace.name, err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || (namespace.suffix != "" && filepath.Ext(entry.Name()) != namespace.suffix) {
				continue
			}
			path := filepath.Join(namespace.dir, entry.Name())
			fingerprint, exists, err := localprojection.FingerprintPath(path)
			if namespace.name == "skill" {
				fingerprint, exists, err = localprojection.ObserveSkillTarget(path)
			}
			if err != nil {
				return nil, err
			}
			if !exists {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), namespace.suffix)
			occupied := capabilitypack.OccupiedName{Namespace: namespace.name, Name: name, OwnerType: "unmanaged", Fingerprint: fingerprint}
			result = append(result, occupied)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func applyRecordedOccupancyOwnership(observation *capabilitypack.SurfaceInspection, ownership []capabilitypack.ProjectionOwnership) {
	byID := make(map[string]capabilitypack.ProjectionOwnership, len(ownership))
	for _, owner := range ownership {
		byID[owner.ID] = owner
	}
	for i := range observation.OccupiedNames {
		occupied := &observation.OccupiedNames[i]
		for _, projection := range observation.Projections {
			if !projection.Exists || projection.ObservedFingerprint != occupied.Fingerprint {
				continue
			}
			namespace, name, ok := strings.Cut(projection.ID, ":")
			if !ok || namespace != occupied.Namespace || name != occupied.Name {
				continue
			}
			owner, recorded := byID[projection.ID]
			if recorded && owner.Fingerprint == occupied.Fingerprint {
				occupied.OwnerType, occupied.OwnerID = "packy", owner.PackID
			}
		}
	}
}

func (a *SurfaceAdapter) inspectPriorTransition(ctx context.Context, active, desired capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	current, err := a.inspectDesired(ctx, active, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	result, err := a.inspectDesired(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	retained := map[string]bool{}
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	for _, projection := range current.Projections {
		if !retained[projection.ID] {
			result.Projections = append(result.Projections, capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", "remove Agents projection "+projection.ID))
		}
	}
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].ID < result.Projections[j].ID })
	result.Revision = localprojection.FingerprintBytes([]byte(current.Revision + "\n" + result.Revision))
	return result, nil
}

func (a *SurfaceAdapter) inspectOwnershipResidual(ctx context.Context, desired capabilitypack.Pack, ownership []capabilitypack.ProjectionOwnership, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	result, err := a.inspectDesired(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	retained := map[string]bool{}
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	for _, owner := range ownership {
		if retained[owner.ID] {
			continue
		}
		projection, err := a.inspectOwnedProjection(owner.ID)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		result.Projections = append(result.Projections, projection)
	}
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].ID < result.Projections[j].ID })
	return result, nil
}

func (a *SurfaceAdapter) inspectOwnedProjection(id string) (capabilitypack.ObservedProjection, error) {
	root, target, skill, err := a.receiptTarget(id)
	if err != nil {
		return capabilitypack.ObservedProjection{}, err
	}
	if err := localprojection.ValidateReceiptTarget(root, target, skill); err != nil {
		return capabilitypack.ObservedProjection{}, err
	}
	kind := capabilitypack.ActionCodexAgentFile
	observed, exists, err := localprojection.FingerprintPath(target)
	if skill {
		kind = capabilitypack.ActionSkillTree
		observed, exists, err = localprojection.ObserveSkillTarget(target)
	}
	projection := capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, Action: capabilitypack.ProjectionAction{ID: id, Kind: kind, Target: target}}
	return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", "remove Agents projection "+id), err
}

func (a *SurfaceAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	executor := localprojection.Executor{
		Host:      "Agents",
		TreeKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionSkillTree: true, capabilitypack.ActionAgentsProjectSkillTree: true},
		FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionInstructionFile:     true,
			capabilitypack.ActionProjectManifestFile: true,
			capabilitypack.ActionProjectLockFile:     true,
			capabilitypack.ActionProjectNoticesFile:  true,
			capabilitypack.ActionCodexAgentFile:      true,
		},
	}
	err := executor.Apply(actions)
	if err == nil {
		return nil
	}
	if actionErr, ok := err.(capabilitypack.ProjectionActionError); ok {
		return &actionErr
	}
	return &capabilitypack.ProjectionActionError{ID: actions[0].ID, Err: err}
}

func agentsBindingName(resource capabilitypack.Resource, projection string) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceAgents && binding.Projection == projection {
			return binding.Name, true
		}
	}
	return "", false
}

func fileProjection(id string, kind capabilitypack.ProjectionActionKind, target, content, description string) (capabilitypack.ObservedProjection, string, error) {
	observed, exists, err := localprojection.FingerprintPath(target)
	if err != nil {
		return capabilitypack.ObservedProjection{}, "", err
	}
	desired := localprojection.FingerprintBytes([]byte(content))
	return capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: kind, Target: target, Content: content, Description: description}}, id + "=" + observed, nil
}

func pathsOverlap(left, right string) bool {
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func exclusivePathKind(kind capabilitypack.ProjectionActionKind) bool {
	return kind == capabilitypack.ActionSkillTree || kind == capabilitypack.ActionCodexAgentFile
}

func extractBlock(content, startMarker, endMarker string) (string, bool) {
	start := strings.Index(content, startMarker)
	if start < 0 {
		return "", false
	}
	relEnd := strings.Index(content[start:], endMarker)
	if relEnd < 0 {
		return "", false
	}
	end := start + relEnd + len(endMarker)
	return content[start:end], true
}
func mergeBlock(content, block, startMarker, endMarker string) string {
	if existing, ok := extractBlock(content, startMarker, endMarker); ok {
		return strings.Replace(content, existing, block, 1)
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}
