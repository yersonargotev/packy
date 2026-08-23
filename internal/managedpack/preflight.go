package managedpack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

// PreflightResult seals the authoring validation together with the exact
// runtime manifest and deterministic fitness matrix loaded from its
// materialized Declared Pack Closure.
type PreflightResult struct {
	Validation      Validation
	RuntimeManifest capabilitypack.Pack
	Fitness         capabilitypack.RuntimeFitnessMatrix
}

type runtimeFitnessError struct {
	err error
}

func (e *runtimeFitnessError) Error() string { return fmt.Sprintf("runtime fitness: %v", e.err) }
func (e *runtimeFitnessError) Unwrap() error { return e.err }

// IsRuntimeFitnessFailure reports whether preflight rejected the
// materialized runtime manifest or one of its surface-selection rows.
func IsRuntimeFitnessFailure(err error) bool {
	var failure *runtimeFitnessError
	return errors.As(err, &failure)
}

// Preflight validates one Managed Pack Project, materializes its exact sealed
// closure into temporary runtime bundle layout, loads that artifact through
// the production manifest path, and evaluates every selectable runtime row.
// It reads but never executes project or origin content.
func Preflight(ctx context.Context, projectRoot string, resolver OriginResolver) (PreflightResult, error) {
	validation, err := ValidateProject(ctx, projectRoot, resolver)
	if err != nil {
		return PreflightResult{}, err
	}
	bundleRoot, err := os.MkdirTemp("", "packy-managed-preflight-")
	if err != nil {
		return PreflightResult{}, fmt.Errorf("create temporary runtime bundle: %w", err)
	}
	defer os.RemoveAll(bundleRoot)

	if err := MaterializeClosure(ctx, projectRoot, bundleRoot, validation); err != nil {
		return PreflightResult{}, fmt.Errorf("materialize validated closure: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return PreflightResult{}, err
	}
	manifestPath := filepath.Join(bundleRoot, "packs", validation.Manifest.ID, "pack.json")
	runtimeManifest, err := capabilitypack.LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err != nil {
		return PreflightResult{}, &runtimeFitnessError{err: fmt.Errorf("load materialized Pack: %w", err)}
	}
	if err := ctx.Err(); err != nil {
		return PreflightResult{}, err
	}
	fitness, err := capabilitypack.EvaluateRuntimeFitness(runtimeManifest)
	if err != nil {
		return PreflightResult{}, &runtimeFitnessError{err: err}
	}
	return PreflightResult{Validation: validation, RuntimeManifest: runtimeManifest, Fitness: fitness}, nil
}
