package repositorycandidate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProductionSuiteGateRunsTheCompleteCIValidation(t *testing.T) {
	repositoryRoot := t.TempDir()
	scriptPath := filepath.Join(repositoryRoot, "scripts", "validate-packy.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\ntest \"$#\" -eq 1 && test \"$1\" = \"--ci\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := (productionGates{}).ValidateSuite(context.Background(), repositoryRoot); err != nil {
		t.Fatal(err)
	}
}
