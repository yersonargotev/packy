package capabilitypack

import (
	"fmt"
	"sort"
)

// ResourceDefinition explains the reviewed body selected for one logical resource.
// It contains catalog-relative provenance, never installed paths or runtime evidence.
type ResourceDefinition struct {
	Resource   ResourceIdentity   `json:"resource"`
	Surface    Surface            `json:"surface"`
	Definition string             `json:"definition"`
	Source     string             `json:"source,omitempty"`
	Origin     *ResourceOrigin    `json:"origin,omitempty"`
	Notices    []ResourceIdentity `json:"notices"`
}

// Summary is the shared human explanation for CLI and interactive inspection.
func (definition ResourceDefinition) Summary() string {
	kind := "common definition"
	if definition.Definition == "surface_variant" {
		kind = "surface variant"
	}
	text := fmt.Sprintf("%s on %s: %s", definition.Resource, definition.Surface, kind)
	if definition.Source != "" {
		text += "; source=" + definition.Source
	}
	if definition.Origin != nil {
		text += fmt.Sprintf("; origin=%s:%s (%s)", definition.Origin.ID, definition.Origin.Path, definition.Origin.Relationship)
	}
	return text
}

// ResourceDefinitionsFor describes applicable bodies using the same resolver as
// planning. Exclusions remain visible in the lifecycle contract, not as selections.
func ResourceDefinitionsFor(pack Pack, surface Surface) []ResourceDefinition {
	result := make([]ResourceDefinition, 0, len(pack.Resources))
	for _, common := range pack.Resources {
		if _, excluded := exclusionFor(common, surface); excluded {
			continue
		}
		if common.Kind != "asset" && common.Kind != "notice" && !resourceHasSurfaceBinding(common, surface) {
			continue
		}
		resource := ResolveResourceForSurface(common, surface)
		kind := "common"
		if resource.ResolvedVariant != "" {
			kind = "surface_variant"
		}
		notices := resourceIdentities(resource.Notices)
		sort.Slice(notices, func(i, j int) bool { return notices[i].String() < notices[j].String() })
		result = append(result, ResourceDefinition{Resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, Surface: surface, Definition: kind, Source: resource.Source, Origin: resource.Origin, Notices: notices})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Resource.String() < result[j].Resource.String() })
	return result
}

func definitionsInGraph(definitions []ResourceDefinition, graph ResourceGraph) []ResourceDefinition {
	selected := make(map[ResourceIdentity]bool, len(graph.Resources))
	for _, fact := range graph.Resources {
		selected[fact.Resource] = true
	}
	result := make([]ResourceDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if selected[definition.Resource] {
			result = append(result, definition)
		}
	}
	return result
}
