package capabilitypack

import (
	"bytes"
	"encoding/json"
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

// DecodeOnDemandSkill preserves the distinction between an omitted contract
// and an explicitly invalid null declaration at both manifest boundaries.
func DecodeOnDemandSkill(data json.RawMessage) (*OnDemandSkill, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("on_demand must be an object, not null")
	}
	var value OnDemandSkill
	if err := strictDecode(data, &value); err != nil {
		return nil, fmt.Errorf("decode on_demand: %w", err)
	}
	return &value, nil
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
