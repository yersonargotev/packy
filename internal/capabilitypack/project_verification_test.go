package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyProjectRejectsAbsentContractDeterministically(t *testing.T) {
	report := VerifyProject(context.Background(), t.TempDir(), map[Surface]SurfaceAdapter{})
	if report.Result != ProjectVerificationFailed || report.ProjectRoot != "<project-root>" || report.Summary.Findings != 1 || len(report.Findings) != 1 || report.Findings[0].Code != "project_contract_absent" {
		t.Fatalf("verification report = %#v", report)
	}
}

func TestVerifyProjectTurnsInvalidContractIntoPortableFinding(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "packy.json"), []byte(`{"schema_version":99,"packs":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "packy.lock.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	report := VerifyProject(context.Background(), root, map[Surface]SurfaceAdapter{})
	if report.Result != ProjectVerificationFailed || len(report.Findings) != 1 || report.Findings[0].Code != "project_contract_invalid" {
		t.Fatalf("verification report = %#v", report)
	}
}
