package capabilitypack

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCheckedInPstackPackPreservesCompatibilityMatrix(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	catalog, err := Discover(context.Background(), bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show(context.Background(), "pstack")
	if err != nil {
		t.Fatal(err)
	}
	if pack.manifestVersion != manifestSchemaV4 || !pack.Selectable {
		t.Fatalf("pstack identity = version %q schema %d selectable %v", pack.Version, pack.manifestVersion, pack.Selectable)
	}
	if got, want := pack.Surfaces, []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pstack surfaces = %v want %v", got, want)
	}
	if counts := pack.ResourceCounts(); counts.Skills != 26 || counts.Notices != 1 || len(pack.Resources) != 27 {
		t.Fatalf("pstack resource counts = %+v total=%d", counts, len(pack.Resources))
	}

	for _, resource := range pack.Resources {
		identity := (ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()
		if resource.Kind == "notice" {
			if identity != "notice:pstack-mit" || resource.License != "MIT" || resource.Attribution != "Copyright (c) 2026 Lauren Tan" {
				t.Fatalf("pstack notice = %+v", resource)
			}
			continue
		}
		if resource.Kind != "skill" || !reflect.DeepEqual(bindingSurfaces(resource.Bindings), pack.Surfaces) ||
			!reflect.DeepEqual(resource.Notices, []string{"notice:pstack-mit"}) || len(resource.Conflicts) != 0 || len(resource.SurfaceExclusions) != 0 {
			t.Fatalf("pstack skill %q contract = %+v", identity, resource)
		}
	}

	for _, surface := range pack.Surfaces {
		assertPstackSelectionTargets(t, pack, surface, ResourceSelection{Mode: SelectionAll})
		for _, resource := range pack.Resources {
			if resource.Kind != "skill" {
				continue
			}
			assertPstackSelectionTargets(t, pack, surface, ResourceSelection{
				Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: resource.Kind, ID: resource.ID}},
			})
		}
	}
}

func assertPstackSelectionTargets(t *testing.T, pack Pack, surface Surface, selection ResourceSelection) {
	t.Helper()
	selected, err := selectPackResources(pack, selection)
	if err != nil {
		t.Fatalf("select pstack %s roots %+v: %v", surface, selection.Roots, err)
	}
	targets := map[string]string{}
	for _, resource := range selected.Resources {
		for _, binding := range resource.Bindings {
			if binding.Surface != surface {
				continue
			}
			if previous := targets[binding.Name]; previous != "" {
				t.Fatalf("pstack %s selection has target collision %q between %s and %s", surface, binding.Name, previous, resource.ID)
			}
			targets[binding.Name] = resource.ID
		}
	}
}

func bindingSurfaces(bindings []Binding) []Surface {
	result := make([]Surface, len(bindings))
	for i, binding := range bindings {
		result[i] = binding.Surface
	}
	return result
}
