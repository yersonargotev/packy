package issuedelivery

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

func TestMechanicalProfileFloorClassifiesEveryCandidateEffect(t *testing.T) {
	tests := []struct {
		name       string
		effect     CandidateEffect
		profile    deliveryevidence.DeliveryRiskProfile
		boundary   SensitiveBoundary
		specialist string
	}{
		{"passive", EffectPassive, deliveryevidence.RiskLow, "", ""},
		{"ordinary behavior", EffectOrdinaryBehavior, deliveryevidence.RiskStandard, "", ""},
		{"installation", EffectInstallation, deliveryevidence.RiskHigh, BoundaryInstallation, "installation-specialist"},
		{"real configuration", EffectRealConfiguration, deliveryevidence.RiskHigh, BoundaryRealConfiguration, "configuration-specialist"},
		{"security", EffectSecurity, deliveryevidence.RiskHigh, BoundarySecurity, "security-specialist"},
		{"publication", EffectPublication, deliveryevidence.RiskHigh, BoundaryPublication, "publication-specialist"},
		{"migration", EffectMigration, deliveryevidence.RiskHigh, BoundaryMigration, "migration-specialist"},
		{"persistent format", EffectPersistentFormat, deliveryevidence.RiskHigh, BoundaryPersistentFormat, "persistent-format-specialist"},
		{"governance", EffectGovernance, deliveryevidence.RiskHigh, BoundaryGovernance, "governance-specialist"},
		{"destructive effect", EffectDestructive, deliveryevidence.RiskHigh, BoundaryDestructive, "destructive-effect-specialist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mechanicalProfileFloor([]EffectObservation{{
				Effect:   tt.effect,
				Evidence: "candidate observation",
				Complete: true,
			}})
			if got.Profile != tt.profile || !got.Complete {
				t.Fatalf("assessment = %#v, want profile %q complete", got, tt.profile)
			}
			wantBoundaries := []SensitiveBoundary{}
			wantSpecialists := []string{}
			if tt.boundary != "" {
				wantBoundaries = []SensitiveBoundary{tt.boundary}
				wantSpecialists = []string{tt.specialist}
			}
			if !reflect.DeepEqual(got.Boundaries, wantBoundaries) {
				t.Fatalf("boundaries = %#v, want %#v", got.Boundaries, wantBoundaries)
			}
			if !reflect.DeepEqual(got.Specialists, wantSpecialists) {
				t.Fatalf("specialists = %#v, want %#v", got.Specialists, wantSpecialists)
			}
		})
	}
}

func TestMechanicalProfileFloorCanonicalizesAndDeduplicatesObservations(t *testing.T) {
	got := mechanicalProfileFloor([]EffectObservation{
		{Effect: EffectSecurity, Evidence: "  threat model  ", Complete: true},
		{Effect: EffectPassive, Evidence: "docs only", Complete: true},
		{Effect: EffectSecurity, Evidence: "threat model", Complete: true},
		{Effect: EffectInstallation, Evidence: "installer", Complete: true},
	})

	wantEffects := []EffectObservation{
		{Effect: EffectInstallation, Evidence: "installer", Complete: true},
		{Effect: EffectPassive, Evidence: "docs only", Complete: true},
		{Effect: EffectSecurity, Evidence: "threat model", Complete: true},
	}
	wantBoundaries := []SensitiveBoundary{BoundaryInstallation, BoundarySecurity}
	wantSpecialists := []string{"installation-specialist", "security-specialist"}
	if got.Profile != deliveryevidence.RiskHigh || !got.Complete {
		t.Fatalf("assessment = %#v, want complete high-risk", got)
	}
	if !reflect.DeepEqual(got.Effects, wantEffects) {
		t.Fatalf("effects = %#v, want %#v", got.Effects, wantEffects)
	}
	if !reflect.DeepEqual(got.Boundaries, wantBoundaries) {
		t.Fatalf("boundaries = %#v, want %#v", got.Boundaries, wantBoundaries)
	}
	if !reflect.DeepEqual(got.Specialists, wantSpecialists) {
		t.Fatalf("specialists = %#v, want %#v", got.Specialists, wantSpecialists)
	}
}

func TestMechanicalProfileFloorFailsClosedForUnknownOrIncompleteEvidence(t *testing.T) {
	tests := []struct {
		name         string
		observations []EffectObservation
	}{
		{"missing observations", nil},
		{"unknown effect", []EffectObservation{{Effect: "unknown", Evidence: "unclassified diff", Complete: true}}},
		{"missing evidence", []EffectObservation{{Effect: EffectPassive, Complete: true}}},
		{"incomplete observation", []EffectObservation{{Effect: EffectPassive, Evidence: "partial scan"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mechanicalProfileFloor(tt.observations)
			if got.Profile != deliveryevidence.RiskHigh || got.Complete {
				t.Fatalf("assessment = %#v, want incomplete high-risk", got)
			}
			if !reflect.DeepEqual(got.Boundaries, []SensitiveBoundary{BoundaryGovernance}) {
				t.Fatalf("boundaries = %#v, want conservative governance boundary", got.Boundaries)
			}
			if !reflect.DeepEqual(got.Specialists, []string{"governance-specialist"}) {
				t.Fatalf("specialists = %#v, want conservative governance specialist", got.Specialists)
			}
		})
	}
}

func TestMaxRiskProfileIsMonotonicAndFailsClosed(t *testing.T) {
	tests := []struct {
		left, right deliveryevidence.DeliveryRiskProfile
		want        deliveryevidence.DeliveryRiskProfile
	}{
		{deliveryevidence.RiskLow, deliveryevidence.RiskLow, deliveryevidence.RiskLow},
		{deliveryevidence.RiskLow, deliveryevidence.RiskStandard, deliveryevidence.RiskStandard},
		{deliveryevidence.RiskStandard, deliveryevidence.RiskLow, deliveryevidence.RiskStandard},
		{deliveryevidence.RiskStandard, deliveryevidence.RiskHigh, deliveryevidence.RiskHigh},
		{deliveryevidence.RiskHigh, deliveryevidence.RiskLow, deliveryevidence.RiskHigh},
		{"", deliveryevidence.RiskLow, deliveryevidence.RiskHigh},
		{deliveryevidence.RiskLow, "unknown", deliveryevidence.RiskHigh},
	}

	for _, tt := range tests {
		if got := maxRiskProfile(tt.left, tt.right); got != tt.want {
			t.Errorf("maxRiskProfile(%q, %q) = %q, want %q", tt.left, tt.right, got, tt.want)
		}
	}
}
