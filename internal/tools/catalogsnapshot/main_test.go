package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBuildsAValidatedCatalogSnapshot(t *testing.T) {
	project := t.TempDir()
	writeCatalogFixture(t, project, "alpha", "1.2.3", "guidance\n")
	out := filepath.Join(t.TempDir(), "dist")
	commit := strings.Repeat("a", 40)
	builder := "yersonargotev/packy@" + strings.Repeat("b", 40)

	var stdout, stderr bytes.Buffer
	exit := run([]string{
		"--project", project,
		"--source-repository", "yersonargotev/packy-catalog",
		"--source-commit", commit,
		"--builder", builder,
		"--out-dir", out,
	}, &stdout, &stderr, nil)
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	for _, name := range []string{"catalog-snapshot.tar.gz", "SHA256SUMS"} {
		if info, err := os.Stat(filepath.Join(out, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("artifact %s: info=%v err=%v", name, info, err)
		}
	}
	for _, want := range []string{"built Catalog Snapshot", "alpha@1.2.3", commit, "catalog_sha256=", "archive_sha256="} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunRejectsIncompletePublicationIdentity(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := run([]string{"--project", t.TempDir()}, &stdout, &stderr, nil)
	if exit != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "source-repository") {
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
