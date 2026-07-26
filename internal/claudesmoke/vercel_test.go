package claudesmoke

import (
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

func TestParseClaudeSkillDebugRequiresCorroboratingExactCount(t *testing.T) {
	debug := `[DEBUG] Loaded 9 unique skills (9 unconditional, 0 conditional, managed: 0, user: 9, project: 0, additional: 0, legacy commands: 0)
[DEBUG] getSkills returning: 9 skill dir commands, 0 plugin skills, 35 bundled skills, 0 builtin plugin skills`
	count, summary, err := ParseClaudeSkillDebug(debug)
	if err != nil || count != 9 || summary != "Loaded 9 unique skills; getSkills returned 9 skill dir commands" {
		t.Fatalf("got count=%d summary=%q err=%v", count, summary, err)
	}
	if _, _, err := ParseClaudeSkillDebug(strings.Replace(debug, "returning: 9", "returning: 8", 1)); err == nil {
		t.Fatal("accepted disagreeing Claude startup summaries")
	}
}

func TestVercelRuntimeEvidenceCoversExactTwentyEightModesSafely(t *testing.T) {
	rows, preflight, err := evaluateSafeRuntimeModes(vercelacceptance.Canonical().Pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 28 || !preflight {
		t.Fatalf("rows=%d typed preflight=%v", len(rows), preflight)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		key := row.ResourceID + ":" + row.ModeID
		if seen[key] || !row.FailBeforeEffects {
			t.Fatalf("bad runtime row %#v", row)
		}
		seen[key] = true
		if row.State != capabilitypack.RuntimeModeUnverified && row.State != capabilitypack.RuntimeModeAvailable {
			t.Fatalf("%s made unsafe claim %s", key, row.State)
		}
	}
}

func TestValidateVercelEvidenceExactNamesCountsAndSafety(t *testing.T) {
	pack := vercelacceptance.Canonical().Pack
	rows, preflight, err := evaluateSafeRuntimeModes(pack)
	if err != nil {
		t.Fatal(err)
	}
	e := VercelEvidence{
		SchemaVersion: 1, PackySHA: strings.Repeat("a", 40), FixtureSHA256: vercelacceptance.ExactArchiveSHA256,
		ClaudeVersion: ExactVercelClaudeVersion, ClaudeNPMIntegrity: "sha512-redacted",
		ClaudeExecutableSHA256: strings.Repeat("c", 64), RuntimeModes: rows,
		TypedFailBeforeEffectsPreflight: preflight,
		Positive:                        VercelHostObservation{UserSkillDirCommands: 9},
		MissingOne:                      VercelHostObservation{UserSkillDirCommands: 8},
		Safety:                          VercelSafetyFacts{true, true, true, true},
	}
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface == capabilitypack.SurfaceClaude {
				e.Skills = append(e.Skills, VercelSkillEvidence{binding.Name, binding.Invocation, strings.Repeat("d", 64)})
			}
		}
	}
	for _, skill := range e.Skills {
		e.Positive.Names = append(e.Positive.Names, skill.Name)
	}
	e.MissingOne.Names = append(e.MissingOne.Names, e.Positive.Names[1:]...)
	if err := ValidateVercelEvidence(e); err != nil {
		t.Fatal(err)
	}
	e.Skills = e.Skills[:8]
	if err := ValidateVercelEvidence(e); err == nil {
		t.Fatal("accepted eight positive skills")
	}
}
