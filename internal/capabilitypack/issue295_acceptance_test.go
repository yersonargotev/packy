package capabilitypack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	asset.Source = "skills/alpha/guide.md"
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

type issue295ProductionAdapter struct {
	binary, surface, bundleRoot, hostRoot string
	failOnce                              string
	transition                            SurfaceTransition
	appliedIDs                            []string
}

func (a *issue295ProductionAdapter) invoke(operation string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	cmd := exec.Command(a.binary, a.surface, operation, a.bundleRoot, a.hostRoot)
	cmd.Stdin = bytes.NewReader(data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("production %s adapter %s: %w: %s", a.surface, operation, err, strings.TrimSpace(stderr.String()))
	}
	if output == nil {
		return nil
	}
	return json.Unmarshal(stdout.Bytes(), output)
}

func (a *issue295ProductionAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	a.transition = transition
	var observation SurfaceInspection
	err := a.invoke("inspect", transition, &observation)
	if err != nil {
		return SurfaceInspection{}, err
	}
	const observedAt = "2000-01-01T00:00:00Z"
	normalize := func(evidence *RuntimeEvidence) {
		for i := range evidence.Requirements {
			evidence.Requirements[i].ObservedAt = observedAt
		}
		for i := range evidence.Authorities {
			evidence.Authorities[i].ObservedAt = observedAt
		}
	}
	for i := range observation.RuntimeModeEvidence {
		normalize(&observation.RuntimeModeEvidence[i].Evidence)
	}
	for i := range observation.RuntimeModeResults {
		normalize(&observation.RuntimeModeResults[i].Evidence)
	}
	return observation, nil
}

func (a *issue295ProductionAdapter) ApplyProjections(_ context.Context, actions []ProjectionAction) *ProjectionActionError {
	if a.failOnce != "" {
		for _, action := range actions {
			if action.ID == a.failOnce {
				a.failOnce = ""
				return &ProjectionActionError{ID: action.ID, Err: errors.New("injected sandbox write failure")}
			}
		}
	}
	var failure struct {
		ID, Error string
	}
	if err := a.invoke("apply", struct {
		Actions    []ProjectionAction
		Transition SurfaceTransition
	}{Actions: actions, Transition: a.transition}, &failure); err != nil {
		return &ProjectionActionError{Err: err}
	}
	if failure.Error != "" {
		return &ProjectionActionError{ID: failure.ID, Err: errors.New(failure.Error)}
	}
	for _, action := range actions {
		a.appliedIDs = append(a.appliedIDs, action.ID)
	}
	return nil
}

func issue295BuildAdapterHelper(t *testing.T) string {
	t.Helper()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	virtualSource := filepath.Join(repositoryRoot, "internal", "capabilitypack", "testdata", "issue295adapter", "main.go")
	fixtureSource := virtualSource + ".txt"
	overlay := filepath.Join(t.TempDir(), "overlay.json")
	overlayDocument, err := json.Marshal(map[string]any{"Replace": map[string]string{virtualSource: fixtureSource}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, overlayDocument, 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "issue295-adapter")
	cmd := exec.Command("go", "build", "-overlay", overlay, "-o", binary, "./internal/capabilitypack/testdata/issue295adapter")
	cmd.Dir = repositoryRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build production adapter helper: %v\n%s", err, output)
	}
	return binary
}

func issue295WriteBundle(t *testing.T, root string, packs ...Pack) {
	t.Helper()
	for _, pack := range packs {
		for _, resource := range pack.Resources {
			if resource.Source == "" {
				continue
			}
			target := filepath.Join(root, filepath.Clean(resource.Source))
			switch resource.Kind {
			case "skill":
				if err := os.MkdirAll(target, 0o700); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(target, "SKILL.md")
			default:
				if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(target, []byte("# "+resource.ID+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func issue295AppliedCount(adapter *issue295ProductionAdapter, id string) int {
	count := 0
	for _, applied := range adapter.appliedIDs {
		if applied == id {
			count++
		}
	}
	return count
}

func containsResourceIdentity(values []ResourceIdentity, target ResourceIdentity) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func issue295ProviderPath(surface Surface, hostRoot string) string {
	if surface == SurfaceClaude {
		return filepath.Join(hostRoot, ".claude", "skills", "storage")
	}
	return filepath.Join(hostRoot, "skills", "storage")
}

func issue295AssertHostClean(t *testing.T, hostRoot string) {
	t.Helper()
	forbidden := []string{"personal-alpha", "skill:storage", "# storage", "# beta", "packy:pack:beta", "pack:granular", "pack:storage-a"}
	err := filepath.WalkDir(hostRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(path)
			return fmt.Errorf("Packy-owned host link remains at %s -> %s", path, target)
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, value := range forbidden {
			if strings.Contains(string(content), value) {
				return fmt.Errorf("Packy-owned content %q remains in %s", value, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
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
	helper := issue295BuildAdapterHelper(t)
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		t.Run(string(surface), func(t *testing.T) {
			current, provider, alternateProvider := issue295Fixture(surface)
			legacy := issue295LegacyFixture(current)
			bundleRoot := t.TempDir()
			hostRoot := t.TempDir()
			issue295WriteBundle(t, bundleRoot, current, legacy, provider, alternateProvider)
			adapter := &issue295ProductionAdapter{
				binary: helper, surface: string(surface), bundleRoot: bundleRoot, hostRoot: hostRoot,
			}
			store := &fakeActivationStore{}
			facade := NewFacade(
				Catalog{packs: []Pack{legacy, provider, alternateProvider}, allowSyntheticHistory: true},
				WithActivation(store, map[Surface]SurfaceAdapter{surface: adapter}),
			)
			legacyRoot := ResourceIdentity{Kind: "skill", ID: "legacy"}
			betaRoot := ResourceIdentity{Kind: "instruction", ID: "beta"}
			sharedRoot := ResourceIdentity{Kind: "skill", ID: "shared"}
			providerResource := ResourceIdentity{Kind: "skill", ID: "storage"}
			choice := ProviderChoice{Capability: "cap:storage", ProviderPack: provider.ID, ProviderResource: &providerResource}

			ambiguous, err := facade.Preview(context.Background(), ActivationRequest{
				PackID: legacy.ID, Surface: surface,
				Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{legacyRoot}},
			})
			if err != nil || ambiguous.Applicable() || len(store.saves) != 0 {
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
			if issue295AppliedCount(adapter, "skill:storage") == 0 {
				t.Fatal("provider projection did not participate in production adapter application")
			}
			if _, err := os.Lstat(issue295ProviderPath(surface, hostRoot)); err != nil {
				t.Fatalf("provider projection missing after activation: %v", err)
			}

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
				Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{sharedRoot}},
			})
			if err != nil || promotion.NoOp() {
				t.Fatalf("dependency promotion plan noop=%t err=%v", promotion.NoOp(), err)
			}
			if got := promotion.Selection().Roots; len(got) != 3 || !containsResourceIdentity(got, sharedRoot) {
				t.Fatalf("promoted roots=%v", got)
			}
			issue295Apply(t, facade, promotion)

			demotion, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{
				PackID: legacy.ID, Surface: surface, Resources: []ResourceIdentity{sharedRoot},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := demotion.Selection().Roots; len(got) != 2 || containsResourceIdentity(got, sharedRoot) {
				t.Fatalf("demoted roots=%v", got)
			}
			retainedShared := false
			for _, retained := range demotion.RetainedProjections() {
				retainedShared = retainedShared || retained.ID == "skill:shared"
			}
			if !retainedShared {
				t.Fatalf("dependency demotion did not retain shared projection: %+v", demotion.JSONReport(true))
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

			alphaPath := filepath.Join(hostRoot, "skills", "personal-alpha")
			if surface == SurfaceClaude {
				alphaPath = filepath.Join(hostRoot, ".claude", "skills", "personal-alpha")
			}
			if err := os.RemoveAll(alphaPath); err != nil {
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

			betaPath := filepath.Join(hostRoot, "AGENTS.md")
			switch surface {
			case SurfaceOpenCode:
				betaPath = filepath.Join(hostRoot, "beta.md")
			case SurfaceClaude:
				betaPath = filepath.Join(hostRoot, ".claude", "CLAUDE.md")
			}
			if err := os.Remove(betaPath); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
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
				store.state.Intent.Selection.Roots[0] != betaRoot || len(removeAlpha.RetainedProjections()) == 0 {
				t.Fatalf("incremental deactivation lost retained selection/dependency: %+v", store.state.Intent)
			}
			cleanup, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{
				PackID: current.ID, Surface: surface, Resources: []ResourceIdentity{betaRoot},
			})
			if err != nil {
				t.Fatal(err)
			}
			issue295Apply(t, facade, cleanup)
			if store.state.Intent.Active {
				t.Fatalf("final cleanup left intent active: %+v", store.state.Intent)
			}
			if issue295AppliedCount(adapter, "skill:storage") < 2 {
				t.Fatalf("final provider cleanup did not reach production adapter: applied=%v plan=%+v", adapter.appliedIDs, cleanup.JSONReport(true))
			}
			if _, err := os.Lstat(issue295ProviderPath(surface, hostRoot)); !os.IsNotExist(err) {
				t.Fatalf("provider projection remains after final cleanup: %v", err)
			}
			issue295AssertHostClean(t, hostRoot)
		})
	}
}
