package testsupport_test

import (
	"context"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/managedpack"
)

type originResolver map[string]string

func (r originResolver) Resolve(_ context.Context, origin managedpack.Origin) (string, error) {
	return r[origin.ID], nil
}

func TestProvenanceSafeMutationsRemainValidManagedProjects(t *testing.T) {
	tests := []struct {
		name    string
		fixture testsupport.Fixture
		want    testsupport.Relationship
	}{
		{
			name: "exact copy updates origin tree",
			fixture: testsupport.PortableAllSurfaces("exact-fixture").WithExactCopyBytes(
				"instruction:guidance", ".", []byte("updated exact guidance\n"),
			),
			want: testsupport.RelationshipExactCopy,
		},
		{
			name: "adaptation retains notice coverage",
			fixture: testsupport.PortableAllSurfaces("adapted-fixture").WithAdaptedBytes(
				"instruction:guidance", ".", []byte("adapted guidance\n"),
			),
			want: testsupport.RelationshipAdapted,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectRoot, originsRoot := t.TempDir(), t.TempDir()
			origins, err := test.fixture.WriteProject(projectRoot, originsRoot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := managedpack.ValidateProject(context.Background(), projectRoot, originResolver(origins)); err != nil {
				t.Fatalf("ValidateProject() error = %v", err)
			}
			resource := test.fixture.Manifest().Resources[0]
			if resource.Origin == nil || resource.Origin.Relationship != test.want {
				t.Fatalf("origin = %#v, want relationship %q", resource.Origin, test.want)
			}
			if len(resource.Notices) == 0 {
				t.Fatal("derived mutation lost notice coverage")
			}
		})
	}
}

func TestEveryRoleWritesAValidCurrentBundle(t *testing.T) {
	fixtures := []testsupport.Fixture{
		testsupport.PortableAllSurfaces("portable-role"),
		testsupport.CapabilityRich("capability-role"),
		testsupport.ExternalTool("external-role"),
	}
	first, second := testsupport.CollisionPair("collision-alpha", "collision-beta")
	fixtures = append(fixtures, first, second)

	bundleRoot := t.TempDir()
	for i, fixture := range fixtures {
		if err := fixture.WriteBundle(bundleRoot); err != nil {
			t.Fatal(err)
		}
		projectRoot, originsRoot := t.TempDir(), t.TempDir()
		origins, err := fixture.WriteProject(projectRoot, originsRoot)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := managedpack.ValidateProject(context.Background(), projectRoot, originResolver(origins)); err != nil {
			t.Fatalf("ValidateProject() synthetic role %d error = %v", i, err)
		}
	}
	if _, err := capabilitypack.Discover(context.Background(), bundleRoot); err != nil {
		t.Fatalf("Discover() synthetic role bundle error = %v", err)
	}
}
