package codexsmoke

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

func TestEvidenceIdentityRequiresRunIDAndUTCObservation(t *testing.T) {
	observedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if err := validateArtifactIdentity("run-262", observedAt); err != nil {
		t.Fatal(err)
	}
	if err := validateArtifactIdentity("", observedAt); err == nil {
		t.Fatal("accepted empty run ID")
	}
	if err := validateArtifactIdentity("run-262", time.Time{}); err == nil {
		t.Fatal("accepted zero observation time")
	}
	data, err := json.Marshal(Evidence{RunID: "run-262", ObservedAt: observedAt})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"run_id":"run-262"`) || !strings.Contains(string(data), `"observed_at":"2026-07-25T12:00:00Z"`) {
		t.Fatalf("missing artifact identity in %s", data)
	}
}

func TestArtifactOwnsSemanticRerunAndExactZeroMutation(t *testing.T) {
	digest := strings.Repeat("a", 64)
	if !(vercelacceptance.SemanticRerunEvidence{FirstSHA256: digest, SecondSHA256: digest, ExactMatch: true}).Valid() {
		t.Fatal("rejected exact semantic rerun")
	}
	if (vercelacceptance.SemanticRerunEvidence{FirstSHA256: digest, SecondSHA256: strings.Repeat("b", 64), ExactMatch: true}).Valid() {
		t.Fatal("accepted differing semantic rerun digests")
	}
	if !(vercelacceptance.MutationObservation{Root: "$SANDBOX/bundle", BeforeSHA256: digest, AfterSHA256: digest, AllowedChanges: []string{}, ChangedPaths: []string{}, ZeroMutationExact: true}).Valid() {
		t.Fatal("rejected exact zero-mutation observation")
	}
	if (vercelacceptance.MutationObservation{Root: "$SANDBOX/bundle", BeforeSHA256: digest, AfterSHA256: digest, ChangedPaths: []string{"SKILL.md"}, ZeroMutationExact: true}).Valid() {
		t.Fatal("accepted a changed path as zero mutation")
	}
}

func TestResolveSelectorRequiresExactVersionAndIntegrity(t *testing.T) {
	version, integrity, err := ResolveSelector(ExactFloor, `{"version":"0.145.0","dist.integrity":"sha512-exact"}`)
	if err != nil || version != ExactFloor || integrity != "sha512-exact" {
		t.Fatalf("ResolveSelector() = %q, %q, %v", version, integrity, err)
	}
	if _, _, err := ResolveSelector("stable", `{"version":"0.145.0","dist.integrity":"sha512-exact"}`); err == nil {
		t.Fatal("moving selector was accepted")
	}
}

func TestExactFixtureProjectsNineCodexSkills(t *testing.T) {
	root := t.TempDir()
	bundle, home := filepath.Join(root, "bundle"), filepath.Join(root, "home")
	if err := materialize(bundle); err != nil {
		t.Fatal(err)
	}
	adapter := codex.NewSurfaceAdapter(bundle, filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".codex", "AGENTS.md"))
	got, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: vercelacceptance.Canonical().Pack})
	if err != nil {
		t.Fatal(err)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range got.Projections {
		if strings.HasPrefix(projection.ID, "skill:") {
			actions = append(actions, projection.Action)
		}
	}
	if len(actions) != 9 {
		t.Fatalf("got %d skill projections", len(actions))
	}
	if actionErr := adapter.ApplyProjections(context.Background(), actions); actionErr != nil {
		t.Fatal(actionErr)
	}
	for _, action := range actions {
		source, sourceErr := filepath.EvalSymlinks(action.Source)
		if same, err := filepath.EvalSymlinks(action.Target); err != nil || sourceErr != nil || same != source {
			t.Errorf("%s projection target = %q, %v", action.ID, same, err)
		}
	}
}

func TestValidatePromptInputFakeProtocol(t *testing.T) {
	fake := `[{"type":"message","content":[{"type":"input_text","text":"<skills>\n- vercel-optimize: optimize safely\n</skills>"}]},{"type":"message","content":[{"type":"input_text","text":"$vercel-optimize"}]}]`
	if err := validatePromptInput([]byte(fake), "vercel-optimize", "$vercel-optimize"); err != nil {
		t.Fatal(err)
	}
	if err := validatePromptInput([]byte(fake), "missing", "$missing"); err == nil {
		t.Fatal("fake protocol accepted a missing invocation")
	}
}
