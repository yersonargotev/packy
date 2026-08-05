package cli

import (
	"bytes"
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
	failureOutput, failureErr := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--alias", "invalid", "--json")
	if failureErr == nil {
		t.Fatalf("invalid project intent unexpectedly succeeded: %s", failureOutput)
	}
	failure := lastJSONDocument(t, failureOutput)
	recoveryBytes, err := json.Marshal(capabilitypack.JSONProjectRecovery{
		SchemaVersion: capabilitypack.ProjectLifecycleJSONSchemaVersion,
		Report:        "project-recovery", Operation: "install", Status: "recovered",
		NextCommand: "packy pack install",
	})
	if err != nil {
		t.Fatal(err)
	}

	documents := map[string][]byte{
		"project-manifest.schema.json": mustReadProjectContract(t, filepath.Join(project, "packy.json")),
		"project-lock.schema.json":     mustReadProjectContract(t, filepath.Join(project, "packy.lock.json")),
		"project-status.schema.json":   []byte(status),
		"project-preview.schema.json":  []byte(preview),
		"project-apply.schema.json":    apply,
		"project-failure.schema.json":  failure,
		"project-recovery.schema.json": recoveryBytes,
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

func lastJSONDocument(t *testing.T, stream string) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stream), "\n")
	if len(lines) == 0 || !json.Valid([]byte(lines[len(lines)-1])) {
		t.Fatalf("missing final JSON document:\n%s", stream)
	}
	return []byte(lines[len(lines)-1])
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
