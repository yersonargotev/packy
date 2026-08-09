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
	ID                   string            `json:"id"`
	Version              string            `json:"version"`
	Description          string            `json:"description"`
	Selectable           *bool             `json:"selectable"`
	Surfaces             []Surface         `json:"surfaces"`
	ExternalRequirements []string          `json:"external_requirements"`
	Resources            []json.RawMessage `json:"resources"`
	Exclusions           []Exclusion       `json:"exclusions"`
	SourceReference      *SourceReference  `json:"source_reference,omitempty"`
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

// LoadCurrentManifest loads the one current Pack authoring contract. It has no
// schema selector and rejects fields from retired manifest generations.
func LoadCurrentManifest(path, bundleRoot string, validateSources bool) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, fmt.Errorf("read Pack manifest %s: %w", path, err)
	}
	var raw currentManifest
	if err := strictDecode(data, &raw); err != nil {
		return Pack{}, fmt.Errorf("decode Pack manifest %s: %w", path, err)
	}
	if raw.Selectable == nil {
		return Pack{}, fmt.Errorf("invalid Pack manifest %s: field selectable is required", path)
	}
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              raw.ID,
		Version:         raw.Version,
		Description:     raw.Description,
		Selectable:      *raw.Selectable,
		Surfaces:        raw.Surfaces,
		Requires:        Requirements{Tools: raw.ExternalRequirements},
		Contract:        Contract{Exclusions: raw.Exclusions, OptionalModes: []OptionalMode{}},
		SourceReference: raw.SourceReference,
	}
	for i, encoded := range raw.Resources {
		var wire currentResourceWire
		if err := strictDecode(encoded, &wire); err != nil {
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
		if err := validatePackSources(pack, bundleRoot); err != nil {
			return Pack{}, fmt.Errorf("invalid Pack manifest %s: %w", path, err)
		}
	}
	return pack, nil
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
	if pack.Requires.Tools == nil || !sortedPortableSet(pack.Requires.Tools, idPattern.MatchString) {
		return fmt.Errorf("Pack %q field external_requirements must be a sorted set of lowercase kebab-case tool identities", pack.ID)
	}
	if pack.Resources == nil {
		return fmt.Errorf("Pack %q field resources is a required non-null array", pack.ID)
	}
	if pack.Contract.Exclusions == nil {
		return fmt.Errorf("Pack %q field exclusions is a required non-null array", pack.ID)
	}
	if pack.SourceReference != nil && (strings.TrimSpace(pack.SourceReference.Repository) == "" || strings.TrimSpace(pack.SourceReference.Revision) == "") {
		return fmt.Errorf("Pack %q field source_reference requires repository and revision", pack.ID)
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
		for _, binding := range resource.Bindings {
			if binding.AgentAuthority != nil {
				if err := validateAgentAuthority(*binding.AgentAuthority, resource.Tools, resource.Permissions, []OptionalMode{}); err != nil {
					return fmt.Errorf("Pack %q resource %q: %w", pack.ID, identity, err)
				}
			}
		}
	}
	if !sort.StringsAreSorted(ordered) {
		return fmt.Errorf("Pack %q field resources must be sorted by kind and id", pack.ID)
	}
	if err := validateDependencies(pack.Resources, identities, manifestSchemaV4); err != nil {
		return fmt.Errorf("Pack %q: %w", pack.ID, err)
	}
	if err := validateResourceConflicts(pack.Resources, identities); err != nil {
		return fmt.Errorf("Pack %q: %w", pack.ID, err)
	}
	if err := validateContract(pack.Contract, pack.Resources); err != nil {
		return fmt.Errorf("Pack %q field exclusions: %w", pack.ID, err)
	}
	return nil
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
