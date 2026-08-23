package testsupport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPortableAllSurfacesWritesBundleAndDerivesVersions(t *testing.T) {
	fixture := PortableAllSurfaces("portable-one")

	if got, want := fixture.CurrentVersion(), "1.0.0"; got != want {
		t.Fatalf("CurrentVersion() = %q, want %q", got, want)
	}
	if got, want := fixture.CandidateVersion(), "1.0.1"; got != want {
		t.Fatalf("CandidateVersion() = %q, want %q", got, want)
	}
	if got, want := fixture.ID(), "portable-one"; got != want {
		t.Fatalf("ID() = %q, want %q", got, want)
	}
	if got, want := fixture.OperationalResource(), (ResourceIdentity{Kind: "instruction", ID: "guidance"}); got != want {
		t.Fatalf("OperationalResource() = %#v, want %#v", got, want)
	}
	manifest := fixture.Manifest()
	if got, want := manifest.ID, "portable-one"; got != want {
		t.Fatalf("manifest ID = %q, want %q", got, want)
	}
	if got, want := manifest.Surfaces, []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}; !reflect.DeepEqual(got, want) {
		t.Fatalf("surfaces = %#v, want %#v", got, want)
	}

	bundleRoot := t.TempDir()
	if err := fixture.WriteBundle(bundleRoot); err != nil {
		t.Fatal(err)
	}
	manifestData, err := os.ReadFile(filepath.Join(bundleRoot, "packs", "portable-one", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	var written Manifest
	if err := json.Unmarshal(manifestData, &written); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(written, manifest) {
		t.Fatalf("written manifest = %#v, want %#v", written, manifest)
	}
	if got, err := os.ReadFile(filepath.Join(bundleRoot, "instructions", "portable-one.md")); err != nil {
		t.Fatal(err)
	} else if want := []byte("# Portable One guidance\n\nSynthetic guidance for every supported surface.\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("guidance bytes = %q, want %q", got, want)
	}
}

func TestRoleConstructorsUseDeliberatelySyntheticIdentities(t *testing.T) {
	fixtures := []Fixture{
		CapabilityRich("capability-one"),
		ExternalTool("external-one"),
	}
	first, second := CollisionPair("collision-one", "collision-two")
	fixtures = append(fixtures, first, second)

	wantIDs := []string{"capability-one", "external-one", "collision-one", "collision-two"}
	for i, fixture := range fixtures {
		if got := fixture.Manifest().ID; got != wantIDs[i] {
			t.Fatalf("fixture %d ID = %q, want %q", i, got, wantIDs[i])
		}
	}
	firstResource := manifestResource(t, first.Manifest(), first.OperationalResource())
	secondResource := manifestResource(t, second.Manifest(), second.OperationalResource())
	if firstResource.Kind == secondResource.Kind && firstResource.ID == secondResource.ID {
		t.Fatalf("collision fixtures reuse resource identity %s:%s", firstResource.Kind, firstResource.ID)
	}
	if firstResource.Source == secondResource.Source {
		t.Fatalf("collision fixtures reuse source %q", firstResource.Source)
	}
	if len(firstResource.Bindings) != len(secondResource.Bindings) {
		t.Fatalf("collision fixture bindings differ: %#v vs %#v", firstResource.Bindings, secondResource.Bindings)
	}
	for index := range firstResource.Bindings {
		firstBinding := firstResource.Bindings[index]
		secondBinding := secondResource.Bindings[index]
		if firstBinding.Surface != secondBinding.Surface || firstBinding.Projection != secondBinding.Projection || firstBinding.Name != secondBinding.Name {
			t.Fatalf("collision fixtures do not share target binding %d: %#v vs %#v", index, firstBinding, secondBinding)
		}
	}
}

func TestCandidateRewritesVersionAndCapabilityRichOwnsRuntimeLifecycle(t *testing.T) {
	current := CapabilityRich("capability-one")
	candidate := current.Candidate()

	if got, want := candidate.CurrentVersion(), current.CandidateVersion(); got != want {
		t.Fatalf("candidate version = %q, want %q", got, want)
	}
	if current.CurrentVersion() == candidate.CurrentVersion() {
		t.Fatal("Candidate() mutated the current fixture")
	}
	manifest := current.Manifest()
	if !hasResource(manifest.Resources, "lifecycle", "session") {
		t.Fatalf("CapabilityRich resources = %#v, want lifecycle:session", manifest.Resources)
	}
}

func TestWithRetainedRequirementsKeepsCapabilityReferencesCoherentAndWritesBundle(t *testing.T) {
	workflow := ResourceIdentity{Kind: "skill", ID: "workflow"}
	helper := ResourceIdentity{Kind: "skill", ID: "helper"}
	current := CapabilityRich("requirements-one")
	narrowed := current.WithRetainedRequirements(workflow, helper)

	currentWorkflow := manifestResource(t, current.Manifest(), workflow)
	if got, want := currentWorkflow.Requires, []string{"asset:reference", "skill:helper"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("current requirements = %#v, want %#v", got, want)
	}
	narrowedWorkflow := manifestResource(t, narrowed.Manifest(), workflow)
	if got, want := narrowedWorkflow.Requires, []string{"skill:helper"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("narrowed requirements = %#v, want %#v", got, want)
	}
	composite := narrowedWorkflow.Bindings[0].Capabilities[0].ClaudeCompositeSkill
	if composite == nil || !reflect.DeepEqual(composite.Dependencies, []ResourceIdentity{helper}) || len(composite.References) != 0 {
		t.Fatalf("narrowed Claude capability = %#v", composite)
	}

	retired := narrowed.Candidate().WithRetainedRequirements(workflow)
	retiredWorkflow := manifestResource(t, retired.Manifest(), workflow)
	retiredComposite := retiredWorkflow.Bindings[0].Capabilities[0].ClaudeCompositeSkill
	if len(retiredWorkflow.Requires) != 0 || retiredComposite == nil || len(retiredComposite.Dependencies) != 0 || len(retiredComposite.References) != 0 {
		t.Fatalf("retired requirement contract = resource %#v capability %#v", retiredWorkflow, retiredComposite)
	}
	if retired.CurrentVersion() != narrowed.CandidateVersion() {
		t.Fatalf("retired candidate version = %q, want %q", retired.CurrentVersion(), narrowed.CandidateVersion())
	}

	bundleRoot := t.TempDir()
	if err := retired.WriteBundle(bundleRoot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(bundleRoot, "packs", retired.ID(), "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	var written Manifest
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(written, retired.Manifest()) {
		t.Fatalf("written retained-requirement manifest = %#v, want %#v", written, retired.Manifest())
	}
}

func TestWithRetainedRequirementsRejectsInventedDependency(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("WithRetainedRequirements accepted an undeclared dependency")
		}
	}()
	CapabilityRich("requirements-one").WithRetainedRequirements(
		ResourceIdentity{Kind: "skill", ID: "workflow"},
		ResourceIdentity{Kind: "skill", ID: "missing"},
	)
}

func hasResource(resources []Resource, kind, id string) bool {
	for _, resource := range resources {
		if resource.Kind == kind && resource.ID == id {
			return true
		}
	}
	return false
}

func manifestResource(t *testing.T, manifest Manifest, identity ResourceIdentity) Resource {
	t.Helper()
	for _, resource := range manifest.Resources {
		if resource.Kind == identity.Kind && resource.ID == identity.ID {
			return resource
		}
	}
	t.Fatalf("manifest %q has no resource %s", manifest.ID, identity)
	return Resource{}
}
