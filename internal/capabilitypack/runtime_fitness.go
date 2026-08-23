package capabilitypack

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeFitnessMatrix describes every supported runtime selection without
// invoking a surface adapter or reading resource content.
type RuntimeFitnessMatrix struct {
	Rows []RuntimeFitnessRow `json:"rows"`
}

// RuntimeFitnessRow records one declared surface and selection outcome.
type RuntimeFitnessRow struct {
	Surface      Surface               `json:"surface"`
	Selection    ResourceSelection     `json:"selection"`
	Availability SelectionAvailability `json:"availability"`
	Resources    []ResourceIdentity    `json:"resources"`
	Projections  []RuntimeProjection   `json:"projections"`
}

// RuntimeProjection identifies one host-native name claimed by a selected
// resource binding.
type RuntimeProjection struct {
	Resource   ResourceIdentity `json:"resource"`
	Projection string           `json:"projection"`
	Name       string           `json:"name"`
}

// EvaluateRuntimeFitness returns the deterministic selection and projection
// matrix implied by a Pack manifest. It is intentionally pure: adapter
// execution and resource content are outside this contract.
func EvaluateRuntimeFitness(pack Pack) (RuntimeFitnessMatrix, error) {
	result := RuntimeFitnessMatrix{Rows: []RuntimeFitnessRow{}}
	surfaces := sortedFitnessSurfaces(pack.Surfaces)
	roots := operationalFitnessRoots(pack)

	for _, surface := range surfaces {
		all := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
		row, err := evaluateRuntimeFitnessRow(pack, surface, all)
		if err != nil {
			return RuntimeFitnessMatrix{}, err
		}
		result.Rows = append(result.Rows, row)

		for _, root := range roots {
			selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{root}}
			row, err := evaluateRuntimeFitnessRow(pack, surface, selection)
			if err != nil {
				return RuntimeFitnessMatrix{}, err
			}
			result.Rows = append(result.Rows, row)
		}
	}
	return result, nil
}

func evaluateRuntimeFitnessRow(pack Pack, surface Surface, selection ResourceSelection) (RuntimeFitnessRow, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return RuntimeFitnessRow{}, err
	}
	availability, err := selectionAvailability(pack, selection, surface)
	if err != nil {
		return RuntimeFitnessRow{}, fmt.Errorf("evaluate runtime fitness for %s selection %s: %w", surface, fitnessSelectionName(selection), err)
	}
	if !availability.Available {
		if validUnavailableFitnessRow(selection, availability) {
			return RuntimeFitnessRow{
				Surface: surface, Selection: selection, Availability: availability,
				Resources: []ResourceIdentity{}, Projections: []RuntimeProjection{},
			}, nil
		}
		return RuntimeFitnessRow{}, fmt.Errorf(
			"evaluate runtime fitness for %s selection %s: unavailable: %s",
			surface, fitnessSelectionName(selection), renderFitnessReasons(availability.Reasons),
		)
	}
	selected, err := selectPackResourcesForSurface(pack, selection, surface)
	if err != nil {
		return RuntimeFitnessRow{}, fmt.Errorf("evaluate runtime fitness for %s selection %s: %w", surface, fitnessSelectionName(selection), err)
	}

	resources := make([]ResourceIdentity, 0, len(selected.Resources))
	projections := []RuntimeProjection{}
	for _, resource := range selected.Resources {
		identity := identityFromResource(resource)
		resources = append(resources, identity)
		for _, binding := range resource.Bindings {
			if binding.Surface == surface {
				projections = append(projections, RuntimeProjection{Resource: identity, Projection: binding.Projection, Name: binding.Name})
			}
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].String() < resources[j].String() })
	sort.Slice(projections, func(i, j int) bool {
		if projections[i].Resource != projections[j].Resource {
			return projections[i].Resource.String() < projections[j].Resource.String()
		}
		if projections[i].Projection != projections[j].Projection {
			return projections[i].Projection < projections[j].Projection
		}
		return projections[i].Name < projections[j].Name
	})
	if err := rejectRuntimeProjectionCollisions(surface, selection, projections); err != nil {
		return RuntimeFitnessRow{}, err
	}
	return RuntimeFitnessRow{Surface: surface, Selection: selection, Availability: availability, Resources: resources, Projections: projections}, nil
}

func validUnavailableFitnessRow(selection ResourceSelection, availability SelectionAvailability) bool {
	if selection.Mode != SelectionCustom || len(selection.Roots) != 1 || len(availability.Reasons) == 0 {
		return false
	}
	root := selection.Roots[0]
	for _, reason := range availability.Reasons {
		if reason.Code != SelectionReasonRootExcluded || reason.Resource != root {
			return false
		}
	}
	return true
}

func rejectRuntimeProjectionCollisions(surface Surface, selection ResourceSelection, projections []RuntimeProjection) error {
	owners := map[string]ResourceIdentity{}
	for _, projection := range projections {
		key := projection.Projection + "+" + projection.Name
		if owner, exists := owners[key]; exists && owner != projection.Resource {
			left, right := owner.String(), projection.Resource.String()
			if right < left {
				left, right = right, left
			}
			return fmt.Errorf("runtime projection collision on %s selection %s at %s between %s and %s", surface, fitnessSelectionName(selection), key, left, right)
		}
		owners[key] = projection.Resource
	}
	return nil
}

func sortedFitnessSurfaces(values []Surface) []Surface {
	set := make(map[Surface]bool, len(values))
	for _, surface := range values {
		set[surface] = true
	}
	result := make([]Surface, 0, len(set))
	for surface := range set {
		result = append(result, surface)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func operationalFitnessRoots(pack Pack) []ResourceIdentity {
	result := make([]ResourceIdentity, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		if resource.Kind != "asset" && resource.Kind != "notice" {
			result = append(result, identityFromResource(resource))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	return result
}

func fitnessSelectionName(selection ResourceSelection) string {
	if selection.Mode == SelectionAll {
		return string(SelectionAll)
	}
	values := make([]string, len(selection.Roots))
	for i, root := range selection.Roots {
		values[i] = root.String()
	}
	return strings.Join(values, ",")
}

func renderFitnessReasons(reasons []SelectionValidityReason) string {
	values := make([]string, len(reasons))
	for i, reason := range reasons {
		values[i] = string(reason.Code) + ": " + reason.Detail
	}
	return strings.Join(values, "; ")
}
