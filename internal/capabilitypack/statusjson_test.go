package capabilitypack

import (
	"reflect"
	"testing"
)

func TestStatusJSONDistinguishesObservedFalseFromUnknownAndSorts(t *testing.T) {
	knownFalse := StatusEntry{Pack: Pack{ID: "z", Version: "1"}, Surface: SurfaceCodex, IntentPresent: true, Intent: IntentStatus{Revision: 2}, Readiness: ReadinessStatus{Configured: ReadinessTrue, Authorized: ReadinessFalse, Usable: ReadinessUnknown}, OptionalAuthorities: []OptionalAuthorityObservation{
		{ModeID: "shipping", Authority: "deploy", State: OptionalAuthorityUnavailable, Fallback: "none"},
		{ModeID: "browser", Authority: "network", State: OptionalAuthorityAvailable, Fallback: "static evidence"},
		{ModeID: "browser", Authority: "browser", State: OptionalAuthorityUnknown, Fallback: "static evidence"},
	}, Conditions: []ReadinessCondition{{Type: ConditionRuntimeUsability, Dimension: ReadinessUsable, Value: ReadinessUnknown, Reason: ReasonRuntimeUnobservable, Message: "runtime usability cannot be observed", Evidence: []string{}, Freshness: ReadinessFreshness{ObservedAt: "2026-08-09T00:00:00Z", ValidityIdentity: "z/usable"}}}, Blockers: []string{"z", "a"}, Evidence: nil, PendingHumanActions: []string{"reload", "login"}}
	unknown := StatusEntry{Pack: Pack{ID: "a", Version: "1"}, Surface: SurfaceOpenCode, Readiness: ReadinessStatus{Configured: ReadinessTrue, Authorized: ReadinessUnknown, Usable: ReadinessUnknown}}
	report := (StatusReport{Entries: []StatusEntry{knownFalse, unknown}}).JSONReport(false)
	if report.SchemaVersion != StatusSchemaVersion {
		t.Fatalf("status schema version = %d", report.SchemaVersion)
	}
	if report.Entries[0].Pack != "a" || report.Entries[1].Pack != "z" {
		t.Fatalf("entries not sorted: %#v", report.Entries)
	}
	entry := report.Entries[1]
	if entry.Intent.State != "known" || entry.Intent.Active == nil || *entry.Intent.Active || entry.Readiness.Authorized != ReadinessFalse {
		t.Fatalf("observed false lost: %#v", entry)
	}
	if entry.Readiness.Usable != ReadinessUnknown {
		t.Fatalf("unknown lost: %#v", entry.Readiness.Usable)
	}
	if len(entry.Conditions) != 1 || entry.Conditions[0].Value != ReadinessUnknown || entry.Conditions[0].Freshness.ValidityIdentity != "z/usable" {
		t.Fatalf("conditions lost: %#v", entry.Conditions)
	}
	wantAuthorities := []JSONOptionalAuthority{
		{ModeID: "browser", Authority: "browser", State: OptionalAuthorityUnknown, Fallback: "static evidence"},
		{ModeID: "browser", Authority: "network", State: OptionalAuthorityAvailable, Fallback: "static evidence"},
		{ModeID: "shipping", Authority: "deploy", State: OptionalAuthorityUnavailable, Fallback: "none"},
	}
	if !reflect.DeepEqual(entry.OptionalAuthorities, wantAuthorities) {
		t.Fatalf("optional authorities = %#v, want %#v", entry.OptionalAuthorities, wantAuthorities)
	}
	if entry.Readiness.Authorized != ReadinessFalse {
		t.Fatalf("optional authority availability changed readiness authorization: %#v", entry.Readiness)
	}
	if !reflect.DeepEqual(entry.Blockers, []string{"a", "z"}) || !reflect.DeepEqual(entry.PendingHumanActions, []string{"login", "reload"}) || entry.Evidence == nil {
		t.Fatalf("arrays not deterministic/non-null: %#v", entry)
	}
}

func TestStatusJSONCarriesFocusedResourceReadinessAndRequirement(t *testing.T) {
	resource := ResourceStatus{
		Resource: ResourceIdentity{Kind: "skill", ID: "shared"}, Role: ResourceRoleDependency,
		DependencyChain: []ResourceIdentity{{Kind: "command", ID: "ship"}, {Kind: "skill", ID: "shared"}},
		Readiness:       ReadinessStatus{Configured: ReadinessTrue, Authorized: ReadinessTrue, Usable: ReadinessUnknown}, Conditions: []ReadinessCondition{{Type: ConditionRuntimeUsability, Dimension: ReadinessUsable, Value: ReadinessUnknown, Reason: ReasonRuntimeUnobservable, Message: "runtime usability cannot be observed", Evidence: []string{}, Freshness: ReadinessFreshness{ObservedAt: "2026-08-09T00:00:00Z", ValidityIdentity: "shared/usable"}}},
		Projections: ProjectionSummary{Verified: 1}, Blockers: []string{},
	}
	report := (StatusReport{
		Entries: []StatusEntry{{Pack: Pack{ID: "app"}, Surface: SurfaceCodex, Resources: []ResourceStatus{resource}}},
		Focused: &resource, Requirement: &StatusRequirement{Resource: resource.Resource, Readiness: "usable", Satisfied: false},
	}).JSONReport(true)
	if report.SchemaVersion != StatusSchemaVersion || report.Focused == nil || report.Focused.Resource != resource.Resource || report.Focused.Readiness.Usable != ReadinessUnknown {
		t.Fatalf("focused JSON = %#v", report)
	}
	if report.Requirement == nil || report.Requirement.Satisfied || report.Requirement.Readiness != "usable" {
		t.Fatalf("requirement JSON = %#v", report.Requirement)
	}
	if len(report.Entries[0].Resources) != 1 || report.Entries[0].Resources[0].Blockers == nil {
		t.Fatalf("resource JSON = %#v", report.Entries[0].Resources)
	}
	if !reflect.DeepEqual(report.Focused.Conditions, resource.Conditions) || !reflect.DeepEqual(report.Entries[0].Resources[0].Conditions, resource.Conditions) {
		t.Fatalf("resource conditions lost: focused=%#v resources=%#v", report.Focused.Conditions, report.Entries[0].Resources[0].Conditions)
	}
	packRequirement := (StatusReport{
		Entries:     []StatusEntry{{Pack: Pack{ID: "app"}, Surface: SurfaceCodex}},
		Requirement: &StatusRequirement{Readiness: "usable", Satisfied: false},
	}).JSONReport(true)
	if packRequirement.Requirement == nil || packRequirement.Requirement.Resource != nil {
		t.Fatalf("Pack requirement JSON invented a resource: %#v", packRequirement.Requirement)
	}
}

func TestStatusJSONPreservesUnobservableExternalRequirementReason(t *testing.T) {
	condition := ReadinessCondition{
		Type: ConditionExternalRequirement, Dimension: ReadinessUsable, Value: ReadinessUnknown,
		Reason: ReasonRequirementUnobservable, Message: "external requirement engram cannot be observed",
		Evidence: []string{"executable:engram"}, Freshness: ReadinessFreshness{ObservedAt: "2026-08-09T00:00:00Z", ValidityIdentity: "engram/requirement"},
	}
	report := (StatusReport{Entries: []StatusEntry{{Pack: Pack{ID: "engram", Version: "1"}, Surface: SurfaceCodex, Conditions: []ReadinessCondition{condition}}}}).JSONReport(false)
	if len(report.Entries) != 1 || len(report.Entries[0].Conditions) != 1 || report.Entries[0].Conditions[0].Reason != ReasonRequirementUnobservable {
		t.Fatalf("condition reason lost: %#v", report.Entries)
	}
}
