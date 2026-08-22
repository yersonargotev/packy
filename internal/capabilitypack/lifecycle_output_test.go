package capabilitypack

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestResourceGraphForExplainsRootDependencyAssetNoticeAndUnselected(t *testing.T) {
	pack := Pack{Resources: []Resource{
		{Kind: "skill", ID: "other"},
		{Kind: "skill", ID: "root", Requires: []string{"command:helper"}, Notices: []string{"notice:terms"}},
		{Kind: "command", ID: "helper", Requires: []string{"asset:script"}},
		{Kind: "asset", ID: "script"},
		{Kind: "notice", ID: "terms"},
	}}
	graph := ResourceGraphFor(pack, ResourceSelection{
		Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "root"}},
	}, true)
	got := map[string]ResourceClosureFact{}
	for _, fact := range graph.Resources {
		got[fact.Resource.String()] = fact
	}
	for id, role := range map[string]ResourceRole{
		"skill:root": ResourceRoleRoot, "command:helper": ResourceRoleDependency,
		"asset:script": ResourceRoleAsset, "notice:terms": ResourceRoleNotice,
		"skill:other": ResourceRoleUnselected,
	} {
		if got[id].Role != role {
			t.Fatalf("%s role = %q, want %q", id, got[id].Role, role)
		}
	}
	if chain := got["asset:script"].DependencyChain; len(chain) != 3 ||
		chain[0].String() != "skill:root" || chain[1].String() != "command:helper" || chain[2].String() != "asset:script" {
		t.Fatalf("asset chain = %#v", chain)
	}
}

func TestResourceGraphForAllTreatsEveryOperationalResourceAsSelectableRoot(t *testing.T) {
	pack := Pack{Resources: []Resource{
		{Kind: "command", ID: "consumer", Requires: []string{"skill:shared"}},
		{Kind: "skill", ID: "shared"},
	}}
	graph := ResourceGraphFor(pack, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, true)
	for _, fact := range graph.Resources {
		if fact.Resource.Kind != "asset" && fact.Resource.Kind != "notice" &&
			(fact.Role != ResourceRoleRoot || len(fact.DependencyChain) != 1) {
			t.Fatalf("operational resource is not selectable root: %#v", fact)
		}
	}
}

func TestSensitiveEffectOriginsIncludeOnlySurfaceCapabilityDependencies(t *testing.T) {
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "synthetic", Resources: []Resource{
		{Kind: "agent", ID: "reviewer", Permissions: []string{"network"}, Requires: []string{}},
		{Kind: "skill", ID: "workflow", Requires: []string{}, Bindings: []Binding{{
			Surface: SurfaceClaude,
			Capabilities: []SurfaceCapability{{
				Type: SurfaceCapabilityClaudeCompositeSkill,
				ClaudeCompositeSkill: &ClaudeCompositeSkillCapability{
					Dependencies: []ResourceIdentity{{Kind: "agent", ID: "reviewer"}},
					References:   []ResourceIdentity{},
				},
			}},
		}}},
	}}
	selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "workflow"}}}
	activation := []PlannedActivation{{Pack: pack, Role: ActivationRequested, Selection: selection}}

	claude := sensitiveEffectOriginsForComposition([]Pack{pack}, activation, nil, pack.ID, selection, SurfaceClaude)
	if len(claude) != 1 || claude[0].Resource != (ResourceIdentity{Kind: "agent", ID: "reviewer"}) ||
		!reflect.DeepEqual(claude[0].DependencyChain, []ResourceIdentity{{Kind: "skill", ID: "workflow"}, {Kind: "agent", ID: "reviewer"}}) ||
		!reflect.DeepEqual(claude[0].PromptAuthorities, []string{"network"}) {
		t.Fatalf("Claude sensitive effects = %#v", claude)
	}
	if codex := sensitiveEffectOriginsForComposition([]Pack{pack}, activation, nil, pack.ID, selection, SurfaceCodex); len(codex) != 0 {
		t.Fatalf("Codex sensitive effects = %#v", codex)
	}
}

func TestLifecycleContractForIsCanonicalAndSurfaceScoped(t *testing.T) {
	pack := Pack{ID: "lifecycle-fixture", Version: "1.0.0",
		Resources: []Resource{
			{Kind: "agent", ID: "reviewer", Permissions: []string{"network", "filesystem", "network"}, Bindings: []Binding{
				{Surface: SurfaceOpenCode, Projection: "agent", Name: "fixture-reviewer", Invocation: "@fixture-reviewer", Mode: "native", Sharing: "exclusive"},
				{Surface: SurfaceCodex, Projection: "agent", Name: "reviewer", Invocation: "delegate", Mode: "degraded", Degradation: "no nested delegation", Sharing: "exclusive"},
			}},
			{Kind: "skill", ID: "run", Permissions: []string{"process"}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "run", Invocation: "$run", Mode: "native", Sharing: "shared"}}},
		}, Contract: Contract{
			Exclusions:    []Exclusion{{ID: "hooks", SourcePaths: []string{"z", "a"}, Reason: "excluded"}},
			OptionalModes: []OptionalMode{{ID: "deploy", Authorities: []string{"write", "network"}, Fallback: "prompt"}},
		}}
	aliases := []SurfaceAlias{{Kind: "skill", ID: "run", Name: "z"}, {Kind: "agent", ID: "reviewer", Name: "a"}}
	got := LifecycleContractFor(pack, SurfaceCodex, aliases)
	if got.Counts != (ResourceCounts{Agents: 1, Skills: 1}) {
		t.Fatalf("counts = %#v", got.Counts)
	}
	if !reflect.DeepEqual(got.DependencyClosure, []string{}) {
		t.Fatalf("closure = %#v", got.DependencyClosure)
	}
	if len(got.Bindings) != 2 || got.Bindings[0].ID != "reviewer" || got.Bindings[0].Degradation != "no nested delegation" {
		t.Fatalf("bindings = %#v", got.Bindings)
	}
	if !reflect.DeepEqual(got.PromptAuthorities, []string{"filesystem", "network", "process", "write"}) {
		t.Fatalf("authorities = %#v", got.PromptAuthorities)
	}
	if got.Aliases[0].Kind != "agent" || !reflect.DeepEqual(got.Exclusions[0].SourcePaths, []string{"a", "z"}) {
		t.Fatalf("contract not canonical: %#v", got)
	}
	if pack.Contract.Exclusions[0].SourcePaths[0] != "z" {
		t.Fatal("derivation mutated pack")
	}
}

func TestLifecycleOutputsEncodeCollectionsAsArrays(t *testing.T) {
	contract := LifecycleContractFor(Pack{}, SurfaceCodex, nil)
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"dependency_closure", "bindings", "exclusions", "optional_modes", "prompt_authorities", "aliases"} {
		if _, ok := decoded[key].([]any); !ok {
			t.Fatalf("%s is not a JSON array: %s", key, encoded)
		}
	}
}

func TestLifecycleCompatibilityIsIndependentFromReadinessAndIntent(t *testing.T) {
	resource := func(binding *Binding, exclusion *SurfaceExclusion) Resource {
		r := Resource{Kind: "instruction", ID: "guide"}
		if binding != nil {
			r.Bindings = []Binding{*binding}
		}
		if exclusion != nil {
			r.SurfaceExclusions = []SurfaceExclusion{*exclusion}
		}
		return r
	}
	tests := []struct {
		name string
		pack Pack
		want Compatibility
	}{
		{"complete", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(&Binding{Surface: SurfaceClaude, Mode: "native"}, nil)}}, CompatibilityComplete},
		{"degraded binding", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(&Binding{Surface: SurfaceClaude, Mode: "degraded", Degradation: "fallback"}, nil)}}, CompatibilityDegraded},
		{"optional exclusion", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(nil, &SurfaceExclusion{Surface: SurfaceClaude, Mode: "optional"})}}, CompatibilityDegraded},
		{"mandatory exclusion", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(nil, &SurfaceExclusion{Surface: SurfaceClaude, Mode: "mandatory"})}}, CompatibilityBlocked},
		{"missing outcome", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}, CompatibilityBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := LifecycleContractFor(tt.pack, SurfaceClaude, nil)
			if contract.Compatibility != tt.want {
				t.Fatalf("compatibility = %q, want %q", contract.Compatibility, tt.want)
			}
		})
	}
}

func TestLifecycleCompatibilityBlocksExcludedDependencyAndRendersSurfaceExclusion(t *testing.T) {
	pack := Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{
		{Kind: "instruction", ID: "guide", Requires: []string{"lifecycle:memory"}, Bindings: []Binding{{Surface: SurfaceClaude, Mode: "native"}}},
		{Kind: "lifecycle", ID: "memory", SurfaceExclusions: []SurfaceExclusion{{Surface: SurfaceClaude, Mode: "optional", Code: "generic-lifecycle-unsupported", Reason: "requires an explicit typed hook"}}},
	}}
	contract := LifecycleContractFor(pack, SurfaceClaude, nil)
	if contract.Compatibility != CompatibilityBlocked {
		t.Fatalf("compatibility = %s", contract.Compatibility)
	}
	if len(contract.Exclusions) != 1 || contract.Exclusions[0].ID != "lifecycle:memory" || contract.Exclusions[0].Code != "generic-lifecycle-unsupported" || contract.Exclusions[0].Mode != "optional" {
		t.Fatalf("exclusions = %#v", contract.Exclusions)
	}

	pack.Resources[0].Requires = []string{}
	if got := LifecycleContractFor(pack, SurfaceClaude, nil).Compatibility; got != CompatibilityDegraded {
		t.Fatalf("independent optional exclusion = %s", got)
	}
}

func TestReconciliationPlanJSONReportIsDeterministicAndComplete(t *testing.T) {
	plan := ReconciliationPlan{id: "p", digest: "d", pack: Pack{ID: "lifecycle-report", Version: "1.0.0", manifestVersion: manifestSchemaV3}, operation: OperationActivate,
		surface: SurfaceCodex, intentRevision: 3, aliases: []SurfaceAlias{{Kind: "skill", ID: "z", Name: "z"}},
		readiness: ReadinessStatus{Configured: ReadinessFalse, Authorized: ReadinessUnknown, Usable: ReadinessUnknown}, conditions: []ReadinessCondition{{Type: ConditionRuntimeUsability, Dimension: ReadinessUsable, Value: ReadinessUnknown, Reason: ReasonRuntimeUnobservable, Message: "runtime usability cannot be observed", Evidence: []string{}, Freshness: ReadinessFreshness{ObservedAt: "2026-08-09T00:00:00Z", ValidityIdentity: "lifecycle-report/usable"}}}, pendingEvidence: []string{"z", "a"},
		blockers:            []PlanBlocker{{Kind: BlockerAlias, Subject: "z", Detail: "collision"}},
		pendingHumanActions: []string{"z", "a"}, phases: []PlanPhase{{Kind: ConsentReversibleLocal, Digest: "phase", ApprovalRequired: true,
			Actions: []ProjectionAction{{ID: "z"}, {ID: "a"}}}}}
	first, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("unstable JSON:\n%s\n%s", first, second)
	}
	got := plan.JSONReport(true)
	if got.SchemaVersion != LifecycleJSONSchemaVersion || got.Disposition != PlanMixed || got.IntentRevision != 3 {
		t.Fatalf("facts = %#v", got)
	}
	if got.Contract.Compatibility != CompatibilityBlocked || got.ExpectedReadiness.Configured != ReadinessFalse || !reflect.DeepEqual(got.PendingEvidence, []string{"a", "z"}) {
		t.Fatalf("planned lifecycle facts = %#v", got)
	}
	if len(got.Conditions) != 1 || got.Conditions[0].Value != ReadinessUnknown || got.Conditions[0].Freshness.ValidityIdentity != "lifecycle-report/usable" {
		t.Fatalf("conditions lost from lifecycle plan: %#v", got.Conditions)
	}
	failure := JSONFailureFor("apply", ErrStalePlan, &plan, nil, nil)
	if failure.SchemaVersion != LifecycleJSONSchemaVersion || failure.Plan.Contract.Compatibility != CompatibilityBlocked {
		t.Fatalf("stale failure lifecycle facts = %#v", failure)
	}
	failureJSON, err := json.Marshal(failure)
	if err != nil || !json.Valid(failureJSON) || !strings.Contains(string(failureJSON), `"compatibility":"blocked"`) {
		t.Fatalf("stale failure wire contract = %s, err=%v", failureJSON, err)
	}
	if !reflect.DeepEqual([]string{got.MandatoryActions[0].ID, got.MandatoryActions[1].ID}, []string{"z", "a"}) {
		t.Fatalf("canonical facts = %#v", got)
	}
}

func TestLifecycleReportRedactsSealedExternalHostContent(t *testing.T) {
	plan := ReconciliationPlan{pack: Pack{ID: "p", Version: "1"}, phases: []PlanPhase{
		{Kind: ConsentReversibleLocal, Actions: []ProjectionAction{{ID: "instruction:x", Content: "foreign-document"}}},
		{Kind: ConsentExecutableExternal, Actions: []ProjectionAction{{ID: "hook:x", Consent: ConsentExecutableExternal, Content: "foreign-secret", Args: []string{"mcp", "add", "--env", "TOKEN=secret", "--env=OTHER=value"}, Description: "event=SessionStart command=example-tool"}}},
	}}
	encoded, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "foreign-secret") || strings.Contains(string(encoded), "foreign-document") || strings.Contains(string(encoded), "TOKEN=secret") || strings.Contains(string(encoded), "OTHER=value") || !strings.Contains(string(encoded), "redacted") || !strings.Contains(string(encoded), "event=SessionStart command=example-tool") {
		t.Fatalf("report = %s", encoded)
	}
	cause := errors.New("apply failed for TOKEN=secret, OTHER=value, and foreign-document")
	safe := ReportSafeError(cause, &plan)
	if !errors.Is(safe, cause) || strings.Contains(safe.Error(), "secret") || strings.Contains(safe.Error(), "value") || strings.Contains(safe.Error(), "foreign-document") {
		t.Fatalf("safe error = %q", safe)
	}
	failure := JSONFailureFor("apply", cause, &plan, nil, nil)
	if strings.Contains(failure.Error, "secret") || strings.Contains(failure.Error, "value") || strings.Contains(failure.Error, "foreign-document") {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestLifecycleReportRedactsOverlappingHostPathsDeterministically(t *testing.T) {
	action := ProjectionAction{
		Source:      "/tmp/packy/source",
		Target:      "/tmp/packy/source/target",
		Command:     "/tmp/packy/source/target/command",
		Description: "copy /tmp/packy/source to /tmp/packy/source/target with /tmp/packy/source/target/command",
	}
	const want = "copy <host-path>/source to <host-path>/target with <host-path>/command"
	for i := 0; i < 100; i++ {
		if got := actionForReport(action).Description; got != want {
			t.Fatalf("redacted description changed on iteration %d: got %q, want %q", i, got, want)
		}
	}
}
