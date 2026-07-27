package deliveryevidence

import (
	"reflect"
	"strings"
	"testing"
)

func validationObservation(b Bundle) ValidationObservation {
	return ValidationObservation{
		Repository: b.Repository, CheckoutSHA256: strings.Repeat("1", 64),
		CommitSHA: strings.Repeat("2", 40), TreeSHA: strings.Repeat("3", 40),
		WorkspaceClean: true, ValidatorIdentity: "scripts/validate-packy.sh",
		ValidatorSHA256:            strings.Repeat("4", 64),
		ValidatorIdentityExpiresAt: "2026-08-01T00:00:00Z",
		RequiredCommand:            "./scripts/validate-packy.sh",
		Sandbox:                    SandboxFacts{HomeRoot: "/tmp/packy-home", ConfigHomeRoot: "/tmp/packy-config", Sandboxed: true},
	}
}

func TestExhaustiveValidationExactReuseAndInvalidation(t *testing.T) {
	b := fixture()
	observed := validationObservation(b)
	var err error
	b, err = RecordExhaustiveValidation(b, ExhaustiveValidationResult{
		Observation: observed, CompletedAt: "2026-07-27T12:00:00Z", Succeeded: true, Completed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReusableExhaustiveValidation(b, observed, "2026-07-27T13:00:00Z"); err != nil {
		t.Fatalf("exact receipt was not reusable: %v", err)
	}
	tests := map[string]func(*ValidationObservation){
		"tree":       func(v *ValidationObservation) { v.TreeSHA = strings.Repeat("5", 40) },
		"dirty":      func(v *ValidationObservation) { v.WorkspaceClean = false },
		"validator":  func(v *ValidationObservation) { v.ValidatorSHA256 = strings.Repeat("5", 64) },
		"command":    func(v *ValidationObservation) { v.RequiredCommand += " --changed" },
		"repository": func(v *ValidationObservation) { v.Repository.NodeID = "foreign" },
		"checkout":   func(v *ValidationObservation) { v.CheckoutSHA256 = strings.Repeat("5", 64) },
		"sandbox":    func(v *ValidationObservation) { v.Sandbox.ConfigHomeRoot += "-other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			current := observed
			mutate(&current)
			if _, err := ReusableExhaustiveValidation(b, current, "2026-07-27T13:00:00Z"); err == nil {
				t.Fatal("changed observation reused")
			}
		})
	}
	if _, err := ReusableExhaustiveValidation(b, observed, observed.ValidatorIdentityExpiresAt); err == nil {
		t.Fatal("expired validator identity reused")
	}
}

func TestFailedIncompleteAndFocusedEvidenceAreNotAuthoritative(t *testing.T) {
	b := fixture()
	original := b
	for _, result := range []ExhaustiveValidationResult{
		{Observation: validationObservation(b), CompletedAt: "2026-07-27T12:00:00Z", Completed: true},
		{Observation: validationObservation(b), CompletedAt: "2026-07-27T12:00:00Z", Succeeded: true},
	} {
		got, err := RecordExhaustiveValidation(b, result)
		if err == nil || !reflect.DeepEqual(got, original) {
			t.Fatal("failed or incomplete validation changed authoritative evidence")
		}
	}
	var err error
	b, err = RecordFocusedValidation(b, FocusedValidationEvidence{
		Identity: "changed-impact", Command: "./scripts/validate-changed.sh",
		CommitSHA: strings.Repeat("2", 40), TreeSHA: strings.Repeat("3", 40),
		CompletedAt: "2026-07-27T12:00:00Z", Succeeded: true, Completed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReusableExhaustiveValidation(b, validationObservation(b), "2026-07-27T13:00:00Z"); err == nil {
		t.Fatal("focused-only evidence became authoritative")
	}
}

func TestValidationEvidenceCanonicalAndStrict(t *testing.T) {
	b := fixture()
	b.ValidationReceipts = nil
	b.FocusedValidation = nil
	if _, err := CanonicalJSON(b); err != nil {
		t.Fatalf("pre-validation bundle schema stopped being compatible: %v", err)
	}
	var err error
	b, err = RecordExhaustiveValidation(b, ExhaustiveValidationResult{Observation: validationObservation(b), CompletedAt: "2026-07-27T12:00:00Z", Succeeded: true, Completed: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := CanonicalJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	mixed := strings.Replace(string(data), `"succeeded":true`, `"focused":true,"succeeded":true`, 1)
	if _, err := Decode([]byte(mixed)); err == nil {
		t.Fatal("mixed or unknown validation evidence decoded")
	}
	b.ValidationReceipts[0].Completed = false
	if err := Validate(b); err == nil {
		t.Fatal("incomplete authoritative receipt validated")
	}
}
