package cataloglayout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

// ResourceBody identifies a reviewed common or surface-specific definition.
// Surface is empty for the common definition; logical resource IDs stay intact.
type ResourceBody struct {
	Resource
	Surface capabilitypack.Surface
}

// DeclaredResourceBodies enumerates the common definition and every declared
// surface adaptation using the catalog's shared effective-resource resolver.
func DeclaredResourceBodies(resource Resource) []ResourceBody {
	var result []ResourceBody
	for i, body := range declaredBodies([]Resource{resource}) {
		surface := capabilitypack.Surface("")
		if i > 0 {
			surface = resource.Variants[i-1].Surface
		}
		result = append(result, ResourceBody{Resource: body, Surface: surface})
	}
	return result
}

// validateNoticeVariants preserves the complete reviewed original notice. Legal
// text is not interpreted or shortened automatically; adaptations may add text.
func validateNoticeVariants(root string, resources []Resource) error {
	for _, resource := range resources {
		if resource.Kind != "notice" || len(resource.Variants) == 0 {
			continue
		}
		original, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(resource.Source)))
		if err != nil {
			return err
		}
		for _, body := range DeclaredResourceBodies(resource)[1:] {
			content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(body.Source)))
			if err != nil {
				return err
			}
			if len(bytes.TrimSpace(original)) == 0 || !bytes.Contains(content, original) {
				return fmt.Errorf("notice %q variant %q must preserve complete common notice text", resource.ID, body.Surface)
			}
			if body.License != resource.License || !strings.Contains(body.Attribution, resource.Attribution) {
				return fmt.Errorf("notice %q variant %q must preserve common license and attribution", resource.ID, body.Surface)
			}
		}
	}
	return nil
}

func declaredBodies(resources []Resource) []Resource {
	var bodies []Resource
	for _, resource := range resources {
		common := capabilitypack.Resource{
			Description: resource.Description,
			Source:      resource.Source,
			Origin:      resource.Origin,
			Notices:     resource.Notices,
			Requires:    resource.Requires,
			Conflicts:   resource.Conflicts,
			Command:     resource.Command,
			Args:        resource.Args,
			Mode:        resource.Mode,
			Tools:       resource.Tools,
			Permissions: resource.Permissions,
			Arguments:   resource.Arguments,
			License:     resource.License,
			Attribution: resource.Attribution,
			Kind:        resource.Kind, ID: resource.ID, Variants: resource.Variants,
		}
		for _, effective := range capabilitypack.DeclaredResourceBodies(common) {
			body := resource
			body.Variants = nil
			body.Description = effective.Description
			body.Source = effective.Source
			body.Origin = effective.Origin
			body.Notices = effective.Notices
			body.Requires = effective.Requires
			body.Conflicts = effective.Conflicts
			body.Command = effective.Command
			body.Args = effective.Args
			body.Mode = effective.Mode
			body.Tools = effective.Tools
			body.Permissions = effective.Permissions
			body.Arguments = effective.Arguments
			body.License = effective.License
			body.Attribution = effective.Attribution
			bodies = append(bodies, body)
		}
	}
	return bodies
}

// MarshalJSON retains explicit empty required typed arrays during authoring
// round trips without emitting inapplicable fields for other resource kinds.
func (resource Resource) MarshalJSON() ([]byte, error) {
	type wire Resource
	value := struct {
		wire
		Tools       *[]string `json:"tools,omitempty"`
		Permissions *[]string `json:"permissions,omitempty"`
		Args        *[]string `json:"args,omitempty"`
	}{wire: wire(resource)}
	if resource.Kind == "agent" {
		value.Tools = &resource.Tools
		value.Permissions = &resource.Permissions
	}
	if resource.Kind == "mcp_server" {
		value.Args = &resource.Args
	}
	return json.Marshal(value)
}
