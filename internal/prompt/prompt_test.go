package prompt

import (
	"os"
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
	external := "<!-- dots:rules -->\n" +
		strings.Replace(RulesContent(), "## Packy Agent Rules", "## Dots Agent Rules", 1) +
		"<!-- /dots:rules -->"

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

func TestWriteCodexAddsAndRemovesRulesSection(t *testing.T) {
	path := t.TempDir() + "/AGENTS.md"
	original := "# User notes\n\nKeep this.\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write original prompt: %v", err)
	}

	if _, err := WriteCodex(path); err != nil {
		t.Fatalf("WriteCodex failed: %v", err)
	}
	updatedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated prompt: %v", err)
	}
	updated := string(updatedBytes)
	for _, want := range []string{
		"<!-- packy:skills-router -->",
		RulesSectionContent(),
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated prompt missing %q:\n%s", want, updated)
		}
	}

	if err := RemoveCodex(path); err != nil {
		t.Fatalf("RemoveCodex failed: %v", err)
	}
	removedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read removed prompt: %v", err)
	}
	if removed := string(removedBytes); removed != original {
		t.Fatalf("RemoveCodex should remove all Packy sections:\ngot:  %q\nwant: %q", removed, original)
	}
}

func TestWriteCodexUsesExactDotsRulesWithoutDuplicatingOrTakingOwnership(t *testing.T) {
	path := t.TempDir() + "/AGENTS.md"
	external := "<!-- dots:rules -->\n" +
		strings.Replace(RulesContent(), "## Packy Agent Rules", "## Dots Agent Rules", 1) +
		"<!-- /dots:rules -->"
	duplicated := external + "\n<!-- packy:rules -->\nstale Packy copy\n<!-- /packy:rules -->"
	if err := os.WriteFile(path, []byte(duplicated), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := WriteCodex(path)
	if err != nil {
		t.Fatal(err)
	}
	updatedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := string(updatedBytes)
	if !strings.Contains(updated, external) {
		t.Fatalf("external rules changed:\n%s", updated)
	}
	if strings.Contains(updated, "<!-- packy:rules -->") {
		t.Fatalf("redundant Packy rules remain:\n%s", updated)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "externally satisfied") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}

	repeated, err := WriteCodex(path)
	if err != nil {
		t.Fatal(err)
	}
	repeatedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(repeatedBytes) != updated || len(repeated.Warnings) != 1 {
		t.Fatalf("repeated update changed result: warnings=%#v\n%s", repeated.Warnings, repeatedBytes)
	}

	if err := RemoveCodex(path); err != nil {
		t.Fatal(err)
	}
	removedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(removedBytes) != external+"\n" {
		t.Fatalf("uninstall changed external rules:\n%s", removedBytes)
	}
}

func TestWriteCodexPreservesDifferingAndMalformedDotsRules(t *testing.T) {
	for name, external := range map[string]string{
		"different": "<!-- dots:rules -->\n## Dots Agent Rules\n\nDifferent.\n<!-- /dots:rules -->",
		"malformed": "<!-- dots:rules -->\n## Dots Agent Rules\n\nUnclosed.",
	} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir() + "/AGENTS.md"
			if err := os.WriteFile(path, []byte(external), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := WriteCodex(path)
			if err != nil {
				t.Fatal(err)
			}
			updatedBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			updated := string(updatedBytes)
			if !strings.Contains(updated, external) {
				t.Fatalf("external content changed:\n%s", updated)
			}
			if strings.Count(updated, "<!-- packy:rules -->") != 1 {
				t.Fatalf("Packy baseline missing or duplicated:\n%s", updated)
			}
			if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "Packy projected") {
				t.Fatalf("warnings = %#v", result.Warnings)
			}
		})
	}
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
