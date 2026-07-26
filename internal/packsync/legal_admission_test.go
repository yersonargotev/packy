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
	cases := []struct {
		name     string
		expected LegalAdmissionExpected
	}{
		{"agent skills", VercelAgentSkillsLegalAdmissionExpected()},
		{"web interface guidelines", VercelWebInterfaceGuidelinesLegalAdmissionExpected()},
		{"writing guidelines", VercelWritingGuidelinesLegalAdmissionExpected()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			raw, err := os.ReadFile("../../" + test.expected.EvidenceReference)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ValidateLegalAdmission(raw, test.expected)
			if err != nil {
				t.Fatal(err)
			}
			if got.EvidenceID != test.expected.EvidenceID || got.SHA256 != test.expected.EvidenceSHA256 ||
				got.Disposition != RedistributableDisposition {
				t.Fatalf("admission = %#v", got)
			}

			t.Run("changed bytes", func(t *testing.T) {
				changed := append(append([]byte(nil), raw...), '\n')
				if _, err := ValidateLegalAdmission(changed, test.expected); !errors.Is(err, ErrLegalAdmissionDigest) {
					t.Fatalf("error = %v, want %v", err, ErrLegalAdmissionDigest)
				}
			})
			t.Run("changed candidate binding", func(t *testing.T) {
				changed := test.expected
				changed.Candidate.Commit = strings.Repeat("0", 40)
				if _, err := ValidateLegalAdmission(raw, changed); !errors.Is(err, ErrLegalAdmissionBinding) {
					t.Fatalf("error = %v, want %v", err, ErrLegalAdmissionBinding)
				}
			})
			t.Run("changed scope binding", func(t *testing.T) {
				changed := test.expected
				changed.Scope.SelectedRoots = append([]string(nil), test.expected.Scope.SelectedRoots...)
				changed.Scope.SelectedRoots[0] = "other"
				if _, err := ValidateLegalAdmission(raw, changed); !errors.Is(err, ErrLegalAdmissionBinding) {
					t.Fatalf("error = %v, want %v", err, ErrLegalAdmissionBinding)
				}
			})
		})
	}
}

func TestVercelGuidelineEvidenceIsConsumableByCompositeAdmission(t *testing.T) {
	cases := []struct {
		name         string
		expected     LegalAdmissionExpected
		registration SourceConfig
	}{
		{
			name:     "web interface guidelines",
			expected: VercelWebInterfaceGuidelinesLegalAdmissionExpected(),
			registration: SourceConfig{
				ID:         "vercel-web-interface-guidelines",
				Provider:   "github",
				Repository: "vercel-labs/web-interface-guidelines",
				Selector:   Selector{Mode: SelectorCommit, Ref: "4e799d45c17aec1498c269287a83b9dba22b966b"},
				Resources: []Binding{
					{PackID: "vercel", Kind: "asset", ResourceID: "web-interface-guidelines-rules", UpstreamPath: "command.md"},
					{PackID: "vercel", Kind: "notice", ResourceID: "web-interface-guidelines-mit", UpstreamPath: "LICENSE"},
				},
			},
		},
		{
			name:     "writing guidelines",
			expected: VercelWritingGuidelinesLegalAdmissionExpected(),
			registration: SourceConfig{
				ID:         "vercel-writing-guidelines",
				Provider:   "github",
				Repository: "vercel-labs/writing-guidelines",
				Selector:   Selector{Mode: SelectorCommit, Ref: "83e2316b034cf572400513538e4e4da01c4cc742"},
				Resources: []Binding{
					{PackID: "vercel", Kind: "asset", ResourceID: "writing-guidelines-rules", UpstreamPath: "command.md"},
					{PackID: "vercel", Kind: "notice", ResourceID: "writing-guidelines-mit", UpstreamPath: "LICENSE"},
				},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			member := CompositeRegistrationMember{
				Registration: test.registration,
				LegalAdmission: CompositeLegalAdmission{
					EvidenceReference: test.expected.EvidenceReference,
					EvidenceSHA256:    test.expected.EvidenceSHA256,
					Disposition:       RedistributableDisposition,
				},
			}
			if err := validateCompositeLegalAdmission("../..", member, Candidate{Commit: test.expected.Candidate.Commit}); err != nil {
				t.Fatal(err)
			}

			member.LegalAdmission.EvidenceSHA256 = strings.Repeat("0", 64)
			if err := validateCompositeLegalAdmission("../..", member, Candidate{Commit: test.expected.Candidate.Commit}); !errors.Is(err, ErrLegalAdmissionDigest) {
				t.Fatalf("error = %v, want %v", err, ErrLegalAdmissionDigest)
			}
		})
	}
}

func TestVercelLegalAdmissionExpectedReturnsFreshScopeSlices(t *testing.T) {
	first := VercelAgentSkillsLegalAdmissionExpected()
	first.Scope.SelectedRoots[0] = "mutated"
	first.Scope.Exclusions[0] = "mutated"

	second := VercelAgentSkillsLegalAdmissionExpected()
	if second.Scope.SelectedRoots[0] != "skills/composition-patterns" ||
		second.Scope.Exclusions[0] != "react-best-practices.zip" {
		t.Fatalf("production admission anchor was mutable: %#v", second.Scope)
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
	tests := map[string]struct {
		mutate func(*legalAdmissionEvidence, *LegalAdmissionExpected)
		want   error
	}{
		"candidate": {mutate: func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Candidate.Commit = strings.Repeat("1", 40)
		}, want: ErrLegalAdmissionBinding},
		"readme blob": {mutate: func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Candidate.READMEBlob = strings.Repeat("2", 40)
		}, want: ErrLegalAdmissionBinding},
		"length": {mutate: func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) { e.Candidate.READMELength++ }, want: ErrLegalAdmissionBinding},
		"digest": {mutate: func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Candidate.READMESHA256 = strings.Repeat("3", 64)
		}, want: ErrLegalAdmissionBinding},
		"scope": {mutate: func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Scope.SelectedRoots[0] = "skills/other"
		}, want: ErrLegalAdmissionBinding},
		"disposition": {mutate: func(e *legalAdmissionEvidence, _ *LegalAdmissionExpected) {
			e.Disposition = "blocked"
		}, want: ErrLegalAdmissionDisposition},
		"evidence digest": {mutate: func(_ *legalAdmissionEvidence, x *LegalAdmissionExpected) {
			x.EvidenceSHA256 = strings.Repeat("4", 64)
		}, want: ErrLegalAdmissionDigest},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			e := base
			e.Scope.SelectedRoots = append([]string(nil), base.Scope.SelectedRoots...)
			e.Scope.Exclusions = append([]string(nil), base.Scope.Exclusions...)
			x := baseExpected
			test.mutate(&e, &x)
			raw := baseRaw
			if name != "evidence digest" {
				raw, _ = legalAdmissionBytes(t, e)
				x.EvidenceSHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
			}
			_, err := ValidateLegalAdmission(raw, x)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
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
