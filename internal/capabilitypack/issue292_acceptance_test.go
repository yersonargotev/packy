package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateMigratesCustomRootAndAliasOnlyThroughSealedApprovedPlan(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex},
		RootMigrations: []RootMigration{{From: ResourceIdentity{Kind: "skill", ID: "legacy"}, To: ResourceIdentity{Kind: "skill", ID: "current"}}},
		Resources: []Resource{{
			Kind: "skill", ID: "current", Source: "current.md",
			Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "current", Invocation: "$current", Mode: "native", Sharing: "exclusive"}},
		}},
	}
	pending := SurfaceInspection{Revision: "host-1", Projections: []ObservedProjection{{
		ID: "skill:current", ObservedFingerprint: "old", DesiredFingerprint: "new",
		Action: ProjectionAction{ID: "skill:current", Description: "write current"},
	}}}
	verified := pending
	verified.Revision = "host-2"
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].ObservedFingerprint = "new"
	state := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "legacy"}}},
		Aliases:   []SurfaceAlias{{Kind: "skill", ID: "legacy", Name: "my-app"}},
	}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, verified}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(
		Catalog{packs: []Pack{pack}, allowSyntheticHistory: true},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
	)

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	wantSelection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "current"}}}
	if !reflect.DeepEqual(plan.Selection(), wantSelection) ||
		!reflect.DeepEqual(plan.Aliases(), []SurfaceAlias{{Kind: "skill", ID: "current", Name: "my-app"}}) ||
		!reflect.DeepEqual(plan.RootMigrations(), pack.RootMigrations) {
		t.Fatalf("selection=%#v aliases=%#v migrations=%#v", plan.Selection(), plan.Aliases(), plan.RootMigrations())
	}
	report := plan.JSONReport(true)
	if !reflect.DeepEqual(report.Migrations, []string{
		"resource root migrates from skill:legacy to skill:current",
		"surface-local aliases change",
	}) {
		t.Fatalf("preview migrations = %#v", report.Migrations)
	}
	if store.state.Intent.Selection.Roots[0].ID != "legacy" || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("preview mutated activation state or host")
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("unapproved migration error = %v", err)
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("unapproved migration executed effects")
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Interactive: true,
		Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.state.Intent.Selection, wantSelection) ||
		!reflect.DeepEqual(store.state.Intent.Aliases, []SurfaceAlias{{Kind: "skill", ID: "current", Name: "my-app"}}) {
		t.Fatalf("persisted intent = %#v", store.state.Intent)
	}
}

func TestUpdateRejectsMissingRootMigrationBeforeHostObservation(t *testing.T) {
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, RootMigrations: []RootMigration{}, Resources: []Resource{{Kind: "skill", ID: "current"}}}
	state := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "legacy"}}},
	}}
	facade, adapter, store := updateFixture([]Pack{pack}, state)
	facade.catalog.allowSyntheticHistory = true
	_, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err == nil || !strings.Contains(err.Error(), `selected canonical root "skill:legacy"`) {
		t.Fatalf("error = %v", err)
	}
	if len(adapter.calls) != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("missing migration reached mutation or host observation")
	}
}

func TestUpdateMigratesAliasForRenamedDerivedResourceThroughTheSealedRootMapping(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex},
		RootMigrations: []RootMigration{{From: ResourceIdentity{Kind: "skill", ID: "legacy-dependency"}, To: ResourceIdentity{Kind: "skill", ID: "current-dependency"}}},
		Resources: []Resource{
			{Kind: "skill", ID: "app", Requires: []string{"skill:current-dependency"}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "app", Invocation: "$app", Mode: "native", Sharing: "exclusive"}}},
			{Kind: "skill", ID: "current-dependency", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "current-dependency", Invocation: "$current-dependency", Mode: "native", Sharing: "exclusive"}}},
		},
	}
	state := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "app"}}},
		Aliases:   []SurfaceAlias{{Kind: "skill", ID: "legacy-dependency", Name: "dependency"}},
	}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, SurfaceInspection{Revision: "host"})
	facade.catalog.allowSyntheticHistory = true

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Selection(), state.Intent.Selection) ||
		!reflect.DeepEqual(plan.Aliases(), []SurfaceAlias{{Kind: "skill", ID: "current-dependency", Name: "dependency"}}) ||
		!reflect.DeepEqual(plan.RootMigrations(), pack.RootMigrations) {
		t.Fatalf("selection=%#v aliases=%#v migrations=%#v", plan.Selection(), plan.Aliases(), plan.RootMigrations())
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("derived alias migration preview executed effects")
	}
}

func TestUpdateMigratesAllModeAliasThroughPreviewAndApproval(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex},
		RootMigrations: []RootMigration{{From: ResourceIdentity{Kind: "skill", ID: "legacy"}, To: ResourceIdentity{Kind: "skill", ID: "current"}}},
		Resources: []Resource{{
			Kind: "skill", ID: "current",
			Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "current", Invocation: "$current", Mode: "native", Sharing: "exclusive"}},
		}},
	}
	pending := SurfaceInspection{Revision: "host-1", Projections: []ObservedProjection{{
		ID: "skill:current", ObservedFingerprint: "old", DesiredFingerprint: "new",
		Action: ProjectionAction{ID: "skill:current", Description: "write current"},
	}}}
	verified := pending
	verified.Revision = "host-2"
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].ObservedFingerprint = "new"
	state := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 5,
		Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
		Aliases:   []SurfaceAlias{{Kind: "skill", ID: "legacy", Name: "my-app"}},
	}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, pending, pending, verified)
	facade.catalog.allowSyntheticHistory = true

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Aliases(), []SurfaceAlias{{Kind: "skill", ID: "current", Name: "my-app"}}) ||
		!reflect.DeepEqual(plan.RootMigrations(), pack.RootMigrations) {
		t.Fatalf("aliases=%#v migrations=%#v", plan.Aliases(), plan.RootMigrations())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("unapproved all-mode alias migration error = %v", err)
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("unapproved all-mode alias migration executed effects")
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Interactive: true,
		Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)},
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.state.Intent.Aliases, []SurfaceAlias{{Kind: "skill", ID: "current", Name: "my-app"}}) {
		t.Fatalf("persisted aliases = %#v", store.state.Intent.Aliases)
	}
}

func TestUpdateRequiresApprovalForActionlessAllModeOperationalChanges(t *testing.T) {
	operational := func(id string) Resource {
		return Resource{
			Kind: "lifecycle", ID: id, Requires: []string{}, Conflicts: []string{}, Notices: []string{},
			ProvidesCapabilities: []string{}, RequiresCapabilities: []string{}, RequiresTools: []string{},
			CapabilityConflicts: []string{},
			Bindings:            []Binding{{Surface: SurfaceCodex, Projection: "lifecycle", Name: id, Invocation: "$" + id, Mode: "native", Sharing: "exclusive"}},
			SurfaceExclusions:   []SurfaceExclusion{},
		}
	}
	old := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Provides: []string{}, Requires: Requirements{Capabilities: []string{}, Tools: []string{}}, Conflicts: []string{},
		Resources: []Resource{operational("old")}, RootMigrations: []RootMigration{}, Contract: Contract{Exclusions: []Exclusion{}},
	}
	bundle := filepath.Join(t.TempDir(), "bundle")
	root := filepath.Join(bundle, "history", "app", old.Version)
	encoded, err := EncodePortableManifestV4(old)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "pack.json"), encoded, 0o600)
	mustWrite(t, filepath.Join(root, "artifact.json"), []byte("{}\n"), 0o600)
	decoded := mustDecodeHistoricalManifest(t, root)
	artifact, err := inspectHistoricalArtifact(root, decoded)
	if err != nil {
		t.Fatal(err)
	}
	writeHistoricalArtifact(t, root, artifact)
	trustKey := "app@1.0.0"
	trustedHistoricalAggregates[trustKey] = artifact.AggregateSHA256
	t.Cleanup(func() { delete(trustedHistoricalAggregates, trustKey) })

	target := old
	target.Version = "2.0.0"
	target.Resources = []Resource{operational("new")}
	targetManifest, err := EncodePortableManifestV4(target)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(bundle, "packs", target.ID, "pack.json"), targetManifest, 0o600)
	state := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: old.Version, Active: true, Revision: 2,
		Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
		Aliases:   []SurfaceAlias{},
	}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "host"}}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(
		Catalog{
			packs:               []Pack{target},
			bundleRoot:          bundle,
			enforceUpdateRoutes: true,
			entries: []catalogEntry{{
				ID: target.ID, Surfaces: target.Surfaces, HistoricalVersions: []string{old.Version, target.Version},
				UpdateRoutes: []UpdateRoute{{FromVersion: old.Version, ToVersion: target.Version, ExistingSurfaces: []Surface{SurfaceCodex}}},
			}},
		},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
	)

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.JSONReport(true).Migrations; !reflect.DeepEqual(got, []string{
		"all selection adds operational resource lifecycle:new",
		"all selection removes operational resource lifecycle:old",
	}) {
		t.Fatalf("all-mode contract changes = %#v", got)
	}
	if actions := flattenActions(plan.Phases()); len(actions) != 0 {
		t.Fatalf("actionless update has actions = %#v", actions)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("unapproved all-mode contract change error = %v", err)
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("unapproved all-mode contract change executed effects")
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Interactive: true,
		Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)},
	}); err != nil {
		t.Fatal(err)
	}
	if store.state.Intent.Version != target.Version {
		t.Fatalf("persisted version = %q", store.state.Intent.Version)
	}
}

func TestUpdateRejectsChangedRootMigrationFactsWithZeroEffects(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex},
		RootMigrations: []RootMigration{{From: ResourceIdentity{Kind: "skill", ID: "legacy"}, To: ResourceIdentity{Kind: "skill", ID: "current"}}},
		Resources: []Resource{
			{Kind: "skill", ID: "alternate", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "alternate", Mode: "native", Sharing: "exclusive"}}},
			{Kind: "skill", ID: "current", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "current", Mode: "native", Sharing: "exclusive"}}},
		},
	}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "skill:current", ObservedFingerprint: "old", DesiredFingerprint: "new",
		Action: ProjectionAction{ID: "skill:current"},
	}}}
	state := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "legacy"}}},
	}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, observation)
	facade.catalog.allowSyntheticHistory = true
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable() {
		t.Fatalf("preview blockers = %#v", plan.Blockers())
	}
	facade.catalog.packs[0].RootMigrations[0].To = ResourceIdentity{Kind: "skill", ID: "alternate"}
	_, err = facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Interactive: true,
		Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)},
	})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("stale migration err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}
