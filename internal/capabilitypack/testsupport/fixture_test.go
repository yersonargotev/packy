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
	firstResource := first.Manifest().Resources[0]
	secondResource := second.Manifest().Resources[0]
	if firstResource.Kind == secondResource.Kind && firstResource.ID == secondResource.ID {
		t.Fatalf("collision fixtures reuse resource identity %s:%s", firstResource.Kind, firstResource.ID)
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

func hasResource(resources []Resource, kind, id string) bool {
	for _, resource := range resources {
		if resource.Kind == kind && resource.ID == id {
			return true
		}
	}
	return false
}
