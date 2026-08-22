package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidatesProjectWithLocalOrigin(t *testing.T) {
	project := t.TempDir()
	origin := t.TempDir()
	writeToolFile(t, filepath.Join(project, "notices", "mit"), "MIT notice\n")
	writeToolFile(t, filepath.Join(project, "skills", "guide", "SKILL.md"), "guidance\n")
	writeToolFile(t, filepath.Join(origin, "guide", "SKILL.md"), "guidance\n")
	writeToolFile(t, filepath.Join(project, "pack.json"), toolManifest)

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--project", project, "--origin", "upstream=" + origin}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "validated example@1.0.0") || !strings.Contains(output, "files=3") {
		t.Fatalf("stdout = %q", output)
	}
}

func writeToolFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const toolManifest = `{
  "schema_version": 1,
  "id": "example",
  "version": "1.0.0",
  "description": "Example Managed Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [{"id":"upstream","repository":"example/upstream","commit":"0123456789abcdef0123456789abcdef01234567"}],
  "resources": [
    {
      "kind":"notice","id":"mit","source":"notices/mit","description":"MIT notice","license":"MIT","attribution":"Copyright Example",
      "requires":[],"conflicts":[],"bindings":[],"surface_exclusions":[]
    },
    {
      "kind":"skill","id":"guide","source":"skills/guide","description":"Guidance","requires":[],"conflicts":[],"notices":["notice:mit"],
      "origin":{"id":"upstream","path":"guide","relationship":"exact-copy"},
      "bindings":[{"surface":"codex","projection":"skill","name":"guide","invocation":"$guide","mode":"native","sharing":"exclusive","capabilities":[]}],
      "surface_exclusions":[]
    }
  ]
}
`
