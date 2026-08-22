package repositorycandidate

import (
	"errors"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestManagedSemVerFloorRequiresMajorForAnAddedExternalRequirement(t *testing.T) {
	current := semverManifest("1.0.0")
	candidate := semverManifest("1.1.0")
	candidate.ExternalRequirements = []string{"helper-cli"}

	requireCompatibilityFloorRejection(t, enforceVersionFloor(current, candidate))
}

func TestManagedSemVerFloorRequiresMajorForAnAddedResourceGraphConstraint(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*managedpack.Resource)
	}{
		{name: "requirement", mutate: func(resource *managedpack.Resource) { resource.Requires = []string{"asset:shared"} }},
		{name: "conflict", mutate: func(resource *managedpack.Resource) { resource.Conflicts = []string{"skill:other"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := semverManifest("1.0.0")
			candidate := semverManifest("1.1.0")
			test.mutate(&candidate.Resources[0])

			requireCompatibilityFloorRejection(t, enforceVersionFloor(current, candidate))
		})
	}
}

func TestManagedSemVerFloorDistinguishesIsolatedAndMandatoryNewResources(t *testing.T) {
	t.Run("isolated resource accepts minor", func(t *testing.T) {
		current := semverManifest("1.0.0")
		candidate := semverManifest("1.1.0")
		candidate.Resources = append(candidate.Resources, semverResource("asset", "shared"))

		if err := enforceVersionFloor(current, candidate); err != nil {
			t.Fatalf("isolated resource with minor increment: %v", err)
		}
	})

	t.Run("isolated resource rejects patch", func(t *testing.T) {
		current := semverManifest("1.0.0")
		candidate := semverManifest("1.0.1")
		candidate.Resources = append(candidate.Resources, semverResource("asset", "shared"))

		requireCompatibilityFloorRejection(t, enforceVersionFloor(current, candidate))
	})

	for _, test := range []struct {
		name   string
		mutate func(*managedpack.Resource)
	}{
		{name: "requirements", mutate: func(resource *managedpack.Resource) { resource.Requires = []string{"asset:shared"} }},
		{name: "conflicts", mutate: func(resource *managedpack.Resource) { resource.Conflicts = []string{"skill:other"} }},
		{name: "tools", mutate: func(resource *managedpack.Resource) { resource.Tools = []string{"Read"} }},
		{name: "permissions", mutate: func(resource *managedpack.Resource) { resource.Permissions = []string{"read"} }},
		{name: "surface capabilities", mutate: func(resource *managedpack.Resource) {
			resource.Bindings = []capabilitypack.Binding{{
				Surface: capabilitypack.SurfaceCodex,
				Capabilities: []capabilitypack.SurfaceCapability{{
					Type: capabilitypack.SurfaceCapabilityProjectInstruction,
					ProjectInstruction: &capabilitypack.ProjectInstructionCapability{
						ID: "shared", Source: "instructions/shared.md",
					},
				}},
			}}
		}},
	} {
		t.Run("mandatory "+test.name+" rejects minor", func(t *testing.T) {
			current := semverManifest("1.0.0")
			candidate := semverManifest("1.1.0")
			added := semverResource("asset", "shared")
			test.mutate(&added)
			candidate.Resources = append(candidate.Resources, added)

			requireCompatibilityFloorRejection(t, enforceVersionFloor(current, candidate))
		})
	}
}

func TestManagedSemVerFloorRequiresMajorForAnExistingBindingProjectionChange(t *testing.T) {
	binding := capabilitypack.Binding{
		Surface: capabilitypack.SurfaceCodex, Projection: "skill", Name: "guide", Invocation: "guide", Mode: "native", Sharing: "exclusive",
	}
	t.Run("binding added", func(t *testing.T) {
		current := semverManifest("1.0.0")
		candidate := semverManifest("1.1.0")
		candidate.Resources[0].Bindings = []capabilitypack.Binding{binding}

		requireCompatibilityFloorRejection(t, enforceVersionFloor(current, candidate))
	})

	t.Run("projection changed", func(t *testing.T) {
		current := semverManifest("1.0.0")
		current.Resources[0].Bindings = []capabilitypack.Binding{binding}
		candidate := semverManifest("1.1.0")
		candidate.Resources[0].Bindings = []capabilitypack.Binding{binding}
		candidate.Resources[0].Bindings[0].Projection = "command"

		requireCompatibilityFloorRejection(t, enforceVersionFloor(current, candidate))
	})
}

func TestManagedSemVerFloorAllowsMetadataAndProvenanceOnlyPatch(t *testing.T) {
	current := semverManifest("1.0.0")
	candidate := semverManifest("1.0.1")
	candidate.Resources[0].Description = "Updated guidance"
	candidate.Resources[0].Notices = []string{"notice:terms"}
	candidate.Resources[0].Origin = &managedpack.ResourceOrigin{
		ID: "upstream", Path: "guide", Relationship: managedpack.RelationshipAdapted,
	}

	if err := enforceVersionFloor(current, candidate); err != nil {
		t.Fatalf("metadata-only patch: %v", err)
	}
}

func semverManifest(version string) managedpack.Manifest {
	return managedpack.Manifest{
		SchemaVersion: 1,
		ID:            "example",
		Version:       version,
		Selectable:    true,
		Surfaces:      []capabilitypack.Surface{capabilitypack.SurfaceCodex},
		Resources:     []managedpack.Resource{semverResource("skill", "guide")},
	}
}

func semverResource(kind, id string) managedpack.Resource {
	return managedpack.Resource{
		Kind: kind, ID: id, Description: id,
		Requires: []string{}, Conflicts: []string{}, Bindings: []capabilitypack.Binding{}, SurfaceExclusions: []capabilitypack.SurfaceExclusion{},
	}
}

func requireCompatibilityFloorRejection(t *testing.T, err error) {
	t.Helper()
	var rejection *managedpackpromotion.RejectionError
	if !errors.As(err, &rejection) {
		t.Fatalf("error = %v, want typed compatibility-floor rejection", err)
	}
	if rejection.Gate != managedpackpromotion.GateCompatibilityFloor {
		t.Fatalf("rejection gate = %s, want %s", rejection.Gate, managedpackpromotion.GateCompatibilityFloor)
	}
}
