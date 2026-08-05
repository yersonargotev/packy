package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ProjectInstallPreviewSchemaVersion = 1
	projectContractSchemaV1            = 1
	projectContractSchemaV2            = 2
	projectContractSchemaV3            = 3
	projectContractCapabilityV1        = "project-installation-v1"
	projectContractCapabilityV2        = "project-installation-v2"
	projectContractCapabilityV3        = "project-activation-v1"
)

type ProjectInstallDisposition string

const (
	ProjectInstallPreviewable ProjectInstallDisposition = "previewable"
	ProjectInstallBlocked     ProjectInstallDisposition = "blocked"
	ProjectInstallConverged   ProjectInstallDisposition = "converged"
)

type ProjectInstallRequest struct {
	PackID          string
	Version         string
	Surface         Surface
	ProjectRoot     string
	Selection       ResourceSelection
	Aliases         []SurfaceAlias
	ProviderChoices []ProviderChoice
	manifestPack    ProjectManifestPack
	reconcile       bool
	reconcileAll    bool
	replaceContract bool
}

type ProjectUpdateRequest struct {
	PackID      string
	Version     string
	ProjectRoot string
}

type ProjectInstallBlocker struct {
	Code        string           `json:"code"`
	Resource    ResourceIdentity `json:"resource,omitempty"`
	Target      string           `json:"target,omitempty"`
	Detail      string           `json:"detail"`
	Remediation string           `json:"remediation"`
}

type ProjectContractProposal struct {
	Path          string                `json:"path"`
	SchemaVersion int                   `json:"schema_version"`
	Packs         []ProjectManifestPack `json:"packs"`
}

type ProjectManifestPack struct {
	ID              string                 `json:"id"`
	Version         string                 `json:"version"`
	Surfaces        []Surface              `json:"surfaces"`
	Selection       ResourceSelection      `json:"selection"`
	Aliases         []SurfaceAlias         `json:"aliases"`
	ProviderChoices []ProviderChoice       `json:"provider_choices"`
	SurfaceIntents  []ProjectSurfaceIntent `json:"surface_intents,omitempty"`
}

type ProjectSurfaceIntent struct {
	Surface         Surface           `json:"surface"`
	Selection       ResourceSelection `json:"selection"`
	Aliases         []SurfaceAlias    `json:"aliases"`
	ProviderChoices []ProviderChoice  `json:"provider_choices"`
}

type ProjectLockProposal struct {
	Path                   string                       `json:"path"`
	SchemaVersion          int                          `json:"schema_version"`
	MinimumPackyCapability string                       `json:"minimum_packy_capability"`
	Source                 ProjectPackSourceIdentity    `json:"source"`
	Sources                []ProjectPackSourceIdentity  `json:"sources,omitempty"`
	Packs                  []ProjectResolvedPack        `json:"packs,omitempty"`
	ResourceGraph          ResourceGraph                `json:"resource_graph"`
	Bindings               []LifecycleBinding           `json:"bindings"`
	Degradations           []LifecycleExclusion         `json:"degradations,omitempty"`
	Modes                  []OptionalMode               `json:"modes"`
	Sensitive              []ProjectSensitiveDisclosure `json:"sensitive"`
	ManifestSHA256         string                       `json:"manifest_sha256"`
	NoticesSHA256          string                       `json:"notices_sha256"`
	NoticesFileMode        uint32                       `json:"notices_file_mode"`
	Projections            []ProjectProjectionPlan      `json:"projections"`
}

type ProjectResolvedPack struct {
	ID              string            `json:"id"`
	Version         string            `json:"version"`
	Role            ActivationRole    `json:"role"`
	Selection       ResourceSelection `json:"selection"`
	ProviderChoices []ProviderChoice  `json:"provider_choices"`
	ResourceGraph   ResourceGraph     `json:"resource_graph"`
}

type ProjectNoticesProposal struct {
	Path          string                      `json:"path"`
	Contributions []ProjectNoticeContribution `json:"contributions"`
}

type ProjectSensitiveChange struct {
	Change   string                    `json:"change"`
	Category ProjectActivationCategory `json:"category"`
	Surface  Surface                   `json:"surface"`
	Resource ResourceIdentity          `json:"resource"`
	Detail   string                    `json:"detail"`
}

type ProjectPackSourceIdentity struct {
	PackID           string `json:"pack_id"`
	PackVersion      string `json:"pack_version"`
	ManifestSchema   int    `json:"manifest_schema"`
	SourceID         string `json:"source_id"`
	Provider         string `json:"provider"`
	Repository       string `json:"repository"`
	Commit           string `json:"commit"`
	Tree             string `json:"tree"`
	Reference        string `json:"reference,omitempty"`
	SourceLockSHA256 string `json:"source_lock_sha256"`
}

type ProjectNoticeContribution struct {
	Resource    ResourceIdentity `json:"resource"`
	License     string           `json:"license,omitempty"`
	Attribution string           `json:"attribution,omitempty"`
}

type ProjectSelectionPreview struct {
	Mode      SelectionMode         `json:"mode"`
	Resources []ResourceClosureFact `json:"resources"`
}

type ProjectProjectionPlan struct {
	Resource           ResourceIdentity `json:"resource"`
	Target             string           `json:"target"`
	Mode               string           `json:"mode"`
	FileMode           uint32           `json:"file_mode"`
	DesiredFingerprint string           `json:"desired_fingerprint,omitempty"`
	ObservedState      string           `json:"observed_state"`
	Contributor        string           `json:"contributor"`
	Contributors       []string         `json:"contributors,omitempty"`
	Command            string           `json:"command,omitempty"`
	Args               []string         `json:"args,omitempty"`
	DiscoverableBy     []Surface        `json:"discoverable_by,omitempty"`
}

type JSONProjectInstallPreview struct {
	SchemaVersion    int                       `json:"schema_version"`
	Report           string                    `json:"report"`
	DryRun           bool                      `json:"dry_run"`
	ProjectRoot      string                    `json:"project_root"`
	Pack             ProjectManifestPack       `json:"pack"`
	Surface          Surface                   `json:"surface"`
	Selection        ProjectSelectionPreview   `json:"selection"`
	Manifest         ProjectContractProposal   `json:"manifest"`
	Lock             ProjectLockProposal       `json:"lock"`
	Notices          ProjectNoticesProposal    `json:"notices"`
	Projections      []ProjectProjectionPlan   `json:"projections"`
	Retirements      []ProjectProjectionPlan   `json:"retirements,omitempty"`
	SensitiveChanges []ProjectSensitiveChange  `json:"sensitive_changes,omitempty"`
	Requirements     []string                  `json:"requirements"`
	Blockers         []ProjectInstallBlocker   `json:"blockers"`
	Disposition      ProjectInstallDisposition `json:"disposition"`
	Observation      string                    `json:"observation"`
	projectRoot      string
	pack             Pack
	actions          []ProjectionAction
	noticeContent    string
	noticeMode       uint32
	noticeBefore     string
	noticeIntact     bool
	request          ProjectInstallRequest
	updateRequest    ProjectUpdateRequest
}

type ProjectInstallNotActionableError struct{ Disposition ProjectInstallDisposition }

func (e ProjectInstallNotActionableError) Error() string {
	return fmt.Sprintf("project install preview is not actionable: %s", e.Disposition)
}

type ProjectInstallFreshness struct {
	Disposition ProjectInstallDisposition `json:"disposition"`
	Blockers    []ProjectInstallBlocker   `json:"blockers"`
}

func (f Facade) CheckProjectInstallFreshness(ctx context.Context, preview JSONProjectInstallPreview, adapter SurfaceAdapter) (ProjectInstallFreshness, error) {
	if preview.projectRoot == "" {
		return ProjectInstallFreshness{}, errors.New("project install preview no longer carries its sealed project root")
	}
	request := preview.request
	if request.ProjectRoot == "" {
		request = ProjectInstallRequest{PackID: preview.Pack.ID, Surface: preview.Surface, ProjectRoot: preview.projectRoot}
	}
	fresh, err := f.PreviewProjectInstall(ctx, request, adapter)
	if err != nil {
		return ProjectInstallFreshness{}, err
	}
	if fresh.Observation != preview.Observation {
		return ProjectInstallFreshness{Disposition: ProjectInstallBlocked, Blockers: []ProjectInstallBlocker{{Code: "stale_observation", Detail: "project targets changed after the preview was created", Remediation: "run the install dry-run again to obtain a fresh preview"}}}, nil
	}
	return ProjectInstallFreshness{Disposition: preview.Disposition, Blockers: append([]ProjectInstallBlocker(nil), preview.Blockers...)}, nil
}

// PreviewProjectReconcile derives a repair plan from the exact manifest and
// portable lock already committed to the project. It never floats a version.
func (f Facade) PreviewProjectReconcile(ctx context.Context, projectRoot string, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	return f.PreviewProjectInstall(ctx, ProjectInstallRequest{ProjectRoot: projectRoot, reconcileAll: true}, adapter)
}

func (f Facade) previewCompleteProjectReconcile(ctx context.Context, projectRoot string, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	installation, err := LoadProjectInstallation(projectRoot)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	resolver, ok := adapter.(projectSurfaceAdapterResolver)
	if !ok {
		return JSONProjectInstallPreview{}, errors.New("complete project reconcile requires the installed surface adapter set")
	}
	pack := installation.Manifest.Packs[0]
	intents := projectSurfaceIntents(pack)
	reports := make([]JSONProjectInstallPreview, 0, len(intents))
	for _, intent := range intents {
		surfaceAdapter, found := resolver.projectSurfaceAdapter(intent.Surface)
		if !found {
			return JSONProjectInstallPreview{}, fmt.Errorf("complete project reconcile is missing the %s adapter", intent.Surface)
		}
		report, previewErr := f.previewProjectInstall(ctx, ProjectInstallRequest{
			PackID: pack.ID, Version: pack.Version, Surface: intent.Surface, ProjectRoot: projectRoot,
			Selection: intent.Selection, Aliases: intent.Aliases, ProviderChoices: intent.ProviderChoices,
			manifestPack: pack, reconcile: true,
		}, surfaceAdapter)
		if previewErr != nil {
			return JSONProjectInstallPreview{}, previewErr
		}
		reports = append(reports, report)
	}
	if len(reports) == 0 {
		return JSONProjectInstallPreview{}, errors.New("complete project reconcile has no installed surfaces")
	}
	combined := reports[0]
	combined.Surface = ""
	combined.Pack = pack
	combined.Selection = ProjectSelectionPreview{Mode: pack.Selection.Mode, Resources: append([]ResourceClosureFact(nil), installation.Lock.ResourceGraph.Resources...)}
	combined.Projections = nil
	combined.actions = nil
	combined.Requirements = nil
	combined.Blockers = nil
	combined.Notices.Contributions = nil
	combined.pack.Resources = nil
	combined.request = ProjectInstallRequest{ProjectRoot: projectRoot, reconcileAll: true}
	combined.projectRoot = projectRoot
	combined.Disposition = ProjectInstallConverged
	seenProjection := map[string]bool{}
	seenAction := map[string]bool{}
	seenResource := map[string]bool{}
	seenNotice := map[string]bool{}
	var observations []string
	for _, report := range reports {
		observations = append(observations, string(report.Surface)+"="+report.Observation)
		if report.Disposition == ProjectInstallBlocked {
			combined.Disposition = ProjectInstallBlocked
		} else if report.Disposition == ProjectInstallPreviewable && combined.Disposition == ProjectInstallConverged {
			combined.Disposition = ProjectInstallPreviewable
		}
		combined.Blockers = append(combined.Blockers, report.Blockers...)
		combined.Requirements = append(combined.Requirements, report.Requirements...)
		for _, projection := range report.Projections {
			key := projection.Resource.String() + "\x00" + filepath.Clean(projection.Target)
			if !seenProjection[key] {
				combined.Projections = append(combined.Projections, projection)
				seenProjection[key] = true
			}
		}
		for _, action := range report.actions {
			key := action.ID + "\x00" + filepath.Clean(action.Target)
			if !seenAction[key] {
				combined.actions = append(combined.actions, action)
				seenAction[key] = true
			}
		}
		for _, resource := range report.pack.Resources {
			key := resource.Kind + ":" + resource.ID
			if !seenResource[key] {
				combined.pack.Resources = append(combined.pack.Resources, resource)
				seenResource[key] = true
			}
		}
		for _, notice := range report.Notices.Contributions {
			key := notice.Resource.String()
			if !seenNotice[key] {
				combined.Notices.Contributions = append(combined.Notices.Contributions, notice)
				seenNotice[key] = true
			}
		}
	}
	combined.Requirements = sortedUnique(combined.Requirements)
	sort.Slice(combined.Projections, func(i, j int) bool {
		if combined.Projections[i].Target != combined.Projections[j].Target {
			return combined.Projections[i].Target < combined.Projections[j].Target
		}
		return combined.Projections[i].Resource.String() < combined.Projections[j].Resource.String()
	})
	sort.Strings(observations)
	combined.Observation = sealProjectInstallPreview(combined, strings.Join(observations, "\n"))
	return combined, nil
}

func (f Facade) PreviewProjectInstall(ctx context.Context, request ProjectInstallRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	if request.reconcileAll {
		return withBundleObservation(ctx, f, func(locked Facade) (JSONProjectInstallPreview, error) {
			return locked.previewCompleteProjectReconcile(ctx, request.ProjectRoot, adapter)
		})
	}
	return withBundleObservation(ctx, f, func(locked Facade) (JSONProjectInstallPreview, error) {
		return locked.previewProjectInstall(ctx, request, adapter)
	})
}

// PreviewProjectUpdate changes one exact project pack version across every
// installed surface while preserving each surface's selected intent.
func (f Facade) PreviewProjectUpdate(ctx context.Context, request ProjectUpdateRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (JSONProjectInstallPreview, error) {
		return locked.previewProjectUpdate(ctx, request, adapter)
	})
}

func (f Facade) previewProjectUpdate(ctx context.Context, request ProjectUpdateRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	if request.ProjectRoot == "" || request.PackID == "" || request.Version == "" {
		return JSONProjectInstallPreview{}, errors.New("project update requires the project root, pack, and exact version")
	}
	installation, err := LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	prior := installation.Manifest.Packs[0]
	if prior.ID != request.PackID {
		return JSONProjectInstallPreview{}, fmt.Errorf("project declares capability pack %q, not %q", prior.ID, request.PackID)
	}
	targetPack, err := f.resolveExactProjectUpdatePackUnlocked(request.PackID, request.Version)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	exactCatalog, err := f.catalog.withExactHistoricalCurrentPacks()
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	f.catalog = exactCatalog
	resolver, ok := adapter.(projectSurfaceAdapterResolver)
	if !ok {
		return JSONProjectInstallPreview{}, errors.New("project update requires the installed surface adapter set")
	}
	target := prior
	target.Version = request.Version
	intents := projectSurfaceIntents(prior)
	reports := make([]JSONProjectInstallPreview, 0, len(intents))
	for _, intent := range intents {
		surfaceAdapter, found := resolver.projectSurfaceAdapter(intent.Surface)
		if !found {
			return JSONProjectInstallPreview{}, fmt.Errorf("project update is missing the %s adapter", intent.Surface)
		}
		report, previewErr := f.previewProjectInstall(ctx, ProjectInstallRequest{
			PackID: request.PackID, Version: request.Version, Surface: intent.Surface, ProjectRoot: request.ProjectRoot,
			Selection: intent.Selection, Aliases: intent.Aliases, ProviderChoices: intent.ProviderChoices,
			manifestPack: target, reconcile: true, replaceContract: true,
		}, surfaceAdapter)
		if previewErr != nil {
			return JSONProjectInstallPreview{}, previewErr
		}
		reports = append(reports, report)
	}
	combined, err := combineProjectUpdateReports(request.ProjectRoot, target, reports)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	return f.addProjectUpdateRetirements(ctx, installation, targetPack, combined, resolver)
}

func (f Facade) addProjectUpdateRetirements(ctx context.Context, prior ProjectInstallation, target Pack, combined JSONProjectInstallPreview, resolver projectSurfaceAdapterResolver) (JSONProjectInstallPreview, error) {
	desired := map[string]bool{}
	for _, projection := range combined.Lock.Projections {
		for _, surface := range projectProjectionContributorSurfaces(projection) {
			desired[string(surface)+"\x00"+projection.Resource.String()+"\x00"+filepath.Clean(projection.Target)] = true
		}
	}
	var removals []ProjectionAction
	seenRetirement := map[string]bool{}
	priorHistoryVerified := false
	for _, surface := range prior.Manifest.Packs[0].Surfaces {
		adapter, found := resolver.projectSurfaceAdapter(surface)
		if !found {
			return JSONProjectInstallPreview{}, fmt.Errorf("project update is missing the %s adapter", surface)
		}
		observation, err := inspectSurface(ctx, adapter, SurfaceTransition{ProjectRoot: combined.projectRoot, ProjectInstallation: &prior, ProjectGoal: ProjectionAbsent})
		if err != nil {
			return JSONProjectInstallPreview{}, err
		}
		for _, observed := range observation.Projections {
			resource, err := ParseResourceIdentity(observed.ID)
			if err != nil {
				return JSONProjectInstallPreview{}, err
			}
			relativeTarget, err := RelativeProjectTarget(combined.projectRoot, observed.Action.Target)
			if err != nil {
				return JSONProjectInstallPreview{}, err
			}
			key := string(surface) + "\x00" + resource.String() + "\x00" + filepath.Clean(relativeTarget)
			if desired[key] {
				continue
			}
			if !priorHistoryVerified {
				if _, err := f.resolveExactProjectUpdatePackUnlocked(prior.Manifest.Packs[0].ID, prior.Manifest.Packs[0].Version); err != nil {
					return JSONProjectInstallPreview{}, fmt.Errorf("project update retirement requires the exact immutable prior artifact: %w", err)
				}
				priorHistoryVerified = true
			}
			if err := f.catalog.validateUpdateRoute(target.ID, prior.Manifest.Packs[0].Version, target.Version, target.manifestVersion, surface); err != nil {
				return JSONProjectInstallPreview{}, err
			}
			locked, owned := findProjectLockProjection(prior.Lock, resource, relativeTarget)
			if !owned || !observed.Exists || normalizeProjectProjectionFingerprint(observed.ObservedFingerprint) != locked.DesiredFingerprint {
				combined.Blockers = append(combined.Blockers, ProjectInstallBlocker{Code: "owned_drift", Resource: resource, Target: relativeTarget, Detail: "retiring Packy-owned project target differs from the locked content", Remediation: "restore the locked content before project update"})
				continue
			}
			retirementKey := resource.String() + "\x00" + filepath.Clean(relativeTarget)
			if !seenRetirement[retirementKey] {
				retired := locked
				retired.ObservedState = "retire"
				combined.Retirements = append(combined.Retirements, retired)
				seenRetirement[retirementKey] = true
			}
			action := observed.Action
			action.PreviewOnly = false
			removals = append(removals, action)
		}
	}
	removals = coalesceProjectComposableActions(removals)
	seenAction := map[string]bool{}
	for _, action := range combined.actions {
		seenAction[action.ID+"\x00"+filepath.Clean(action.Target)] = true
	}
	for _, action := range removals {
		key := action.ID + "\x00" + filepath.Clean(action.Target)
		if !seenAction[key] {
			combined.actions = append(combined.actions, action)
			seenAction[key] = true
		}
	}
	if len(combined.Blockers) > 0 {
		combined.Disposition = ProjectInstallBlocked
	}
	combined.Observation = sealProjectInstallPreview(combined, combined.Observation)
	return combined, nil
}

func combineProjectUpdateReports(projectRoot string, target ProjectManifestPack, reports []JSONProjectInstallPreview) (JSONProjectInstallPreview, error) {
	if len(reports) == 0 {
		return JSONProjectInstallPreview{}, errors.New("project update has no installed surfaces")
	}
	installation, err := LoadProjectInstallation(projectRoot)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	combined := reports[0]
	combined.Surface = ""
	combined.Pack = target
	combined.Manifest = ProjectContractProposal{Path: "packy.json", SchemaVersion: projectContractSchemaV3, Packs: []ProjectManifestPack{target}}
	combined.Lock = ProjectLockProposal{Path: "packy.lock.json", SchemaVersion: projectContractSchemaV3, MinimumPackyCapability: projectContractCapabilityV3, Source: reports[0].Lock.Source, NoticesFileMode: installation.Lock.NoticesFileMode}
	combined.Selection = ProjectSelectionPreview{Mode: target.Selection.Mode, Resources: []ResourceClosureFact{}}
	combined.Notices = ProjectNoticesProposal{Path: "PACKY-NOTICES.md", Contributions: []ProjectNoticeContribution{}}
	combined.Projections = nil
	combined.Requirements = nil
	combined.Blockers = nil
	combined.actions = nil
	combined.pack.Resources = nil
	combined.request = ProjectInstallRequest{}
	combined.updateRequest = ProjectUpdateRequest{PackID: target.ID, Version: target.Version, ProjectRoot: projectRoot}
	combined.projectRoot = projectRoot
	combined.Disposition = ProjectInstallPreviewable
	seenAction := map[string]bool{}
	seenResource := map[string]bool{}
	seenNotice := map[string]bool{}
	var observations []string
	for _, report := range reports {
		observations = append(observations, string(report.Surface)+"="+report.Observation)
		combined.Blockers = append(combined.Blockers, report.Blockers...)
		combined.Requirements = append(combined.Requirements, report.Requirements...)
		combined.Lock.Sources = mergeProjectSources(combined.Lock.Sources, report.Lock.Sources, target.ID)
		combined.Lock.Packs = mergeProjectResolvedPacks(combined.Lock.Packs, report.Lock.Packs, target.ID)
		combined.Lock.ResourceGraph = mergeProjectResourceGraphs(combined.Lock.ResourceGraph, report.Lock.ResourceGraph)
		combined.Lock.Bindings = mergeProjectBindings(combined.Lock.Bindings, report.Lock.Bindings)
		combined.Lock.Degradations = mergeProjectDegradations(combined.Lock.Degradations, report.Lock.Degradations)
		combined.Lock.Modes = mergeProjectModes(combined.Lock.Modes, report.Lock.Modes)
		combined.Lock.Sensitive = mergeProjectSensitiveDisclosures(combined.Lock.Sensitive, report.Lock.Sensitive)
		combined.Lock.Projections, report.Projections = mergeProjectProjections(combined.Lock.Projections, report.Lock.Projections, report.Projections)
		combined.Projections, _ = mergeProjectProjections(combined.Projections, report.Projections, nil)
		for _, action := range report.actions {
			key := action.ID + "\x00" + filepath.Clean(action.Target)
			if !seenAction[key] {
				combined.actions = append(combined.actions, action)
				seenAction[key] = true
			}
		}
		for _, resource := range report.pack.Resources {
			key := resource.Kind + ":" + resource.ID
			if !seenResource[key] {
				combined.pack.Resources = append(combined.pack.Resources, resource)
				seenResource[key] = true
			}
		}
		for _, notice := range report.Notices.Contributions {
			key := notice.Resource.String()
			if !seenNotice[key] {
				combined.Notices.Contributions = append(combined.Notices.Contributions, notice)
				seenNotice[key] = true
			}
		}
	}
	combined.Selection.Resources = append([]ResourceClosureFact(nil), combined.Lock.ResourceGraph.Resources...)
	combined.Requirements = sortedUnique(combined.Requirements)
	manifestBytes, err := marshalProjectManifest(combined.Manifest)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	combined.Lock.ManifestSHA256 = fingerprintProjectBytes(manifestBytes)
	combined.Lock.NoticesSHA256 = fingerprintProjectBytes([]byte(renderProjectNoticeBlock(combined)))
	combined.SensitiveChanges = projectSensitiveChanges(installation.Lock, combined.Lock)
	combined.noticeContent, combined.noticeMode, combined.noticeBefore, combined.noticeIntact, combined.Blockers, err = planCombinedProjectUpdateNotices(combined, installation.Lock, combined.Blockers)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if len(combined.Blockers) > 0 {
		combined.Disposition = ProjectInstallBlocked
	} else {
		converged, contractBlockers, inspectErr := inspectProjectContract(combined, installation.Lock, true, true)
		if inspectErr != nil {
			return JSONProjectInstallPreview{}, inspectErr
		}
		combined.Blockers = append(combined.Blockers, contractBlockers...)
		if len(contractBlockers) > 0 {
			combined.Disposition = ProjectInstallBlocked
		} else if converged {
			combined.Disposition = ProjectInstallConverged
		}
	}
	sort.Strings(observations)
	combined.Observation = sealProjectInstallPreview(combined, strings.Join(observations, "\n")+"\nnotices="+combined.noticeBefore)
	return combined, nil
}

func projectSensitiveChanges(prior, desired ProjectLockProposal) []ProjectSensitiveChange {
	key := func(value ProjectSensitiveDisclosure) string {
		return string(value.Surface) + "\x00" + string(value.Category) + "\x00" + value.Resource.String()
	}
	before := map[string]ProjectSensitiveDisclosure{}
	after := map[string]ProjectSensitiveDisclosure{}
	for _, value := range prior.Sensitive {
		before[key(value)] = value
	}
	for _, value := range desired.Sensitive {
		after[key(value)] = value
	}
	keys := map[string]bool{}
	for value := range before {
		keys[value] = true
	}
	for value := range after {
		keys[value] = true
	}
	result := make([]ProjectSensitiveChange, 0, len(keys))
	for value := range keys {
		old, hadOld := before[value]
		current, hasCurrent := after[value]
		change := "changed"
		disclosure := current
		switch {
		case !hadOld:
			change = "added"
		case !hasCurrent:
			change, disclosure = "removed", old
		case digestJSON(old) == digestJSON(current) && digestJSON(prior.Source) == digestJSON(desired.Source):
			continue
		}
		result = append(result, ProjectSensitiveChange{Change: change, Category: disclosure.Category, Surface: disclosure.Surface, Resource: disclosure.Resource, Detail: "requires fresh personal project activation before the changed runtime effect can be used"})
	}
	sort.Slice(result, func(i, j int) bool {
		left := string(result[i].Surface) + "\x00" + string(result[i].Category) + "\x00" + result[i].Resource.String()
		right := string(result[j].Surface) + "\x00" + string(result[j].Category) + "\x00" + result[j].Resource.String()
		return left < right
	})
	return result
}

func planCombinedProjectUpdateNotices(preview JSONProjectInstallPreview, prior ProjectLockProposal, blockers []ProjectInstallBlocker) (string, uint32, string, bool, []ProjectInstallBlocker, error) {
	content, mode, before, intact, noticeBlockers, err := planProjectNotices(preview, true, true, prior.NoticesSHA256)
	return content, mode, before, intact, append(blockers, noticeBlockers...), err
}

func coalesceProjectComposableActions(actions []ProjectionAction) []ProjectionAction {
	result := make([]ProjectionAction, 0, len(actions))
	indices := map[string]int{}
	for _, action := range actions {
		composable := action.Kind == ActionOpenCodeInstructionFile || action.Kind == ActionClaudeProjectInstruction || action.Kind == ActionClaudeProjectMCP
		if !composable {
			result = append(result, action)
			continue
		}
		key := string(action.Surface) + "\x00" + filepath.Clean(action.Target)
		if index, ok := indices[key]; ok {
			result[index] = action
			continue
		}
		indices[key] = len(result)
		result = append(result, action)
	}
	return result
}

func (f Facade) previewProjectInstall(ctx context.Context, request ProjectInstallRequest, adapter SurfaceAdapter) (JSONProjectInstallPreview, error) {
	if request.Surface != SurfaceCodex && request.Surface != SurfaceOpenCode && request.Surface != SurfaceClaude {
		return JSONProjectInstallPreview{}, fmt.Errorf("project installation preview does not support CLI surface %q", request.Surface)
	}
	if request.ProjectRoot == "" {
		return JSONProjectInstallPreview{}, errors.New("project root is required")
	}
	pack, err := f.resolveProjectPackUnlocked(request.PackID, request.Version)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if !projectSupportsSurface(pack.Surfaces, request.Surface) {
		return JSONProjectInstallPreview{}, fmt.Errorf("capability pack %q does not support CLI surface %q", request.PackID, request.Surface)
	}
	selection, err := canonicalSelection(request.Selection)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	selectedPack, err := selectProjectPackResources(pack, selection)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	aliases := cloneAliases(request.Aliases)
	if err := canonicalizeAliases(&aliases); err != nil {
		return JSONProjectInstallPreview{}, err
	}
	for _, alias := range aliases {
		if !idPattern.MatchString(alias.Name) {
			return JSONProjectInstallPreview{}, fmt.Errorf("project alias name %q is invalid", alias.Name)
		}
	}
	providerChoices, err := canonicalProviderChoices(request.ProviderChoices)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if providerChoices == nil {
		providerChoices = []ProviderChoice{}
	}
	explicit := true
	intent := ActivationIntent{PackID: pack.ID, Surface: request.Surface, Version: pack.Version, Active: true, Aliases: aliases, Selection: selection, ProviderChoices: providerChoices, Explicit: &explicit}
	composition, err := f.composeProject(pack, ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}, request.Surface, aliases)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	for _, alias := range aliases {
		matched := false
		for _, composedPack := range composition.packs {
			matched = matched || packHasAliasTarget(composedPack, alias, request.Surface)
		}
		if !matched {
			return JSONProjectInstallPreview{}, fmt.Errorf("project alias %s:%s does not identify a resource in the resulting selected closure bound to %s", alias.Kind, alias.ID, request.Surface)
		}
	}
	selectedPack = composition.combinedPack()
	compositionBlockers := projectCompositionBlockers(composition.blockers)
	source, err := f.catalog.projectPackSourceIdentity(pack)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	resolvedPacks := make([]ProjectResolvedPack, 0, len(composition.activations))
	sources := make([]ProjectPackSourceIdentity, 0, len(composition.activations))
	graph := ResourceGraph{Resources: []ResourceClosureFact{}}
	graphResources := map[ResourceIdentity]bool{}
	for _, activation := range composition.activations {
		resolvedGraph := ResourceGraphFor(activation.Pack, activation.Selection, false)
		resolvedChoices := cloneProviderChoices(activation.ProviderChoices)
		if resolvedChoices == nil {
			resolvedChoices = []ProviderChoice{}
		}
		resolvedPacks = append(resolvedPacks, ProjectResolvedPack{ID: activation.Pack.ID, Version: activation.Pack.Version, Role: activation.Role, Selection: activation.Selection, ProviderChoices: resolvedChoices, ResourceGraph: resolvedGraph})
		resolvedSource, sourceErr := f.catalog.projectPackSourceIdentity(activation.Pack)
		if sourceErr != nil {
			return JSONProjectInstallPreview{}, sourceErr
		}
		sources = append(sources, resolvedSource)
		for _, fact := range resolvedGraph.Resources {
			if !graphResources[fact.Resource] {
				graph.Resources = append(graph.Resources, fact)
				graphResources[fact.Resource] = true
			}
		}
	}
	sort.Slice(resolvedPacks, func(i, j int) bool { return projectResolvedPackLess(resolvedPacks[i], resolvedPacks[j], pack.ID) })
	sort.Slice(sources, func(i, j int) bool { return projectSourceLess(sources[i], sources[j], pack.ID) })
	if len(resolvedPacks) > 0 && resolvedPacks[0].ID == pack.ID {
		providerChoices = cloneProviderChoices(resolvedPacks[0].ProviderChoices)
		if providerChoices == nil {
			providerChoices = []ProviderChoice{}
		}
	}
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{Desired: selectedPack, ProjectRoot: request.ProjectRoot})
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	projections := make([]ProjectProjectionPlan, 0, len(observation.Projections))
	actions := make([]ProjectionAction, 0, len(observation.Projections))
	blockers := append([]ProjectInstallBlocker(nil), compositionBlockers...)
	existingLock, lockExists, lockErr := readExistingProjectLock(request.ProjectRoot)
	var existingInstallation ProjectInstallation
	existingContract := false
	if lockErr != nil {
		blockers = append(blockers, ProjectInstallBlocker{Code: "invalid_project_lock", Target: "packy.lock.json", Detail: lockErr.Error(), Remediation: "restore or remove the invalid project lock before installation"})
	} else if lockExists {
		installation, contractErr := LoadProjectInstallation(request.ProjectRoot)
		if contractErr != nil {
			blockers = append(blockers, ProjectInstallBlocker{Code: "invalid_project_contract", Target: "packy.json", Detail: contractErr.Error(), Remediation: "restore the supported project manifest and lock before installation"})
			lockExists = false
		} else {
			existingLock = installation.Lock
			existingInstallation, existingContract = installation, true
		}
	}
	for _, resource := range observation.Unrepresentable {
		blockers = append(blockers, ProjectInstallBlocker{Code: "unrepresentable_resource", Resource: resource.Resource, Detail: resource.Reason, Remediation: "choose a surface with a declared project-native representation"})
	}
	for _, projection := range observation.Projections {
		projection.ObservedFingerprint = normalizeProjectProjectionFingerprint(projection.ObservedFingerprint)
		projection.DesiredFingerprint = normalizeProjectProjectionFingerprint(projection.DesiredFingerprint)
		resource, err := ParseResourceIdentity(projection.ID)
		if err != nil {
			return JSONProjectInstallPreview{}, fmt.Errorf("project projection identity: %w", err)
		}
		target, err := RelativeProjectTarget(request.ProjectRoot, projection.Action.Target)
		if err != nil {
			return JSONProjectInstallPreview{}, err
		}
		if err := validateProjectTargetPath(request.ProjectRoot, projection.Action.Target); err != nil {
			blockers = append(blockers, ProjectInstallBlocker{Code: "unsafe_path", Resource: resource, Target: target, Detail: err.Error(), Remediation: "remove the unsafe link or non-directory project path before installation"})
		}
		state := "missing"
		if projection.Exists {
			state = "foreign"
			lockedProjection, owned := findProjectLockProjection(existingLock, resource, target)
			if lockExists && owned && (request.replaceContract || lockedProjection.DesiredFingerprint == projection.DesiredFingerprint) {
				state = "owned"
				if projection.ObservedFingerprint != lockedProjection.DesiredFingerprint {
					state = "drifted"
					blockers = append(blockers, ProjectInstallBlocker{Code: "owned_drift", Resource: resource, Target: target, Detail: "Packy-owned project target differs from the locked content", Remediation: "restore the locked content before installation"})
				} else if lockedProjection.DesiredFingerprint != projection.DesiredFingerprint {
					state = "outdated"
				}
			} else {
				blockers = append(blockers, ProjectInstallBlocker{Code: "foreign_target", Resource: resource, Target: target, Detail: "project target already exists without portable Packy ownership", Remediation: "move or remove the foreign target before installation"})
			}
		}
		mode, fileMode := "copy_file", projection.Action.FileMode
		if projection.Action.Kind == ActionCodexProjectSkillTree || projection.Action.Kind == ActionClaudeProjectSkillTree {
			mode, fileMode = "copy_tree", 0o700
		}
		if projection.Action.Kind == ActionInstructionFile || projection.Action.Kind == ActionCodexMCPConfig || projection.Action.Kind == ActionOpenCodeInstructionFile || projection.Action.Kind == ActionClaudeProjectInstruction {
			mode, fileMode = "merge_marked_file", projection.Action.FileMode
		} else if projection.Action.Kind == ActionOpenCodeMCPConfig || projection.Action.Kind == ActionClaudeProjectMCP {
			mode, fileMode = "merge_structured_file", projection.Action.FileMode
		}
		primaryContributor := "surface:" + string(request.Surface) + ":pack:" + pack.ID
		contributors := append(contributorsForSurface(request.Surface, composition.contributorSet(projection.ID)), primaryContributor)
		contributors = sortedUnique(contributors)
		projections = append(projections, ProjectProjectionPlan{Resource: resource, Target: target, Mode: mode, FileMode: fileMode, DesiredFingerprint: projection.DesiredFingerprint, ObservedState: state, Contributor: primaryContributor, Contributors: contributors, Command: projection.Action.Command, Args: append([]string(nil), projection.Action.Args...), DiscoverableBy: append([]Surface(nil), projection.DiscoverableBy...)})
		if state != "owned" && state != "drifted" {
			action := projection.Action
			action.PreviewOnly = false
			actions = append(actions, action)
		}
	}
	actions = coalesceProjectComposableActions(actions)
	sort.Slice(projections, func(i, j int) bool {
		if projections[i].Target != projections[j].Target {
			return projections[i].Target < projections[j].Target
		}
		return projections[i].Resource.String() < projections[j].Resource.String()
	})
	sort.Slice(blockers, func(i, j int) bool {
		if blockers[i].Code != blockers[j].Code {
			return blockers[i].Code < blockers[j].Code
		}
		return blockers[i].Resource.String() < blockers[j].Resource.String()
	})
	notices := make([]ProjectNoticeContribution, 0)
	for _, resource := range selectedPack.Resources {
		if resource.Kind == "notice" {
			notices = append(notices, ProjectNoticeContribution{Resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, License: resource.License, Attribution: resource.Attribution})
		}
	}
	requirements := projectRequirements(selectedPack)
	if aliases == nil {
		aliases = []SurfaceAlias{}
	}
	manifestPack := ProjectManifestPack{ID: pack.ID, Version: pack.Version, Surfaces: []Surface{request.Surface}, Selection: selection, Aliases: aliases, ProviderChoices: providerChoices}
	if request.reconcile {
		manifestPack = request.manifestPack
	} else if existingContract {
		prior := existingInstallation.Manifest.Packs[0]
		if prior.ID != pack.ID || prior.Version != pack.Version {
			blockers = append(blockers, ProjectInstallBlocker{Code: "project_pack_conflict", Detail: "the project already declares a different pack identity or exact version", Remediation: "use project update or uninstall the existing project pack first"})
		}
		manifestPack = prior
		if !projectSupportsSurface(prior.Surfaces, request.Surface) {
			manifestPack = withProjectSurfaceIntent(manifestPack, ProjectSurfaceIntent{Surface: request.Surface, Selection: selection, Aliases: aliases, ProviderChoices: providerChoices})
			blockers = append(blockers, divergentSharedProjectAliasBlockers(existingLock.Projections, projections)...)
		}
	}
	disposition := ProjectInstallPreviewable
	if len(blockers) > 0 {
		disposition = ProjectInstallBlocked
	}
	lockProjections := append([]ProjectProjectionPlan(nil), projections...)
	for i := range lockProjections {
		lockProjections[i].ObservedState = "installed"
	}
	contract := LifecycleContractFor(selectedPack, request.Surface, nil)
	lockBindings := append([]LifecycleBinding{}, contract.Bindings...)
	for i := range lockBindings {
		lockBindings[i].Surface = request.Surface
	}
	lockDegradations := append([]LifecycleExclusion{}, contract.Exclusions...)
	lockModes := append([]OptionalMode{}, contract.OptionalModes...)
	lockSensitive := projectSensitiveDisclosures(selectedPack, request.Surface)
	if existingContract && !request.replaceContract {
		graph = mergeProjectResourceGraphs(existingLock.ResourceGraph, graph)
		resolvedPacks = mergeProjectResolvedPacks(existingLock.Packs, resolvedPacks, pack.ID)
		sources = mergeProjectSources(existingLock.Sources, sources, pack.ID)
		lockBindings = mergeProjectBindings(existingLock.Bindings, lockBindings)
		lockDegradations = mergeProjectDegradations(existingLock.Degradations, lockDegradations)
		lockModes = mergeProjectModes(existingLock.Modes, lockModes)
		lockSensitive = mergeProjectSensitiveDisclosures(existingLock.Sensitive, lockSensitive)
		lockProjections, projections = mergeProjectProjections(existingLock.Projections, lockProjections, projections)
	}
	report := JSONProjectInstallPreview{
		SchemaVersion: ProjectInstallPreviewSchemaVersion, Report: "project-install-preview", DryRun: true,
		ProjectRoot: "<project-root>", Pack: manifestPack, Surface: request.Surface, projectRoot: request.ProjectRoot, pack: selectedPack, actions: actions, request: request,
		Selection:   ProjectSelectionPreview{Mode: selection.Mode, Resources: graph.Resources},
		Manifest:    ProjectContractProposal{Path: "packy.json", SchemaVersion: projectContractSchemaV3, Packs: []ProjectManifestPack{manifestPack}},
		Lock:        ProjectLockProposal{Path: "packy.lock.json", SchemaVersion: projectContractSchemaV3, MinimumPackyCapability: projectContractCapabilityV3, Source: source, Sources: sources, Packs: resolvedPacks, ResourceGraph: graph, Bindings: lockBindings, Degradations: lockDegradations, Modes: lockModes, Sensitive: lockSensitive, Projections: lockProjections},
		Notices:     ProjectNoticesProposal{Path: "PACKY-NOTICES.md", Contributions: notices},
		Projections: projections, Requirements: append([]string{}, requirements...), Blockers: append([]ProjectInstallBlocker{}, blockers...), Disposition: disposition,
	}
	manifestBytes, err := marshalProjectManifest(report.Manifest)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	report.Lock.ManifestSHA256 = fingerprintProjectBytes(manifestBytes)
	report.Lock.NoticesSHA256 = fingerprintProjectBytes([]byte(renderProjectNoticeBlock(report)))
	if lockExists {
		report.Lock.NoticesFileMode = existingLock.NoticesFileMode
	}
	noticeContent, noticeMode, noticeBefore, noticeIntact, noticeBlockers, err := planProjectNotices(report, lockExists, request.reconcile, existingLock.NoticesSHA256)
	if err != nil {
		return JSONProjectInstallPreview{}, err
	}
	if !lockExists {
		report.Lock.NoticesFileMode = noticeMode
	}
	report.noticeContent, report.noticeMode, report.noticeBefore, report.noticeIntact = noticeContent, noticeMode, noticeBefore, noticeIntact
	report.Blockers = append(report.Blockers, noticeBlockers...)
	if len(noticeBlockers) > 0 {
		report.Disposition = ProjectInstallBlocked
	}
	if len(blockers) == 0 {
		allowRefresh := request.reconcile || existingContract && !projectSupportsSurface(existingInstallation.Manifest.Packs[0].Surfaces, request.Surface)
		converged, contractBlockers, err := inspectProjectContract(report, existingLock, lockExists, allowRefresh)
		if err != nil {
			return JSONProjectInstallPreview{}, err
		}
		report.Blockers = append(report.Blockers, contractBlockers...)
		if len(contractBlockers) > 0 {
			report.Disposition = ProjectInstallBlocked
		} else if converged {
			report.Disposition = ProjectInstallConverged
		}
	}
	sort.Slice(report.Blockers, func(i, j int) bool {
		if report.Blockers[i].Code != report.Blockers[j].Code {
			return report.Blockers[i].Code < report.Blockers[j].Code
		}
		if report.Blockers[i].Target != report.Blockers[j].Target {
			return report.Blockers[i].Target < report.Blockers[j].Target
		}
		return report.Blockers[i].Resource.String() < report.Blockers[j].Resource.String()
	})
	report.Observation = sealProjectInstallPreview(report, observationDigest(observation)+"\nnotices="+noticeBefore)
	return report, nil
}

func normalizeProjectProjectionFingerprint(value string) string {
	return strings.TrimPrefix(value, "sha256:")
}

func projectCompositionBlockers(values []PlanBlocker) []ProjectInstallBlocker {
	result := make([]ProjectInstallBlocker, 0, len(values))
	for _, blocker := range values {
		code := "project_dependency"
		remediation := "repair the admitted project pack graph before installation"
		switch {
		case blocker.Kind == BlockerDependency && strings.Contains(blocker.Detail, "multiple eligible providers"):
			code = "ambiguous_provider"
			remediation = "select one eligible provider with --provider"
		case blocker.Kind == BlockerDependency && strings.Contains(blocker.Detail, "no provider"):
			code = "missing_provider"
			remediation = "admit a compatible provider before installation"
		case blocker.Kind == BlockerDependency && strings.Contains(blocker.Detail, "provider choice"):
			code = "invalid_provider_choice"
			remediation = "select an eligible admitted provider"
		case blocker.Kind == BlockerAlias:
			code = "native_name_collision"
			remediation = "supply an explicit valid --alias for one colliding resource"
		case blocker.Kind == BlockerCompatibility:
			code = "unrepresentable_resource"
			remediation = "choose a surface whose native binding or declared degradation represents the complete closure"
		case blocker.Kind == BlockerSharing || blocker.Kind == BlockerIncompatibleContribution || blocker.Kind == BlockerResourceConflict:
			code = "resource_conflict"
			remediation = "repair the conflicting admitted resource contracts before installation"
		}
		result = append(result, ProjectInstallBlocker{Code: code, Detail: blocker.Subject + ": " + blocker.Detail, Remediation: remediation})
	}
	return result
}

func projectResolvedPackLess(left, right ProjectResolvedPack, requestedID string) bool {
	if left.ID == requestedID || right.ID == requestedID {
		return left.ID == requestedID && right.ID != requestedID
	}
	return left.ID < right.ID
}

func projectSourceLess(left, right ProjectPackSourceIdentity, requestedID string) bool {
	if left.PackID == requestedID || right.PackID == requestedID {
		return left.PackID == requestedID && right.PackID != requestedID
	}
	return left.PackID < right.PackID
}

func selectProjectPackResources(pack Pack, selection ResourceSelection) (Pack, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return Pack{}, err
	}
	if selection.Mode == SelectionAll {
		return clonePack(pack), nil
	}
	return selectPackResourceClosure(pack, selection)
}

func (f Facade) resolveProjectPackUnlocked(id, version string) (Pack, error) {
	current, err := f.catalog.catalogMetadata(id)
	if err != nil {
		return Pack{}, err
	}
	if version == "" || version == current.Version {
		return f.catalog.showUnlocked(id)
	}
	if f.catalog.bundleRoot == "" || !validSemver(version) {
		return Pack{}, fmt.Errorf("capability pack %q targets unavailable exact version %q", id, version)
	}
	pack, err := loadHistoricalArtifact(filepath.Join(f.catalog.bundleRoot, "history", id, version), f.catalog.bundleRoot, id, version)
	if err != nil {
		return Pack{}, fmt.Errorf("capability pack %q targets unavailable exact version %q: %w", id, version, err)
	}
	pack.Description = current.Description
	if pack.manifestVersion < manifestSchemaV3 {
		entry, ok := f.catalog.catalogEntry(id)
		if !ok {
			return Pack{}, fmt.Errorf("capability pack %q has no immutable catalog entry", id)
		}
		pack.Surfaces = append([]Surface(nil), entry.Surfaces...)
	}
	return pack, nil
}

func (f Facade) resolveExactProjectUpdatePackUnlocked(id, version string) (Pack, error) {
	current, err := f.catalog.catalogMetadata(id)
	if err != nil {
		return Pack{}, err
	}
	entry, ok := f.catalog.catalogEntry(id)
	if !ok || f.catalog.bundleRoot == "" || !validSemver(version) {
		return Pack{}, fmt.Errorf("capability pack %q targets unavailable exact version %q", id, version)
	}
	pack, err := loadHistoricalArtifact(filepath.Join(f.catalog.bundleRoot, "history", id, version), f.catalog.bundleRoot, id, version)
	if err != nil {
		return Pack{}, fmt.Errorf("capability pack %q targets unavailable exact version %q: %w", id, version, err)
	}
	pack.Description = current.Description
	if pack.manifestVersion < manifestSchemaV3 {
		pack.Surfaces = append([]Surface(nil), entry.Surfaces...)
	}
	return pack, nil
}

func (c Catalog) withExactHistoricalCurrentPacks() (Catalog, error) {
	exact := c
	exact.packs = make([]Pack, 0, len(c.packs))
	for _, metadata := range c.packs {
		facade := Facade{catalog: c}
		pack, err := facade.resolveExactProjectUpdatePackUnlocked(metadata.ID, metadata.Version)
		if err != nil {
			return Catalog{}, err
		}
		exact.packs = append(exact.packs, pack)
	}
	return exact, nil
}

func projectRequirements(pack Pack) []string {
	values := make([]string, 0)
	for _, capability := range pack.Requires.Capabilities {
		values = append(values, "capability:"+capability)
	}
	for _, tool := range pack.Requires.Tools {
		values = append(values, "tool:"+tool)
	}
	for _, resource := range pack.Resources {
		for _, tool := range resource.RequiresTools {
			values = append(values, "tool:"+tool)
		}
		for _, mode := range resource.RuntimeModes {
			for _, requirement := range mode.Requirements {
				values = append(values, string(requirement.Kind)+":"+requirement.ID)
			}
		}
	}
	return sortedUnique(values)
}

func projectSupportsSurface(surfaces []Surface, target Surface) bool {
	for _, surface := range surfaces {
		if surface == target {
			return true
		}
	}
	return false
}

func sortedProjectSurfaces(values []Surface) []Surface {
	seen := map[Surface]bool{}
	result := make([]Surface, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func projectSurfaceIntents(pack ProjectManifestPack) []ProjectSurfaceIntent {
	if len(pack.SurfaceIntents) > 0 {
		result := append([]ProjectSurfaceIntent(nil), pack.SurfaceIntents...)
		return result
	}
	result := make([]ProjectSurfaceIntent, 0, len(pack.Surfaces))
	for _, surface := range pack.Surfaces {
		result = append(result, ProjectSurfaceIntent{
			Surface: surface, Selection: cloneSelection(pack.Selection), Aliases: cloneAliases(pack.Aliases), ProviderChoices: cloneProviderChoices(pack.ProviderChoices),
		})
	}
	return result
}

func withProjectSurfaceIntent(pack ProjectManifestPack, intent ProjectSurfaceIntent) ProjectManifestPack {
	intent.Selection, _ = canonicalSelection(intent.Selection)
	if intent.Aliases == nil {
		intent.Aliases = []SurfaceAlias{}
	}
	if intent.ProviderChoices == nil {
		intent.ProviderChoices = []ProviderChoice{}
	}
	intents := projectSurfaceIntents(pack)
	replaced := false
	for i := range intents {
		if intents[i].Surface == intent.Surface {
			intents[i] = intent
			replaced = true
		}
	}
	if !replaced {
		intents = append(intents, intent)
	}
	sort.Slice(intents, func(i, j int) bool { return intents[i].Surface < intents[j].Surface })
	pack.SurfaceIntents = intents
	pack.Surfaces = make([]Surface, 0, len(intents))
	combined := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{}}
	for _, value := range intents {
		pack.Surfaces = append(pack.Surfaces, value.Surface)
		if value.Selection.Mode == SelectionAll {
			combined = ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
		} else if combined.Mode != SelectionAll {
			combined.Roots = append(combined.Roots, value.Selection.Roots...)
		}
	}
	pack.Selection, _ = canonicalSelection(combined)
	// These legacy aggregate fields remain canonical for schema-v1 readers. The
	// exact host-specific aliases and provider decisions live in SurfaceIntents.
	pack.Aliases = cloneAliases(intents[0].Aliases)
	pack.ProviderChoices = cloneProviderChoices(intents[0].ProviderChoices)
	return pack
}

func withoutProjectSurfaceIntent(pack ProjectManifestPack, surface Surface) ProjectManifestPack {
	intents := projectSurfaceIntents(pack)
	kept := intents[:0]
	for _, intent := range intents {
		if intent.Surface != surface {
			kept = append(kept, intent)
		}
	}
	pack.SurfaceIntents = nil
	pack.Surfaces = nil
	if len(kept) == 0 {
		return pack
	}
	pack.Selection = kept[0].Selection
	pack.Aliases = cloneAliases(kept[0].Aliases)
	pack.ProviderChoices = cloneProviderChoices(kept[0].ProviderChoices)
	for _, intent := range kept {
		pack = withProjectSurfaceIntent(pack, intent)
	}
	if len(kept) == 1 {
		pack.SurfaceIntents = nil
	}
	return pack
}

func divergentSharedProjectAliasBlockers(existing, added []ProjectProjectionPlan) []ProjectInstallBlocker {
	var blockers []ProjectInstallBlocker
	for _, candidate := range added {
		for _, locked := range existing {
			if candidate.Resource != locked.Resource || candidate.DesiredFingerprint != locked.DesiredFingerprint {
				continue
			}
			if filepath.Clean(candidate.Target) == filepath.Clean(locked.Target) {
				continue
			}
			if projectionsShareDiscovery(candidate, locked) {
				blockers = append(blockers, ProjectInstallBlocker{Code: "divergent_shared_alias", Resource: candidate.Resource, Target: candidate.Target, Detail: "Codex and OpenCode aliases diverge for one shared project resource", Remediation: "use the same alias for this shared project resource"})
			}
		}
	}
	return blockers
}

func projectionsShareDiscovery(left, right ProjectProjectionPlan) bool {
	for _, surface := range projectProjectionContributorSurfaces(right) {
		if projectSupportsSurface(left.DiscoverableBy, surface) {
			return true
		}
	}
	for _, surface := range projectProjectionContributorSurfaces(left) {
		if projectSupportsSurface(right.DiscoverableBy, surface) {
			return true
		}
	}
	return false
}

func projectProjectionContributorSurfaces(projection ProjectProjectionPlan) []Surface {
	contributors := append([]string(nil), projection.Contributors...)
	if projection.Contributor != "" {
		contributors = append(contributors, projection.Contributor)
	}
	var surfaces []Surface
	for _, contributor := range contributors {
		if !strings.HasPrefix(contributor, "surface:") {
			continue
		}
		value := strings.TrimPrefix(contributor, "surface:")
		surface, _, ok := strings.Cut(value, ":")
		if ok {
			surfaces = append(surfaces, Surface(surface))
		}
	}
	return sortedProjectSurfaces(surfaces)
}

func mergeProjectResourceGraphs(existing, added ResourceGraph) ResourceGraph {
	byResource := make(map[ResourceIdentity]ResourceClosureFact, len(existing.Resources)+len(added.Resources))
	for _, fact := range existing.Resources {
		byResource[fact.Resource] = fact
	}
	for _, fact := range added.Resources {
		byResource[fact.Resource] = fact
	}
	resources := make([]ResourceClosureFact, 0, len(byResource))
	for _, fact := range byResource {
		resources = append(resources, fact)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Resource.String() < resources[j].Resource.String() })
	return ResourceGraph{Resources: resources}
}

func mergeProjectResolvedPacks(existing, added []ProjectResolvedPack, requestedID string) []ProjectResolvedPack {
	byID := map[string]ProjectResolvedPack{}
	for _, resolution := range existing {
		byID[resolution.ID] = resolution
	}
	for _, resolution := range added {
		if prior, ok := byID[resolution.ID]; ok {
			resolution.ResourceGraph = mergeProjectResourceGraphs(prior.ResourceGraph, resolution.ResourceGraph)
			if resolution.ID == requestedID {
				if prior.Selection.Mode == SelectionAll || resolution.Selection.Mode == SelectionAll {
					resolution.Selection = ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
				} else {
					resolution.Selection, _ = mergeCustomSelections(prior.Selection, resolution.Selection)
				}
			}
		}
		byID[resolution.ID] = resolution
	}
	result := make([]ProjectResolvedPack, 0, len(byID))
	if requested, ok := byID[requestedID]; ok {
		result = append(result, requested)
		delete(byID, requestedID)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result = append(result, byID[id])
	}
	return result
}

func mergeProjectSources(existing, added []ProjectPackSourceIdentity, requestedID string) []ProjectPackSourceIdentity {
	byID := map[string]ProjectPackSourceIdentity{}
	for _, source := range append(append([]ProjectPackSourceIdentity(nil), existing...), added...) {
		byID[source.PackID] = source
	}
	result := make([]ProjectPackSourceIdentity, 0, len(byID))
	if source, ok := byID[requestedID]; ok {
		result = append(result, source)
		delete(byID, requestedID)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result = append(result, byID[id])
	}
	return result
}

func mergeProjectBindings(existing, added []LifecycleBinding) []LifecycleBinding {
	result := append([]LifecycleBinding{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[digestJSON(value)] = true
	}
	for _, value := range added {
		if !seen[digestJSON(value)] {
			result = append(result, value)
			seen[digestJSON(value)] = true
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		if result[i].Projection != result[j].Projection {
			return result[i].Projection < result[j].Projection
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func mergeProjectDegradations(existing, added []LifecycleExclusion) []LifecycleExclusion {
	result := append([]LifecycleExclusion(nil), existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[digestJSON(value)] = true
	}
	for _, value := range added {
		if !seen[digestJSON(value)] {
			result = append(result, value)
			seen[digestJSON(value)] = true
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Surface < result[j].Surface
	})
	return result
}

func mergeProjectModes(existing, added []OptionalMode) []OptionalMode {
	result := append([]OptionalMode{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[value.ID] = true
	}
	for _, value := range added {
		if !seen[value.ID] {
			result = append(result, value)
			seen[value.ID] = true
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func mergeProjectSensitiveDisclosures(existing, added []ProjectSensitiveDisclosure) []ProjectSensitiveDisclosure {
	return deduplicateProjectSensitiveDisclosures(append(append([]ProjectSensitiveDisclosure{}, existing...), added...))
}

func mergeProjectProjections(existing, added, preview []ProjectProjectionPlan) ([]ProjectProjectionPlan, []ProjectProjectionPlan) {
	result := append([]ProjectProjectionPlan(nil), existing...)
	for i := range added {
		match := -1
		for j := range result {
			if result[j].Resource == added[i].Resource && filepath.Clean(result[j].Target) == filepath.Clean(added[i].Target) && result[j].DesiredFingerprint == added[i].DesiredFingerprint {
				match = j
				break
			}
		}
		if match < 0 {
			result = append(result, added[i])
			continue
		}
		contributors := append([]string(nil), result[match].Contributors...)
		if len(contributors) == 0 && result[match].Contributor != "" {
			contributors = append(contributors, result[match].Contributor)
		}
		contributors = append(contributors, added[i].Contributors...)
		contributors = append(contributors, added[i].Contributor)
		contributors = sortedUnique(contributors)
		result[match].Contributors = contributors
		for j := range preview {
			if preview[j].Resource == added[i].Resource && filepath.Clean(preview[j].Target) == filepath.Clean(added[i].Target) {
				preview[j].Contributor = result[match].Contributor
				preview[j].Contributors = append([]string(nil), contributors...)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Target != result[j].Target {
			return result[i].Target < result[j].Target
		}
		return result[i].Resource.String() < result[j].Resource.String()
	})
	return result, preview
}

func sealProjectInstallPreview(report JSONProjectInstallPreview, surfaceObservation string) string {
	report.Observation = ""
	data, _ := json.Marshal(struct {
		Report             JSONProjectInstallPreview
		SurfaceObservation string
	}{Report: report, SurfaceObservation: surfaceObservation})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c Catalog) projectPackSourceIdentity(pack Pack) (ProjectPackSourceIdentity, error) {
	registryData, err := os.ReadFile(filepath.Join(c.bundleRoot, "sources.json"))
	if err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("read admitted Pack Source registry: %w", err)
	}
	var registry struct {
		Sources []struct {
			ID         string `json:"id"`
			Provider   string `json:"provider"`
			Repository string `json:"repository"`
			Resources  []struct {
				PackID string `json:"pack_id"`
			} `json:"resources"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(registryData, &registry); err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("decode admitted Pack Source registry: %w", err)
	}
	var matches []struct{ ID, Provider, Repository string }
	for _, source := range registry.Sources {
		for _, resource := range source.Resources {
			if resource.PackID == pack.ID {
				matches = append(matches, struct{ ID, Provider, Repository string }{source.ID, source.Provider, source.Repository})
				break
			}
		}
	}
	if len(matches) != 1 {
		return ProjectPackSourceIdentity{}, fmt.Errorf("pack %q does not resolve to one exact admitted Pack Source", pack.ID)
	}
	source := matches[0]
	lockData, err := os.ReadFile(filepath.Join(c.bundleRoot, "sources", source.ID+".lock.json"))
	if err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("read admitted Pack Source lock %q: %w", source.ID, err)
	}
	var lock struct {
		SourceID   string `json:"source_id"`
		Provider   string `json:"provider"`
		Repository string `json:"repository"`
		Candidate  struct {
			Commit     string `json:"commit"`
			Tree       string `json:"tree"`
			TagRefName string `json:"tag_ref_name"`
		} `json:"candidate"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		return ProjectPackSourceIdentity{}, fmt.Errorf("decode admitted Pack Source lock %q: %w", source.ID, err)
	}
	if lock.SourceID != source.ID || lock.Provider != source.Provider || lock.Repository != source.Repository || lock.Candidate.Commit == "" || lock.Candidate.Tree == "" {
		return ProjectPackSourceIdentity{}, fmt.Errorf("admitted Pack Source lock %q lacks an exact immutable identity", source.ID)
	}
	digest := sha256.Sum256(lockData)
	return ProjectPackSourceIdentity{
		PackID: pack.ID, PackVersion: pack.Version, ManifestSchema: pack.manifestVersion,
		SourceID: source.ID, Provider: source.Provider, Repository: source.Repository,
		Commit: lock.Candidate.Commit, Tree: lock.Candidate.Tree, Reference: lock.Candidate.TagRefName,
		SourceLockSHA256: hex.EncodeToString(digest[:]),
	}, nil
}

func DiscoverProjectRoot(currentDirectory string) (string, error) {
	if currentDirectory == "" {
		return "", errors.New("current directory is unavailable")
	}
	absolute, err := filepath.Abs(currentDirectory)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	for candidate := filepath.Clean(canonical); ; candidate = filepath.Dir(candidate) {
		marker := filepath.Join(candidate, ".git")
		info, statErr := os.Lstat(marker)
		if statErr == nil {
			valid, markerErr := validGitWorktreeMarker(candidate, marker, info)
			if markerErr != nil {
				return "", markerErr
			}
			if valid {
				return candidate, nil
			}
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect Git worktree marker: %w", statErr)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return "", fmt.Errorf("current directory %q is outside a Git worktree", currentDirectory)
}

func validGitWorktreeMarker(root, marker string, info os.FileInfo) (bool, error) {
	if info.IsDir() {
		return validGitDirectory(marker)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return false, fmt.Errorf("read Git worktree marker: %w", err)
	}
	value := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(value, prefix) {
		return false, nil
	}
	gitDirectory := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if !filepath.IsAbs(gitDirectory) {
		gitDirectory = filepath.Join(root, gitDirectory)
	}
	return validGitDirectory(gitDirectory)
}

func validGitDirectory(gitDirectory string) (bool, error) {
	head, err := os.ReadFile(filepath.Join(gitDirectory, "HEAD"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read Git worktree HEAD: %w", err)
	}
	if strings.TrimSpace(string(head)) == "" {
		return false, nil
	}
	commonDirectory := gitDirectory
	commondir, err := os.ReadFile(filepath.Join(gitDirectory, "commondir"))
	if err == nil {
		commonDirectory = strings.TrimSpace(string(commondir))
		if commonDirectory == "" {
			return false, nil
		}
		if !filepath.IsAbs(commonDirectory) {
			commonDirectory = filepath.Join(gitDirectory, commonDirectory)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("read linked Git common directory: %w", err)
	}
	for _, name := range []string{"objects", "refs"} {
		info, err := os.Stat(filepath.Join(commonDirectory, name))
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("inspect Git %s directory: %w", name, err)
		}
		if !info.IsDir() {
			return false, nil
		}
	}
	return true, nil
}

func RelativeProjectTarget(projectRoot, target string) (string, error) {
	relative, err := filepath.Rel(projectRoot, target)
	if err != nil {
		return "", err
	}
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project target %q escapes the project root", target)
	}
	return filepath.Clean(relative), nil
}

func validateProjectTargetPath(projectRoot, target string) error {
	relative, err := RelativeProjectTarget(projectRoot, target)
	if err != nil {
		return err
	}
	current := filepath.Clean(projectRoot)
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("project target ancestor %q is not a real directory", current)
		}
	}
	if info, err := os.Lstat(filepath.Clean(target)); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("project target %q is an unsafe filesystem object", target)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
