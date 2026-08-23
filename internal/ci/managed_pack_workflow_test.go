package ci

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestManagedPackWorkflowRunsThePackyOwnedValidator(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "managed-pack-validation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		"workflow_call:",
		"permissions: {}",
		"persist-credentials: false",
		"go run ./internal/tools/managedpackvalidate --project ../managed-pack-project",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("Managed Pack workflow is missing %q", required)
		}
	}
}

func TestManagedPackWorkflowCommandPreflightsAMaterializedRuntimeFixture(t *testing.T) {
	project := t.TempDir()
	writeManagedPackWorkflowFile(t, filepath.Join(project, "skills", "guide", "SKILL.md"), "# Managed guidance\n")
	writeManagedPackWorkflowFile(t, filepath.Join(project, "pack.json"), `{
  "schema_version": 1,
  "id": "workflow-fixture",
  "version": "1.0.0",
  "description": "Managed Pack workflow fixture",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [
    {
      "kind": "skill",
      "id": "guide",
      "source": "skills/guide",
      "description": "Managed guidance",
      "requires": [],
      "conflicts": [],
      "bindings": [
        {
          "surface": "codex",
          "projection": "skill",
          "name": "guide",
          "invocation": "$guide",
          "mode": "native",
          "sharing": "exclusive",
          "capabilities": []
        }
      ],
      "surface_exclusions": []
    }
  ]
}
`)

	command := exec.Command("go", "run", "./internal/tools/managedpackvalidate", "--project", project)
	command.Dir = filepath.Join("..", "..")
	command.Env = testprocess.GoOfflineEnv(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Managed Pack workflow command failed: %v\n%s", err, output)
	}
	for _, want := range []string{"validated workflow-fixture@1.0.0", "files=2", "fitness_rows=2"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("Managed Pack workflow output = %q, want %q", output, want)
		}
	}
}

func writeManagedPackWorkflowFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
