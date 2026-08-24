package ci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

func TestIssue704IssueDeliveryManagedPackOwnsCurrentRuntimeContractAndClosure(t *testing.T) {
	root := repositoryRoot(t)
	bundleRoot := filepath.Join(root, "bundle")
	manifestPath := filepath.Join(bundleRoot, "packs", "issue-delivery", "pack.json")
	pack, err := capabilitypack.LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "issue-delivery" || pack.Version != "1.1.2" || !pack.Selectable || pack.SourceReference != nil ||
		!reflect.DeepEqual(pack.Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceCodex}) ||
		!reflect.DeepEqual(pack.ReadinessObligations, []capabilitypack.ReadinessObligation{capabilitypack.ReadinessRuntimeUsability, capabilitypack.ReadinessSurfaceAuthorization}) ||
		!reflect.DeepEqual(pack.Requires.Tools, []string{"gh", "git"}) {
		t.Fatalf("Issue Delivery runtime identity = %#v", pack)
	}
	counts := pack.ResourceCounts()
	if counts.Skills != 3 || counts.Notices != 1 || counts.MCPServers != 0 || counts.Lifecycles != 0 || counts.Agents != 0 || counts.Commands != 0 || counts.Assets != 0 || counts.Instructions != 0 {
		t.Fatalf("Issue Delivery resource counts = %#v", counts)
	}
	if len(pack.Resources) != 4 {
		t.Fatalf("Issue Delivery resources = %#v", pack.Resources)
	}
	notice, deliverIssue, deliverIssueMatt, setupIssueDelivery := pack.Resources[0], pack.Resources[1], pack.Resources[2], pack.Resources[3]
	if notice.Kind != "notice" || notice.ID != "mit" || notice.Source != "notices/issue-delivery-mit" || notice.License != "MIT" || notice.Attribution != "Copyright (c) 2026 Yerson Argote" {
		t.Fatalf("Issue Delivery legal notice = %#v", notice)
	}
	assertIssue704Skill(t, deliverIssue, "deliver-issue", "skills/deliver-issue", "$deliver-issue")
	if len(deliverIssue.Requires) != 0 {
		t.Fatalf("deliver-issue dependencies = %#v", deliverIssue.Requires)
	}
	assertIssue704Skill(t, deliverIssueMatt, "deliver-issue-matt", "skills/deliver-issue-matt", "$deliver-issue-matt")
	if len(deliverIssueMatt.Requires) != 0 || !strings.Contains(deliverIssueMatt.Description, "requires the tdd and code-review skills from the matty Pack at runtime") {
		t.Fatalf("deliver-issue-matt dependency contract = %#v", deliverIssueMatt)
	}
	assertIssue704Skill(t, setupIssueDelivery, "setup-issue-delivery", "skills/setup-issue-delivery", "$setup-issue-delivery")
	if !reflect.DeepEqual(setupIssueDelivery.Requires, []string{"skill:deliver-issue"}) {
		t.Fatalf("setup-issue-delivery dependencies = %#v", setupIssueDelivery.Requires)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest managedpack.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	wantOrigins := []managedpack.Origin{{
		ID:         "mattpocock-skills",
		Repository: "mattpocock/skills",
		Commit:     "84fdeffd12f2ee307994d1eb6feb48173b6e0502",
		Revision:   "84fdeffd12f2ee307994d1eb6feb48173b6e0502",
	}}
	if !reflect.DeepEqual(manifest.Origins, wantOrigins) {
		t.Fatalf("Issue Delivery origins = %#v; want %#v", manifest.Origins, wantOrigins)
	}
	for _, index := range []int{0, 1, 3} {
		if manifest.Resources[index].Origin != nil {
			t.Fatalf("Pack-authored resource provenance = %#v", manifest.Resources[index])
		}
	}
	wantMattOrigin := &managedpack.ResourceOrigin{ID: "mattpocock-skills", Path: "skills/engineering/implement", Relationship: managedpack.RelationshipAdapted}
	if !reflect.DeepEqual(manifest.Resources[2].Origin, wantMattOrigin) {
		t.Fatalf("deliver-issue-matt provenance = %#v; want %#v", manifest.Resources[2].Origin, wantMattOrigin)
	}

	record, err := managedpack.LoadAdmissionRecord(filepath.Join(root, "managed-packs", "admissions", "issue-delivery", "1.1.2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if record.PackID != "issue-delivery" || record.PackVersion != "1.1.2" || record.Project != "yersonargotev/issue-deliver-pack" ||
		record.RepositoryID != 1331493580 || record.ReleaseID != 375397827 || !record.ReleaseImmutable || record.Tag != "pack-v1.1.2" ||
		record.TagRefType != "commit" || record.TagRefSHA != "232da50b1a3ee087c48af6bdfc04cafd94319851" || len(record.TagObjects) != 0 ||
		record.Commit != "232da50b1a3ee087c48af6bdfc04cafd94319851" || record.RootTree != "18e86c09195a93e8a3489be7ea5b59c08eb848cb" ||
		record.ManifestSHA256 != "a84d3ad49e52611ce6b194954048d0443f12b2bb68aff983eaf22f29141d9b6b" || record.ClosureSHA256 != "56d9d93bcfe7af38af6c45c259ce72f83b92e3988713beb2656f25ccde7a19ca" {
		t.Fatalf("Issue Delivery admission identity = %#v", record)
	}
	actual := currentManagedPackClosure(t, bundleRoot, manifestPath, pack.Resources)
	if !reflect.DeepEqual(actual, record.Files) {
		t.Fatalf("current Issue Delivery closure = %#v; want admitted closure %#v", actual, record.Files)
	}
}

func assertIssue704Skill(t *testing.T, resource capabilitypack.Resource, id, source, invocation string) {
	t.Helper()
	if resource.Kind != "skill" || resource.ID != id || resource.Source != source || !reflect.DeepEqual(resource.Notices, []string{"notice:mit"}) || len(resource.Bindings) != 1 {
		t.Fatalf("Issue Delivery skill %s = %#v", id, resource)
	}
	binding := resource.Bindings[0]
	if binding.Surface != capabilitypack.SurfaceCodex || binding.Projection != "skill" || binding.Name != id || binding.Invocation != invocation || binding.Mode != "native" || binding.Sharing != "exclusive" || len(binding.Capabilities) != 0 {
		t.Fatalf("Issue Delivery skill %s binding = %#v", id, binding)
	}
}

func TestIssue704IssueDeliveryLegacyAuthorityIsAbsent(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"bundle/compatibility/issue-delivery",
		"bundle/history/issue-delivery",
		"bundle/sources/issue-delivery-source.lock.json",
		"docs/research/evidence/issue-deliver-pack-1.0.0-legal-admission.json",
		"docs/research/evidence/issue-deliver-pack-1.1.0-legal-admission.json",
		"docs/research/evidence/issue-deliver-pack-1.1.1-legal-admission.json",
		"docs/research/evidence/generic-issue-delivery-pack-viability.md",
		"internal/packsync/issue_delivery_legal_admission_test.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Errorf("legacy Issue Delivery authority remains at %s: %v", relative, err)
		}
	}

	manifestPath := filepath.Join(root, "bundle", "packs", "issue-delivery", "pack.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var rawManifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawManifest); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"exclusions", "source_reference"} {
		if _, ok := rawManifest[key]; ok {
			t.Errorf("legacy Issue Delivery manifest field %q remains", key)
		}
	}

	sources := loadCheckedInSources(t, root)
	if len(sources) != 0 {
		t.Errorf("legacy Pack Source registrations remain: %#v", sources)
	}
	assertRemainingLegacySourcesAreClosed(t, root, sources)
}
