package packsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestIssueDeliveryLegalAdmissionAcceptsTheApprovedEvidence(t *testing.T) {
	expected := IssueDeliveryLegalAdmissionExpected()
	raw := readIssueDeliveryLegalAdmission(t, expected.EvidenceReference)

	admission, err := ValidateLegalAdmission(raw, expected)
	if err != nil {
		t.Fatal(err)
	}
	if admission.EvidenceID != "issue-deliver-pack-1.0.0-mit" || admission.SHA256 != expected.EvidenceSHA256 || admission.Disposition != RedistributableDisposition {
		t.Fatalf("admission = %+v", admission)
	}
	if !equalStrings(admission.Scope.SelectedRoots, []string{"LICENSE", "deliver-issue", "setup-issue-delivery"}) || !equalStrings(admission.Scope.Exclusions, []string{".github", ".gitignore", "AGENTS.md", "README.md", "scripts"}) {
		t.Fatalf("scope = %+v", admission.Scope)
	}
}

func TestIssueDeliveryLegalAdmissionRejectsInvalidatedEvidence(t *testing.T) {
	expected := IssueDeliveryLegalAdmissionExpected()
	raw := readIssueDeliveryLegalAdmission(t, expected.EvidenceReference)

	tests := []struct {
		name       string
		mutate     func(*legalAdmissionEvidence)
		unknown    bool
		updateHash bool
		want       error
	}{
		{name: "digest", mutate: func(evidence *legalAdmissionEvidence) { evidence.Issuer = "another issuer" }, want: ErrLegalAdmissionDigest},
		{name: "shape", unknown: true, updateHash: true, want: ErrLegalAdmissionShape},
		{name: "candidate", mutate: func(evidence *legalAdmissionEvidence) {
			evidence.Candidate.Commit = "1534b2af6c164d56bc8a95a81758749a721d29ae"
		}, updateHash: true, want: ErrLegalAdmissionBinding},
		{name: "selected scope", mutate: func(evidence *legalAdmissionEvidence) {
			evidence.Scope.SelectedRoots = []string{"LICENSE", "deliver-issue"}
		}, updateHash: true, want: ErrLegalAdmissionBinding},
		{name: "excluded scope", mutate: func(evidence *legalAdmissionEvidence) { evidence.Scope.Exclusions = []string{".github"} }, updateHash: true, want: ErrLegalAdmissionBinding},
		{name: "disposition", mutate: func(evidence *legalAdmissionEvidence) { evidence.Disposition = "review-required" }, updateHash: true, want: ErrLegalAdmissionDisposition},
		{name: "invalidation boundary", mutate: func(evidence *legalAdmissionEvidence) { evidence.Invalidation = "candidate change only" }, updateHash: true, want: ErrLegalAdmissionBinding},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := mutateIssueDeliveryEvidence(t, raw, test.mutate, test.unknown)
			mutatedExpected := expected
			if test.updateHash {
				mutatedExpected.EvidenceSHA256 = fmt.Sprintf("%x", sha256.Sum256(mutated))
			}
			if _, err := ValidateLegalAdmission(mutated, mutatedExpected); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func readIssueDeliveryLegalAdmission(t *testing.T, reference string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(reference)))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mutateIssueDeliveryEvidence(t *testing.T, raw []byte, mutate func(*legalAdmissionEvidence), unknown bool) []byte {
	t.Helper()
	if unknown {
		return bytes.Replace(raw, []byte("\n}\n"), []byte(",\n  \"unexpected\": true\n}\n"), 1)
	}
	var evidence legalAdmissionEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	mutate(&evidence)
	mutated, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(mutated, '\n')
}
