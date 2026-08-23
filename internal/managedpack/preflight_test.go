package managedpack

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPreflightLoadsTheExactMaterializedRuntimeAndReturnsDeterministicFitness(t *testing.T) {
	project, origin := writeValidProject(t)
	skillPath := filepath.Join(project, "skills", "guide", "SKILL.md")
	if err := os.Chmod(skillPath, 0o755); err != nil {
		t.Fatal(err)
	}

	first, err := Preflight(context.Background(), project, originResolver{"upstream": origin})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Preflight(context.Background(), project, originResolver{"upstream": origin})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("preflight is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	if first.Validation.ManifestSHA256 == "" || first.Validation.ClosureSHA256 == "" {
		t.Fatalf("validation identity = %#v", first.Validation)
	}
	if !validSHA256(first.RuntimeManifestSHA256) {
		t.Fatalf("runtime manifest identity = %q", first.RuntimeManifestSHA256)
	}
	var executableSkill bool
	for _, file := range first.Validation.Files {
		if file.Path == "skills/guide/SKILL.md" && file.Mode == "100755" {
			executableSkill = true
		}
	}
	if !executableSkill {
		t.Fatalf("validation files did not preserve executable mode: %#v", first.Validation.Files)
	}
	if first.RuntimeManifest.ID != first.Validation.Manifest.ID || first.RuntimeManifest.Version != first.Validation.Manifest.Version {
		t.Fatalf("runtime manifest identity = %s@%s, validation = %s@%s", first.RuntimeManifest.ID, first.RuntimeManifest.Version, first.Validation.Manifest.ID, first.Validation.Manifest.Version)
	}
	if got, want := len(first.Fitness.Rows), 6; got != want {
		t.Fatalf("fitness rows = %d, want %d: %#v", got, want, first.Fitness.Rows)
	}
	wantRows := []struct {
		surface   capabilitypack.Surface
		selection capabilitypack.ResourceSelection
	}{
		{capabilitypack.SurfaceClaude, capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll, Roots: []capabilitypack.ResourceIdentity{}}},
		{capabilitypack.SurfaceClaude, capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{{Kind: "skill", ID: "guide"}}}},
		{capabilitypack.SurfaceCodex, capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll, Roots: []capabilitypack.ResourceIdentity{}}},
		{capabilitypack.SurfaceCodex, capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{{Kind: "skill", ID: "guide"}}}},
		{capabilitypack.SurfaceOpenCode, capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll, Roots: []capabilitypack.ResourceIdentity{}}},
		{capabilitypack.SurfaceOpenCode, capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionCustom, Roots: []capabilitypack.ResourceIdentity{{Kind: "skill", ID: "guide"}}}},
	}
	for index, want := range wantRows {
		row := first.Fitness.Rows[index]
		if row.Surface != want.surface || !reflect.DeepEqual(row.Selection, want.selection) {
			t.Fatalf("fitness row %d = %#v, want surface=%s selection=%#v", index, row, want.surface, want.selection)
		}
	}
}

func TestPreflightPreservesTypedCapabilitiesAndDeclarationOnlyResources(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "instructions", "guide.md"), "managed guidance\n", 0o755)
	manifest := strings.ReplaceAll(lifecycleManifest, "prompts/guide.md", "instructions/guide.md")
	writeFile(t, filepath.Join(project, "pack.json"), manifest, 0o644)
	validation, err := ValidateProject(context.Background(), project, nil)
	if err != nil {
		t.Fatal(err)
	}
	bundleRoot := t.TempDir()
	if err := MaterializeClosure(context.Background(), project, bundleRoot, validation); err != nil {
		t.Fatal(err)
	}
	runtimeManifest, err := capabilitypack.LoadCurrentManifest(filepath.Join(bundleRoot, "packs", "example", "pack.json"), bundleRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range validation.Files {
		sourcePath := filepath.Join(project, filepath.FromSlash(record.Path))
		destinationPath := filepath.Join(bundleRoot, filepath.FromSlash(record.Path))
		if record.Path == "pack.json" {
			destinationPath = filepath.Join(bundleRoot, "packs", "example", "pack.json")
		}
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		destination, err := os.ReadFile(destinationPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(source, destination) || digestBytes(destination) != record.SHA256 {
			t.Fatalf("materialized closure file %q did not preserve bytes or digest", record.Path)
		}
		info, err := os.Lstat(destinationPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := canonicalMode(info.Mode()); got != record.Mode {
			t.Fatalf("materialized closure file %q mode = %s, want %s", record.Path, got, record.Mode)
		}
	}

	result, err := Preflight(context.Background(), project, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Validation, validation) || !reflect.DeepEqual(result.RuntimeManifest, runtimeManifest) {
		t.Fatalf("preflight identities differ from characterized materialization:\nresult=%#v\nvalidation=%#v\nruntime=%#v", result, validation, runtimeManifest)
	}
	if got, want := len(result.RuntimeManifest.Resources), 1; got != want {
		t.Fatalf("runtime resources = %d, want %d", got, want)
	}
	resource := result.RuntimeManifest.Resources[0]
	if resource.Kind != "lifecycle" || resource.Source != "" {
		t.Fatalf("declaration-only resource = %#v", resource)
	}
	capability, ok := resource.SurfaceCapability(capabilitypack.SurfaceCodex, capabilitypack.SurfaceCapabilityProjectInstruction)
	if !ok || capability.ProjectInstruction == nil || capability.ProjectInstruction.Source != "instructions/guide.md" {
		t.Fatalf("typed project-instruction capability = %#v, present=%v", capability, ok)
	}
	if got, want := result.Validation.Files, []FileRecord{
		{Path: "instructions/guide.md", Mode: "100755", SHA256: digestBytes([]byte("managed guidance\n"))},
		{Path: "pack.json", Mode: "100644", SHA256: result.Validation.ManifestSHA256},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("declaration-only closure = %#v, want %#v", got, want)
	}
	if got, want := len(result.Fitness.Rows), 2; got != want {
		t.Fatalf("declaration-only fitness rows = %d, want %d", got, want)
	}
}

func TestPreflightRejectsRuntimeProjectionCollisionsBeforePublication(t *testing.T) {
	project, origin := writeValidProject(t)
	writeFile(t, filepath.Join(project, "skills", "other", "SKILL.md"), "other guidance\n", 0o644)
	mutateManifest(t, project, func(manifest map[string]any) {
		resources := manifest["resources"].([]any)
		other := deepCopyMap(t, resources[1].(map[string]any))
		other["id"] = "other"
		other["source"] = "skills/other"
		delete(other, "origin")
		delete(other, "notices")
		manifest["resources"] = append(resources, other)
	})
	if _, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin}); err != nil {
		t.Fatalf("authoring validation unexpectedly rejected collision fixture: %v", err)
	}

	_, err := Preflight(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !strings.Contains(err.Error(), "runtime fitness") || !strings.Contains(err.Error(), "projection collision") {
		t.Fatalf("preflight collision error = %v", err)
	}
}
