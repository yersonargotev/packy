package managedpackpromotion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/managedpack"
)

// Module hides the acquisition, validation, candidate, and publication phases
// behind the one Managed Pack Promotion interface.
type Module struct {
	acquirer  Acquirer
	validator OfflineValidator
	preparer  CandidatePreparer
	publisher Publisher
}

func NewModule(acquirer Acquirer, validator OfflineValidator, preparer CandidatePreparer, publisher Publisher) Module {
	return Module{acquirer: acquirer, validator: validator, preparer: preparer, publisher: publisher}
}

// Promote returns exactly one no-change, typed rejection, or protected
// proposal. Operational failures and cancellation remain errors.
func (module Module) Promote(ctx context.Context, request Request) (result Result, err error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	registry, err := managedpack.LoadRegistry(filepath.Join(request.RepositoryRoot, "managed-packs", "registry.json"))
	if err != nil {
		return Result{}, err
	}
	project := ""
	for _, registration := range registry.Packs {
		if registration.PackID == request.Coordinate.PackID {
			project = registration.Project
			break
		}
	}
	if project == "" {
		return rejected(GateRegistration, fmt.Sprintf("Pack %q is not registered", request.Coordinate.PackID)), nil
	}

	acquisition, err := module.acquirer.Acquire(ctx, project, request.Coordinate)
	if err != nil {
		return resultForError(err)
	}
	if acquisition.Cleanup == nil {
		return Result{}, fmt.Errorf("acquirer returned no cleanup function")
	}
	defer func() {
		if cleanupErr := acquisition.Cleanup(); cleanupErr != nil {
			result = Result{}
			err = errors.Join(err, fmt.Errorf("clean up acquired Managed Pack release: %w", cleanupErr))
		}
	}()
	if strings.TrimSpace(acquisition.ProjectRoot) == "" {
		return Result{}, fmt.Errorf("acquirer returned no local Managed Pack Project root")
	}
	if acquisition.OriginRoots == nil {
		return Result{}, fmt.Errorf("acquirer returned no local origin roots map")
	}
	if reason := validateRelease(acquisition.Release, project, request.Coordinate); reason != "" {
		return rejected(GateRelease, reason), nil
	}
	acquisition.Release.Project = project

	preflight, err := module.validator.Validate(ctx, acquisition)
	if err != nil {
		return resultForError(err)
	}
	validation := preflight.Validation
	if validation.Manifest.ID != request.Coordinate.PackID {
		return rejected(GateValidation, fmt.Sprintf("manifest Pack ID %q does not match coordinate %q", validation.Manifest.ID, request.Coordinate.PackID)), nil
	}
	if validation.Manifest.Version != request.Coordinate.Version {
		return rejected(GateValidation, fmt.Sprintf("manifest version %q does not match coordinate %q", validation.Manifest.Version, request.Coordinate.Version)), nil
	}

	prepared, err := module.preparer.Prepare(ctx, request.RepositoryRoot, acquisition, validation)
	if err != nil {
		return resultForError(err)
	}
	if prepared.Cleanup == nil {
		return Result{}, fmt.Errorf("candidate preparer returned no cleanup function")
	}
	defer func() {
		if cleanupErr := prepared.Cleanup(); cleanupErr != nil {
			result = Result{}
			err = errors.Join(err, fmt.Errorf("clean up prepared Managed Pack candidate: %w", cleanupErr))
		}
	}()
	if prepared.Candidate == nil {
		if prepared.NoChangeReason == "" {
			return Result{}, fmt.Errorf("candidate preparer returned neither candidate nor no-change")
		}
		return Result{Status: StatusNoChange, Reason: prepared.NoChangeReason}, nil
	}
	if prepared.NoChangeReason != "" {
		return Result{}, fmt.Errorf("candidate preparer returned both candidate and no-change")
	}
	if strings.TrimSpace(prepared.Candidate.Summary) == "" {
		return Result{}, fmt.Errorf("candidate preparer returned no sealed proposal summary")
	}

	publication, err := module.publisher.Publish(ctx, *prepared.Candidate)
	if err != nil {
		return resultForError(err)
	}
	if publication.Proposal == nil {
		if publication.NoChangeReason == "" {
			return Result{}, fmt.Errorf("publisher returned neither proposal nor no-change")
		}
		return Result{Status: StatusNoChange, Reason: publication.NoChangeReason}, nil
	}
	if publication.NoChangeReason != "" {
		return Result{}, fmt.Errorf("publisher returned both proposal and no-change")
	}
	return Result{Status: StatusProposal, Proposal: publication.Proposal}, nil
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.RepositoryRoot) == "" {
		return fmt.Errorf("Packy repository root is required")
	}
	info, err := os.Stat(request.RepositoryRoot)
	if err != nil {
		return fmt.Errorf("inspect Packy repository root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("Packy repository root must be a directory")
	}
	coordinate, err := ParseCoordinate(request.Coordinate.String())
	if err != nil || coordinate != request.Coordinate {
		if err == nil {
			err = fmt.Errorf("coordinate is not canonical")
		}
		return fmt.Errorf("invalid Managed Pack coordinate: %w", err)
	}
	return nil
}

func resultForError(err error) (Result, error) {
	var rejection *RejectionError
	if errors.As(err, &rejection) {
		return rejected(rejection.Gate, rejection.Reason), nil
	}
	return Result{}, err
}

func rejected(gate Gate, reason string) Result {
	return Result{Status: StatusRejected, Rejection: &Rejection{Gate: gate, Reason: reason}}
}
