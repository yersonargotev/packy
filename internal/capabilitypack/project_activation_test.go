package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectActivationRecognizesEverySupportedSurface(t *testing.T) {
	facade := NewFacade(Catalog{})
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		_, err := facade.PreviewProjectActivation(context.Background(), ProjectActivationRequest{Surface: surface})
		if err == nil || strings.Contains(err.Error(), "supports only") {
			t.Fatalf("%s activation qualification error = %v", surface, err)
		}
	}
}

func TestProjectActivationIdentityIsCanonicalAndCheckoutLocal(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	if err := os.MkdirAll(filepath.Join(first, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "first-link")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "packy-home")
	canonical, err := projectActivationDirectory(home, first)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := projectActivationDirectory(home, filepath.Join(link, "nested", ".."))
	if err != nil {
		t.Fatal(err)
	}
	separate, err := projectActivationDirectory(home, second)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != equivalent {
		t.Fatalf("equivalent checkout paths have different identities: %q != %q", canonical, equivalent)
	}
	if canonical == separate {
		t.Fatal("separate checkouts shared personal activation identity")
	}

	moved := filepath.Join(root, "moved")
	if err := os.Rename(first, moved); err != nil {
		t.Fatal(err)
	}
	movedIdentity, err := projectActivationDirectory(home, moved)
	if err != nil {
		t.Fatal(err)
	}
	if movedIdentity == canonical {
		t.Fatal("moved checkout retained personal activation identity")
	}
}

func TestProjectActivationIdentityIsSurfaceScoped(t *testing.T) {
	resource := ResourceIdentity{Kind: "mcp_server", ID: "memory"}
	lock := ProjectLockProposal{
		Sensitive: []ProjectSensitiveDisclosure{
			{Category: ProjectActivationMCP, Surface: SurfaceCodex, Resource: resource, Detail: "codex-mcp"},
			{Category: ProjectActivationMCP, Surface: SurfaceOpenCode, Resource: resource, Detail: "opencode-mcp"},
		},
		Bindings: []LifecycleBinding{
			{Surface: SurfaceCodex, Kind: resource.Kind, ID: resource.ID, Projection: "mcp_server"},
			{Surface: SurfaceOpenCode, Kind: resource.Kind, ID: resource.ID, Projection: "mcp_server"},
		},
		Projections: []ProjectProjectionPlan{
			{Resource: resource, Target: ".codex/config.toml", Contributor: "surface:codex:pack:pack", Command: "codex-command"},
			{Resource: resource, Target: "opencode.json", Contributor: "surface:opencode:pack:pack", Command: "opencode-command"},
		},
	}
	categories := projectActivationCategories(lock, SurfaceCodex)
	if len(categories) != 1 || len(categories[0].Details) != 2 {
		t.Fatalf("Codex categories included the wrong surface: %+v", categories)
	}
	original := projectSensitiveLockIdentity(lock, categories)
	lock.Projections[1].Command = "changed-opencode-command"
	if changed := projectSensitiveLockIdentity(lock, categories); changed != original {
		t.Fatal("OpenCode-only change invalidated Codex consent")
	}
	lock.Projections[0].Command = "changed-codex-command"
	if changed := projectSensitiveLockIdentity(lock, categories); changed == original {
		t.Fatal("Codex-sensitive change retained Codex consent")
	}
}

func TestProjectActivationDocumentsAreSurfaceScoped(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	rootDigest, err := projectActivationRootDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	approval := []ProjectActivationApproval{{Category: ProjectActivationMCP, Digest: "approved"}}
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		receipt := []projectActivationReceipt{{Category: ProjectActivationMCP, Digest: "approved", Details: []ProjectSensitiveDisclosure{{Category: ProjectActivationMCP, Surface: surface, Resource: ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Detail: "command:memory"}}}}
		state := projectActivationState{SchemaVersion: projectActivationDocumentSchemaVersion, PackID: "pack", Version: "1.0.0", Surface: surface, ProjectRootDigest: rootDigest, SensitiveLockIdentity: "lock-" + string(surface)}
		if err := saveProjectActivationRecords(home, root, state, approval, receipt, nil, "clean"); err != nil {
			t.Fatal(err)
		}
	}
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		document, exists, err := loadProjectActivationDocumentForSurface(home, root, "pack", surface)
		if err != nil || !exists || document.State.Surface != surface || document.SensitiveLockIdentity != "lock-"+string(surface) {
			t.Fatalf("%s document = %+v exists=%v err=%v", surface, document, exists, err)
		}
	}
}

func TestProjectActivationSealIncludesPersonalTargetObservation(t *testing.T) {
	action := ProjectionAction{ID: "project_trust:codex", Kind: ActionCodexProjectTrust, Target: "/personal/config.toml", Version: "exact-contribution", FileMode: 0o600, Precondition: "before"}
	first := projectActivationActionObservation(action)
	action.Precondition = "changed"
	second := projectActivationActionObservation(action)
	if first == second {
		t.Fatal("personal target change retained the approved observation identity")
	}
	preview := JSONProjectActivationPreview{SchemaVersion: 1, Report: "project-activation-preview", Effects: []ProjectActivationEffectPreview{{Category: ProjectActivationTrust, Action: ActionCodexProjectTrust, Target: "<codex-home>/config.toml", Identity: action.Version, Observation: first}}}
	approved := sealProjectActivationPreview(preview)
	preview.Effects[0].Observation = second
	if sealProjectActivationPreview(preview) == approved {
		t.Fatal("personal target change retained the activation preview seal")
	}
}

func TestProjectSensitiveDisclosuresIncludeEveryResourceToolRequirement(t *testing.T) {
	pack := Pack{Resources: []Resource{
		{Kind: "skill", ID: "helper", RequiresTools: []string{"helper-cli"}},
		{Kind: "lifecycle", ID: "hook", RequiresTools: []string{"hook-cli"}},
	}}

	disclosures := projectSensitiveDisclosures(pack, SurfaceCodex)
	want := []ProjectSensitiveDisclosure{
		{Category: ProjectActivationExternalRequirements, Surface: SurfaceCodex, Resource: ResourceIdentity{Kind: "lifecycle", ID: "hook"}, Detail: "tool:hook-cli"},
		{Category: ProjectActivationExternalRequirements, Surface: SurfaceCodex, Resource: ResourceIdentity{Kind: "skill", ID: "helper"}, Detail: "tool:helper-cli"},
	}
	if len(disclosures) != len(want) {
		t.Fatalf("resource tool disclosures = %+v, want %+v", disclosures, want)
	}
	for i := range want {
		if disclosures[i] != want[i] {
			t.Fatalf("resource tool disclosure %d = %+v, want %+v", i, disclosures[i], want[i])
		}
	}
}

func TestProjectSensitiveDisclosuresIncludeUnmatchedPackToolRequirement(t *testing.T) {
	pack := Pack{
		ID:       "helper-pack",
		Requires: Requirements{Tools: []string{"pack-cli"}},
		Resources: []Resource{
			{Kind: "skill", ID: "helper"},
		},
	}

	disclosures := projectSensitiveDisclosures(pack, SurfaceCodex)
	want := ProjectSensitiveDisclosure{
		Category: ProjectActivationExternalRequirements,
		Surface:  SurfaceCodex,
		Resource: ResourceIdentity{Kind: "pack", ID: "helper-pack"},
		Detail:   "tool:pack-cli",
	}
	if len(disclosures) != 1 || disclosures[0] != want {
		t.Fatalf("pack tool disclosures = %+v, want %+v", disclosures, want)
	}
}
