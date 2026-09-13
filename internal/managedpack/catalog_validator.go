package managedpack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver/v3"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

// CatalogValidation is the deterministic result of validating one complete
// Catalog Project. Packs are ordered by identity.
type CatalogValidation struct {
	Packs       []Validation
	FitnessRows int
}

// ValidateCatalogProject validates every Pack and its Declared Pack Closure as
// inert data. When baselineRoot is non-empty, it also enforces independent Pack
// version changes between the two complete Catalog Projects.
func ValidateCatalogProject(ctx context.Context, projectRoot, baselineRoot string, resolver OriginResolver) (CatalogValidation, error) {
	candidate, err := validateCatalog(ctx, projectRoot, resolver)
	if err != nil {
		return CatalogValidation{}, err
	}
	if baselineRoot == "" {
		return candidate, nil
	}
	baseline, err := validateCatalog(ctx, baselineRoot, resolver)
	if err != nil {
		return CatalogValidation{}, fmt.Errorf("validate baseline Catalog Project: %w", err)
	}
	if err := validateCatalogVersions(candidate, baseline); err != nil {
		return CatalogValidation{}, err
	}
	return candidate, nil
}

func validateCatalog(ctx context.Context, projectRoot string, resolver OriginResolver) (CatalogValidation, error) {
	bundleRoot := filepath.Join(projectRoot, "bundle")
	packsRoot := filepath.Join(bundleRoot, "packs")
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		return CatalogValidation{}, fmt.Errorf("read Catalog Project Packs: %w", err)
	}
	if len(entries) == 0 {
		return CatalogValidation{}, fmt.Errorf("Catalog Project must contain at least one Pack")
	}

	result := CatalogValidation{Packs: make([]Validation, 0, len(entries))}
	owners := map[string]string{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return CatalogValidation{}, err
		}
		if !entry.IsDir() {
			return CatalogValidation{}, fmt.Errorf("unexpected Catalog Project Pack entry %q", entry.Name())
		}
		validation, err := ValidateCatalogPack(ctx, bundleRoot, entry.Name(), resolver)
		if err != nil {
			return CatalogValidation{}, fmt.Errorf("validate Pack %q: %w", entry.Name(), err)
		}
		for _, file := range validation.Files {
			if owner, exists := owners[file.Path]; exists {
				return CatalogValidation{}, fmt.Errorf("declared closure path %q is owned by Packs %q and %q", file.Path, owner, validation.Manifest.ID)
			}
			owners[file.Path] = validation.Manifest.ID
		}
		manifestPath := filepath.Join(bundleRoot, "packs", entry.Name(), "pack.json")
		runtimePack, err := capabilitypack.LoadCurrentManifest(manifestPath, bundleRoot, true)
		if err != nil {
			return CatalogValidation{}, fmt.Errorf("load Pack %q through runtime manifest: %w", entry.Name(), err)
		}
		fitness, err := capabilitypack.EvaluateRuntimeFitness(runtimePack)
		if err != nil {
			return CatalogValidation{}, fmt.Errorf("evaluate Pack %q runtime fitness: %w", entry.Name(), err)
		}
		result.FitnessRows += len(fitness.Rows)
		result.Packs = append(result.Packs, validation)
	}
	return result, nil
}

func validateCatalogVersions(candidate, baseline CatalogValidation) error {
	baselinePacks := make(map[string]Validation, len(baseline.Packs))
	for _, pack := range baseline.Packs {
		baselinePacks[pack.Manifest.ID] = pack
	}
	for _, current := range candidate.Packs {
		previous, exists := baselinePacks[current.Manifest.ID]
		if !exists {
			continue
		}
		unchanged, err := samePackContent(current, previous)
		if err != nil {
			return fmt.Errorf("compare Pack %q content: %w", current.Manifest.ID, err)
		}
		if unchanged {
			if current.Manifest.Version != previous.Manifest.Version {
				return fmt.Errorf("Pack %q unchanged content must retain version %s", current.Manifest.ID, previous.Manifest.Version)
			}
			continue
		}
		currentVersion, err := semver.StrictNewVersion(current.Manifest.Version)
		if err != nil {
			return fmt.Errorf("parse Pack %q version: %w", current.Manifest.ID, err)
		}
		previousVersion, err := semver.StrictNewVersion(previous.Manifest.Version)
		if err != nil {
			return fmt.Errorf("parse baseline Pack %q version: %w", current.Manifest.ID, err)
		}
		if !currentVersion.GreaterThan(previousVersion) {
			return fmt.Errorf("Pack %q changed content requires a version greater than %s", current.Manifest.ID, previous.Manifest.Version)
		}
	}
	return nil
}

func samePackContent(left, right Validation) (bool, error) {
	leftManifest := left.Manifest
	rightManifest := right.Manifest
	leftManifest.Version = ""
	rightManifest.Version = ""
	type contentIdentity struct {
		Manifest Manifest
		Files    []FileRecord
	}
	leftData, err := json.Marshal(contentIdentity{Manifest: leftManifest, Files: resourceFiles(left)})
	if err != nil {
		return false, err
	}
	rightData, err := json.Marshal(contentIdentity{Manifest: rightManifest, Files: resourceFiles(right)})
	if err != nil {
		return false, err
	}
	return digestBytes(leftData) == digestBytes(rightData), nil
}

func resourceFiles(validation Validation) []FileRecord {
	manifestPath := filepath.ToSlash(filepath.Join("packs", validation.Manifest.ID, "pack.json"))
	files := make([]FileRecord, 0, len(validation.Files)-1)
	for _, file := range validation.Files {
		if file.Path != manifestPath && file.Path != "pack.json" {
			files = append(files, file)
		}
	}
	return files
}
