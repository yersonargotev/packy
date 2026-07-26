package claudesmoke

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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
	pack := vercelacceptance.Canonical().Pack
	rows, preflight, err := evaluateSafeRuntimeModes(pack, allSelections(pack))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 28 || !preflight {
		t.Fatalf("rows=%d typed preflight=%v", len(rows), preflight)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		key := row.ResourceID + ":" + row.ModeID
		if seen[key] || !row.FailBeforeEffects || !row.SelectionObserved || row.Invocation == "" {
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
	rows, preflight, err := evaluateSafeRuntimeModes(pack, allSelections(pack))
	if err != nil {
		t.Fatal(err)
	}
	e := VercelEvidence{
		SchemaVersion: 1, RunID: "run-262", ObservedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PackySHA: strings.Repeat("a", 40), FixtureSHA256: vercelacceptance.ExactArchiveSHA256,
		ClaudeVersion: ExactVercelClaudeVersion, ClaudeVersionOutput: ExactVercelClaudeVersionOutput, ClaudeNPMIntegrity: "sha512-redacted",
		ClaudeExecutableSHA256: strings.Repeat("c", 64), RuntimeModes: rows,
		SemanticRerun:                   vercelacceptance.SemanticRerunEvidence{FirstSHA256: strings.Repeat("e", 64), SecondSHA256: strings.Repeat("e", 64), ExactMatch: true},
		Mutation:                        vercelacceptance.MutationObservation{Root: "$SANDBOX/bundle", BeforeSHA256: strings.Repeat("f", 64), AfterSHA256: strings.Repeat("f", 64), AllowedChanges: []string{}, ChangedPaths: []string{}, ZeroMutationExact: true},
		TypedFailBeforeEffectsPreflight: preflight,
		PreflightBeforeHostSelection:    true,
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
	for _, impostor := range []string{"2.1.203", "v2.1.203 (Claude Code)", "2.1.203 (Claude Code) extra", "prefix 2.1.203 (Claude Code)"} {
		e.ClaudeVersionOutput = impostor
		if err := ValidateVercelEvidence(e); err == nil {
			t.Fatalf("accepted Claude version-output impostor %q", impostor)
		}
	}
	e.ClaudeVersionOutput = ExactVercelClaudeVersionOutput
	e.SemanticRerun.SecondSHA256 = strings.Repeat("0", 64)
	if err := ValidateVercelEvidence(e); err == nil {
		t.Fatal("accepted differing semantic rerun")
	}
	e.SemanticRerun.SecondSHA256 = e.SemanticRerun.FirstSHA256
	e.Mutation.ChangedPaths = []string{"SKILL.md"}
	if err := ValidateVercelEvidence(e); err == nil {
		t.Fatal("accepted changed path as zero mutation")
	}
	e.Mutation.ChangedPaths = []string{}
	e.RunID = ""
	if err := ValidateVercelEvidence(e); err == nil {
		t.Fatal("accepted empty run ID")
	}
	e.RunID = "run-262"
	e.ObservedAt = time.Time{}
	if err := ValidateVercelEvidence(e); err == nil {
		t.Fatal("accepted zero observation time")
	}
	e.ObservedAt = time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	e.Skills = e.Skills[:8]
	if err := ValidateVercelEvidence(e); err == nil {
		t.Fatal("accepted eight positive skills")
	}
}

func TestValidateClaudeSelectionRequestRequiresInvocationAndCompleteSkillBody(t *testing.T) {
	invocation := "/deploy production"
	skill := "---\nname: deploy\n---\nComplete instructions.\n"
	request, err := json.Marshal(map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "<command-name>/deploy</command-name>\n<command-args>production</command-args>"}},
		"system":   []any{map[string]any{"type": "text", "text": "prefix\n" + skill + "\nsuffix"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateClaudeSelectionRequest(request, invocation, skill); err != nil {
		t.Fatal(err)
	}
	if err := validateClaudeSelectionRequest(request, "/deploy preview", skill); err == nil {
		t.Fatal("accepted request without selected invocation")
	}
	if err := validateClaudeSelectionRequest(request, invocation, skill+"missing"); err == nil {
		t.Fatal("accepted request without complete SKILL.md body")
	}
	if err := validateClaudeSelectionRequest([]byte("{"), invocation, skill); err == nil {
		t.Fatal("accepted malformed request JSON")
	}
}

func TestEvaluateSafeRuntimeModesDoesNotHardcodeSelectionOrTypedFailure(t *testing.T) {
	pack := vercelacceptance.Canonical().Pack
	rows, preflight, err := evaluateSafeRuntimeModes(pack, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !preflight {
		t.Fatal("typed fail-before-effects preflight was not derived")
	}
	for _, row := range rows {
		if row.SelectionObserved {
			t.Fatalf("selection was hardcoded true: %#v", row)
		}
		if !row.FailBeforeEffects || len(row.Evidence.Requirements) != len(row.Requirements) ||
			len(row.Evidence.Authorities) != len(row.Authorities) {
			t.Fatalf("incomplete per-row runtime evidence: %#v", row)
		}
	}
}

func allSelections(pack capabilitypack.Pack) map[string]bool {
	out := map[string]bool{}
	for _, resource := range pack.Resources {
		for _, mode := range resource.RuntimeModes {
			out[resource.ID+":"+mode.ID] = true
		}
	}
	return out
}
