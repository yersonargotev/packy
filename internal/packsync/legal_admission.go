package packsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	RedistributableDisposition = "redistributable"

	IssueDeliveryLegalAdmissionEvidenceReference = "docs/research/evidence/issue-deliver-pack-1.0.0-legal-admission.json"
	IssueDeliveryLegalAdmissionEvidenceSHA256    = "5ee40cad82c6d7cee0983b1b2fd3b69754ff7fd31726b5c944de0c04e9bd7194"

	VercelAgentSkillsLegalAdmissionEvidenceReference = "docs/research/evidence/vercel-agent-skills-legal-admission.json"
	VercelAgentSkillsLegalAdmissionEvidenceSHA256    = "e98ea93b2fc7ee5e4b49364ab0fc4e13fe4b0801d6439bd7e07180a7751e6dc3"

	VercelWebInterfaceGuidelinesLegalAdmissionEvidenceReference = "docs/research/evidence/vercel-web-interface-guidelines-legal-admission.json"
	VercelWebInterfaceGuidelinesLegalAdmissionEvidenceSHA256    = "f53f20a752db7bcb91f3ed1044fe1c4a49603599d9c25936994761994fcc8cc4"

	VercelWritingGuidelinesLegalAdmissionEvidenceReference = "docs/research/evidence/vercel-writing-guidelines-legal-admission.json"
	VercelWritingGuidelinesLegalAdmissionEvidenceSHA256    = "0e6e060ab7a7b4980d671de99a2516f713ea3584be175af3bb28e9773aeb9966"
)

var (
	ErrLegalAdmissionShape       = errors.New("legal admission evidence has invalid shape")
	ErrLegalAdmissionBinding     = errors.New("legal admission evidence does not match expected binding")
	ErrLegalAdmissionDisposition = errors.New("legal admission evidence is not redistributable")
	ErrLegalAdmissionDigest      = errors.New("legal admission evidence digest does not match")
)

type LegalAdmissionScope struct {
	SelectedRoots []string `json:"selected_roots"`
	Exclusions    []string `json:"exclusions"`
}

type LegalAdmissionCandidate struct {
	Repository   string `json:"repository"`
	Commit       string `json:"commit"`
	READMEBlob   string `json:"readme_blob"`
	READMELength int    `json:"readme_length"`
	READMESHA256 string `json:"readme_sha256"`
}

type LegalAdmissionExpected struct {
	EvidenceReference string
	EvidenceSHA256    string
	EvidenceID        string
	Candidate         LegalAdmissionCandidate
	Scope             LegalAdmissionScope
	Invalidation      string
}

type LegalAdmission struct {
	EvidenceID  string
	SHA256      string
	Disposition string
	Scope       LegalAdmissionScope
}

// IssueDeliveryLegalAdmissionExpected returns the immutable issue-654
// admission anchor. Each call owns its scope slices so callers cannot mutate
// the production binding observed by later callers.
func IssueDeliveryLegalAdmissionExpected() LegalAdmissionExpected {
	return LegalAdmissionExpected{
		EvidenceReference: IssueDeliveryLegalAdmissionEvidenceReference,
		EvidenceSHA256:    IssueDeliveryLegalAdmissionEvidenceSHA256,
		EvidenceID:        "issue-deliver-pack-1.0.0-mit",
		Candidate: LegalAdmissionCandidate{
			Repository:   "yersonargotev/issue-deliver-pack",
			Commit:       "0534b2af6c164d56bc8a95a81758749a721d29ae",
			READMEBlob:   "d0594e2c663e5352cc219ec28e474592e2d5c9f3",
			READMELength: 1421,
			READMESHA256: "b86b7fa0a968fe6fec6bf87b802904c38470983aafd8c765e76aa782131a2208",
		},
		Scope: LegalAdmissionScope{
			SelectedRoots: []string{"LICENSE", "deliver-issue", "setup-issue-delivery"},
			Exclusions:    []string{".github", ".gitignore", "AGENTS.md", "README.md", "scripts"},
		},
		Invalidation: "any release, candidate, selected-root, license, README identity, disposition, evidence digest, or scope change",
	}
}

// VercelAgentSkillsLegalAdmissionExpected returns the immutable issue-254
// admission anchor. Each call owns its scope slices so callers cannot mutate
// the production binding observed by later callers.
func VercelAgentSkillsLegalAdmissionExpected() LegalAdmissionExpected {
	return LegalAdmissionExpected{
		EvidenceReference: VercelAgentSkillsLegalAdmissionEvidenceReference,
		EvidenceSHA256:    VercelAgentSkillsLegalAdmissionEvidenceSHA256,
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
}

// VercelWebInterfaceGuidelinesLegalAdmissionExpected returns the immutable
// secondary-source admission anchor recorded by the accepted Vercel evidence.
func VercelWebInterfaceGuidelinesLegalAdmissionExpected() LegalAdmissionExpected {
	return LegalAdmissionExpected{
		EvidenceReference: VercelWebInterfaceGuidelinesLegalAdmissionEvidenceReference,
		EvidenceSHA256:    VercelWebInterfaceGuidelinesLegalAdmissionEvidenceSHA256,
		EvidenceID:        "vercel-web-interface-guidelines-4e799d4-license-mit",
		Candidate: LegalAdmissionCandidate{
			Repository:   "vercel-labs/web-interface-guidelines",
			Commit:       "4e799d45c17aec1498c269287a83b9dba22b966b",
			READMEBlob:   "b3575a3c1358eac4b9ee36a4c851872d81417760",
			READMELength: 1068,
			READMESHA256: "6cd1609c9c12233507cdd2ce0d32e9a721e3c27494951be06b90090deeeb7af2",
		},
		Scope: LegalAdmissionScope{
			SelectedRoots: []string{"LICENSE", "command.md"},
			Exclusions:    []string{},
		},
	}
}

// VercelWritingGuidelinesLegalAdmissionExpected returns the immutable
// secondary-source admission anchor recorded by the accepted Vercel evidence.
func VercelWritingGuidelinesLegalAdmissionExpected() LegalAdmissionExpected {
	return LegalAdmissionExpected{
		EvidenceReference: VercelWritingGuidelinesLegalAdmissionEvidenceReference,
		EvidenceSHA256:    VercelWritingGuidelinesLegalAdmissionEvidenceSHA256,
		EvidenceID:        "vercel-writing-guidelines-83e2316-license-mit",
		Candidate: LegalAdmissionCandidate{
			Repository:   "vercel-labs/writing-guidelines",
			Commit:       "83e2316b034cf572400513538e4e4da01c4cc742",
			READMEBlob:   "094e15e1beb5b639309cc5a920e9b85d2be725ce",
			READMELength: 1068,
			READMESHA256: "7ecf613390251c6a08d66982519db39f2ae7fc2e474c65630adea78e84dc4445",
		},
		Scope: LegalAdmissionScope{
			SelectedRoots: []string{"LICENSE", "command.md"},
			Exclusions:    []string{},
		},
	}
}

type legalAdmissionEvidence struct {
	SchemaVersion    int                     `json:"schema_version"`
	EvidenceID       string                  `json:"evidence_id"`
	DurableReference string                  `json:"durable_reference"`
	Issuer           string                  `json:"issuer"`
	EvidenceOrigin   string                  `json:"evidence_origin"`
	Decision         string                  `json:"decision"`
	Candidate        LegalAdmissionCandidate `json:"candidate"`
	Disposition      string                  `json:"disposition"`
	Rights           []string                `json:"rights"`
	Obligations      []string                `json:"obligations"`
	Disclosures      []string                `json:"disclosures"`
	Scope            LegalAdmissionScope     `json:"scope"`
	Validity         string                  `json:"validity"`
	Invalidation     string                  `json:"invalidation"`
}

func ValidateLegalAdmission(raw []byte, expected LegalAdmissionExpected) (LegalAdmission, error) {
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	if digest != expected.EvidenceSHA256 {
		return LegalAdmission{}, ErrLegalAdmissionDigest
	}
	var evidence legalAdmissionEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil || ensureEOF(decoder) != nil {
		return LegalAdmission{}, ErrLegalAdmissionShape
	}
	canonical, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil || !bytes.Equal(raw, append(canonical, '\n')) {
		return LegalAdmission{}, ErrLegalAdmissionShape
	}
	if evidence.SchemaVersion != 1 || evidence.EvidenceID == "" || evidence.Issuer == "" || evidence.EvidenceOrigin == "" || evidence.Decision == "" || evidence.Validity == "" || evidence.Invalidation == "" || len(evidence.Rights) == 0 || len(evidence.Obligations) == 0 || len(evidence.Disclosures) == 0 || len(evidence.Scope.SelectedRoots) == 0 || evidence.Candidate.READMELength <= 0 {
		return LegalAdmission{}, ErrLegalAdmissionShape
	}
	if evidence.EvidenceID != expected.EvidenceID || evidence.DurableReference != expected.EvidenceReference || evidence.Candidate != expected.Candidate || !equalStrings(evidence.Scope.SelectedRoots, expected.Scope.SelectedRoots) || !equalStrings(evidence.Scope.Exclusions, expected.Scope.Exclusions) || expected.Invalidation != "" && evidence.Invalidation != expected.Invalidation {
		return LegalAdmission{}, ErrLegalAdmissionBinding
	}
	if evidence.Disposition != RedistributableDisposition {
		return LegalAdmission{}, ErrLegalAdmissionDisposition
	}
	return LegalAdmission{EvidenceID: evidence.EvidenceID, SHA256: digest, Disposition: evidence.Disposition, Scope: evidence.Scope}, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
