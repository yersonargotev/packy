package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue463PublishedProjectSchemasCoverLiveContractsAndCanonicalNegatives(t *testing.T) {
	opts, home, repoRoot := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	preview, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("project preview: %v\n%s", err, preview)
	}
	applyEvents, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--json")
	if err != nil {
		t.Fatalf("project apply: %v\n%s", err, applyEvents)
	}
	apply := lastJSONDocument(t, applyEvents)
	status, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("project status: %v\n%s", err, status)
	}
	const secretArgument = "TOKEN=issue-463-secret"
	failureOutput, failureErr := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--alias", secretArgument, "--json")
	if failureErr == nil {
		t.Fatalf("invalid project intent unexpectedly succeeded: %s", failureOutput)
	}
	if strings.Contains(failureOutput, secretArgument) || strings.Contains(failureErr.Error(), secretArgument) {
		t.Fatalf("project failure disclosed a raw argument: %s / %v", failureOutput, failureErr)
	}
	failure := lastJSONDocument(t, failureOutput)
	projectRoot, err := capabilitypack.DiscoverProjectRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectRecoveryJournal(t, filepath.Join(home, ".packy"), projectRoot)
	recoveryEvents, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--json")
	if err != nil {
		t.Fatalf("project recovery: %v\n%s", err, recoveryEvents)
	}
	recovery := firstJSONDocument(t, recoveryEvents)
	seedProjectRecoveryJournal(t, filepath.Join(home, ".packy"), projectRoot)
	updateRecovery, updateErr := executeCommand(t, NewRootCommand(opts), "pack", "update", "PACK-TOKEN-issue-463", "--project", "--version", "VERSION-TOKEN-issue-463", "--json")
	if updateErr == nil {
		t.Fatalf("unknown project update unexpectedly succeeded: %s", updateRecovery)
	}
	firstUpdateEvent := string(firstJSONDocument(t, updateRecovery))
	if strings.Contains(firstUpdateEvent, "TOKEN-issue-463") || !strings.Contains(firstUpdateEvent, `"next_command":"packy pack update \u003cpack\u003e --project --version \u003cversion\u003e"`) {
		t.Fatalf("project recovery command is not safely templated: %s", firstUpdateEvent)
	}

	documents := map[string][]byte{
		"project-manifest.schema.json": mustReadProjectContract(t, filepath.Join(project, "packy.json")),
		"project-lock.schema.json":     mustReadProjectContract(t, filepath.Join(project, "packy.lock.json")),
		"project-status.schema.json":   []byte(status),
		"project-preview.schema.json":  []byte(preview),
		"project-apply.schema.json":    apply,
		"project-failure.schema.json":  failure,
		"project-recovery.schema.json": recovery,
	}
	for schemaName, document := range documents {
		if err := validateProjectContractSchema(repoRoot, schemaName, document); err != nil {
			t.Fatalf("%s live contract: %v\n%s", schemaName, err, document)
		}
		for _, forbidden := range []string{project, home, "TOKEN=", "SECRET="} {
			if forbidden != "" && strings.Contains(string(document), forbidden) {
				t.Fatalf("%s discloses %q: %s", schemaName, forbidden, document)
			}
		}
	}

	negativeRoot := filepath.Join(repoRoot, "internal", "cli", "testdata", "project-contract", "v1.0.0", "negative")
	entries, err := os.ReadDir(negativeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(documents) {
		t.Fatalf("canonical project negative fixtures = %d, want %d", len(entries), len(documents))
	}
	for _, entry := range entries {
		fixture := mustReadProjectContract(t, filepath.Join(negativeRoot, entry.Name()))
		schemaName := strings.TrimSuffix(entry.Name(), ".invalid.json") + ".schema.json"
		if err := validateProjectContractSchema(repoRoot, schemaName, fixture); err == nil {
			t.Fatalf("%s accepted canonical negative fixture %s", schemaName, entry.Name())
		}
	}
}

func seedProjectRecoveryJournal(t *testing.T, packyHome, projectRoot string) {
	t.Helper()
	type recoveryJournal struct {
		SchemaVersion int                               `json:"schema_version"`
		Observation   string                            `json:"observation"`
		ProjectRoot   string                            `json:"project_root"`
		Reverse       []capabilitypack.ProjectionAction `json:"reverse"`
		Seal          string                            `json:"seal"`
	}
	journal := recoveryJournal{SchemaVersion: 1, Observation: "issue-463-recovery", ProjectRoot: projectRoot, Reverse: []capabilitypack.ProjectionAction{}}
	unsigned, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	seal := sha256.Sum256(unsigned)
	journal.Seal = hex.EncodeToString(seal[:])
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	projectDigest := sha256.Sum256([]byte(filepath.Clean(projectRoot)))
	journalPath := filepath.Join(packyHome, "projects", hex.EncodeToString(projectDigest[:]), "install-journal.json")
	if err := os.MkdirAll(filepath.Dir(journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestIssue463ProjectHelpExplainsTheTwoPhaseLifecycle(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	for _, args := range [][]string{{"pack", "--help"}, {"pack", "install", "--help"}, {"pack", "activate", "--help"}, {"pack", "status", "--help"}, {"pack", "uninstall", "--help"}} {
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		for _, want := range []string{"project installation", "personal", "--project"} {
			if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
				t.Fatalf("%v help does not distinguish the two phases; missing %q:\n%s", args, want, out)
			}
		}
	}
}

func TestIssue463ProjectArgumentFailuresEmitTerminalJSON(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	tests := []struct {
		operation string
		args      []string
	}{
		{"install", []string{"pack", "install", "one", "two", "--json"}},
		{"uninstall", []string{"pack", "uninstall", "--json"}},
		{"activate", []string{"pack", "activate", "--project", "--json"}},
		{"deactivate", []string{"pack", "deactivate", "--project", "--json"}},
		{"update", []string{"pack", "update", "--project", "--json"}},
		{"status", []string{"pack", "status", "one", "two", "--project", "--json"}},
	}
	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			output, err := executeCommand(t, NewRootCommand(opts), tt.args...)
			if err == nil {
				t.Fatalf("argument failure unexpectedly succeeded: %s", output)
			}
			var failure capabilitypack.JSONProjectFailure
			if err := json.Unmarshal(lastJSONDocument(t, output), &failure); err != nil {
				t.Fatal(err)
			}
			if failure.Report != "project-lifecycle-failure" || failure.Operation != tt.operation || failure.Stage != "command" {
				t.Fatalf("terminal failure = %+v", failure)
			}
		})
	}
}

func lastJSONDocument(t *testing.T, stream string) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stream), "\n")
	if len(lines) == 0 || !json.Valid([]byte(lines[len(lines)-1])) {
		t.Fatalf("missing final JSON document:\n%s", stream)
	}
	return []byte(lines[len(lines)-1])
}

func firstJSONDocument(t *testing.T, stream string) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stream), "\n")
	if len(lines) == 0 || !json.Valid([]byte(lines[0])) {
		t.Fatalf("missing first JSON document:\n%s", stream)
	}
	return []byte(lines[0])
}

func mustReadProjectContract(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func validateProjectContractSchema(repoRoot, schemaName string, instance []byte) error {
	const suite = "v1.0.0"
	compiler := jsonschema.NewCompiler()
	schemaRoot := filepath.Join(repoRoot, "schemas", "project", suite)
	entries, err := os.ReadDir(schemaRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		schemaBytes, err := os.ReadFile(filepath.Join(schemaRoot, entry.Name()))
		if err != nil {
			return err
		}
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
		if err != nil {
			return err
		}
		if err := compiler.AddResource("https://yersonargotev.github.io/packy/schemas/project/"+suite+"/"+entry.Name(), document); err != nil {
			return err
		}
	}
	schema, err := compiler.Compile("https://yersonargotev.github.io/packy/schemas/project/" + suite + "/" + schemaName)
	if err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(instance))
	if err != nil {
		return err
	}
	return schema.Validate(value)
}
