package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
