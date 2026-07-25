package packsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

const RedistributableDisposition = "redistributable"

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
