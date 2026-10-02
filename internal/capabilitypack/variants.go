package capabilitypack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ResourceOrigin identifies the reviewed relationship of one resource body.
type ResourceOrigin struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Relationship string `json:"relationship"`
}

// ResourceVariants is an optional non-null collection of reviewed overrides.
type ResourceVariants []ResourceVariant

func (v *ResourceVariants) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("variants must not be null")
	}
	var values []ResourceVariant
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*v = values
	return nil
}

// ResourceVariant replaces explicitly present typed fields for one surface.
// Pointer fields distinguish inheritance from intentional empty values.
type ResourceVariant struct {
	Surface     Surface           `json:"surface"`
	Description *string           `json:"description,omitempty"`
	Source      *string           `json:"source,omitempty"`
	Origin      *ResourceOrigin   `json:"origin,omitempty"`
	Notices     *[]string         `json:"notices,omitempty"`
	Requires    *[]string         `json:"requires,omitempty"`
	Conflicts   *[]string         `json:"conflicts,omitempty"`
	Command     *string           `json:"command,omitempty"`
	Args        *[]string         `json:"args,omitempty"`
	Mode        *string           `json:"mode,omitempty"`
	Tools       *[]string         `json:"tools,omitempty"`
	Permissions *[]string         `json:"permissions,omitempty"`
	Arguments   *CommandArguments `json:"arguments,omitempty"`
	License     *string           `json:"license,omitempty"`
	Attribution *string           `json:"attribution,omitempty"`
}

func (v *ResourceVariant) UnmarshalJSON(data []byte) error {
	type wire ResourceVariant
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("variant must be an object")
	}
	for key, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("variant field %q must not be null", key)
		}
	}
	var decoded wire
	if err := strictDecode(data, &decoded); err != nil {
		return err
	}
	*v = ResourceVariant(decoded)
	return nil
}

func cloneVariants(values []ResourceVariant) []ResourceVariant {
	if values == nil {
		return nil
	}
	result := append([]ResourceVariant{}, values...)
	for i := range result {
		v := &result[i]
		if v.Description != nil {
			value := *v.Description
			v.Description = &value
		}
		if v.Source != nil {
			value := *v.Source
			v.Source = &value
		}
		if v.Origin != nil {
			value := *v.Origin
			v.Origin = &value
		}
		if v.Notices != nil {
			value := *v.Notices
			value = append([]string{}, value...)
			v.Notices = &value
		}
		if v.Requires != nil {
			value := *v.Requires
			value = append([]string{}, value...)
			v.Requires = &value
		}
		if v.Conflicts != nil {
			value := *v.Conflicts
			value = append([]string{}, value...)
			v.Conflicts = &value
		}
		if v.Command != nil {
			value := *v.Command
			v.Command = &value
		}
		if v.Args != nil {
			value := *v.Args
			value = append([]string{}, value...)
			v.Args = &value
		}
		if v.Mode != nil {
			value := *v.Mode
			v.Mode = &value
		}
		if v.Tools != nil {
			value := *v.Tools
			value = append([]string{}, value...)
			v.Tools = &value
		}
		if v.Permissions != nil {
			value := *v.Permissions
			value = append([]string{}, value...)
			v.Permissions = &value
		}
		if v.Arguments != nil {
			value := *v.Arguments
			v.Arguments = &value
		}
		if v.License != nil {
			value := *v.License
			v.License = &value
		}
		if v.Attribution != nil {
			value := *v.Attribution
			v.Attribution = &value
		}
	}
	return result
}

// ResolveResourceForSurface returns an independent effective resource. It never
// changes logical identity, bindings, support, or the retained common catalog.
func ResolveResourceForSurface(resource Resource, surface Surface) Resource {
	result := clonePack(Pack{Resources: []Resource{resource}}).Resources[0]
	for _, variant := range result.Variants {
		if variant.Surface != surface {
			continue
		}
		if variant.Description != nil {
			result.Description = *variant.Description
		}
		if variant.Source != nil {
			result.Source = *variant.Source
		}
		if variant.Origin != nil {
			result.Origin = variant.Origin
		}
		if variant.Notices != nil {
			result.Notices = *variant.Notices
		}
		if variant.Requires != nil {
			result.Requires = *variant.Requires
		}
		if variant.Conflicts != nil {
			result.Conflicts = *variant.Conflicts
		}
		if variant.Command != nil {
			result.Command = *variant.Command
		}
		if variant.Args != nil {
			result.Args = *variant.Args
		}
		if variant.Mode != nil {
			result.Mode = *variant.Mode
		}
		if variant.Tools != nil {
			result.Tools = *variant.Tools
		}
		if variant.Permissions != nil {
			result.Permissions = *variant.Permissions
		}
		if variant.Arguments != nil {
			result.Arguments = *variant.Arguments
		}
		if variant.License != nil {
			result.License = *variant.License
		}
		if variant.Attribution != nil {
			result.Attribution = *variant.Attribution
		}
		result.ResolvedVariant = surface
		break
	}
	result.Variants = nil
	return result
}

// ResolvePackForSurface resolves all logical resources in the same host context
// before dependencies or projections are selected.
func ResolvePackForSurface(pack Pack, surface Surface) Pack {
	result := clonePack(pack)
	for i, resource := range pack.Resources {
		result.Resources[i] = ResolveResourceForSurface(resource, surface)
	}
	return result
}

// DeclaredResourceBodies includes the common body and every reviewed variant.
func DeclaredResourceBodies(resource Resource) []Resource {
	common := ResolveResourceForSurface(resource, "")
	bodies := []Resource{common}
	for _, variant := range resource.Variants {
		bodies = append(bodies, ResolveResourceForSurface(resource, variant.Surface))
	}
	return bodies
}

func validateVariants(pack Pack) error {
	for _, resource := range pack.Resources {
		for i, variant := range resource.Variants {
			identity := resource.Kind + ":" + resource.ID
			if !containsSurface(pack.Surfaces, variant.Surface) {
				return fmt.Errorf("resource %q variant has unknown or undeclared surface %q", identity, variant.Surface)
			}
			if i > 0 && resource.Variants[i-1].Surface >= variant.Surface {
				return fmt.Errorf("resource %q variants must be sorted by surface without duplicates", identity)
			}
			if _, excluded := exclusionFor(resource, variant.Surface); excluded {
				return fmt.Errorf("resource %q variant surface %q is excluded", identity, variant.Surface)
			}
			if resource.Kind != "asset" && resource.Kind != "notice" {
				supported := false
				for _, binding := range resource.Bindings {
					supported = supported || binding.Surface == variant.Surface
				}
				if !supported {
					return fmt.Errorf("resource %q variant surface %q has no binding", identity, variant.Surface)
				}
			}
			if variant.Source != nil && *variant.Source != resource.Source && resource.Origin != nil && variant.Origin == nil {
				return fmt.Errorf("resource %q variant %q changes imported source without explicit origin", identity, variant.Surface)
			}
			if variant.Command != nil && resource.Kind != "mcp_server" {
				return fmt.Errorf("resource %q variant command is only applicable to mcp_server", identity)
			}
			if variant.Args != nil && resource.Kind != "mcp_server" {
				return fmt.Errorf("resource %q variant args are only applicable to mcp_server", identity)
			}
			if (variant.Mode != nil || variant.Tools != nil || variant.Permissions != nil) && resource.Kind != "agent" {
				return fmt.Errorf("resource %q variant authority fields are only applicable to agent", identity)
			}
			if variant.Arguments != nil && resource.Kind != "command" {
				return fmt.Errorf("resource %q variant arguments are only applicable to command", identity)
			}
			if (variant.License != nil || variant.Attribution != nil) && resource.Kind != "notice" {
				return fmt.Errorf("resource %q variant legal fields are only applicable to notice", identity)
			}
			if (variant.Source != nil || variant.Origin != nil) && (resource.Kind == "mcp_server" || resource.Kind == "lifecycle") {
				return fmt.Errorf("resource %q variant cannot have source or origin", identity)
			}
			effective := ResolveResourceForSurface(resource, variant.Surface)
			if strings.TrimSpace(effective.Description) == "" {
				return fmt.Errorf("resource %q variant %q description is required", identity, variant.Surface)
			}

			effective = resourceForSurfaceValidation(effective, variant.Surface)
			if err := validateResourceV3(effective, []Surface{variant.Surface}, nil); err != nil {
				return fmt.Errorf("resource %q variant %q: %w", identity, variant.Surface, err)
			}
		}
	}
	for _, surface := range pack.Surfaces {
		effective := ResolvePackForSurface(pack, surface)
		identities := map[string]bool{}
		for _, resource := range effective.Resources {
			identities[resource.Kind+":"+resource.ID] = true
		}
		validation := clonePack(effective)
		for i, resource := range validation.Resources {
			validation.Resources[i] = resourceForSurfaceValidation(resource, surface)
		}
		if err := validateClaudeCompositionCapabilities(validation, identities); err != nil {
			return fmt.Errorf("surface %q effective capability graph: %w", surface, err)
		}
		effective = withSurfaceCapabilityDependencies(effective, surface)
		if err := validateDependencies(effective.Resources, identities); err != nil {
			return fmt.Errorf("surface %q effective graph: %w", surface, err)
		}
		if err := validateResourceConflicts(effective.Resources, identities); err != nil {
			return fmt.Errorf("surface %q effective graph: %w", surface, err)
		}
		for _, resource := range effective.Resources {
			if _, excluded := exclusionFor(resource, surface); excluded {
				continue
			}
			for _, dependency := range append(append([]string{}, resource.Requires...), resource.Notices...) {
				for _, target := range effective.Resources {
					if target.Kind+":"+target.ID == dependency {
						if _, excluded := exclusionFor(target, surface); excluded {
							return fmt.Errorf("surface %q resource %q dependency %q is excluded", surface, resource.Kind+":"+resource.ID, dependency)
						}
					}
				}
			}
		}
	}
	return nil
}

func resourceForSurfaceValidation(resource Resource, surface Surface) Resource {
	bindings := []Binding{}
	for _, binding := range resource.Bindings {
		if binding.Surface == surface {
			bindings = append(bindings, binding)
		}
	}
	exclusions := []SurfaceExclusion{}
	for _, exclusion := range resource.SurfaceExclusions {
		if exclusion.Surface == surface {
			exclusions = append(exclusions, exclusion)
		}
	}
	resource.Bindings, resource.SurfaceExclusions = bindings, exclusions
	return resource
}
