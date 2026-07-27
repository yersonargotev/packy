package deliveryevidence

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

const ValidationReceiptV1 = "packy.exhaustive-validation/v1"

// SandboxFacts records only the roots used by validation. It deliberately
// excludes environment values and all root contents.
type SandboxFacts struct {
	HomeRoot       string `json:"home_root"`
	ConfigHomeRoot string `json:"config_home_root"`
	Sandboxed      bool   `json:"sandboxed"`
}

// ValidationObservation is the complete current state to which exhaustive
// validation is sealed. CheckoutSHA256 distinguishes two checkouts of the same
// GitHub repository without retaining a local checkout path.
type ValidationObservation struct {
	Repository                 RepositoryIdentity `json:"repository"`
	CheckoutSHA256             string             `json:"checkout_sha256"`
	CommitSHA                  string             `json:"commit_sha"`
	TreeSHA                    string             `json:"tree_sha"`
	WorkspaceClean             bool               `json:"workspace_clean"`
	ValidatorIdentity          string             `json:"validator_identity"`
	ValidatorSHA256            string             `json:"validator_sha256"`
	ValidatorIdentityExpiresAt string             `json:"validator_identity_expires_at"`
	RequiredCommand            string             `json:"required_command"`
	Sandbox                    SandboxFacts       `json:"sandbox"`
}

// ValidationReceipt is authoritative only because its constructor requires a
// completed successful exhaustive run. Persisted values are validated again.
type ValidationReceipt struct {
	Schema ValidationReceiptSchema `json:"schema"`
	ValidationObservation
	CompletedAt string `json:"completed_at"`
	Succeeded   bool   `json:"succeeded"`
	Completed   bool   `json:"completed"`
}

type ValidationReceiptSchema string

// FocusedValidationEvidence is useful iteration evidence but is never accepted
// by ReusableExhaustiveValidation.
type FocusedValidationEvidence struct {
	Identity    string `json:"identity"`
	Command     string `json:"command"`
	CommitSHA   string `json:"commit_sha"`
	TreeSHA     string `json:"tree_sha"`
	CompletedAt string `json:"completed_at"`
	Succeeded   bool   `json:"succeeded"`
	Completed   bool   `json:"completed"`
}

type ExhaustiveValidationResult struct {
	Observation ValidationObservation
	CompletedAt string
	Succeeded   bool
	Completed   bool
}

// RecordExhaustiveValidation appends an authoritative receipt only after a
// successful, complete run. On every failure it returns the original bundle.
func RecordExhaustiveValidation(bundle Bundle, result ExhaustiveValidationResult) (Bundle, error) {
	if !result.Succeeded || !result.Completed {
		return bundle, errors.New("authoritative validation requires a successful completed exhaustive run")
	}
	receipt := ValidationReceipt{
		Schema:                ValidationReceiptV1,
		ValidationObservation: result.Observation,
		CompletedAt:           result.CompletedAt,
		Succeeded:             true,
		Completed:             true,
	}
	next := bundle
	next.ValidationReceipts = append(clone(bundle.ValidationReceipts), receipt)
	canonicalize(&next)
	if err := Validate(next); err != nil {
		return bundle, err
	}
	return next, nil
}

func RecordFocusedValidation(bundle Bundle, evidence FocusedValidationEvidence) (Bundle, error) {
	next := bundle
	next.FocusedValidation = append(clone(bundle.FocusedValidation), evidence)
	canonicalize(&next)
	if err := Validate(next); err != nil {
		return bundle, err
	}
	return next, nil
}

// ReusableExhaustiveValidation returns an exact receipt only when every sealed
// fact still equals the caller's current observation.
func ReusableExhaustiveValidation(bundle Bundle, current ValidationObservation, observedAt string) (ValidationReceipt, error) {
	if err := Validate(bundle); err != nil {
		return ValidationReceipt{}, err
	}
	if err := validateObservation(bundle.Repository, current, observedAt); err != nil {
		return ValidationReceipt{}, err
	}
	for i := len(bundle.ValidationReceipts) - 1; i >= 0; i-- {
		receipt := bundle.ValidationReceipts[i]
		if reflect.DeepEqual(receipt.ValidationObservation, current) {
			return receipt, nil
		}
	}
	return ValidationReceipt{}, errors.New("no exhaustive validation receipt matches the current observation")
}

func validateValidationEvidence(b Bundle) error {
	for _, receipt := range b.ValidationReceipts {
		if receipt.Schema != ValidationReceiptV1 {
			return fmt.Errorf("unsupported validation receipt schema %q", receipt.Schema)
		}
		if !receipt.Succeeded || !receipt.Completed {
			return errors.New("authoritative validation receipt must record completed success")
		}
		if err := validateObservation(b.Repository, receipt.ValidationObservation, receipt.CompletedAt); err != nil {
			return err
		}
	}
	for _, focused := range b.FocusedValidation {
		if err := safeText("focused validation identity", focused.Identity); err != nil {
			return err
		}
		if err := safeText("focused validation command", focused.Command); err != nil {
			return err
		}
		if !gitSHA(focused.CommitSHA) || !gitSHA(focused.TreeSHA) {
			return errors.New("focused validation requires commit and tree identities")
		}
		if _, err := parseTimestamp("focused validation completion", focused.CompletedAt); err != nil {
			return err
		}
		if !focused.Completed {
			return errors.New("focused validation evidence must be complete")
		}
	}
	return nil
}

func validateObservation(repository RepositoryIdentity, observation ValidationObservation, observedAt string) error {
	if observation.Repository != repository {
		return errors.New("validation observation belongs to a foreign repository")
	}
	if !digest(observation.CheckoutSHA256) || !gitSHA(observation.CommitSHA) || !gitSHA(observation.TreeSHA) {
		return errors.New("validation observation requires checkout, commit, and tree identities")
	}
	if !observation.WorkspaceClean {
		return errors.New("authoritative validation requires a clean workspace")
	}
	if err := safeText("validator identity", observation.ValidatorIdentity); err != nil {
		return err
	}
	if !digest(observation.ValidatorSHA256) {
		return errors.New("validator SHA-256 digest is required")
	}
	if err := safeText("required validation command", observation.RequiredCommand); err != nil {
		return err
	}
	if strings.TrimSpace(observation.RequiredCommand) != observation.RequiredCommand {
		return errors.New("required validation command must be normalized")
	}
	completed, err := parseTimestamp("validation observation time", observedAt)
	if err != nil {
		return err
	}
	expires, err := parseTimestamp("validator identity expiry", observation.ValidatorIdentityExpiresAt)
	if err != nil {
		return err
	}
	if !completed.Before(expires) {
		return errors.New("validator identity is expired")
	}
	if !observation.Sandbox.Sandboxed || !filepath.IsAbs(observation.Sandbox.HomeRoot) || !filepath.IsAbs(observation.Sandbox.ConfigHomeRoot) {
		return errors.New("validation requires explicit absolute sandbox HOME and configuration roots")
	}
	if err := safeText("sandbox HOME root", observation.Sandbox.HomeRoot); err != nil {
		return err
	}
	if err := safeText("sandbox configuration root", observation.Sandbox.ConfigHomeRoot); err != nil {
		return err
	}
	if filepath.Clean(observation.Sandbox.HomeRoot) != observation.Sandbox.HomeRoot ||
		filepath.Clean(observation.Sandbox.ConfigHomeRoot) != observation.Sandbox.ConfigHomeRoot {
		return errors.New("sandbox roots must be normalized")
	}
	return nil
}

func parseTimestamp(name, value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || t.Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("%s must be a canonical RFC3339 timestamp", name)
	}
	return t, nil
}
