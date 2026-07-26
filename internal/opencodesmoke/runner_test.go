package opencodesmoke

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

func TestEvidenceIdentityRequiresRunIDAndUTCObservation(t *testing.T) {
	observedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if err := validateArtifactIdentity("run-262", observedAt); err != nil {
		t.Fatal(err)
	}
	if err := validateArtifactIdentity(" ", observedAt); err == nil {
		t.Fatal("accepted blank run ID")
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

func TestPreflightEveryVercelModeFailsBeforeHostEffects(t *testing.T) {
	modes, err := preflightEveryMode(vercelacceptance.Canonical().Pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 28 {
		t.Fatalf("runtime modes = %d, want 28", len(modes))
	}
	for _, mode := range modes {
		if !mode.FailBeforeHostEffects || mode.Invocation == "" ||
			len(mode.Authorities) == 0 || len(mode.Affected) == 0 {
			t.Fatalf("incomplete fail-before-effects evidence: %#v", mode)
		}
	}
}

func TestParseSkillsRequiresNameLocationAndLoadedContent(t *testing.T) {
	skills, err := parseSkills([]byte(`[
	  {"name":"vercel-optimize","location":"/tmp/vercel-optimize/SKILL.md","content":"# Optimize"},
	  {"name":"missing-content","location":"/tmp/missing/SKILL.md"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills["vercel-optimize"].content != "# Optimize" {
		t.Fatalf("skills = %#v", skills)
	}
	if _, err := parseSkills([]byte(`[]`)); err == nil {
		t.Fatal("empty native discovery evidence was accepted")
	}
}

func TestSkillBodyMatchesOpenCodeLoadedContent(t *testing.T) {
	document := "---\nname: demo\ndescription: demo\n---\n\n# Demo\n\nLoaded body.\n"
	if got := skillBody(document); got != "# Demo\n\nLoaded body." {
		t.Fatalf("skill body = %q", got)
	}
	if got := skillBody("# Plain\n"); got != "# Plain" {
		t.Fatalf("plain skill body = %q", got)
	}
	if strings.Contains(skillBody(document), "description:") {
		t.Fatal("frontmatter leaked into the expected loaded body")
	}
}

func TestCollectAvailableCommandsReadsACPUpdate(t *testing.T) {
	got := map[string]bool{}
	collectAvailableCommands(map[string]any{"method": "session/update", "params": map[string]any{"update": map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": []any{map[string]any{"name": "/vercel-deploy"}, map[string]any{"name": "vercel-logs"}}}}}, got)
	for _, name := range []string{"vercel-deploy", "vercel-logs"} {
		if !got[name] {
			t.Fatalf("ACP command %q not collected: %#v", name, got)
		}
	}
}
