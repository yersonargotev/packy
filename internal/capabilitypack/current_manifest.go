package capabilitypack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type currentManifest struct {
	ID                   string                `json:"id"`
	Version              string                `json:"version"`
	Description          string                `json:"description"`
	Selectable           *bool                 `json:"selectable"`
	Surfaces             []Surface             `json:"surfaces"`
	ReadinessObligations []ReadinessObligation `json:"readiness_obligations"`
	ExternalRequirements []string              `json:"external_requirements"`
	Resources            []json.RawMessage     `json:"resources"`
}

type managedCurrentManifest struct {
	SchemaVersion        int                   `json:"schema_version"`
	ID                   string                `json:"id"`
	Version              string                `json:"version"`
	Description          string                `json:"description"`
	Selectable           *bool                 `json:"selectable"`
	Surfaces             []Surface             `json:"surfaces"`
	ReadinessObligations []ReadinessObligation `json:"readiness_obligations"`
	ExternalRequirements []string              `json:"external_requirements"`
	Origins              []managedOriginWire   `json:"origins"`
	Resources            []json.RawMessage     `json:"resources"`
}

type managedOriginWire struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Revision   string `json:"revision,omitempty"`
}

type currentResourceWire struct {
	Kind              string             `json:"kind"`
	ID                string             `json:"id"`
	Source            string             `json:"source,omitempty"`
	Command           string             `json:"command,omitempty"`
	Args              []string           `json:"args,omitempty"`
	Description       string             `json:"description,omitempty"`
	Mode              string             `json:"mode,omitempty"`
	Tools             []string           `json:"tools,omitempty"`
	Permissions       []string           `json:"permissions,omitempty"`
	Arguments         CommandArguments   `json:"arguments,omitempty"`
	License           string             `json:"license,omitempty"`
	Attribution       string             `json:"attribution,omitempty"`
	Requires          []string           `json:"requires"`
	Conflicts         []string           `json:"conflicts"`
	Notices           []string           `json:"notices,omitempty"`
	Bindings          []Binding          `json:"bindings"`
	SurfaceExclusions []SurfaceExclusion `json:"surface_exclusions"`
}

type managedCurrentResourceWire struct {
	currentResourceWire
	Origin *managedResourceOriginWire `json:"origin,omitempty"`
}

type managedResourceOriginWire struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Relationship string `json:"relationship"`
}

// LoadCurrentManifest loads a materialized Managed Pack Project schema v1
// contract. It rejects manifests outside the current authoring model.
func LoadCurrentManifest(path, bundleRoot string, validateSources bool) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, fmt.Errorf("read Pack manifest %s: %w", path, err)
	}
	var shape struct {
		SchemaVersion json.RawMessage `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &shape); err != nil {
		return Pack{}, fmt.Errorf("decode Pack manifest %s: %w", path, err)
	}
	if shape.SchemaVersion == nil {
		return Pack{}, fmt.Errorf("invalid Pack manifest %s: Managed Pack schema_version is required", path)
	}
	return loadManagedCurrentManifest(data, path, bundleRoot, validateSources)
}

func loadManagedCurrentManifest(data []byte, path, bundleRoot string, validateSources bool) (Pack, error) {
	var managed managedCurrentManifest
	if err := strictDecode(data, &managed); err != nil {
		return Pack{}, fmt.Errorf("decode Pack manifest %s: %w", path, err)
	}
	if managed.SchemaVersion != 1 {
		return Pack{}, fmt.Errorf("invalid Pack manifest %s: Managed Pack schema_version must be 1", path)
	}
	if managed.Origins == nil {
		return Pack{}, fmt.Errorf("invalid Pack manifest %s: field origins is required", path)
	}
	return loadCurrentManifestRuntime(currentManifest{
		ID:                   managed.ID,
		Version:              managed.Version,
		Description:          managed.Description,
		Selectable:           managed.Selectable,
		Surfaces:             managed.Surfaces,
		ReadinessObligations: managed.ReadinessObligations,
		ExternalRequirements: managed.ExternalRequirements,
		Resources:            managed.Resources,
	}, path, bundleRoot, validateSources)
}

func loadCurrentManifestRuntime(raw currentManifest, path, bundleRoot string, validateSources bool) (Pack, error) {
	if raw.Selectable == nil {
		return Pack{}, fmt.Errorf("invalid Pack manifest %s: field selectable is required", path)
	}
	pack := Pack{
		ID:                   raw.ID,
		Version:              raw.Version,
		Description:          raw.Description,
		Selectable:           *raw.Selectable,
		Surfaces:             raw.Surfaces,
		ReadinessObligations: raw.ReadinessObligations,
		Requires:             Requirements{Tools: raw.ExternalRequirements},
		Contract:             Contract{OptionalModes: []OptionalMode{}},
	}
	for i, encoded := range raw.Resources {
		wire, err := decodeCurrentResource(encoded)
		if err != nil {
			return Pack{}, fmt.Errorf("Pack %q resource %d: %w", raw.ID, i, err)
		}
		pack.Resources = append(pack.Resources, Resource{
			Kind: wire.Kind, ID: wire.ID, Source: wire.Source, Command: wire.Command,
			Args: wire.Args, Description: wire.Description, Mode: wire.Mode,
			Tools: wire.Tools, Permissions: wire.Permissions, Arguments: wire.Arguments,
			License: wire.License, Attribution: wire.Attribution,
			Requires: wire.Requires, Conflicts: wire.Conflicts, Notices: wire.Notices,
			Bindings: wire.Bindings, SurfaceExclusions: wire.SurfaceExclusions,
			RequiresTools: []string{},
		})
	}
	if err := validateCurrentPack(pack); err != nil {
		return Pack{}, fmt.Errorf("invalid Pack manifest %s: %w", path, err)
	}
	if validateSources {
		if err := validatePackResourceSources(pack, bundleRoot); err != nil {
			return Pack{}, fmt.Errorf("invalid Pack manifest %s: %w", path, err)
		}
	}
	return pack, nil
}

func decodeCurrentResource(encoded json.RawMessage) (currentResourceWire, error) {
	var wire managedCurrentResourceWire
	if err := strictDecode(encoded, &wire); err != nil {
		return currentResourceWire{}, err
	}
	return wire.currentResourceWire, nil
}

// ValidateProjectPack validates the runtime-facing portion of a Managed Pack
// manifest against the same capability vocabulary used by Packy's catalog.
// Managed Pack provenance and closure remain owned by internal/managedpack.
func ValidateProjectPack(pack Pack, projectRoot string) error {
	pack.Contract = Contract{OptionalModes: []OptionalMode{}}
	if err := validateCurrentPack(pack); err != nil {
		return err
	}
	return validatePackResourceSources(pack, projectRoot)
}

func validateCurrentPack(pack Pack) error {
	if !idPattern.MatchString(pack.ID) {
		return fmt.Errorf("Pack %q field id must be lowercase kebab-case", pack.ID)
	}
	if !validSemver(pack.Version) {
		return fmt.Errorf("Pack %q field version %q must be SemVer", pack.ID, pack.Version)
	}
	if strings.TrimSpace(pack.Description) == "" {
		return fmt.Errorf("Pack %q field description is required", pack.ID)
	}
	if err := validateV3Surfaces(pack.Surfaces); err != nil {
		return fmt.Errorf("Pack %q field surfaces: %w", pack.ID, err)
	}
	if !validReadinessObligations(pack.ReadinessObligations) {
		return fmt.Errorf("Pack %q field readiness_obligations must be a sorted set of supported readiness obligations", pack.ID)
	}
	if pack.Requires.Tools == nil || !sortedPortableSet(pack.Requires.Tools, idPattern.MatchString) {
		return fmt.Errorf("Pack %q field external_requirements must be a sorted set of lowercase kebab-case tool identities", pack.ID)
	}
	if pack.Resources == nil {
		return fmt.Errorf("Pack %q field resources is a required non-null array", pack.ID)
	}
	identities := make(map[string]bool, len(pack.Resources))
	ordered := make([]string, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		identity := resource.Kind + ":" + resource.ID
		if !idPattern.MatchString(resource.ID) {
			return fmt.Errorf("Pack %q resource %q field id must be lowercase kebab-case", pack.ID, identity)
		}
		if identities[identity] {
			return fmt.Errorf("Pack %q has duplicate resource %q", pack.ID, identity)
		}
		identities[identity] = true
		ordered = append(ordered, identity)
		if strings.TrimSpace(resource.Description) == "" {
			return fmt.Errorf("Pack %q resource %q field description is required", pack.ID, identity)
		}
		if resource.Conflicts == nil {
			return fmt.Errorf("Pack %q resource %q field conflicts is a required non-null array", pack.ID, identity)
		}
		if err := validateResourceV3(resource, pack.Surfaces, nil); err != nil {
			return fmt.Errorf("Pack %q resource %q: %w", pack.ID, identity, err)
		}
	}
	if !sort.StringsAreSorted(ordered) {
		return fmt.Errorf("Pack %q field resources must be sorted by kind and id", pack.ID)
	}
	if err := validateDependencies(pack.Resources, identities); err != nil {
		return fmt.Errorf("Pack %q: %w", pack.ID, err)
	}
	if err := validateClaudeCompositionCapabilities(pack, identities); err != nil {
		return fmt.Errorf("Pack %q: %w", pack.ID, err)
	}
	if err := validateResourceConflicts(pack.Resources, identities); err != nil {
		return fmt.Errorf("Pack %q: %w", pack.ID, err)
	}
	if err := validateOptionalModes(pack.Contract.OptionalModes); err != nil {
		return fmt.Errorf("Pack %q: %w", pack.ID, err)
	}
	acquisitions := map[string]string{}
	for _, resource := range pack.Resources {
		for _, binding := range resource.Bindings {
			capability, ok := resource.SurfaceCapability(binding.Surface, SurfaceCapabilityExternalExecutableAcquisition)
			if !ok {
				continue
			}
			tool := capability.ExternalExecutableAcquisition.Tool
			if !containsString(pack.Requires.Tools, tool) {
				return fmt.Errorf("Pack %q surface capability %q requires external requirement %q", pack.ID, capability.Type, tool)
			}
			key := string(binding.Surface) + ":" + tool
			identity := resource.Kind + ":" + resource.ID
			if prior, exists := acquisitions[key]; exists {
				return fmt.Errorf("Pack %q resources %q and %q declare duplicate external executable acquisition for %s", pack.ID, prior, identity, key)
			}
			acquisitions[key] = identity
		}
	}
	return nil
}

func validateClaudeCompositionCapabilities(pack Pack, identities map[string]bool) error {
	byIdentity := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		byIdentity[resource.Kind+":"+resource.ID] = resource
	}
	for _, resource := range pack.Resources {
		owner := resource.Kind + ":" + resource.ID
		for _, binding := range resource.Bindings {
			for _, capability := range binding.Capabilities {
				var roles []struct {
					name     string
					values   []ResourceIdentity
					kind     string
					required bool
				}
				switch capability.Type {
				case SurfaceCapabilityClaudeCompositeSkill:
					roles = append(roles,
						struct {
							name     string
							values   []ResourceIdentity
							kind     string
							required bool
						}{"dependencies", capability.ClaudeCompositeSkill.Dependencies, "", true},
						struct {
							name     string
							values   []ResourceIdentity
							kind     string
							required bool
						}{"references", capability.ClaudeCompositeSkill.References, "asset", false},
					)
				case SurfaceCapabilityClaudeAgentDocument:
					if err := validateAgentAuthority(capability.ClaudeAgentDocument.Authority, resource.Tools, resource.Permissions, []OptionalMode{}); err != nil {
						return fmt.Errorf("resource %q Claude agent document authority: %w", owner, err)
					}
					roles = append(roles, struct {
						name     string
						values   []ResourceIdentity
						kind     string
						required bool
					}{"skills", capability.ClaudeAgentDocument.Skills, "skill", true})
				}
				for _, role := range roles {
					for i, value := range role.values {
						identity := value.Kind + ":" + value.ID
						if !identities[identity] || role.kind != "" && value.Kind != role.kind {
							return fmt.Errorf("resource %q Claude capability %s contains invalid %q", owner, role.name, identity)
						}
						if i > 0 {
							prior := role.values[i-1]
							if prior.Kind > value.Kind || prior.Kind == value.Kind && prior.ID >= value.ID {
								return fmt.Errorf("resource %q Claude capability %s must be sorted without duplicates", owner, role.name)
							}
						}
						if role.required && !containsString(resource.Requires, identity) {
							return fmt.Errorf("resource %q Claude capability %s must be a direct dependency %q", owner, role.name, identity)
						}
						dependency := byIdentity[identity]
						if value.Kind == "skill" || value.Kind == "agent" {
							matches := 0
							for _, candidate := range dependency.Bindings {
								if candidate.Surface == SurfaceClaude && candidate.Name != "" && candidate.Projection == value.Kind {
									matches++
								}
							}
							if matches != 1 {
								return fmt.Errorf("resource %q Claude capability %s dependency %q has no unique Claude binding", owner, role.name, identity)
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func validReadinessObligations(obligations []ReadinessObligation) bool {
	if obligations == nil {
		return false
	}
	values := make([]string, len(obligations))
	for i, obligation := range obligations {
		switch obligation {
		case ReadinessRuntimeUsability, ReadinessSurfaceAuthorization:
			values[i] = string(obligation)
		default:
			return false
		}
	}
	return sort.StringsAreSorted(values) && !hasDuplicateReadinessObligations(values)
}

func hasDuplicateReadinessObligations(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i] == values[i-1] {
			return true
		}
	}
	return false
}

func currentManifestPath(bundleRoot, pack string) (string, string, error) {
	packDir := pack
	if !filepath.IsAbs(pack) && filepath.Clean(pack) == pack && !strings.ContainsRune(pack, filepath.Separator) {
		packDir = filepath.Join(bundleRoot, "packs", pack)
	}
	abs, err := filepath.Abs(packDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve Pack directory %q: %w", pack, err)
	}
	return filepath.Join(abs, "pack.json"), abs, nil
}
