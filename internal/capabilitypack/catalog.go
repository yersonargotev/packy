// Package capabilitypack owns capability-pack discovery and policy.
package capabilitypack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	// schemaVersion remains the current state/history manifest version used by
	// the original capability-pack lifecycle documents.
	schemaVersion = manifestSchemaV1
)

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
	Capabilities []string `json:"capabilities"`
	Tools        []string `json:"tools"`
}

type Resource struct {
	Kind                 string
	ID                   string
	Source               string
	Command              string
	Args                 []string
	Description          string
	Mode                 string
	Tools                []string
	Permissions          []string
	Requires             []string
	Conflicts            []string
	ProvidesCapabilities []string
	RequiresCapabilities []string
	RequiresTools        []string
	CapabilityConflicts  []string
	Notices              []string
	Bindings             []Binding
	SurfaceExclusions    []SurfaceExclusion
	Arguments            CommandArguments
	License              string
	Attribution          string
	RuntimeModes         []RuntimeMode
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
	Surface        Surface         `json:"surface"`
	Projection     string          `json:"projection"`
	Name           string          `json:"name"`
	Invocation     string          `json:"invocation"`
	Mode           string          `json:"mode"`
	Degradation    string          `json:"degradation,omitempty"`
	Sharing        string          `json:"sharing"`
	AgentAuthority *AgentAuthority `json:"agent_authority,omitempty"`
	Hook           *CommandHook    `json:"hook,omitempty"`
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
	manifestVersion int
	ID              string
	Version         string
	Description     string
	Surfaces        []Surface
	Provides        []string
	Requires        Requirements
	Conflicts       []string
	Resources       []Resource
	RootMigrations  []RootMigration
	Contract        Contract
}

// RootMigration declares one exact manifest-v4 update identity transition.
type RootMigration struct {
	From ResourceIdentity `json:"from"`
	To   ResourceIdentity `json:"to"`
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
	allowSyntheticHistory bool
	deferSourceValidation bool
	transactionHeld       bool
	enforceUpdateRoutes   bool
}

type catalogEntry struct {
	ID                 string
	Description        string
	Surfaces           []Surface
	Withdrawn          bool
	HistoricalVersions []string
	UpdateRoutes       []UpdateRoute
}

// UpdateRoute identifies one exact, supported transition between catalog
// versions.
type UpdateRoute struct {
	FromVersion      string
	ToVersion        string
	ExistingSurfaces []Surface
}

// CatalogDetail is a detached view of a known catalog entry. Current reports
// whether the pack is selectable; withdrawn packs remain addressable by Show.
type CatalogDetail struct {
	Current            bool
	Withdrawn          bool
	Pack               Pack
	HistoricalVersions []string
	UpdateRoutes       []UpdateRoute
}

var initialCatalog = []catalogEntry{
	{
		ID:                 "addy",
		Description:        "Addy agent skills",
		Surfaces:           []Surface{SurfaceCodex, SurfaceOpenCode},
		HistoricalVersions: []string{"1.0.0", "1.1.0"},
		UpdateRoutes: []UpdateRoute{{
			FromVersion:      "1.0.0",
			ToVersion:        "1.1.0",
			ExistingSurfaces: []Surface{SurfaceCodex, SurfaceOpenCode},
		}},
	},
	{ID: "argote", Description: "Yerson Argote's engineering and communication guidance", Surfaces: []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}},
	{ID: "engram", Description: "Persistent memory for agent work", Surfaces: []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}},
	{ID: "matty", Description: "Matty workflow", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
}

// Discover loads the strict initial catalog from a Packy-owned bundle root.
func Discover(bundleRoot string) (Catalog, error) {
	return discoverProductionCatalog(bundleRoot, true)
}

// DiscoverForDurableIntents loads catalog metadata while deferring current
// source validation until a catalog-current pack is selected. This lets an
// existing pinned intent be reproduced solely from its historical artifact.
func DiscoverForDurableIntents(bundleRoot string) (Catalog, error) {
	return discoverProductionCatalog(bundleRoot, false)
}

func discoverProductionCatalog(bundleRoot string, validateSources bool) (Catalog, error) {
	catalog, err := discoverCatalogWithSourceValidation(bundleRoot, initialCatalog, validateSources)
	catalog.enforceUpdateRoutes = true
	return catalog, err
}

func discoverCatalog(bundleRoot string, entries []catalogEntry) (Catalog, error) {
	return discoverCatalogWithSourceValidation(bundleRoot, entries, true)
}

func discoverCatalogWithSourceValidation(bundleRoot string, entries []catalogEntry, validateSources bool) (Catalog, error) {
	var catalog Catalog
	err := bundletransaction.WithExclusive(context.Background(), filepath.Dir(filepath.Clean(bundleRoot)), func() error {
		var err error
		catalog, err = discoverCatalogUnlocked(bundleRoot, entries, validateSources)
		return err
	})
	return catalog, err
}

func discoverCatalogUnlocked(bundleRoot string, entries []catalogEntry, validateSources bool) (Catalog, error) {
	packs := make([]Pack, 0, len(entries))
	for _, entry := range entries {
		if err := validateCatalogMetadata(entry); err != nil {
			return Catalog{}, fmt.Errorf("catalog entry %q: %w", entry.ID, err)
		}
		manifestPath := filepath.Join(bundleRoot, "packs", entry.ID, "pack.json")
		pack, err := decodeManifestWithSourceValidation(manifestPath, bundleRoot, validateSources)
		if err != nil {
			return Catalog{}, err
		}
		if pack.ID != entry.ID {
			return Catalog{}, fmt.Errorf("catalog entry %q: manifest id is %q", entry.ID, pack.ID)
		}
		pack.Description = entry.Description
		manifestOwnedSurfaces := len(pack.Surfaces) > 0
		if !manifestOwnedSurfaces {
			pack.Surfaces = append([]Surface(nil), entry.Surfaces...)
		}
		if err := validateSurfaces(pack.Surfaces); err != nil {
			return Catalog{}, fmt.Errorf("pack %q: %w", pack.ID, err)
		}
		if len(entry.HistoricalVersions) > 0 && !containsString(entry.HistoricalVersions, pack.Version) {
			return Catalog{}, fmt.Errorf("catalog entry %q: current version %q is absent from immutable history metadata", entry.ID, pack.Version)
		}
		if pack.Contract.Exclusions != nil && !manifestOwnedSurfaces {
			if err := validateBindingsForSurfaces(pack); err != nil {
				return Catalog{}, fmt.Errorf("pack %q: %w", pack.ID, err)
			}
		}
		packs = append(packs, pack)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].ID < packs[j].ID })
	return Catalog{packs: packs, bundleRoot: bundleRoot, entries: cloneCatalogEntries(entries), deferSourceValidation: !validateSources}, nil
}

func (c Catalog) refreshed() (Catalog, error) {
	if c.bundleRoot == "" {
		return c, nil
	}
	var refreshed Catalog
	err := c.withBundleLock(context.Background(), func(locked Catalog) error {
		var err error
		refreshed, err = discoverCatalogUnlocked(c.bundleRoot, c.entries, !c.deferSourceValidation)
		refreshed.allowSyntheticHistory = c.allowSyntheticHistory
		refreshed.enforceUpdateRoutes = c.enforceUpdateRoutes
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
func (c Catalog) ListDetails() ([]CatalogDetail, error) {
	var details []CatalogDetail
	err := c.withBundleLock(context.Background(), func(locked Catalog) error {
		fresh, err := locked.refreshed()
		if err != nil {
			return err
		}
		details = make([]CatalogDetail, 0, len(fresh.packs))
		for _, metadata := range fresh.packs {
			entry, _ := fresh.catalogEntry(metadata.ID)
			if entry.Withdrawn {
				continue
			}
			pack, err := fresh.showUnlocked(metadata.ID)
			if err != nil {
				return err
			}
			details = append(details, catalogDetail(pack, entry))
		}
		return nil
	})
	return details, err
}

// ShowDetail returns detached metadata for a known pack, including a withdrawn
// pack that is intentionally absent from normal listing.
func (c Catalog) ShowDetail(id string) (CatalogDetail, error) {
	pack, err := c.Show(id)
	if err != nil {
		return CatalogDetail{}, err
	}
	entry, ok := c.catalogEntry(id)
	if !ok {
		return CatalogDetail{}, fmt.Errorf("unknown capability pack %q; run `packy pack list` to see available packs", id)
	}
	return catalogDetail(pack, entry), nil
}

// ListCurrent returns only after every advertised catalog-current pack has
// passed the same source validation as direct current selection.
func (c Catalog) ListCurrent() ([]Pack, error) {
	var packs []Pack
	err := c.withBundleLock(context.Background(), func(locked Catalog) error {
		fresh, err := locked.refreshed()
		if err != nil {
			return err
		}
		packs = make([]Pack, 0, len(fresh.packs))
		for _, metadata := range fresh.packs {
			entry, _ := fresh.catalogEntry(metadata.ID)
			if entry.Withdrawn {
				continue
			}
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

func (c Catalog) Show(id string) (Pack, error) {
	if !c.deferSourceValidation {
		return c.showUnlocked(id)
	}
	var pack Pack
	err := c.withBundleLock(context.Background(), func(locked Catalog) error {
		fresh, err := locked.refreshed()
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
	return Pack{}, fmt.Errorf("unknown capability pack %q; run `packy pack list` to see available packs", id)
}

func (c Catalog) catalogMetadata(id string) (Pack, error) {
	for _, pack := range c.packs {
		if pack.ID == id {
			return clonePack(pack), nil
		}
	}
	return Pack{}, fmt.Errorf("unknown capability pack %q; run `packy pack list` to see available packs", id)
}

func (c Catalog) withdrawn(id string) bool {
	entry, ok := c.catalogEntry(id)
	return ok && entry.Withdrawn
}

func clonePack(pack Pack) Pack {
	pack.Surfaces = append([]Surface(nil), pack.Surfaces...)
	pack.Provides = append([]string(nil), pack.Provides...)
	pack.Requires.Capabilities = append([]string(nil), pack.Requires.Capabilities...)
	pack.Requires.Tools = append([]string(nil), pack.Requires.Tools...)
	pack.Conflicts = append([]string(nil), pack.Conflicts...)
	pack.RootMigrations = append([]RootMigration(nil), pack.RootMigrations...)
	pack.Resources = append([]Resource(nil), pack.Resources...)
	for i := range pack.Resources {
		pack.Resources[i].Args = append([]string(nil), pack.Resources[i].Args...)
		pack.Resources[i].Tools = append([]string(nil), pack.Resources[i].Tools...)
		pack.Resources[i].Permissions = append([]string(nil), pack.Resources[i].Permissions...)
		pack.Resources[i].Requires = append([]string(nil), pack.Resources[i].Requires...)
		pack.Resources[i].Conflicts = append([]string(nil), pack.Resources[i].Conflicts...)
		pack.Resources[i].ProvidesCapabilities = append([]string(nil), pack.Resources[i].ProvidesCapabilities...)
		pack.Resources[i].RequiresCapabilities = append([]string(nil), pack.Resources[i].RequiresCapabilities...)
		pack.Resources[i].RequiresTools = append([]string(nil), pack.Resources[i].RequiresTools...)
		pack.Resources[i].CapabilityConflicts = append([]string(nil), pack.Resources[i].CapabilityConflicts...)
		pack.Resources[i].Notices = append([]string(nil), pack.Resources[i].Notices...)
		pack.Resources[i].Bindings = append([]Binding(nil), pack.Resources[i].Bindings...)
		pack.Resources[i].SurfaceExclusions = append([]SurfaceExclusion(nil), pack.Resources[i].SurfaceExclusions...)
		for j := range pack.Resources[i].Bindings {
			binding := &pack.Resources[i].Bindings[j]
			if binding.AgentAuthority != nil {
				copy := *binding.AgentAuthority
				copy.Authorities = append([]AuthorityRecord(nil), copy.Authorities...)
				for k := range copy.Authorities {
					copy.Authorities[k].Declarations = append([]string(nil), copy.Authorities[k].Declarations...)
					copy.Authorities[k].ClaudeTools = append([]string(nil), copy.Authorities[k].ClaudeTools...)
				}
				binding.AgentAuthority = &copy
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

func catalogDetail(pack Pack, entry catalogEntry) CatalogDetail {
	return CatalogDetail{
		Current:            !entry.Withdrawn,
		Withdrawn:          entry.Withdrawn,
		Pack:               clonePack(pack),
		HistoricalVersions: append([]string(nil), entry.HistoricalVersions...),
		UpdateRoutes:       cloneUpdateRoutes(entry.UpdateRoutes),
	}
}

func cloneCatalogEntries(entries []catalogEntry) []catalogEntry {
	cloned := make([]catalogEntry, len(entries))
	for i, entry := range entries {
		cloned[i] = entry
		cloned[i].Surfaces = append([]Surface(nil), entry.Surfaces...)
		cloned[i].HistoricalVersions = append([]string(nil), entry.HistoricalVersions...)
		cloned[i].UpdateRoutes = cloneUpdateRoutes(entry.UpdateRoutes)
	}
	return cloned
}

func cloneUpdateRoutes(routes []UpdateRoute) []UpdateRoute {
	cloned := make([]UpdateRoute, len(routes))
	for i, route := range routes {
		cloned[i] = route
		cloned[i].ExistingSurfaces = append([]Surface(nil), route.ExistingSurfaces...)
	}
	return cloned
}

func validateCatalogMetadata(entry catalogEntry) error {
	versions := make(map[string]bool, len(entry.HistoricalVersions))
	for i, version := range entry.HistoricalVersions {
		if !validSemver(version) {
			return fmt.Errorf("historical version %q is not valid SemVer", version)
		}
		if versions[version] {
			return fmt.Errorf("historical version %q is duplicated", version)
		}
		if i > 0 && compareSemanticVersions(entry.HistoricalVersions[i-1], version) >= 0 {
			return fmt.Errorf("historical versions must be in ascending canonical order")
		}
		versions[version] = true
	}
	routeKeys := make(map[string]bool, len(entry.UpdateRoutes))
	for i, route := range entry.UpdateRoutes {
		if !validSemver(route.FromVersion) || !validSemver(route.ToVersion) || route.FromVersion == route.ToVersion {
			return fmt.Errorf("update route %q -> %q is malformed", route.FromVersion, route.ToVersion)
		}
		if err := validateSurfaces(route.ExistingSurfaces); err != nil {
			return fmt.Errorf("update route %q -> %q: %w", route.FromVersion, route.ToVersion, err)
		}
		if !versions[route.FromVersion] || !versions[route.ToVersion] {
			return fmt.Errorf("update route %q -> %q references an unknown historical version", route.FromVersion, route.ToVersion)
		}
		key := route.FromVersion + "\x00" + route.ToVersion
		if routeKeys[key] {
			return fmt.Errorf("update route %q -> %q is duplicated", route.FromVersion, route.ToVersion)
		}
		if i > 0 && compareUpdateRoutes(entry.UpdateRoutes[i-1], route) >= 0 {
			return fmt.Errorf("update routes must be in ascending canonical order")
		}
		routeKeys[key] = true
	}
	return nil
}

func compareUpdateRoutes(left, right UpdateRoute) int {
	if compared := compareSemanticVersions(left.FromVersion, right.FromVersion); compared != 0 {
		return compared
	}
	return compareSemanticVersions(left.ToVersion, right.ToVersion)
}

func compareSemanticVersions(left, right string) int {
	leftVersion := parseSemanticVersion(left)
	rightVersion := parseSemanticVersion(right)
	for i := range leftVersion.core {
		if compared := compareSemanticVersionNumbers(leftVersion.core[i], rightVersion.core[i]); compared != 0 {
			return compared
		}
	}
	switch {
	case len(leftVersion.prerelease) == 0 && len(rightVersion.prerelease) > 0:
		return 1
	case len(leftVersion.prerelease) > 0 && len(rightVersion.prerelease) == 0:
		return -1
	}
	for i := 0; i < len(leftVersion.prerelease) && i < len(rightVersion.prerelease); i++ {
		leftIdentifier := leftVersion.prerelease[i]
		rightIdentifier := rightVersion.prerelease[i]
		leftNumeric := isSemanticVersionNumber(leftIdentifier)
		rightNumeric := isSemanticVersionNumber(rightIdentifier)
		switch {
		case leftNumeric && rightNumeric:
			if compared := compareSemanticVersionNumbers(leftIdentifier, rightIdentifier); compared != 0 {
				return compared
			}
		case leftNumeric && !rightNumeric:
			return -1
		case !leftNumeric && rightNumeric:
			return 1
		case !leftNumeric && !rightNumeric && leftIdentifier < rightIdentifier:
			return -1
		case !leftNumeric && !rightNumeric && leftIdentifier > rightIdentifier:
			return 1
		}
	}
	if len(leftVersion.prerelease) < len(rightVersion.prerelease) {
		return -1
	}
	if len(leftVersion.prerelease) > len(rightVersion.prerelease) {
		return 1
	}
	return strings.Compare(left, right)
}

type semanticVersion struct {
	core       [3]string
	prerelease []string
}

func parseSemanticVersion(value string) semanticVersion {
	withoutBuild, _, _ := strings.Cut(value, "+")
	core, prerelease, _ := strings.Cut(withoutBuild, "-")
	parts := strings.Split(core, ".")
	version := semanticVersion{}
	for i := range version.core {
		version.core[i] = parts[i]
	}
	if prerelease != "" {
		version.prerelease = strings.Split(prerelease, ".")
	}
	return version
}

func isSemanticVersionNumber(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compareSemanticVersionNumbers(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return strings.Compare(left, right)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type manifest struct {
	SchemaVersion  int               `json:"schema_version"`
	ID             string            `json:"id"`
	Version        string            `json:"version"`
	Provides       []string          `json:"provides"`
	Requires       Requirements      `json:"requires"`
	Conflicts      []string          `json:"conflicts"`
	Resources      []json.RawMessage `json:"resources"`
	RootMigrations *[]struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"root_migrations,omitempty"`
	Contract *Contract  `json:"contract,omitempty"`
	Surfaces *[]Surface `json:"surfaces,omitempty"`
}

func decodeManifest(path, bundleRoot string) (Pack, error) {
	return decodeManifestWithSourceValidation(path, bundleRoot, true)
}

// LoadPortableManifest exposes capability-pack's strict runtime decoder to
// Packy-owned producers and validators so they cannot accept a weaker wire
// contract than catalog discovery.
func LoadPortableManifest(path, bundleRoot string) (Pack, error) {
	return decodeManifestWithSourceValidation(path, bundleRoot, false)
}

// EncodePortableManifestV4 is the canonical producer seam for Manifest v4.
// Its output is accepted by LoadPortableManifest without a weaker producer-only
// interpretation of the contract.
func EncodePortableManifestV4(pack Pack) ([]byte, error) {
	if pack.Contract.OptionalModes != nil {
		return nil, fmt.Errorf("contract.optional_modes is forbidden for schema_version 4")
	}
	if err := validatePackMetadataWithContract(pack, manifestSchemaV4, true); err != nil {
		return nil, fmt.Errorf("invalid pack manifest: %w", err)
	}
	resources := make([]map[string]any, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		wire := map[string]any{
			"kind": resource.Kind, "id": resource.ID, "requires": resource.Requires,
			"bindings": resource.Bindings, "surface_exclusions": resource.SurfaceExclusions,
			"provides_capabilities": resource.ProvidesCapabilities,
			"requires_capabilities": resource.RequiresCapabilities,
			"requires_tools":        resource.RequiresTools,
			"capability_conflicts":  resource.CapabilityConflicts,
		}
		if resource.Kind != "notice" {
			wire["conflicts"] = resource.Conflicts
			wire["notices"] = resource.Notices
		}
		switch resource.Kind {
		case "skill", "instruction", "asset":
			wire["source"] = resource.Source
		case "mcp_server":
			wire["command"], wire["args"] = resource.Command, resource.Args
		case "lifecycle":
		case "agent":
			wire["source"], wire["description"], wire["mode"] = resource.Source, resource.Description, resource.Mode
			wire["tools"], wire["permissions"] = resource.Tools, resource.Permissions
		case "command":
			wire["source"], wire["arguments"] = resource.Source, resource.Arguments
		case "notice":
			wire["source"], wire["license"], wire["attribution"] = resource.Source, resource.License, resource.Attribution
		default:
			return nil, fmt.Errorf("unsupported resource kind %q", resource.Kind)
		}
		if resource.Kind == "skill" || resource.Kind == "agent" || resource.Kind == "command" {
			wire["runtime_modes"] = resource.RuntimeModes
		}
		resources = append(resources, wire)
	}
	wire := struct {
		SchemaVersion  int              `json:"schema_version"`
		ID             string           `json:"id"`
		Version        string           `json:"version"`
		Surfaces       []Surface        `json:"surfaces"`
		Provides       []string         `json:"provides"`
		Requires       Requirements     `json:"requires"`
		Conflicts      []string         `json:"conflicts"`
		Resources      []map[string]any `json:"resources"`
		RootMigrations []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"root_migrations"`
		Contract struct {
			Exclusions []Exclusion `json:"exclusions"`
		} `json:"contract"`
	}{
		SchemaVersion: manifestSchemaV4,
		ID:            pack.ID,
		Version:       pack.Version,
		Surfaces:      pack.Surfaces,
		Provides:      pack.Provides,
		Requires:      pack.Requires,
		Conflicts:     pack.Conflicts,
		Resources:     resources,
		RootMigrations: make([]struct {
			From string `json:"from"`
			To   string `json:"to"`
		}, 0, len(pack.RootMigrations)),
	}
	for _, migration := range pack.RootMigrations {
		wire.RootMigrations = append(wire.RootMigrations, struct {
			From string `json:"from"`
			To   string `json:"to"`
		}{From: migration.From.String(), To: migration.To.String()})
	}
	wire.Contract.Exclusions = pack.Contract.Exclusions
	encoded, err := json.MarshalIndent(wire, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func decodeManifestWithSourceValidation(path, bundleRoot string, validateSources bool) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, fmt.Errorf("read pack manifest %s: %w", path, err)
	}
	var raw manifest
	if err := strictDecode(data, &raw); err != nil {
		return Pack{}, fmt.Errorf("decode pack manifest %s: %w", path, err)
	}
	if (raw.SchemaVersion == manifestSchemaV3 || raw.SchemaVersion == manifestSchemaV4) && raw.Surfaces == nil {
		return Pack{}, fmt.Errorf("invalid pack manifest %s: surfaces is a required non-null array for schema_version %d", path, raw.SchemaVersion)
	}
	if raw.SchemaVersion != manifestSchemaV3 && raw.SchemaVersion != manifestSchemaV4 && raw.Surfaces != nil {
		return Pack{}, fmt.Errorf("invalid pack manifest %s: surfaces is forbidden before schema_version 3", path)
	}
	if raw.SchemaVersion == manifestSchemaV4 && raw.RootMigrations == nil {
		return Pack{}, fmt.Errorf("invalid pack manifest %s: root_migrations is a required non-null array for schema_version 4", path)
	}
	if raw.SchemaVersion != manifestSchemaV4 && raw.RootMigrations != nil {
		return Pack{}, fmt.Errorf("invalid pack manifest %s: root_migrations is forbidden before schema_version 4", path)
	}
	if raw.SchemaVersion == manifestSchemaV4 && raw.Contract != nil {
		var wire struct {
			Contract map[string]json.RawMessage `json:"contract"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return Pack{}, err
		}
		if _, present := wire.Contract["optional_modes"]; present {
			return Pack{}, fmt.Errorf("invalid pack manifest %s: contract.optional_modes is forbidden for schema_version 4", path)
		}
	}
	pack := Pack{manifestVersion: raw.SchemaVersion, ID: raw.ID, Version: raw.Version, Provides: raw.Provides, Requires: raw.Requires, Conflicts: raw.Conflicts}
	if raw.Surfaces != nil {
		pack.Surfaces = append([]Surface(nil), (*raw.Surfaces)...)
	}
	for i, encoded := range raw.Resources {
		resource, err := decodeResource(encoded, raw.SchemaVersion)
		if err != nil {
			return Pack{}, fmt.Errorf("pack %q resource %d: %w", raw.ID, i, err)
		}
		pack.Resources = append(pack.Resources, resource)
	}
	if raw.RootMigrations != nil {
		pack.RootMigrations = make([]RootMigration, 0, len(*raw.RootMigrations))
		for i, encoded := range *raw.RootMigrations {
			from, fromErr := ParseResourceIdentity(encoded.From)
			if fromErr != nil {
				return Pack{}, fmt.Errorf("pack %q root migration %d from: %w", raw.ID, i, fromErr)
			}
			to, toErr := ParseResourceIdentity(encoded.To)
			if toErr != nil {
				return Pack{}, fmt.Errorf("pack %q root migration %d to: %w", raw.ID, i, toErr)
			}
			pack.RootMigrations = append(pack.RootMigrations, RootMigration{From: from, To: to})
		}
	}
	if raw.Contract != nil {
		pack.Contract = *raw.Contract
	}
	if err := validatePackMetadataWithContract(pack, raw.SchemaVersion, raw.Contract != nil); err != nil {
		return Pack{}, fmt.Errorf("invalid pack manifest %s: %w", path, err)
	}
	if validateSources {
		if err := validatePackSources(pack, bundleRoot); err != nil {
			return Pack{}, fmt.Errorf("invalid pack manifest %s: %w", path, err)
		}
	}
	return pack, nil
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func decodeResource(data []byte, version int) (Resource, error) {
	var discriminator struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return Resource{}, err
	}
	if version == manifestSchemaV2 {
		return decodeResourceV2(data, discriminator.Kind)
	}
	if version == manifestSchemaV3 {
		return decodeResourceV3(data, discriminator.Kind)
	}
	if version == manifestSchemaV4 {
		return decodeResourceV4(data, discriminator.Kind)
	}
	if version != manifestSchemaV1 {
		return Resource{}, fmt.Errorf("schema_version must be %d, %d, %d, or %d", manifestSchemaV1, manifestSchemaV2, manifestSchemaV3, manifestSchemaV4)
	}
	switch discriminator.Kind {
	case "skill", "instruction":
		var raw struct{ Kind, ID, Source string }
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Source: raw.Source}, nil
	case "mcp_server":
		var raw struct {
			Kind, ID, Command string
			Args              []string
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Command: raw.Command, Args: raw.Args}, nil
	case "lifecycle":
		var raw struct{ Kind, ID string }
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID}, nil
	default:
		return Resource{}, fmt.Errorf("unsupported resource kind %q", discriminator.Kind)
	}
}

func decodeResourceV4(data []byte, kind string) (Resource, error) {
	if err := validateRuntimeModeWirePresence(data, kind == "skill" || kind == "agent" || kind == "command"); err != nil {
		return Resource{}, err
	}
	if err := validateNoticeWirePresence(data, kind == "notice"); err != nil {
		return Resource{}, err
	}
	if err := validateResourceConflictWirePresence(data, kind == "notice"); err != nil {
		return Resource{}, err
	}
	type resourceWireV4 struct {
		Kind                 string             `json:"kind"`
		ID                   string             `json:"id"`
		Requires             []string           `json:"requires"`
		Conflicts            []string           `json:"conflicts"`
		Bindings             []Binding          `json:"bindings"`
		SurfaceExclusions    []SurfaceExclusion `json:"surface_exclusions"`
		RuntimeModes         []RuntimeMode      `json:"runtime_modes"`
		Notices              []string           `json:"notices"`
		ProvidesCapabilities []string           `json:"provides_capabilities"`
		RequiresCapabilities []string           `json:"requires_capabilities"`
		RequiresTools        []string           `json:"requires_tools"`
		CapabilityConflicts  []string           `json:"capability_conflicts"`
	}
	type sourced struct {
		resourceWireV4
		Source string `json:"source"`
	}
	toResource := func(raw resourceWireV4) Resource {
		return Resource{Kind: raw.Kind, ID: raw.ID, Requires: raw.Requires, Conflicts: raw.Conflicts, Notices: raw.Notices, Bindings: raw.Bindings, SurfaceExclusions: raw.SurfaceExclusions, RuntimeModes: raw.RuntimeModes,
			ProvidesCapabilities: raw.ProvidesCapabilities, RequiresCapabilities: raw.RequiresCapabilities, RequiresTools: raw.RequiresTools, CapabilityConflicts: raw.CapabilityConflicts}
	}
	switch kind {
	case "skill", "instruction", "asset":
		var raw sourced
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.resourceWireV4)
		resource.Source = raw.Source
		return resource, nil
	case "mcp_server":
		var raw struct {
			resourceWireV4
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.resourceWireV4)
		resource.Command, resource.Args = raw.Command, raw.Args
		return resource, nil
	case "lifecycle":
		var raw resourceWireV4
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		if err := validateTypedBindingWirePresence(data); err != nil {
			return Resource{}, err
		}
		return toResource(raw), nil
	case "agent":
		var raw struct {
			sourced
			Description string   `json:"description"`
			Mode        string   `json:"mode"`
			Tools       []string `json:"tools"`
			Permissions []string `json:"permissions"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		if err := validateTypedBindingWirePresence(data); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.resourceWireV4)
		resource.Source, resource.Description, resource.Mode = raw.Source, raw.Description, raw.Mode
		resource.Tools, resource.Permissions = raw.Tools, raw.Permissions
		return resource, nil
	case "command":
		var raw struct {
			sourced
			Arguments CommandArguments `json:"arguments"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.resourceWireV4)
		resource.Source, resource.Arguments = raw.Source, raw.Arguments
		return resource, nil
	case "notice":
		var raw struct {
			sourced
			License     string `json:"license"`
			Attribution string `json:"attribution"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.resourceWireV4)
		resource.Source, resource.License, resource.Attribution = raw.Source, raw.License, raw.Attribution
		return resource, nil
	default:
		return Resource{}, fmt.Errorf("unsupported resource kind %q", kind)
	}
}

func validateResourceConflictWirePresence(data []byte, notice bool) error {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	value, present := wire["conflicts"]
	if notice {
		if present {
			return fmt.Errorf("conflicts is forbidden for notice resources")
		}
		return nil
	}
	if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return fmt.Errorf("conflicts is a required non-null array")
	}
	return nil
}

func validateNoticeWirePresence(data []byte, notice bool) error {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	value, present := wire["notices"]
	if notice {
		if present {
			return fmt.Errorf("notices is forbidden for notice resources")
		}
		return nil
	}
	if !present || bytes.Equal(value, []byte("null")) {
		return fmt.Errorf("notices is a required non-null array")
	}
	return nil
}

func validateRuntimeModeWirePresence(data []byte, executable bool) error {
	var resource map[string]json.RawMessage
	if err := json.Unmarshal(data, &resource); err != nil {
		return err
	}
	encoded, present := resource["runtime_modes"]
	if executable && (!present || bytes.Equal(bytes.TrimSpace(encoded), []byte("null"))) {
		return fmt.Errorf("runtime_modes is a required non-null array for executable resources")
	}
	if !executable && present {
		return fmt.Errorf("runtime_modes is forbidden for non-executable resources")
	}
	if !present {
		return nil
	}
	var modes []struct {
		Requirements []map[string]json.RawMessage `json:"requirements"`
		Fallback     map[string]json.RawMessage   `json:"fallback"`
	}
	if err := json.Unmarshal(encoded, &modes); err != nil {
		return err
	}
	for _, mode := range modes {
		var fallbackKind string
		if err := json.Unmarshal(mode.Fallback["kind"], &fallbackKind); err != nil {
			return err
		}
		_, hasMode := mode.Fallback["mode"]
		if fallbackKind == "none" && hasMode {
			return fmt.Errorf("fallback none forbids mode")
		}
		if fallbackKind == "mode" && !hasMode {
			return fmt.Errorf("fallback mode requires mode")
		}
		for _, requirement := range mode.Requirements {
			var kind string
			if err := json.Unmarshal(requirement["kind"], &kind); err != nil {
				return err
			}
			if encodedVersion, hasVersion := requirement["version"]; hasVersion {
				if kind != "tool" {
					return fmt.Errorf("requirement kind %q forbids version", kind)
				}
				var version string
				if err := json.Unmarshal(encodedVersion, &version); err != nil || version == "" {
					return fmt.Errorf("tool requirement version must be a non-null normalized SemVer predicate when present")
				}
			}
		}
	}
	return nil
}

func decodeResourceV3(data []byte, kind string) (Resource, error) {
	type outcomes struct {
		Kind              string             `json:"kind"`
		ID                string             `json:"id"`
		Requires          []string           `json:"requires"`
		Bindings          []Binding          `json:"bindings"`
		SurfaceExclusions []SurfaceExclusion `json:"surface_exclusions"`
	}
	type sourced struct {
		outcomes
		Source string `json:"source"`
	}
	toResource := func(raw outcomes) Resource {
		return Resource{Kind: raw.Kind, ID: raw.ID, Requires: raw.Requires, Bindings: raw.Bindings, SurfaceExclusions: raw.SurfaceExclusions}
	}
	switch kind {
	case "skill", "instruction", "asset":
		var raw sourced
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.outcomes)
		resource.Source = raw.Source
		return resource, nil
	case "mcp_server":
		var raw struct {
			outcomes
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.outcomes)
		resource.Command, resource.Args = raw.Command, raw.Args
		return resource, nil
	case "lifecycle":
		var raw outcomes
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		if err := validateTypedBindingWirePresence(data); err != nil {
			return Resource{}, err
		}
		return toResource(raw), nil
	case "agent":
		var raw struct {
			sourced
			Description string   `json:"description"`
			Mode        string   `json:"mode"`
			Tools       []string `json:"tools"`
			Permissions []string `json:"permissions"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		if err := validateTypedBindingWirePresence(data); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.outcomes)
		resource.Source, resource.Description, resource.Mode = raw.Source, raw.Description, raw.Mode
		resource.Tools, resource.Permissions = raw.Tools, raw.Permissions
		return resource, nil
	case "command":
		var raw struct {
			sourced
			Arguments CommandArguments `json:"arguments"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.outcomes)
		resource.Source, resource.Arguments = raw.Source, raw.Arguments
		return resource, nil
	case "notice":
		var raw struct {
			sourced
			License     string `json:"license"`
			Attribution string `json:"attribution"`
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		resource := toResource(raw.outcomes)
		resource.Source, resource.License, resource.Attribution = raw.Source, raw.License, raw.Attribution
		return resource, nil
	default:
		return Resource{}, fmt.Errorf("unsupported resource kind %q", kind)
	}
}

func validateTypedBindingWirePresence(data []byte) error {
	var resource struct {
		Bindings []map[string]json.RawMessage `json:"bindings"`
	}
	if err := json.Unmarshal(data, &resource); err != nil {
		return err
	}
	for _, binding := range resource.Bindings {
		hookData, ok := binding["hook"]
		if !ok {
			continue
		}
		var hook map[string]json.RawMessage
		if err := json.Unmarshal(hookData, &hook); err != nil {
			return err
		}
		for _, field := range []string{"matcher", "blocking"} {
			if _, ok := hook[field]; !ok {
				return fmt.Errorf("hook %s is required", field)
			}
		}
	}
	return nil
}

func decodeResourceV2(data []byte, kind string) (Resource, error) {
	type sourceResource struct {
		Kind     string    `json:"kind"`
		ID       string    `json:"id"`
		Source   string    `json:"source"`
		Requires []string  `json:"requires"`
		Bindings []Binding `json:"bindings"`
	}
	if kind == "skill" || kind == "agent" || kind == "command" {
		if err := validateBindingWirePresence(data); err != nil {
			return Resource{}, err
		}
	}
	switch kind {
	case "skill":
		var raw sourceResource
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Source: raw.Source, Requires: raw.Requires, Bindings: raw.Bindings}, nil
	case "agent":
		var raw struct {
			Kind, ID, Source, Description, Mode string
			Tools, Permissions, Requires        []string
			Bindings                            []Binding
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Source: raw.Source, Description: raw.Description, Mode: raw.Mode, Tools: raw.Tools, Permissions: raw.Permissions, Requires: raw.Requires, Bindings: raw.Bindings}, nil
	case "command":
		var raw struct {
			Kind, ID, Source string
			Arguments        CommandArguments
			Requires         []string
			Bindings         []Binding
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		var wire struct {
			Arguments map[string]json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return Resource{}, err
		}
		if raw.Arguments.Mode == "none" {
			if _, present := wire.Arguments["placeholder"]; present {
				return Resource{}, fmt.Errorf("none arguments forbid placeholder")
			}
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Source: raw.Source, Arguments: raw.Arguments, Requires: raw.Requires, Bindings: raw.Bindings}, nil
	case "asset":
		var raw struct {
			Kind, ID, Source string
			Requires         []string
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Source: raw.Source, Requires: raw.Requires}, nil
	case "notice":
		var raw struct {
			Kind, ID, Source, License, Attribution string
			Requires                               []string
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Source: raw.Source, License: raw.License, Attribution: raw.Attribution, Requires: raw.Requires}, nil
	default:
		return Resource{}, fmt.Errorf("unsupported resource kind %q", kind)
	}
}

func validateBindingWirePresence(data []byte) error {
	var resource struct {
		Bindings []json.RawMessage `json:"bindings"`
	}
	if err := json.Unmarshal(data, &resource); err != nil {
		return err
	}
	for _, data := range resource.Bindings {
		var binding map[string]json.RawMessage
		if err := json.Unmarshal(data, &binding); err != nil {
			return err
		}
		if _, present := binding["agent_authority"]; present {
			return fmt.Errorf("agent_authority is forbidden before schema_version 3")
		}
		if _, present := binding["hook"]; present {
			return fmt.Errorf("hook is forbidden before schema_version 3")
		}
		var mode string
		if err := json.Unmarshal(binding["mode"], &mode); err != nil {
			return err
		}
		if mode == "native" {
			if _, present := binding["degradation"]; present {
				return fmt.Errorf("degradation is forbidden when mode is native")
			}
		}
	}
	return nil
}

func validatePack(pack Pack, version int, bundleRoot string) error {
	if err := validatePackMetadata(pack, version); err != nil {
		return err
	}
	return validatePackSources(pack, bundleRoot)
}

func validatePackMetadata(pack Pack, version int) error {
	return validatePackMetadataWithContract(pack, version, version == manifestSchemaV2)
}

func validatePackMetadataWithContract(pack Pack, version int, contractPresent bool) error {
	if version != manifestSchemaV1 && version != manifestSchemaV2 && version != manifestSchemaV3 && version != manifestSchemaV4 {
		return fmt.Errorf("schema_version must be %d, %d, %d, or %d", manifestSchemaV1, manifestSchemaV2, manifestSchemaV3, manifestSchemaV4)
	}
	if (version == manifestSchemaV2 || version == manifestSchemaV3 || version == manifestSchemaV4) && !contractPresent {
		return fmt.Errorf("contract is required for schema_version %d", version)
	}
	if version == manifestSchemaV1 && contractPresent {
		return fmt.Errorf("contract is forbidden for schema_version 1")
	}
	if version == manifestSchemaV3 || version == manifestSchemaV4 {
		if err := validateV3Surfaces(pack.Surfaces); err != nil {
			return err
		}
	}
	if !idPattern.MatchString(pack.ID) {
		return fmt.Errorf("id %q must be lowercase kebab-case", pack.ID)
	}
	if !validSemver(pack.Version) {
		return fmt.Errorf("version %q must be SemVer", pack.Version)
	}
	if pack.Provides == nil || pack.Requires.Capabilities == nil || pack.Requires.Tools == nil || pack.Conflicts == nil || pack.Resources == nil {
		return fmt.Errorf("provides, requires.capabilities, requires.tools, conflicts, and resources are required arrays")
	}
	seenCapabilities := map[string]string{}
	for _, group := range []struct {
		name   string
		values []string
	}{{"provides", pack.Provides}, {"requires.capabilities", pack.Requires.Capabilities}, {"conflicts", pack.Conflicts}} {
		for _, capability := range group.values {
			if err := validateCapability(capability); err != nil {
				return fmt.Errorf("%s: %w", group.name, err)
			}
			if previous, ok := seenCapabilities[capability]; ok {
				return fmt.Errorf("capability %q appears in both %s and %s", capability, previous, group.name)
			}
			seenCapabilities[capability] = group.name
		}
	}
	seenTools := map[string]bool{}
	for _, tool := range pack.Requires.Tools {
		if !idPattern.MatchString(tool) {
			return fmt.Errorf("required tool %q must be lowercase kebab-case", tool)
		}
		if seenTools[tool] {
			return fmt.Errorf("duplicate required tool %q", tool)
		}
		seenTools[tool] = true
	}
	seenResources := map[string]bool{}
	identities := make([]string, 0, len(pack.Resources))
	if version == manifestSchemaV4 && (len(pack.Provides) != 0 || len(pack.Requires.Capabilities) != 0 || len(pack.Requires.Tools) != 0 || len(pack.Conflicts) != 0) {
		return fmt.Errorf("schema_version 4 resource capability contracts cannot be combined with Pack-level provides, requires, or conflicts")
	}
	for _, resource := range pack.Resources {
		if !idPattern.MatchString(resource.ID) {
			return fmt.Errorf("resource id %q must be lowercase kebab-case", resource.ID)
		}
		identity := resource.Kind + ":" + resource.ID
		if seenResources[identity] {
			return fmt.Errorf("duplicate resource %q", identity)
		}
		seenResources[identity] = true
		identities = append(identities, identity)
		if _, duplicate := seenCapabilities[identity]; duplicate {
			return fmt.Errorf("resource capability %q must not be declared at top level", identity)
		}
		if version == manifestSchemaV2 || version == manifestSchemaV3 || version == manifestSchemaV4 {
			if version == manifestSchemaV3 || version == manifestSchemaV4 {
				if err := validateResourceV3(resource, pack.Surfaces, pack.Contract.OptionalModes); err != nil {
					return fmt.Errorf("resource %q: %w", identity, err)
				}
				if version == manifestSchemaV4 {
					if resource.ProvidesCapabilities == nil || resource.RequiresCapabilities == nil || resource.RequiresTools == nil || resource.CapabilityConflicts == nil {
						return fmt.Errorf("resource %q: provides_capabilities, requires_capabilities, requires_tools, and capability_conflicts are required non-null arrays", identity)
					}
					seenResourceCapabilities := map[string]string{}
					for _, group := range []struct {
						name   string
						values []string
					}{
						{"provides_capabilities", resource.ProvidesCapabilities},
						{"requires_capabilities", resource.RequiresCapabilities},
						{"capability_conflicts", resource.CapabilityConflicts},
					} {
						if !sortedPortableSet(group.values, validCapabilityIdentity) {
							return fmt.Errorf("resource %q: %s must be a sorted set of canonical capability identities", identity, group.name)
						}
						for _, capability := range group.values {
							if previous, ok := seenResourceCapabilities[capability]; ok {
								return fmt.Errorf("resource %q: capability %q appears in both %s and %s", identity, capability, previous, group.name)
							}
							seenResourceCapabilities[capability] = group.name
						}
					}
					if !sortedPortableSet(resource.RequiresTools, idPattern.MatchString) {
						return fmt.Errorf("resource %q: requires_tools must be a sorted set of lowercase kebab-case tool identities", identity)
					}
					if resource.Kind == "notice" && (len(resource.ProvidesCapabilities) != 0 || len(resource.RequiresCapabilities) != 0 || len(resource.RequiresTools) != 0 || len(resource.CapabilityConflicts) != 0) {
						return fmt.Errorf("resource %q: notice capability and tool arrays must be empty", identity)
					}
					if resource.Kind == "asset" && len(resource.ProvidesCapabilities) != 0 {
						return fmt.Errorf("resource %q: non-rootable asset cannot provide capabilities", identity)
					}
					if err := validateRuntimeModes(resource); err != nil {
						return fmt.Errorf("resource %q: %w", identity, err)
					}
					if resource.Kind == "notice" {
						if resource.Conflicts != nil {
							return fmt.Errorf("resource %q: conflicts is forbidden for notice resources", identity)
						}
						if resource.Notices != nil {
							return fmt.Errorf("resource %q: notices is forbidden for notice resources", identity)
						}
					} else {
						if resource.Conflicts == nil {
							return fmt.Errorf("resource %q: conflicts is a required non-null array", identity)
						}
						if !sort.StringsAreSorted(resource.Conflicts) || hasDuplicateStrings(resource.Conflicts) {
							return fmt.Errorf("resource %q: conflicts must be a sorted set of canonical resource identities", identity)
						}
						for _, conflict := range resource.Conflicts {
							if _, err := ParseResourceIdentity(conflict); err != nil {
								return fmt.Errorf("resource %q: conflict identity %q must be canonical", identity, conflict)
							}
						}
						if resource.Notices == nil {
							return fmt.Errorf("resource %q: notices is a required non-null array", identity)
						}
						if !sort.StringsAreSorted(resource.Notices) || hasDuplicateStrings(resource.Notices) {
							return fmt.Errorf("resource %q: notices must be a sorted set of canonical notice identities", identity)
						}
						for _, notice := range resource.Notices {
							parsed, err := ParseResourceIdentity(notice)
							if err != nil || parsed.Kind != "notice" {
								return fmt.Errorf("resource %q: notices identity %q must be canonical notice:<id>", identity, notice)
							}
						}
					}
				}
				continue
			}
			if err := validateResourceV2(resource); err != nil {
				return fmt.Errorf("resource %q: %w", identity, err)
			}
			continue
		}
		switch resource.Kind {
		case "skill", "instruction":
			if err := validateSourcePath(resource.Source); err != nil {
				return fmt.Errorf("resource %q source: %w", identity, err)
			}
		case "mcp_server":
			if strings.TrimSpace(resource.Command) == "" {
				return fmt.Errorf("resource %q command is required", identity)
			}
			if resource.Args == nil {
				return fmt.Errorf("resource %q args is required", identity)
			}
		case "lifecycle":
		default:
			return fmt.Errorf("unsupported resource kind %q", resource.Kind)
		}
	}
	if version == manifestSchemaV2 || version == manifestSchemaV3 || version == manifestSchemaV4 {
		if !sortedPortableSet(pack.Provides, validCapabilityIdentity) || !sortedPortableSet(pack.Requires.Capabilities, validCapabilityIdentity) || !sortedPortableSet(pack.Requires.Tools, idPattern.MatchString) || !sortedPortableSet(pack.Conflicts, validCapabilityIdentity) {
			return fmt.Errorf("provides, requires, and conflicts arrays must be sorted sets")
		}
		if !sort.StringsAreSorted(identities) {
			return fmt.Errorf("resources must be sorted by kind and id")
		}
		if err := validateDependencies(pack.Resources, seenResources, version); err != nil {
			return err
		}
		if version == manifestSchemaV4 {
			if pack.RootMigrations == nil {
				return fmt.Errorf("root_migrations is a required non-null array")
			}
			if err := validateRootMigrations(pack); err != nil {
				return err
			}
			if err := validateResourceConflicts(pack.Resources, seenResources); err != nil {
				return err
			}
			for _, resource := range pack.Resources {
				identity := resource.Kind + ":" + resource.ID
				for _, notice := range resource.Notices {
					if !seenResources[notice] {
						return fmt.Errorf("resource %q notice %q does not exist", identity, notice)
					}
				}
			}
		}
		contract := pack.Contract
		if version == manifestSchemaV4 {
			if contract.Exclusions == nil {
				return fmt.Errorf("contract exclusions is a required non-null array")
			}
			contract.OptionalModes = []OptionalMode{}
		}
		if err := validateContract(contract, pack.Resources); err != nil {
			return err
		}
	}
	return nil
}

func validateRootMigrations(pack Pack) error {
	resources := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	sources, targets := map[string]bool{}, map[string]bool{}
	previous := ""
	for _, migration := range pack.RootMigrations {
		from, to := migration.From.String(), migration.To.String()
		if _, err := ParseResourceIdentity(from); err != nil {
			return fmt.Errorf("root migration from %q is not canonical", from)
		}
		if _, err := ParseResourceIdentity(to); err != nil {
			return fmt.Errorf("root migration to %q is not canonical", to)
		}
		if migration.From.Kind == "asset" || migration.From.Kind == "notice" ||
			migration.To.Kind == "asset" || migration.To.Kind == "notice" {
			return fmt.Errorf("root migration %q to %q must use operational root identities", from, to)
		}
		if from == to {
			return fmt.Errorf("root migration %q must change identity", from)
		}
		if _, exists := resources[from]; exists {
			return fmt.Errorf("root migration source %q must be absent from target resources", from)
		}
		target, exists := resources[to]
		if !exists {
			return fmt.Errorf("root migration target %q does not exist in target resources", to)
		}
		if target.Kind == "asset" || target.Kind == "notice" {
			return fmt.Errorf("root migration target %q is not an operational root", to)
		}
		if sources[from] {
			return fmt.Errorf("duplicate root migration source %q", from)
		}
		if targets[to] {
			return fmt.Errorf("duplicate root migration target %q", to)
		}
		key := from + "\x00" + to
		if previous != "" && previous >= key {
			return fmt.Errorf("root_migrations must be sorted by from then to")
		}
		if targets[from] || sources[to] {
			return fmt.Errorf("root migration chains and cycles are forbidden between %q and %q", from, to)
		}
		sources[from], targets[to], previous = true, true, key
	}
	return nil
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

var runtimeEffectScopes = map[RuntimeEffectKind]map[RuntimeScope]bool{
	RuntimeEffectAuthenticationStateChange:       {RuntimeScopeVercelAccount: true},
	RuntimeEffectConsumerProjectFileChange:       {RuntimeScopeConsumerProject: true},
	RuntimeEffectConsumerProjectDependencyChange: {RuntimeScopeConsumerProject: true},
	RuntimeEffectLocalGitChange:                  {RuntimeScopeLocalGit: true},
	RuntimeEffectRemoteGitChange:                 {RuntimeScopeRemoteGit: true},
	RuntimeEffectToolInstallation:                {RuntimeScopeWorkstation: true},
	RuntimeEffectVercelProjectChange:             {RuntimeScopeVercelProject: true},
	RuntimeEffectVercelEnvironmentChange:         {RuntimeScopeVercelProject: true},
	RuntimeEffectVercelDomainChange:              {RuntimeScopeVercelProject: true},
	RuntimeEffectUpload:                          {RuntimeScopeDeploymentPayload: true},
	RuntimeEffectPreviewDeployment:               {RuntimeScopeVercelProject: true},
	RuntimeEffectProductionDeployment:            {RuntimeScopeVercelProject: true},
}

var semverPredicatePattern = regexp.MustCompile(`^>=(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func validateRuntimeModes(resource Resource) error {
	executable := resource.Kind == "skill" || resource.Kind == "agent" || resource.Kind == "command"
	if executable && resource.RuntimeModes == nil {
		return fmt.Errorf("runtime_modes is a required non-null array for executable resources")
	}
	if !executable && resource.RuntimeModes != nil {
		return fmt.Errorf("runtime_modes is forbidden for non-executable resources")
	}
	modes := make(map[string]RuntimeMode, len(resource.RuntimeModes))
	for i, mode := range resource.RuntimeModes {
		if !idPattern.MatchString(mode.ID) {
			return fmt.Errorf("runtime mode id %q must be lowercase kebab-case", mode.ID)
		}
		if i > 0 && resource.RuntimeModes[i-1].ID >= mode.ID {
			return fmt.Errorf("runtime_modes must be sorted by id without duplicates")
		}
		if mode.Role != RuntimeModePrimary && mode.Role != RuntimeModeFallbackOnly {
			return fmt.Errorf("runtime mode %q role must be primary or fallback_only", mode.ID)
		}
		if mode.Requirements == nil || mode.Authorities == nil || mode.Effects == nil {
			return fmt.Errorf("runtime mode %q arrays are required and non-null", mode.ID)
		}
		for j, requirement := range mode.Requirements {
			if !runtimeRequirementKinds[requirement.Kind] || !idPattern.MatchString(requirement.ID) {
				return fmt.Errorf("runtime mode %q has invalid requirement %q:%q", mode.ID, requirement.Kind, requirement.ID)
			}
			if requirement.Kind == RuntimeRequirementTool {
				if requirement.Version != "" && !semverPredicatePattern.MatchString(requirement.Version) {
					return fmt.Errorf("runtime mode %q tool requirement version %q must be a normalized SemVer predicate", mode.ID, requirement.Version)
				}
			} else if requirement.Version != "" {
				return fmt.Errorf("runtime mode %q requirement kind %q forbids version", mode.ID, requirement.Kind)
			}
			if j > 0 && runtimeRequirementKey(mode.Requirements[j-1]) >= runtimeRequirementKey(requirement) {
				return fmt.Errorf("runtime mode %q requirements must be a sorted set", mode.ID)
			}
		}
		for j, authority := range mode.Authorities {
			if !runtimeAuthorityScopes[authority.Kind][authority.Scope] {
				return fmt.Errorf("runtime mode %q has invalid authority kind or scope %q:%q", mode.ID, authority.Kind, authority.Scope)
			}
			if j > 0 && runtimeScopedKey(mode.Authorities[j-1].Kind, mode.Authorities[j-1].Scope) >= runtimeScopedKey(authority.Kind, authority.Scope) {
				return fmt.Errorf("runtime mode %q authorities must be a sorted set", mode.ID)
			}
		}
		for j, effect := range mode.Effects {
			if !runtimeEffectScopes[effect.Kind][effect.Scope] {
				return fmt.Errorf("runtime mode %q has invalid effect kind or scope %q:%q", mode.ID, effect.Kind, effect.Scope)
			}
			if j > 0 && runtimeScopedKey(mode.Effects[j-1].Kind, mode.Effects[j-1].Scope) >= runtimeScopedKey(effect.Kind, effect.Scope) {
				return fmt.Errorf("runtime mode %q effects must be a sorted set", mode.ID)
			}
		}
		if mode.OnUnavailable != RuntimeFailBeforeEffects {
			return fmt.Errorf("runtime mode %q on_unavailable must be fail_before_effects", mode.ID)
		}
		if mode.Fallback.Kind == RuntimeFallbackNone {
			if mode.Fallback.Mode != "" {
				return fmt.Errorf("runtime mode %q fallback none forbids mode", mode.ID)
			}
		} else if mode.Fallback.Kind != RuntimeFallbackMode || !idPattern.MatchString(mode.Fallback.Mode) {
			return fmt.Errorf("runtime mode %q fallback must be none or a valid mode reference", mode.ID)
		}
		modes[mode.ID] = mode
	}
	for _, mode := range resource.RuntimeModes {
		if mode.Fallback.Kind != RuntimeFallbackMode {
			continue
		}
		if _, exists := modes[mode.Fallback.Mode]; !exists {
			return fmt.Errorf("runtime mode %q fallback must reference a mode in the same resource", mode.ID)
		}
		seen := map[string]bool{}
		for current := mode; current.Fallback.Kind == RuntimeFallbackMode; {
			if seen[current.ID] {
				return fmt.Errorf("runtime mode fallback graph must be acyclic")
			}
			seen[current.ID] = true
			current = modes[current.Fallback.Mode]
		}
	}
	for _, mode := range resource.RuntimeModes {
		if mode.Fallback.Kind != RuntimeFallbackMode {
			continue
		}
		target := modes[mode.Fallback.Mode]
		if mode.Role != RuntimeModePrimary || target.Role != RuntimeModeFallbackOnly {
			return fmt.Errorf("runtime mode %q fallback must reference a fallback_only mode from a primary mode", mode.ID)
		}
	}
	return nil
}

func runtimeRequirementKey(value RuntimeRequirement) string {
	return string(value.Kind) + "\x00" + value.ID + "\x00" + value.Version
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
	if binding.Surface != SurfaceClaude {
		if binding.AgentAuthority != nil || binding.Hook != nil {
			return fmt.Errorf("Claude typed binding fields are forbidden on %s", binding.Surface)
		}
		return validateBinding(kind, binding)
	}
	want := map[string]string{"skill": "skill", "instruction": "instruction", "mcp_server": "mcp_server", "agent": "agent", "command": "skill", "lifecycle": "command_hook"}[kind]
	if binding.Projection != want {
		return fmt.Errorf("%s binding on claude must project as %s", kind, want)
	}
	if (binding.AgentAuthority != nil) != (kind == "agent") || (binding.Hook != nil) != (kind == "lifecycle") {
		return fmt.Errorf("typed Claude binding field does not match %s projection", kind)
	}
	if binding.AgentAuthority != nil {
		return validateAgentAuthority(*binding.AgentAuthority, resource.Tools, resource.Permissions, optionalModes)
	}
	if binding.Hook != nil {
		return validateCommandHook(*binding.Hook)
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

func validateResourceV2(resource Resource) error {
	if err := validateSourcePath(resource.Source); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if resource.Requires == nil {
		return fmt.Errorf("requires is a required array")
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
	case "skill":
		if resource.Bindings == nil {
			return fmt.Errorf("bindings is a required array")
		}
	case "agent":
		if strings.TrimSpace(resource.Description) == "" || (resource.Mode != "primary" && resource.Mode != "subagent") {
			return fmt.Errorf("description and primary or subagent mode are required")
		}
		if resource.Tools == nil || resource.Permissions == nil || resource.Bindings == nil {
			return fmt.Errorf("tools, permissions, and bindings are required arrays")
		}
		if !sortedPortableSet(resource.Tools, idPattern.MatchString) {
			return fmt.Errorf("tools must be a sorted portable set")
		}
		if !sortedPortableSet(resource.Permissions, func(value string) bool { return portableAuthorities[value] }) {
			return fmt.Errorf("permissions must be a sorted authority set")
		}
	case "command":
		if resource.Bindings == nil {
			return fmt.Errorf("bindings is a required array")
		}
		if resource.Arguments.Mode == "none" {
			if resource.Arguments.Placeholder != "" {
				return fmt.Errorf("none arguments forbid placeholder")
			}
		} else if resource.Arguments.Mode != "freeform" || resource.Arguments.Placeholder != "$ARGUMENTS" {
			return fmt.Errorf("arguments must be none or freeform with $ARGUMENTS")
		}
	case "asset":
		if resource.Bindings != nil {
			return fmt.Errorf("bindings are forbidden")
		}
	case "notice":
		if resource.License == "" || strings.TrimSpace(resource.Attribution) == "" || len(resource.Requires) != 0 || resource.Bindings != nil {
			return fmt.Errorf("license and attribution are required; requires must be empty and bindings are forbidden")
		}
	default:
		return fmt.Errorf("unsupported resource kind %q", resource.Kind)
	}
	for _, binding := range resource.Bindings {
		if err := validateBinding(resource.Kind, binding); err != nil {
			return err
		}
	}
	for i := 1; i < len(resource.Bindings); i++ {
		if resource.Bindings[i-1].Surface >= resource.Bindings[i].Surface {
			return fmt.Errorf("bindings must be sorted by surface without duplicates")
		}
	}
	return nil
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

func validateBindingsForSurfaces(pack Pack) error {
	declared := make(map[Surface]bool, len(pack.Surfaces))
	for _, surface := range pack.Surfaces {
		declared[surface] = true
	}
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" && resource.Kind != "agent" && resource.Kind != "command" {
			continue
		}
		if len(resource.Bindings) != len(pack.Surfaces) {
			return fmt.Errorf("resource %q must have exactly one binding for each declared surface", resource.Kind+":"+resource.ID)
		}
		for _, binding := range resource.Bindings {
			if !declared[binding.Surface] {
				return fmt.Errorf("resource %q must have exactly one binding for each declared surface", resource.Kind+":"+resource.ID)
			}
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

func validCapabilityIdentity(value string) bool {
	return validateCapability(value) == nil
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
		if resource.Kind != "skill" && resource.Kind != "instruction" && resource.Kind != "agent" && resource.Kind != "command" && resource.Kind != "asset" && resource.Kind != "notice" {
			continue
		}
		if err := validateSource(bundleRoot, resource); err != nil {
			return fmt.Errorf("resource %q source: %w", resource.Kind+":"+resource.ID, err)
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

func validateCapability(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || !idPattern.MatchString(parts[0]) || !idPattern.MatchString(parts[1]) {
		return fmt.Errorf("capability %q must have two lowercase kebab-case segments", value)
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

func validateSurfaces(surfaces []Surface) error {
	if len(surfaces) == 0 {
		return fmt.Errorf("at least one supported CLI surface is required")
	}
	seen := map[Surface]bool{}
	for _, surface := range surfaces {
		if surface != SurfaceCodex && surface != SurfaceOpenCode && surface != SurfaceClaude {
			return fmt.Errorf("unsupported CLI surface %q", surface)
		}
		if seen[surface] {
			return fmt.Errorf("duplicate CLI surface %q", surface)
		}
		seen[surface] = true
	}
	return nil
}
