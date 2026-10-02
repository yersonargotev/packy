package cataloglayout

import "github.com/yersonargotev/packy/internal/capabilitypack"

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
