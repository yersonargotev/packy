package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidatesACompleteCatalogAgainstItsBaseline(t *testing.T) {
	baseline := t.TempDir()
	writeCatalogFixture(t, baseline, "alpha", "1.0.0", "old\n")
	candidate := t.TempDir()
	writeCatalogFixture(t, candidate, "alpha", "1.1.0", "new\n")
	writeCatalogFixture(t, candidate, "beta", "1.0.0", "added\n")

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--project", candidate, "--baseline", baseline}, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	for _, want := range []string{"validated Catalog Project packs=2", "alpha@1.1.0", "beta@1.0.0"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunReportsPackVersionFailure(t *testing.T) {
	baseline := t.TempDir()
	writeCatalogFixture(t, baseline, "alpha", "1.0.0", "old\n")
	candidate := t.TempDir()
	writeCatalogFixture(t, candidate, "alpha", "1.0.0", "new\n")

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--project", candidate, "--baseline", baseline}, &stdout, &stderr)
	if exit != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "changed content requires a version greater than 1.0.0") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
}

func writeCatalogFixture(t *testing.T, root, id, version, content string) {
	t.Helper()
	source := filepath.Join(root, "bundle", "skills", id, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("{\"schema_version\":1,\"id\":%q,\"version\":%q,\"description\":\"Fixture Pack\",\"selectable\":true,\"surfaces\":[\"codex\"],\"readiness_obligations\":[\"runtime-usability\",\"surface-authorization\"],\"external_requirements\":[],\"origins\":[],\"resources\":[{\"kind\":\"skill\",\"id\":%q,\"source\":%q,\"description\":\"Fixture skill\",\"requires\":[],\"conflicts\":[],\"bindings\":[{\"surface\":\"codex\",\"projection\":\"skill\",\"name\":%q,\"invocation\":%q,\"mode\":\"native\",\"sharing\":\"exclusive\",\"capabilities\":[]}],\"surface_exclusions\":[]}]}\n", id, version, id, "skills/"+id, id, "$"+id)
	manifestPath := filepath.Join(root, "bundle", "packs", id, "pack.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}
