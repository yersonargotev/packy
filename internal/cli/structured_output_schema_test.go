package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yersonargotev/packy/internal/setuphealth"
)

var structuredOutputFixtures = map[string]string{
	"classic-lifecycle-preview.json": "classic-lifecycle.schema.json",
	"classic-lifecycle-result.json":  "classic-lifecycle.schema.json",
	"doctor.json":                    "doctor.schema.json",
	"pack-show.json":                 "pack-show.schema.json",
	"pack-lifecycle-preview.json":    "pack-lifecycle.schema.json",
	"pack-status.json":               "pack-status.schema.json",
}

var structuredOutputV3Fixtures = map[string]string{
	"pack-lifecycle-preview.json": "pack-lifecycle.schema.json",
	"pack-status.json":            "pack-status.schema.json",
}

var structuredOutputV4Fixtures = map[string]string{
	"pack-show.json":              "pack-show.schema.json",
	"pack-lifecycle-preview.json": "pack-lifecycle.schema.json",
	"pack-status.json":            "pack-status.schema.json",
}

var structuredOutputV5Fixtures = map[string]string{
	"pack-lifecycle-preview.json": "pack-lifecycle.schema.json",
	"pack-status.json":            "pack-status.schema.json",
}

var structuredOutputV6Fixtures = map[string]string{
	"pack-lifecycle-preview.json": "pack-lifecycle.schema.json",
	"pack-status.json":            "pack-status.schema.json",
}

var structuredOutputV7Fixtures = map[string]string{
	"pack-lifecycle-preview.json": "pack-lifecycle.schema.json",
	"pack-status.json":            "pack-status.schema.json",
}

var structuredOutputV8Fixtures = map[string]string{
	"pack-lifecycle-apply.json":   "pack-lifecycle.schema.json",
	"pack-lifecycle-failure.json": "pack-lifecycle.schema.json",
	"pack-lifecycle-preview.json": "pack-lifecycle.schema.json",
}

func TestStructuredOutputSchemasValidateFixturesAndProducers(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixtureRoot := filepath.Join("testdata", "structured-output", "v2")
	for fixtureName, schemaName := range structuredOutputFixtures {
		fixture, err := os.ReadFile(filepath.Join(fixtureRoot, fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("fixture %s: %v", fixtureName, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("fixture %s canonical order: %v", fixtureName, err)
		}
		for _, forbidden := range []string{"TOKEN=", "SECRET=", "/Users/", "foreign-document", "mixed-store"} {
			if strings.Contains(string(fixture), forbidden) {
				t.Fatalf("fixture %s leaks %q", fixtureName, forbidden)
			}
		}
	}
	for fixtureName, schemaName := range structuredOutputV3Fixtures {
		fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v3", fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("v3 fixture %s: %v", fixtureName, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("v3 fixture %s canonical order: %v", fixtureName, err)
		}
	}
	for fixtureName, schemaName := range structuredOutputV4Fixtures {
		fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v4", fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("v4 fixture %s: %v", fixtureName, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("v4 fixture %s canonical order: %v", fixtureName, err)
		}
	}
	for fixtureName, schemaName := range structuredOutputV5Fixtures {
		fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v5", fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("v5 fixture %s: %v", fixtureName, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("v5 fixture %s canonical order: %v", fixtureName, err)
		}
	}
	for fixtureName, schemaName := range structuredOutputV6Fixtures {
		fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v6", fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("v6 fixture %s: %v", fixtureName, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("v6 fixture %s canonical order: %v", fixtureName, err)
		}
	}
	for fixtureName, schemaName := range structuredOutputV7Fixtures {
		fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v7", fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("v7 fixture %s: %v", fixtureName, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("v7 fixture %s canonical order: %v", fixtureName, err)
		}
	}
	for fixtureName, schemaName := range structuredOutputV8Fixtures {
		fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v8", fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("v8 fixture %s: %v", fixtureName, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("v8 fixture %s canonical order: %v", fixtureName, err)
		}
	}

	opts, _, _ := sandboxOptions(t)
	classic, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "classic-lifecycle.schema.json", classic)

	opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
		return setuphealth.Report{SchemaVersion: 2, Kind: "doctor", Checks: []setuphealth.Check{{Name: "claude-readiness", Severity: setuphealth.Warn, Detail: "runtime usability is unknown; start Claude Code explicitly"}}, Summary: setuphealth.Summary{Status: "warnings", Warnings: 1}}, nil
	}
	doctor, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "doctor.schema.json", doctor)

	packReadOpts := Options{Env: MapEnv{"HOME": t.TempDir(), "XDG_CONFIG_HOME": filepath.Join(t.TempDir(), "xdg"), "PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(root, "bundle", "skills")}}
	show, err := executeCommand(t, NewRootCommand(packReadOpts), "pack", "show", "engram", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-show.schema.json", show)

	status, err := executeCommand(t, NewRootCommand(packReadOpts), "pack", "status", "ma"+"tty", "--surface", "claude", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-status.schema.json", status)

	packOpts, _, _ := packActivationOptions(t, &fakeTerminal{})
	preview, err := executeCommand(t, NewRootCommand(packOpts), "pack", "activate", "ma"+"tty", "--surface", "claude", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("pack preview: %v\n%s", err, preview)
	}
	assertStructuredOutput(t, root, "pack-lifecycle.schema.json", preview)
}

func TestStructuredOutputV2SchemasRejectWrongVersionAndUnknownFields(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	for _, document := range []string{
		`{"schema_version":1,"report":"doctor","checks":[],"summary":{"status":"healthy","passes":0,"warnings":0,"failures":0}}`,
		`{"schema_version":2,"report":"doctor","checks":[],"summary":{"status":"healthy","passes":0,"warnings":0,"failures":0},"unknown":true}`,
	} {
		if err := validateStructuredOutput(t, root, "doctor.schema.json", []byte(document)); err == nil {
			t.Fatalf("invalid document passed: %s", document)
		}
	}
}

func TestStructuredOutputV2SchemasRejectMismatchedReadinessState(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v2", "pack-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	entry := document["entries"].([]any)[0].(map[string]any)
	readiness := entry["readiness"].(map[string]any)
	for name, invalid := range map[string]map[string]any{
		"unknown boolean": {"state": "unknown", "value": true},
		"known null":      {"state": "known", "value": nil},
	} {
		t.Run(name, func(t *testing.T) {
			original := readiness["configured"]
			readiness["configured"] = invalid
			defer func() { readiness["configured"] = original }()
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateStructuredOutput(t, root, "pack-status.schema.json", encoded); err == nil {
				t.Fatalf("mismatched readiness passed: %s", encoded)
			}
		})
	}
}

func TestRuntimeModeV2SchemaAcceptsSanitizedFactsAndRejectsUnknownProbeData(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	valid := []byte(`{
		"resource_id":"vercel-deploy","mode_id":"preview","role":"primary","state":"unavailable",
		"requirements":[{"kind":"tool","id":"vercel","version":">=53.0.0"}],
		"authorities":[{"kind":"network","scope":"vercel_account"}],
		"effects":[{"kind":"preview_deployment","scope":"deployment_payload"}],
		"fallback":{"kind":"mode","mode":"local"},"fallback_state":"unverified",
		"on_unavailable":"fail_before_effects",
		"evidence":{
			"requirements":[{"kind":"tool","id":"vercel","state":"unavailable","reason":"not_found","observed_at":"2026-07-25T12:00:00Z","observer_revision":"codex-v1","redacted_identity":"vercel"}],
			"authorities":[{"kind":"network","scope":"vercel_account","state":"unverified","reason":"observer_error","observed_at":"2026-07-25T12:00:00Z","observer_revision":"codex-v1"}]
		},
		"affected":["requirement:tool:vercel"]
	}`)
	if err := validateStructuredOutput(t, root, "runtime-mode.schema.json", valid); err != nil {
		t.Fatalf("valid runtime mode: %v", err)
	}
	var invalid map[string]any
	if err := json.Unmarshal(valid, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid["probe_output"] = "TOKEN=secret"
	encoded, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStructuredOutput(t, root, "runtime-mode.schema.json", encoded); err == nil {
		t.Fatal("runtime mode schema accepted secret-bearing probe output")
	}
}

func TestPackOperatorSchemasRejectCanonicalNegativeTwins(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	fixtureRoot := filepath.Join("testdata", "structured-output", "v2")
	load := func(t *testing.T, name string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(fixtureRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		return document
	}
	reject := func(t *testing.T, schema string, document map[string]any) {
		t.Helper()
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schema, encoded); err == nil {
			t.Fatalf("negative twin passed %s: %s", schema, encoded)
		}
	}

	t.Run("unknown fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		document["unknown"] = true
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("duplicate fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		surfaces := document["surfaces"].([]any)
		document["surfaces"] = append(surfaces, surfaces[0])
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("missing fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		delete(document, "source_identity")
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("contradictory fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		intent := document["surface_contracts"].([]any)[0].(map[string]any)["intent"].(map[string]any)
		intent["state"] = "known"
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("unredacted ambient target", func(t *testing.T) {
		document := load(t, "pack-status.json")
		detail := document["entries"].([]any)[0].(map[string]any)["projection_details"].([]any)[0].(map[string]any)
		detail["target"] = "/Users/operator/.claude/skills/example"
		reject(t, "pack-status.schema.json", document)
	})
	for _, test := range []struct {
		name    string
		fixture string
		edit    func(map[string]any)
	}{
		{"nondeterministic top-level order", "pack-show.json", func(document map[string]any) {
			values := document["surfaces"].([]any)
			values[0], values[1] = values[1], values[0]
		}},
		{"nondeterministic contract order", "pack-show.json", func(document map[string]any) {
			contract := document["surface_contracts"].([]any)[0].(map[string]any)["contract"].(map[string]any)
			values := contract["bindings"].([]any)
			values[0], values[1] = values[1], values[0]
		}},
		{"nondeterministic status order", "pack-status.json", func(document map[string]any) {
			entry := document["entries"].([]any)[0].(map[string]any)
			values := entry["optional_authorities"].([]any)
			values[0], values[1] = values[1], values[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := load(t, test.fixture)
			test.edit(document)
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateCanonicalOperatorOrder(encoded); err == nil {
				t.Fatalf("out-of-order canonical facts passed: %s", encoded)
			}
		})
	}
}

func validateCanonicalOperatorOrder(instance []byte) error {
	var document map[string]any
	if err := json.Unmarshal(instance, &document); err != nil {
		return err
	}
	requireOrdered := func(name string, values []any, key func(any) string) error {
		for i := 1; i < len(values); i++ {
			if key(values[i-1]) > key(values[i]) {
				return fmt.Errorf("%s is not canonically ordered", name)
			}
		}
		return nil
	}
	stringKey := func(value any) string { text, _ := value.(string); return text }
	requireStrings := func(name string, values []any) error {
		return requireOrdered(name, values, stringKey)
	}
	objectKey := func(fields ...string) func(any) string {
		return func(value any) string {
			object := value.(map[string]any)
			parts := make([]string, len(fields))
			for i, field := range fields {
				parts[i], _ = object[field].(string)
			}
			return strings.Join(parts, "\x00")
		}
	}
	nestedObjectKey := func(parent string, fields ...string) func(any) string {
		return func(value any) string {
			object := value.(map[string]any)[parent].(map[string]any)
			parts := make([]string, len(fields))
			for i, field := range fields {
				parts[i], _ = object[field].(string)
			}
			return strings.Join(parts, "\x00")
		}
	}
	validateAliases := func(name string, values []any) error {
		return requireOrdered(name, values, objectKey("kind", "id", "name"))
	}
	validateContract := func(name string, contract map[string]any) error {
		checks := []struct {
			suffix string
			values []any
			key    func(any) string
		}{
			{"dependency_closure", contract["dependency_closure"].([]any), stringKey},
			{"bindings", contract["bindings"].([]any), objectKey("kind", "id", "projection", "name")},
			{"exclusions", contract["exclusions"].([]any), objectKey("id", "code")},
			{"optional_modes", contract["optional_modes"].([]any), objectKey("id")},
			{"prompt_authorities", contract["prompt_authorities"].([]any), stringKey},
			{"aliases", contract["aliases"].([]any), objectKey("kind", "id", "name")},
		}
		for _, check := range checks {
			if err := requireOrdered(name+"."+check.suffix, check.values, check.key); err != nil {
				return err
			}
		}
		for _, value := range contract["exclusions"].([]any) {
			exclusion := value.(map[string]any)
			if err := requireStrings(name+".exclusions.source_paths", exclusion["source_paths"].([]any)); err != nil {
				return err
			}
		}
		for _, value := range contract["optional_modes"].([]any) {
			mode := value.(map[string]any)
			if err := requireStrings(name+".optional_modes.authorities", mode["authorities"].([]any)); err != nil {
				return err
			}
		}
		return nil
	}
	validateRuntimeModes := func(name string, modes []any) error {
		if err := requireOrdered(name, modes, objectKey("resource_id", "mode_id")); err != nil {
			return err
		}
		for _, value := range modes {
			mode := value.(map[string]any)
			for field, key := range map[string]func(any) string{
				"requirements": objectKey("kind", "id"),
				"authorities":  objectKey("kind", "scope"),
				"effects":      objectKey("kind", "scope"),
			} {
				if err := requireOrdered(name+"."+field, mode[field].([]any), key); err != nil {
					return err
				}
			}
			evidence := mode["evidence"].(map[string]any)
			if err := requireOrdered(name+".evidence.requirements", evidence["requirements"].([]any), objectKey("kind", "id")); err != nil {
				return err
			}
			if err := requireOrdered(name+".evidence.authorities", evidence["authorities"].([]any), objectKey("kind", "scope")); err != nil {
				return err
			}
			if err := requireStrings(name+".affected", mode["affected"].([]any)); err != nil {
				return err
			}
		}
		return nil
	}
	switch document["report"] {
	case "pack-show":
		for _, field := range []string{"historical_versions", "surfaces", "provides", "conflicts"} {
			if err := requireStrings(field, document[field].([]any)); err != nil {
				return err
			}
		}
		requires := document["requires"].(map[string]any)
		for _, field := range []string{"capabilities", "tools"} {
			if err := requireStrings("requires."+field, requires[field].([]any)); err != nil {
				return err
			}
		}
		routes := document["update_routes"].([]any)
		if err := requireOrdered("update_routes", routes, objectKey("from_version", "to_version")); err != nil {
			return err
		}
		for _, value := range routes {
			route := value.(map[string]any)
			if err := requireStrings("update_routes.existing_surfaces", route["existing_surfaces"].([]any)); err != nil {
				return err
			}
		}
		contracts := document["surface_contracts"].([]any)
		if err := requireOrdered("surface_contracts", contracts, objectKey("surface")); err != nil {
			return err
		}
		for _, value := range contracts {
			surface := value.(map[string]any)
			if err := validateContract("surface_contracts.contract", surface["contract"].(map[string]any)); err != nil {
				return err
			}
			intent := surface["intent"].(map[string]any)
			if err := validateAliases("surface_contracts.intent.aliases", intent["aliases"].([]any)); err != nil {
				return err
			}
		}
	case "pack-status", "pack-status-overview":
		entries := document["entries"].([]any)
		if err := requireOrdered("entries", entries, objectKey("pack", "surface")); err != nil {
			return err
		}
		for _, value := range entries {
			entry := value.(map[string]any)
			if err := validateContract("entries.contract", entry["contract"].(map[string]any)); err != nil {
				return err
			}
			if modes, ok := entry["runtime_modes"].([]any); ok {
				if err := validateRuntimeModes("entries.runtime_modes", modes); err != nil {
					return err
				}
			}
			projections := entry["projection_details"].([]any)
			if err := requireOrdered("projection_details", projections, objectKey("id")); err != nil {
				return err
			}
			for _, projectionValue := range projections {
				projection := projectionValue.(map[string]any)
				if err := requireStrings("projection_details.contributors", projection["contributors"].([]any)); err != nil {
					return err
				}
			}
			if err := requireOrdered("optional_authorities", entry["optional_authorities"].([]any), objectKey("mode_id", "authority")); err != nil {
				return err
			}
			if resources, ok := entry["resources"].([]any); ok {
				if err := requireOrdered("resources", resources, nestedObjectKey("resource", "kind", "id")); err != nil {
					return err
				}
				for _, resourceValue := range resources {
					resource := resourceValue.(map[string]any)
					if err := requireStrings("resources.blockers", resource["blockers"].([]any)); err != nil {
						return err
					}
				}
			}
			for _, field := range []string{"blockers", "evidence", "pending_human_actions"} {
				if err := requireStrings(field, entry[field].([]any)); err != nil {
					return err
				}
			}
		}
	case "pack-lifecycle-preview":
		if err := validateContract("contract", document["contract"].(map[string]any)); err != nil {
			return err
		}
		if origins, ok := document["sensitive_effects"].([]any); ok {
			originKey := func(value any) string {
				origin := value.(map[string]any)
				root := origin["root"].(map[string]any)
				resource := origin["resource"].(map[string]any)
				parts := []string{origin["pack"].(string), root["kind"].(string), root["id"].(string), resource["kind"].(string), resource["id"].(string)}
				for _, member := range origin["dependency_chain"].([]any) {
					identity := member.(map[string]any)
					parts = append(parts, identity["kind"].(string), identity["id"].(string))
				}
				return strings.Join(parts, "\x00")
			}
			if err := requireOrdered("sensitive_effects", origins, originKey); err != nil {
				return err
			}
			for _, value := range origins {
				origin := value.(map[string]any)
				if err := requireStrings("sensitive_effects.prompt_authorities", origin["prompt_authorities"].([]any)); err != nil {
					return err
				}
				if err := requireOrdered("sensitive_effects.runtime_authorities", origin["runtime_authorities"].([]any), objectKey("mode_id", "kind", "scope")); err != nil {
					return err
				}
				if err := requireOrdered("sensitive_effects.runtime_effects", origin["runtime_effects"].([]any), objectKey("mode_id", "kind", "scope")); err != nil {
					return err
				}
			}
		}
		if modes, ok := document["runtime_modes"].([]any); ok {
			return validateRuntimeModes("runtime_modes", modes)
		}
	}
	return nil
}

func assertStructuredOutput(t *testing.T, root, schemaName, document string) {
	t.Helper()
	if err := validateStructuredOutput(t, root, schemaName, []byte(document)); err != nil {
		t.Fatalf("%s producer: %v\n%s", schemaName, err, document)
	}
}

func validateStructuredOutput(t *testing.T, root, schemaName string, instance []byte) error {
	t.Helper()
	var versioned struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(instance, &versioned); err != nil {
		return err
	}
	if versioned.SchemaVersion <= 0 {
		if schemaName == "runtime-mode.schema.json" || schemaName == "lifecycle-contract.schema.json" {
			versioned.SchemaVersion = 2
		} else {
			return fmt.Errorf("structured output schema_version is required")
		}
	}
	compiler := jsonschema.NewCompiler()
	schemaVersion := fmt.Sprintf("v%d", versioned.SchemaVersion)
	schemaRoot := filepath.Join(root, "schemas", "cli", schemaVersion)
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
			return fmt.Errorf("parse schema %s: %w", entry.Name(), err)
		}
		if err := compiler.AddResource("https://yersonargotev.github.io/packy/schemas/cli/"+schemaVersion+"/"+entry.Name(), document); err != nil {
			return err
		}
	}
	schema, err := compiler.Compile("https://yersonargotev.github.io/packy/schemas/cli/" + schemaVersion + "/" + schemaName)
	if err != nil {
		t.Fatalf("compile schema %s: %v", schemaName, err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(instance))
	if err != nil {
		return err
	}
	if encoded, err := json.Marshal(value); err != nil || !json.Valid(encoded) {
		return fmt.Errorf("invalid decoded JSON: %w", err)
	}
	return schema.Validate(value)
}
