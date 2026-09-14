package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue797ProjectInstallApplyRejectsOrphanCreatedAfterPreview(t *testing.T) {
	project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
	pack := Pack{
		ID: "orphan-race", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode},
		ReadinessObligations: []ReadinessObligation{}, Requires: Requirements{Tools: []string{}}, Resources: []Resource{}, Contract: Contract{OptionalModes: []OptionalMode{}},
	}
	adapter := &fakeSurfaceAdapter{}
	facade := NewFacade(Catalog{packs: []Pack{pack}})
	preview, err := facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{
		PackID: pack.ID, Surface: SurfaceCodex, ProjectRoot: project, PackyHome: packyHome, Selection: ResourceSelection{Mode: SelectionAll},
	}, adapter)
	if err != nil || preview.Disposition != ProjectInstallPreviewable {
		t.Fatalf("initial project install preview = %#v, %v", preview, err)
	}

	rootDigest, err := projectActivationRootDigest(project)
	if err != nil {
		t.Fatal(err)
	}
	resource := ResourceIdentity{Kind: "instruction", ID: "guidance"}
	detail := ProjectSensitiveDisclosure{Category: ProjectActivationTrust, Surface: SurfaceOpenCode, Resource: resource, Detail: "project-trust"}
	state := projectActivationState{
		SchemaVersion: projectActivationDocumentSchemaVersion, PackID: pack.ID, Version: pack.Version, Surface: SurfaceOpenCode,
		ProjectRootDigest: rootDigest, SensitiveLockIdentity: "orphan-lock", Active: true,
	}
	if err := saveProjectActivationRecords(packyHome, project, state,
		[]ProjectActivationApproval{{Category: ProjectActivationTrust, Digest: "approved"}},
		[]projectActivationReceipt{{Category: ProjectActivationTrust, Digest: "approved", Details: []ProjectSensitiveDisclosure{detail}}}, nil,
	); err != nil {
		t.Fatal(err)
	}

	_, err = facade.ApplyProjectInstall(context.Background(), ProjectInstallApplyRequest{Preview: preview, PackyHome: packyHome, Adapter: adapter})
	if err == nil || (!strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "stale")) {
		t.Fatalf("Apply accepted an orphan created after Preview: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, "packy.json")); !os.IsNotExist(statErr) {
		t.Fatalf("blocked Apply wrote the project manifest: %v", statErr)
	}
}

func TestIssue797FocusedProjectStatusIgnoresUnrelatedOrphan(t *testing.T) {
	project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
	installed := Pack{
		ID: "installed", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}, Requires: Requirements{Tools: []string{}},
		Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide", Description: "Guide", Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{}}},
		Contract:  Contract{OptionalModes: []OptionalMode{}},
	}
	adapter := &syntheticRequirementAdapter{target: filepath.Join(project, ".agents", "skills", "guide")}
	facade := NewFacade(Catalog{packs: []Pack{installed}})
	preview, err := facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{
		PackID: installed.ID, Surface: SurfaceCodex, ProjectRoot: project, PackyHome: packyHome, Selection: ResourceSelection{Mode: SelectionAll},
	}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := marshalProjectManifest(preview.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := marshalProjectLock(preview.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), lock, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "PACKY-NOTICES.md"), []byte(preview.noticeContent), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter.applied = true

	saveIssue797Orphan(t, packyHome, project, "unrelated", SurfaceOpenCode, []ProjectActivationEffectReceipt{{
		Action: ActionCodexProjectTrust, Surface: SurfaceOpenCode, Target: filepath.Join(t.TempDir(), "config.toml"),
		ContributionIdentity: "unrelated-contribution", AdapterProvenance: "opencode-project/v1/project-trust", StartMarker: "start", EndMarker: "end", PriorState: "absent",
	}})

	report, err := InspectProjectStatus(context.Background(), ProjectStatusRequest{
		ProjectRoot: project, PackyHome: packyHome, PackID: installed.ID, Surface: SurfaceCodex,
		Adapters: map[Surface]SurfaceAdapter{SurfaceCodex: adapter},
	})
	if err != nil {
		t.Fatalf("focused status inspected an unrelated orphan: %v", err)
	}
	if len(report.Packs) != 1 || report.Packs[0].Pack.ID != installed.ID || report.Packs[0].Surface != SurfaceCodex {
		t.Fatalf("focused status = %#v", report.Packs)
	}
}

func TestIssue797OrphanFailsProjectStatusRequirements(t *testing.T) {
	project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
	saveIssue797Orphan(t, packyHome, project, "orphan", SurfaceCodex, nil)

	for _, test := range []struct {
		name             string
		requireInstalled bool
		requireUsable    bool
		wantRequirement  string
	}{
		{name: "installed", requireInstalled: true, wantRequirement: "installed"},
		{name: "usable", requireUsable: true, wantRequirement: "usable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := InspectProjectStatus(context.Background(), ProjectStatusRequest{
				ProjectRoot: project, PackyHome: packyHome, RequireInstalled: test.requireInstalled, RequireUsable: test.requireUsable,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Packs) != 1 {
				t.Fatalf("status packs = %#v", report.Packs)
			}
			status := report.Packs[0]
			if status.Installation != ProjectInstallationAbsent || status.Requirement != test.wantRequirement || status.RequirementSatisfied {
				t.Fatalf("orphan requirement status = %#v", status)
			}
		})
	}
}

func saveIssue797Orphan(t *testing.T, packyHome, projectRoot, packID string, surface Surface, effects []ProjectActivationEffectReceipt) {
	t.Helper()
	rootDigest, err := projectActivationRootDigest(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	state := projectActivationState{
		SchemaVersion: projectActivationDocumentSchemaVersion, PackID: packID, Version: "1.0.0", Surface: surface,
		ProjectRootDigest: rootDigest, SensitiveLockIdentity: "orphan-lock", Active: true,
	}
	resource := ResourceIdentity{Kind: "instruction", ID: "guidance"}
	detail := ProjectSensitiveDisclosure{Category: ProjectActivationTrust, Surface: surface, Resource: resource, Detail: "project-trust"}
	if err := saveProjectActivationRecords(packyHome, projectRoot, state,
		[]ProjectActivationApproval{{Category: ProjectActivationTrust, Digest: "approved"}},
		[]projectActivationReceipt{{Category: ProjectActivationTrust, Digest: "approved", Details: []ProjectSensitiveDisclosure{detail}}}, effects,
	); err != nil {
		t.Fatal(err)
	}
}
