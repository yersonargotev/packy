package packsync

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestVercelLegalAdmissionEvidence(t *testing.T) {
	raw, err := os.ReadFile("../../docs/research/evidence/vercel-agent-skills-legal-admission.json")
	if err != nil {
		t.Fatal(err)
	}
	expected := LegalAdmissionExpected{
		EvidenceReference: "docs/research/evidence/vercel-agent-skills-legal-admission.json",
		EvidenceSHA256:    "e98ea93b2fc7ee5e4b49364ab0fc4e13fe4b0801d6439bd7e07180a7751e6dc3",
		EvidenceID:        "vercel-agent-skills-7c180d9-readme-mit",
		Candidate: LegalAdmissionCandidate{
			Repository:   "vercel-labs/agent-skills",
			Commit:       "7c180d9044c9ae2b442b567aad4e42a28dd5ed62",
			READMEBlob:   "daecfea1e60f8f045a3d711c605d70edcdf9d92a",
			READMELength: 7538,
			READMESHA256: "c0a05286fc2a9d52ec2480bf070867665b9357beef37c9c2812e5b2ece571b6a",
		},
		Scope: LegalAdmissionScope{
			SelectedRoots: []string{
				"skills/composition-patterns",
				"skills/deploy-to-vercel",
				"skills/react-best-practices",
				"skills/react-native-skills",
				"skills/react-view-transitions",
				"skills/vercel-cli-with-tokens",
				"skills/vercel-optimize",
				"skills/web-design-guidelines",
				"skills/writing-guidelines",
			},
			Exclusions: []string{
				"react-best-practices.zip",
				"react-native-skills.zip",
				"vercel-composition-patterns.zip",
				"vercel-deploy-claimable.zip",
				"vercel-react-best-practices.zip",
				"skills/deploy-to-vercel/Archive.zip",
			},
		},
	}
	got, err := ValidateLegalAdmission(raw, expected)
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceID != expected.EvidenceID || got.SHA256 != expected.EvidenceSHA256 ||
		got.Disposition != RedistributableDisposition {
		t.Fatalf("admission = %#v", got)
	}
}

func TestValidateLegalAdmissionBindsExactRedistributableEvidence(t *testing.T) {
	evidence := legalAdmissionFixture()
	raw, expected := legalAdmissionBytes(t, evidence)
	got, err := ValidateLegalAdmission(raw, expected)
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceID != evidence.EvidenceID || got.SHA256 != expected.EvidenceSHA256 || got.Disposition != RedistributableDisposition || len(got.Scope.SelectedRoots) != 9 {
		t.Fatalf("admission = %#v", got)
	}
}

func TestValidateLegalAdmissionRejectsOneFactNegativeTwins(t *testing.T) {
	base := legalAdmissionFixture()
	baseRaw, baseExpected := legalAdmissionBytes(t, base)
	tests := map[string]func(*legalAdmissionEvidence, *LegalAdmissionExpected){
		"candidate": func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Candidate.Commit = strings.Repeat("1", 40)
		},
		"readme blob": func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Candidate.READMEBlob = strings.Repeat("2", 40)
		},
		"length": func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) { e.Candidate.READMELength++ },
		"digest": func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Candidate.READMESHA256 = strings.Repeat("3", 64)
		},
		"scope":           func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) { e.Scope.SelectedRoots[0] = "skills/other" },
		"disposition":     func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) { e.Disposition = "blocked" },
		"evidence digest": func(_ *legalAdmissionEvidence, x *LegalAdmissionExpected) { x.EvidenceSHA256 = strings.Repeat("4", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			e := base
			e.Scope.SelectedRoots = append([]string(nil), base.Scope.SelectedRoots...)
			e.Scope.Exclusions = append([]string(nil), base.Scope.Exclusions...)
			x := baseExpected
			mutate(&e, &x)
			raw := baseRaw
			if name != "evidence digest" {
				raw, _ = legalAdmissionBytes(t, e)
			}
			_, err := ValidateLegalAdmission(raw, x)
			if err == nil {
				t.Fatal("negative twin accepted")
			}
		})
	}
}

func TestValidateLegalAdmissionRejectsUnknownFieldsAndSanitizesDiagnostics(t *testing.T) {
	e := legalAdmissionFixture()
	raw, expected := legalAdmissionBytes(t, e)
	raw = []byte(strings.Replace(string(raw), "{\n", "{\n  \"secret_fact\": \"DO-NOT-ECHO\",\n", 1))
	expected.EvidenceSHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
	_, err := ValidateLegalAdmission(raw, expected)
	if !errors.Is(err, ErrLegalAdmissionShape) || strings.Contains(err.Error(), "DO-NOT-ECHO") || strings.Contains(err.Error(), e.Candidate.Commit) {
		t.Fatalf("unsafe error = %v", err)
	}
}

func legalAdmissionBytes(t *testing.T, evidence legalAdmissionEvidence) ([]byte, LegalAdmissionExpected) {
	t.Helper()
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	return raw, LegalAdmissionExpected{EvidenceReference: evidence.DurableReference, EvidenceSHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), EvidenceID: evidence.EvidenceID, Candidate: evidence.Candidate, Scope: evidence.Scope}
}

func legalAdmissionFixture() legalAdmissionEvidence {
	return legalAdmissionEvidence{SchemaVersion: 1, EvidenceID: "vercel-agent-skills-7c180d9-readme-mit", DurableReference: "docs/research/evidence/vercel-agent-skills-legal-admission.json", Issuer: "vercel-labs/agent-skills root README", EvidenceOrigin: "README.md at pinned Git blob", Decision: "Packy maintainer issue 254", Candidate: LegalAdmissionCandidate{Repository: "vercel-labs/agent-skills", Commit: "7c180d9044c9ae2b442b567aad4e42a28dd5ed62", READMEBlob: "daecfea1e60f8f045a3d711c605d70edcdf9d92a", READMELength: 7538, READMESHA256: "c0a05286fc2a9d52ec2480bf070867665b9357beef37c9c2812e5b2ece571b6a"}, Disposition: RedistributableDisposition, Rights: []string{"adaptation", "publication", "redistribution"}, Obligations: []string{"retain supplied MIT copyright and permission notice"}, Disclosures: []string{"no standalone license text supplied", "no copyright notice or holder text supplied; Packy does not fabricate either"}, Scope: LegalAdmissionScope{SelectedRoots: []string{"skills/composition-patterns", "skills/deploy-to-vercel", "skills/react-best-practices", "skills/react-native-skills", "skills/react-view-transitions", "skills/vercel-cli-with-tokens", "skills/vercel-optimize", "skills/web-design-guidelines", "skills/writing-guidelines"}, Exclusions: []string{"react-best-practices.zip", "react-native-skills.zip", "vercel-composition-patterns.zip", "vercel-deploy-claimable.zip", "vercel-react-best-practices.zip", "skills/deploy-to-vercel/Archive.zip"}}, Validity: "valid only for the exact candidate and selected scope", Invalidation: "any candidate, README identity or bytes, selected root, exclusion, disposition, rights, obligation, disclosure, issuer, origin, decision, or evidence byte change requires fresh maintainer admission"}
}
