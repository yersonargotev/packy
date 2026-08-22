package managedpack

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
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
	if !reflect.DeepEqual(loaded, record) {
		t.Fatalf("loaded = %#v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	canonicalFields := []string{`"tag":`, `"tag_ref_type":`, `"tag_ref_sha":`, `"tag_objects":`, `"commit":`}
	previous := -1
	for _, field := range canonicalFields {
		at := strings.Index(string(data), field)
		if at <= previous {
			t.Fatalf("canonical field %s is missing or out of order in %s", field, data)
		}
		previous = at
	}
	if strings.Contains(string(data), `"tag_object":`) {
		t.Fatalf("record contains retired tag_object field: %s", data)
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

func TestAdmissionRecordSupportsLightweightTag(t *testing.T) {
	record := validAdmissionRecord()
	record.TagRefType = "commit"
	record.TagRefSHA = record.Commit
	record.TagObjects = []TagObject{}

	data, err := MarshalAdmissionRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"tag_objects\": []") {
		t.Fatalf("lightweight tag_objects = %s", data)
	}
}

func TestAdmissionRecordRejectsBrokenTagObjectChains(t *testing.T) {
	otherSHA := strings.Repeat("f", 40)
	for _, test := range []struct {
		name string
		edit func(*AdmissionRecord)
		want string
	}{
		{"missing tag objects", func(record *AdmissionRecord) { record.TagObjects = nil }, "tag_objects must be an array"},
		{"invalid tag ref SHA", func(record *AdmissionRecord) { record.TagRefSHA = "ABC" }, "tag_ref_sha"},
		{"annotated tag ref moved", func(record *AdmissionRecord) { record.TagRefSHA = otherSHA }, "first tag object"},
		{"tag chain moved", func(record *AdmissionRecord) { record.TagObjects[0].TargetSHA = otherSHA }, "continuous"},
		{"commit target moved", func(record *AdmissionRecord) { record.TagObjects[1].TargetSHA = otherSHA }, "continuous"},
		{"intermediate target type changed", func(record *AdmissionRecord) { record.TagObjects[0].TargetType = "commit" }, "continuous"},
		{"final target type changed", func(record *AdmissionRecord) { record.TagObjects[1].TargetType = "tag" }, "continuous"},
		{"invalid object SHA", func(record *AdmissionRecord) {
			record.TagObjects[0].TargetSHA = "ABC"
			record.TagObjects[1].SHA = "ABC"
		}, "object IDs"},
		{"repeated object", func(record *AdmissionRecord) {
			record.TagObjects[1].SHA = record.TagObjects[0].SHA
			record.TagObjects[0].TargetSHA = record.TagObjects[0].SHA
		}, "must not repeat"},
		{"lightweight tag ref moved", func(record *AdmissionRecord) {
			record.TagRefType = "commit"
			record.TagRefSHA = otherSHA
			record.TagObjects = []TagObject{}
		}, "point directly to commit"},
		{"lightweight tag has objects", func(record *AdmissionRecord) { record.TagRefType = "commit" }, "annotated tag ref"},
		{"annotated tag has no objects", func(record *AdmissionRecord) {
			record.TagRefType = "tag"
			record.TagObjects = []TagObject{}
		}, "lightweight tag ref"},
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

func TestLoadAdmissionRecordRejectsRetiredTagObject(t *testing.T) {
	record := validAdmissionRecord()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "tag_ref_type")
	delete(document, "tag_ref_sha")
	delete(document, "tag_objects")
	document["tag_object"] = record.TagRefSHA
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAdmissionRecord(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("legacy load error = %v", err)
	}
}

func TestAdmissionRecordSchemaDistinguishesLightweightAndAnnotatedTags(t *testing.T) {
	compiled := compileSchema(t, filepath.Join("..", "..", "schemas", "managed-pack", "v1", "admission-record.schema.json"), "admission-record.schema.json")
	annotated := validAdmissionRecord()
	lightweight := validAdmissionRecord()
	lightweight.TagRefType = "commit"
	lightweight.TagRefSHA = lightweight.Commit
	lightweight.TagObjects = []TagObject{}

	for name, record := range map[string]AdmissionRecord{"annotated": annotated, "lightweight": lightweight} {
		t.Run(name, func(t *testing.T) {
			if err := validateAdmissionRecordSchema(compiled, record); err != nil {
				t.Fatal(err)
			}
		})
	}

	invalid := validAdmissionRecord()
	invalid.TagRefType = "commit"
	if err := validateAdmissionRecordSchema(compiled, invalid); err == nil {
		t.Fatal("schema accepted tag objects for a lightweight tag ref")
	}
	invalid = lightweight
	invalid.TagRefType = "tag"
	if err := validateAdmissionRecordSchema(compiled, invalid); err == nil {
		t.Fatal("schema accepted an empty annotated tag chain")
	}
}

func validateAdmissionRecordSchema(compiled *jsonschema.Schema, record AdmissionRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return compiled.Validate(instance)
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
		TagRefType:       "tag",
		TagRefSHA:        "0123456789abcdef0123456789abcdef01234567",
		TagObjects: []TagObject{
			{SHA: "0123456789abcdef0123456789abcdef01234567", TargetSHA: "123456789abcdef0123456789abcdef012345678", TargetType: "tag"},
			{SHA: "123456789abcdef0123456789abcdef012345678", TargetSHA: "23456789abcdef0123456789abcdef0123456789", TargetType: "commit"},
		},
		Commit:         "23456789abcdef0123456789abcdef0123456789",
		RootTree:       "3456789abcdef0123456789abcdef0123456789a",
		ManifestSHA256: manifestDigest,
		ClosureSHA256:  digestIndex(files),
		Files:          files,
	}
}
