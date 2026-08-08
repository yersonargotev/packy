package packsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestPackSourceV230IsCompleteOfflineSuiteAndEarlierSuitesAreImmutable(t *testing.T) {
	root := filepath.Join("..", "..", "schemas", "pack-source")
	entries, err := os.ReadDir(filepath.Join(root, "v2.3.0"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("v2.3.0 suite has %d documents, want 5", len(entries))
	}
	compiler := jsonschema.NewCompiler()
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, "v2.3.0", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var header struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(data, &header); err != nil || header.ID != "https://yersonargotev.github.io/packy/schemas/pack-source/v2.3.0/"+entry.Name() {
			t.Fatalf("%s canonical id = %q, err=%v", entry.Name(), header.ID, err)
		}
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(header.ID, document); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range entries {
		id := "https://yersonargotev.github.io/packy/schemas/pack-source/v2.3.0/" + entry.Name()
		if _, err := compiler.Compile(id); err != nil {
			t.Fatalf("compile %s offline: %v", entry.Name(), err)
		}
	}
	if got := earlierPackSourceSuitesDigest(t, root); got != "e27f56ec137de8b6e7eb8d2d3eb379b8b00693e46afc76244e645ba741e9da56" {
		t.Fatalf("earlier immutable suites changed: %s", got)
	}
}

func TestPackSourceV230AcceptsOnlyCompleteInitialAdmissionShapes(t *testing.T) {
	root := filepath.Join("..", "..", "schemas", "pack-source", "v2.3.0")
	compiler := loadPackSourceSuite(t, root)
	dispatch, err := compiler.Compile("https://yersonargotev.github.io/packy/schemas/pack-source/v2.3.0/pack-source-dispatch.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := map[string]any{
		"schema_version": 2, "operation": "register", "source_id": "orchestrate-source",
		"selector": "latest-stable", "classification_mode": "ai", "request_reason": "initial admission",
		"registration": map[string]any{
			"id": "orchestrate-source", "provider": "github", "repository": "example/orchestrate",
			"selector":  map[string]any{"mode": "stable-release"},
			"resources": []any{map[string]any{"pack_id": "orchestrate", "kind": "skill", "resource_id": "orchestrate", "upstream_path": "orchestrate"}},
		},
		"registration_sha256": sha, "proposed_version": "1.0.0",
		"proposed_manifest":        map[string]any{"id": "orchestrate", "version": "1.0.0"},
		"proposed_manifest_sha256": sha,
		"legal_admission":          map[string]any{"evidence_reference": "docs/evidence.json", "evidence_sha256": sha, "disposition": "redistributable"},
	}
	if err := dispatch.Validate(request); err != nil {
		t.Fatalf("complete initial request rejected: %v", err)
	}
	for _, field := range []string{"proposed_version", "proposed_manifest", "proposed_manifest_sha256", "legal_admission"} {
		partial := cloneJSONMap(t, request)
		delete(partial, field)
		if err := dispatch.Validate(partial); err == nil {
			t.Fatalf("partial initial request without %s accepted", field)
		}
	}
	legacy := cloneJSONMap(t, request)
	for _, field := range []string{"proposed_version", "proposed_manifest", "proposed_manifest_sha256", "legal_admission"} {
		delete(legacy, field)
	}
	if err := dispatch.Validate(legacy); err != nil {
		t.Fatalf("existing v2 registration shape changed: %v", err)
	}

	validation, err := compiler.Compile("https://yersonargotev.github.io/packy/schemas/pack-source/v2.3.0/pack-source-validation.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{
		"schema_version": 2, "source_id": "orchestrate-source", "plan_id": "pack-sync-plan",
		"base_sha": strings.Repeat("b", 40), "candidate_sha": strings.Repeat("c", 40),
		"source_lock_sha256": sha, "lock_set_sha256": sha, "config_sha256": sha, "manifests_sha256": sha,
		"result_tree_sha": strings.Repeat("d", 40), "packy_suite": true, "apply": true, "contains_upstream_bytes": false,
		"pack_id": "orchestrate", "proposed_version": "1.0.0", "proposed_manifest_sha256": sha,
		"legal_evidence_reference": "docs/evidence.json", "legal_evidence_sha256": sha, "result_bundle_sha256": sha,
	}
	if err := validation.Validate(proof); err != nil {
		t.Fatalf("complete initial validation artifact rejected: %v", err)
	}
}

func loadPackSourceSuite(t *testing.T, root string) *jsonschema.Compiler {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		id := "https://yersonargotev.github.io/packy/schemas/pack-source/v2.3.0/" + entry.Name()
		if err := compiler.AddResource(id, document); err != nil {
			t.Fatal(err)
		}
	}
	return compiler
}

func cloneJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func earlierPackSourceSuitesDigest(t *testing.T, root string) string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Base(filepath.Dir(path)) == "v2.3.0" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var framed bytes.Buffer
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		relative, err := filepath.Rel(filepath.Join(root, "..", ".."), path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&framed, "%x  %s\n", digest, filepath.ToSlash(relative))
	}
	return fmt.Sprintf("%x", sha256.Sum256(framed.Bytes()))
}
