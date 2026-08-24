package capabilitypack

import (
	"context"
	"reflect"
	"testing"
)

func TestFacadeShowReturnsCanonicalDescriptiveResourceInventory(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "example",
		Version:         "1.0.0",
		Description:     "Example Pack",
		Surfaces:        []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "notice", ID: "terms", Description: "Explains the license terms."},
			{Kind: "skill", ID: "review", Description: "Reviews a change.", Requires: []string{"asset:rubric"}, Notices: []string{"notice:terms"}},
			{Kind: "asset", ID: "rubric", Description: "Provides the review rubric."},
		},
	}
	facade := NewFacade(
		Catalog{packs: []Pack{pack}, entries: []catalogEntry{{ID: pack.ID}}},
		WithActivation(&fakeActivationStore{}, nil),
	)

	report, err := facade.Show(context.Background(), pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantAuthority := "Packy's Pack Admission Record is the immutable release and provenance authority; it remains outside the end-user bundle."
	if report.SourceIdentity.Limitation != wantAuthority {
		t.Fatalf("source identity authority = %q, want %q", report.SourceIdentity.Limitation, wantAuthority)
	}
	want := []DescriptiveResource{
		{
			Resource:     ResourceIdentity{Kind: "asset", ID: "rubric"},
			Description:  "Provides the review rubric.",
			Role:         ResourceInventoryRoleSupporting,
			Dependencies: []ResourceIdentity{},
			Notices:      []ResourceIdentity{},
		},
		{
			Resource:     ResourceIdentity{Kind: "notice", ID: "terms"},
			Description:  "Explains the license terms.",
			Role:         ResourceInventoryRoleNotice,
			Dependencies: []ResourceIdentity{},
			Notices:      []ResourceIdentity{},
		},
		{
			Resource:     ResourceIdentity{Kind: "skill", ID: "review"},
			Description:  "Reviews a change.",
			Role:         ResourceInventoryRoleOperational,
			Dependencies: []ResourceIdentity{{Kind: "asset", ID: "rubric"}},
			Notices:      []ResourceIdentity{{Kind: "notice", ID: "terms"}},
		},
	}
	if !reflect.DeepEqual(report.ResourceInventory, want) {
		t.Fatalf("resource inventory = %#v, want %#v", report.ResourceInventory, want)
	}
}
