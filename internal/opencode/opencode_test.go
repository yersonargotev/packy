package opencode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	packyprompt "github.com/yersonargotev/packy/internal/prompt"
)

func quoted(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func exactDotsRulesFixture() string {
	return "<!-- dots:rules -->\n" +
		strings.Replace(packyprompt.RulesContent(), "## Packy Agent Rules", "## Dots Agent Rules", 1) +
		"<!-- /dots:rules -->"
}

func TestWriteCreatesPromptAndConfigInstruction(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")

	result, err := Write(configPath, promptPath)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", result.Warnings)
	}
	prompt := readString(t, promptPath)
	for _, want := range []string{"~/.agents/skills", "ask-matt", "Engram memory tools", "delegation rules", packyprompt.RulesSectionContent()} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	config := readJSON(t, configPath)
	instructions := stringSlice(t, config["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v, want only %q", instructions, promptPath)
	}
}

func TestWriteUsesExactReferencedDotsRulesWithoutDuplicatingOrTakingOwnership(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	externalPath := filepath.Join(dir, "dots.md")
	external := exactDotsRulesFixture()
	if err := os.WriteFile(externalPath, []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `{"instructions":[` + quoted(externalPath) + `]}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte(packyprompt.RulesSectionContent()), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := PreviewWrite(configPath, promptPath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyWrite(plan)
	if err != nil {
		t.Fatal(err)
	}
	updatedPrompt := readString(t, promptPath)
	if strings.Contains(updatedPrompt, "<!-- packy:rules -->") {
		t.Fatalf("Packy prompt retained redundant rules:\n%s", updatedPrompt)
	}
	if !strings.Contains(updatedPrompt, "## Packy global workflow") {
		t.Fatalf("Packy workflow missing:\n%s", updatedPrompt)
	}
	if got := readString(t, externalPath); got != external {
		t.Fatalf("external rules changed:\n got %q\nwant %q", got, external)
	}
	wantWarnings := []string{"OpenCode baseline rules are externally satisfied by exact dots:rules in " + externalPath + "; Packy preserved the external instruction and omitted its own rules contribution"}
	if !slices.Equal(result.Warnings, wantWarnings) {
		t.Fatalf("warnings = %#v, want %#v", result.Warnings, wantWarnings)
	}

	repeatedPlan, err := PreviewWrite(configPath, promptPath)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := ApplyWrite(repeatedPlan)
	if err != nil {
		t.Fatal(err)
	}
	if readString(t, promptPath) != updatedPrompt || !slices.Equal(repeated.Warnings, wantWarnings) {
		t.Fatalf("repeated write changed result: warnings=%#v", repeated.Warnings)
	}

	if err := Remove(configPath, promptPath); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, externalPath); got != external {
		t.Fatalf("uninstall changed external rules:\n got %q\nwant %q", got, external)
	}
	instructions := stringSlice(t, readJSON(t, configPath)["instructions"])
	if !slices.Equal(instructions, []string{externalPath}) {
		t.Fatalf("instructions = %#v", instructions)
	}
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("Packy prompt still exists: %v", err)
	}
}

func TestWriteResolvesRelativeAndGlobInstructionReferencesWithoutAddingDuplicates(t *testing.T) {
	t.Run("relative Packy prompt", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "opencode.json")
		promptPath := filepath.Join(dir, "packy.md")
		config := `{"instructions":["packy.md"]}`
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := Write(configPath, promptPath); err != nil {
			t.Fatal(err)
		}
		if got := readString(t, configPath); got != config {
			t.Fatalf("relative reference was duplicated:\n got %q\nwant %q", got, config)
		}
		inspection, err := Inspect(configPath, promptPath)
		if err != nil || !inspection.HasPackyInstruction {
			t.Fatalf("inspection = %#v, err = %v", inspection, err)
		}
		if err := Remove(configPath, promptPath); err != nil {
			t.Fatal(err)
		}
		if got := readString(t, configPath); got != config {
			t.Fatalf("uninstall changed foreign relative reference:\n got %q\nwant %q", got, config)
		}
	})

	t.Run("glob covers Packy prompt and exact external rules", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "opencode.json")
		promptPath := filepath.Join(dir, "packy.md")
		externalPath := filepath.Join(dir, "dots.md")
		config := `{"instructions":["*.md"]}`
		external := exactDotsRulesFixture()
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(externalPath, []byte(external), 0o600); err != nil {
			t.Fatal(err)
		}

		result, err := Write(configPath, promptPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := readString(t, configPath); got != config {
			t.Fatalf("glob reference was duplicated:\n got %q\nwant %q", got, config)
		}
		if strings.Contains(readString(t, promptPath), "<!-- packy:rules -->") {
			t.Fatal("exact rules reached through glob did not suppress Packy rules")
		}
		if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], externalPath) {
			t.Fatalf("warnings = %#v", result.Warnings)
		}
		inspection, err := Inspect(configPath, promptPath)
		if err != nil || !inspection.HasPackyInstruction || !inspection.RulesExternallySatisfied {
			t.Fatalf("inspection = %#v, err = %v", inspection, err)
		}
		if err := Remove(configPath, promptPath); err != nil {
			t.Fatal(err)
		}
		if got := readString(t, configPath); got != config {
			t.Fatalf("uninstall changed foreign glob reference:\n got %q\nwant %q", got, config)
		}
		if got := readString(t, externalPath); got != external {
			t.Fatalf("uninstall changed external rules:\n got %q\nwant %q", got, external)
		}
	})

	t.Run("opaque reference is preserved", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "opencode.json")
		promptPath := filepath.Join(dir, "packy.md")
		opaque := "https://example.test/instructions.md"
		if err := os.WriteFile(configPath, []byte(`{"instructions":[`+quoted(opaque)+`]}`), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := Write(configPath, promptPath); err != nil {
			t.Fatal(err)
		}
		if got := stringSlice(t, readJSON(t, configPath)["instructions"]); !slices.Equal(got, []string{opaque, promptPath}) {
			t.Fatalf("instructions = %#v", got)
		}
		if err := Remove(configPath, promptPath); err != nil {
			t.Fatal(err)
		}
		if got := stringSlice(t, readJSON(t, configPath)["instructions"]); !slices.Equal(got, []string{opaque}) {
			t.Fatalf("instructions after uninstall = %#v", got)
		}
	})
}

func TestWritePreservesDifferingAndMalformedReferencedDotsRules(t *testing.T) {
	for name, tc := range map[string]struct {
		external string
		warning  string
	}{
		"different": {
			external: "<!-- dots:rules -->\n## Dots Agent Rules\n\nDifferent.\n<!-- /dots:rules -->",
			warning:  "OpenCode dots:rules in %s differs from the Packy baseline; Packy projected its baseline and preserved the external instruction; align the external provider contract before retrying",
		},
		"malformed": {
			external: "<!-- dots:rules -->\n## Dots Agent Rules\n\nUnclosed.",
			warning:  "OpenCode dots:rules markers in %s are malformed; Packy projected its baseline and preserved the external instruction; repair the external provider markers before retrying",
		},
		"unknown": {
			external: "<!-- other:rules -->\nSame-looking but foreign.\n<!-- /other:rules -->",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "opencode.json")
			promptPath := filepath.Join(dir, "packy.md")
			externalPath := filepath.Join(dir, "external.md")
			if err := os.WriteFile(externalPath, []byte(tc.external), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, []byte(`{"instructions":[`+quoted(externalPath)+`]}`), 0o600); err != nil {
				t.Fatal(err)
			}

			result, err := Write(configPath, promptPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := readString(t, externalPath); got != tc.external {
				t.Fatalf("external instruction changed:\n got %q\nwant %q", got, tc.external)
			}
			if count := strings.Count(readString(t, promptPath), "<!-- packy:rules -->"); count != 1 {
				t.Fatalf("Packy rules count = %d", count)
			}
			var wantWarnings []string
			if tc.warning != "" {
				wantWarnings = []string{fmt.Sprintf(tc.warning, externalPath)}
			}
			if !slices.Equal(result.Warnings, wantWarnings) {
				t.Fatalf("warnings = %#v, want %#v", result.Warnings, wantWarnings)
			}
		})
	}
}

func TestApplyWriteRejectsExternalRulesThatChangedAfterPreview(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	externalPath := filepath.Join(dir, "external.md")
	config := `{"instructions":[` + quoted(externalPath) + `]}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PreviewWrite(configPath, promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalPath, []byte(exactDotsRulesFixture()), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyWrite(plan); !errors.Is(err, ErrStaleWritePlan) {
		t.Fatalf("ApplyWrite error = %v, want %v", err, ErrStaleWritePlan)
	}
	if got := readString(t, configPath); got != config {
		t.Fatalf("stale apply changed config:\n got %q\nwant %q", got, config)
	}
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("stale apply wrote Packy prompt: %v", err)
	}
}

func TestWriteMergesOpenCodeConfigAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `{
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-sonnet-4-5",
  "mcp": {"jira": {"type": "remote", "enabled": true}},
  "provider": {"openai": {"npm": "@ai-sdk/openai"}},
  "plugin": ["gentle-ai"],
  "agent": {"gentle-ai": {"prompt": "keep"}},
  "profile": {"gentle-ai": {"agent": "gentle-ai"}},
  "instructions": ["CONTRIBUTING.md"]
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	result, err := Write(configPath, promptPath)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "gentle-ai") {
		t.Fatalf("warnings = %#v, want gentle-ai warning", result.Warnings)
	}
	first := readString(t, configPath)
	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("second Write failed: %v", err)
	}
	second := readString(t, configPath)
	if second != first {
		t.Fatalf("Write should be idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	config := readJSON(t, configPath)
	for _, key := range []string{"$schema", "model", "mcp", "provider", "plugin", "agent", "profile"} {
		if _, ok := config[key]; !ok {
			t.Fatalf("merged config lost %q: %#v", key, config)
		}
	}
	instructions := stringSlice(t, config["instructions"])
	if got := strings.Join(instructions, "\n"); got != strings.Join([]string{"CONTRIBUTING.md", promptPath}, "\n") {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWriteOnlyWarnsForKnownGentleAIOverlays(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `{
  "instructions": ["docs/gentle-ai-migration-notes.md"]
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	result, err := Write(configPath, promptPath)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for non-overlay gentle-ai text", result.Warnings)
	}
}

func TestWriteDoesNotWarnForPluginNamesThatOnlyContainGentleAI(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `{
  "plugin": ["my-gentle-ai-helper"]
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	result, err := Write(configPath, promptPath)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for plugin value that only contains gentle-ai", result.Warnings)
	}
}

func TestWriteMergesOpenCodeJSONCConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `{
  // OpenCode accepts JSONC global config.
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-sonnet-4-5",
  "mcp": {
    "jira": {
      "type": "remote",
      "url": "https://jira.example.com/mcp", // keep URL strings intact
      "enabled": true,
    },
  },
  "instructions": [
    "CONTRIBUTING.md",
  ],
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed for JSONC config: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"// OpenCode accepts JSONC global config.", "// keep URL strings intact", "\"enabled\": true,"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("JSONC merge did not preserve %q:\n%s", want, updated)
		}
	}
	config := decodeJSONC(t, updated)
	if config["model"] != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("model was not preserved: %#v", config)
	}
	mcp, ok := config["mcp"].(map[string]any)
	if !ok || mcp["jira"] == nil {
		t.Fatalf("mcp config was not preserved: %#v", config["mcp"])
	}
	instructions := stringSlice(t, config["instructions"])
	if got := strings.Join(instructions, "\n"); got != strings.Join([]string{"CONTRIBUTING.md", promptPath}, "\n") {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWritePreservesLeadingJSONCComments(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `// global OpenCode config
{
  "model": "anthropic/claude-sonnet-4-5"
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	if !strings.HasPrefix(updated, "// global OpenCode config") {
		t.Fatalf("leading comment was not preserved:\n%s", updated)
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWriteAndRemovePreserveInstructionComments(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `{
  "instructions": [
    // keep user rationale
    "CONTRIBUTING.md", // keep inline note
  ]
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"// keep user rationale", "// keep inline note", "\"CONTRIBUTING.md\",", promptPath} {
		if !strings.Contains(updated, want) {
			t.Fatalf("OpenCode merge did not preserve %q:\n%s", want, updated)
		}
	}
	if err := Remove(configPath, promptPath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	removed := readString(t, configPath)
	for _, want := range []string{"// keep user rationale", "// keep inline note", "\"CONTRIBUTING.md\","} {
		if !strings.Contains(removed, want) {
			t.Fatalf("OpenCode remove did not preserve %q:\n%s", want, removed)
		}
	}
	if strings.Contains(removed, promptPath) {
		t.Fatalf("Remove left Packy instruction reference:\n%s", removed)
	}
}

func TestWritePreservesTrailingPropertyCommentWhenAddingInstructions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `{
  "model": "anthropic/claude-sonnet-4-5" // keep model note
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"\"model\": \"anthropic/claude-sonnet-4-5\", // keep model note", promptPath} {
		if !strings.Contains(updated, want) {
			t.Fatalf("OpenCode merge did not preserve/comment-comma %q:\n%s", want, updated)
		}
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWritePreservesTrailingInstructionCommentWhenAppending(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := `{
  "instructions": [
    "CONTRIBUTING.md" // keep instruction note
  ]
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"\"CONTRIBUTING.md\", // keep instruction note", promptPath} {
		if !strings.Contains(updated, want) {
			t.Fatalf("OpenCode merge did not preserve/comment-comma %q:\n%s", want, updated)
		}
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if got := strings.Join(instructions, "\n"); got != strings.Join([]string{"CONTRIBUTING.md", promptPath}, "\n") {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWriteInsertsIntoCommentOnlyJSONCConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	existing := "{\n  // keep this comment\n}\n"
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	if !strings.Contains(updated, "// keep this comment") {
		t.Fatalf("comment was not preserved:\n%s", updated)
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestRemoveDeletesOnlyPackyOpenCodeEntries(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "packy.md")
	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	config := readJSON(t, configPath)
	config["instructions"] = []any{"CONTRIBUTING.md", promptPath, "docs/rules.md", promptPath}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("encode test config: %v", err)
	}
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	if err := Remove(configPath, promptPath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("prompt still exists or stat failed: %v", err)
	}
	instructions := stringSlice(t, readJSON(t, configPath)["instructions"])
	if got := strings.Join(instructions, "\n"); got != "CONTRIBUTING.md\ndocs/rules.md" {
		t.Fatalf("instructions after remove = %#v", instructions)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	return decodeJSONC(t, readString(t, path))
}

func decodeJSONC(t *testing.T, content string) map[string]any {
	t.Helper()
	data, err := jsoncToJSON(content)
	if err != nil {
		t.Fatalf("convert JSONC: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode JSONC: %v", err)
	}
	return out
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want []any", value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("item = %#v, want string", item)
		}
		out = append(out, s)
	}
	return out
}
