package managedpack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ValidateRepositoryIntegrity validates that a Packy repository has one
// internally consistent Managed Pack Registry, admission history, and current
// bundled Pack catalog.
func ValidateRepositoryIntegrity(ctx context.Context, repositoryRoot string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	managedPacksRoot := filepath.Join(repositoryRoot, "managed-packs")
	registry, err := LoadRegistry(filepath.Join(managedPacksRoot, "registry.json"))
	if err != nil {
		return err
	}
	registrations := make(map[string]Registration, len(registry.Packs))
	for _, registration := range registry.Packs {
		registrations[registration.PackID] = registration
	}

	admissions, err := loadRepositoryAdmissions(ctx, filepath.Join(managedPacksRoot, "admissions"), registrations)
	if err != nil {
		return err
	}
	return validateBundledCatalog(ctx, filepath.Join(repositoryRoot, "bundle"), registrations, admissions)
}

func loadRepositoryAdmissions(ctx context.Context, root string, registrations map[string]Registration) (map[string]AdmissionRecord, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read Pack Admission Record root: %w", err)
	}
	records := make(map[string]AdmissionRecord)
	seenPacks := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		registration, registered := registrations[entry.Name()]
		if !entry.IsDir() || !registered {
			return nil, fmt.Errorf("unexpected Pack Admission Record root entry %q", entry.Name())
		}
		seenPacks[entry.Name()] = true
		packRoot := filepath.Join(root, entry.Name())
		files, err := os.ReadDir(packRoot)
		if err != nil {
			return nil, fmt.Errorf("read Pack Admission Records for %q: %w", entry.Name(), err)
		}
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			info, err := file.Info()
			if err != nil {
				return nil, fmt.Errorf("inspect Pack Admission Record entry %q: %w", filepath.Join(entry.Name(), file.Name()), err)
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("Pack Admission Record entry %q must be a regular file", filepath.Join(entry.Name(), file.Name()))
			}
			path := filepath.Join(packRoot, file.Name())
			record, err := LoadAdmissionRecord(path)
			if err != nil {
				return nil, fmt.Errorf("validate Pack Admission Record %q: %w", filepath.Join(entry.Name(), file.Name()), err)
			}
			if record.PackID != entry.Name() || file.Name() != record.PackVersion+".json" {
				return nil, fmt.Errorf("Pack Admission Record path %q does not match record identity %s@%s", filepath.Join(entry.Name(), file.Name()), record.PackID, record.PackVersion)
			}
			if record.Project != registration.Project {
				return nil, fmt.Errorf("Pack Admission Record %s@%s project %q does not match registry project %q", record.PackID, record.PackVersion, record.Project, registration.Project)
			}
			key := admissionKey(record.PackID, record.PackVersion)
			if _, exists := records[key]; exists {
				return nil, fmt.Errorf("duplicate Pack Admission Record for %s@%s", record.PackID, record.PackVersion)
			}
			records[key] = record
		}
	}
	for packID := range registrations {
		if !seenPacks[packID] {
			return nil, fmt.Errorf("registered Pack %q has no Pack Admission Record directory", packID)
		}
	}
	return records, nil
}

func validateBundledCatalog(ctx context.Context, bundleRoot string, registrations map[string]Registration, admissions map[string]AdmissionRecord) error {
	packsRoot := filepath.Join(bundleRoot, "packs")
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		return fmt.Errorf("read bundled Pack catalog: %w", err)
	}
	bundled := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			return fmt.Errorf("unexpected bundled Pack catalog entry %q", entry.Name())
		}
		if _, registered := registrations[entry.Name()]; !registered {
			return fmt.Errorf("bundled Pack %q is not registered", entry.Name())
		}
		if err := validateBundledPack(ctx, bundleRoot, entry.Name(), admissions); err != nil {
			return err
		}
		bundled[entry.Name()] = true
	}
	for packID := range registrations {
		if !bundled[packID] {
			return fmt.Errorf("registered Pack %q is missing from the bundled Pack catalog", packID)
		}
	}
	return nil
}

func validateBundledPack(ctx context.Context, bundleRoot, packID string, admissions map[string]AdmissionRecord) error {
	packRoot := filepath.Join(bundleRoot, "packs", packID)
	entries, err := os.ReadDir(packRoot)
	if err != nil {
		return fmt.Errorf("read bundled Pack %q: %w", packID, err)
	}
	if len(entries) != 1 || entries[0].Name() != "pack.json" {
		return fmt.Errorf("bundled Pack directory %q must contain only pack.json", packID)
	}
	manifestPath := filepath.Join(packRoot, "pack.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return fmt.Errorf("inspect bundled Pack manifest %q: %w", packID, err)
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return fmt.Errorf("bundled Pack manifest %q must be a regular file", packID)
	}
	if manifestInfo.Size() > maxIndexedFileBytes {
		return fmt.Errorf("bundled Pack manifest %q exceeds maximum file size of %d bytes", packID, maxIndexedFileBytes)
	}
	manifestData, err := readFileBounded(ctx, manifestPath, manifestInfo)
	if err != nil {
		return fmt.Errorf("read bundled Pack manifest %q: %w", packID, err)
	}
	manifest, err := decodeManifest(manifestData)
	if err != nil {
		return fmt.Errorf("validate bundled Pack manifest %q: %w", packID, err)
	}
	if manifest.ID != packID {
		return fmt.Errorf("bundled Pack path %q does not match manifest identity %q", packID, manifest.ID)
	}
	if err := validateManifest(manifest, bundleRoot); err != nil {
		return fmt.Errorf("validate bundled Pack manifest %q: %w", packID, err)
	}

	record, exists := admissions[admissionKey(packID, manifest.Version)]
	if !exists {
		return fmt.Errorf("bundled Pack %s@%s has no exact Pack Admission Record", packID, manifest.Version)
	}
	files, err := declaredClosure(ctx, bundleRoot, manifest, manifestInfo.Size())
	if err != nil {
		return fmt.Errorf("validate bundled Pack %s@%s closure: %w", packID, manifest.Version, err)
	}
	manifestDigest := digestBytes(manifestData)
	files = append(files, FileRecord{Path: "pack.json", Mode: canonicalMode(manifestInfo.Mode()), SHA256: manifestDigest})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	closureDigest := digestIndex(files)

	if record.ManifestSHA256 != manifestDigest {
		return fmt.Errorf("bundled Pack %s@%s manifest digest does not match its Pack Admission Record", packID, manifest.Version)
	}
	if record.ClosureSHA256 != closureDigest {
		return fmt.Errorf("bundled Pack %s@%s closure digest does not match its Pack Admission Record", packID, manifest.Version)
	}
	if !equalFileRecords(files, record.Files) {
		return fmt.Errorf("bundled Pack %s@%s paths, modes, or SHA-256 digests do not match its Pack Admission Record", packID, manifest.Version)
	}
	return nil
}

func admissionKey(packID, version string) string {
	return packID + "\x00" + version
}

func equalFileRecords(left, right []FileRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
