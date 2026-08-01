package prompt

import (
	"strings"
	"testing"
)

func TestSectionInsertUpdateRemove(t *testing.T) {
	existing := "# User notes\n\nKeep this.\n"
	inserted := upsertSection(existing, codexPackySectionID, CodexContent())
	for _, want := range []string{
		"# User notes\n\nKeep this.",
		"<!-- packy:skills-router -->",
		"~/.agents/skills",
		"ask-matt",
		"Engram memory tools",
		"host delegation rules",
		"<!-- /packy:skills-router -->",
	} {
		if !strings.Contains(inserted, want) {
			t.Fatalf("inserted prompt missing %q:\n%s", want, inserted)
		}
	}

	updated := upsertSection(inserted, codexPackySectionID, "replacement\n")
	if strings.Count(updated, "<!-- packy:skills-router -->") != 1 {
		t.Fatalf("updated prompt should have one Packy marker:\n%s", updated)
	}
	if !strings.Contains(updated, "replacement") || strings.Contains(updated, "ask-matt") {
		t.Fatalf("Packy block was not replaced surgically:\n%s", updated)
	}
	if !strings.Contains(updated, "# User notes\n\nKeep this.") {
		t.Fatalf("user content was not preserved:\n%s", updated)
	}

	removed := removeSection(updated, codexPackySectionID)
	if strings.Contains(removed, "packy:skills-router") || strings.Contains(removed, "replacement") {
		t.Fatalf("Packy block was not removed:\n%s", removed)
	}
	if removed != existing {
		t.Fatalf("remove should preserve original content exactly:\ngot:  %q\nwant: %q", removed, existing)
	}
}

func TestInspectRulesContractRecognizesOnlyExactDotsRules(t *testing.T) {
	external := exactDotsRulesFixture()

	observation := InspectRulesContract(external)
	if observation.Disposition != RulesExternallySatisfied {
		t.Fatalf("disposition = %q, want %q", observation.Disposition, RulesExternallySatisfied)
	}
	if observation.Fingerprint != RulesFingerprint() {
		t.Fatalf("fingerprint = %q, want %q", observation.Fingerprint, RulesFingerprint())
	}

	drifted := strings.Replace(external, "Keep diffs surgical", "Keep every diff surgical", 1)
	if got := InspectRulesContract(drifted).Disposition; got != RulesExternalDrift {
		t.Fatalf("drifted disposition = %q, want %q", got, RulesExternalDrift)
	}

	unmarked := strings.ReplaceAll(strings.ReplaceAll(external, "<!-- dots:rules -->\n", ""), "<!-- /dots:rules -->", "")
	if got := InspectRulesContract(unmarked).Disposition; got != RulesNoExternalProvider {
		t.Fatalf("unmarked disposition = %q, want %q", got, RulesNoExternalProvider)
	}

	malformed := strings.TrimSuffix(external, "<!-- /dots:rules -->")
	if got := InspectRulesContract(malformed).Disposition; got != RulesMalformedExternalProvider {
		t.Fatalf("malformed disposition = %q, want %q", got, RulesMalformedExternalProvider)
	}

	for name, tc := range map[string]struct {
		content       string
		wantDrift     bool
		wantMalformed bool
	}{
		"exact-before-drift":     {content: external + "\n" + drifted, wantDrift: true},
		"exact-after-drift":      {content: drifted + "\n" + external, wantDrift: true},
		"exact-before-malformed": {content: external + "\n<!-- dots:rules -->\nUnclosed.", wantMalformed: true},
		"exact-after-malformed":  {content: "<!-- dots:rules -->\nUnclosed.\n" + external, wantMalformed: true},
	} {
		t.Run(name, func(t *testing.T) {
			got := InspectRulesContract(tc.content)
			if got.Disposition != RulesExternallySatisfied || !got.Exact || got.Drift != tc.wantDrift || got.Malformed != tc.wantMalformed {
				t.Fatalf("mixed observation = %#v", got)
			}
		})
	}
}

func TestRulesContentUsesPackyOwnershipTerminology(t *testing.T) {
	content := RulesContent()
	if !strings.HasPrefix(content, "## Packy Agent Rules\n") {
		t.Fatalf("content does not use Packy ownership terminology:\n%s", content)
	}
	if strings.Contains(content, "Dots Agent Rules") {
		t.Fatalf("Packy-owned content claims Dots ownership:\n%s", content)
	}
}

func TestHasExactPackyRulesRequiresOneCanonicalOwnedSection(t *testing.T) {
	exact := RulesSectionContent()
	if !HasExactPackyRules(exact) {
		t.Fatal("canonical Packy rules section was not recognized")
	}
	for name, content := range map[string]string{
		"marker-only": "<!-- packy:rules -->\nrules\n<!-- /packy:rules -->",
		"drifted":     strings.Replace(exact, "Keep diffs surgical", "Keep every diff surgical", 1),
		"duplicated":  exact + "\n" + exact,
		"malformed":   strings.TrimSuffix(exact, "<!-- /packy:rules -->"),
	} {
		t.Run(name, func(t *testing.T) {
			if HasExactPackyRules(content) {
				t.Fatalf("non-canonical Packy rules were recognized:\n%s", content)
			}
		})
	}
}

func exactDotsRulesFixture() string {
	return "<!-- dots:rules -->\n" +
		strings.Replace(RulesContent(), "## Packy Agent Rules", "## Dots Agent Rules", 1) +
		"<!-- /dots:rules -->"
}

func TestSectionPreservesGentleAIAndEngramBlocks(t *testing.T) {
	existing := strings.Join([]string{
		"# User intro",
		"",
		"<!-- gentle-ai:persona -->",
		"Gentle persona.",
		"<!-- /gentle-ai:persona -->",
		"",
		"<!-- gentle-ai:engram-protocol -->",
		"Engram protocol.",
		"<!-- /gentle-ai:engram-protocol -->",
		"",
		"User footer.",
		"",
	}, "\n")

	updated := upsertSection(existing, codexPackySectionID, CodexContent())
	withoutPacky := removeSection(updated, codexPackySectionID)
	if withoutPacky != existing {
		t.Fatalf("non-Packy content changed after insert/remove:\ngot:\n%s\nwant:\n%s", withoutPacky, existing)
	}
	for _, want := range []string{"<!-- gentle-ai:persona -->", "Gentle persona.", "<!-- gentle-ai:engram-protocol -->", "Engram protocol."} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated prompt lost %q:\n%s", want, updated)
		}
	}
}

func TestSectionUpdateAndRemoveAllPackyBlocks(t *testing.T) {
	existing := "before\n" +
		"<!-- packy:skills-router -->\none\n<!-- /packy:skills-router -->" +
		"\nbetween\n" +
		"<!-- packy:skills-router -->\ntwo\n<!-- /packy:skills-router -->" +
		"\nafter"

	updated := upsertSection(existing, codexPackySectionID, "replacement\n")
	if got := strings.Count(updated, "<!-- packy:skills-router -->"); got != 1 {
		t.Fatalf("updated prompt should collapse to one Packy block, got %d:\n%s", got, updated)
	}
	if strings.Contains(updated, "one") || strings.Contains(updated, "two") {
		t.Fatalf("old Packy block content remained:\n%s", updated)
	}
	for _, want := range []string{"before\n", "\nbetween\n", "\nafter"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("outside content %q not preserved in update:\n%s", want, updated)
		}
	}

	removed := removeSection(existing, codexPackySectionID)
	want := "before\n\nbetween\n\nafter"
	if removed != want {
		t.Fatalf("remove should delete all Packy blocks and preserve intervening bytes:\ngot:  %q\nwant: %q", removed, want)
	}
}

func TestDetectExternalManagedBlocks(t *testing.T) {
	warnings := DetectExternalManagedBlocks("<!-- gentle-ai:persona -->\n<!-- /gentle-ai:persona -->\n<!-- engram:memory -->\n")
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, want gentle-ai and Engram warnings", warnings)
	}
}
