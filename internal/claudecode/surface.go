package claudecode

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
	"github.com/yersonargotev/packy/internal/skilltemplate"
)

const (
	ActionSkillLink               capabilitypack.ProjectionActionKind = "claude-skill-link"
	ActionSkillTree               capabilitypack.ProjectionActionKind = "claude-skill-tree"
	ActionInstructionContribution capabilitypack.ProjectionActionKind = "claude-instruction-contribution"
	ActionAgentFile               capabilitypack.ProjectionActionKind = "claude-agent-file"
)

type RuntimeEvidenceObserver interface {
	ObserveRuntimeEvidence(context.Context) []RuntimeEvidence
}

type SurfaceAdapter struct {
	layout                             CanonicalLayout
	catalogRoot, stateRoot, executable string
	runner                             Runner
	ownership                          OwnershipSnapshotProvider
	authorization                      AuthorizationObserver
	runtimeEvidence                    RuntimeEvidenceObserver
}

func (a *SurfaceAdapter) WithRuntimeEvidence(observer RuntimeEvidenceObserver) *SurfaceAdapter {
	a.runtimeEvidence = observer
	return a
}

func NewSurfaceAdapterWithAuthorization(catalogRoot string, layout CanonicalLayout, stateRoot, executable string, runner Runner, ownership OwnershipSnapshotProvider, authorization AuthorizationObserver) *SurfaceAdapter {
	a := NewSurfaceAdapter(catalogRoot, layout, stateRoot, executable, runner, ownership)
	a.authorization = authorization
	return a
}

func NewSurfaceAdapter(catalogRoot string, layout CanonicalLayout, stateRoot, executable string, runner Runner, ownership OwnershipSnapshotProvider) *SurfaceAdapter {
	return &SurfaceAdapter{catalogRoot: catalogRoot, layout: layout, stateRoot: stateRoot, executable: executable, runner: runner, ownership: ownership}
}

func (a *SurfaceAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	if transition.ReceiptOwnership != nil {
		return a.inspectReceipt(transition.ReceiptOwnership)
	}
	if transition.ProjectInstallation != nil {
		if transition.ProjectRoot == "" || len(transition.ProjectInstallation.Manifest.Packs) != 1 {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("locked Claude project inspection requires one manifest pack and the project root")
		}
		return a.inspectLockedProject(ctx, transition.ProjectRoot, transition.ProjectInstallation.Manifest.Packs[0], transition.ProjectInstallation.Lock, transition.ProjectGoal, transition.ProjectInspectionScope)
	}
	if transition.ProjectRoot != "" {
		return a.inspectProject(ctx, transition.Desired, transition.ProjectRoot)
	}
	ownership := OwnershipSnapshot{}
	if a.ownership != nil && !transition.ObservationOnly {
		var err error
		ownership, err = a.ownership.ObserveOwnership(ctx)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("observe Claude ownership: %w", err)
		}
	}
	pack := transition.Desired
	templateNames := capabilitypack.SkillTemplateNames(pack, capabilitypack.SurfaceClaude)
	var result capabilitypack.SurfaceInspection
	revision := []string{}
	var instructionDocument, instructionOriginal []byte
	var instructionLoaded bool
	for _, r := range pack.Resources {
		b, ok := claudeBinding(r)
		if !ok {
			continue
		}
		if r.Kind != b.Projection || (r.Kind != "skill" && r.Kind != "instruction" && r.Kind != "agent") {
			return result, fmt.Errorf("unsupported Claude %s projection for %s:%s", b.Projection, r.Kind, r.ID)
		}
		id := r.Kind + ":" + b.Name
		switch b.Projection {
		case "skill":
			if r.RequestsSurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilitySkillTemplate) {
				source := filepath.Join(r.CatalogRootOr(a.catalogRoot), filepath.Clean(r.Source))
				files, desired, err := localprojection.RenderSkillTemplate(source, r.ID, templateNames, 0o700)
				if err != nil {
					return result, fmt.Errorf("render Claude skill template %q: %w", r.ID, err)
				}
				target := filepath.Join(a.layout.SkillsDir, b.Name)
				observed, exists, err := localprojection.ObserveSkillTarget(target)
				if err != nil {
					return result, err
				}
				provenance := skilltemplate.RendererVersion + "/claude/global"
				action := capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillTree, Target: target, TreeFiles: files, FileMode: 0o700, Version: desired, Precondition: observed, AdapterProvenance: provenance, Description: "render Claude Code skill template " + b.Name}
				result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, AdapterProvenance: provenance, Action: action})
				revision = append(revision, id+observed)
				continue
			}
			if isClaudeCompositeProjection(pack, r, b) {
				composite, err := claudeCompositeSkill(pack, r, b, a.catalogRoot)
				if err != nil {
					return result, err
				}
				payload, err := canonicalCompositeSkillPayload(composite)
				if err != nil {
					return result, err
				}
				provenance, err := canonicalCompositeOwnership(composite.Ownership)
				if err != nil {
					return result, err
				}
				target := filepath.Join(a.layout.SkillsDir, b.Name)
				observed, exists, err := observeCompositeTree(target)
				if err != nil {
					return result, err
				}
				action := capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillTree, Target: target, Content: string(payload), AdapterProvenance: provenance, Description: "write exact Claude composite skill " + b.Name}
				result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: composite.TreeFingerprint, Action: action})
				revision = append(revision, id+observed)
				continue
			}
			source := filepath.Join(r.CatalogRootOr(a.catalogRoot), filepath.Clean(r.Source))
			if err := a.validateSkillAssetClosure(pack, r, source); err != nil {
				return result, err
			}
			expectedSource, err := canonicalPath(source)
			if err != nil {
				return result, err
			}
			desired, err := localprojection.FingerprintTree(source)
			if err != nil {
				return result, err
			}
			target := filepath.Join(a.layout.SkillsDir, b.Name)
			skill := ObserveSkill(target, expectedSource)
			if skill.Err != nil {
				return result, skill.Err
			}
			exists := skill.Kind != PathMissing
			observed := "missing"
			if exists {
				observed = skill.TreeFingerprint
				if observed == "" {
					observed = string(skill.Kind)
				}
			}
			needsReconciliation := skill.Kind == PathSymlink && skill.ResolvedTarget != expectedSource
			action := capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillLink, Source: source, Target: target, Description: "link Claude Code skill " + b.Name}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, NeedsReconciliation: needsReconciliation, Action: action})
			linkRevision := canonicalFingerprint([]string{string(skill.Kind), skill.ResolvedTarget, expectedSource, observed})
			revision = append(revision, id+linkRevision)
		case "instruction":
			content, err := os.ReadFile(filepath.Join(r.CatalogRootOr(a.catalogRoot), filepath.Clean(r.Source)))
			if err != nil {
				return result, err
			}
			if !instructionLoaded {
				instructionOriginal, err = readOptional(a.layout.InstructionsFile)
				if err != nil {
					return result, fmt.Errorf("read Claude instructions: %w", err)
				}
				instructionDocument = append([]byte(nil), instructionOriginal...)
				instructionLoaded = true
			}
			merged, err := UpsertInstructionContribution(string(instructionDocument), InstructionContribution{ContributorID: "pack:" + pack.ID + ":" + r.ID, Content: string(content)})
			if err != nil {
				return result, err
			}
			instructionDocument = []byte(merged)
			desired := Fingerprint([]byte(strings.TrimSpace(string(content))))
			action := capabilitypack.ProjectionAction{ID: id, Kind: ActionInstructionContribution, Target: a.layout.InstructionsFile, Command: Fingerprint(instructionOriginal), Description: "merge Claude Code instruction " + r.ID}
			io := ObserveInstructions(a.layout.InstructionsFile)
			if io.Err != nil {
				return result, io.Err
			}
			contributor := "pack:" + pack.ID + ":" + r.ID
			observed, exists := io.Contributions[contributor]
			if !exists {
				observed = "missing"
			}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: action})
			revision = append(revision, "instructions="+Fingerprint(instructionOriginal))
		case "agent":
			content, err := os.ReadFile(filepath.Join(r.CatalogRootOr(a.catalogRoot), filepath.Clean(r.Source)))
			if err != nil {
				return result, err
			}
			_, hasAgentDocument := r.SurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilityClaudeAgentDocument)
			if hasAgentDocument {
				content, err = renderClaudeAgentDocument(pack, r, b, content)
				if err != nil {
					return result, err
				}
			}
			content, err = a.embedConsumerAssets(pack, r, content)
			if err != nil {
				return result, err
			}
			target := filepath.Join(a.layout.AgentsDir, b.Name+".md")
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return result, err
			}
			if !hasAgentDocument {
				if len(r.Tools) > 0 || len(r.Permissions) > 0 {
					return result, fmt.Errorf("Claude agent %s is missing explicit authority translations", r.ID)
				}
			}
			desired := localprojection.FingerprintBytes(content)
			current, err := readOptional(target)
			if err != nil {
				return result, err
			}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionAgentFile, Target: target, Content: string(content), Command: Fingerprint(current), Description: "write Claude Code agent " + b.Name}})
			revision = append(revision, id+observed)
		}
	}
	if transition.Prior.ID != "" {
		for _, r := range transition.Prior.Resources {
			b, ok := claudeBinding(r)
			if !ok || resourceProjectionRetained(transition.Desired, r, b) {
				continue
			}
			projection, part, err := a.inspectRemoval(transition.Prior, r, b, ownership)
			if err != nil {
				return result, err
			}
			if b.Projection == "instruction" {
				if !instructionLoaded {
					instructionOriginal, err = readOptional(a.layout.InstructionsFile)
					if err != nil {
						return result, fmt.Errorf("read Claude instructions: %w", err)
					}
					instructionDocument = append([]byte(nil), instructionOriginal...)
					instructionLoaded = true
				}
				merged, err := RemoveInstructionContribution(string(instructionDocument), "pack:"+transition.Prior.ID+":"+r.ID)
				if err != nil {
					return result, err
				}
				instructionDocument = []byte(merged)
			}
			result.Projections = append(result.Projections, projection)
			revision = append(revision, part)
		}
	}
	if len(transition.ResidualOwnership) > 0 {
		retained := make(map[string]bool, len(result.Projections))
		for _, projection := range result.Projections {
			if projection.Goal != capabilitypack.ProjectionAbsent {
				retained[projection.ID] = true
			}
		}
		records := make(map[string]OwnershipRecord, len(ownership.Records))
		for _, record := range ownership.Records {
			records[record.ID] = record
		}
		for _, owner := range transition.ResidualOwnership {
			id := owner.ProjectionID
			if id == "" {
				id = owner.ID
			}
			if owner.Surface != capabilitypack.SurfaceClaude || retained[id] {
				continue
			}
			record, ok := records[id]
			if !ok {
				return result, fmt.Errorf("Claude residual ownership %s has no fresh ownership record", id)
			}
			projection, err := inspectClaudeResidualProjection(record)
			if err != nil {
				return result, err
			}
			result.Projections = append(result.Projections, projection)
			revision = append(revision, projection.ID+projection.ObservedFingerprint)
			retained[id] = true
		}
	}
	if instructionLoaded {
		for i := range result.Projections {
			if result.Projections[i].Action.Kind == ActionInstructionContribution {
				result.Projections[i].Action.Content = string(instructionDocument)
				result.Projections[i].Action.Command = Fingerprint(instructionOriginal)
			}
		}
	}
	for i := range result.Projections {
		if result.Projections[i].Goal == "" {
			result.Projections[i].Goal = capabilitypack.ProjectionPresent
		}
	}
	bindAdapterProvenance(&result)
	sort.Strings(revision)
	result.Revision = Fingerprint([]byte(strings.Join(revision, "\n")))
	version := ObserveVersion(ctx, a.executable, a.runner)
	versionClass := ClassifyVersion(version)
	supported := versionClass == CompatibilitySupported
	versionObserved := versionClass != CompatibilityUnreadable && versionClass != CompatibilityFailed && versionClass != CompatibilityTimedOut
	configured := true
	for _, p := range result.Projections {
		if p.Goal == capabilitypack.ProjectionPresent && (!p.Exists || p.NeedsReconciliation || p.ObservedFingerprint != p.DesiredFingerprint || p.ExternallyManaged) {
			configured = false
		}
	}
	auth := AuthorizationObservation{}
	if a.authorization != nil {
		auth = a.authorization.ObserveAuthorization(ctx)
	}
	readinessPack := pack
	if readinessPack.ID == "" {
		readinessPack = transition.Prior
	}
	optionalAuthorities, err := optionalAuthorityReadiness(readinessPack, auth)
	if err != nil {
		return result, err
	}
	authorizationObserved := versionObserved && auth.Err == nil && auth.PolicyObserved && auth.ToolPermissionObserved
	authorized := authorizationObserved && configured && supported && !auth.Disabled && !auth.Shadowed
	pending := []string{}
	if !supported {
		pending = append(pending, ClassifyVersion(version).Remediation())
	}
	if auth.Err != nil || !auth.PolicyObserved || !auth.ToolPermissionObserved || auth.Disabled || auth.Shadowed {
		pending = append(pending, "provide explicit observable Claude Code policy and tool-permission evidence")
	}
	if !configured {
		pending = append(pending, "converge every exact desired Claude Code projection")
	}
	usable, usabilityObserved, runtimeFacts := a.runtimeReadiness(ctx, pack, result.Projections, version, auth)
	if !usabilityObserved {
		pending = append(pending, "supply explicit current Claude Code runtime evidence for every included resource")
	}
	result.Readiness = capabilitypack.ReadinessObservation{AuthorizationObserved: authorizationObserved, Authorized: authorized, UsabilityObserved: usabilityObserved, Usable: authorized && usable, OptionalAuthorities: optionalAuthorities, PendingHumanActions: pending, Evidence: append([]string{"filesystem projections inspected; runtime use was not invoked"}, runtimeFacts...)}
	result.RuntimeModeEvidence, err = capabilitypack.UnverifiedRuntimeModeEvidence(
		readinessPack, time.Now().UTC(), result.Revision,
	)
	return result, err
}

func inspectClaudeResidualProjection(record OwnershipRecord) (capabilitypack.ObservedProjection, error) {
	action := capabilitypack.ProjectionAction{ID: record.ID, Kind: capabilitypack.ProjectionActionKind(record.Kind), Target: record.Target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove residual Claude projection " + record.ID}
	var (
		observed string
		exists   bool
		err      error
	)
	switch action.Kind {
	case ActionSkillLink:
		observed, exists, err = localprojection.FingerprintPath(record.Target)
	case capabilitypack.ActionSkillTree, ActionSkillTree:
		observed, exists, err = localprojection.ObserveExactTree(record.Target)
	case ActionAgentFile:
		observed, exists, err = localprojection.FingerprintPath(record.Target)
	default:
		return capabilitypack.ObservedProjection{}, fmt.Errorf("Claude residual projection %s of kind %s requires its historical contract", record.ID, record.Kind)
	}
	if err != nil {
		return capabilitypack.ObservedProjection{}, err
	}
	return capabilitypack.ObservedProjection{ID: record.ID, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: observed, Action: action}, nil
}

func bindAdapterProvenance(observation *capabilitypack.SurfaceInspection) {
	for i := range observation.Projections {
		projection := &observation.Projections[i]
		provenance := projection.AdapterProvenance
		if provenance == "" {
			provenance = projection.Action.AdapterProvenance
		}
		if provenance == "" && projection.Action.Consent == capabilitypack.ConsentExecutableExternal && projection.Action.Source != "" {
			provenance = projection.Action.Source
		}
		if provenance == "" {
			provenance = "claude-projection/v1/" + string(projection.Action.Kind)
		}
		projection.AdapterProvenance = provenance
	}
}

func resourceProjectionRetained(pack capabilitypack.Pack, prior capabilitypack.Resource, priorBinding capabilitypack.Binding) bool {
	for _, resource := range pack.Resources {
		if resource.Kind == prior.Kind && resource.ID == prior.ID {
			binding, ok := claudeBinding(resource)
			return ok && binding.Projection == priorBinding.Projection && binding.Name == priorBinding.Name
		}
	}
	return false
}
func (a *SurfaceAdapter) inspectRemoval(pack capabilitypack.Pack, r capabilitypack.Resource, b capabilitypack.Binding, ownership OwnershipSnapshot) (capabilitypack.ObservedProjection, string, error) {
	id := r.Kind + ":" + b.Name
	switch b.Projection {
	case "skill":
		if r.RequestsSurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilitySkillTemplate) {
			target := filepath.Join(a.layout.SkillsDir, b.Name)
			fp, exists, err := localprojection.ObserveExactTree(target)
			return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillTree, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove exact Claude skill template " + b.Name}}, id + fp, err
		}
		if isClaudeCompositeProjection(pack, r, b) {
			composite, err := claudeCompositeSkill(pack, r, b, a.catalogRoot)
			if err != nil {
				return capabilitypack.ObservedProjection{}, "", err
			}
			provenance, err := canonicalCompositeOwnership(composite.Ownership)
			if err != nil {
				return capabilitypack.ObservedProjection{}, "", err
			}
			target := filepath.Join(a.layout.SkillsDir, b.Name)
			fp, exists, err := observeCompositeTree(target)
			return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, AdapterProvenance: provenance, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillTree, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove exact Claude composite skill " + b.Name}}, id + fp, err
		}
		target := filepath.Join(a.layout.SkillsDir, b.Name)
		fp, exists, err := localprojection.FingerprintPath(target)
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillLink, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove Claude Code skill " + b.Name}}, id + fp, err
	case "agent":
		target := filepath.Join(a.layout.AgentsDir, b.Name+".md")
		fp, exists, err := localprojection.FingerprintPath(target)
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionAgentFile, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove Claude Code agent " + b.Name}}, id + fp, err
	case "instruction":
		current, err := readOptional(a.layout.InstructionsFile)
		if err != nil {
			return capabilitypack.ObservedProjection{}, "", err
		}
		contributor := "pack:" + pack.ID + ":" + r.ID
		merged, err := RemoveInstructionContribution(string(current), contributor)
		if err != nil {
			return capabilitypack.ObservedProjection{}, "", err
		}
		o := ObserveInstructions(a.layout.InstructionsFile)
		if o.Err != nil {
			return capabilitypack.ObservedProjection{}, "", o.Err
		}
		fp, exists := o.Contributions[contributor]
		if !exists {
			fp = "missing"
		}
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionInstructionContribution, Target: a.layout.InstructionsFile, Content: merged, Command: Fingerprint(current), Mode: capabilitypack.ProjectionRemoveContent, Description: "remove Claude Code instruction " + r.ID}}, id + fp, nil
	}
	return capabilitypack.ObservedProjection{}, "", fmt.Errorf("unsupported Claude projection %q", b.Projection)
}

func (a *SurfaceAdapter) ApplyProjections(ctx context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	if claudeProjectActions(actions) {
		executor := localprojection.Executor{Host: "Claude project", TreeKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionClaudeProjectSkillTree: true}, FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionClaudeProjectFile: true, capabilitypack.ActionClaudeProjectInstruction: true,
			capabilitypack.ActionProjectManifestFile: true, capabilitypack.ActionProjectLockFile: true, capabilitypack.ActionProjectNoticesFile: true,
		}}
		if err := executor.Apply(actions); err != nil {
			var actionErr capabilitypack.ProjectionActionError
			if errors.As(err, &actionErr) {
				return &actionErr
			}
			return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
		}
		return nil
	}
	if err := a.validateActions(actions); err != nil {
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	unlock, err := a.lock()
	if err != nil {
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	defer unlock()
	snapshot := OwnershipSnapshot{}
	if a.ownership != nil {
		snapshot, err = a.ownership.ObserveOwnership(ctx)
		if err != nil {
			return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: fmt.Errorf("observe fresh Claude ownership: %w", err)}
		}
	}
	if id, err := a.preflight(actions, snapshot); err != nil {
		return &capabilitypack.ProjectionActionError{ID: id, Err: err}
	}
	filesystemChanges := make([]localprojection.FilesystemChange, 0)
	for _, action := range actions {
		if action.Kind == ActionSkillLink {
			filesystemChanges = append(filesystemChanges, localprojection.FilesystemChange{
				ID: action.ID, Target: action.Target, SymlinkSource: action.Source,
				Delete: action.Mode == capabilitypack.ProjectionDeleteTarget,
			})
			continue
		}
		if action.Kind == capabilitypack.ActionSkillTree {
			change := localprojection.FilesystemChange{ID: action.ID, Target: action.Target, Delete: action.Mode == capabilitypack.ProjectionDeleteTarget, ExpectedTargetFingerprint: action.Precondition, ExactTreeTarget: true}
			if !change.Delete {
				change.FileMode = fs.FileMode(action.FileMode)
				change.ExpectedTreeFingerprint = action.Version
				change.TreeFiles = make([]localprojection.TreeFile, len(action.TreeFiles))
				for i, file := range action.TreeFiles {
					change.TreeFiles[i] = localprojection.TreeFile{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: fs.FileMode(file.Mode)}
				}
			}
			filesystemChanges = append(filesystemChanges, change)
			continue
		}
		if action.Kind != ActionSkillTree {
			continue
		}
		if action.Mode == capabilitypack.ProjectionDeleteTarget {
			filesystemChanges = append(filesystemChanges, localprojection.FilesystemChange{
				ID: action.ID, Target: action.Target, Delete: true,
			})
			continue
		}
		composite, err := decodeCompositeSkillPayload([]byte(action.Content))
		if err != nil {
			return &capabilitypack.ProjectionActionError{ID: action.ID, Err: err}
		}
		filesystemChanges = append(filesystemChanges, localprojection.FilesystemChange{
			ID: action.ID, Target: action.Target, TreeFiles: compositeTreeFiles(composite),
			ExpectedTreeFingerprint: composite.TreeFingerprint,
		})
	}
	if err := localprojection.ApplyFilesystemChanges(filesystemChanges); err != nil {
		var actionErr capabilitypack.ProjectionActionError
		if errors.As(err, &actionErr) {
			return &actionErr
		}
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	appliedShared := map[string]string{}
	for _, action := range actions {
		if action.Kind == ActionSkillTree || action.Kind == capabilitypack.ActionSkillTree || action.Kind == ActionSkillLink {
			continue
		}
		if action.Kind == ActionInstructionContribution {
			if content, ok := appliedShared[action.Target]; ok && content == action.Content {
				continue
			}
			appliedShared[action.Target] = action.Content
		}
		if err := a.apply(ctx, action); err != nil {
			return &capabilitypack.ProjectionActionError{ID: action.ID, Err: err}
		}
	}
	return nil
}

func claudeProjectActions(actions []capabilitypack.ProjectionAction) bool {
	if len(actions) == 0 {
		return true
	}
	for _, action := range actions {
		switch action.Kind {
		case capabilitypack.ActionClaudeProjectSkillTree, capabilitypack.ActionClaudeProjectFile, capabilitypack.ActionClaudeProjectInstruction,
			capabilitypack.ActionProjectManifestFile, capabilitypack.ActionProjectLockFile, capabilitypack.ActionProjectNoticesFile:
		default:
			return false
		}
	}
	return true
}
func (a *SurfaceAdapter) validateActions(actions []capabilitypack.ProjectionAction) error {
	seen := map[string]bool{}
	targets := map[string]capabilitypack.ProjectionAction{}
	for _, x := range actions {
		if x.ID == "" || seen[x.ID] {
			return errors.New("duplicate or empty Claude projection identity")
		}
		seen[x.ID] = true
		switch x.Kind {
		case ActionSkillLink:
			if !directChild(x.Target, a.layout.SkillsDir) {
				return errors.New("Claude skill target must be a direct child of the canonical skills directory")
			}
			if x.Mode != capabilitypack.ProjectionDeleteTarget {
				info, err := os.Stat(x.Source)
				if err != nil || !info.IsDir() {
					return errors.New("Claude skill source must be an existing directory")
				}
				if !filepath.IsAbs(x.Source) {
					return errors.New("Claude skill source must be absolute")
				}
			}
			if exclusiveSkillTargetConflict(targets, x) {
				return errors.New("overlapping exclusive Claude skill target")
			}
			targets[x.Target] = x
		case ActionSkillTree:
			if !directChild(x.Target, a.layout.SkillsDir) {
				return errors.New("Claude composite skill target must be a direct child of the canonical skills directory")
			}
			if x.Mode == capabilitypack.ProjectionDeleteTarget {
				if x.Content != "" || x.AdapterProvenance != "" {
					return errors.New("Claude composite skill removal must not carry replacement content")
				}
			} else {
				composite, err := decodeCompositeSkillPayload([]byte(x.Content))
				if err != nil {
					return err
				}
				provenance, err := decodeCompositeOwnership(x.AdapterProvenance)
				if err != nil {
					return err
				}
				if composite.Ownership != provenance || provenance.EffectiveName != filepath.Base(x.Target) || provenance.TreeFingerprint != composite.TreeFingerprint {
					return errors.New("Claude composite skill action identity does not match its target and payload")
				}
			}
			if exclusiveSkillTargetConflict(targets, x) {
				return errors.New("overlapping exclusive Claude skill target")
			}
			targets[x.Target] = x
		case capabilitypack.ActionSkillTree:
			if !directChild(x.Target, a.layout.SkillsDir) {
				return errors.New("Claude rendered skill target must be a direct child of the canonical skills directory")
			}
			if x.Mode == capabilitypack.ProjectionDeleteTarget {
				if len(x.TreeFiles) != 0 || x.Content != "" || x.Source != "" || x.Version != "" || x.AdapterProvenance != "" {
					return errors.New("Claude rendered skill removal must not carry replacement content")
				}
			} else if len(x.TreeFiles) == 0 || x.Version == "" || x.FileMode != 0o700 || x.Content != "" || x.Source != "" || !strings.HasPrefix(x.AdapterProvenance, skilltemplate.RendererVersion+"/") {
				return errors.New("Claude rendered skill action is not fully sealed")
			}
			if exclusiveSkillTargetConflict(targets, x) {
				return errors.New("overlapping exclusive Claude skill target")
			}
			targets[x.Target] = x
		case ActionAgentFile:
			if !directChild(x.Target, a.layout.AgentsDir) || filepath.Ext(x.Target) != ".md" {
				return errors.New("Claude agent target must be one Markdown file in the canonical agents directory")
			}
			if _, ok := targets[x.Target]; ok {
				return errors.New("overlapping exclusive Claude agent target")
			}
			targets[x.Target] = x
		case ActionInstructionContribution:
			if filepath.Clean(x.Target) != filepath.Clean(a.layout.InstructionsFile) {
				return errors.New("Claude instruction target must be canonical CLAUDE.md")
			}
			if prior, ok := targets[x.Target]; ok && prior.Content != x.Content {
				return errors.New("shared Claude instruction actions must be aggregated into one sealed document")
			}
			targets[x.Target] = x
		default:
			return fmt.Errorf("unsealed Claude projection action kind %q", x.Kind)
		}
	}
	return nil
}
func (a *SurfaceAdapter) apply(ctx context.Context, x capabilitypack.ProjectionAction) error {
	if x.Mode == capabilitypack.ProjectionDeleteTarget {
		if _, err := os.Lstat(x.Target); os.IsNotExist(err) {
			return nil
		}
	}
	if x.Kind == ActionInstructionContribution || x.Kind == ActionAgentFile {
		current, err := os.ReadFile(x.Target)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if x.Mode != capabilitypack.ProjectionDeleteTarget && string(current) == x.Content {
			return nil
		}
		if x.Command != "" && Fingerprint(current) != x.Command {
			return errors.New("stale Claude shared document revision")
		}
		if x.Mode == capabilitypack.ProjectionRemoveContent && string(current) == x.Content {
			return nil
		}
	}
	switch x.Kind {
	case ActionSkillLink:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExact(x.Target)
		}
		expected, err := canonicalPath(x.Source)
		if err != nil {
			return err
		}
		sourceFP, err := localprojection.FingerprintTree(expected)
		if err != nil {
			return err
		}
		observed := ObserveSkill(x.Target, expected)
		if observed.Err != nil {
			return observed.Err
		}
		if observed.Kind == PathSymlink && observed.ResolvedTarget == expected && observed.TreeFingerprint == sourceFP {
			return nil
		}
		return replaceSymlink(x.Source, x.Target)
	case ActionSkillTree:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExactTree(x.Target)
		}
		composite, err := decodeCompositeSkillPayload([]byte(x.Content))
		if err != nil {
			return err
		}
		return localprojection.ReplaceTree(x.Target, compositeTreeFiles(composite), composite.TreeFingerprint)
	case ActionAgentFile, ActionInstructionContribution:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExact(x.Target)
		}
		return atomicWrite(x.Target, []byte(x.Content), 0644)
	}
	return nil
}

func (a *SurfaceAdapter) validateFreshOwnership(x capabilitypack.ProjectionAction, snapshot OwnershipSnapshot) error {
	if snapshot.Revision != "" && snapshot.Revision != canonicalFingerprint(snapshot.Records) {
		return errors.New("stale composite Claude ownership snapshot")
	}
	var record *OwnershipRecord
	ambiguous := false
	for i := range snapshot.Records {
		r := &snapshot.Records[i]
		if r.ID == x.ID {
			if record != nil {
				ambiguous = true
			}
			record = r
		}
	}
	if ambiguous {
		return errors.New("ambiguous Claude ownership")
	}
	if record != nil {
		if record.StateOwner == "" || record.ContributorID == "" || !compatibleClaudeOwnershipKind(record.Kind, x.Kind) || filepath.Clean(record.Target) != filepath.Clean(x.Target) || !slices.Contains(record.Contributors, record.ContributorID) {
			return errors.New("Claude ownership identity does not exactly match the sealed action")
		}
		if x.Kind == ActionInstructionContribution {
			o := ObserveInstructions(x.Target)
			if o.Err != nil {
				return o.Err
			}
			fp, ok := o.Contributions[record.ContributorID]
			if ok && !fingerprintsEqual(fp, record.Fingerprint) {
				return errors.New("owned Claude instruction contribution changed; preserving it")
			}
		}
	}
	if x.Kind == ActionSkillTree {
		observed, exists, err := freshCompositeTreeFingerprint(x.Target)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if record == nil {
			return errors.New("foreign Claude composite skill collision")
		}
		recordProvenance, err := canonicalCompositeOwnership(record.Composite)
		if err != nil {
			return err
		}
		if _, err := decodeCompositeOwnership(recordProvenance); err != nil || record.Composite.TreeFingerprint != record.Fingerprint {
			return errors.New("invalid Claude composite ownership identity")
		}
		if !fingerprintsEqual(observed, record.Fingerprint) {
			return errors.New("owned Claude composite skill changed; preserving it")
		}
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			if !record.DeletionAuthorized || len(record.Contributors) > 1 {
				return errors.New("Claude composite skill deletion is not authorized")
			}
			return nil
		}
		next, err := decodeCompositeOwnership(x.AdapterProvenance)
		if err != nil {
			return err
		}
		if record.Composite.PackID != next.PackID || record.Composite.PortableKind != next.PortableKind || record.Composite.PortableID != next.PortableID || record.Composite.EffectiveName != next.EffectiveName || record.Composite.TargetType != next.TargetType {
			return errors.New("Claude composite replacement changes its owned portable identity")
		}
		return nil
	}
	if x.Kind == capabilitypack.ActionSkillTree {
		observed, exists, err := localprojection.ObserveSkillTarget(x.Target)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if record == nil {
			return errors.New("foreign Claude rendered skill collision")
		}
		if !fingerprintsEqual(observed, record.Fingerprint) {
			return errors.New("owned Claude rendered skill changed; preserving it")
		}
		if x.Mode == capabilitypack.ProjectionDeleteTarget && (!record.DeletionAuthorized || len(record.Contributors) > 1) {
			return errors.New("Claude rendered skill deletion is not authorized")
		}
		return nil
	}
	if x.Kind == ActionSkillLink {
		if record != nil && record.Kind == string(capabilitypack.ActionSkillTree) {
			observed, exists, err := localprojection.ObserveExactTree(x.Target)
			if err != nil {
				return err
			}
			if exists && !fingerprintsEqual(observed, record.Fingerprint) {
				return errors.New("owned Claude rendered skill changed; preserving it")
			}
			if x.Mode == capabilitypack.ProjectionDeleteTarget && (!record.DeletionAuthorized || len(record.Contributors) > 1) {
				return errors.New("Claude rendered skill deletion is not authorized")
			}
			if x.Mode != capabilitypack.ProjectionDeleteTarget {
				_, err = canonicalPath(x.Source)
			}
			return err
		}
		o := ObserveSkill(x.Target, "")
		if o.Err != nil {
			return o.Err
		}
		exists := o.Kind != PathMissing
		if !exists {
			return nil
		}
		if record == nil {
			return errors.New("foreign Claude skill collision")
		}
		if !record.MatchesSkill("claude", x.ID, x.Target, record.Skill.ExpectedSource, o) {
			return errors.New("owned Claude skill identity changed; preserving it")
		}
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			if !record.DeletionAuthorized || len(record.Contributors) > 1 {
				return errors.New("Claude skill deletion is not authorized")
			}
			return nil
		}
		if _, err := canonicalPath(x.Source); err != nil {
			return err
		}
		return nil
	}
	fp, exists, err := localprojection.FingerprintPath(x.Target)
	if err != nil {
		return err
	}
	if x.Mode == capabilitypack.ProjectionDeleteTarget {
		if !exists {
			return nil
		}
		if record == nil || !record.DeletionAuthorized || len(record.Contributors) > 1 {
			return errors.New("Claude deletion is not authorized by the fresh composite ownership snapshot")
		}
		if !fingerprintsEqual(fp, record.Fingerprint) {
			return errors.New("owned Claude projection changed; preserving it")
		}
		if x.Kind == ActionInstructionContribution {
			return errors.New("shared Claude documents cannot be deleted as exclusive targets")
		}
		return nil
	}
	if exists && (x.Kind == ActionSkillLink || x.Kind == ActionAgentFile) {
		if record == nil {
			return errors.New("foreign Claude projection collision")
		}
		if !fingerprintsEqual(fp, record.Fingerprint) {
			return errors.New("owned Claude projection changed; preserving it")
		}
	}
	return nil
}

func (a *SurfaceAdapter) preflight(actions []capabilitypack.ProjectionAction, snapshot OwnershipSnapshot) (string, error) {
	for _, x := range actions {
		if err := a.validateFreshOwnership(x, snapshot); err != nil {
			return x.ID, err
		}
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			if _, err := os.Lstat(x.Target); os.IsNotExist(err) {
				continue
			}
		}
		if x.Kind == ActionInstructionContribution || x.Kind == ActionAgentFile {
			current, err := readOptional(x.Target)
			if err != nil {
				return x.ID, err
			}
			if x.Command != "" && Fingerprint(current) != x.Command && string(current) != x.Content {
				return x.ID, errors.New("stale Claude shared document revision")
			}
		}
	}
	return "", nil
}

func (a *SurfaceAdapter) runtimeReadiness(ctx context.Context, pack capabilitypack.Pack, projections []capabilitypack.ObservedProjection, version VersionObservation, auth AuthorizationObservation) (bool, bool, []string) {
	if a.runtimeEvidence == nil {
		return false, false, nil
	}
	evidence := a.runtimeEvidence.ObserveRuntimeEvidence(ctx)
	policy := RuntimeEvidencePolicyFingerprint(pack, auth)
	byID := make(map[string]RuntimeEvidence, len(evidence))
	for _, item := range evidence {
		byID[item.ID] = item
	}
	facts := []string{}
	participants := 0
	for _, projection := range projections {
		if projection.Goal != capabilitypack.ProjectionPresent || strings.HasPrefix(projection.ID, "asset:") {
			continue
		}
		participants++
		signal := "loading"
		item, ok := byID[projection.ID]
		wantRevision := runtimeEvidenceRevision(pack, projection, version.Version, policy)
		if !ok || item.Kind != string(projection.Action.Kind) || item.Signal != signal || item.Revision != wantRevision {
			return false, false, facts
		}
		facts = append(facts, projection.ID+": current Claude Code runtime "+signal+" evidence")
	}
	if participants == 0 {
		return false, false, nil
	}
	sort.Strings(facts)
	return true, true, facts
}

func runtimeEvidenceRevision(pack capabilitypack.Pack, projection capabilitypack.ObservedProjection, hostVersion, policy string) string {
	return canonicalFingerprint(struct{ PackID, PackVersion, ResourceID, ProjectionID, Projection, Definition, Target, Kind, HostVersion, Policy string }{pack.ID, pack.Version, portableProjectionIdentity(pack, projection.ID), projection.ID, projection.ObservedFingerprint, projection.DesiredFingerprint, projection.Action.Target, string(projection.Action.Kind), hostVersion, policy})
}

func NewRuntimeEvidence(pack capabilitypack.Pack, projection capabilitypack.ObservedProjection, hostVersion string, auth AuthorizationObservation, signal string) RuntimeEvidence {
	return RuntimeEvidence{Kind: string(projection.Action.Kind), ID: projection.ID, Signal: signal, Revision: runtimeEvidenceRevision(pack, projection, hostVersion, RuntimeEvidencePolicyFingerprint(pack, auth))}
}

func RuntimeEvidencePolicyFingerprint(pack capabilitypack.Pack, auth AuthorizationObservation) string {
	return canonicalFingerprint(struct {
		Disabled, Shadowed, Policy, Tools bool
	}{auth.Disabled, auth.Shadowed, auth.PolicyObserved, auth.ToolPermissionObserved})
}

func portableProjectionIdentity(pack capabilitypack.Pack, projectionID string) string {
	for _, resource := range pack.Resources {
		binding, ok := claudeBinding(resource)
		if ok && resource.Kind+":"+binding.Name == projectionID {
			return resource.Kind + ":" + resource.ID
		}
	}
	return ""
}

func optionalAuthorityReadiness(pack capabilitypack.Pack, auth AuthorizationObservation) ([]capabilitypack.OptionalAuthorityObservation, error) {
	declared := make(map[string]capabilitypack.OptionalAuthorityObservation)
	for _, mode := range pack.Contract.OptionalModes {
		for _, authority := range mode.Authorities {
			key := mode.ID + "\x00" + authority
			if _, duplicate := declared[key]; duplicate {
				return nil, fmt.Errorf("duplicate optional authority declaration %s:%s", mode.ID, authority)
			}
			declared[key] = capabilitypack.OptionalAuthorityObservation{ModeID: mode.ID, Authority: authority, State: capabilitypack.OptionalAuthorityUnknown, Fallback: mode.Fallback}
		}
	}
	observed := make(map[string]bool, len(auth.OptionalAuthorities))
	for _, authority := range auth.OptionalAuthorities {
		key := authority.ModeID + "\x00" + authority.Authority
		record, exists := declared[key]
		if !exists {
			return nil, fmt.Errorf("Claude authorization reported undeclared optional authority %s:%s", authority.ModeID, authority.Authority)
		}
		if observed[key] {
			return nil, fmt.Errorf("Claude authorization reported duplicate optional authority %s:%s", authority.ModeID, authority.Authority)
		}
		switch authority.State {
		case capabilitypack.OptionalAuthorityAvailable, capabilitypack.OptionalAuthorityUnavailable, capabilitypack.OptionalAuthorityUnknown:
		default:
			return nil, fmt.Errorf("Claude authorization reported unsupported optional authority state %q", authority.State)
		}
		record.State = authority.State
		declared[key] = record
		observed[key] = true
	}
	result := make([]capabilitypack.OptionalAuthorityObservation, 0, len(declared))
	for _, authority := range declared {
		result = append(result, authority)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ModeID != result[j].ModeID {
			return result[i].ModeID < result[j].ModeID
		}
		return result[i].Authority < result[j].Authority
	})
	return result, nil
}

func (a *SurfaceAdapter) validateSkillAssetClosure(pack capabilitypack.Pack, consumer capabilitypack.Resource, source string) error {
	root, err := canonicalPath(source)
	if err != nil {
		return err
	}
	for _, asset := range dependencyAssets(pack, consumer) {
		path, err := canonicalPath(filepath.Join(asset.CatalogRootOr(a.catalogRoot), filepath.Clean(asset.Source)))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("Claude skill %s dependency asset %s is outside its complete source tree", consumer.ID, asset.ID)
		}
	}
	return nil
}

func (a *SurfaceAdapter) embedConsumerAssets(pack capabilitypack.Pack, consumer capabilitypack.Resource, prompt []byte) ([]byte, error) {
	assets := dependencyAssets(pack, consumer)
	if len(assets) == 0 {
		return prompt, nil
	}
	result := strings.TrimSpace(string(prompt))
	for _, asset := range assets {
		content, err := os.ReadFile(filepath.Join(asset.CatalogRootOr(a.catalogRoot), filepath.Clean(asset.Source)))
		if err != nil {
			return nil, err
		}
		result += fmt.Sprintf("\n\n## Packy dependency asset: %s\n\n%s", asset.ID, strings.TrimSpace(string(content)))
	}
	return []byte(result + "\n"), nil
}

func fingerprintsEqual(a, b string) bool { return a == b || "sha256:"+a == b || a == "sha256:"+b }

func directChild(path, root string) bool {
	return filepath.Dir(filepath.Clean(path)) == filepath.Clean(root) && filepath.Base(path) != "." && filepath.Base(path) != ".."
}
func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}
func (a *SurfaceAdapter) lock() (func(), error) {
	if a.stateRoot == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(a.stateRoot, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(a.stateRoot, "claude-host-effect.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("acquire Claude host-effect lock: %w", err)
	}
	f.Close()
	return func() { _ = os.Remove(p) }, nil
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".packy-claude-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func replaceSymlink(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp := target + ".packy-new"
	_ = os.Remove(tmp)
	if err := os.Symlink(source, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)
	return os.Rename(tmp, target)
}
func removeExact(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func removeExactTree(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Claude composite skill target changed type; preserving it")
	}
	return os.RemoveAll(path)
}

func observeCompositeTree(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "type:symlink", true, nil
	}
	if !info.IsDir() {
		return "type:" + info.Mode().Type().String(), true, nil
	}
	fingerprint, err := localprojection.FingerprintExactTree(path)
	if err != nil {
		return "invalid:" + Fingerprint([]byte(err.Error())), true, nil
	}
	return fingerprint, true, nil
}

func freshCompositeTreeFingerprint(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", true, errors.New("Claude composite skill target has an unexpected path type")
	}
	fingerprint, err := localprojection.FingerprintExactTree(path)
	return fingerprint, true, err
}

func isClaudeCompositeProjection(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding) bool {
	return claudeCompositeSkillBuilder(pack, resource, binding) != nil
}

func exclusiveSkillTargetConflict(targets map[string]capabilitypack.ProjectionAction, action capabilitypack.ProjectionAction) bool {
	target := filepath.Clean(action.Target)
	for existingTarget, existing := range targets {
		existingTarget = filepath.Clean(existingTarget)
		if existingTarget == target {
			return true
		}
		if !isClaudeTreeAction(action.Kind) && !isClaudeTreeAction(existing.Kind) {
			continue
		}
		if pathContains(target, existingTarget) || pathContains(existingTarget, target) {
			return true
		}
	}
	return false
}

func isClaudeTreeAction(kind capabilitypack.ProjectionActionKind) bool {
	return kind == ActionSkillTree || kind == capabilitypack.ActionSkillTree
}

func compatibleClaudeOwnershipKind(recorded string, desired capabilitypack.ProjectionActionKind) bool {
	if recorded == string(desired) {
		return true
	}
	return recorded == string(ActionSkillLink) && desired == capabilitypack.ActionSkillTree || recorded == string(capabilitypack.ActionSkillTree) && desired == ActionSkillLink
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}
func firstID(a []capabilitypack.ProjectionAction) string {
	if len(a) > 0 {
		return a[0].ID
	}
	return "batch"
}
func claudeBinding(r capabilitypack.Resource) (capabilitypack.Binding, bool) {
	for _, b := range r.Bindings {
		if b.Surface == capabilitypack.SurfaceClaude {
			return b, true
		}
	}
	return capabilitypack.Binding{}, false
}
