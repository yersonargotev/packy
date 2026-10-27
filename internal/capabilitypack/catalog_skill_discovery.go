package capabilitypack

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// SkillDiscovery describes a current catalog candidate, independently of native
// installation state. Dependencies and notices are local to PackID; typed
// on-demand dependencies carry their own qualified target.
type SkillDiscovery struct {
	DescriptiveResource
	QualifiedSkill    string             `json:"qualified_skill"`
	PackID            string             `json:"pack_id"`
	PackVersion       string             `json:"pack_version"`
	Definition        ResourceDefinition `json:"definition"`
	SupportedSurfaces []Surface          `json:"supported_surfaces"`
	NativeName        string             `json:"native_name"`
	Exclusion         *SurfaceExclusion  `json:"exclusion"`
	PluginExports     PluginExports      `json:"plugin_exports,omitempty"`
	OnDemand          *OnDemandSkill     `json:"on_demand"`
	Compatible        bool               `json:"compatible"`
	Reason            string             `json:"reason"`
	Prerequisites     []string           `json:"prerequisites"`
}

// SkillDiscoveryReport contains metadata only; no resources are prepared and no
// prerequisites or host runtimes are observed.
type SkillDiscoveryReport struct {
	PackID                string           `json:"pack_id"`
	PackVersion           string           `json:"pack_version"`
	Surface               Surface          `json:"surface"`
	Skills                []SkillDiscovery `json:"skills"`
	PrerequisitesVerified bool             `json:"prerequisites_verified"`
	Limitation            string           `json:"limitation"`
}

// DiscoverSkills inspects only the current local catalog. An installed or
// retained Pack never substitutes for a current candidate.
func (c Catalog) DiscoverSkills(ctx context.Context, id string, surface Surface) (SkillDiscoveryReport, error) {
	if surface != SurfaceAgents && surface != SurfaceClaude {
		return SkillDiscoveryReport{}, fmt.Errorf("unsupported on-demand discovery surface %q; use --surface agents or --surface claude", surface)
	}
	packs, err := c.ListCurrent(ctx)
	if err != nil {
		return SkillDiscoveryReport{}, err
	}
	for _, pack := range packs {
		if pack.ID != id {
			continue
		}
		effective := ResolvePackForSurface(pack, surface)
		report := SkillDiscoveryReport{PackID: pack.ID, PackVersion: pack.Version, Surface: surface, Skills: []SkillDiscovery{}, Limitation: "Prerequisites are known declarations, not verified; unknown requirements may exist. Compatibility describes resource access, not native registration, runtime readiness, or workflow success."}
		inventory := descriptiveResourceInventory(effective)
		for _, description := range inventory {
			if description.Resource.Kind != "skill" {
				continue
			}
			resource := effective.Resources[slices.IndexFunc(effective.Resources, func(r Resource) bool { return r.Kind == "skill" && r.ID == description.Resource.ID })]
			definition := "common"
			if resource.ResolvedVariant != "" {
				definition = "surface_variant"
			}
			candidate := SkillDiscovery{DescriptiveResource: description, QualifiedSkill: pack.ID + ":" + resource.ID, PackID: pack.ID, PackVersion: pack.Version,
				Definition:        ResourceDefinition{Resource: description.Resource, Surface: surface, Definition: definition, Source: resource.Source, Origin: resource.Origin, Notices: description.Notices},
				SupportedSurfaces: []Surface{}, PluginExports: clonePluginExports(resource.PluginExports), OnDemand: cloneOnDemand(resource.OnDemand), Prerequisites: DeclaredSkillPrerequisites(pack, resource)}
			for _, binding := range resource.Bindings {
				candidate.SupportedSurfaces = append(candidate.SupportedSurfaces, binding.Surface)
				if binding.Surface == surface {
					candidate.NativeName = binding.Name
				}
			}
			slices.Sort(candidate.SupportedSurfaces)
			if exclusion, ok := exclusionFor(resource, surface); ok {
				candidate.Exclusion = &exclusion
			}
			_, accessErr := ResolveOnDemand(packs, candidate.QualifiedSkill, surface)
			candidate.Compatible = accessErr == nil
			if accessErr != nil {
				candidate.Reason = accessErr.Error()
				if candidate.Exclusion != nil {
					candidate.Reason += ": " + candidate.Exclusion.Reason
				}
			}
			report.Skills = append(report.Skills, candidate)
		}
		return report, nil
	}
	return SkillDiscoveryReport{}, fmt.Errorf("unknown current capability pack %q; run `packy list` to see available packs", id)
}

// DeclaredSkillPrerequisites combines known skill and Pack declarations, without
// claiming that an empty list establishes absence of unknown requirements.
func DeclaredSkillPrerequisites(pack Pack, resource Resource) []string {
	result := append([]string{}, pack.Requires.Tools...)
	if resource.OnDemand != nil {
		result = append(result, resource.OnDemand.Prerequisites...)
	}
	slices.SortFunc(result, strings.Compare)
	return slices.Compact(result)
}
