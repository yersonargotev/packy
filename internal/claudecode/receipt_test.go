package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestReceiptInspectionRequiresExactInstructionContributor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	layout := NewCanonicalLayout(home)
	if err := os.MkdirAll(layout.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	owner := capabilitypack.ProjectionOwnership{ID: "instruction:guide", PackID: "historical", Surface: capabilitypack.SurfaceClaude, Target: layout.InstructionsFile, Fingerprint: Fingerprint([]byte("installed guidance"))}
	adapter := NewSurfaceAdapter("", layout, "", "", nil, OwnershipSnapshotFunc(func(context.Context) (OwnershipSnapshot, error) {
		t.Fatal("receipt inspection resolved a historical contract")
		return OwnershipSnapshot{}, nil
	}))
	for _, tc := range []struct {
		name, contributor, content string
		exists, exact              bool
	}{
		{"exact", "pack:historical:guide", "installed guidance", true, true},
		{"drift", "pack:historical:guide", "changed guidance", true, false},
		{"renamed", "pack:historical:other", "installed guidance", false, false},
		{"other pack", "pack:other:guide", "installed guidance", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document, err := UpsertInstructionContribution("personal guidance\n", InstructionContribution{ContributorID: tc.contributor, Content: tc.content})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(layout.InstructionsFile, []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ReceiptOwnership: []capabilitypack.ProjectionOwnership{owner}})
			if err != nil {
				t.Fatal(err)
			}
			p := report.Projections[0]
			if p.Exists != tc.exists || (p.ObservedFingerprint == owner.Fingerprint) != tc.exact || p.Action.Target != owner.Target || p.DesiredFingerprint != owner.Fingerprint {
				t.Fatalf("projection = %+v", p)
			}
			got, err := os.ReadFile(layout.InstructionsFile)
			if err != nil || string(got) != document {
				t.Fatal("receipt inspection changed instructions")
			}
		})
	}
}

func TestStatusObservationDoesNotResolveUnrelatedHistoricalMutationOwnership(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	layout := NewCanonicalLayout(home)
	state := capabilitypack.ActivationState{Intent: capabilitypack.ActivationIntent{PackID: "older", Version: "1.0.0", Active: true, Surface: capabilitypack.SurfaceClaude}}
	binding := capabilitypack.Binding{Surface: capabilitypack.SurfaceClaude, Projection: "instruction", Name: "guide"}
	current := capabilitypack.Pack{ID: "other", Version: "1.0.0", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceClaude}, Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md", Bindings: []capabilitypack.Binding{binding}}}}
	document, err := UpsertInstructionContribution("", InstructionContribution{ContributorID: "pack:other:guide", Content: "Reviewed guidance"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "guide.md"), []byte("Reviewed guidance"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.InstructionsFile, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	provider := NewCapabilityPackOwnershipProvider(ownershipStore{state}, ownershipPackResolver(map[string]capabilitypack.Pack{"older": {ID: "older", Version: "2.0.0"}, "other": current}), layout, t.TempDir())
	adapter := NewSurfaceAdapter(home, layout, "", "", nil, provider)
	observation, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ObservationOnly: true, Desired: current})
	if err != nil {
		t.Fatalf("unrelated historical receipt blocked current Pack Status: %v", err)
	}
	if len(observation.Projections) != 1 || !observation.Projections[0].Exists || observation.Projections[0].ObservedFingerprint != observation.Projections[0].DesiredFingerprint {
		t.Fatalf("exact current instruction reported false drift without mutation ownership: %+v", observation.Projections)
	}
	if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: current}); err == nil || !strings.Contains(err.Error(), "no exact registered adapter contract") {
		t.Fatalf("mutation inspection lost strict historical ownership guard: %v", err)
	}
}
