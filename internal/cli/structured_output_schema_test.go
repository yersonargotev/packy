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
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/setuphealth"
)

var structuredOutputFixtures = []struct {
	version, fixture, schema string
}{
	{"v1", "pack-audit.json", "pack-audit.schema.json"},
	{"v1", "pack-list.json", "pack-list.schema.json"},
	{"v3", "doctor.json", "doctor.schema.json"},
	{"v5", "pack-show.json", "pack-show.schema.json"},
	{"v11", "pack-status.json", "pack-status.schema.json"},
	{"v11", "pack-lifecycle-apply.json", "pack-lifecycle.schema.json"},
	{"v11", "pack-lifecycle-failure.json", "pack-lifecycle.schema.json"},
	{"v11", "pack-lifecycle-preview.json", "pack-lifecycle.schema.json"},
}

func TestStructuredOutputSchemasValidateFixturesAndProducers(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range structuredOutputFixtures {
		fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", current.version, current.fixture))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, current.schema, fixture); err != nil {
			t.Fatalf("fixture %s/%s: %v", current.version, current.fixture, err)
		}
		if err := validateCanonicalOperatorOrder(fixture); err != nil {
			t.Fatalf("fixture %s/%s canonical order: %v", current.version, current.fixture, err)
		}
		for _, forbidden := range []string{"TOKEN=", "SECRET=", "/Users/", "foreign-document", "mixed-store"} {
			if strings.Contains(string(fixture), forbidden) {
				t.Fatalf("fixture %s/%s leaks %q", current.version, current.fixture, forbidden)
			}
		}
	}

	opts, _, _ := sandboxOptions(t)
	opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
		return setuphealth.Report{SchemaVersion: 3, Kind: "doctor", Checks: []setuphealth.Check{{Name: "claude-readiness", Severity: setuphealth.Info, Detail: "runtime usability cannot be observed"}}, Summary: setuphealth.Summary{Status: "healthy", Infos: 1}}, nil
	}
	doctor, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "doctor.schema.json", doctor)

	auditOutput, err := executeCommand(t, NewRootCommand(opts), "audit", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-audit.schema.json", auditOutput)

	pack := testsupport.PortableAllSurfaces("schema-portable")
	manifest := pack.Manifest()
	packReadFixture := newSyntheticCLIFixture(t, &fakeTerminal{}, pack)
	packReadOpts := packReadFixture.options
	list, err := executeCommand(t, NewRootCommand(packReadOpts), "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-list.schema.json", list)
	if err := validateCanonicalOperatorOrder([]byte(list)); err != nil {
		t.Fatalf("pack-list producer canonical order: %v", err)
	}
	show, err := executeCommand(t, NewRootCommand(packReadOpts), "show", manifest.ID, "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-show.schema.json", show)
	if err := validateCanonicalOperatorOrder([]byte(show)); err != nil {
		t.Fatalf("pack-show producer canonical order: %v", err)
	}

	status, err := executeCommand(t, NewRootCommand(packReadOpts), "status", manifest.ID, "--surface", "claude", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-status.schema.json", status)

	preview, err := executeCommand(t, NewRootCommand(packReadOpts), "activate", manifest.ID, "--surface", "claude", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("pack preview: %v\n%s", err, preview)
	}
	assertStructuredOutput(t, root, "pack-lifecycle.schema.json", preview)

	project := t.TempDir()
	writeTestGitWorktree(t, project)
	projectFixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	projectOpts := projectFixture.options
	projectOpts.Getwd = func() (string, error) { return project, nil }
	resource := syntheticResource(t, pack, "instruction", "guidance")
	resourceID := resource.Kind + ":" + resource.ID
	projectPreview, err := executeCommand(t, NewRootCommand(projectOpts), "install", manifest.ID, "--surface", "codex", "--resource", resourceID, "--dry-run", "--json")
	if err != nil {
		t.Fatalf("project preview: %v\n%s", err, projectPreview)
	}
	assertProjectStructuredOutput(t, root, "project-preview.schema.json", projectPreview)
	applied, err := executeCommand(t, NewRootCommand(projectOpts), "install", manifest.ID, "--surface", "codex", "--resource", resourceID, "--json")
	if err != nil {
		t.Fatalf("project install: %v\n%s", err, applied)
	}
	projectDocuments := strings.Split(strings.TrimSpace(applied), "\n")
	if len(projectDocuments) != 2 {
		t.Fatalf("project install JSON documents = %d\n%s", len(projectDocuments), applied)
	}
	assertProjectStructuredOutput(t, root, "project-preview.schema.json", projectDocuments[0])
	assertProjectStructuredOutput(t, root, "project-apply.schema.json", projectDocuments[1])
	lockData, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lockDocument map[string]any
	if err := json.Unmarshal(lockData, &lockDocument); err != nil {
		t.Fatal(err)
	}
	projections := lockDocument["receipts"].([]any)[0].(map[string]any)["projections"].([]any)
	delete(projections[0].(map[string]any), "file_mode")
	missingMode, err := json.Marshal(lockDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateProjectStructuredOutput(t, root, "project-lock.schema.json", string(missingMode)); err == nil {
		t.Fatal("project lock schema accepted a projection without file_mode")
	}
	projectStatus, err := executeCommand(t, NewRootCommand(projectOpts), "status", manifest.ID, "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("project status: %v\n%s", err, projectStatus)
	}
	assertProjectStructuredOutput(t, root, "project-status.schema.json", projectStatus)
	projectVerification, err := executeCommand(t, NewRootCommand(projectOpts), "verify", "--json")
	if err != nil {
		t.Fatalf("project verification: %v\n%s", err, projectVerification)
	}
	assertProjectStructuredOutput(t, root, "project-verification.schema.json", projectVerification)
	emptyProject := t.TempDir()
	writeTestGitWorktree(t, emptyProject)
	projectOpts.Getwd = func() (string, error) { return emptyProject, nil }
	failedVerification, err := executeCommand(t, NewRootCommand(projectOpts), "verify", "--json")
	if err == nil {
		t.Fatal("absent project verification unexpectedly passed")
	}
	assertProjectStructuredOutput(t, root, "project-verification.schema.json", failedVerification)
}

func TestStructuredOutputV3DoctorSchemaRejectsWrongVersionAndUnknownFields(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	for _, document := range []string{
		`{"schema_version":2,"report":"doctor","checks":[],"summary":{"status":"healthy","passes":0,"infos":0,"warnings":0,"failures":0}}`,
		`{"schema_version":3,"report":"doctor","checks":[],"summary":{"status":"healthy","passes":0,"infos":0,"warnings":0,"failures":0},"unknown":true}`,
	} {
		if err := validateStructuredOutput(t, root, "doctor.schema.json", []byte(document)); err == nil {
			t.Fatalf("invalid document passed: %s", document)
		}
	}
}

func TestPackAuditSchemaRejectsWrongVersionAndUnknownFields(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	for _, document := range []string{
		`{"schema_version":2,"report":"packy-audit","result":"healthy","checks":[],"project":{"state":"absent","packs":0,"surfaces":0,"projections":0,"verified":0,"findings":0},"summary":{"passes":0,"infos":0,"warnings":0,"failures":0}}`,
		`{"schema_version":1,"report":"packy-audit","result":"healthy","checks":[],"project":{"state":"absent","packs":0,"surfaces":0,"projections":0,"verified":0,"findings":0},"summary":{"passes":0,"infos":0,"warnings":0,"failures":0},"unknown":true}`,
	} {
		if err := validateStructuredOutput(t, root, "pack-audit.schema.json", []byte(document)); err == nil {
			t.Fatalf("invalid audit document passed: %s", document)
		}
	}
}

func TestPackStatusSchemaAcceptsUnobservableExternalRequirementReason(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v11", "pack-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	entries := document["entries"].([]any)
	entry := entries[0].(map[string]any)
	entry["conditions"] = append(entry["conditions"].([]any), map[string]any{
		"type": "external-requirement", "scope": map[string]any{"kind": "global", "pack": entry["pack"], "surface": entry["surface"]},
		"dimension": "usable", "value": "unknown", "reason": "requirement-unobservable",
		"message": "external requirement cannot be observed", "evidence": []any{"executable:engram"},
		"freshness": map[string]any{"observed_at": "2026-08-09T00:00:00Z", "validity_identity": "engram/requirement"},
	})
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStructuredOutput(t, root, "pack-status.schema.json", encoded); err != nil {
		t.Fatalf("unobservable external requirement condition: %v", err)
	}
}

func TestPackLifecycleSchemaRejectsIncompleteReadinessCondition(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v11", "pack-lifecycle-preview.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	document["conditions"] = []any{map[string]any{}}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStructuredOutput(t, root, "pack-lifecycle.schema.json", encoded); err == nil {
		t.Fatal("lifecycle schema accepted an incomplete readiness condition")
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
	load := func(t *testing.T, name string) map[string]any {
		t.Helper()
		version := map[string]string{"pack-show.json": "v5", "pack-status.json": "v9"}[name]
		data, err := os.ReadFile(filepath.Join("testdata", "structured-output", version, name))
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
	t.Run("incomplete descriptive resource", func(t *testing.T) {
		document := load(t, "pack-show.json")
		resource := document["resource_inventory"].([]any)[0].(map[string]any)
		delete(resource, "description")
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
	case "pack-list":
		packs := document["packs"].([]any)
		if err := requireOrdered("packs", packs, objectKey("id")); err != nil {
			return err
		}
		for _, value := range packs {
			pack := value.(map[string]any)
			if err := requireStrings("packs.surfaces", pack["surfaces"].([]any)); err != nil {
				return err
			}
		}
	case "pack-show":
		if err := requireStrings("surfaces", document["surfaces"].([]any)); err != nil {
			return err
		}
		requires := document["requires"].(map[string]any)
		if err := requireStrings("requires.tools", requires["tools"].([]any)); err != nil {
			return err
		}
		inventory := document["resource_inventory"].([]any)
		if err := requireOrdered("resource_inventory", inventory, nestedObjectKey("resource", "kind", "id")); err != nil {
			return err
		}
		for _, value := range inventory {
			resource := value.(map[string]any)
			for _, field := range []string{"dependencies", "notices"} {
				if err := requireOrdered("resource_inventory."+field, resource[field].([]any), objectKey("kind", "id")); err != nil {
					return err
				}
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

func assertProjectStructuredOutput(t *testing.T, root, schemaName, instance string) {
	t.Helper()
	if err := validateProjectStructuredOutput(t, root, schemaName, instance); err != nil {
		t.Fatalf("%s producer: %v\n%s", schemaName, err, instance)
	}
}

func validateProjectStructuredOutput(t *testing.T, root, schemaName, instance string) error {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schemaRoot := filepath.Join(root, "schemas", "project", "v1.0.0")
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
			return fmt.Errorf("parse project schema %s: %w", entry.Name(), err)
		}
		if err := compiler.AddResource("https://yersonargotev.github.io/packy/schemas/project/v1.0.0/"+entry.Name(), document); err != nil {
			return err
		}
	}
	schema, err := compiler.Compile("https://yersonargotev.github.io/packy/schemas/project/v1.0.0/" + schemaName)
	if err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(instance))
	if err != nil {
		return err
	}
	return schema.Validate(value)
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
		return fmt.Errorf("compile schema %s: %w", schemaName, err)
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
