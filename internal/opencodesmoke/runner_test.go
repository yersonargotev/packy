package opencodesmoke

import (
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

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
