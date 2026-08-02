package capabilitypack

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestVerifiedExternalSetupRecordsVersionedReversibleReceipt(t *testing.T) {
	pack := Pack{
		ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Requires:  Requirements{Capabilities: []string{}, Tools: []string{"engram"}},
		Resources: []Resource{{Kind: "mcp_server", ID: "engram", Command: "engram"}},
	}
	pending := SurfaceInspection{Revision: "before", Projections: []ObservedProjection{{
		ID: "external_setup:engram:codex:mcp", Exists: false, ObservedFingerprint: "missing", ExactFingerprint: "missing",
		DesiredFingerprint: "engram-codex-mcp-v1", ExternallyManaged: true, AdapterProvenance: "codex-engram-setup/v1/mcp",
		Action: ProjectionAction{ID: "external_setup:engram:codex:mcp", Kind: ActionCodexMCPConfig},
	}}}
	verified := pending
	verified.Revision = "after"
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].Exists = true
	verified.Projections[0].ObservedFingerprint = "engram-codex-mcp-v1"
	verified.Projections[0].ExactFingerprint = "exact-created-mcp"

	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, pending, verified}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
		WithExternalEffects(&fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}, &fakeExternalExecutor{}),
	)
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true}); err != nil {
		t.Fatal(err)
	}

	if len(store.state.External) != 1 || store.state.External[0].Receipt == nil {
		t.Fatalf("external effect = %#v", store.state.External)
	}
	receipt := store.state.External[0].Receipt
	if receipt.SchemaVersion != 1 || receipt.EffectID != "external:engram:setup:codex" || receipt.Surface != SurfaceCodex ||
		receipt.Reversal.Consent != ConsentDestructiveCleanup || len(receipt.Contributions) != 1 ||
		receipt.Contributions[0].ID != "external_setup:engram:codex:mcp" || receipt.Contributions[0].ObservedFingerprint != "exact-created-mcp" {
		t.Fatalf("receipt = %#v", receipt)
	}
}

func TestDeactivationReversesAndRetiresOnlyExactReceiptBackedSetup(t *testing.T) {
	pack := Pack{
		ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Requires:  Requirements{Capabilities: []string{}, Tools: []string{"engram"}},
		Resources: []Resource{{Kind: "mcp_server", ID: "engram", Command: "engram"}},
	}
	removal := SurfaceInspection{Revision: "configured", Projections: []ObservedProjection{{
		ID: "external_setup:engram:codex:mcp", Exists: true, ObservedFingerprint: "engram-codex-mcp-v1", ExactFingerprint: "exact-created-mcp",
		ExternallyManaged: true, AdapterProvenance: "codex-engram-setup/v1/mcp",
		Action: ProjectionAction{ID: "external_setup:engram:codex:mcp", Kind: ActionCodexMCPConfig, Mode: ProjectionRemoveContent, Description: "remove exact receipted Engram MCP contribution"},
	}}}
	absent := removal
	absent.Revision = "removed"
	absent.Projections = append([]ObservedProjection(nil), removal.Projections...)
	absent.Projections[0].Exists = false
	absent.Projections[0].ObservedFingerprint = "missing"
	absent.Projections[0].ExactFingerprint = "missing"

	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{removal, removal, removal, absent}}
	store := &fakeActivationStore{state: ActivationState{
		SchemaVersion: 3,
		Intent:        ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}},
		External: []ExternalEffect{{ID: "external:engram:setup:codex", Fingerprint: "setup-fingerprint", Receipt: &ExternalEffectReceipt{
			SchemaVersion: 1, EffectID: "external:engram:setup:codex", EffectFingerprint: "setup-fingerprint", Surface: SurfaceCodex,
			Contributors:  []string{"surface:codex:pack:engram:external:engram"},
			Contributions: []ExternalContribution{{ID: "external_setup:engram:codex:mcp", ObservedFingerprint: "exact-created-mcp", AdapterProvenance: "codex-engram-setup/v1/mcp"}},
			Reversal:      ExternalReversalContract{SchemaVersion: 1, Consent: ConsentDestructiveCleanup, AuthorityLimits: []string{"configuration only"}},
		}}},
	}}
	facade := NewFacade(Catalog{packs: []Pack{pack}},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
		WithExternalEffects(&fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}, &fakeExternalExecutor{}),
	)
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	actions := phaseActions(plan.Phases(), ConsentDestructiveCleanup)
	if len(actions) != 1 || actions[0].ID != "external_setup:engram:codex:mcp" || !plan.Applicable() {
		t.Fatalf("receipt-backed reversal plan = %#v disposition=%s", plan.Phases(), plan.Disposition())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if len(store.state.External) != 0 || len(adapter.actions) != 1 || adapter.actions[0].ID != "external_setup:engram:codex:mcp" {
		t.Fatalf("state=%#v actions=%#v", store.state.External, adapter.actions)
	}
}

func TestDeactivationPreservesFingerprintOnlyOrStaleExternalSetup(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{}, Tools: []string{"engram"}}, Resources: []Resource{{Kind: "mcp_server", ID: "engram", Command: "engram"}}}
	projection := ObservedProjection{
		ID: "external_setup:engram:codex:mcp", Exists: true, ObservedFingerprint: "contract", ExactFingerprint: "fresh-exact", ExternallyManaged: true,
		AdapterProvenance: "codex-engram-setup/v1/mcp", Action: ProjectionAction{ID: "external_setup:engram:codex:mcp", Kind: ActionCodexMCPConfig, Mode: ProjectionRemoveContent},
	}
	base := ActivationState{SchemaVersion: 3, Intent: ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}}}
	for _, tc := range []struct {
		name   string
		effect ExternalEffect
	}{
		{name: "fingerprint alone", effect: ExternalEffect{ID: "external:engram:setup:codex", Fingerprint: "legacy-only"}},
		{name: "stale exact observation", effect: ExternalEffect{ID: "external:engram:setup:codex", Fingerprint: "sealed", Receipt: &ExternalEffectReceipt{
			SchemaVersion: 1, EffectID: "external:engram:setup:codex", EffectFingerprint: "sealed", Surface: SurfaceCodex,
			Contributors:  []string{"surface:codex:pack:engram:external:engram"},
			Contributions: []ExternalContribution{{ID: projection.ID, ObservedFingerprint: "old-exact", AdapterProvenance: projection.AdapterProvenance}},
			Reversal:      ExternalReversalContract{SchemaVersion: 1, Consent: ConsentDestructiveCleanup, AuthorityLimits: []string{"configuration only"}},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := cloneActivationState(base)
			state.External = []ExternalEffect{tc.effect}
			adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "fresh", Projections: []ObservedProjection{projection}}}}
			store := &fakeActivationStore{state: state}
			facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithExternalEffects(&fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}, &fakeExternalExecutor{}))
			plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			if len(phaseActions(plan.Phases(), ConsentDestructiveCleanup)) != 0 || len(plan.PendingHumanActions()) != 1 || !strings.Contains(plan.PendingHumanActions()[0], "no complete, exact, fresh external-effect receipt") {
				t.Fatalf("unsafe reversal plan = phases:%#v pending:%#v", plan.Phases(), plan.PendingHumanActions())
			}
			if len(store.saves) != 0 || len(adapter.actions) != 0 {
				t.Fatal("preview mutated state")
			}
		})
	}
}

func TestReceiptBackedReversalFailureRetainsReceiptAndRecoveryEvidence(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{}, Tools: []string{"engram"}}, Resources: []Resource{{Kind: "mcp_server", ID: "engram", Command: "engram"}}}
	projection := ObservedProjection{ID: "external_setup:engram:codex:mcp", Exists: true, ObservedFingerprint: "contract", ExactFingerprint: "exact", ExternallyManaged: true, AdapterProvenance: "codex-engram-setup/v1/mcp", Action: ProjectionAction{ID: "external_setup:engram:codex:mcp", Kind: ActionCodexMCPConfig, Mode: ProjectionRemoveContent}}
	receipt := &ExternalEffectReceipt{SchemaVersion: 1, EffectID: "external:engram:setup:codex", EffectFingerprint: "sealed", Surface: SurfaceCodex, Contributors: []string{"surface:codex:pack:engram:external:engram"}, Contributions: []ExternalContribution{{ID: projection.ID, ObservedFingerprint: "exact", AdapterProvenance: projection.AdapterProvenance}}, Reversal: ExternalReversalContract{SchemaVersion: 1, Consent: ConsentDestructiveCleanup, AuthorityLimits: []string{"configuration only"}}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}}, External: []ExternalEffect{{ID: receipt.EffectID, Fingerprint: receipt.EffectFingerprint, Receipt: receipt}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "fresh", Projections: []ObservedProjection{projection}}, {Revision: "fresh", Projections: []ObservedProjection{projection}}, {Revision: "fresh", Projections: []ObservedProjection{projection}}}, applyErr: ProjectionActionError{ID: projection.ID, Err: errors.New("reversal interrupted")}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithExternalEffects(&fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}, &fakeExternalExecutor{}))
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true}); err == nil {
		t.Fatal("reversal failure unexpectedly succeeded")
	}
	if len(store.state.External) != 1 || store.state.External[0].Receipt == nil || store.state.Journal == nil || store.state.Journal.Outcome != AttemptRecoveryRequired || store.state.Journal.FailedAction != projection.ID {
		t.Fatalf("recovery state = %#v", store.state)
	}
}

func TestPartiallySuccessfulExternalSetupFailureCapturesReceiptAndCanResume(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceOpenCode}, Requires: Requirements{Capabilities: []string{}, Tools: []string{"engram"}}, Resources: []Resource{{Kind: "mcp_server", ID: "engram", Command: "engram"}}}
	pending := SurfaceInspection{Revision: "before", Projections: []ObservedProjection{{ID: "external_setup:engram:opencode:plugin", Exists: false, ObservedFingerprint: "missing", ExactFingerprint: "missing", DesiredFingerprint: "plugin-contract", ExternallyManaged: true, AdapterProvenance: "opencode-engram-setup/v1/plugin", Action: ProjectionAction{ID: "external_setup:engram:opencode:plugin", Kind: ActionOpenCodeAssetFile}}}}
	partial := pending
	partial.Revision = "partial"
	partial.Projections = append([]ObservedProjection(nil), pending.Projections...)
	partial.Projections[0].Exists = true
	partial.Projections[0].ObservedFingerprint = "plugin-contract"
	partial.Projections[0].ExactFingerprint = "created-plugin"
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, pending, partial}}
	store := &fakeActivationStore{}
	executor := &fakeExternalExecutor{failID: "external:engram:setup:opencode", failErr: errors.New("setup reported failure after writing plugin")}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}), WithExternalEffects(&fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}, executor))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true}); err == nil {
		t.Fatal("partial setup unexpectedly succeeded")
	}
	if store.state.Journal == nil || store.state.Journal.FailedAction != executor.failID || len(store.state.External) != 1 || store.state.External[0].Receipt == nil || len(store.state.External[0].Receipt.Contributions) != 1 {
		t.Fatalf("partial setup recovery state = %#v", store.state)
	}
	executor.failID = ""
	recovery, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if !recovery.Recovery() || len(phaseActions(recovery.Phases(), ConsentToolHostSetup)) != 1 {
		t.Fatalf("recovery plan = %#v", recovery.Phases())
	}
}

func TestDeactivationKeepsReceiptWhileAnotherPackStillContributes(t *testing.T) {
	first := Pack{ID: "engram-a", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{}, Tools: []string{"engram"}}, Resources: []Resource{{Kind: "mcp_server", ID: "engram", Command: "engram"}}}
	second := first
	second.ID = "engram-b"
	projection := ObservedProjection{ID: "external_setup:engram:codex:mcp", Exists: true, ObservedFingerprint: "contract", ExactFingerprint: "exact", DesiredFingerprint: "contract", ExternallyManaged: true, AdapterProvenance: "codex-engram-setup/v1/mcp", Action: ProjectionAction{ID: "external_setup:engram:codex:mcp", Kind: ActionCodexMCPConfig, Mode: ProjectionRemoveContent}}
	contributors := externalReceiptContributors([]Pack{first, second}, "engram", SurfaceCodex)
	receipt := &ExternalEffectReceipt{SchemaVersion: 1, EffectID: "external:engram:setup:codex", EffectFingerprint: "sealed", Surface: SurfaceCodex, Contributors: contributors, Contributions: []ExternalContribution{{ID: projection.ID, ObservedFingerprint: "exact", AdapterProvenance: projection.AdapterProvenance}}, Reversal: ExternalReversalContract{SchemaVersion: 1, Consent: ConsentDestructiveCleanup, AuthorityLimits: []string{"configuration only"}}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{Revision: 2}, Intents: []ActivationIntent{
		{PackID: first.ID, Surface: SurfaceCodex, Version: first.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}},
		{PackID: second.ID, Surface: SurfaceCodex, Version: second.Version, Active: true, Revision: 2, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}},
	}, External: []ExternalEffect{{ID: receipt.EffectID, Fingerprint: receipt.EffectFingerprint, Receipt: receipt}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "shared", Projections: []ObservedProjection{projection}}, {Revision: "shared", Projections: []ObservedProjection{projection}}, {Revision: "shared", Projections: []ObservedProjection{projection}}}}
	facade := NewFacade(Catalog{packs: []Pack{first, second}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithExternalEffects(&fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}, &fakeExternalExecutor{}))
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: first.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(phaseActions(plan.Phases(), ConsentDestructiveCleanup)) != 0 {
		t.Fatalf("shared external setup was scheduled for removal: %#v", plan.Phases())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if len(adapter.actions) != 0 || len(store.state.External) != 1 || store.state.External[0].Receipt == nil || len(store.state.External[0].Receipt.Contributors) != 1 || !strings.Contains(store.state.External[0].Receipt.Contributors[0], second.ID) {
		t.Fatalf("shared receipt state=%#v actions=%#v", store.state.External, adapter.actions)
	}
}

func TestStaleReceiptBackedDeactivationHasZeroDestructiveEffects(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{}, Tools: []string{"engram"}}, Resources: []Resource{{Kind: "mcp_server", ID: "engram", Command: "engram"}}}
	projection := ObservedProjection{ID: "external_setup:engram:codex:mcp", Exists: true, ObservedFingerprint: "contract", ExactFingerprint: "sealed-exact", ExternallyManaged: true, AdapterProvenance: "codex-engram-setup/v1/mcp", Action: ProjectionAction{ID: "external_setup:engram:codex:mcp", Kind: ActionCodexMCPConfig, Mode: ProjectionRemoveContent, Content: "TOKEN=super-secret", Args: []string{"--env", "TOKEN=super-secret"}}}
	changed := projection
	changed.ExactFingerprint = "operator-changed"
	receipt := &ExternalEffectReceipt{SchemaVersion: 1, EffectID: "external:engram:setup:codex", EffectFingerprint: "sealed", Surface: SurfaceCodex, Contributors: []string{"surface:codex:pack:engram:external:engram"}, Contributions: []ExternalContribution{{ID: projection.ID, ObservedFingerprint: projection.ExactFingerprint, AdapterProvenance: projection.AdapterProvenance}}, Reversal: ExternalReversalContract{SchemaVersion: 1, Consent: ConsentDestructiveCleanup, AuthorityLimits: []string{"configuration only"}}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}}, External: []ExternalEffect{{ID: receipt.EffectID, Fingerprint: receipt.EffectFingerprint, Receipt: receipt}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "same", Projections: []ObservedProjection{projection}}, {Revision: "same", Projections: []ObservedProjection{changed}}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithExternalEffects(&fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}, &fakeExternalExecutor{}))
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	report, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"consent":"destructive-cleanup"`, `"rollback_limits":"configuration only"`, "removes only the exact external configuration contribution"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("structured reversal output omitted %q: %s", want, report)
		}
	}
	for _, secret := range []string{"super-secret", "sealed-exact"} {
		if strings.Contains(string(report), secret) {
			t.Fatalf("structured reversal output leaked %q: %s", secret, report)
		}
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale apply error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("stale receipt crossed mutation boundary: actions=%#v saves=%d", adapter.actions, len(store.saves))
	}
}
