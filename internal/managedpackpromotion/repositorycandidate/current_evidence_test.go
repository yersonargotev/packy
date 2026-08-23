package repositorycandidate

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
)

func TestCurrentFileEvidenceUsesTheSealedAdmissionIndex(t *testing.T) {
	_, validation := managedProject(t, "1.1.0", "admitted\n", nil)
	repositoryRoot := t.TempDir()
	if _, err := managedpack.WriteAdmissionRecord(repositoryRoot+"/managed-packs/admissions", admissionRecord(validation)); err != nil {
		t.Fatal(err)
	}

	got, err := currentFileEvidence(repositoryRoot, validation.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, validation.Files) {
		t.Fatalf("current file evidence = %#v\nwant sealed admission files %#v", got, validation.Files)
	}
}

func TestCurrentFileEvidenceIndexesTheSealedBaseForALegacyAdmission(t *testing.T) {
	repositoryRoot := t.TempDir()
	writeTestFile(t, filepath.Join(repositoryRoot, "bundle", "skills", "guide", "SKILL.md"), "old\n", 0o644)
	current := managedpack.Manifest{
		ID:      "example",
		Version: "1.0.0",
		Resources: []managedpack.Resource{{
			Kind: "skill", ID: "guide", Source: "skills/guide",
		}},
	}

	got, err := currentFileEvidence(repositoryRoot, current)
	if err != nil {
		t.Fatal(err)
	}
	want := []managedpack.FileRecord{{
		Path: "skills/guide/SKILL.md", Mode: "100644",
		SHA256: "01d09d19c2139a46aebfb577780d123d7396e97201bc7ead210a2ebff8239dee",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("current file evidence = %#v\nwant base-sealed files %#v", got, want)
	}
}
