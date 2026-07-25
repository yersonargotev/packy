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

	VercelAgentSkillsLegalAdmissionEvidenceReference = "docs/research/evidence/vercel-agent-skills-legal-admission.json"
	VercelAgentSkillsLegalAdmissionEvidenceSHA256    = "e98ea93b2fc7ee5e4b49364ab0fc4e13fe4b0801d6439bd7e07180a7751e6dc3"
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
}

type LegalAdmission struct {
	EvidenceID  string
	SHA256      string
	Disposition string
	Scope       LegalAdmissionScope
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
	if evidence.EvidenceID != expected.EvidenceID || evidence.DurableReference != expected.EvidenceReference || evidence.Candidate != expected.Candidate || !equalStrings(evidence.Scope.SelectedRoots, expected.Scope.SelectedRoots) || !equalStrings(evidence.Scope.Exclusions, expected.Scope.Exclusions) {
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
