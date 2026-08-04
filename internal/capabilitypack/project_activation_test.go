package capabilitypack

import (
	"os"
	"path/filepath"
	"testing"
)

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
		Source: ProjectPackSourceIdentity{PackID: "pack", PackVersion: "1.0.0"},
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
