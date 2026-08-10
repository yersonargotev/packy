package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFacadeControlledCheckRecordsCurrentResultsAndGatesUsability(t *testing.T) {
	pack := controlledCheckTestPack("app")
	state := ActivationState{Intent: ActivationIntent{PackID: pack.ID, Version: pack.Version, Surface: SurfaceCodex, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}}, Ownership: []ProjectionOwnership{{ID: "skill:guide", ProjectionID: "skill:guide", PackID: pack.ID, Surface: SurfaceCodex, Fingerprint: "exact"}}}
	observation := controlledCheckTestObservation("codex-v1", "1.2.3")
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	home := t.TempDir()
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(&fakeActivationStore{state: state}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithControlledCheckEvidence(NewFileControlledCheckStore(home)))
	request := ControlledCheckRequest{PackID: pack.ID, Surface: SurfaceCodex, PackyHome: home}
	preview, err := facade.PreviewControlledCheck(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CurrentEvidence.State != ControlledCheckUnknown || preview.AdapterVersion != "codex/v2" || preview.HostVersion != "1.2.3" || len(preview.Resources) != 1 || len(preview.Instructions) == 0 {
		t.Fatalf("preview = %#v", preview)
	}
	unknown, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, Resource: "skill:guide", RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Focused == nil || unknown.Focused.Readiness.Usable != ReadinessUnknown || unknown.Requirement.Satisfied || !hasReadinessReason(unknown.Focused.Conditions, ReasonRuntimeUnobservable) {
		t.Fatalf("unknown controlled check did not fail focused strict gate: %#v %#v", unknown.Focused, unknown.Requirement)
	}
	if _, err := facade.RecordControlledCheck(context.Background(), preview, ReadinessTrue); err != nil {
		t.Fatal(err)
	}
	report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if entry.ControlledCheck.State != ControlledCheckCurrent || entry.ControlledCheck.Result != ReadinessTrue || !report.Requirement.Satisfied || entry.Readiness.Usable != ReadinessTrue {
		t.Fatalf("positive controlled check did not satisfy strict gate: %#v %#v", entry, report.Requirement)
	}
	focused, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, Resource: "skill:guide", RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	if focused.Focused == nil || focused.Focused.Readiness.Usable != ReadinessTrue || !focused.Requirement.Satisfied || !hasReadinessReason(focused.Focused.Conditions, ReasonRuntimeConfirmed) {
		t.Fatalf("positive controlled check did not satisfy focused strict gate: %#v %#v", focused.Focused, focused.Requirement)
	}
	if _, err := facade.RecordControlledCheck(context.Background(), preview, ReadinessFalse); err != nil {
		t.Fatal(err)
	}
	report, err = facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Entries[0].Readiness.Usable != ReadinessFalse || report.Requirement.Satisfied {
		t.Fatalf("negative controlled check did not fail strict gate: %#v %#v", report.Entries[0], report.Requirement)
	}
	focused, err = facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, Resource: "skill:guide", RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	if focused.Focused == nil || focused.Focused.Readiness.Usable != ReadinessFalse || focused.Requirement.Satisfied || !hasReadinessReason(focused.Focused.Conditions, ReasonRuntimeRejected) {
		t.Fatalf("negative controlled check did not fail focused strict gate: %#v %#v", focused.Focused, focused.Requirement)
	}
}

func TestFacadeControlledCheckRejectsStalePreviewAndReportsStaleEvidence(t *testing.T) {
	pack := controlledCheckTestPack("app")
	state := ActivationState{Intent: ActivationIntent{PackID: pack.ID, Version: pack.Version, Surface: SurfaceCodex, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}}, Ownership: []ProjectionOwnership{{ID: "skill:guide", ProjectionID: "skill:guide", PackID: pack.ID, Surface: SurfaceCodex, Fingerprint: "exact"}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{controlledCheckTestObservation("codex-v1", "1.2.3"), controlledCheckTestObservation("codex-v2", "1.2.3")}}
	home := t.TempDir()
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(&fakeActivationStore{state: state}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithControlledCheckEvidence(NewFileControlledCheckStore(home)))
	preview, err := facade.PreviewControlledCheck(context.Background(), ControlledCheckRequest{PackID: pack.ID, Surface: SurfaceCodex, PackyHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.RecordControlledCheck(context.Background(), preview, ReadinessTrue); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale preview error = %v", err)
	}
	// Record a current identity, then make only the host identity stale.
	adapter.observations = []SurfaceInspection{controlledCheckTestObservation("codex-v2", "1.2.3")}
	preview, err = facade.PreviewControlledCheck(context.Background(), ControlledCheckRequest{PackID: pack.ID, Surface: SurfaceCodex, PackyHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.RecordControlledCheck(context.Background(), preview, ReadinessTrue); err != nil {
		t.Fatal(err)
	}
	adapter.observations = []SurfaceInspection{controlledCheckTestObservation("codex-v2", "1.2.4")}
	report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Entries[0].ControlledCheck.State != ControlledCheckStale || report.Entries[0].Readiness.Usable != ReadinessUnknown || report.Requirement.Satisfied || !hasReadinessReason(report.Entries[0].Conditions, ReasonRuntimeCheckStale) {
		t.Fatalf("stale controlled check = %#v", report.Entries[0])
	}
	focused, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, Resource: "skill:guide", RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	if focused.Focused == nil || focused.Focused.Readiness.Usable != ReadinessUnknown || focused.Requirement.Satisfied || !hasReadinessReason(focused.Focused.Conditions, ReasonRuntimeCheckStale) {
		t.Fatalf("stale controlled check did not fail focused strict gate: %#v %#v", focused.Focused, focused.Requirement)
	}
}

func TestControlledCheckStoreStalesEveryIdentityDimensionAndIsNonPortable(t *testing.T) {
	store := NewFileControlledCheckStore(t.TempDir())
	base := ControlledCheckIdentity{Pack: "first", PackVersion: "1.0.0", Scope: ControlledCheckGlobal, Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "guide"}}, ProjectionRevision: "projection-v1", AdapterVersion: "codex/v2", HostVersion: "1.2.3"}
	if _, err := store.Record(context.Background(), base, ReadinessTrue, time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	for name, changed := range map[string]ControlledCheckIdentity{
		"version":    {Pack: base.Pack, PackVersion: "2.0.0", Scope: base.Scope, Surface: base.Surface, Resources: base.Resources, ProjectionRevision: base.ProjectionRevision, AdapterVersion: base.AdapterVersion, HostVersion: base.HostVersion},
		"resources":  {Pack: base.Pack, PackVersion: base.PackVersion, Scope: base.Scope, Surface: base.Surface, Resources: []ResourceIdentity{{Kind: "skill", ID: "other"}}, ProjectionRevision: base.ProjectionRevision, AdapterVersion: base.AdapterVersion, HostVersion: base.HostVersion},
		"projection": {Pack: base.Pack, PackVersion: base.PackVersion, Scope: base.Scope, Surface: base.Surface, Resources: base.Resources, ProjectionRevision: "projection-v2", AdapterVersion: base.AdapterVersion, HostVersion: base.HostVersion},
		"adapter":    {Pack: base.Pack, PackVersion: base.PackVersion, Scope: base.Scope, Surface: base.Surface, Resources: base.Resources, ProjectionRevision: base.ProjectionRevision, AdapterVersion: "codex/v3", HostVersion: base.HostVersion},
		"host":       {Pack: base.Pack, PackVersion: base.PackVersion, Scope: base.Scope, Surface: base.Surface, Resources: base.Resources, ProjectionRevision: base.ProjectionRevision, AdapterVersion: base.AdapterVersion, HostVersion: "1.2.4"},
	} {
		t.Run(name, func(t *testing.T) {
			status, err := store.Status(context.Background(), changed)
			if err != nil || status.State != ControlledCheckStale {
				t.Fatalf("status = %#v, %v", status, err)
			}
		})
	}
	secondPack := base
	secondPack.Pack = "second"
	status, err := store.Status(context.Background(), secondPack)
	if err != nil || status.State != ControlledCheckUnknown {
		t.Fatalf("evidence crossed synthetic Pack identities: %#v, %v", status, err)
	}
	project := base
	project.Scope, project.ProjectDigest = ControlledCheckProject, strings.Repeat("a", 64)
	status, err = store.Status(context.Background(), project)
	if err != nil || status.State != ControlledCheckUnknown {
		t.Fatalf("project evidence leaked from global state: %#v, %v", status, err)
	}
}

func TestFileControlledCheckStoreRejectsUnknownJSONFields(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "controlled-checks.json"), []byte(`{"schema_version":1,"entries":[],"secret":"no"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewFileControlledCheckStore(home).Status(context.Background(), ControlledCheckIdentity{Pack: "app", PackVersion: "1.0.0", Scope: ControlledCheckGlobal, Surface: SurfaceCodex, Resources: []ResourceIdentity{}, ProjectionRevision: "v1", AdapterVersion: "a", HostVersion: "h"})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func controlledCheckTestPack(id string) Pack {
	return Pack{manifestVersion: manifestSchemaV4, ID: id, Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, ReadinessObligations: []ReadinessObligation{ReadinessSurfaceAuthorization, ReadinessRuntimeUsability}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide", Description: "Guide", Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{}}}, Contract: Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}}}
}

func controlledCheckTestObservation(revision, host string) SurfaceInspection {
	return SurfaceInspection{Revision: revision, ControlledCheck: ControlledCheckDescriptor{AdapterVersion: "codex/v2", HostVersion: host, Instructions: []string{"Run the named Pack behavior."}}, Projections: []ObservedProjection{{ID: "skill:guide", Exists: true, ObservedFingerprint: "exact", DesiredFingerprint: "exact", Action: ProjectionAction{ID: "skill:guide", Target: "/tmp/guide"}}}, Readiness: ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: false}}
}

func hasReadinessReason(conditions []ReadinessCondition, reason ReadinessReason) bool {
	for _, condition := range conditions {
		if condition.Reason == reason {
			return true
		}
	}
	return false
}
