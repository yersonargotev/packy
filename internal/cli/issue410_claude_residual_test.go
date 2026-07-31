package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/claudecode"
)

func TestIssue410ClaudeDriftedSkillSurvivesInactiveDeactivationThenExactRetryRemovesIt(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	observer := &countingClaudeObserver{}
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts := Options{
		Env: MapEnv{
			"HOME": home, "XDG_CONFIG_HOME": xdg, "PATH": "",
			"PACKY_SKILLS_SOURCE": filepath.Join(bundle, "skills"),
		},
		Runner: &fakeRunner{}, Terminal: terminal,
		ClaudeLookPath: func(string) (string, error) { return "/sandbox/bin/claude", nil },
		ClaudeRunner:   observer,
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "addy", "--surface", "claude"); err != nil {
		t.Fatalf("activate Addy on Claude: %v\n%s", err, out)
	}

	const residualID = "skill:api-and-interface-design"
	target := filepath.Join(home, ".claude", "skills", "api-and-interface-design")
	skillFile := filepath.Join(target, "SKILL.md")
	exactContent, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}
	store := capabilitypack.NewFileActivationStore(capabilitypack.NewStateLayout(filepath.Join(home, ".packy")).File())
	active, err := store.Load(context.Background(), capabilitypack.SurfaceClaude)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := issue410Ownership(active.Ownership, residualID)
	if !ok || owner.AdapterProvenance == "" {
		t.Fatalf("activation did not persist sealed Claude skill ownership: %+v", owner)
	}

	if err := os.WriteFile(skillFile, append(append([]byte(nil), exactContent...), []byte("\noperator drift from the v0.1.11 case\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "addy", "--surface", "claude"); err != nil {
		t.Fatalf("deactivate with drifted Claude skill: %v\n%s", err, out)
	}

	inactive, err := store.Load(context.Background(), capabilitypack.SurfaceClaude)
	if err != nil {
		t.Fatal(err)
	}
	retained, ok := issue410Ownership(inactive.Ownership, residualID)
	if inactive.Intent.Active || !ok || !reflect.DeepEqual(retained, owner) {
		t.Fatalf("drifted Claude residual lost inactive deletion authority: intent=%+v retained=%+v want=%+v", inactive.Intent, retained, owner)
	}
	if got, err := os.ReadFile(skillFile); err != nil || reflect.DeepEqual(got, exactContent) {
		t.Fatalf("drifted Claude skill was not preserved: err=%v", err)
	}
	if len(inactive.Ownership) != 1 {
		t.Fatalf("exact Claude peers were not retired during partial physical cleanup: %+v", inactive.Ownership)
	}

	if err := os.WriteFile(skillFile, exactContent, 0o600); err != nil {
		t.Fatal(err)
	}

	catalog, err := capabilitypack.DiscoverForDurableIntents(bundle)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show("addy")
	if err != nil {
		t.Fatal(err)
	}
	provider := claudecode.NewCapabilityPackOwnershipProvider(store, map[string]capabilitypack.Pack{"addy": pack}, claudecode.NewCanonicalLayout(home), bundle)
	adapter := claudecode.NewSurfaceAdapter(bundle, claudecode.NewCanonicalLayout(home), filepath.Join(home, ".packy"), "/sandbox/bin/claude", observer, provider)
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: pack, ResidualOwnership: inactive.Ownership})
	if err != nil {
		t.Fatal(err)
	}
	projection, ok := issue410Projection(inspection.Projections, residualID)
	if !ok || projection.ObservedFingerprint != owner.Fingerprint || projection.AdapterProvenance != owner.AdapterProvenance || projection.Action.AdapterProvenance != "" {
		t.Fatalf("exact inactive residual inspection lost its bound Claude identity: found=%v observed=%q owner=%q observed_provenance=%q owner_provenance=%q delete_action_provenance=%q", ok, projection.ObservedFingerprint, owner.Fingerprint, projection.AdapterProvenance, owner.AdapterProvenance, projection.Action.AdapterProvenance)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "addy", "--surface", "claude"); err != nil {
		t.Fatalf("repeat inactive deactivation after restoring exact content: %v\n%s", err, out)
	}

	clean, err := store.Load(context.Background(), capabilitypack.SurfaceClaude)
	if err != nil {
		t.Fatal(err)
	}
	if clean.Intent.Active || len(clean.Ownership) != 0 {
		t.Fatalf("exact retry reactivated Addy or retained ownership: intent=%+v ownership=%+v", clean.Intent, clean.Ownership)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("exact residual Claude skill still exists after retry: %v", err)
	}
}

func issue410Ownership(values []capabilitypack.ProjectionOwnership, id string) (capabilitypack.ProjectionOwnership, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return capabilitypack.ProjectionOwnership{}, false
}

func issue410Projection(values []capabilitypack.ObservedProjection, id string) (capabilitypack.ObservedProjection, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return capabilitypack.ObservedProjection{}, false
}
