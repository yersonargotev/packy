package capabilitypack

import (
	"reflect"
	"testing"
)

func TestStatusJSONDistinguishesObservedFalseFromUnknownAndSorts(t *testing.T) {
	knownFalse := StatusEntry{Pack: Pack{ID: "z", Version: "1"}, Surface: SurfaceCodex, IntentPresent: true, Intent: IntentStatus{Revision: 2}, ReadinessObserved: ReadinessObservationStatus{Configured: true, Authorization: true}, OptionalAuthorities: []OptionalAuthorityObservation{
		{ModeID: "shipping", Authority: "deploy", State: OptionalAuthorityUnavailable, Fallback: "none"},
		{ModeID: "browser", Authority: "network", State: OptionalAuthorityAvailable, Fallback: "static evidence"},
		{ModeID: "browser", Authority: "browser", State: OptionalAuthorityUnknown, Fallback: "static evidence"},
	}, Blockers: []string{"z", "a"}, Evidence: nil, PendingHumanActions: []string{"reload", "login"}}
	unknown := StatusEntry{Pack: Pack{ID: "a", Version: "1"}, Surface: SurfaceOpenCode, ReadinessObserved: ReadinessObservationStatus{Configured: true}}
	report := (StatusReport{Entries: []StatusEntry{knownFalse, unknown}}).JSONReport(false)
	if report.SchemaVersion != StatusSchemaVersion {
		t.Fatalf("status schema version = %d", report.SchemaVersion)
	}
	if report.Entries[0].Pack != "a" || report.Entries[1].Pack != "z" {
		t.Fatalf("entries not sorted: %#v", report.Entries)
	}
	entry := report.Entries[1]
	if entry.Intent.State != "known" || entry.Intent.Active == nil || *entry.Intent.Active || entry.Readiness.Authorized.State != "known" || entry.Readiness.Authorized.Value == nil || *entry.Readiness.Authorized.Value {
		t.Fatalf("observed false lost: %#v", entry)
	}
	if entry.Readiness.Usable.State != "unknown" || entry.Readiness.Usable.Value != nil {
		t.Fatalf("unknown lost: %#v", entry.Readiness.Usable)
	}
	wantAuthorities := []JSONOptionalAuthority{
		{ModeID: "browser", Authority: "browser", State: OptionalAuthorityUnknown, Fallback: "static evidence"},
		{ModeID: "browser", Authority: "network", State: OptionalAuthorityAvailable, Fallback: "static evidence"},
		{ModeID: "shipping", Authority: "deploy", State: OptionalAuthorityUnavailable, Fallback: "none"},
	}
	if !reflect.DeepEqual(entry.OptionalAuthorities, wantAuthorities) {
		t.Fatalf("optional authorities = %#v, want %#v", entry.OptionalAuthorities, wantAuthorities)
	}
	if entry.Readiness.Authorized.State != "known" || entry.Readiness.Authorized.Value == nil || *entry.Readiness.Authorized.Value {
		t.Fatalf("optional authority availability changed readiness authorization: %#v", entry.Readiness)
	}
	if !reflect.DeepEqual(entry.Blockers, []string{"a", "z"}) || !reflect.DeepEqual(entry.PendingHumanActions, []string{"login", "reload"}) || entry.Evidence == nil {
		t.Fatalf("arrays not deterministic/non-null: %#v", entry)
	}
}

func TestStatusJSONNormalizesUnknownAttemptOutcome(t *testing.T) {
	report := (StatusReport{Entries: []StatusEntry{{Pack: Pack{ID: "matty"}, Surface: SurfaceCodex, LatestAttempt: &AttemptStatus{Outcome: "future-value", PlanID: "plan-1"}}}}).JSONReport(true)
	if got := report.Entries[0].LatestAttempt; got == nil || got.Outcome != "unknown" || got.PlanID != "plan-1" {
		t.Fatalf("attempt = %#v", got)
	}
}

func TestStatusJSONCarriesFocusedResourceReadinessAndRequirement(t *testing.T) {
	resource := ResourceStatus{
		Resource: ResourceIdentity{Kind: "skill", ID: "shared"}, Role: ResourceRoleDependency,
		DependencyChain: []ResourceIdentity{{Kind: "command", ID: "ship"}, {Kind: "skill", ID: "shared"}},
		Readiness:       ReadinessStatus{Configured: true, Authorized: true}, ReadinessObserved: ReadinessObservationStatus{Configured: true, Authorization: true},
		Projections: ProjectionSummary{Verified: 1}, Blockers: []string{},
	}
	report := (StatusReport{
		Entries: []StatusEntry{{Pack: Pack{ID: "app"}, Surface: SurfaceCodex, Resources: []ResourceStatus{resource}}},
		Focused: &resource, Requirement: &StatusRequirement{Resource: resource.Resource, Readiness: "usable", Satisfied: false},
	}).JSONReport(true)
	if report.SchemaVersion != 5 || report.Focused == nil || report.Focused.Resource != resource.Resource || report.Focused.Readiness.Usable.State != "unknown" {
		t.Fatalf("focused JSON = %#v", report)
	}
	if report.Requirement == nil || report.Requirement.Satisfied || report.Requirement.Readiness != "usable" {
		t.Fatalf("requirement JSON = %#v", report.Requirement)
	}
	if len(report.Entries[0].Resources) != 1 || report.Entries[0].Resources[0].Blockers == nil {
		t.Fatalf("resource JSON = %#v", report.Entries[0].Resources)
	}
	packRequirement := (StatusReport{
		Entries:     []StatusEntry{{Pack: Pack{ID: "app"}, Surface: SurfaceCodex}},
		Requirement: &StatusRequirement{Readiness: "usable", Satisfied: false},
	}).JSONReport(true)
	if packRequirement.Requirement == nil || packRequirement.Requirement.Resource != nil {
		t.Fatalf("Pack requirement JSON invented a resource: %#v", packRequirement.Requirement)
	}
}
