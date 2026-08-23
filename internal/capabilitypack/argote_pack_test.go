package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCheckedInArgotePackHasCollisionFreeNativeRoots(t *testing.T) {
	catalog, err := Discover(context.Background(), filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show(context.Background(), "argote")
	if err != nil {
		t.Fatal(err)
	}
	if pack.manifestVersion != manifestSchemaV4 {
		t.Fatalf("argote identity = version %q schema %d", pack.Version, pack.manifestVersion)
	}
	if got, want := pack.Surfaces, []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}; !reflect.DeepEqual(got, want) {
		t.Fatalf("argote surfaces = %v want %v", got, want)
	}

	wantRoots := []string{"instruction:guidance", "skill:espera-que"}
	if len(pack.Resources) != len(wantRoots) {
		t.Fatalf("argote resources = %+v", pack.Resources)
	}
	for i, resource := range pack.Resources {
		identity := (ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()
		if identity != wantRoots[i] {
			t.Fatalf("argote resource %d = %q want %q", i, identity, wantRoots[i])
		}
		if len(resource.Requires) != 0 || len(resource.SurfaceExclusions) != 0 {
			t.Fatalf("argote root %q is not independent: requires=%v exclusions=%v", identity, resource.Requires, resource.SurfaceExclusions)
		}
		if len(resource.Bindings) != len(pack.Surfaces) {
			t.Fatalf("argote root %q bindings = %+v", identity, resource.Bindings)
		}
		for j, binding := range resource.Bindings {
			if binding.Surface != pack.Surfaces[j] || binding.Mode != "native" {
				t.Fatalf("argote root %q binding %d = %+v", identity, j, binding)
			}
		}

		selected, selectErr := selectPackResources(pack, ResourceSelection{
			Mode:  SelectionCustom,
			Roots: []ResourceIdentity{{Kind: resource.Kind, ID: resource.ID}},
		})
		if selectErr != nil {
			t.Fatalf("select argote root %q: %v", identity, selectErr)
		}
		if len(selected.Resources) != 1 || selected.Resources[0].Kind != resource.Kind || selected.Resources[0].ID != resource.ID {
			t.Fatalf("selected argote root %q closure = %+v", identity, selected.Resources)
		}
	}

	guidance, err := os.ReadFile(filepath.Join("..", "..", "bundle", "instructions", "argote-guidance.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Choose the simplest implementation", "Use neutral, international Spanish", "Write code, identifiers, comments, documentation, plans, ADRs, and commit messages in English"} {
		if !strings.Contains(string(guidance), want) {
			t.Fatalf("Argote guidance missing %q", want)
		}
	}
}
