package managedpack

import (
	"context"
	"encoding/json"
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
	Validation            Validation
	RuntimeManifest       capabilitypack.Pack
	RuntimeManifestSHA256 string
	Fitness               capabilitypack.RuntimeFitnessMatrix
}

// PreflightEvidence is the deterministic, serializable proof returned by
// preventive and offline-promotion adapters. It is informational; promotion
// still reacquires content and runs every repository admission gate.
type PreflightEvidence struct {
	Validation            Validation             `json:"validation"`
	RuntimeManifestSHA256 string                 `json:"runtime_manifest_sha256"`
	Fitness               RuntimeFitnessEvidence `json:"fitness"`
}

// RuntimeFitnessEvidence seals a complete deterministic fitness matrix in a
// protocol-bounded representation. RowCount makes its shape inspectable while
// SHA256 preserves exact preventive/promotion parity without repeating every
// dependency closure in worker output.
type RuntimeFitnessEvidence struct {
	RowCount int    `json:"row_count"`
	SHA256   string `json:"sha256"`
}

// Evidence returns the serializable identity and fitness result for this
// exact preflight without exposing its temporary materialized bundle.
func (result PreflightResult) Evidence() PreflightEvidence {
	fitnessData, err := json.Marshal(result.Fitness)
	if err != nil {
		panic(fmt.Sprintf("encode runtime fitness evidence: %v", err))
	}
	return PreflightEvidence{
		Validation: result.Validation, RuntimeManifestSHA256: result.RuntimeManifestSHA256,
		Fitness: RuntimeFitnessEvidence{RowCount: len(result.Fitness.Rows), SHA256: digestBytes(fitnessData)},
	}
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
	runtimeManifestData, err := json.Marshal(runtimeManifest)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("encode runtime manifest identity: %w", err)
	}
	fitness, err := capabilitypack.EvaluateRuntimeFitness(runtimeManifest)
	if err != nil {
		return PreflightResult{}, &runtimeFitnessError{err: err}
	}
	return PreflightResult{
		Validation: validation, RuntimeManifest: runtimeManifest,
		RuntimeManifestSHA256: digestBytes(runtimeManifestData), Fitness: fitness,
	}, nil
}
