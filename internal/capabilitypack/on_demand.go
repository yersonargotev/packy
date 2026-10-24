package capabilitypack

import (
	"fmt"
	"slices"
	"strings"
)

// OnDemandSkill is a reviewed file-tree compatibility contract, independent of
// native installation. Prerequisites are declarations, never runtime evidence.
type OnDemandSkill struct {
	Protocol      string   `json:"protocol"`
	Prerequisites []string `json:"prerequisites"`
}

func cloneOnDemand(value *OnDemandSkill) *OnDemandSkill {
	if value == nil {
		return nil
	}
	result := *value
	result.Prerequisites = slices.Clone(value.Prerequisites)
	return &result
}

func validateOnDemand(resource Resource) error {
	contract := resource.OnDemand
	if contract == nil {
		return nil
	}
	if resource.Kind != "skill" {
		return fmt.Errorf("on_demand is only applicable to skill resources")
	}
	if contract.Protocol != "skill-tree-v1" {
		return fmt.Errorf("unsupported on_demand protocol %q; expected skill-tree-v1", contract.Protocol)
	}
	if contract.Prerequisites == nil || !sortedPortableSet(contract.Prerequisites, func(value string) bool {
		return strings.TrimSpace(value) == value && value != ""
	}) {
		return fmt.Errorf("on_demand prerequisites must be a non-null sorted set of nonempty declarations")
	}
	return nil
}
