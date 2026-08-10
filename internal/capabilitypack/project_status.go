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

const ProjectStatusSchemaVersion = 5

type ProjectInstallationState string

const (
	ProjectInstallationAbsent    ProjectInstallationState = "absent"
	ProjectInstallationInstalled ProjectInstallationState = "installed"
	ProjectInstallationDrifted   ProjectInstallationState = "drifted"
	ProjectInstallationBlocked   ProjectInstallationState = "blocked"
)

// ProjectUpdateAvailable reports whether an intact installed project Pack is
// older than the reviewed catalog Pack. Blocked or drifted installations must
// be repaired before update can be offered as an applicable action.
func ProjectUpdateAvailable(installation ProjectInstallationState, installedVersion, catalogVersion string) bool {
	return installation == ProjectInstallationInstalled && installedVersion != "" && installedVersion != catalogVersion
}

type ProjectRuntimeState string

const (
	ProjectRuntimeNotRequired     ProjectRuntimeState = "not-required"
	ProjectRuntimePending         ProjectRuntimeState = "pending"
	ProjectRuntimeActive          ProjectRuntimeState = "active"
	ProjectRuntimeInheritedGlobal ProjectRuntimeState = "inherited-global"
	ProjectRuntimeStale           ProjectRuntimeState = "stale"
	ProjectRuntimeOrphaned        ProjectRuntimeState = "orphaned"
	ProjectRuntimeBlocked         ProjectRuntimeState = "blocked"
)

type ProjectRuntimeCoverage string

const (
	ProjectRuntimeCoveragePending          ProjectRuntimeCoverage = "pending"
	ProjectRuntimeCoverageProject          ProjectRuntimeCoverage = "project"
	ProjectRuntimeCoverageInheritedGlobal  ProjectRuntimeCoverage = "inherited-global"
	ProjectRuntimeCoverageGlobalAndProject ProjectRuntimeCoverage = "global-and-project"
	ProjectRuntimeCoverageConflict         ProjectRuntimeCoverage = "conflict"
)

type ProjectRuntimeEffectStatus struct {
	Category      ProjectActivationCategory `json:"category"`
	Resource      ResourceIdentity          `json:"resource"`
	Detail        string                    `json:"detail"`
	Coverage      ProjectRuntimeCoverage    `json:"coverage"`
	GlobalVersion string                    `json:"global_version,omitempty"`
	Conflict      string                    `json:"conflict,omitempty"`
}

type ProjectProjectionStatus struct {
	Resource            ResourceIdentity `json:"resource"`
	Target              string           `json:"target"`
	Mode                string           `json:"mode"`
	Health              string           `json:"health"`
	ObservedFingerprint string           `json:"observed_fingerprint"`
	DesiredFingerprint  string           `json:"desired_fingerprint"`
	OwnerPack           string           `json:"owner_pack"`
	Surface             Surface          `json:"surface"`
}

type JSONProjectPackStatus struct {
	Pack                           ProjectManifestPack          `json:"pack"`
	Surface                        Surface                      `json:"surface"`
	Installation                   ProjectInstallationState     `json:"installation"`
	Runtime                        ProjectRuntimeState          `json:"runtime"`
	RuntimeRequired                bool                         `json:"runtime_required"`
	RuntimeEffects                 []ProjectRuntimeEffectStatus `json:"runtime_effects"`
	Readiness                      ReadinessStatus              `json:"readiness"`
	Conditions                     []ReadinessCondition         `json:"conditions"`
	Projections                    []ProjectProjectionStatus    `json:"projections"`
	Blockers                       []ProjectInstallBlocker      `json:"blockers"`
	PendingHumanActions            []string                     `json:"pending_human_actions"`
	Evidence                       []string                     `json:"evidence"`
	ControlledCheck                ControlledCheckStatus        `json:"controlled_check"`
	ControlledCheckActionAvailable bool                         `json:"-"`
	Requirement                    string                       `json:"requirement,omitempty"`
	RequirementSatisfied           bool                         `json:"requirement_satisfied"`
	readinessObservation           ReadinessObservation
	readinessRevision              string
	controlledCheck                ControlledCheckDescriptor
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
	RequireUsable    bool
	PackyHome        string
	Adapters         map[Surface]SurfaceAdapter
	Resolver         ExecutableResolver
}

type ProjectInstallation struct {
	Manifest ProjectContractProposal
	Lock     ProjectLockProposal
}

func projectInstallationForPack(installation ProjectInstallation, packID string) ProjectInstallation {
	pack, found := findProjectManifestPack(installation.Manifest.Packs, packID)
	if !found {
		return ProjectInstallation{}
	}
	installation.Manifest.Packs = []ProjectManifestPack{pack}
	installation.Lock = projectLockForPack(installation.Lock, packID)
	return installation
}

func projectLockForPack(lock ProjectLockProposal, packID string) ProjectLockProposal {
	resources := map[ResourceIdentity]bool{}
	receipts := make([]installedPackReceipt, 0)
	for _, receipt := range lock.Receipts {
		if receipt.Pack.ID == packID {
			receipts = append(receipts, receipt)
			for _, resource := range receipt.Resources {
				resources[resource] = true
			}
		}
	}
	lock.Receipts = receipts
	facts := make([]ResourceClosureFact, 0, len(resources))
	for resource := range resources {
		facts = append(facts, ResourceClosureFact{Resource: resource, Role: ResourceRoleRoot, DependencyChain: []ResourceIdentity{resource}, Requires: []ResourceIdentity{}, Notices: []ResourceIdentity{}})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].Resource.String() < facts[j].Resource.String() })
	lock.ResourceGraph = ResourceGraph{Resources: facts}
	filterResource := func(identity ResourceIdentity) bool { return resources[identity] }
	bindings := make([]LifecycleBinding, 0, len(lock.Bindings))
	for _, binding := range lock.Bindings {
		if filterResource(ResourceIdentity{Kind: binding.Kind, ID: binding.ID}) {
			bindings = append(bindings, binding)
		}
	}
	lock.Bindings = bindings
	degradations := make([]LifecycleExclusion, 0, len(lock.Degradations))
	for _, degradation := range lock.Degradations {
		identity, err := ParseResourceIdentity(degradation.ID)
		if degradation.ResourceKind == "" || err == nil && filterResource(identity) {
			degradations = append(degradations, degradation)
		}
	}
	lock.Degradations = degradations
	sensitive := make([]ProjectSensitiveDisclosure, 0, len(lock.Sensitive))
	for _, disclosure := range lock.Sensitive {
		if filterResource(disclosure.Resource) || disclosure.Resource == (ResourceIdentity{Kind: "pack", ID: packID}) {
			sensitive = append(sensitive, disclosure)
		}
	}
	lock.Sensitive = sensitive
	projections := make([]ProjectProjectionPlan, 0, len(lock.Projections))
	for _, projection := range lock.Projections {
		if projectProjectionOwnedByPack(projection, packID) {
			projections = append(projections, projection)
		}
	}
	lock.Projections = projections
	return lock
}

func projectProjectionOwnedByPack(projection ProjectProjectionPlan, packID string) bool {
	return projection.OwnerPack == packID
}

type projectManifestDocument struct {
	SchemaVersion int                           `json:"schema_version"`
	Packs         []projectManifestPackDocument `json:"packs"`
}

type projectManifestPackDocument struct {
	ID             string                 `json:"id"`
	SurfaceIntents []ProjectSurfaceIntent `json:"surface_intents"`
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
	if !supportedProjectContractVersion(document.SchemaVersion, "") {
		return ProjectInstallation{}, errors.New("project manifest schema or minimum Packy capability is unsupported")
	}
	packs := make([]ProjectManifestPack, 0, len(document.Packs))
	for _, persisted := range document.Packs {
		packs = append(packs, deriveProjectManifestPack(ProjectManifestPack{ID: persisted.ID, SurfaceIntents: persisted.SurfaceIntents}))
	}
	manifest := ProjectContractProposal{Path: "packy.json", SchemaVersion: document.SchemaVersion, Packs: packs}
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
			requirement, requirementSatisfied := "", true
			if request.RequireInstalled {
				requirement, requirementSatisfied = "installed", false
			}
			if request.RequireUsable {
				requirement, requirementSatisfied = "usable", false
			}
			runtime := ProjectRuntimePending
			version := ""
			pendingActions := []string{}
			effects := []ProjectRuntimeEffectStatus{}
			blockers := []ProjectInstallBlocker{}
			if request.PackyHome != "" {
				document, exists, loadErr := loadProjectActivationDocumentForSurface(request.PackyHome, request.ProjectRoot, request.PackID, request.Surface)
				if loadErr != nil {
					return report, loadErr
				}
				if exists {
					if document.State.PackID != request.PackID {
						return report, fmt.Errorf("personal project activation belongs to capability pack %q, not %q", document.State.PackID, request.PackID)
					}
					version = document.State.Version
					runtime = ProjectRuntimeOrphaned
					for _, receipt := range document.Receipts {
						for _, detail := range receipt.Details {
							effects = append(effects, ProjectRuntimeEffectStatus{Category: receipt.Category, Resource: detail.Resource, Detail: detail.Detail, Coverage: ProjectRuntimeCoverageProject})
						}
					}
					adapter := request.Adapters[request.Surface]
					if adapter == nil && len(document.Effects) > 0 {
						return report, fmt.Errorf("project activation inspection does not support CLI surface %q", request.Surface)
					}
					observed, inspectErr := inspectProjectEffectReceipts(ctx, adapter, request.ProjectRoot, document.Effects)
					if inspectErr != nil {
						return report, inspectErr
					}
					for _, effect := range observed {
						if effect.State == ProjectEffectDrifted {
							runtime = ProjectRuntimeBlocked
							blockers = append(blockers, ProjectInstallBlocker{Code: "personal_effect_drift", Target: "<personal-host-path>", Detail: "the receipted personal contribution differs from exact adapter evidence", Remediation: "restore the exact receipted contribution before retrying project deactivation"})
						}
					}
					pendingActions = append(pendingActions, fmt.Sprintf("packy deactivate %s --surface %s --project", request.PackID, request.Surface))
				}
			}
			readiness, conditions := evaluateReadiness(readinessEvaluation{
				Pack:    Pack{ID: request.PackID, Version: version, ReadinessObligations: []ReadinessObligation{}},
				Surface: request.Surface, Scope: ReadinessScopeProject, Revision: "project-installation-absent",
			})
			report.Packs = append(report.Packs, JSONProjectPackStatus{
				Pack:    ProjectManifestPack{ID: request.PackID, Version: version, Surfaces: []Surface{request.Surface}, Selection: ResourceSelection{Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}},
				Surface: request.Surface, Installation: ProjectInstallationAbsent, Runtime: runtime, RuntimeRequired: len(effects) > 0,
				RuntimeEffects: effects, Readiness: readiness, Conditions: conditions, Projections: []ProjectProjectionStatus{}, Blockers: blockers, PendingHumanActions: sortedUnique(pendingActions), Evidence: []string{}, Requirement: requirement, RequirementSatisfied: requirementSatisfied,
				readinessRevision: "project-installation-absent",
			})
		} else if request.PackyHome != "" {
			orphaned, orphanErr := inspectOrphanedProjectActivations(ctx, request)
			if orphanErr != nil {
				return report, orphanErr
			}
			report.Packs = append(report.Packs, orphaned...)
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
	actualManifest, readErr := os.ReadFile(filepath.Join(request.ProjectRoot, "packy.json"))
	manifestHealthy := readErr == nil && string(actualManifest) == string(manifestBytes)
	contractHealthy, contractBlockers, err := inspectOfflineProjectFiles(request.ProjectRoot, installation)
	if err != nil {
		return report, err
	}
	for _, pack := range installation.Manifest.Packs {
		packLock := projectLockForPack(installation.Lock, pack.ID)
		for _, surface := range pack.Surfaces {
			surfacePack := pack
			for _, intent := range projectSurfaceIntents(pack) {
				if intent.Surface == surface {
					surfacePack.Version = intent.Version
					break
				}
			}
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
			scopedInstallation := projectInstallationForPack(installation, pack.ID)
			readinessPack := projectReadinessPack(packLock, surface, surfacePack)
			resolutions, unobservedRequirements := observeProjectRequirements(ctx, readinessPack.Requires.Tools, request.Resolver)
			observation, inspectErr := inspectSurface(ctx, adapter, SurfaceTransition{
				ProjectRoot: request.ProjectRoot, ProjectInstallation: &scopedInstallation, ProjectGoal: ProjectionPresent, ResolvedExecutables: resolutions,
			})
			if inspectErr != nil {
				return report, inspectErr
			}
			projections, inspectErr := projectProjectionStatusesFromObservation(request.ProjectRoot, installation.Lock, observation, surface, pack.ID)
			if inspectErr != nil {
				return report, inspectErr
			}
			blockers := append([]ProjectInstallBlocker{}, contractBlockers...)
			for _, resolution := range resolutions {
				if !resolution.Available {
					blockers = append(blockers, ProjectInstallBlocker{Code: "external_requirement_missing", Detail: "required executable " + resolution.Tool + " is missing", Remediation: "install the required executable and rerun project status"})
				}
			}
			noticeHealthy, noticeBlockers, inspectErr := inspectProjectNoticeFile(request.ProjectRoot, installation, pack.ID, surface)
			if inspectErr != nil {
				return report, inspectErr
			}
			blockers = append(blockers, noticeBlockers...)
			state := ProjectInstallationInstalled
			if !manifestHealthy {
				state = ProjectInstallationBlocked
				blockers = append(blockers, ProjectInstallBlocker{Code: "manifest_lock_mismatch", Target: "packy.json", Detail: "the human manifest does not match the exact generated lock", Remediation: "run the named Pack install again to refresh its receipt"})
			} else if !contractHealthy || !noticeHealthy {
				state = ProjectInstallationDrifted
			}
			for _, projection := range projections {
				if projection.Health != "verified" && state == ProjectInstallationInstalled {
					state = ProjectInstallationDrifted
				}
				if projection.Health != "verified" {
					blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Resource: projection.Resource, Target: projection.Target, Detail: "the installed project projection is " + projection.Health, Remediation: "restore the exact locked projection with the named Pack install"})
				}
			}
			sort.Slice(blockers, func(i, j int) bool {
				if blockers[i].Code != blockers[j].Code {
					return blockers[i].Code < blockers[j].Code
				}
				return blockers[i].Target < blockers[j].Target
			})
			categories := projectActivationCategories(packLock, surface)
			runtimeRequired := len(categories) != 0
			controlledCheck := ControlledCheckStatus{State: ControlledCheckUnknown}
			if request.PackyHome != "" && state == ProjectInstallationInstalled {
				projectDigest, digestErr := projectActivationRootDigest(request.ProjectRoot)
				if digestErr != nil {
					return report, digestErr
				}
				identity := controlledCheckIdentityFor(readinessPack, surface, ControlledCheckProject, projectDigest, controlledCheckResources(projectLockForPack(installation.Lock, pack.ID).ResourceGraph), observation, normalizedControlledCheckDescriptor(surface, observation.ControlledCheck))
				controlledCheck, inspectErr = NewFileControlledCheckStore(request.PackyHome).Status(ctx, identity)
				if inspectErr != nil {
					return report, inspectErr
				}
			}
			readiness, conditions := evaluateReadiness(readinessEvaluation{
				Pack: readinessPack, Surface: surface, Scope: ReadinessScopeProject,
				Projections: projectReadinessProjections(projections, state), Resolutions: resolutions, UnobservedRequirements: unobservedRequirements, Observation: observation.Readiness, Revision: observation.Revision,
				ControlledCheck: &controlledCheck,
			})
			runtime := ProjectRuntimeNotRequired
			effects := pendingProjectRuntimeEffects(categories)
			if runtimeRequired {
				runtime, effects = projectPersonalRuntimeStatus(ctx, adapter, request.PackyHome, request.ProjectRoot, surfacePack, surface, state, packLock, categories)
			}
			status := JSONProjectPackStatus{Pack: surfacePack, Surface: surface, Installation: state, Runtime: runtime, RuntimeRequired: runtimeRequired, RuntimeEffects: effects, Readiness: readiness, Conditions: conditions, Projections: projections, Blockers: blockers, PendingHumanActions: append([]string{}, observation.Readiness.PendingHumanActions...), Evidence: append([]string{}, observation.Readiness.Evidence...), ControlledCheck: controlledCheck, ControlledCheckActionAvailable: controlledCheckActionAvailable(readiness, conditions), RequirementSatisfied: true, readinessObservation: observation.Readiness, readinessRevision: observation.Revision, controlledCheck: observation.ControlledCheck}
			switch runtime {
			case ProjectRuntimePending, ProjectRuntimeStale:
				if runtimeRequired {
					status.PendingHumanActions = append(status.PendingHumanActions, fmt.Sprintf("packy activate %s --surface %s --project", pack.ID, surface))
				}
			case ProjectRuntimeBlocked:
				status.PendingHumanActions = append(status.PendingHumanActions, fmt.Sprintf("packy deactivate %s --surface %s --project", pack.ID, surface))
			}
			if state != ProjectInstallationInstalled {
				status.PendingHumanActions = append(status.PendingHumanActions, "packy install "+pack.ID+" --surface "+string(surface))
			}
			status.PendingHumanActions = sortedUnique(status.PendingHumanActions)
			if request.RequireInstalled {
				status.Requirement = "installed"
				status.RequirementSatisfied = state == ProjectInstallationInstalled
			}
			if request.RequireUsable {
				status.Requirement = "usable"
				status.RequirementSatisfied = state == ProjectInstallationInstalled && readiness.SatisfiesUsable() && (!runtimeRequired || runtime == ProjectRuntimeActive)
			}
			report.Packs = append(report.Packs, status)
		}
	}
	if len(report.Packs) == 0 {
		return report, fmt.Errorf("pack %q on %s is not declared by this project installation", request.PackID, request.Surface)
	}
	return report, nil
}

func projectReadinessPack(lock ProjectLockProposal, surface Surface, installed ProjectManifestPack) Pack {
	for _, receipt := range lock.Receipts {
		if receipt.Surface == surface {
			return Pack{ID: installed.ID, Version: installed.Version, ReadinessObligations: append([]ReadinessObligation(nil), receipt.ReadinessObligations...), Requires: Requirements{Tools: append([]string(nil), receipt.ExternalRequirements...)}}
		}
	}
	return Pack{ID: installed.ID, Version: installed.Version, ReadinessObligations: []ReadinessObligation{}, Requires: Requirements{Tools: []string{}}}
}

func observeProjectRequirements(ctx context.Context, requirements []string, resolver ExecutableResolver) ([]ExecutableResolution, []string) {
	resolutions := make([]ExecutableResolution, 0, len(requirements))
	unobserved := make([]string, 0)
	for _, tool := range requirements {
		if resolver == nil {
			unobserved = append(unobserved, tool)
			continue
		}
		resolution, err := resolver.Resolve(ctx, tool)
		if err != nil {
			unobserved = append(unobserved, tool)
			continue
		}
		resolution.Tool = tool
		if resolution.Precondition == "" {
			resolution.Precondition = resolutionFingerprint(resolution)
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, unobserved
}

func inspectOrphanedProjectActivations(ctx context.Context, request ProjectStatusRequest) ([]JSONProjectPackStatus, error) {
	directory, err := projectActivationDirectory(request.PackyHome, request.ProjectRoot)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return []JSONProjectPackStatus{}, nil
	}
	if err != nil {
		return nil, err
	}
	statuses := make([]JSONProjectPackStatus, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "state-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var candidate projectActivationDocument
		if strictDecode(data, &candidate) != nil {
			continue
		}
		surface := candidate.State.Surface
		if surface != SurfaceCodex && surface != SurfaceOpenCode && surface != SurfaceClaude {
			continue
		}
		document, exists, loadErr := loadProjectActivationDocumentForSurface(request.PackyHome, request.ProjectRoot, candidate.State.PackID, surface)
		if loadErr != nil {
			return nil, loadErr
		}
		if !exists {
			continue
		}
		focused := request
		focused.PackID, focused.Surface = document.State.PackID, surface
		focusedReport, inspectErr := InspectProjectStatus(ctx, focused)
		if inspectErr != nil {
			return nil, inspectErr
		}
		statuses = append(statuses, focusedReport.Packs...)
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Pack.ID != statuses[j].Pack.ID {
			return statuses[i].Pack.ID < statuses[j].Pack.ID
		}
		return statuses[i].Surface < statuses[j].Surface
	})
	return statuses, nil
}

func (f Facade) InspectProjectStatus(ctx context.Context, request ProjectStatusRequest) (JSONProjectStatusReport, error) {
	report, err := InspectProjectStatus(ctx, request)
	if err != nil {
		return report, err
	}
	installation, installationErr := LoadProjectInstallation(request.ProjectRoot)
	catalogAvailable := make(map[string]bool, len(report.Packs))
	for i := range report.Packs {
		status := &report.Packs[i]
		var pack Pack
		if status.Pack.Version != "" {
			pack, err = f.catalog.resolveIntentPack(status.Pack.ID, status.Pack.Version)
		} else {
			pack, err = f.catalog.Show(status.Pack.ID)
		}
		if err != nil {
			// The installed receipt remains authoritative when the reviewed
			// catalog is unavailable. Project status must stay inspectable
			// offline; catalog-backed requirement evidence is additive.
			continue
		}
		catalogAvailable[status.Pack.ID+"\x00"+string(status.Surface)] = true
		if store := f.controlledCheckStore(request.PackyHome); store != nil && installationErr == nil && status.Installation == ProjectInstallationInstalled {
			digest, digestErr := projectActivationRootDigest(request.ProjectRoot)
			if digestErr != nil {
				return report, digestErr
			}
			observation := SurfaceInspection{Revision: status.readinessRevision, ControlledCheck: status.controlledCheck}
			identity := controlledCheckIdentityFor(pack, status.Surface, ControlledCheckProject, digest, controlledCheckResources(projectLockForPack(installation.Lock, status.Pack.ID).ResourceGraph), observation, normalizedControlledCheckDescriptor(status.Surface, status.controlledCheck))
			status.ControlledCheck, err = store.Status(ctx, identity)
			if err != nil {
				return report, err
			}
		} else {
			status.ControlledCheck = ControlledCheckStatus{State: ControlledCheckUnknown}
		}
		resolutions, resolveErr := f.resolveExecutables(ctx, pack, status.Surface, false)
		unobservedRequirements := []string{}
		if resolveErr != nil {
			status.Blockers = append(status.Blockers, ProjectInstallBlocker{
				Code: "external_requirement_unobservable", Detail: resolveErr.Error(),
				Remediation: "configure executable inspection and rerun project status",
			})
			unobservedRequirements = append(unobservedRequirements, pack.Requires.Tools...)
			resolutions = nil
		}
		status.Readiness, status.Conditions = evaluateReadiness(readinessEvaluation{
			Pack: pack, Surface: status.Surface, Scope: ReadinessScopeProject,
			Projections: projectReadinessProjections(status.Projections, status.Installation), Resolutions: resolutions, UnobservedRequirements: unobservedRequirements,
			Observation: status.readinessObservation, Revision: status.readinessRevision, ControlledCheck: &status.ControlledCheck,
		})
		status.ControlledCheckActionAvailable = controlledCheckActionAvailable(status.Readiness, status.Conditions)
		if request.RequireUsable {
			status.RequirementSatisfied = status.Installation == ProjectInstallationInstalled && status.Readiness.SatisfiesUsable() && (!status.RuntimeRequired || status.Runtime == ProjectRuntimeActive || status.Runtime == ProjectRuntimeInheritedGlobal)
		}
	}
	if f.activation == nil || f.activation.store == nil {
		return report, nil
	}
	hasInstalledRuntime := false
	for _, status := range report.Packs {
		hasInstalledRuntime = hasInstalledRuntime || status.Installation != ProjectInstallationAbsent && status.RuntimeRequired
	}
	if !hasInstalledRuntime {
		return report, nil
	}
	installation, err = LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return report, err
	}
	for i := range report.Packs {
		status := &report.Packs[i]
		if !status.RuntimeRequired || status.Installation != ProjectInstallationInstalled {
			continue
		}
		if !catalogAvailable[status.Pack.ID+"\x00"+string(status.Surface)] {
			continue
		}
		composition, composeErr := f.composeProjectRuntime(ctx, projectInstallationForPack(installation, status.Pack.ID), status.Surface)
		if composeErr != nil {
			return report, composeErr
		}
		personalRuntime := status.Runtime
		personalCoverage := make(map[string]ProjectRuntimeCoverage, len(status.RuntimeEffects))
		for _, effect := range status.RuntimeEffects {
			personalCoverage[projectRuntimeDisclosureKey(effect.Category, effect.Resource, effect.Detail)] = effect.Coverage
		}
		status.RuntimeEffects = composition.effects
		for j := range status.RuntimeEffects {
			effect := &status.RuntimeEffects[j]
			coverage := personalCoverage[projectRuntimeDisclosureKey(effect.Category, effect.Resource, effect.Detail)]
			if effect.Coverage == ProjectRuntimeCoverageInheritedGlobal && coverage == ProjectRuntimeCoverageProject {
				effect.Coverage = ProjectRuntimeCoverageGlobalAndProject
			} else if effect.Coverage == ProjectRuntimeCoveragePending && coverage == ProjectRuntimeCoverageProject {
				effect.Coverage = ProjectRuntimeCoverageProject
			}
		}
		status.Runtime = personalRuntime
		if composition.conflict {
			status.Runtime = ProjectRuntimeBlocked
			status.Blockers = append(status.Blockers, ProjectInstallBlocker{Code: "activation_scope_conflict", Detail: composition.conflictDetail, Remediation: "deactivate the incompatible global contribution or align the global and project contracts"})
		} else if personalRuntime != ProjectRuntimeBlocked && personalRuntime != ProjectRuntimeStale {
			status.Runtime = composedProjectRuntimeState(status.RuntimeEffects)
		}
		activateCommand := fmt.Sprintf("packy activate %s --surface %s --project", status.Pack.ID, status.Surface)
		deactivateCommand := fmt.Sprintf("packy deactivate %s --surface %s --project", status.Pack.ID, status.Surface)
		pendingActions := make([]string, 0, len(status.PendingHumanActions)+1)
		for _, action := range status.PendingHumanActions {
			if action != activateCommand && action != deactivateCommand {
				pendingActions = append(pendingActions, action)
			}
		}
		switch {
		case composition.conflict:
			pendingActions = append(pendingActions, fmt.Sprintf("packy deactivate %s --surface %s", status.Pack.ID, status.Surface))
		case personalRuntime == ProjectRuntimeBlocked:
			pendingActions = append(pendingActions, deactivateCommand)
		case status.Runtime == ProjectRuntimePending || status.Runtime == ProjectRuntimeStale:
			pendingActions = append(pendingActions, activateCommand)
		}
		status.PendingHumanActions = sortedUnique(pendingActions)
		if request.RequireUsable {
			status.RequirementSatisfied = status.Installation == ProjectInstallationInstalled && status.Readiness.SatisfiesUsable() && (status.Runtime == ProjectRuntimeActive || status.Runtime == ProjectRuntimeInheritedGlobal)
		}
	}
	return report, nil
}

func projectReadinessProjections(values []ProjectProjectionStatus, installation ProjectInstallationState) []ProjectionStatus {
	result := make([]ProjectionStatus, 0, len(values)+1)
	for _, value := range values {
		result = append(result, ProjectionStatus{
			ID: value.Resource.String(), Target: value.Target, Health: ProjectionHealth(value.Health),
			ObservedFingerprint: value.ObservedFingerprint, DesiredFingerprint: value.DesiredFingerprint,
		})
	}
	if installation != ProjectInstallationInstalled {
		health := ProjectionDrifted
		if installation == ProjectInstallationAbsent {
			health = ProjectionMissing
		}
		result = append(result, ProjectionStatus{ID: "project-installation", Health: health})
	}
	return result
}

func pendingProjectRuntimeEffects(categories []ProjectActivationCategoryPreview) []ProjectRuntimeEffectStatus {
	result := make([]ProjectRuntimeEffectStatus, 0)
	for _, category := range categories {
		for _, detail := range category.Details {
			result = append(result, ProjectRuntimeEffectStatus{Category: category.Kind, Resource: detail.Resource, Detail: detail.Detail, Coverage: ProjectRuntimeCoveragePending})
		}
	}
	return result
}

func projectProjectionStatusesFromObservation(projectRoot string, lock ProjectLockProposal, observation SurfaceInspection, surface Surface, packID string) ([]ProjectProjectionStatus, error) {
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
		if locked.OwnerPack != packID || locked.Surface != surface {
			continue
		}
		projection, ok := observed[locked.Resource]
		if !ok {
			return nil, fmt.Errorf("project adapter omitted locked projection %s (observed %d projections)", locked.Resource, len(observed))
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
			ObservedFingerprint: projection.ObservedFingerprint, DesiredFingerprint: locked.DesiredFingerprint, OwnerPack: locked.OwnerPack, Surface: locked.Surface,
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

func projectPersonalRuntimeStatus(ctx context.Context, adapter SurfaceAdapter, packyHome, projectRoot string, pack ProjectManifestPack, surface Surface, installation ProjectInstallationState, lock ProjectLockProposal, categories []ProjectActivationCategoryPreview) (ProjectRuntimeState, []ProjectRuntimeEffectStatus) {
	effects := pendingProjectRuntimeEffects(categories)
	if installation != ProjectInstallationInstalled {
		return ProjectRuntimeBlocked, effects
	}
	if packyHome == "" {
		return ProjectRuntimePending, effects
	}
	document, exists, err := loadProjectActivationDocumentForSurface(packyHome, projectRoot, pack.ID, surface)
	if err != nil {
		return ProjectRuntimeBlocked, effects
	}
	state := document.State
	if !exists || !state.Active {
		return ProjectRuntimePending, effects
	}
	observed, inspectErr := inspectProjectEffectReceipts(ctx, adapter, projectRoot, document.Effects)
	if inspectErr != nil {
		return ProjectRuntimeBlocked, effects
	}
	for _, effect := range observed {
		if effect.State == ProjectEffectDrifted {
			return ProjectRuntimeBlocked, effects
		}
		if effect.State == ProjectEffectAbsent {
			return ProjectRuntimeStale, effects
		}
	}
	if state.PackID != pack.ID || state.Version != pack.Version || state.Surface != surface || state.SensitiveLockIdentity != projectSensitiveLockIdentity(lock, categories) {
		return ProjectRuntimeStale, effects
	}
	approved := map[string]bool{}
	for _, receipt := range document.Receipts {
		for _, detail := range receipt.Details {
			approved[projectRuntimeDisclosureKey(receipt.Category, detail.Resource, detail.Detail)] = true
		}
	}
	matched := 0
	for i := range effects {
		key := projectRuntimeDisclosureKey(effects[i].Category, effects[i].Resource, effects[i].Detail)
		if approved[key] {
			effects[i].Coverage = ProjectRuntimeCoverageProject
			matched++
		}
	}
	if matched != len(approved) {
		return ProjectRuntimeStale, pendingProjectRuntimeEffects(categories)
	}
	if matched == len(effects) {
		return ProjectRuntimeActive, effects
	}
	return ProjectRuntimePending, effects
}

func composedProjectRuntimeState(effects []ProjectRuntimeEffectStatus) ProjectRuntimeState {
	hasProject := false
	for _, effect := range effects {
		switch effect.Coverage {
		case ProjectRuntimeCoveragePending:
			return ProjectRuntimePending
		case ProjectRuntimeCoverageProject, ProjectRuntimeCoverageGlobalAndProject:
			hasProject = true
		}
	}
	if hasProject {
		return ProjectRuntimeActive
	}
	return ProjectRuntimeInheritedGlobal
}

func projectRuntimePendingCategories(categories []ProjectActivationCategoryPreview, effects []ProjectRuntimeEffectStatus) []ProjectActivationCategoryPreview {
	pending := map[string]bool{}
	for _, effect := range effects {
		if effect.Coverage == ProjectRuntimeCoveragePending {
			pending[projectRuntimeDisclosureKey(effect.Category, effect.Resource, effect.Detail)] = true
		}
	}
	result := make([]ProjectActivationCategoryPreview, 0, len(categories))
	for _, category := range categories {
		filtered := ProjectActivationCategoryPreview{Kind: category.Kind, ApprovalRequired: category.ApprovalRequired}
		for _, detail := range category.Details {
			if pending[projectRuntimeDisclosureKey(category.Kind, detail.Resource, detail.Detail)] {
				filtered.Details = append(filtered.Details, detail)
			}
		}
		if len(filtered.Details) > 0 {
			result = append(result, filtered)
		}
	}
	return result
}

func projectPathMissing(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	return false, err
}

func validateProjectManifestPack(pack ProjectManifestPack) error {
	if !idPattern.MatchString(pack.ID) || len(pack.SurfaceIntents) == 0 {
		return errors.New("project manifest Pack must use a valid ID and surface intents")
	}
	if len(pack.Surfaces) == 0 || digestJSON(pack.Surfaces) != digestJSON(sortedProjectSurfaces(pack.Surfaces)) {
		return errors.New("project manifest surfaces are empty, duplicated, or unsorted")
	}
	for _, surface := range pack.Surfaces {
		if surface != SurfaceCodex && surface != SurfaceOpenCode && surface != SurfaceClaude {
			return fmt.Errorf("project manifest schema version 1 does not support CLI surface %q", surface)
		}
	}
	selection, err := canonicalSelection(pack.Selection)
	if err != nil || digestJSON(selection) != digestJSON(pack.Selection) || pack.Selection.Roots == nil || pack.Aliases == nil {
		return errors.New("project manifest selection and aliases are incomplete or non-canonical")
	}
	aliases := cloneAliases(pack.Aliases)
	if err := canonicalizeAliases(&aliases); err != nil || digestJSON(aliases) != digestJSON(pack.Aliases) {
		return errors.New("project manifest aliases are invalid or non-canonical")
	}
	if len(pack.SurfaceIntents) != len(pack.Surfaces) {
		return errors.New("project manifest surface intents do not exactly cover installed surfaces")
	}
	for i, intent := range pack.SurfaceIntents {
		canonical, selectionErr := canonicalSelection(intent.Selection)
		intentAliases := cloneAliases(intent.Aliases)
		aliasErr := canonicalizeAliases(&intentAliases)
		if intent.Surface != pack.Surfaces[i] || !semverPattern.MatchString(intent.Version) || intent.Selection.Roots == nil || intent.Aliases == nil || selectionErr != nil || aliasErr != nil || digestJSON(canonical) != digestJSON(intent.Selection) || digestJSON(intentAliases) != digestJSON(intent.Aliases) {
			return errors.New("project manifest surface intents are incomplete or non-canonical")
		}
	}
	derived := deriveProjectManifestPack(ProjectManifestPack{ID: pack.ID, SurfaceIntents: pack.SurfaceIntents})
	if derived.Version != pack.Version || digestJSON(derived.Surfaces) != digestJSON(pack.Surfaces) || digestJSON(derived.Selection) != digestJSON(pack.Selection) || digestJSON(derived.Aliases) != digestJSON(pack.Aliases) {
		return errors.New("project manifest aggregate view does not match surface intents")
	}
	return nil
}

func validateProjectInstallation(manifest ProjectContractProposal, lock ProjectLockProposal) error {
	if len(manifest.Packs) == 0 {
		return errors.New("project manifest must contain at least one direct Pack")
	}
	manifestPacks := make(map[string]ProjectManifestPack, len(manifest.Packs))
	for i, candidate := range manifest.Packs {
		if candidate.ID == "" || manifestPacks[candidate.ID].ID != "" || i > 0 && manifest.Packs[i-1].ID >= candidate.ID {
			return errors.New("project manifest Packs must be unique and sorted by ID")
		}
		if err := validateProjectManifestPack(candidate); err != nil {
			return err
		}
		manifestPacks[candidate.ID] = candidate
	}
	if !supportedProjectContractVersion(manifest.SchemaVersion, "") || lock.SchemaVersion != manifest.SchemaVersion || !supportedProjectContractVersion(lock.SchemaVersion, lock.MinimumPackyCapability) {
		return errors.New("project manifest and lock schema versions do not match")
	}
	if lock.Path != "packy.lock.json" {
		return errors.New("project lock path is invalid")
	}
	receiptSelections := map[string]ResourceSelection{}
	receiptSurfaces := map[string]map[Surface]bool{}
	for _, receipt := range lock.Receipts {
		direct, found := manifestPacks[receipt.Pack.ID]
		if !found || !projectSupportsSurface(direct.Surfaces, receipt.Surface) {
			return errors.New("project receipt identity does not match direct manifest intent")
		}
		intents := projectSurfaceIntents(direct)
		matchedIntent := false
		for _, intent := range intents {
			if intent.Surface != receipt.Surface {
				continue
			}
			if intent.Version != receipt.Pack.Version {
				return errors.New("project receipt version does not match manifest surface intent")
			}
			receiptAliases := cloneAliases(receipt.Aliases)
			_ = canonicalizeAliases(&receiptAliases)
			if digestJSON(intent.Selection) != digestJSON(receipt.Selection) || digestJSON(intent.Aliases) != digestJSON(receiptAliases) {
				return errors.New("project receipt selection or aliases do not match manifest intent")
			}
			matchedIntent = true
			break
		}
		if !matchedIntent {
			return errors.New("project receipt surface has no direct manifest intent")
		}
		if receiptSurfaces[receipt.Pack.ID] == nil {
			receiptSurfaces[receipt.Pack.ID] = map[Surface]bool{}
			receiptSelections[receipt.Pack.ID] = cloneSelection(receipt.Selection)
		} else {
			receiptSelections[receipt.Pack.ID] = mergeProjectReceiptSelections(receiptSelections[receipt.Pack.ID], receipt.Selection)
		}
		receiptSurfaces[receipt.Pack.ID][receipt.Surface] = true
	}
	for _, direct := range manifest.Packs {
		if len(receiptSurfaces[direct.ID]) != len(direct.Surfaces) || digestJSON(receiptSelections[direct.ID]) != digestJSON(direct.Selection) {
			return errors.New("project receipts do not exactly cover direct manifest intent")
		}
	}
	if lock.ResourceGraph.Resources == nil || lock.Projections == nil {
		return errors.New("project lock omits required receipt evidence")
	}
	resources := make(map[ResourceIdentity]bool, len(lock.ResourceGraph.Resources))
	for _, fact := range lock.ResourceGraph.Resources {
		if fact.Resource.Kind == "" || fact.Resource.ID == "" || resources[fact.Resource] {
			return errors.New("project lock resource graph is malformed or contains duplicate identities")
		}
		resources[fact.Resource] = true
	}
	seenTargets := make(map[string]bool, len(lock.Projections))
	sharedTargets := make(map[string]ProjectProjectionPlan, len(lock.Projections))
	validPackIDs := map[string]bool{}
	validPackSurfaces := map[string]map[Surface]bool{}
	for _, direct := range manifest.Packs {
		validPackIDs[direct.ID] = true
		validPackSurfaces[direct.ID] = map[Surface]bool{}
		for _, surface := range direct.Surfaces {
			validPackSurfaces[direct.ID][surface] = true
		}
	}
	for _, projection := range lock.Projections {
		if projection.Mode != "copy_tree" && projection.Mode != "copy_file" && projection.Mode != "merge_marked_file" && projection.Mode != "merge_structured_file" {
			return fmt.Errorf("project lock projection %s has unsupported mode %q", projection.Resource, projection.Mode)
		}
		physicalTargetKey := projection.Target
		targetKey := string(projection.Surface) + "\x00" + projection.Target
		if projection.Mode == "merge_marked_file" || projection.Mode == "merge_structured_file" {
			physicalTargetKey = projection.Resource.String() + "\x00" + projection.Target
			targetKey = string(projection.Surface) + "\x00" + physicalTargetKey
		}
		if projection.Resource.Kind == "" || projection.Resource.ID == "" || !safeProjectContractTarget(projection.Target) || seenTargets[targetKey] || !validPackIDs[projection.OwnerPack] || !validPackSurfaces[projection.OwnerPack][projection.Surface] || projection.ObservedState != "installed" || !projectDigestPattern.MatchString(projection.DesiredFingerprint) {
			return errors.New("project lock contains malformed, duplicate, or unauthorized projection evidence")
		}
		if projection.Mode == "copy_tree" || projection.Mode == "copy_file" {
			if shared, found := sharedTargets[physicalTargetKey]; found && (shared.OwnerPack != projection.OwnerPack || shared.Resource != projection.Resource || shared.DesiredFingerprint != projection.DesiredFingerprint) {
				return errors.New("project lock contains incompatible shared projection evidence")
			}
			sharedTargets[physicalTargetKey] = projection
		}
		if (projection.Mode == "copy_tree" || projection.Mode == "copy_file" || projection.Mode == "merge_structured_file") && !resources[projection.Resource] {
			return fmt.Errorf("project lock projection %s is outside the receipt resource closure", projection.Resource)
		}
		if projection.FileMode == 0 || projection.FileMode&^0o777 != 0 {
			return fmt.Errorf("project lock projection %s has unsupported file mode", projection.Resource)
		}
		seenTargets[targetKey] = true
	}
	return nil
}

func validProjectActivationCategory(category ProjectActivationCategory) bool {
	switch category {
	case ProjectActivationMCP, ProjectActivationHooks, ProjectActivationPlugins, ProjectActivationExternalRequirements, ProjectActivationTrust, ProjectActivationAuthentication:
		return true
	default:
		return false
	}
}

func safeProjectContractTarget(target string) bool {
	return target != "" && target != "." && !filepath.IsAbs(target) && !strings.Contains(target, "\\") && target != ".." && !strings.HasPrefix(target, "../") && filepath.ToSlash(filepath.Clean(filepath.FromSlash(target))) == target
}

func inspectOfflineProjectFiles(projectRoot string, installation ProjectInstallation) (bool, []ProjectInstallBlocker, error) {
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
		blockers = append(blockers, ProjectInstallBlocker{Code: "project_drift", Target: "packy.lock.json", Detail: "the generated project lock bytes or mode have changed", Remediation: "rerun the named Pack install to regenerate its exact receipt"})
	}
	return healthy, blockers, nil
}

func inspectProjectNoticeFile(projectRoot string, installation ProjectInstallation, packID string, surface Surface) (bool, []ProjectInstallBlocker, error) {
	noticesPath := filepath.Join(projectRoot, "PACKY-NOTICES.md")
	noticesInfo, statErr := os.Lstat(noticesPath)
	if errors.Is(statErr, fs.ErrNotExist) {
		return false, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notices are missing", Remediation: "restore the exact locked notice contribution"}}, nil
	} else if statErr != nil {
		return false, nil, statErr
	} else if !noticesInfo.Mode().IsRegular() {
		return false, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notices target is unsafe", Remediation: "restore the exact locked notice contribution"}}, nil
	}
	noticesData, readErr := os.ReadFile(noticesPath)
	if readErr != nil {
		return false, nil, readErr
	}
	start, end := projectNoticeMarkers(packID, surface)
	fragment, found := extractProjectContribution(string(noticesData), start, end)
	locked, receiptFound := projectNoticeReceiptProjection(installation.Lock.Receipts, packID, surface)
	if !found || !receiptFound || fingerprintProjectBytes([]byte(fragment)) != locked.Digest {
		return false, []ProjectInstallBlocker{{Code: "project_drift", Target: "PACKY-NOTICES.md", Detail: "the mandatory project notice contribution or mode differs from the lock", Remediation: "restore the exact locked notice contribution"}}, nil
	}
	return true, nil, nil
}

func (r JSONProjectStatusReport) MarshalJSON() ([]byte, error) {
	type report JSONProjectStatusReport
	packs := r.Packs
	if packs == nil {
		packs = []JSONProjectPackStatus{}
	}
	return json.Marshal(report{
		SchemaVersion: r.SchemaVersion, Report: r.Report, ProjectRoot: r.ProjectRoot,
		Packs: packs,
	})
}
