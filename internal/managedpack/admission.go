package managedpack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Masterminds/semver/v3"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TagObject pins one annotated Git tag object and its immediate target.
type TagObject struct {
	SHA        string `json:"sha"`
	TargetSHA  string `json:"target_sha"`
	TargetType string `json:"target_type"`
}

// AdmissionRecord pins one immutable admitted Managed Pack generation.
type AdmissionRecord struct {
	SchemaVersion    int          `json:"schema_version"`
	PackID           string       `json:"pack_id"`
	PackVersion      string       `json:"pack_version"`
	Project          string       `json:"project"`
	RepositoryID     int64        `json:"repository_id"`
	ReleaseID        int64        `json:"release_id"`
	ReleaseImmutable bool         `json:"release_immutable"`
	Tag              string       `json:"tag"`
	TagRefType       string       `json:"tag_ref_type"`
	TagRefSHA        string       `json:"tag_ref_sha"`
	TagObjects       []TagObject  `json:"tag_objects"`
	Commit           string       `json:"commit"`
	RootTree         string       `json:"root_tree"`
	ManifestSHA256   string       `json:"manifest_sha256"`
	ClosureSHA256    string       `json:"closure_sha256"`
	Files            []FileRecord `json:"files"`
}

type admissionWire struct {
	SchemaVersion    int          `json:"schema_version"`
	PackID           string       `json:"pack_id"`
	PackVersion      string       `json:"pack_version"`
	Project          string       `json:"project"`
	RepositoryID     int64        `json:"repository_id"`
	ReleaseID        int64        `json:"release_id"`
	ReleaseImmutable *bool        `json:"release_immutable"`
	Tag              string       `json:"tag"`
	TagRefType       string       `json:"tag_ref_type"`
	TagRefSHA        string       `json:"tag_ref_sha"`
	TagObjects       []TagObject  `json:"tag_objects"`
	Commit           string       `json:"commit"`
	RootTree         string       `json:"root_tree"`
	ManifestSHA256   string       `json:"manifest_sha256"`
	ClosureSHA256    string       `json:"closure_sha256"`
	Files            []FileRecord `json:"files"`
}

// MarshalAdmissionRecord returns the canonical append-only JSON form.
func MarshalAdmissionRecord(record AdmissionRecord) ([]byte, error) {
	if err := validateAdmissionRecord(record); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Pack Admission Record: %w", err)
	}
	return append(data, '\n'), nil
}

// LoadAdmissionRecord strictly loads and validates one record.
func LoadAdmissionRecord(path string) (AdmissionRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AdmissionRecord{}, fmt.Errorf("read Pack Admission Record: %w", err)
	}
	var wire admissionWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return AdmissionRecord{}, fmt.Errorf("decode Pack Admission Record: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return AdmissionRecord{}, fmt.Errorf("decode Pack Admission Record: %w", err)
	}
	if wire.ReleaseImmutable == nil {
		return AdmissionRecord{}, fmt.Errorf("Pack Admission Record release_immutable is required")
	}
	record := AdmissionRecord{
		SchemaVersion: wire.SchemaVersion, PackID: wire.PackID, PackVersion: wire.PackVersion,
		Project: wire.Project, RepositoryID: wire.RepositoryID, ReleaseID: wire.ReleaseID,
		ReleaseImmutable: *wire.ReleaseImmutable, Tag: wire.Tag, TagRefType: wire.TagRefType,
		TagRefSHA: wire.TagRefSHA, TagObjects: wire.TagObjects,
		Commit: wire.Commit, RootTree: wire.RootTree, ManifestSHA256: wire.ManifestSHA256,
		ClosureSHA256: wire.ClosureSHA256, Files: wire.Files,
	}
	if err := validateAdmissionRecord(record); err != nil {
		return AdmissionRecord{}, err
	}
	return record, nil
}

// WriteAdmissionRecord creates one record at <root>/<pack>/<version>.json and
// refuses to replace any existing generation.
func WriteAdmissionRecord(root string, record AdmissionRecord) (string, error) {
	data, err := MarshalAdmissionRecord(record)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(root, record.PackID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create Pack Admission Record directory: %w", err)
	}
	path := filepath.Join(directory, record.PackVersion+".json")
	temporary, err := os.CreateTemp(directory, ".admission-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary Pack Admission Record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("set Pack Admission Record mode: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write Pack Admission Record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync Pack Admission Record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close Pack Admission Record: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("Pack Admission Record %s already exists", path)
		}
		return "", fmt.Errorf("publish Pack Admission Record: %w", err)
	}
	if err := os.Remove(temporaryPath); err != nil {
		return "", fmt.Errorf("remove published Pack Admission Record temporary file: %w", err)
	}
	return path, nil
}

func validateAdmissionRecord(record AdmissionRecord) error {
	if record.SchemaVersion != SchemaVersion {
		return fmt.Errorf("Pack Admission Record schema_version must be %d", SchemaVersion)
	}
	if !idPattern.MatchString(record.PackID) {
		return fmt.Errorf("Pack Admission Record pack_id must be lowercase kebab-case")
	}
	if _, err := semver.StrictNewVersion(record.PackVersion); err != nil {
		return fmt.Errorf("Pack Admission Record pack_version must be SemVer")
	}
	if !repositoryPattern.MatchString(record.Project) {
		return fmt.Errorf("Pack Admission Record project must be an owner/name identity")
	}
	if record.RepositoryID <= 0 || record.ReleaseID <= 0 {
		return fmt.Errorf("Pack Admission Record repository_id and release_id must be positive")
	}
	if !record.ReleaseImmutable {
		return fmt.Errorf("Pack Admission Record release must be immutable")
	}
	wantTag := "pack-v" + record.PackVersion
	if record.Tag != wantTag {
		return fmt.Errorf("Pack Admission Record tag must be %q", wantTag)
	}
	for field, value := range map[string]string{"tag_ref_sha": record.TagRefSHA, "commit": record.Commit, "root_tree": record.RootTree} {
		if !commitPattern.MatchString(value) {
			return fmt.Errorf("Pack Admission Record %s must be a full lowercase Git object ID", field)
		}
	}
	if record.TagObjects == nil {
		return fmt.Errorf("Pack Admission Record tag_objects must be an array")
	}
	if len(record.TagObjects) == 0 {
		if record.TagRefType != "commit" || record.TagRefSHA != record.Commit {
			return fmt.Errorf("Pack Admission Record lightweight tag ref must point directly to commit")
		}
	} else {
		if record.TagRefType != "tag" || record.TagRefSHA != record.TagObjects[0].SHA {
			return fmt.Errorf("Pack Admission Record annotated tag ref must point to the first tag object")
		}
		seenObjects := make(map[string]struct{}, len(record.TagObjects))
		for i, object := range record.TagObjects {
			if !commitPattern.MatchString(object.SHA) || !commitPattern.MatchString(object.TargetSHA) {
				return fmt.Errorf("Pack Admission Record tag object IDs must be full lowercase Git object IDs")
			}
			if _, exists := seenObjects[object.SHA]; exists {
				return fmt.Errorf("Pack Admission Record tag object chain must not repeat objects")
			}
			seenObjects[object.SHA] = struct{}{}
			wantSHA := record.Commit
			wantType := "commit"
			if i+1 < len(record.TagObjects) {
				wantSHA = record.TagObjects[i+1].SHA
				wantType = "tag"
			}
			if object.TargetSHA != wantSHA || object.TargetType != wantType {
				return fmt.Errorf("Pack Admission Record tag object chain must be continuous through commit")
			}
		}
	}
	if !sha256Pattern.MatchString(record.ManifestSHA256) || !sha256Pattern.MatchString(record.ClosureSHA256) {
		return fmt.Errorf("Pack Admission Record digests must be lowercase SHA-256")
	}
	if record.Files == nil || len(record.Files) == 0 {
		return fmt.Errorf("Pack Admission Record files must be a non-empty array")
	}
	manifestFound := false
	for i, file := range record.Files {
		if err := validateRelativePath(file.Path, false); err != nil {
			return fmt.Errorf("Pack Admission Record file path: %w", err)
		}
		if i > 0 && record.Files[i-1].Path >= file.Path {
			return fmt.Errorf("Pack Admission Record files must be sorted by path without duplicates")
		}
		if file.Mode != "100644" && file.Mode != "100755" {
			return fmt.Errorf("Pack Admission Record file %q has unsupported mode %q", file.Path, file.Mode)
		}
		if !sha256Pattern.MatchString(file.SHA256) {
			return fmt.Errorf("Pack Admission Record file %q digest must be lowercase SHA-256", file.Path)
		}
		if file.Path == "pack.json" {
			manifestFound = true
			if file.SHA256 != record.ManifestSHA256 {
				return fmt.Errorf("Pack Admission Record manifest digest does not match pack.json")
			}
		}
	}
	if !manifestFound {
		return fmt.Errorf("Pack Admission Record files must contain pack.json")
	}
	if digestIndex(record.Files) != record.ClosureSHA256 {
		return fmt.Errorf("Pack Admission Record closure digest does not match files")
	}
	return nil
}
