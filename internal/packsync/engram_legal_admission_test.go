package packsync

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEngramLegalAdmissionAcceptsOnlyTheApprovedCandidateAndSelection(t *testing.T) {
	expected := EngramLegalAdmissionExpected()
	raw, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(expected.EvidenceReference)))
	if err != nil {
		t.Fatal(err)
	}
	admission, err := ValidateLegalAdmission(raw, expected)
	if err != nil {
		t.Fatal(err)
	}
	if admission.EvidenceID != "engram-2.0.0-mit" || admission.SHA256 != expected.EvidenceSHA256 || admission.Disposition != RedistributableDisposition ||
		!equalStrings(admission.Scope.SelectedRoots, []string{"LICENSE", "skills/engram-memory-cli"}) {
		t.Fatalf("admission = %+v", admission)
	}

	for _, test := range []struct {
		name   string
		mutate func(*legalAdmissionEvidence)
		want   error
	}{
		{name: "mutable or mismatched revision", mutate: func(evidence *legalAdmissionEvidence) {
			evidence.Candidate.Commit = "42bf708d740dedb8603a91e3c3104fb876c2090d"
		}, want: ErrLegalAdmissionBinding},
		{name: "incomplete selected tree", mutate: func(evidence *legalAdmissionEvidence) {
			evidence.Scope.SelectedRoots = []string{"LICENSE", "skills/engram-memory-cli/SKILL.md"}
		}, want: ErrLegalAdmissionBinding},
		{name: "sibling skill admission", mutate: func(evidence *legalAdmissionEvidence) {
			evidence.Scope.SelectedRoots = append(evidence.Scope.SelectedRoots, "skills/memory-protocol")
		}, want: ErrLegalAdmissionBinding},
		{name: "altered evidence", mutate: func(evidence *legalAdmissionEvidence) {
			evidence.Disclosures[0] = "selected content was patched"
		}, want: ErrLegalAdmissionDigest},
	} {
		t.Run(test.name, func(t *testing.T) {
			var evidence legalAdmissionEvidence
			if err := json.Unmarshal(raw, &evidence); err != nil {
				t.Fatal(err)
			}
			test.mutate(&evidence)
			mutated, err := json.MarshalIndent(evidence, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			mutated = append(mutated, '\n')
			mutatedExpected := expected
			if test.want != ErrLegalAdmissionDigest {
				mutatedExpected.EvidenceSHA256 = fmt.Sprintf("%x", sha256.Sum256(mutated))
			}
			if _, err := ValidateLegalAdmission(mutated, mutatedExpected); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
