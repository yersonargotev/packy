package capabilitypack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// issue295Fixture is the one synthetic manifest-v4 contract used by the
// complete granular lifecycle tracer. It deliberately combines every v4 graph
// fact without making a product Pack or host adapter own the policy.
func issue295Fixture(surface Surface) (Pack, Pack, Pack) {
	binding := func(kind, id string) []Binding {
		invocation := "$" + id
		if surface == SurfaceClaude {
			invocation = "/" + id
		}
		value := Binding{
			Surface: surface, Projection: kind, Name: id,
			Invocation: invocation, Mode: "native", Sharing: "exclusive",
		}
		if kind == "command" {
			value.Invocation = "/" + id
			switch surface {
			case SurfaceOpenCode:
				value.Projection = "command"
			case SurfaceCodex:
				value.Projection = "skill"
				value.Invocation = "$" + id
				value.Mode = "degraded"
				value.Degradation = "codex-command-as-workflow-skill"
			case SurfaceClaude:
				value.Projection = "skill"
			}
		}
		return []Binding{value}
	}
	resource := func(kind, id string) Resource {
		value := Resource{
			Kind: kind, ID: id, Requires: []string{}, Conflicts: []string{},
			Notices: []string{}, ProvidesCapabilities: []string{},
			RequiresCapabilities: []string{}, RequiresTools: []string{},
			CapabilityConflicts: []string{}, Bindings: binding(kind, id),
			SurfaceExclusions: []SurfaceExclusion{}, RuntimeModes: []RuntimeMode{},
		}
		switch kind {
		case "skill", "instruction", "command", "asset", "notice":
			value.Source = kind + "s/" + id
		}
		return value
	}

	current := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "granular",
		Version:         "2.0.0",
		Surfaces:        []Surface{surface},
		Provides:        []string{},
		Requires:        Requirements{Capabilities: []string{}, Tools: []string{}},
		Conflicts:       []string{},
		RootMigrations: []RootMigration{{
			From: ResourceIdentity{Kind: "skill", ID: "legacy"},
			To:   ResourceIdentity{Kind: "skill", ID: "alpha"},
		}},
		Contract: Contract{Exclusions: []Exclusion{}},
	}
	alpha := resource("skill", "alpha")
	alpha.Requires = []string{"asset:guide", "skill:shared"}
	alpha.RequiresCapabilities = []string{"cap:storage"}
	alpha.Conflicts = []string{"command:alternate"}
	alpha.Notices = []string{"notice:terms"}
	alpha.Permissions = []string{"filesystem-read"}
	beta := resource("instruction", "beta")
	beta.RuntimeModes = nil
	beta.Requires = []string{"skill:shared"}
	shared := resource("skill", "shared")
	alternate := resource("command", "alternate")
	alternate.Conflicts = []string{"skill:alpha"}
	alternate.Arguments = CommandArguments{Mode: "none"}
	asset := resource("asset", "guide")
	asset.Bindings = []Binding{}
	asset.RuntimeModes = nil
	asset.Notices = []string{"notice:terms"}
	notice := resource("notice", "terms")
	notice.Bindings = []Binding{}
	notice.RuntimeModes = nil
	notice.Requires = []string{}
	notice.Conflicts = nil
	notice.Notices = nil
	notice.ProvidesCapabilities = []string{}
	notice.RequiresCapabilities = []string{}
	notice.RequiresTools = []string{}
	notice.CapabilityConflicts = []string{}
	notice.License = "MIT"
	notice.Attribution = "Packy synthetic acceptance fixture"
	current.Resources = []Resource{asset, alternate, beta, notice, alpha, shared}

	provider := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "storage-a", Version: "1.0.0", Surfaces: []Surface{surface},
		Provides: []string{}, Requires: Requirements{Capabilities: []string{}, Tools: []string{}},
		Conflicts: []string{}, RootMigrations: []RootMigration{},
		Contract: Contract{Exclusions: []Exclusion{}},
	}
	storage := resource("skill", "storage")
	storage.ProvidesCapabilities = []string{"cap:storage"}
	provider.Resources = []Resource{storage}

	alternateProvider := clonePack(provider)
	alternateProvider.ID = "storage-b"
	return current, provider, alternateProvider
}

func issue295LegacyFixture(current Pack) Pack {
	legacy := clonePack(current)
	legacy.Version = "1.0.0"
	legacy.RootMigrations = []RootMigration{}
	for i := range legacy.Resources {
		if legacy.Resources[i].Kind == "skill" && legacy.Resources[i].ID == "alpha" {
			legacy.Resources[i].ID = "legacy"
			for j := range legacy.Resources[i].Bindings {
				legacy.Resources[i].Bindings[j].Name = "legacy"
				legacy.Resources[i].Bindings[j].Invocation = strings.ReplaceAll(legacy.Resources[i].Bindings[j].Invocation, "alpha", "legacy")
			}
			legacy.Resources[i].Conflicts = []string{"command:alternate"}
		}
		if legacy.Resources[i].Kind == "command" && legacy.Resources[i].ID == "alternate" {
			legacy.Resources[i].Conflicts = []string{"skill:legacy"}
		}
	}
	return legacy
}

func TestIssue295CanonicalFixtureExercisesTheCompleteManifestV4Contract(t *testing.T) {
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		t.Run(string(surface), func(t *testing.T) {
			current, provider, alternateProvider := issue295Fixture(surface)
			encoded, err := EncodePortableManifestV4(current)
			if err != nil {
				t.Fatal(err)
			}
			path := writeManifestV4(t, string(encoded))
			roundTrip, err := LoadPortableManifest(path, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			reencoded, err := EncodePortableManifestV4(roundTrip)
			if err != nil || string(encoded) != string(reencoded) {
				t.Fatalf("canonical v4 round-trip changed: err=%v", err)
			}

			selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "alpha"}}}
			selected, err := selectPackResources(current, selection)
			if err != nil {
				t.Fatal(err)
			}
			var identities []string
			for _, value := range selected.Resources {
				identities = append(identities, (ResourceIdentity{Kind: value.Kind, ID: value.ID}).String())
			}
			want := []string{"asset:guide", "skill:shared", "skill:alpha", "notice:terms"}
			if !reflect.DeepEqual(identities, want) {
				t.Fatalf("selected closure=%v want=%v", identities, want)
			}
			if len(current.RootMigrations) != 1 || len(current.Resources[4].RequiresCapabilities) != 1 ||
				len(current.Resources[4].Permissions) != 1 || len(provider.Resources[0].ProvidesCapabilities) != 1 ||
				alternateProvider.ID == provider.ID {
				t.Fatalf("canonical v4 fixture lost migration, capability, provider, or authority facts")
			}
			alias := SurfaceAlias{Kind: "skill", ID: "alpha", Name: "personal-alpha"}
			contract := LifecycleContractFor(selected, surface, []SurfaceAlias{alias})
			if len(contract.Aliases) != 1 || len(contract.PromptAuthorities) != 1 ||
				contract.Counts.Assets != 1 || contract.Counts.Notices != 1 {
				t.Fatalf("canonical lifecycle contract=%+v", contract)
			}

			conflicting := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{
				{Kind: "command", ID: "alternate"}, {Kind: "skill", ID: "alpha"},
			}}
			providerResource := ResourceIdentity{Kind: "skill", ID: "storage"}
			facade := NewFacade(
				Catalog{packs: []Pack{current, provider, alternateProvider}},
				WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{surface: &fakeSurfaceAdapter{}}),
			)
			blocked, err := facade.Preview(context.Background(), ActivationRequest{
				PackID: current.ID, Surface: surface, Selection: conflicting,
				ProviderChoices: []ProviderChoice{{
					Capability: "cap:storage", ProviderPack: provider.ID, ProviderResource: &providerResource,
				}},
			})
			if err != nil || blocked.Applicable() || len(blocked.Blockers()) == 0 ||
				!strings.Contains(blocked.Blockers()[0].Detail, "conflict") {
				t.Fatalf("fixture conflict did not fail closed: plan=%+v err=%v", blocked.JSONReport(true), err)
			}
		})
	}
}

type issue295SandboxAdapter struct {
	root     string
	current  map[string]string
	failOnce string
}

func (a *issue295SandboxAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	desired := map[string]bool{}
	for _, resource := range transition.Desired.Resources {
		if resource.Kind != "asset" && resource.Kind != "notice" {
			if resource.ID == "storage" {
				continue
			}
			id := resource.ID
			for _, binding := range resource.Bindings {
				if binding.Name != "" {
					id = binding.Name
					break
				}
			}
			desired[resource.Kind+":"+id] = true
		}
	}
	known := map[string]bool{}
	for id := range desired {
		known[id] = true
	}
	for id := range a.current {
		known[id] = true
	}
	var ids []string
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	projections := make([]ObservedProjection, 0, len(ids))
	for _, id := range ids {
		target := filepath.Join(a.root, strings.ReplaceAll(id, ":", "-"))
		observed, exists := a.current[id]
		projection := ObservedProjection{
			ID: id, Exists: exists, ObservedFingerprint: observed,
			Action: ProjectionAction{ID: id, Target: target, Description: "reconcile " + id},
		}
		if desired[id] {
			projection.Goal = ProjectionPresent
			projection.DesiredFingerprint = "desired:" + id
		} else {
			projection.Goal = ProjectionAbsent
			projection.Action.Mode = ProjectionDeleteTarget
		}
		projections = append(projections, projection)
	}
	var revision []string
	for _, id := range ids {
		revision = append(revision, id+"="+a.current[id])
	}
	return SurfaceInspection{Revision: strings.Join(revision, "\n"), Projections: projections}, nil
}

func (a *issue295SandboxAdapter) ApplyProjections(_ context.Context, actions []ProjectionAction) *ProjectionActionError {
	for _, action := range actions {
		if a.failOnce == action.ID {
			a.failOnce = ""
			return &ProjectionActionError{ID: action.ID, Err: errors.New("injected sandbox write failure")}
		}
		if action.Mode == ProjectionDeleteTarget || action.Mode == ProjectionRemoveContent {
			delete(a.current, action.ID)
			if err := os.Remove(action.Target); err != nil && !os.IsNotExist(err) {
				return &ProjectionActionError{ID: action.ID, Err: err}
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(action.Target), 0o700); err != nil {
			return &ProjectionActionError{ID: action.ID, Err: err}
		}
		value := "desired:" + action.ID
		if err := os.WriteFile(action.Target, []byte(value), 0o600); err != nil {
			return &ProjectionActionError{ID: action.ID, Err: err}
		}
		a.current[action.ID] = value
	}
	return nil
}

func issue295Approvals(facade Facade, plan ReconciliationPlan) []ApprovalReceipt {
	var approvals []ApprovalReceipt
	for _, phase := range plan.Phases() {
		if phase.ApprovalRequired {
			approvals = append(approvals, facade.Approve(plan, phase.Kind))
		}
	}
	return approvals
}

func issue295Apply(t *testing.T, facade Facade, plan ReconciliationPlan) ApplyResult {
	t.Helper()
	result, err := facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Approvals: issue295Approvals(facade, plan), Interactive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestIssue295GranularLifecycleTracerRunsOnEverySupportedSurface(t *testing.T) {
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		t.Run(string(surface), func(t *testing.T) {
			current, provider, alternateProvider := issue295Fixture(surface)
			legacy := issue295LegacyFixture(current)
			adapter := &issue295SandboxAdapter{root: t.TempDir(), current: map[string]string{}}
			store := &fakeActivationStore{}
			facade := NewFacade(
				Catalog{packs: []Pack{legacy, provider, alternateProvider}, allowSyntheticHistory: true},
				WithActivation(store, map[Surface]SurfaceAdapter{surface: adapter}),
			)
			legacyRoot := ResourceIdentity{Kind: "skill", ID: "legacy"}
			betaRoot := ResourceIdentity{Kind: "instruction", ID: "beta"}
			shared := ResourceIdentity{Kind: "skill", ID: "shared"}
			providerResource := ResourceIdentity{Kind: "skill", ID: "storage"}
			choice := ProviderChoice{Capability: "cap:storage", ProviderPack: provider.ID, ProviderResource: &providerResource}

			ambiguous, err := facade.Preview(context.Background(), ActivationRequest{
				PackID: legacy.ID, Surface: surface,
				Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{legacyRoot}},
			})
			if err != nil || ambiguous.Applicable() || len(adapter.current) != 0 || len(store.saves) != 0 {
				t.Fatalf("ambiguous provider crossed boundary: plan=%+v err=%v", ambiguous.JSONReport(true), err)
			}

			activate, err := facade.Preview(context.Background(), ActivationRequest{
				PackID: legacy.ID, Surface: surface,
				Selection:       ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{legacyRoot}},
				Aliases:         []SurfaceAlias{{Kind: "skill", ID: "legacy", Name: "personal-alpha"}},
				ProviderChoices: []ProviderChoice{choice},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(activate.CapabilityRequirements()) != 1 || len(activate.ProviderChoices()) != 1 ||
				len(activate.JSONReport(true).SensitiveEffects) == 0 {
				t.Fatalf("activation omitted provider or authority facts: %+v", activate.JSONReport(true))
			}
			issue295Apply(t, facade, activate)

			additive, err := facade.Preview(context.Background(), ActivationRequest{
				PackID: legacy.ID, Surface: surface,
				Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{betaRoot}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := additive.Selection().Roots; len(got) != 2 {
				t.Fatalf("additive roots=%v", got)
			}
			issue295Apply(t, facade, additive)

			promotion, err := facade.Preview(context.Background(), ActivationRequest{
				PackID: legacy.ID, Surface: surface,
				Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{shared}},
			})
			if err != nil || promotion.NoOp() {
				t.Fatalf("dependency promotion plan noop=%t err=%v", promotion.NoOp(), err)
			}
			issue295Apply(t, facade, promotion)

			demotion, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{
				PackID: legacy.ID, Surface: surface, Resources: []ResourceIdentity{shared},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(demotion.Selection().Roots) != 2 || len(demotion.RetainedProjections()) == 0 {
				t.Fatalf("dependency demotion lost roots or shared projection: %+v", demotion.JSONReport(true))
			}
			issue295Apply(t, facade, demotion)

			facade.catalog.packs[0] = current
			update, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: current.ID, Surface: surface})
			if err != nil {
				t.Fatal(err)
			}
			if got := update.JSONReport(true).Migrations; !containsString(got, "resource root migrates from skill:legacy to skill:alpha") {
				t.Fatalf("update migration facts=%v", got)
			}
			if !update.Applicable() {
				t.Fatalf("update blockers=%+v report=%+v", update.Blockers(), update.JSONReport(true))
			}
			issue295Apply(t, facade, update)

			delete(adapter.current, "skill:personal-alpha")
			if err := os.Remove(filepath.Join(adapter.root, "skill-personal-alpha")); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			reconcile, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: current.ID, Surface: surface})
			if err != nil {
				t.Fatal(err)
			}
			beforeIntent := cloneActivationState(store.state).Intent
			issue295Apply(t, facade, reconcile)
			if !reflect.DeepEqual(store.state.Intent, beforeIntent) {
				t.Fatal("reconcile changed approved selection intent")
			}

			delete(adapter.current, "instruction:beta")
			adapter.failOnce = "instruction:beta"
			recoverySeed, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: current.ID, Surface: surface})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := facade.Apply(context.Background(), ApplyRequest{
				Plan: recoverySeed, Approvals: issue295Approvals(facade, recoverySeed), Interactive: true,
			}); err == nil || store.state.Journal == nil {
				t.Fatalf("sandbox failure did not record recovery: err=%v state=%+v", err, store.state)
			}
			recovery, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: current.ID, Surface: surface})
			if err != nil || !recovery.Recovery() {
				t.Fatalf("fresh recovery plan=%+v err=%v", recovery.JSONReport(true), err)
			}
			issue295Apply(t, facade, recovery)

			removeAlpha, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{
				PackID: current.ID, Surface: surface, Resources: []ResourceIdentity{{Kind: "skill", ID: "alpha"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			issue295Apply(t, facade, removeAlpha)
			if !store.state.Intent.Active || len(store.state.Intent.Selection.Roots) != 1 ||
				store.state.Intent.Selection.Roots[0] != betaRoot || adapter.current["skill:shared"] == "" {
				t.Fatalf("incremental deactivation lost retained selection/dependency: %+v current=%v", store.state.Intent, adapter.current)
			}

			cleanup, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{
				PackID: current.ID, Surface: surface, Resources: []ResourceIdentity{betaRoot},
			})
			if err != nil {
				t.Fatal(err)
			}
			issue295Apply(t, facade, cleanup)
			if store.state.Intent.Active || len(adapter.current) != 0 {
				t.Fatalf("final cleanup incomplete: intent=%+v current=%v", store.state.Intent, adapter.current)
			}
			entries, err := os.ReadDir(adapter.root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("sandbox projection root not clean: entries=%v err=%v", entries, err)
			}
		})
	}
}
