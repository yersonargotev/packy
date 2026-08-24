package ci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

func TestIssue705OrchestrateManagedPackOwnsCurrentRuntimeContractAndClosure(t *testing.T) {
	root := repositoryRoot(t)
	bundleRoot := filepath.Join(root, "bundle")
	manifestPath := filepath.Join(bundleRoot, "packs", "orchestrate", "pack.json")
	pack, err := capabilitypack.LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "orchestrate" || pack.Version != "1.0.2" || !pack.Selectable ||
		!reflect.DeepEqual(pack.Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceCodex}) ||
		!reflect.DeepEqual(pack.ReadinessObligations, []capabilitypack.ReadinessObligation{capabilitypack.ReadinessRuntimeUsability, capabilitypack.ReadinessSurfaceAuthorization}) ||
		len(pack.Requires.Tools) != 0 {
		t.Fatalf("Orchestrate runtime identity = %#v", pack)
	}
	counts := pack.ResourceCounts()
	if counts.Skills != 1 || counts.Notices != 1 || counts.Lifecycles != 1 || counts.MCPServers != 0 || counts.Agents != 0 || counts.Commands != 0 || counts.Assets != 0 || counts.Instructions != 0 {
		t.Fatalf("Orchestrate resource counts = %#v", counts)
	}
	if len(pack.Resources) != 3 {
		t.Fatalf("Orchestrate resources = %#v", pack.Resources)
	}
	lifecycle, notice, skill := pack.Resources[0], pack.Resources[1], pack.Resources[2]
	if lifecycle.Kind != "lifecycle" || lifecycle.ID != "coordinate-session" || lifecycle.Source != "" || len(lifecycle.Bindings) != 1 {
		t.Fatalf("Orchestrate lifecycle = %#v", lifecycle)
	}
	assertIssue705Binding(t, lifecycle.Bindings[0], "lifecycle", "coordinate-session", "coordinate-session")
	if notice.Kind != "notice" || notice.ID != "mit" || notice.Source != "notices/mit" || notice.License != "MIT" || notice.Attribution != "Copyright (c) 2026 Eric Provencher" || !reflect.DeepEqual(notice.Notices, []string{"notice:mit"}) {
		t.Fatalf("Orchestrate legal notice = %#v", notice)
	}
	if skill.Kind != "skill" || skill.ID != "orchestrate" || skill.Source != "skills/orchestrate" || !reflect.DeepEqual(skill.Notices, []string{"notice:mit"}) || len(skill.Bindings) != 1 {
		t.Fatalf("Orchestrate skill = %#v", skill)
	}
	assertIssue705Binding(t, skill.Bindings[0], "skill", "orchestrate", "$orchestrate")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest managedpack.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	wantOrigins := []managedpack.Origin{{
		ID:         "orchestrate-skill",
		Repository: "yersonargotev/orchestrate-skill",
		Commit:     "bd9d77b3c43a2544cea68d08492cbf306752cd22",
		Revision:   "1.0.1",
	}}
	if !reflect.DeepEqual(manifest.Origins, wantOrigins) {
		t.Fatalf("Orchestrate origins = %#v; want %#v", manifest.Origins, wantOrigins)
	}
	if manifest.Resources[0].Source != "" || manifest.Resources[0].Origin != nil {
		t.Fatalf("Pack-authored lifecycle provenance = %#v", manifest.Resources[0])
	}
	wantNoticeOrigin := &managedpack.ResourceOrigin{ID: "orchestrate-skill", Path: "LICENSE", Relationship: managedpack.RelationshipExactCopy}
	if !reflect.DeepEqual(manifest.Resources[1].Origin, wantNoticeOrigin) {
		t.Fatalf("Orchestrate notice origin = %#v; want %#v", manifest.Resources[1].Origin, wantNoticeOrigin)
	}
	wantSkillOrigin := &managedpack.ResourceOrigin{ID: "orchestrate-skill", Path: "orchestrate", Relationship: managedpack.RelationshipExactCopy}
	if !reflect.DeepEqual(manifest.Resources[2].Origin, wantSkillOrigin) {
		t.Fatalf("Orchestrate skill origin = %#v; want %#v", manifest.Resources[2].Origin, wantSkillOrigin)
	}

	record, err := managedpack.LoadAdmissionRecord(filepath.Join(root, "managed-packs", "admissions", "orchestrate", "1.0.2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if record.PackID != "orchestrate" || record.PackVersion != "1.0.2" || record.Project != "yersonargotev/orchestrate-skill" ||
		record.RepositoryID != 1327217855 || record.ReleaseID != 375317485 || !record.ReleaseImmutable || record.Tag != "pack-v1.0.2" ||
		record.TagRefType != "commit" || record.TagRefSHA != "fdae7b6cc227cd1cd0e3cafb5ab1c66cca231cac" || len(record.TagObjects) != 0 ||
		record.Commit != "fdae7b6cc227cd1cd0e3cafb5ab1c66cca231cac" || record.RootTree != "7abb5c2f3d35127af72fec9eb487fc2da45e4ba0" ||
		record.ManifestSHA256 != "5b6d7a55f1603590f01b4d9c56741f2034194af90f1b479878350f3f0b613e69" || record.ClosureSHA256 != "527d6123689105800e5c537bf485609e1c74e84bc6a6b4fd580c057d70d1bd95" {
		t.Fatalf("Orchestrate admission identity = %#v", record)
	}
	actual := currentManagedPackClosure(t, bundleRoot, manifestPath, pack.Resources)
	if !reflect.DeepEqual(actual, record.Files) {
		t.Fatalf("current Orchestrate closure = %#v; want admitted closure %#v", actual, record.Files)
	}
}

func assertIssue705Binding(t *testing.T, binding capabilitypack.Binding, projection, name, invocation string) {
	t.Helper()
	if binding.Surface != capabilitypack.SurfaceCodex || binding.Projection != projection || binding.Name != name || binding.Invocation != invocation || binding.Mode != "native" || binding.Sharing != "exclusive" || len(binding.Capabilities) != 0 {
		t.Fatalf("Orchestrate %s binding = %#v", projection, binding)
	}
}
