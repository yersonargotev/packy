// Package capabilitypack owns capability-pack discovery and policy.
package capabilitypack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

const (
	manifestSchemaV1 = 1
	manifestSchemaV2 = 2
	manifestSchemaV3 = 3
	manifestSchemaV4 = 4
)

// SupportedSurfaces returns Packy's complete product-owned CLI surface set in
// stable display order. Pack manifests decide which members each Pack supports.
func SupportedSurfaces() []Surface {
	return []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}
}

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
)

type Surface string

const (
	SurfaceCodex    Surface = "codex"
	SurfaceOpenCode Surface = "opencode"
	SurfaceClaude   Surface = "claude"
)

type Requirements struct {
	Tools []string `json:"tools"`
}

// ReadinessObligation identifies a Pack-level condition whose satisfaction is
// required for the Pack to be ready on a selected surface.
type ReadinessObligation string

const (
	ReadinessRuntimeUsability     ReadinessObligation = "runtime-usability"
	ReadinessSurfaceAuthorization ReadinessObligation = "surface-authorization"
)

type Resource struct {
	Kind              string
	ID                string
	Source            string
	Command           string
	Args              []string
	Description       string
	Mode              string
	Tools             []string
	Permissions       []string
	Requires          []string
	Conflicts         []string
	RequiresTools     []string
	Notices           []string
	Bindings          []Binding
	SurfaceExclusions []SurfaceExclusion
	Arguments         CommandArguments
	License           string
	Attribution       string
	RuntimeModes      []RuntimeMode
}

type RuntimeModeRole string
type RuntimeRequirementKind string
type RuntimeAuthorityKind string
type RuntimeEffectKind string
type RuntimeScope string
type RuntimeFallbackKind string
type RuntimeUnavailablePolicy string

const (
	RuntimeModePrimary      RuntimeModeRole = "primary"
	RuntimeModeFallbackOnly RuntimeModeRole = "fallback_only"

	RuntimeRequirementTool           RuntimeRequirementKind = "tool"
	RuntimeRequirementAuthentication RuntimeRequirementKind = "authentication"
	RuntimeRequirementProjectLink    RuntimeRequirementKind = "project_link"
	RuntimeRequirementEntitlement    RuntimeRequirementKind = "entitlement"
	RuntimeRequirementServiceData    RuntimeRequirementKind = "service_data"

	RuntimeScopeConsumerProject   RuntimeScope = "consumer_project"
	RuntimeScopePackResource      RuntimeScope = "pack_resource"
	RuntimeScopeWorkstation       RuntimeScope = "workstation"
	RuntimeScopeLocalGit          RuntimeScope = "local_git"
	RuntimeScopeRemoteGit         RuntimeScope = "remote_git"
	RuntimeScopeVercelAccount     RuntimeScope = "vercel_account"
	RuntimeScopeVercelProject     RuntimeScope = "vercel_project"
	RuntimeScopeDeploymentPayload RuntimeScope = "deployment_payload"

	RuntimeFallbackNone RuntimeFallbackKind = "none"
	RuntimeFallbackMode RuntimeFallbackKind = "mode"

	RuntimeFailBeforeEffects RuntimeUnavailablePolicy = "fail_before_effects"
)

const (
	RuntimeAuthorityFilesystemRead          RuntimeAuthorityKind = "filesystem_read"
	RuntimeAuthorityFilesystemWrite         RuntimeAuthorityKind = "filesystem_write"
	RuntimeAuthorityProcessExecute          RuntimeAuthorityKind = "process_execute"
	RuntimeAuthorityNetwork                 RuntimeAuthorityKind = "network"
	RuntimeAuthorityEnvironmentInspect      RuntimeAuthorityKind = "environment_inspect"
	RuntimeAuthoritySecretUse               RuntimeAuthorityKind = "secret_use"
	RuntimeAuthorityPackageManagerExecute   RuntimeAuthorityKind = "package_manager_execute"
	RuntimeAuthorityGitInspect              RuntimeAuthorityKind = "git_inspect"
	RuntimeAuthorityGitCommit               RuntimeAuthorityKind = "git_commit"
	RuntimeAuthorityGitPush                 RuntimeAuthorityKind = "git_push"
	RuntimeAuthorityVercelProjectMutate     RuntimeAuthorityKind = "vercel_project_mutate"
	RuntimeAuthorityVercelEnvironmentMutate RuntimeAuthorityKind = "vercel_environment_mutate"
	RuntimeAuthorityVercelDomainMutate      RuntimeAuthorityKind = "vercel_domain_mutate"
	RuntimeAuthorityPreviewDeploy           RuntimeAuthorityKind = "preview_deploy"
	RuntimeAuthorityProductionDeploy        RuntimeAuthorityKind = "production_deploy"
	RuntimeAuthorityUpload                  RuntimeAuthorityKind = "upload"
	RuntimeAuthoritySubagentDelegate        RuntimeAuthorityKind = "subagent_delegate"
)

const (
	RuntimeEffectAuthenticationStateChange       RuntimeEffectKind = "authentication_state_change"
	RuntimeEffectConsumerProjectFileChange       RuntimeEffectKind = "consumer_project_file_change"
	RuntimeEffectConsumerProjectDependencyChange RuntimeEffectKind = "consumer_project_dependency_change"
	RuntimeEffectLocalGitChange                  RuntimeEffectKind = "local_git_change"
	RuntimeEffectRemoteGitChange                 RuntimeEffectKind = "remote_git_change"
	RuntimeEffectToolInstallation                RuntimeEffectKind = "tool_installation"
	RuntimeEffectVercelProjectChange             RuntimeEffectKind = "vercel_project_change"
	RuntimeEffectVercelEnvironmentChange         RuntimeEffectKind = "vercel_environment_change"
	RuntimeEffectVercelDomainChange              RuntimeEffectKind = "vercel_domain_change"
	RuntimeEffectUpload                          RuntimeEffectKind = "upload"
	RuntimeEffectPreviewDeployment               RuntimeEffectKind = "preview_deployment"
	RuntimeEffectProductionDeployment            RuntimeEffectKind = "production_deployment"
)

type RuntimeMode struct {
	ID            string                   `json:"id"`
	Role          RuntimeModeRole          `json:"role"`
	Requirements  []RuntimeRequirement     `json:"requirements"`
	Authorities   []RuntimeAuthority       `json:"authorities"`
	Effects       []RuntimeEffect          `json:"effects"`
	Fallback      RuntimeFallback          `json:"fallback"`
	OnUnavailable RuntimeUnavailablePolicy `json:"on_unavailable"`
}

type RuntimeRequirement struct {
	Kind    RuntimeRequirementKind `json:"kind"`
	ID      string                 `json:"id"`
	Version string                 `json:"version,omitempty"`
}

type RuntimeAuthority struct {
	Kind  RuntimeAuthorityKind `json:"kind"`
	Scope RuntimeScope         `json:"scope,omitempty"`
}

type RuntimeEffect struct {
	Kind  RuntimeEffectKind `json:"kind"`
	Scope RuntimeScope      `json:"scope,omitempty"`
}

type RuntimeFallback struct {
	Kind RuntimeFallbackKind `json:"kind"`
	Mode string              `json:"mode,omitempty"`
}

type ObservationState string
type ObservationReason string

const (
	ObservationAvailable   ObservationState = "available"
	ObservationUnavailable ObservationState = "unavailable"
	ObservationUnverified  ObservationState = "unverified"

	ObservationReasonVerified         ObservationReason = "verified"
	ObservationReasonNotFound         ObservationReason = "not_found"
	ObservationReasonPermissionDenied ObservationReason = "permission_denied"
	ObservationReasonVersionMismatch  ObservationReason = "version_mismatch"
	ObservationReasonObserverError    ObservationReason = "observer_error"
	ObservationReasonStale            ObservationReason = "stale"
)

type RuntimeObservation struct {
	State            ObservationState  `json:"state"`
	Reason           ObservationReason `json:"reason"`
	ObservedAt       string            `json:"observed_at"`
	ObserverRevision string            `json:"observer_revision"`
	RedactedIdentity string            `json:"redacted_identity,omitempty"`
}

// RuntimeRequirementObservation and RuntimeAuthorityObservation identify only
// declared portable facts. Their wire shapes cannot carry probe output,
// credential values, commands, or secret fingerprints.
type RuntimeRequirementObservation struct {
	Kind RuntimeRequirementKind `json:"kind"`
	ID   string                 `json:"id"`
	RuntimeObservation
}

type RuntimeAuthorityObservation struct {
	Kind  RuntimeAuthorityKind `json:"kind"`
	Scope RuntimeScope         `json:"scope"`
	RuntimeObservation
}

type RuntimeEvidence struct {
	Requirements []RuntimeRequirementObservation `json:"requirements"`
	Authorities  []RuntimeAuthorityObservation   `json:"authorities"`
}

type Binding struct {
	Surface      Surface             `json:"surface"`
	Projection   string              `json:"projection"`
	Name         string              `json:"name"`
	Invocation   string              `json:"invocation"`
	Mode         string              `json:"mode"`
	Degradation  string              `json:"degradation,omitempty"`
	Sharing      string              `json:"sharing"`
	Capabilities []SurfaceCapability `json:"capabilities"`
	Hook         *CommandHook        `json:"hook,omitempty"`
}

// ReferencedSourcePaths returns the typed capability paths owned by this
// binding. Keeping this switch with the capability vocabulary prevents callers
// from silently omitting a new source-bearing capability.
func (b Binding) ReferencedSourcePaths() []string {
	var paths []string
	for _, capability := range b.Capabilities {
		switch capability.Type {
		case SurfaceCapabilityOpenCodePrimaryPrompt:
			if capability.PrimaryPrompt != nil {
				paths = append(paths, capability.PrimaryPrompt.Source)
			}
		case SurfaceCapabilityProjectInstruction:
			if capability.ProjectInstruction != nil {
				paths = append(paths, capability.ProjectInstruction.Source)
			}
		}
	}
	return paths
}

type SurfaceCapabilityType string

const (
	SurfaceCapabilityClaudeAgentDocument           SurfaceCapabilityType = "claude-agent-document"
	SurfaceCapabilityClaudeCompositeSkill          SurfaceCapabilityType = "claude-composite-skill"
	SurfaceCapabilityExternalExecutableAcquisition SurfaceCapabilityType = "external-executable-acquisition"
	SurfaceCapabilityOpenCodePrimaryPrompt         SurfaceCapabilityType = "opencode-primary-prompt"
	SurfaceCapabilityProjectInstruction            SurfaceCapabilityType = "project-instruction"
)

// SurfaceCapability is one reviewed host-native behavior requested by a
// binding. Its closed wire shape deliberately cannot carry extension data.
type SurfaceCapability struct {
	ClaudeAgentDocument           *ClaudeAgentDocumentCapability           `json:"claude_agent_document,omitempty"`
	ClaudeCompositeSkill          *ClaudeCompositeSkillCapability          `json:"claude_composite_skill,omitempty"`
	Type                          SurfaceCapabilityType                    `json:"type"`
	ExternalExecutableAcquisition *ExternalExecutableAcquisitionCapability `json:"external_executable_acquisition,omitempty"`
	PrimaryPrompt                 *PrimaryPromptCapability                 `json:"primary_prompt,omitempty"`
	ProjectInstruction            *ProjectInstructionCapability            `json:"project_instruction,omitempty"`
}

type ClaudeCompositeSkillCapability struct {
	Dependencies []ResourceIdentity `json:"dependencies"`
	References   []ResourceIdentity `json:"references"`
}

type ClaudeAgentDocumentCapability struct {
	Skills    []ResourceIdentity `json:"skills"`
	Authority AgentAuthority     `json:"authority"`
}

type ExternalExecutableAcquisitionCapability struct {
	Tool string `json:"tool"`
}

type PrimaryPromptCapability struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type ProjectInstructionCapability struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

// RequestsSurfaceCapability reports whether the resource's binding for one
// surface requests a reviewed host-native behavior.
func (r Resource) RequestsSurfaceCapability(surface Surface, capability SurfaceCapabilityType) bool {
	_, ok := r.SurfaceCapability(surface, capability)
	return ok
}

// SurfaceCapability returns the reviewed capability data requested by the
// resource's binding for one surface.
func (r Resource) SurfaceCapability(surface Surface, capability SurfaceCapabilityType) (SurfaceCapability, bool) {
	for _, binding := range r.Bindings {
		if binding.Surface != surface {
			continue
		}
		for _, requested := range binding.Capabilities {
			if requested.Type == capability {
				return requested, true
			}
		}
	}
	return SurfaceCapability{}, false
}

func (p Pack) RequestsSurfaceCapability(surface Surface, capability SurfaceCapabilityType) bool {
	for _, resource := range p.Resources {
		if resource.RequestsSurfaceCapability(surface, capability) {
			return true
		}
	}
	return false
}

func (p Pack) externalToolCapability(surface Surface, tool string) (SurfaceCapabilityType, bool) {
	for _, resource := range p.Resources {
		capability, ok := resource.SurfaceCapability(surface, SurfaceCapabilityExternalExecutableAcquisition)
		if ok && capability.ExternalExecutableAcquisition.Tool == tool {
			return SurfaceCapabilityExternalExecutableAcquisition, true
		}
	}
	return "", false
}

type AgentAuthority struct {
	PermissionMode string            `json:"permission_mode"`
	Authorities    []AuthorityRecord `json:"authorities"`
}

type AuthorityRecord struct {
	Portable     string   `json:"portable"`
	Declarations []string `json:"declarations"`
	Outcome      string   `json:"outcome"`
	ClaudeTools  []string `json:"claude_tools"`
	Fallback     string   `json:"fallback"`
}

type CommandHook struct {
	Type           string   `json:"type"`
	Event          string   `json:"event"`
	Matcher        string   `json:"matcher"`
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Blocking       bool     `json:"blocking"`
	Failure        string   `json:"failure"`
	Authorities    []string `json:"authorities"`
}

type SurfaceExclusion struct {
	Surface Surface `json:"surface"`
	Mode    string  `json:"mode"`
	Code    string  `json:"code"`
	Reason  string  `json:"reason"`
}

type CommandArguments struct {
	Mode        string `json:"mode"`
	Placeholder string `json:"placeholder,omitempty"`
}

type Contract struct {
	Exclusions    []Exclusion    `json:"exclusions"`
	OptionalModes []OptionalMode `json:"optional_modes"`
}

type Exclusion struct {
	ID          string   `json:"id"`
	SourcePaths []string `json:"source_paths"`
	Reason      string   `json:"reason"`
}

type OptionalMode struct {
	ID          string   `json:"id"`
	Authorities []string `json:"authorities"`
	Fallback    string   `json:"fallback"`
}

type Pack struct {
	manifestVersion      int
	ID                   string
	Version              string
	Description          string
	Selectable           bool
	Surfaces             []Surface
	ReadinessObligations []ReadinessObligation
	Requires             Requirements
	Resources            []Resource
	Contract             Contract
}

type ResourceCounts struct {
	Skills       int
	Instructions int
	MCPServers   int
	Lifecycles   int
	Agents       int
	Commands     int
	Assets       int
	Notices      int
}

func (p Pack) ResourceCounts() ResourceCounts {
	var counts ResourceCounts
	for _, resource := range p.Resources {
		switch resource.Kind {
		case "skill":
			counts.Skills++
		case "instruction":
			counts.Instructions++
		case "mcp_server":
			counts.MCPServers++
		case "lifecycle":
			counts.Lifecycles++
		case "agent":
			counts.Agents++
		case "command":
			counts.Commands++
		case "asset":
			counts.Assets++
		case "notice":
			counts.Notices++
		}
	}
	return counts
}

type Catalog struct {
	packs                 []Pack
	bundleRoot            string
	entries               []catalogEntry
	deferSourceValidation bool
	transactionHeld       bool
}

type catalogEntry struct {
	ID          string
	Description string
	Surfaces    []Surface
}

type CatalogDetail struct {
	Pack              Pack
	ResourceInventory []DescriptiveResource
}

// Discover loads the strict initial catalog from a Packy-owned bundle root.
func Discover(ctx context.Context, bundleRoot string) (Catalog, error) {
	return discoverProductionCatalog(ctx, bundleRoot, true)
}

// DiscoverForDurableIntents retains the lifecycle-facing name while loading
// only the current manifest generation.
func DiscoverForDurableIntents(ctx context.Context, bundleRoot string) (Catalog, error) {
	return discoverProductionCatalog(ctx, bundleRoot, false)
}

func discoverProductionCatalog(ctx context.Context, bundleRoot string, validateSources bool) (Catalog, error) {
	var catalog Catalog
	err := bundletransaction.WithExclusive(ctx, filepath.Dir(filepath.Clean(bundleRoot)), func() error {
		var err error
		catalog, err = discoverCurrentCatalogUnlocked(bundleRoot, validateSources)
		return err
	})
	return catalog, err
}

func discoverCurrentCatalogUnlocked(bundleRoot string, validateSources bool) (Catalog, error) {
	entries, err := os.ReadDir(filepath.Join(bundleRoot, "packs"))
	if err != nil {
		return Catalog{}, fmt.Errorf("read Pack catalog: %w", err)
	}
	packs := make([]Pack, 0, len(entries))
	metadata := make([]catalogEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			return Catalog{}, fmt.Errorf("unexpected Pack catalog entry %q", entry.Name())
		}
		path := filepath.Join(bundleRoot, "packs", entry.Name(), "pack.json")
		pack, err := LoadCurrentManifest(path, bundleRoot, validateSources)
		if err != nil {
			return Catalog{}, err
		}
		if pack.ID != entry.Name() {
			return Catalog{}, fmt.Errorf("Pack directory %q contains manifest id %q", entry.Name(), pack.ID)
		}
		if !pack.Selectable {
			continue
		}
		packs = append(packs, pack)
		metadata = append(metadata, catalogEntry{ID: pack.ID, Description: pack.Description, Surfaces: append([]Surface(nil), pack.Surfaces...)})
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].ID < packs[j].ID })
	sort.Slice(metadata, func(i, j int) bool { return metadata[i].ID < metadata[j].ID })
	return Catalog{packs: packs, bundleRoot: bundleRoot, entries: metadata, deferSourceValidation: !validateSources}, nil
}

func (c Catalog) refreshed(ctx context.Context) (Catalog, error) {
	if c.bundleRoot == "" {
		return c, nil
	}
	var refreshed Catalog
	err := c.withBundleLock(ctx, func(locked Catalog) error {
		var err error
		refreshed, err = discoverCurrentCatalogUnlocked(c.bundleRoot, !c.deferSourceValidation)
		refreshed.transactionHeld = locked.transactionHeld
		return err
	})
	return refreshed, err
}

func (c Catalog) withBundleLock(ctx context.Context, observe func(Catalog) error) error {
	if c.bundleRoot == "" || c.transactionHeld {
		return observe(c)
	}
	return bundletransaction.WithExclusive(ctx, filepath.Dir(filepath.Clean(c.bundleRoot)), func() error {
		c.transactionHeld = true
		return observe(c)
	})
}

func (c Catalog) List() []Pack {
	packs := make([]Pack, len(c.packs))
	for i, pack := range c.packs {
		packs[i] = clonePack(pack)
	}
	return packs
}

// ListDetails returns detached metadata only after every advertised current
// pack has passed the same fresh validation as ListCurrent.
func (c Catalog) ListDetails(ctx context.Context) ([]CatalogDetail, error) {
	var details []CatalogDetail
	err := c.withBundleLock(ctx, func(locked Catalog) error {
		fresh, err := locked.refreshed(ctx)
		if err != nil {
			return err
		}
		details = make([]CatalogDetail, 0, len(fresh.packs))
		for _, metadata := range fresh.packs {
			pack, err := fresh.showUnlocked(metadata.ID)
			if err != nil {
				return err
			}
			details = append(details, catalogDetail(pack))
		}
		return nil
	})
	return details, err
}

func (c Catalog) ShowDetail(ctx context.Context, id string) (CatalogDetail, error) {
	pack, err := c.Show(ctx, id)
	if err != nil {
		return CatalogDetail{}, err
	}
	return catalogDetail(pack), nil
}

// ListCurrent returns only after every advertised catalog-current pack has
// passed the same source validation as direct current selection.
func (c Catalog) ListCurrent(ctx context.Context) ([]Pack, error) {
	var packs []Pack
	err := c.withBundleLock(ctx, func(locked Catalog) error {
		fresh, err := locked.refreshed(ctx)
		if err != nil {
			return err
		}
		packs = make([]Pack, 0, len(fresh.packs))
		for _, metadata := range fresh.packs {
			pack, err := fresh.showUnlocked(metadata.ID)
			if err != nil {
				return err
			}
			packs = append(packs, pack)
		}
		return nil
	})
	return packs, err
}

func (c Catalog) Show(ctx context.Context, id string) (Pack, error) {
	if !c.deferSourceValidation {
		return c.showUnlocked(id)
	}
	var pack Pack
	err := c.withBundleLock(ctx, func(locked Catalog) error {
		fresh, err := locked.refreshed(ctx)
		if err != nil {
			return err
		}
		pack, err = fresh.showUnlocked(id)
		return err
	})
	return pack, err
}

func (c Catalog) showUnlocked(id string) (Pack, error) {
	for _, pack := range c.packs {
		if pack.ID == id {
			if c.deferSourceValidation {
				if err := validatePackSources(pack, c.bundleRoot); err != nil {
					return Pack{}, fmt.Errorf("invalid catalog-current pack %q: %w", id, err)
				}
			}
			return clonePack(pack), nil
		}
	}
	return Pack{}, fmt.Errorf("unknown capability pack %q; run `packy list` to see available packs", id)
}

func (c Catalog) catalogMetadata(id string) (Pack, error) {
	for _, pack := range c.packs {
		if pack.ID == id {
			return clonePack(pack), nil
		}
	}
	return Pack{}, fmt.Errorf("unknown capability pack %q; run `packy list` to see available packs", id)
}

func clonePack(pack Pack) Pack {
	pack.Surfaces = append([]Surface(nil), pack.Surfaces...)
	pack.ReadinessObligations = append([]ReadinessObligation(nil), pack.ReadinessObligations...)
	pack.Requires.Tools = append([]string(nil), pack.Requires.Tools...)
	pack.Resources = append([]Resource(nil), pack.Resources...)
	for i := range pack.Resources {
		pack.Resources[i].Args = append([]string(nil), pack.Resources[i].Args...)
		pack.Resources[i].Tools = append([]string(nil), pack.Resources[i].Tools...)
		pack.Resources[i].Permissions = append([]string(nil), pack.Resources[i].Permissions...)
		pack.Resources[i].Requires = append([]string(nil), pack.Resources[i].Requires...)
		pack.Resources[i].Conflicts = append([]string(nil), pack.Resources[i].Conflicts...)
		pack.Resources[i].RequiresTools = append([]string(nil), pack.Resources[i].RequiresTools...)
		pack.Resources[i].Notices = append([]string(nil), pack.Resources[i].Notices...)
		pack.Resources[i].Bindings = append([]Binding(nil), pack.Resources[i].Bindings...)
		pack.Resources[i].SurfaceExclusions = append([]SurfaceExclusion(nil), pack.Resources[i].SurfaceExclusions...)
		for j := range pack.Resources[i].Bindings {
			binding := &pack.Resources[i].Bindings[j]
			binding.Capabilities = append([]SurfaceCapability(nil), binding.Capabilities...)
			for k := range binding.Capabilities {
				if binding.Capabilities[k].ClaudeCompositeSkill != nil {
					copy := *binding.Capabilities[k].ClaudeCompositeSkill
					copy.Dependencies = append([]ResourceIdentity(nil), copy.Dependencies...)
					copy.References = append([]ResourceIdentity(nil), copy.References...)
					binding.Capabilities[k].ClaudeCompositeSkill = &copy
				}
				if binding.Capabilities[k].ClaudeAgentDocument != nil {
					copy := *binding.Capabilities[k].ClaudeAgentDocument
					copy.Skills = append([]ResourceIdentity(nil), copy.Skills...)
					copy.Authority.Authorities = append([]AuthorityRecord(nil), copy.Authority.Authorities...)
					for n := range copy.Authority.Authorities {
						copy.Authority.Authorities[n].Declarations = append([]string(nil), copy.Authority.Authorities[n].Declarations...)
						copy.Authority.Authorities[n].ClaudeTools = append([]string(nil), copy.Authority.Authorities[n].ClaudeTools...)
					}
					binding.Capabilities[k].ClaudeAgentDocument = &copy
				}
				if binding.Capabilities[k].ExternalExecutableAcquisition != nil {
					copy := *binding.Capabilities[k].ExternalExecutableAcquisition
					binding.Capabilities[k].ExternalExecutableAcquisition = &copy
				}
				if binding.Capabilities[k].PrimaryPrompt != nil {
					copy := *binding.Capabilities[k].PrimaryPrompt
					binding.Capabilities[k].PrimaryPrompt = &copy
				}
				if binding.Capabilities[k].ProjectInstruction != nil {
					copy := *binding.Capabilities[k].ProjectInstruction
					binding.Capabilities[k].ProjectInstruction = &copy
				}
			}
			if binding.Hook != nil {
				copy := *binding.Hook
				copy.Args = append([]string(nil), copy.Args...)
				copy.Authorities = append([]string(nil), copy.Authorities...)
				binding.Hook = &copy
			}
		}
	}
	pack.Contract.Exclusions = append([]Exclusion(nil), pack.Contract.Exclusions...)
	for i := range pack.Contract.Exclusions {
		pack.Contract.Exclusions[i].SourcePaths = append([]string(nil), pack.Contract.Exclusions[i].SourcePaths...)
	}
	pack.Contract.OptionalModes = append([]OptionalMode(nil), pack.Contract.OptionalModes...)
	for i := range pack.Contract.OptionalModes {
		pack.Contract.OptionalModes[i].Authorities = append([]string(nil), pack.Contract.OptionalModes[i].Authorities...)
	}
	return pack
}

func catalogDetail(pack Pack) CatalogDetail {
	return CatalogDetail{
		Pack:              clonePack(pack),
		ResourceInventory: descriptiveResourceInventory(pack),
	}
}

func validateV3Surfaces(surfaces []Surface) error {
	if len(surfaces) == 0 {
		return fmt.Errorf("surfaces must contain at least one surface")
	}
	for i, surface := range surfaces {
		if surface != SurfaceClaude && surface != SurfaceCodex && surface != SurfaceOpenCode {
			return fmt.Errorf("unsupported CLI surface %q", surface)
		}
		if i > 0 && surfaces[i-1] >= surface {
			return fmt.Errorf("surfaces must be a sorted set")
		}
	}
	return nil
}

var runtimeRequirementKinds = map[RuntimeRequirementKind]bool{
	RuntimeRequirementTool:           true,
	RuntimeRequirementAuthentication: true,
	RuntimeRequirementProjectLink:    true,
	RuntimeRequirementEntitlement:    true,
	RuntimeRequirementServiceData:    true,
}

var runtimeAuthorityScopes = map[RuntimeAuthorityKind]map[RuntimeScope]bool{
	RuntimeAuthorityFilesystemRead:          {RuntimeScopeConsumerProject: true, RuntimeScopePackResource: true},
	RuntimeAuthorityFilesystemWrite:         {RuntimeScopeConsumerProject: true},
	RuntimeAuthorityProcessExecute:          {RuntimeScopeConsumerProject: true, RuntimeScopeLocalGit: true},
	RuntimeAuthorityNetwork:                 {RuntimeScopeRemoteGit: true, RuntimeScopeVercelAccount: true, RuntimeScopeVercelProject: true},
	RuntimeAuthorityEnvironmentInspect:      {RuntimeScopeConsumerProject: true},
	RuntimeAuthoritySecretUse:               {RuntimeScopeVercelAccount: true},
	RuntimeAuthorityPackageManagerExecute:   {RuntimeScopeWorkstation: true},
	RuntimeAuthorityGitInspect:              {RuntimeScopeLocalGit: true},
	RuntimeAuthorityGitCommit:               {RuntimeScopeLocalGit: true},
	RuntimeAuthorityGitPush:                 {RuntimeScopeRemoteGit: true},
	RuntimeAuthorityVercelProjectMutate:     {RuntimeScopeVercelProject: true},
	RuntimeAuthorityVercelEnvironmentMutate: {RuntimeScopeVercelProject: true},
	RuntimeAuthorityVercelDomainMutate:      {RuntimeScopeVercelProject: true},
	RuntimeAuthorityPreviewDeploy:           {RuntimeScopeVercelProject: true},
	RuntimeAuthorityProductionDeploy:        {RuntimeScopeVercelProject: true},
	RuntimeAuthorityUpload:                  {RuntimeScopeDeploymentPayload: true},
	RuntimeAuthoritySubagentDelegate:        {RuntimeScopeConsumerProject: true},
}

func runtimeScopedKey[K ~string, S ~string](kind K, scope S) string {
	return string(kind) + "\x00" + string(scope)
}

var observationReasons = map[ObservationState]map[ObservationReason]bool{
	ObservationAvailable:   {ObservationReasonVerified: true},
	ObservationUnavailable: {ObservationReasonNotFound: true, ObservationReasonPermissionDenied: true, ObservationReasonVersionMismatch: true},
	ObservationUnverified:  {ObservationReasonObserverError: true, ObservationReasonStale: true},
}

func validateRuntimeObservation(observation RuntimeObservation) error {
	if !observationReasons[observation.State][observation.Reason] {
		return fmt.Errorf("observation state or reason is invalid")
	}
	if _, err := time.Parse(time.RFC3339, observation.ObservedAt); err != nil {
		return fmt.Errorf("observation observed_at must be RFC3339")
	}
	if strings.TrimSpace(observation.ObserverRevision) == "" {
		return fmt.Errorf("observation observer_revision is required")
	}
	if observation.RedactedIdentity != "" && !idPattern.MatchString(observation.RedactedIdentity) {
		return fmt.Errorf("observation redacted_identity must be portable kebab-case")
	}
	return nil
}

func ValidateRuntimeRequirementObservation(observation RuntimeRequirementObservation) error {
	if !runtimeRequirementKinds[observation.Kind] || !idPattern.MatchString(observation.ID) {
		return fmt.Errorf("requirement observation identity is invalid")
	}
	return validateRuntimeObservation(observation.RuntimeObservation)
}

func ValidateRuntimeAuthorityObservation(observation RuntimeAuthorityObservation) error {
	if !runtimeAuthorityScopes[observation.Kind][observation.Scope] {
		return fmt.Errorf("authority observation kind or scope is invalid")
	}
	return validateRuntimeObservation(observation.RuntimeObservation)
}

func ValidateRuntimeEvidence(evidence RuntimeEvidence) error {
	if evidence.Requirements == nil || evidence.Authorities == nil {
		return fmt.Errorf("runtime evidence arrays are required and non-null")
	}
	for i, observation := range evidence.Requirements {
		if err := ValidateRuntimeRequirementObservation(observation); err != nil {
			return err
		}
		if i > 0 && runtimeScopedKey(evidence.Requirements[i-1].Kind, evidence.Requirements[i-1].ID) >= runtimeScopedKey(observation.Kind, observation.ID) {
			return fmt.Errorf("runtime requirement observations must be sorted without duplicates")
		}
	}
	for i, observation := range evidence.Authorities {
		if err := ValidateRuntimeAuthorityObservation(observation); err != nil {
			return err
		}
		if i > 0 && runtimeScopedKey(evidence.Authorities[i-1].Kind, evidence.Authorities[i-1].Scope) >= runtimeScopedKey(observation.Kind, observation.Scope) {
			return fmt.Errorf("runtime authority observations must be sorted without duplicates")
		}
	}
	return nil
}

func DecodeRuntimeEvidence(data []byte) (RuntimeEvidence, error) {
	var evidence RuntimeEvidence
	if err := strictDecode(data, &evidence); err != nil {
		return RuntimeEvidence{}, err
	}
	if err := ValidateRuntimeEvidence(evidence); err != nil {
		return RuntimeEvidence{}, err
	}
	return evidence, nil
}

func validateResourceV3(resource Resource, surfaces []Surface, optionalModes []OptionalMode) error {
	if resource.Requires == nil || resource.Bindings == nil || resource.SurfaceExclusions == nil {
		return fmt.Errorf("requires, bindings, and surface_exclusions are required non-null arrays")
	}
	if !sort.StringsAreSorted(resource.Requires) || hasDuplicateStrings(resource.Requires) {
		return fmt.Errorf("requires must be a sorted set of canonical identities")
	}
	for _, dependency := range resource.Requires {
		if !validResourceIdentity(dependency) {
			return fmt.Errorf("requires identity %q must be <kind>:<id>", dependency)
		}
	}
	switch resource.Kind {
	case "skill", "instruction", "agent", "command", "asset", "notice":
		if err := validateSourcePath(resource.Source); err != nil {
			return fmt.Errorf("source: %w", err)
		}
	case "mcp_server":
		if strings.TrimSpace(resource.Command) == "" || resource.Args == nil {
			return fmt.Errorf("command and args are required")
		}
	case "lifecycle":
	default:
		return fmt.Errorf("unsupported resource kind %q", resource.Kind)
	}
	if resource.Kind == "agent" {
		if strings.TrimSpace(resource.Description) == "" || (resource.Mode != "primary" && resource.Mode != "subagent") || resource.Tools == nil || resource.Permissions == nil {
			return fmt.Errorf("agent description, mode, tools, and permissions are required")
		}
		if !sortedPortableSet(resource.Tools, idPattern.MatchString) || !sortedPortableSet(resource.Permissions, func(v string) bool { return portableAuthorities[v] }) {
			return fmt.Errorf("agent tools and permissions must be sorted supported sets")
		}
	}
	if resource.Kind == "command" && resource.Arguments.Mode != "none" && (resource.Arguments.Mode != "freeform" || resource.Arguments.Placeholder != "$ARGUMENTS") {
		return fmt.Errorf("arguments must be none or freeform with $ARGUMENTS")
	}
	if resource.Kind == "notice" {
		if resource.License == "" || strings.TrimSpace(resource.Attribution) == "" || len(resource.Requires) != 0 || len(resource.Bindings) != 0 || len(resource.SurfaceExclusions) != 0 {
			return fmt.Errorf("notice requires license and attribution and empty requires, bindings, and surface_exclusions")
		}
		return nil
	}
	if resource.Kind == "asset" {
		if len(resource.Bindings) != 0 || len(resource.SurfaceExclusions) != 0 {
			return fmt.Errorf("asset bindings and surface_exclusions must be empty")
		}
		return nil
	}
	seen := map[Surface]bool{}
	for _, binding := range resource.Bindings {
		if seen[binding.Surface] {
			return fmt.Errorf("duplicate or contradictory surface outcome %q", binding.Surface)
		}
		seen[binding.Surface] = true
		if err := validateBindingV3(resource, binding, optionalModes); err != nil {
			return err
		}
	}
	for i, exclusion := range resource.SurfaceExclusions {
		if i > 0 && resource.SurfaceExclusions[i-1].Surface >= exclusion.Surface {
			return fmt.Errorf("surface_exclusions must be sorted by surface without duplicates")
		}
		if seen[exclusion.Surface] {
			return fmt.Errorf("duplicate or contradictory surface outcome %q", exclusion.Surface)
		}
		seen[exclusion.Surface] = true
		if exclusion.Mode != "optional" && exclusion.Mode != "mandatory" {
			return fmt.Errorf("surface exclusion mode must be optional or mandatory")
		}
		if !idPattern.MatchString(exclusion.Code) || strings.TrimSpace(exclusion.Reason) == "" {
			return fmt.Errorf("surface exclusion code and reason are required")
		}
	}
	for i, b := range resource.Bindings {
		if i > 0 && resource.Bindings[i-1].Surface >= b.Surface {
			return fmt.Errorf("bindings must be sorted by surface without duplicates")
		}
	}
	for _, surface := range surfaces {
		if !seen[surface] {
			return fmt.Errorf("missing binding-or-exclusion outcome for surface %q", surface)
		}
	}
	if len(seen) != len(surfaces) {
		return fmt.Errorf("surface outcome targets an undeclared surface")
	}
	return nil
}

func validateBindingV3(resource Resource, binding Binding, optionalModes []OptionalMode) error {
	kind := resource.Kind
	if binding.Surface != SurfaceClaude && binding.Surface != SurfaceCodex && binding.Surface != SurfaceOpenCode {
		return fmt.Errorf("binding surface %q is unsupported", binding.Surface)
	}
	if !idPattern.MatchString(binding.Name) || strings.TrimSpace(binding.Invocation) == "" || (binding.Mode != "native" && binding.Mode != "degraded") || (binding.Sharing != "exclusive" && binding.Sharing != "shared") {
		return fmt.Errorf("binding name, invocation, mode, and sharing are invalid")
	}
	if binding.Mode == "degraded" && strings.TrimSpace(binding.Degradation) == "" {
		return fmt.Errorf("degradation is required when mode is degraded")
	}
	if binding.Mode == "native" && binding.Degradation != "" {
		return fmt.Errorf("degradation is forbidden when mode is native")
	}
	if binding.Capabilities == nil {
		return fmt.Errorf("capabilities is a required non-null array")
	}
	for i, capability := range binding.Capabilities {
		if i > 0 && binding.Capabilities[i-1].Type >= capability.Type {
			return fmt.Errorf("capabilities must be sorted by type without duplicates")
		}
		switch capability.Type {
		case SurfaceCapabilityExternalExecutableAcquisition:
			if capability.ExternalExecutableAcquisition == nil {
				return fmt.Errorf("surface capability %q requires external_executable_acquisition data", capability.Type)
			}
			if capability.ClaudeAgentDocument != nil || capability.ClaudeCompositeSkill != nil || capability.PrimaryPrompt != nil || capability.ProjectInstruction != nil {
				return fmt.Errorf("surface capability %q does not accept other capability data", capability.Type)
			}
			if !idPattern.MatchString(capability.ExternalExecutableAcquisition.Tool) {
				return fmt.Errorf("surface capability %q external_executable_acquisition tool must be lowercase kebab-case", capability.Type)
			}
			if capability.ExternalExecutableAcquisition.Tool != "engram" {
				return fmt.Errorf("surface capability %q external_executable_acquisition tool %q is unsupported", capability.Type, capability.ExternalExecutableAcquisition.Tool)
			}
		case SurfaceCapabilityOpenCodePrimaryPrompt:
			if binding.Surface != SurfaceOpenCode {
				return fmt.Errorf("surface capability %q requires an opencode binding", capability.Type)
			}
			if capability.PrimaryPrompt == nil {
				return fmt.Errorf("surface capability %q requires primary_prompt data", capability.Type)
			}
			if capability.ProjectInstruction != nil {
				return fmt.Errorf("surface capability %q does not accept project_instruction data", capability.Type)
			}
			if capability.ClaudeAgentDocument != nil || capability.ClaudeCompositeSkill != nil || capability.ExternalExecutableAcquisition != nil {
				return fmt.Errorf("surface capability %q does not accept external_executable_acquisition data", capability.Type)
			}
			if !idPattern.MatchString(capability.PrimaryPrompt.ID) {
				return fmt.Errorf("surface capability %q primary_prompt id must be lowercase kebab-case", capability.Type)
			}
			if err := validateSourcePath(capability.PrimaryPrompt.Source); err != nil {
				return fmt.Errorf("surface capability %q primary_prompt source: %w", capability.Type, err)
			}
		case SurfaceCapabilityProjectInstruction:
			if binding.Surface != SurfaceCodex && binding.Surface != SurfaceOpenCode {
				return fmt.Errorf("surface capability %q requires a codex or opencode binding", capability.Type)
			}
			if capability.ProjectInstruction == nil {
				return fmt.Errorf("surface capability %q requires project_instruction data", capability.Type)
			}
			if capability.PrimaryPrompt != nil {
				return fmt.Errorf("surface capability %q does not accept primary_prompt data", capability.Type)
			}
			if capability.ClaudeAgentDocument != nil || capability.ClaudeCompositeSkill != nil || capability.ExternalExecutableAcquisition != nil {
				return fmt.Errorf("surface capability %q does not accept external_executable_acquisition data", capability.Type)
			}
			if !idPattern.MatchString(capability.ProjectInstruction.ID) {
				return fmt.Errorf("surface capability %q project_instruction id must be lowercase kebab-case", capability.Type)
			}
			if err := validateSourcePath(capability.ProjectInstruction.Source); err != nil {
				return fmt.Errorf("surface capability %q project_instruction source: %w", capability.Type, err)
			}
		case SurfaceCapabilityClaudeCompositeSkill:
			if binding.Surface != SurfaceClaude || binding.Projection != "skill" || resource.Kind != "skill" && resource.Kind != "command" {
				return fmt.Errorf("surface capability %q requires a Claude skill or command binding", capability.Type)
			}
			if capability.ClaudeCompositeSkill == nil {
				return fmt.Errorf("surface capability %q requires claude_composite_skill data", capability.Type)
			}
			if capability.ClaudeAgentDocument != nil || capability.ExternalExecutableAcquisition != nil || capability.PrimaryPrompt != nil || capability.ProjectInstruction != nil {
				return fmt.Errorf("surface capability %q does not accept other capability data", capability.Type)
			}
			if capability.ClaudeCompositeSkill.Dependencies == nil || capability.ClaudeCompositeSkill.References == nil {
				return fmt.Errorf("surface capability %q dependencies and references are required non-null arrays", capability.Type)
			}
		case SurfaceCapabilityClaudeAgentDocument:
			if binding.Surface != SurfaceClaude || binding.Projection != "agent" || resource.Kind != "agent" {
				return fmt.Errorf("surface capability %q requires a Claude agent binding", capability.Type)
			}
			if capability.ClaudeAgentDocument == nil {
				return fmt.Errorf("surface capability %q requires claude_agent_document data", capability.Type)
			}
			if capability.ClaudeCompositeSkill != nil || capability.ExternalExecutableAcquisition != nil || capability.PrimaryPrompt != nil || capability.ProjectInstruction != nil {
				return fmt.Errorf("surface capability %q does not accept other capability data", capability.Type)
			}
			if capability.ClaudeAgentDocument.Skills == nil {
				return fmt.Errorf("surface capability %q skills is a required non-null array", capability.Type)
			}
		default:
			return fmt.Errorf("surface capability %q is unsupported", capability.Type)
		}
	}
	if binding.Surface != SurfaceClaude {
		if binding.Hook != nil {
			return fmt.Errorf("Claude typed binding fields are forbidden on %s", binding.Surface)
		}
		return validateBinding(kind, binding)
	}
	want := map[string]string{"skill": "skill", "instruction": "instruction", "mcp_server": "mcp_server", "agent": "agent", "command": "skill", "lifecycle": "command_hook"}[kind]
	if binding.Projection != want {
		return fmt.Errorf("%s binding on claude must project as %s", kind, want)
	}
	currentContract := optionalModes == nil
	if !currentContract && (binding.Hook != nil) != (kind == "lifecycle") {
		return fmt.Errorf("typed Claude binding field does not match %s projection", kind)
	}
	if binding.Hook != nil {
		return validateCommandHook(*binding.Hook)
	}
	if kind == "agent" && (len(resource.Tools) > 0 || len(resource.Permissions) > 0) && !resource.RequestsSurfaceCapability(SurfaceClaude, SurfaceCapabilityClaudeAgentDocument) {
		return fmt.Errorf("Claude agent with tools or permissions requires %q", SurfaceCapabilityClaudeAgentDocument)
	}
	return nil
}

func validateAgentAuthority(authority AgentAuthority, tools, permissions []string, optionalModes []OptionalMode) error {
	if authority.PermissionMode != "default" {
		return fmt.Errorf("agent_authority permission_mode must be default")
	}
	if authority.Authorities == nil {
		return fmt.Errorf("agent_authority authorities is a required non-null array")
	}

	expected := map[string]string{}
	for _, tool := range tools {
		expected["tool:"+tool] = tool
	}
	for _, permission := range permissions {
		expected["permission:"+permission] = permission
	}
	for _, mode := range optionalModes {
		for _, portable := range mode.Authorities {
			declaration := "optional-mode:" + mode.ID + ":" + portable
			expected[declaration] = portable
		}
	}
	declaredFallbacks := map[string]string{}
	for _, mode := range optionalModes {
		for _, portable := range mode.Authorities {
			if fallback, exists := declaredFallbacks[portable]; exists && fallback != mode.Fallback {
				return fmt.Errorf("agent_authority portable authority %q has conflicting declared fallbacks", portable)
			}
			declaredFallbacks[portable] = mode.Fallback
		}
	}

	approvedClaudeTools := map[string]bool{
		"Bash": true, "Edit": true, "Glob": true, "Grep": true, "Read": true,
		"WebFetch": true, "WebSearch": true, "Write": true,
	}
	compatibleClaudeTools := map[string]map[string]bool{
		"filesystem": {"Edit": true, "Glob": true, "Grep": true, "Read": true, "Write": true},
		"network":    {"WebFetch": true, "WebSearch": true},
		"process":    {"Bash": true}, "package-manager": {"Bash": true},
		"commit": {"Bash": true}, "deploy": {"Bash": true},
		"browser": {}, "subagent": {},
	}
	seenDeclarations := map[string]bool{}
	for i, record := range authority.Authorities {
		if !portableAuthorities[record.Portable] {
			return fmt.Errorf("agent_authority portable authority %q is unsupported", record.Portable)
		}
		if i > 0 && authority.Authorities[i-1].Portable >= record.Portable {
			return fmt.Errorf("agent_authority authorities must be sorted by portable without duplicates")
		}
		if record.Declarations == nil || record.ClaudeTools == nil {
			return fmt.Errorf("agent_authority declarations and claude_tools are required non-null arrays")
		}
		if len(record.Declarations) == 0 {
			return fmt.Errorf("agent_authority record %q has no declarations", record.Portable)
		}
		if !sort.StringsAreSorted(record.Declarations) || hasDuplicateStrings(record.Declarations) {
			return fmt.Errorf("agent_authority declarations must be sorted without duplicates")
		}
		for _, declaration := range record.Declarations {
			portable, exists := expected[declaration]
			if !exists {
				return fmt.Errorf("agent_authority declaration %q is dangling or unknown", declaration)
			}
			if portable != record.Portable {
				return fmt.Errorf("agent_authority declaration %q belongs to portable authority %q", declaration, portable)
			}
			if seenDeclarations[declaration] {
				return fmt.Errorf("agent_authority declaration %q is duplicated", declaration)
			}
			seenDeclarations[declaration] = true
		}
		if !sort.StringsAreSorted(record.ClaudeTools) || hasDuplicateStrings(record.ClaudeTools) {
			return fmt.Errorf("agent_authority claude_tools must be sorted without duplicates")
		}
		switch record.Outcome {
		case "native", "fallback", "guarded":
		default:
			return fmt.Errorf("agent_authority outcome %q is unsupported", record.Outcome)
		}
		allowedOutcomes := map[string]map[string]bool{
			"browser":         {"fallback": true},
			"subagent":        {"fallback": true},
			"commit":          {"guarded": true},
			"deploy":          {"guarded": true},
			"process":         {"native": true},
			"package-manager": {"native": true},
			"filesystem":      {"native": true},
			"network":         {"native": true, "fallback": true},
		}
		if !allowedOutcomes[record.Portable][record.Outcome] {
			return fmt.Errorf("agent_authority portable authority %q does not allow %s outcome", record.Portable, record.Outcome)
		}
		if fallback, exists := declaredFallbacks[record.Portable]; exists && record.Fallback != fallback {
			return fmt.Errorf("agent_authority portable authority %q fallback must exactly match declared fallback %q", record.Portable, fallback)
		}
		for _, tool := range record.ClaudeTools {
			if !approvedClaudeTools[tool] {
				return fmt.Errorf("agent_authority Claude tool %q is unsupported", tool)
			}
			if !compatibleClaudeTools[record.Portable][tool] {
				return fmt.Errorf("agent_authority Claude tool %q is incompatible with portable authority %q", tool, record.Portable)
			}
		}
		exactTools := map[string][]string{
			"network": {"WebFetch", "WebSearch"}, "process": {"Bash"},
			"package-manager": {"Bash"}, "commit": {"Bash"}, "deploy": {"Bash"},
			"browser": {}, "subagent": {},
		}
		if want, exact := exactTools[record.Portable]; exact {
			if record.Outcome == "fallback" && record.Portable == "network" {
				want = []string{}
			}
			matches := len(record.ClaudeTools) == len(want)
			for i := 0; matches && i < len(record.ClaudeTools); i++ {
				matches = matches && record.ClaudeTools[i] == want[i]
			}
			if !matches {
				return fmt.Errorf("agent_authority portable authority %q requires exact claude_tools %v", record.Portable, want)
			}
		}
		switch record.Outcome {
		case "native":
			if len(record.ClaudeTools) == 0 || strings.TrimSpace(record.Fallback) == "" {
				return fmt.Errorf("agent_authority native outcome requires claude_tools and a nonempty fallback value")
			}
		case "fallback":
			if len(record.ClaudeTools) != 0 || strings.TrimSpace(record.Fallback) == "" || record.Fallback == "none" {
				return fmt.Errorf("agent_authority fallback outcome requires no claude_tools and a non-none fallback")
			}
		case "guarded":
			if len(record.ClaudeTools) == 0 || record.Fallback != "none" {
				return fmt.Errorf("agent_authority guarded outcome requires claude_tools and fallback none")
			}
		}
	}
	for declaration := range expected {
		if !seenDeclarations[declaration] {
			return fmt.Errorf("agent_authority declaration %q is missing", declaration)
		}
	}
	return nil
}

func validateCommandHook(hook CommandHook) error {
	events := map[string]bool{"PreToolUse": true, "PostToolUse": true, "PostToolUseFailure": true, "Notification": true, "UserPromptSubmit": true, "SessionStart": true, "SessionEnd": true, "Stop": true, "SubagentStart": true, "SubagentStop": true, "PreCompact": true, "PermissionRequest": true, "Setup": true}
	if hook.Type != "command" || !events[hook.Event] || strings.TrimSpace(hook.Command) == "" || hook.Args == nil || hook.TimeoutSeconds <= 0 || (hook.Failure != "block" && hook.Failure != "warn") || hook.Authorities == nil {
		return fmt.Errorf("hook type, event, command, args, positive timeout_seconds, failure, and authorities are invalid")
	}
	if !sortedPortableSet(hook.Authorities, func(v string) bool { return portableAuthorities[v] }) {
		return fmt.Errorf("hook authorities must be a sorted supported set")
	}
	matcherRequired := map[string]bool{"PreToolUse": true, "PostToolUse": true, "PostToolUseFailure": true, "PermissionRequest": true}
	if matcherRequired[hook.Event] && strings.TrimSpace(hook.Matcher) == "" {
		return fmt.Errorf("hook matcher is required for event %s", hook.Event)
	}
	return nil
}

var portableAuthorities = map[string]bool{
	"filesystem": true, "process": true, "network": true, "browser": true,
	"subagent": true, "package-manager": true, "commit": true, "deploy": true,
}

func validateBinding(kind string, binding Binding) error {
	if binding.Surface != SurfaceCodex && binding.Surface != SurfaceOpenCode {
		return fmt.Errorf("binding surface %q is unsupported", binding.Surface)
	}
	if !idPattern.MatchString(binding.Name) || strings.TrimSpace(binding.Invocation) == "" {
		return fmt.Errorf("binding name and invocation are required")
	}
	if binding.Mode != "native" && binding.Mode != "degraded" {
		return fmt.Errorf("binding mode must be native or degraded")
	}
	if binding.Mode == "degraded" && strings.TrimSpace(binding.Degradation) == "" {
		return fmt.Errorf("degradation is required when mode is degraded")
	}
	if binding.Mode == "native" && binding.Degradation != "" {
		return fmt.Errorf("degradation is forbidden when mode is native")
	}
	if binding.Sharing != "exclusive" && binding.Sharing != "shared" {
		return fmt.Errorf("sharing must be exclusive or shared")
	}
	if kind != "command" && binding.Mode != "native" {
		return fmt.Errorf("%s bindings must be native", kind)
	}
	wantProjection := kind
	if kind == "command" && binding.Surface == SurfaceCodex {
		wantProjection = "skill"
	}
	if binding.Projection != wantProjection {
		return fmt.Errorf("%s binding on %s must project as %s", kind, binding.Surface, wantProjection)
	}
	if kind == "command" {
		if binding.Surface == SurfaceCodex && (binding.Invocation != "$"+binding.Name || binding.Mode != "degraded" || binding.Degradation != "codex-command-as-workflow-skill") {
			return fmt.Errorf("Codex command binding must use the workflow-skill degradation")
		}
		if binding.Surface == SurfaceOpenCode && (binding.Invocation != "/"+binding.Name || binding.Mode != "native") {
			return fmt.Errorf("OpenCode command binding must be a native slash command")
		}
	}
	return nil
}

func validateDependencies(resources []Resource, identities map[string]bool, version int) error {
	dependencies := make(map[string][]string, len(resources))
	for _, resource := range resources {
		identity := resource.Kind + ":" + resource.ID
		for _, dependency := range resource.Requires {
			if !identities[dependency] {
				return fmt.Errorf("resource %q dependency %q does not exist", identity, dependency)
			}
			kind := strings.SplitN(dependency, ":", 2)[0]
			if kind == "notice" {
				return fmt.Errorf("resource %q dependency may not target notice", identity)
			}
			allowed := map[string]map[string]bool{
				"skill":   {"asset": true},
				"agent":   {"skill": true, "asset": true},
				"command": {"skill": true, "agent": true, "asset": true},
				"asset":   {"asset": true},
				"notice":  {},
			}
			if version != manifestSchemaV4 && !allowed[resource.Kind][kind] {
				return fmt.Errorf("resource %q may not depend on %s", identity, kind)
			}
		}
		dependencies[identity] = resource.Requires
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(identity string) error {
		if visiting[identity] {
			return fmt.Errorf("dependency cycle includes %q", identity)
		}
		if visited[identity] {
			return nil
		}
		visiting[identity] = true
		for _, dependency := range dependencies[identity] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		delete(visiting, identity)
		visited[identity] = true
		return nil
	}
	for identity := range dependencies {
		if err := visit(identity); err != nil {
			return err
		}
	}
	return nil
}

func validateResourceConflicts(resources []Resource, identities map[string]bool) error {
	byIdentity := make(map[string]Resource, len(resources))
	for _, resource := range resources {
		byIdentity[resource.Kind+":"+resource.ID] = resource
	}
	for _, resource := range resources {
		if resource.Kind == "notice" {
			continue
		}
		identity := resource.Kind + ":" + resource.ID
		for _, conflict := range resource.Conflicts {
			if conflict == identity {
				return fmt.Errorf("resource %q must not conflict with itself", identity)
			}
			if !identities[conflict] {
				return fmt.Errorf("resource %q conflict %q does not exist", identity, conflict)
			}
			target := byIdentity[conflict]
			if target.Kind == "notice" {
				return fmt.Errorf("resource %q conflict %q may not target notice", identity, conflict)
			}
			if !containsString(target.Conflicts, identity) {
				return fmt.Errorf("resource conflict between %q and %q must be symmetric", identity, conflict)
			}
		}

		closure := map[string]bool{identity: true}
		var visitDependencies func(string)
		visitDependencies = func(candidate string) {
			for _, dependency := range byIdentity[candidate].Requires {
				if closure[dependency] {
					continue
				}
				closure[dependency] = true
				visitDependencies(dependency)
			}
		}
		visitDependencies(identity)
		for _, conflict := range resource.Conflicts {
			if conflict != identity && closure[conflict] {
				return fmt.Errorf("resource %q must not conflict with mandatory dependency %q", identity, conflict)
			}
		}
		closureIdentities := make([]string, 0, len(closure))
		for candidate := range closure {
			closureIdentities = append(closureIdentities, candidate)
		}
		sort.Strings(closureIdentities)
		for _, candidate := range closureIdentities {
			if candidate == identity {
				continue
			}
			for _, conflict := range byIdentity[candidate].Conflicts {
				if conflict != identity && candidate < conflict && closure[conflict] {
					return fmt.Errorf("resource %q mandatory dependency closure contains conflicting resources %q and %q", identity, candidate, conflict)
				}
			}
		}
	}
	return nil
}

func validateContract(contract Contract, resources []Resource) error {
	if contract.Exclusions == nil || contract.OptionalModes == nil {
		return fmt.Errorf("contract exclusions and optional_modes are required arrays")
	}
	if !sortedByID(contract.Exclusions, func(value Exclusion) string { return value.ID }) || !sortedByID(contract.OptionalModes, func(value OptionalMode) string { return value.ID }) {
		return fmt.Errorf("contract entries must be sorted by id without duplicates")
	}
	sources := make([]string, 0, len(resources))
	for _, resource := range resources {
		sources = append(sources, filepath.ToSlash(filepath.Clean(resource.Source)))
	}
	for _, exclusion := range contract.Exclusions {
		if !idPattern.MatchString(exclusion.ID) || strings.TrimSpace(exclusion.Reason) == "" || exclusion.SourcePaths == nil || len(exclusion.SourcePaths) == 0 || !sort.StringsAreSorted(exclusion.SourcePaths) || hasDuplicateStrings(exclusion.SourcePaths) {
			return fmt.Errorf("exclusion %q must have an id, reason, and sorted source paths", exclusion.ID)
		}
		for _, path := range exclusion.SourcePaths {
			if err := validateSourcePath(path); err != nil {
				return fmt.Errorf("exclusion %q: %w", exclusion.ID, err)
			}
			clean := filepath.ToSlash(filepath.Clean(path))
			for _, source := range sources {
				if clean == source || strings.HasPrefix(clean, source+"/") || strings.HasPrefix(source, clean+"/") {
					return fmt.Errorf("exclusion %q path %q overlaps selected resource %q", exclusion.ID, path, source)
				}
			}
		}
	}
	for _, mode := range contract.OptionalModes {
		if !idPattern.MatchString(mode.ID) || mode.Authorities == nil || len(mode.Authorities) == 0 || !sortedPortableSet(mode.Authorities, func(value string) bool { return portableAuthorities[value] }) {
			return fmt.Errorf("optional mode %q authorities must be sorted supported values", mode.ID)
		}
		if strings.TrimSpace(mode.Fallback) == "" {
			return fmt.Errorf("optional mode %q fallback is required", mode.ID)
		}
	}
	return nil
}

func validResourceIdentity(value string) bool {
	parts := strings.Split(value, ":")
	return len(parts) == 2 && idPattern.MatchString(parts[0]) && idPattern.MatchString(parts[1])
}

func sortedPortableSet(values []string, valid func(string) bool) bool {
	if !sort.StringsAreSorted(values) || hasDuplicateStrings(values) {
		return false
	}
	for _, value := range values {
		if !valid(value) {
			return false
		}
	}
	return true
}

func hasDuplicateStrings(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] == values[i] {
			return true
		}
	}
	return false
}

func sortedByID[T any](values []T, id func(T) string) bool {
	for i := range values {
		if !idPattern.MatchString(id(values[i])) || i > 0 && id(values[i-1]) >= id(values[i]) {
			return false
		}
	}
	return true
}

func validatePackSources(pack Pack, bundleRoot string) error {
	for _, resource := range pack.Resources {
		if resource.Kind == "skill" || resource.Kind == "instruction" || resource.Kind == "agent" || resource.Kind == "command" || resource.Kind == "asset" || resource.Kind == "notice" {
			if err := validateSource(bundleRoot, resource); err != nil {
				return fmt.Errorf("resource %q source: %w", resource.Kind+":"+resource.ID, err)
			}
		}
		for _, binding := range resource.Bindings {
			for _, source := range binding.ReferencedSourcePaths() {
				if err := validateSource(bundleRoot, Resource{Kind: "instruction", Source: source}); err != nil {
					return fmt.Errorf("resource %q surface capability source: %w", resource.Kind+":"+resource.ID, err)
				}
			}
		}
	}
	return nil
}

func validateSourcePath(source string) error {
	if source == "" || filepath.IsAbs(source) {
		return fmt.Errorf("%q must be a relative path", source)
	}
	clean := filepath.Clean(source)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%q escapes the bundle root", source)
	}
	return nil
}

func validSemver(version string) bool {
	if !semverPattern.MatchString(version) {
		return false
	}
	withoutBuild := strings.SplitN(version, "+", 2)[0]
	parts := strings.SplitN(withoutBuild, "-", 2)
	if len(parts) == 1 {
		return true
	}
	for _, identifier := range strings.Split(parts[1], ".") {
		if len(identifier) > 1 && identifier[0] == '0' {
			numeric := true
			for _, char := range identifier {
				if char < '0' || char > '9' {
					numeric = false
					break
				}
			}
			if numeric {
				return false
			}
		}
	}
	return true
}

func validateSource(root string, resource Resource) error {
	source := resource.Source
	if err := validateSourcePath(source); err != nil {
		return err
	}
	clean := filepath.Clean(source)
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve bundle root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		return fmt.Errorf("resolve %q: %w", source, err)
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%q resolves outside the bundle root", source)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("inspect %q: %w", source, err)
	}
	if resource.Kind == "skill" {
		if !info.IsDir() {
			return fmt.Errorf("%q must be a skill directory", source)
		}
		if _, err := os.Stat(filepath.Join(resolved, "SKILL.md")); err != nil {
			return fmt.Errorf("%q missing SKILL.md: %w", source, err)
		}
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%q must be a regular source file", source)
	}
	return nil
}
