package capabilitypack

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestSurfaceVariantsRejectInvalidDeclarationsAndEffectiveGraphs(t *testing.T) {
	fixture := testsupport.SkillVariants("variant-validation", testsupport.SurfaceCodex)
	root := t.TempDir()
	if err := fixture.WriteCatalog(root); err != nil {
		t.Fatal(err)
	}
	pack, err := LoadCurrentManifest(root+"/packs/variant-validation/pack.json", root+"/packs/variant-validation", true)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*Pack)
		want   string
	}{
		{"unknown surface", func(p *Pack) { p.Resources[1].Variants[0].Surface = "cursor" }, "unknown or undeclared"},
		{"duplicate", func(p *Pack) { p.Resources[1].Variants = append(p.Resources[1].Variants, p.Resources[1].Variants[0]) }, "without duplicates"},
		{"missing dependency", func(p *Pack) { v := []string{"skill:absent"}; p.Resources[1].Variants[0].Requires = &v }, "does not exist"},
		{"cycle", func(p *Pack) { v := []string{"skill:guide"}; p.Resources[1].Variants[0].Requires = &v }, "cycle"},
		{"missing origin", func(p *Pack) { p.Resources[1].Variants[0].Origin = nil }, "explicit origin"},
		{"empty source", func(p *Pack) { v := ""; p.Resources[1].Variants[0].Source = &v }, "relative path"},
		{"empty description", func(p *Pack) { v := ""; p.Resources[1].Variants[0].Description = &v }, "description"},
		{"forbidden command", func(p *Pack) { v := "sh"; p.Resources[1].Variants[0].Command = &v }, "command"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := clonePack(pack)
			test.change(&candidate)
			err := validateCurrentPack(candidate)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %s", err, test.want)
			}
		})
	}
	for _, data := range []string{`{"surface":"codex","source":null}`, `{"surface":"codex","requires":null}`, `{"surface":"codex","id":"other"}`, `{"surface":"codex","bindings":[]}`, `{"surface":"codex","overrides":{}}`, `null`} {
		var variant ResourceVariant
		if err := json.Unmarshal([]byte(data), &variant); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestSurfaceVariantDependenciesResolveBeforeSelectionAndRemainDetached(t *testing.T) {
	fixture := testsupport.SkillVariants("variant-graph", testsupport.SurfaceCodex)
	root := t.TempDir()
	if err := fixture.WriteCatalog(root); err != nil {
		t.Fatal(err)
	}
	pack, err := LoadCurrentManifest(root+"/packs/variant-graph/pack.json", root+"/packs/variant-graph", true)
	if err != nil {
		t.Fatal(err)
	}
	helper := clonePack(pack).Resources[1]
	helper.ID = "helper"
	helper.Variants = nil
	for i := range helper.Bindings {
		helper.Bindings[i].Name = "helper"
	}
	pack.Resources = append(pack.Resources, helper)
	requires := []string{"skill:helper"}
	pack.Resources[1].Variants[0].Requires = &requires
	selected, err := selectPackResourcesForSurface(pack, ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "guide"}}}, SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Resources) != 3 {
		t.Fatalf("effective dependency missing: %#v", selected.Resources)
	}
	selected.Resources[1].Requires[0] = "changed"
	common := ResolvePackForSurface(pack, SurfaceClaude)
	if len(common.Resources[1].Requires) != 0 || pack.Resources[1].Variants[0].Requires == nil || (*pack.Resources[1].Variants[0].Requires)[0] != "skill:helper" {
		t.Fatal("resolution mutated common catalog")
	}
	pack.Resources[2].Bindings = pack.Resources[2].Bindings[:1]
	pack.Resources[2].SurfaceExclusions = []SurfaceExclusion{{Surface: SurfaceCodex, Mode: "optional", Code: "unsupported", Reason: "not available"}, {Surface: SurfaceOpenCode, Mode: "optional", Code: "unsupported", Reason: "not available"}}
	if err := validateVariants(pack); err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("excluded effective dependency accepted: %v", err)
	}
}
