package capabilitypack

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

type gatewayAdapter struct {
	inspection SurfaceInspection
	inspect    func(SurfaceTransition)
}

func TestSurfaceGatewayValidatesAndClonesProjectActivationActions(t *testing.T) {
	project := t.TempDir()
	action := ProjectionAction{ID: "project_trust:codex", Surface: SurfaceCodex, Kind: ActionCodexProjectTrust, Description: "trust project", Target: filepath.Join(t.TempDir(), ".codex", "config.toml"), Content: "trusted", Version: "contribution", AdapterProvenance: "codex-project/v1/project-trust", FileMode: 0o600, Precondition: "missing", ContributionStartMarker: "start", ContributionEndMarker: "end", PreviewOnly: true}
	transition := SurfaceTransition{ProjectRoot: project, ProjectInstallation: &ProjectInstallation{Manifest: ProjectContractProposal{Packs: []ProjectManifestPack{{ID: "pack"}}}}}
	adapter := &gatewayAdapter{inspection: SurfaceInspection{ProjectActivationActions: []ProjectionAction{action}}}
	got, err := inspectSurface(context.Background(), adapter, transition)
	if err != nil {
		t.Fatal(err)
	}
	adapter.inspection.ProjectActivationActions[0].Content = "mutated"
	if len(got.ProjectActivationActions) != 1 || got.ProjectActivationActions[0].Content != "trusted" {
		t.Fatalf("personal action was not cloned: %+v", got.ProjectActivationActions)
	}
	for _, malformed := range []ProjectionAction{
		{},
		func() ProjectionAction { value := action; value.PreviewOnly = false; return value }(),
		func() ProjectionAction { value := action; value.Precondition = ""; return value }(),
		func() ProjectionAction { value := action; value.Surface = SurfaceOpenCode; return value }(),
	} {
		_, err := inspectSurface(context.Background(), &gatewayAdapter{inspection: SurfaceInspection{ProjectActivationActions: []ProjectionAction{malformed}}}, transition)
		if err == nil || !strings.Contains(err.Error(), "personal") || !strings.Contains(err.Error(), "project action") {
			t.Fatalf("malformed personal action accepted: %+v, %v", malformed, err)
		}
	}
}

func TestSurfaceGatewayAllowsOnlyExactReceiptedProjectReversals(t *testing.T) {
	target := filepath.Join(t.TempDir(), "config.toml")
	receipt := ProjectActivationEffectReceipt{Action: ActionCodexProjectTrust, Surface: SurfaceCodex, Target: target, ContributionIdentity: "contribution", AdapterProvenance: "codex-project/v1/project-trust", StartMarker: "start", EndMarker: "end", PriorState: "absent"}
	action := ProjectionAction{ID: "deactivate:codex-project-trust", Surface: SurfaceCodex, Kind: ActionCodexProjectTrust, Target: target, Content: "foreign", FileMode: 0o600, Precondition: "before", AdapterProvenance: receipt.AdapterProvenance, Consent: ConsentDestructiveCleanup, PreviewOnly: true}
	effect := ObservedProjectEffect{Kind: receipt.Action, Target: target, State: ProjectEffectExact, ObservedFingerprint: "before", AdapterProvenance: receipt.AdapterProvenance, Action: action}
	transition := SurfaceTransition{ProjectRoot: t.TempDir(), ProjectEffectReceipts: []ProjectActivationEffectReceipt{receipt}}
	got, err := inspectSurface(context.Background(), &gatewayAdapter{inspection: SurfaceInspection{ProjectDeactivationEffects: []ObservedProjectEffect{effect}}}, transition)
	if err != nil || len(got.ProjectDeactivationEffects) != 1 || got.ProjectDeactivationEffects[0].Action.Consent != ConsentDestructiveCleanup {
		t.Fatalf("exact receipted reversal = %+v, %v", got.ProjectDeactivationEffects, err)
	}
	for _, malformed := range []ObservedProjectEffect{
		func() ObservedProjectEffect { value := effect; value.State = ProjectEffectDrifted; return value }(),
		func() ObservedProjectEffect { value := effect; value.Action.Consent = ""; return value }(),
		func() ObservedProjectEffect { value := effect; value.AdapterProvenance = "other-adapter"; return value }(),
	} {
		_, err := inspectSurface(context.Background(), &gatewayAdapter{inspection: SurfaceInspection{ProjectDeactivationEffects: []ObservedProjectEffect{malformed}}}, transition)
		if err == nil {
			t.Fatalf("malformed personal project reversal accepted: %+v", malformed)
		}
	}
}

func (a *gatewayAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	if a.inspect != nil {
		a.inspect(transition)
	}
	return a.inspection, nil
}

func (*gatewayAdapter) ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError {
	return nil
}

func TestSurfaceGatewayRejectsMalformedProjectionContracts(t *testing.T) {
	present := ObservedProjection{Goal: ProjectionPresent, ID: "instruction:guide", DesiredFingerprint: "catalog", Action: ProjectionAction{ID: "instruction:guide"}}
	absent := ObservedProjection{Goal: ProjectionAbsent, ID: "instruction:guide", Action: ProjectionAction{ID: "instruction:guide", Mode: ProjectionDeleteTarget}}
	for _, tc := range []struct {
		name        string
		projections []ObservedProjection
		want        string
	}{
		{name: "zero goal", projections: []ObservedProjection{{ID: present.ID, DesiredFingerprint: present.DesiredFingerprint, Action: present.Action}}, want: "zero goal"},
		{name: "duplicate ID", projections: []ObservedProjection{present, present}, want: "duplicate projection"},
		{name: "missing action ID", projections: []ObservedProjection{{Goal: ProjectionPresent, ID: present.ID, DesiredFingerprint: present.DesiredFingerprint}}, want: "malformed projection identity"},
		{name: "missing present fingerprint", projections: []ObservedProjection{{Goal: ProjectionPresent, ID: present.ID, Action: present.Action}}, want: "incompatible present goal"},
		{name: "present removal", projections: []ObservedProjection{{Goal: ProjectionPresent, ID: present.ID, DesiredFingerprint: present.DesiredFingerprint, Action: absent.Action}}, want: "incompatible present goal"},
		{name: "absent fingerprint", projections: []ObservedProjection{{Goal: ProjectionAbsent, ID: absent.ID, DesiredFingerprint: "missing", Action: absent.Action}}, want: "incompatible absent goal"},
		{name: "absent non-removal", projections: []ObservedProjection{{Goal: ProjectionAbsent, ID: absent.ID, Action: present.Action}}, want: "incompatible absent goal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := inspectSurface(context.Background(), &gatewayAdapter{inspection: SurfaceInspection{Projections: tc.projections}}, SurfaceTransition{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %q", err, tc.want)
			}
		})
	}
}

func TestSurfaceGatewayClonesAndCanonicalizesTransitionAndInspection(t *testing.T) {
	transition := SurfaceTransition{
		Desired:             Pack{ID: "app", Resources: []Resource{{ID: "guide", Args: []string{"original"}}}},
		ResidualOwnership:   []ProjectionOwnership{{ID: "instruction:guide", PackID: "app", Surface: SurfaceCodex}},
		ResolvedExecutables: []ExecutableResolution{{Tool: "tool", AcquisitionArgs: []string{"install"}}},
		ProjectInstallation: &ProjectInstallation{Manifest: ProjectContractProposal{Packs: []ProjectManifestPack{{ID: "project-app"}}}},
	}
	adapter := &gatewayAdapter{
		inspection: SurfaceInspection{
			OccupiedNames: []OccupiedName{{Namespace: "skill", Name: "z", OwnerType: "unmanaged", Fingerprint: "z"}, {Namespace: "agent", Name: "a", OwnerType: "packy", OwnerID: "app", Fingerprint: "a"}},
			Projections: []ObservedProjection{
				{Goal: ProjectionPresent, ID: "z", DesiredFingerprint: "z", Action: ProjectionAction{ID: "z", Args: []string{"z"}}},
				{Goal: ProjectionPresent, ID: "a", DesiredFingerprint: "a", Action: ProjectionAction{ID: "a", Args: []string{"a"}}},
			},
			PendingHumanActions: []string{"z", "a"},
			Readiness:           ReadinessObservation{PendingHumanActions: []string{"z", "a"}, Evidence: []string{"z", "a"}},
		},
		inspect: func(value SurfaceTransition) {
			value.Desired.Resources[0].Args[0] = "mutated"
			value.ResidualOwnership[0].PackID = "mutated"
			value.ResolvedExecutables[0].AcquisitionArgs[0] = "mutated"
			value.ProjectInstallation.Manifest.Packs[0].ID = "mutated"
		},
	}
	got, err := inspectSurface(context.Background(), adapter, transition)
	if err != nil {
		t.Fatal(err)
	}
	adapter.inspection.Projections[0].Action.Args[0] = "mutated"
	if transition.Desired.Resources[0].Args[0] != "original" || transition.ResidualOwnership[0].PackID != "app" || transition.ResolvedExecutables[0].AcquisitionArgs[0] != "install" || transition.ProjectInstallation.Manifest.Packs[0].ID != "project-app" {
		t.Fatalf("adapter mutated gateway input: %+v", transition)
	}
	if got.Projections[0].ID != "a" || got.Projections[1].ID != "z" || got.Projections[1].Action.Args[0] != "z" || got.OccupiedNames[0].Namespace != "agent" || got.OccupiedNames[1].Name != "z" || got.PendingHumanActions[0] != "a" || got.Readiness.PendingHumanActions[0] != "a" || got.Readiness.Evidence[0] != "a" {
		t.Fatalf("gateway result was not cloned/canonicalized: %+v", got)
	}
}

func TestSurfaceGatewayRejectsMalformedOccupiedNames(t *testing.T) {
	for _, names := range [][]OccupiedName{
		{{Namespace: "skill", Name: "review", OwnerType: "unknown", Fingerprint: "x"}},
		{{Namespace: "skill", Name: "review", OwnerType: "unmanaged", Fingerprint: "x"}, {Namespace: "skill", Name: "review", OwnerType: "packy", OwnerID: "app", Fingerprint: "x"}},
	} {
		if _, err := inspectSurface(context.Background(), &gatewayAdapter{inspection: SurfaceInspection{OccupiedNames: names}}, SurfaceTransition{}); err == nil {
			t.Fatalf("occupied names were accepted: %+v", names)
		}
	}
}

func TestObservationDigestIgnoresNewGoalAndReadinessEvidence(t *testing.T) {
	if got, want := observationDigest(SurfaceInspection{Revision: "host-empty"}), "404d76e35b07f22959e16a698eb05838fa44e3edcab1ba471fc21e1f08ff73bc"; got != want {
		t.Fatalf("empty observation digest=%s want %s", got, want)
	}
	projection := ObservedProjection{Goal: ProjectionPresent, ID: "instruction:guide", Exists: true, ObservedFingerprint: "catalog", DesiredFingerprint: "catalog", Action: ProjectionAction{ID: "instruction:guide"}}
	withUnifiedFacts := SurfaceInspection{
		Revision: "host-1", Projections: []ObservedProjection{projection}, PendingHumanActions: []string{"reload"},
		Readiness: ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true, Evidence: []string{"runtime loaded"}},
	}
	legacyFacts := withUnifiedFacts
	legacyFacts.Projections = append([]ObservedProjection(nil), withUnifiedFacts.Projections...)
	legacyFacts.Projections[0].Goal = ""
	legacyFacts.Readiness = ReadinessObservation{}
	if observationDigest(withUnifiedFacts) != observationDigest(legacyFacts) {
		t.Fatal("new adapter goal/readiness facts changed the legacy stale-plan fingerprint")
	}
	changedProvenance := withUnifiedFacts
	changedProvenance.Projections = append([]ObservedProjection(nil), withUnifiedFacts.Projections...)
	withUnifiedFacts.Projections[0].AdapterProvenance = "codex-projection/v1/codex-instruction-file"
	changedProvenance.Projections[0].AdapterProvenance = "opencode-projection/v1/opencode-instruction-file"
	if observationDigest(withUnifiedFacts) == observationDigest(changedProvenance) {
		t.Fatal("authoritative adapter provenance was omitted from the stale-plan fingerprint")
	}
}
