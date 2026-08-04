package capabilitypack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const ProjectStatusSchemaVersion = 1

type ProjectInstallationState string

const (
	ProjectInstallationAbsent    ProjectInstallationState = "absent"
	ProjectInstallationInstalled ProjectInstallationState = "installed"
	ProjectInstallationDrifted   ProjectInstallationState = "drifted"
	ProjectInstallationBlocked   ProjectInstallationState = "blocked"
)

type ProjectRuntimeState string

const (
	ProjectRuntimeNotRequired ProjectRuntimeState = "not-required"
	ProjectRuntimePending     ProjectRuntimeState = "pending"
	ProjectRuntimeActive      ProjectRuntimeState = "active"
	ProjectRuntimeStale       ProjectRuntimeState = "stale"
	ProjectRuntimeBlocked     ProjectRuntimeState = "blocked"
)

type ProjectProjectionStatus struct {
	Resource            ResourceIdentity `json:"resource"`
	Target              string           `json:"target"`
	Mode                string           `json:"mode"`
	Health              string           `json:"health"`
	ObservedFingerprint string           `json:"observed_fingerprint"`
	DesiredFingerprint  string           `json:"desired_fingerprint"`
	Contributor         string           `json:"contributor"`
}

type JSONProjectPackStatus struct {
	Pack                 ProjectManifestPack       `json:"pack"`
	Surface              Surface                   `json:"surface"`
	Installation         ProjectInstallationState  `json:"installation"`
	Runtime              ProjectRuntimeState       `json:"runtime"`
	Projections          []ProjectProjectionStatus `json:"projections"`
	Blockers             []ProjectInstallBlocker   `json:"blockers"`
	Requirement          string                    `json:"requirement,omitempty"`
	RequirementSatisfied bool                      `json:"requirement_satisfied"`
}

type JSONProjectStatusReport struct {
	SchemaVersion int                     `json:"schema_version"`
	Report        string                  `json:"report"`
	ProjectRoot   string                  `json:"project_root"`
	Packs         []JSONProjectPackStatus `json:"packs"`
}

type ProjectStatusRequest struct {
	ProjectRoot      string
	PackID           string
	Surface          Surface
	RequireInstalled bool
	Adapters         map[Surface]SurfaceAdapter
}

type ProjectInstallation struct {
	Manifest ProjectContractProposal
	Lock     ProjectLockProposal
}

type projectManifestDocument struct {
	SchemaVersion          int                   `json:"schema_version"`
	MinimumPackyCapability string                `json:"minimum_packy_capability"`
	Packs                  []ProjectManifestPack `json:"packs"`
}

var projectDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// LoadProjectInstallation interprets the supported self-contained project
// contract without consulting Packy's catalog, network, or personal state.
func LoadProjectInstallation(projectRoot string) (ProjectInstallation, error) {
	manifestPath := filepath.Join(projectRoot, "packy.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return ProjectInstallation{}, fmt.Errorf("inspect project manifest: %w", err)
	}
	if !manifestInfo.Mode().IsRegular() {
		return ProjectInstallation{}, errors.New("project manifest is not a regular file")
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return ProjectInstallation{}, fmt.Errorf("read project manifest: %w", err)
	}
	var document projectManifestDocument
	if err := strictDecode(manifestData, &document); err != nil {
		return ProjectInstallation{}, fmt.Errorf("decode project manifest: %w", err)
	}
	if document.SchemaVersion != 1 || document.MinimumPackyCapability != "project-installation-v1" {
		return ProjectInstallation{}, errors.New("project manifest schema or minimum Packy capability is unsupported")
	}
	manifest := ProjectContractProposal{Path: "packy.json", SchemaVersion: document.SchemaVersion, Packs: document.Packs}
	lock, exists, err := readExistingProjectLock(projectRoot)
	if err != nil {
		return ProjectInstallation{}, err
	}
	if !exists {
		return ProjectInstallation{}, errors.New("project lock is missing")
	}
	if err := validateProjectInstallation(manifest, lock); err != nil {
		return ProjectInstallation{}, err
	}
	return ProjectInstallation{Manifest: manifest, Lock: lock}, nil
}

func InspectProjectStatus(ctx context.Context, request ProjectStatusRequest) (JSONProjectStatusReport, error) {
	report := JSONProjectStatusReport{SchemaVersion: ProjectStatusSchemaVersion, Report: "project-status", ProjectRoot: "<project-root>", Packs: []JSONProjectPackStatus{}}
	manifestMissing, err := projectPathMissing(filepath.Join(request.ProjectRoot, "packy.json"))
	if err != nil {
		return report, err
	}
	lockMissing, err := projectPathMissing(filepath.Join(request.ProjectRoot, "packy.lock.json"))
	if err != nil {
		return report, err
	}
	if manifestMissing && lockMissing {
		if request.PackID != "" && request.Surface != "" {
			requirementSatisfied := !request.RequireInstalled
			report.Packs = append(report.Packs, JSONProjectPackStatus{
				Pack:    ProjectManifestPack{ID: request.PackID, Surfaces: []Surface{request.Surface}, Selection: ResourceSelection{Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}},
				Surface: request.Surface, Installation: ProjectInstallationAbsent, Runtime: ProjectRuntimePending,
				Projections: []ProjectProjectionStatus{}, Blockers: []ProjectInstallBlocker{}, Requirement: map[bool]string{true: "installed"}[request.RequireInstalled], RequirementSatisfied: requirementSatisfied,
			})
		}
		return report, nil
	}
	if manifestMissing || lockMissing {
		return report, errors.New("project installation is incomplete: packy.json and packy.lock.json must either both exist or both be absent")
	}
	installation, err := LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return report, err
	}
	manifestBytes, err := marshalProjectManifest(installation.Manifest)
	if err != nil {
		return report, err
	}
	manifestHealthy := fingerprintProjectBytes(manifestBytes) == installation.Lock.ManifestSHA256
	contractHealthy, contractBlockers, err := inspectOfflineProjectFiles(request.ProjectRoot, installation, manifestBytes)
	if err != nil {
		return report, err
	}
	pack := installation.Manifest.Packs[0]
	for _, surface := range pack.Surfaces {
		if request.PackID != "" && request.PackID != pack.ID {
			continue
		}
		if request.Surface != "" && request.Surface != surface {
			continue
		}
		adapter := request.Adapters[surface]
		if adapter == nil {
			return report, fmt.Errorf("project installation inspection does not support CLI surface %q", surface)
		}
		observation, inspectErr := inspectSurface(ctx, adapter, SurfaceTransition{
			ProjectRoot: request.ProjectRoot, ProjectInstallation: &installation, ProjectGoal: ProjectionPresent,
		})
		if inspectErr != nil {
			return report, inspectErr
		}
		projections, inspectErr := projectProjectionStatusesFromObservation(request.ProjectRoot, installation.Lock, observation)
		if inspectErr != nil {
			return report, inspectErr
		}
		blockers := append([]ProjectInstallBlocker{}, contractBlockers...)
		state := ProjectInstallationInstalled
		if !manifestHealthy {
			state = ProjectInstallationBlocked
			blockers = append(blockers, ProjectInstallBlocker{Code: "manifest_lock_mismatch", Target: "packy.json", Detail: "the human manifest does not match the exact generated lock", Remediation: "run `packy pack install` to reconcile the project contract"})
		} else if !contractHealthy {
			state = ProjectInstallationDrifted
		}
		for _, projection := range projections {
			if projection.Health != "verified" && state == ProjectInstallationInstalled {
				state = ProjectInstallationDrifted
			}
			if projection.Health != "verified" {
				blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Resource: projection.Resource, Target: projection.Target, Detail: "the installed project projection is " + projection.Health, Remediation: "restore the exact locked projection with `packy pack install`"})
			}
		}
		sort.Slice(blockers, func(i, j int) bool {
			if blockers[i].Code != blockers[j].Code {
				return blockers[i].Code < blockers[j].Code
			}
			return blockers[i].Target < blockers[j].Target
		})
		runtime := ProjectRuntimeNotRequired
		if len(installation.Lock.Modes) > 0 {
			runtime = ProjectRuntimePending
		}
		status := JSONProjectPackStatus{Pack: pack, Surface: surface, Installation: state, Runtime: runtime, Projections: projections, Blockers: blockers, RequirementSatisfied: true}
		if request.RequireInstalled {
			status.Requirement = "installed"
			status.RequirementSatisfied = state == ProjectInstallationInstalled
		}
		report.Packs = append(report.Packs, status)
	}
	if len(report.Packs) == 0 {
		return report, fmt.Errorf("pack %q on %s is not declared by this project installation", request.PackID, request.Surface)
	}
	return report, nil
}

func projectProjectionStatusesFromObservation(projectRoot string, lock ProjectLockProposal, observation SurfaceInspection) ([]ProjectProjectionStatus, error) {
	observed := make(map[ResourceIdentity]ObservedProjection, len(observation.Projections))
	for _, projection := range observation.Projections {
		resource, err := ParseResourceIdentity(projection.ID)
		if err != nil {
			return nil, fmt.Errorf("project projection identity: %w", err)
		}
		observed[resource] = projection
	}
	statuses := make([]ProjectProjectionStatus, 0, len(lock.Projections))
	for _, locked := range lock.Projections {
		projection, ok := observed[locked.Resource]
		if !ok {
			return nil, fmt.Errorf("project adapter omitted locked projection %s", locked.Resource)
		}
		target, err := RelativeProjectTarget(projectRoot, projection.Action.Target)
		if err != nil {
			return nil, err
		}
		if filepath.Clean(filepath.FromSlash(locked.Target)) != filepath.Clean(target) {
			return nil, fmt.Errorf("project lock projection %s target %q does not match the re-derived target %q", locked.Resource, locked.Target, filepath.ToSlash(target))
		}
		health := "missing"
		if projection.Exists {
			health = "drifted"
			if projection.ObservedFingerprint == locked.DesiredFingerprint {
				health = "verified"
			}
		}
		statuses = append(statuses, ProjectProjectionStatus{
			Resource: locked.Resource, Target: locked.Target, Mode: locked.Mode, Health: health,
			ObservedFingerprint: projection.ObservedFingerprint, DesiredFingerprint: locked.DesiredFingerprint, Contributor: locked.Contributor,
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

func projectPathMissing(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	return false, err
}

func validateProjectInstallation(manifest ProjectContractProposal, lock ProjectLockProposal) error {
	if len(manifest.Packs) != 1 {
		return errors.New("project manifest must contain exactly one pack in schema version 1")
	}
	pack := manifest.Packs[0]
	if !idPattern.MatchString(pack.ID) || !semverPattern.MatchString(pack.Version) {
		return errors.New("project manifest pack identity must use a valid ID and exact semantic version")
	}
	if len(pack.Surfaces) != 1 || pack.Surfaces[0] != SurfaceCodex {
		return errors.New("project manifest schema version 1 supports exactly the Codex surface")
	}
	selection, err := canonicalSelection(pack.Selection)
	if err != nil || digestJSON(selection) != digestJSON(pack.Selection) || pack.Selection.Roots == nil || pack.Aliases == nil || pack.ProviderChoices == nil {
		return errors.New("project manifest selection, aliases, and provider choices are incomplete")
	}
	aliases := cloneAliases(pack.Aliases)
	if err := canonicalizeAliases(&aliases); err != nil || digestJSON(aliases) != digestJSON(pack.Aliases) {
		return errors.New("project manifest aliases are invalid or non-canonical")
	}
	providerChoices, err := canonicalProviderChoices(pack.ProviderChoices)
	if providerChoices == nil && pack.ProviderChoices != nil {
		providerChoices = []ProviderChoice{}
	}
	if err != nil || digestJSON(providerChoices) != digestJSON(pack.ProviderChoices) {
		return errors.New("project manifest provider choices are invalid or non-canonical")
	}
	if lock.Path != "packy.lock.json" || lock.Source.PackID != pack.ID || lock.Source.PackVersion != pack.Version || lock.Source.ManifestSchema < manifestSchemaV1 || lock.Source.ManifestSchema > manifestSchemaV4 || lock.Source.SourceID == "" || lock.Source.Provider == "" || lock.Source.Repository == "" || lock.Source.Commit == "" || lock.Source.Tree == "" || !projectDigestPattern.MatchString(lock.Source.SourceLockSHA256) {
		return errors.New("project lock source identity does not exactly match the manifest pack")
	}
	if (lock.Sources == nil) != (lock.Packs == nil) {
		return errors.New("project lock resolved packs and sources must be present together")
	}
	if lock.Packs != nil {
		if len(lock.Packs) == 0 || len(lock.Packs) != len(lock.Sources) {
			return errors.New("project lock resolved packs and sources are incomplete")
		}
		sources := make(map[string]ProjectPackSourceIdentity, len(lock.Sources))
		for _, source := range lock.Sources {
			if source.PackID == "" || sources[source.PackID].PackID != "" || source.PackVersion == "" || source.SourceID == "" || source.Provider == "" || source.Repository == "" || source.Commit == "" || source.Tree == "" || !projectDigestPattern.MatchString(source.SourceLockSHA256) {
				return errors.New("project lock contains an incomplete or duplicate admitted source")
			}
			sources[source.PackID] = source
		}
		requested := false
		resolved := make(map[string]ProjectResolvedPack, len(lock.Packs))
		for _, resolution := range lock.Packs {
			canonical, selectionErr := canonicalSelection(resolution.Selection)
			choices, choicesErr := canonicalProviderChoices(resolution.ProviderChoices)
			if choices == nil && resolution.ProviderChoices != nil {
				choices = []ProviderChoice{}
			}
			source, sourceExists := sources[resolution.ID]
			if resolution.ID == "" || resolved[resolution.ID].ID != "" || !semverPattern.MatchString(resolution.Version) || !sourceExists || source.PackVersion != resolution.Version || selectionErr != nil || digestJSON(canonical) != digestJSON(resolution.Selection) || resolution.ProviderChoices == nil || choicesErr != nil || digestJSON(choices) != digestJSON(resolution.ProviderChoices) || resolution.ResourceGraph.Resources == nil {
				return errors.New("project lock contains an incomplete or duplicate resolved pack")
			}
			if resolution.ID == pack.ID {
				if resolution.Role != ActivationRequested || resolution.Version != pack.Version || digestJSON(resolution.Selection) != digestJSON(pack.Selection) || digestJSON(resolution.ProviderChoices) != digestJSON(pack.ProviderChoices) {
					return errors.New("project lock requested resolution does not match manifest intent")
				}
				requested = true
			} else if resolution.Role != ActivationRequired {
				return errors.New("project lock provider resolution has an invalid role")
			}
			resolved[resolution.ID] = resolution
		}
		if !requested || digestJSON(lock.Sources[0]) != digestJSON(lock.Source) {
			return errors.New("project lock does not lead with the exact requested pack source")
		}
		for _, resolution := range lock.Packs {
			for _, choice := range resolution.ProviderChoices {
				provider, exists := resolved[choice.ProviderPack]
				if !exists || provider.Role != ActivationRequired {
					return fmt.Errorf("project lock provider choice %s does not name a required resolved pack", choice.Capability)
				}
				if choice.ProviderResource != nil && !projectGraphContains(provider.ResourceGraph, *choice.ProviderResource) {
					return fmt.Errorf("project lock provider choice %s does not bind its exact provider resource", choice.Capability)
				}
			}
		}
		expectedGraph := make(map[ResourceIdentity]ResourceClosureFact)
		for _, resolution := range lock.Packs {
			for _, fact := range resolution.ResourceGraph.Resources {
				if previous, exists := expectedGraph[fact.Resource]; exists && digestJSON(previous) != digestJSON(fact) {
					return fmt.Errorf("project lock resolved packs disagree on shared resource %s", fact.Resource)
				}
				expectedGraph[fact.Resource] = fact
			}
		}
		if len(expectedGraph) != len(lock.ResourceGraph.Resources) {
			return errors.New("project lock combined resource graph does not match resolved pack graphs")
		}
		for _, fact := range lock.ResourceGraph.Resources {
			if expected, exists := expectedGraph[fact.Resource]; !exists || digestJSON(expected) != digestJSON(fact) {
				return fmt.Errorf("project lock combined resource %s does not match resolved pack graphs", fact.Resource)
			}
		}
	}
	if lock.ResourceGraph.Resources == nil || lock.Bindings == nil || lock.Modes == nil || lock.Projections == nil || !projectDigestPattern.MatchString(lock.ManifestSHA256) || !projectDigestPattern.MatchString(lock.NoticesSHA256) || lock.NoticesFileMode == 0 {
		return errors.New("project lock omits required closure, identity, mode, or notice evidence")
	}
	resources := make(map[ResourceIdentity]bool, len(lock.ResourceGraph.Resources))
	for _, fact := range lock.ResourceGraph.Resources {
		if fact.Resource.Kind == "" || fact.Resource.ID == "" || resources[fact.Resource] || !validProjectResourceRole(fact.Role) || fact.DependencyChain == nil || fact.Requires == nil || fact.Notices == nil {
			return errors.New("project lock resource graph is malformed or contains duplicate identities")
		}
		resources[fact.Resource] = true
	}
	bindings := make(map[ResourceIdentity]bool, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		identity := ResourceIdentity{Kind: binding.Kind, ID: binding.ID}
		validMode := binding.Mode == "native" && binding.Degradation == "" || binding.Mode == "degraded" && binding.Degradation != ""
		if !resources[identity] || bindings[identity] || binding.Projection == "" || binding.Name == "" || !validMode || binding.Sharing == "" {
			return errors.New("project lock bindings are incomplete, duplicated, or outside the locked closure")
		}
		bindings[identity] = true
	}
	degradations := make(map[ResourceIdentity]bool)
	for _, degradation := range lock.Degradations {
		if degradation.ResourceKind == "" {
			continue
		}
		identity, err := ParseResourceIdentity(degradation.ID)
		if err != nil || identity.Kind != degradation.ResourceKind || !resources[identity] || degradations[identity] || degradation.Surface != pack.Surfaces[0] || degradation.Mode != "optional" || degradation.Code == "" || degradation.Reason == "" || degradation.SourcePaths == nil {
			return errors.New("project lock declared degradations are incomplete, duplicated, or outside the locked closure")
		}
		degradations[identity] = true
	}
	for resource := range resources {
		if projectOperationalResource(resource.Kind) && !bindings[resource] && !degradations[resource] {
			return fmt.Errorf("project lock operational resource %s has no native binding or declared degradation", resource)
		}
	}
	modeIDs := make(map[string]bool, len(lock.Modes))
	for _, mode := range lock.Modes {
		if mode.ID == "" || modeIDs[mode.ID] || mode.Authorities == nil || mode.Fallback == "" {
			return errors.New("project lock runtime modes are incomplete or duplicated")
		}
		modeIDs[mode.ID] = true
	}
	for _, fact := range lock.ResourceGraph.Resources {
		for _, dependency := range append(append([]ResourceIdentity(nil), fact.Requires...), fact.Notices...) {
			if !resources[dependency] {
				return fmt.Errorf("project lock resource graph references missing dependency %s", dependency)
			}
		}
		for _, member := range fact.DependencyChain {
			if !resources[member] {
				return fmt.Errorf("project lock dependency chain references missing resource %s", member)
			}
		}
	}
	seenTargets := make(map[string]bool, len(lock.Projections))
	seenResources := make(map[ResourceIdentity]bool, len(lock.Projections))
	contributor := "surface:" + string(pack.Surfaces[0]) + ":pack:" + pack.ID
	for _, projection := range lock.Projections {
		validContributors := projection.Contributor == contributor
		if projection.Contributors != nil {
			validContributors = validContributors && len(projection.Contributors) > 0 && digestJSON(projection.Contributors) == digestJSON(sortedUnique(projection.Contributors))
			for _, value := range projection.Contributors {
				validContributors = validContributors && strings.HasPrefix(value, "surface:"+string(pack.Surfaces[0])+":pack:")
			}
		}
		if projection.Resource.Kind == "" || projection.Resource.ID == "" || !safeProjectContractTarget(projection.Target) || seenTargets[projection.Target] || seenResources[projection.Resource] || !validContributors || projection.ObservedState != "installed" || !projectDigestPattern.MatchString(projection.DesiredFingerprint) {
			return errors.New("project lock contains malformed, duplicate, or unauthorized projection evidence")
		}
		if projection.Mode != "copy_tree" && projection.Mode != "merge_marked_file" {
			return fmt.Errorf("project lock projection %s has unsupported mode %q", projection.Resource, projection.Mode)
		}
		if projection.Mode == "copy_tree" && (!resources[projection.Resource] || !bindings[projection.Resource]) {
			return fmt.Errorf("project lock projection %s is outside the locked resource graph or bindings", projection.Resource)
		}
		if projection.FileMode == 0 || projection.FileMode&^0o777 != 0 {
			return fmt.Errorf("project lock projection %s has unsupported file mode", projection.Resource)
		}
		seenTargets[projection.Target], seenResources[projection.Resource] = true, true
	}
	for resource := range bindings {
		if !seenResources[resource] {
			return fmt.Errorf("project lock native-bound operational resource %s has no projection plan", resource)
		}
	}
	return nil
}

func projectGraphContains(graph ResourceGraph, target ResourceIdentity) bool {
	for _, fact := range graph.Resources {
		if fact.Resource == target {
			return true
		}
	}
	return false
}

func projectOperationalResource(kind string) bool {
	switch kind {
	case "skill", "instruction", "mcp_server", "agent", "command", "lifecycle":
		return true
	default:
		return false
	}
}

func validProjectResourceRole(role ResourceRole) bool {
	switch role {
	case ResourceRoleRoot, ResourceRoleDependency, ResourceRoleAsset, ResourceRoleNotice:
		return true
	default:
		return false
	}
}

func safeProjectContractTarget(target string) bool {
	return target != "" && target != "." && !filepath.IsAbs(target) && !strings.Contains(target, "\\") && target != ".." && !strings.HasPrefix(target, "../") && filepath.ToSlash(filepath.Clean(filepath.FromSlash(target))) == target
}

func inspectOfflineProjectFiles(projectRoot string, installation ProjectInstallation, manifestBytes []byte) (bool, []ProjectInstallBlocker, error) {
	healthy := true
	var blockers []ProjectInstallBlocker
	manifestPath := filepath.Join(projectRoot, "packy.json")
	if info, err := os.Lstat(manifestPath); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o644 {
		healthy = false
		blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Target: "packy.json", Detail: "the project manifest is missing, unsafe, or has the wrong mode", Remediation: "restore the project manifest before reconciliation"})
	}
	lockPath := filepath.Join(projectRoot, "packy.lock.json")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		return false, nil, err
	}
	canonicalLock, err := marshalProjectLock(installation.Lock)
	if err != nil {
		return false, nil, err
	}
	if info, statErr := os.Lstat(lockPath); statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o644 || string(lockData) != string(canonicalLock) {
		healthy = false
		blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Target: "packy.lock.json", Detail: "the generated project lock bytes or mode have changed", Remediation: "run `packy pack install` to regenerate the exact lock"})
	}
	_ = manifestBytes
	noticesPath := filepath.Join(projectRoot, "PACKY-NOTICES.md")
	noticesInfo, statErr := os.Lstat(noticesPath)
	if errors.Is(statErr, fs.ErrNotExist) {
		healthy = false
		blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notices are missing", Remediation: "restore the exact locked notice contribution"})
	} else if statErr != nil {
		return false, nil, statErr
	} else if !noticesInfo.Mode().IsRegular() {
		healthy = false
		blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notices target is unsafe", Remediation: "restore the exact locked notice contribution"})
	} else {
		noticesData, readErr := os.ReadFile(noticesPath)
		if readErr != nil {
			return false, nil, readErr
		}
		start, end := projectNoticeMarkers(installation.Manifest.Packs[0].ID)
		fragment, found := extractProjectContribution(string(noticesData), start, end)
		if !found || uint32(noticesInfo.Mode().Perm()) != installation.Lock.NoticesFileMode || fingerprintProjectBytes([]byte(fragment)) != installation.Lock.NoticesSHA256 {
			healthy = false
			blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notice contribution or mode differs from the lock", Remediation: "restore the exact locked notice contribution"})
		}
	}
	return healthy, blockers, nil
}

func (r JSONProjectStatusReport) MarshalJSON() ([]byte, error) {
	type report JSONProjectStatusReport
	packs := r.Packs
	if packs == nil {
		packs = []JSONProjectPackStatus{}
	}
	return json.Marshal(report{SchemaVersion: r.SchemaVersion, Report: r.Report, ProjectRoot: r.ProjectRoot, Packs: packs})
}
