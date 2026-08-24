package capabilitypack

import (
	"reflect"
	"strings"
	"testing"
)

func TestEvaluateRuntimeFitnessBuildsDeterministicSurfaceSelectionMatrix(t *testing.T) {
	pack := Pack{
		ID:       "fitness",
		Surfaces: []Surface{SurfaceOpenCode, SurfaceCodex},
		Resources: []Resource{
			{Kind: "asset", ID: "payload"},
			{Kind: "notice", ID: "license"},
			{
				Kind: "skill", ID: "consumer", Requires: []string{"instruction:shared"}, Notices: []string{"notice:license"},
				Bindings: []Binding{{Surface: SurfaceOpenCode, Projection: "skill", Name: "consumer"}, {Surface: SurfaceCodex, Projection: "skill", Name: "consumer"}},
			},
			{
				Kind: "instruction", ID: "shared",
				Bindings: []Binding{{Surface: SurfaceOpenCode, Projection: "instruction", Name: "shared"}, {Surface: SurfaceCodex, Projection: "instruction", Name: "shared"}},
			},
			{
				Kind: "agent", ID: "opencode-only",
				Bindings:          []Binding{{Surface: SurfaceOpenCode, Projection: "agent", Name: "helper"}},
				SurfaceExclusions: []SurfaceExclusion{{Surface: SurfaceCodex, Mode: "optional", Code: "unsupported", Reason: "not supported"}},
			},
		},
	}

	first, err := EvaluateRuntimeFitness(pack)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EvaluateRuntimeFitness(pack)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("fitness matrix is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	if len(first.Rows) != 8 {
		t.Fatalf("row count = %d, want 8: %#v", len(first.Rows), first.Rows)
	}
	wantSelections := []struct {
		surface Surface
		mode    SelectionMode
		root    string
	}{
		{SurfaceCodex, SelectionAll, ""},
		{SurfaceCodex, SelectionCustom, "agent:opencode-only"},
		{SurfaceCodex, SelectionCustom, "instruction:shared"},
		{SurfaceCodex, SelectionCustom, "skill:consumer"},
		{SurfaceOpenCode, SelectionAll, ""},
		{SurfaceOpenCode, SelectionCustom, "agent:opencode-only"},
		{SurfaceOpenCode, SelectionCustom, "instruction:shared"},
		{SurfaceOpenCode, SelectionCustom, "skill:consumer"},
	}
	for i, want := range wantSelections {
		row := first.Rows[i]
		root := ""
		if len(row.Selection.Roots) == 1 {
			root = row.Selection.Roots[0].String()
		}
		if row.Surface != want.surface || row.Selection.Mode != want.mode || root != want.root {
			t.Fatalf("row %d identity = %s/%s/%s, want %s/%s/%s", i, row.Surface, row.Selection.Mode, root, want.surface, want.mode, want.root)
		}
	}
	excluded := first.Rows[1]
	if excluded.Availability.Available || len(excluded.Availability.Reasons) != 1 || excluded.Availability.Reasons[0].Code != SelectionReasonRootExcluded || len(excluded.Resources) != 0 || len(excluded.Projections) != 0 {
		t.Fatalf("surface-excluded row = %#v", excluded)
	}
	consumer := first.Rows[3]
	if got, want := consumer.Resources, []ResourceIdentity{{Kind: "instruction", ID: "shared"}, {Kind: "notice", ID: "license"}, {Kind: "skill", ID: "consumer"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("consumer closure = %#v, want %#v", got, want)
	}
	if got, want := consumer.Projections, []RuntimeProjection{
		{Resource: ResourceIdentity{Kind: "instruction", ID: "shared"}, Projection: "instruction", Name: "shared"},
		{Resource: ResourceIdentity{Kind: "skill", ID: "consumer"}, Projection: "skill", Name: "consumer"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("consumer projections = %#v, want %#v", got, want)
	}
}

func TestEvaluateRuntimeFitnessRejectsUnavailableSelections(t *testing.T) {
	tests := []struct {
		name string
		pack Pack
		want string
	}{
		{
			name: "dependency unavailable",
			pack: Pack{ID: "dependency", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
				{Kind: "skill", ID: "consumer", Requires: []string{"instruction:missing-here"}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "consumer"}}},
				{Kind: "instruction", ID: "missing-here", Bindings: []Binding{{Surface: SurfaceOpenCode, Projection: "instruction", Name: "missing-here"}}, SurfaceExclusions: []SurfaceExclusion{{Surface: SurfaceCodex, Mode: "optional", Code: "unsupported", Reason: "not supported"}}},
			}},
			want: "dependency-unavailable",
		},
		{
			name: "conflict",
			pack: Pack{ID: "conflict", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
				{Kind: "skill", ID: "left", Conflicts: []string{"skill:right"}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "left"}}},
				{Kind: "skill", ID: "right", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "right"}}},
			}},
			want: "resource-conflict",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EvaluateRuntimeFitness(tc.pack)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestEvaluateRuntimeFitnessRejectsProjectionNameCollision(t *testing.T) {
	pack := Pack{ID: "collision", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "skill", ID: "left", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "same"}}},
		{Kind: "skill", ID: "right", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "same"}}},
	}}
	_, err := EvaluateRuntimeFitness(pack)
	if err == nil {
		t.Fatal("expected projection collision")
	}
	for _, fragment := range []string{"codex", "all", "skill:left", "skill:right", "skill+same"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("collision error %q does not contain %q", err, fragment)
		}
	}
}
