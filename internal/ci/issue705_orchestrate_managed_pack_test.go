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

func TestIssue705OrchestrateManagedPackOwnsCurrentRuntimeContractAndClosure(t *testing.T) {
	root := repositoryRoot(t)
	bundleRoot := filepath.Join(root, "bundle")
	manifestPath := filepath.Join(bundleRoot, "packs", "orchestrate", "pack.json")
	pack, err := capabilitypack.LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "orchestrate" || pack.Version != "1.0.2" || !pack.Selectable || pack.SourceReference != nil ||
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

func TestIssue705OrchestrateLegacyAuthorityIsAbsentAndRemainingSourcesAreClosed(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"bundle/compatibility/orchestrate",
		"bundle/history/orchestrate",
		"bundle/sources/orchestrate-source.lock.json",
		"docs/research/evidence/orchestrate-skill-1.0.0-legal-admission.json",
		"docs/research/evidence/orchestrate-skill-1.0.1-legal-admission.json",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Errorf("legacy Orchestrate authority remains at %s: %v", relative, err)
		}
	}

	manifestPath := filepath.Join(root, "bundle", "packs", "orchestrate", "pack.json")
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
			t.Errorf("legacy Orchestrate manifest field %q remains", key)
		}
	}

	sources := loadCheckedInSources(t, root)
	for _, source := range sources {
		if source.ID == "orchestrate-source" {
			t.Errorf("legacy Orchestrate source registration remains: %#v", source)
		}
		for _, resource := range source.Resources {
			if resource.PackID == "orchestrate" {
				t.Errorf("legacy Orchestrate source binding remains in %s", source.ID)
			}
		}
	}
	assertRemainingLegacySourcesAreClosed(t, root, sources)
}

type checkedInSourceConfiguration struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Resources  []struct {
		PackID string `json:"pack_id"`
	} `json:"resources"`
}

func loadCheckedInSources(t *testing.T, root string) []checkedInSourceConfiguration {
	t.Helper()
	var configuration struct {
		Sources []checkedInSourceConfiguration `json:"sources"`
	}
	data, err := os.ReadFile(filepath.Join(root, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &configuration); err != nil {
		t.Fatal(err)
	}
	return configuration.Sources
}

func assertRemainingLegacySourcesAreClosed(t *testing.T, root string, sources []checkedInSourceConfiguration) {
	t.Helper()
	consumerPacks := make(map[string]struct{})
	sourceRepositories := make(map[string]struct{})
	for _, source := range sources {
		sourceRepositories[source.Repository] = struct{}{}
		lockPath := filepath.Join(root, "bundle", "sources", source.ID+".lock.json")
		if info, err := os.Stat(lockPath); err != nil || !info.Mode().IsRegular() {
			t.Errorf("source %s lock is not a regular file: %v", source.ID, err)
		}
		for _, resource := range source.Resources {
			consumerPacks[resource.PackID] = struct{}{}
			manifestPath := filepath.Join(root, "bundle", "packs", resource.PackID, "pack.json")
			pack, err := capabilitypack.LoadCurrentManifest(manifestPath, filepath.Join(root, "bundle"), true)
			if err != nil {
				t.Errorf("load source consumer %s: %v", resource.PackID, err)
				continue
			}
			if pack.SourceReference == nil {
				t.Errorf("source %s consumer %s lacks source_reference", source.ID, resource.PackID)
			}
		}
	}
	for _, relativeRoot := range []string{"bundle/history", "bundle/compatibility"} {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(relativeRoot)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if _, ok := consumerPacks[entry.Name()]; !ok {
				t.Errorf("%s/%s has no remaining Pack Source consumer", relativeRoot, entry.Name())
			}
		}
	}

	evidenceRoot := filepath.Join(root, "docs", "research", "evidence")
	entries, err := os.ReadDir(evidenceRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "-legal-admission.json") {
			continue
		}
		var evidence struct {
			Candidate struct {
				Repository string `json:"repository"`
			} `json:"candidate"`
		}
		data, err := os.ReadFile(filepath.Join(evidenceRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &evidence); err != nil {
			t.Fatalf("decode %s: %v", entry.Name(), err)
		}
		if _, ok := sourceRepositories[evidence.Candidate.Repository]; !ok {
			t.Errorf("legacy legal evidence %s has no remaining Pack Source repository", entry.Name())
		}
	}
}
