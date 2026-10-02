package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestSelectedDefinitionExplanationIsSharedAcrossPresentations(t *testing.T) {
	variantSource := "skills/codex/guide"
	pack := capabilitypack.Pack{ID: "definitions", Version: "1.0.0", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceClaude}, Resources: []capabilitypack.Resource{{
		Kind: "skill", ID: "guide", Description: "Reviewed guide", Source: "skills/common/guide",
		Origin:   &capabilitypack.ResourceOrigin{ID: "upstream", Path: "guide", Relationship: "exact-copy"},
		Notices:  []string{"notice:mit"},
		Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex}, {Surface: capabilitypack.SurfaceClaude}},
		Variants: []capabilitypack.ResourceVariant{{Surface: capabilitypack.SurfaceCodex, Source: &variantSource, Origin: &capabilitypack.ResourceOrigin{ID: "upstream", Path: "guide", Relationship: "adapted"}}},
	}}}
	for _, surface := range pack.Surfaces {
		contract := capabilitypack.LifecycleContractFor(pack, surface, nil)
		if len(contract.ResourceDefinitions) != 1 {
			t.Fatalf("definitions = %#v", contract.ResourceDefinitions)
		}
		definition := contract.ResourceDefinitions[0]
		wantKind, wantSource, wantRelationship := "common", "skills/common/guide", "exact-copy"
		if surface == capabilitypack.SurfaceCodex {
			wantKind, wantSource, wantRelationship = "surface_variant", variantSource, "adapted"
		}
		if definition.Definition != wantKind || definition.Source != wantSource || definition.Origin.Relationship != wantRelationship || len(definition.Notices) != 1 {
			t.Fatalf("selected definition = %#v", definition)
		}
		var human bytes.Buffer
		if err := renderPackShowContract(&human, contract); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(human.String(), definition.Summary()) {
			t.Fatalf("human output omits selected body: %s", human.String())
		}
		preview := globalPreviewForTUI(capabilitypack.JSONLifecyclePlan{Contract: contract})
		if !reflect.DeepEqual(preview.ResourceDefinitions, []string{definition.Summary()}) {
			t.Fatalf("TUI definitions = %#v", preview.ResourceDefinitions)
		}
		definitions := capabilitypack.ResourceDefinitionsFor(pack, surface)
		definitions[0].Origin.Relationship = "modified"
	}
	if pack.Resources[0].Source != "skills/common/guide" || pack.Resources[0].Origin.Relationship != "exact-copy" || pack.Resources[0].Variants[0].Origin.Relationship != "adapted" {
		t.Fatal("presentation mutated retained catalog")
	}
}
