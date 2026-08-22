package managedpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAdmissionRecordIsCanonicalAndAppendOnly(t *testing.T) {
	root := t.TempDir()
	record := validAdmissionRecord()
	path, err := WriteAdmissionRecord(root, record)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "example", "1.0.0.json") {
		t.Fatalf("path = %q", path)
	}
	loaded, err := LoadAdmissionRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ClosureSHA256 != record.ClosureSHA256 || loaded.Files[0].Path != "pack.json" {
		t.Fatalf("loaded = %#v", loaded)
	}
	if _, err := WriteAdmissionRecord(root, record); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second write error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "example"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "1.0.0.json" {
		t.Fatalf("admission directory entries = %#v", entries)
	}
}

func TestAdmissionRecordRejectsMutableOrInconsistentEvidence(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*AdmissionRecord)
		want string
	}{
		{"mutable release", func(record *AdmissionRecord) { record.ReleaseImmutable = false }, "immutable"},
		{"wrong tag", func(record *AdmissionRecord) { record.Tag = "v1.0.0" }, "pack-v1.0.0"},
		{"manifest mismatch", func(record *AdmissionRecord) { record.ManifestSHA256 = strings.Repeat("b", 64) }, "manifest digest"},
		{"closure mismatch", func(record *AdmissionRecord) { record.ClosureSHA256 = strings.Repeat("c", 64) }, "closure digest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := validAdmissionRecord()
			test.edit(&record)
			if _, err := MarshalAdmissionRecord(record); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func validAdmissionRecord() AdmissionRecord {
	manifestDigest := strings.Repeat("a", 64)
	files := []FileRecord{{Path: "pack.json", Mode: "100644", SHA256: manifestDigest}}
	return AdmissionRecord{
		SchemaVersion:    1,
		PackID:           "example",
		PackVersion:      "1.0.0",
		Project:          "example/managed-pack",
		RepositoryID:     101,
		ReleaseID:        202,
		ReleaseImmutable: true,
		Tag:              "pack-v1.0.0",
		TagObject:        "0123456789abcdef0123456789abcdef01234567",
		Commit:           "123456789abcdef0123456789abcdef012345678",
		RootTree:         "23456789abcdef0123456789abcdef0123456789",
		ManifestSHA256:   manifestDigest,
		ClosureSHA256:    digestIndex(files),
		Files:            files,
	}
}
