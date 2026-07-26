package packsyncworkflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/yersonargotev/packy/internal/packsync"
)

// OperationRegisterBundle is the only operation in the private v3 workflow
// suite. It intentionally has a distinct request and artifact family from the
// v1/v2 single-source contracts.
const OperationRegisterBundle DispatchOperation = "register_bundle"

type BundleRegistration = packsync.CompositeRegistrationMember

type BundleDispatchRequest struct {
	SchemaVersion            int                  `json:"schema_version"`
	Operation                DispatchOperation    `json:"operation"`
	PackID                   string               `json:"pack_id"`
	ProposedVersion          string               `json:"proposed_version"`
	ProposedManifest         json.RawMessage      `json:"proposed_manifest"`
	ProposedManifestSHA256   string               `json:"proposed_manifest_sha256"`
	Registrations            []BundleRegistration `json:"registrations"`
	RegistrationBundleSHA256 string               `json:"registration_bundle_sha256"`
	ClassificationMode       ClassificationMode   `json:"classification_mode"`
	RequestReason            string               `json:"request_reason"`
	ExpectedPlanID           string               `json:"expected_plan_id,omitempty"`
	ExpectedBaseSHA          string               `json:"expected_base_sha,omitempty"`
	HumanEvidence            json.RawMessage      `json:"human_evidence,omitempty"`
}

func CanonicalRegistrationBundleSHA256(packID string, registrations []BundleRegistration) (string, error) {
	_, digest, err := normalizeBundleRegistrations(packID, registrations)
	return digest, err
}

func normalizeBundleRegistrations(packID string, registrations []BundleRegistration) ([]BundleRegistration, string, error) {
	if !ValidSourceID(packID) || len(registrations) < 2 {
		return nil, "", errors.New("register_bundle requires one safe pack id and at least two registrations")
	}
	result, digest, err := packsync.CanonicalRegistrationBundle(registrations)
	if err != nil {
		return nil, "", err
	}
	for i, member := range result {
		if registrations[i].Registration.ID != member.Registration.ID {
			return nil, "", errors.New("bundle registrations must be uniquely ordered by source_id")
		}
		for _, binding := range member.Registration.Resources {
			if binding.PackID != packID {
				return nil, "", errors.New("bundle registration contains a binding outside the declared pack")
			}
		}
	}
	return result, digest, nil
}

func (request BundleDispatchRequest) Validate() error {
	if request.SchemaVersion != 3 || request.Operation != OperationRegisterBundle {
		return errors.New("v3 dispatch requires register_bundle")
	}
	if request.ClassificationMode != ClassificationAI && request.ClassificationMode != ClassificationHuman {
		return errors.New("classification mode must be explicitly ai or human")
	}
	if request.RequestReason == "" {
		return errors.New("request reason is required")
	}
	var manifest struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if len(request.ProposedManifest) == 0 || json.Unmarshal(request.ProposedManifest, &manifest) != nil ||
		manifest.ID != request.PackID || manifest.Version != request.ProposedVersion {
		return errors.New("proposed manifest must identify the declared Pack generation")
	}
	canonicalManifest, err := packsync.CanonicalCompositePackManifest(request.ProposedManifest)
	if err != nil || !bytes.Equal(canonicalManifest, request.ProposedManifest) {
		return errors.New("proposed manifest must use canonical composite Pack bytes")
	}
	manifestSum := sha256.Sum256(canonicalManifest)
	if fmt.Sprintf("%x", manifestSum) != request.ProposedManifestSHA256 {
		return errors.New("proposed manifest SHA-256 does not match its exact bytes")
	}
	hasHumanBinding := request.ExpectedPlanID != "" || request.ExpectedBaseSHA != "" || len(request.HumanEvidence) != 0
	if request.ClassificationMode == ClassificationAI && hasHumanBinding {
		return errors.New("AI dispatch contradicts human evidence binding")
	}
	if hasHumanBinding {
		var evidence struct {
			SchemaVersion int             `json:"schema_version"`
			PlanID        string          `json:"plan_id"`
			PackID        string          `json:"pack_id"`
			Evidence      json.RawMessage `json:"evidence"`
		}
		if request.ClassificationMode != ClassificationHuman || request.ExpectedPlanID == "" ||
			requireFullSHA("expected base", request.ExpectedBaseSHA) != nil ||
			!json.Valid(request.HumanEvidence) || json.Unmarshal(request.HumanEvidence, &evidence) != nil ||
			evidence.SchemaVersion != 1 || evidence.PlanID != request.ExpectedPlanID ||
			evidence.PackID != request.PackID || len(evidence.Evidence) == 0 {
			return errors.New("human evidence dispatch requires exact composite plan, base, and one-Pack evidence")
		}
	}
	digest, err := CanonicalRegistrationBundleSHA256(request.PackID, request.Registrations)
	if err != nil || digest != request.RegistrationBundleSHA256 {
		return errors.New("registration bundle SHA-256 does not match the canonical registrations")
	}
	return nil
}

func (request *BundleDispatchRequest) UnmarshalJSON(data []byte) error {
	type plain BundleDispatchRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded plain
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*request = BundleDispatchRequest(decoded)
	return nil
}

type BundleArtifactMember struct {
	SourceID            string `json:"source_id"`
	CandidateSHA        string `json:"candidate_sha"`
	SourceLockSHA256    string `json:"source_lock_sha256"`
	LegalEvidenceRef    string `json:"legal_evidence_ref"`
	LegalEvidenceSHA256 string `json:"legal_evidence_sha256"`
}

// BundleArtifactIdentity is the common, complete seal carried by every v3
// artifact. SourceIDs and Members use the same canonical source_id order.
type BundleArtifactIdentity struct {
	PackID                   string                 `json:"pack_id"`
	SourceIDs                []string               `json:"source_ids"`
	RegistrationBundleSHA256 string                 `json:"registration_bundle_sha256"`
	ProposedVersion          string                 `json:"proposed_version"`
	ProposedManifestSHA256   string                 `json:"proposed_manifest_sha256"`
	Members                  []BundleArtifactMember `json:"members"`
	PlanID                   string                 `json:"plan_id"`
	BaseSHA                  string                 `json:"base_sha"`
	ConfigSHA256             string                 `json:"config_sha256"`
	ManifestsSHA256          string                 `json:"manifests_sha256"`
	LockSetSHA256            string                 `json:"lock_set_sha256"`
	ResultBundleSHA256       string                 `json:"result_bundle_sha256"`
	ContainsSecrets          bool                   `json:"contains_secrets"`
	ContainsUpstreamBytes    bool                   `json:"contains_upstream_bytes"`
}

func (identity BundleArtifactIdentity) Validate() error {
	if !ValidSourceID(identity.PackID) || len(identity.SourceIDs) < 2 || len(identity.SourceIDs) != len(identity.Members) ||
		identity.PlanID == "" || requireFullSHA("base", identity.BaseSHA) != nil ||
		requireSHA256("registration bundle", identity.RegistrationBundleSHA256) != nil ||
		identity.ProposedVersion == "" || requireSHA256("proposed manifest", identity.ProposedManifestSHA256) != nil ||
		requireSHA256("configuration", identity.ConfigSHA256) != nil ||
		requireSHA256("manifests", identity.ManifestsSHA256) != nil ||
		requireSHA256("lock set", identity.LockSetSHA256) != nil ||
		requireSHA256("result bundle", identity.ResultBundleSHA256) != nil ||
		identity.ContainsSecrets || identity.ContainsUpstreamBytes {
		return errors.New("v3 bundle artifact identity is incomplete or contradictory")
	}
	previous := ""
	for i, member := range identity.Members {
		if identity.SourceIDs[i] != member.SourceID || member.SourceID <= previous ||
			requireFullSHA("candidate", member.CandidateSHA) != nil ||
			requireSHA256("source lock", member.SourceLockSHA256) != nil ||
			member.LegalEvidenceRef == "" ||
			requireSHA256("legal evidence", member.LegalEvidenceSHA256) != nil {
			return errors.New("v3 bundle artifact members are incomplete, mixed, or out of order")
		}
		previous = member.SourceID
	}
	return nil
}

// Matches rejects a later phase that carries valid-looking but stale or mixed
// members. Adapters call it against the identity emitted by the immediately
// preceding admitted phase; workflow evidence is never patched forward.
func (identity BundleArtifactIdentity) Matches(expected BundleArtifactIdentity) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("expected v3 bundle identity is invalid: %w", err)
	}
	left, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	right, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	if !bytes.Equal(left, right) {
		return errors.New("v3 bundle artifact identity is stale or mixed")
	}
	return nil
}

type BundleInspectionArtifact struct {
	SchemaVersion int `json:"schema_version"`
	BundleArtifactIdentity
}

type BundleClassificationArtifact struct {
	SchemaVersion int `json:"schema_version"`
	BundleArtifactIdentity
	ClassificationSHA256 string `json:"classification_sha256"`
}

type BundleValidationArtifact struct {
	SchemaVersion int `json:"schema_version"`
	BundleArtifactIdentity
	ResultTreeSHA string          `json:"result_tree_sha"`
	Validation    ValidationGates `json:"validation"`
}

type BundleFailureMember struct {
	SourceID            string `json:"source_id"`
	CandidateSHA        string `json:"candidate_sha"`
	LegalEvidenceRef    string `json:"legal_evidence_ref"`
	LegalEvidenceSHA256 string `json:"legal_evidence_sha256"`
}

type BundleFailurePlanIdentity struct {
	PlanID             string                 `json:"plan_id"`
	BaseSHA            string                 `json:"base_sha"`
	ConfigSHA256       string                 `json:"config_sha256"`
	ManifestsSHA256    string                 `json:"manifests_sha256"`
	LockSetSHA256      string                 `json:"lock_set_sha256"`
	ResultBundleSHA256 string                 `json:"result_bundle_sha256"`
	Members            []BundleArtifactMember `json:"members"`
}

// BundleFailureIdentity carries the dispatch seal even when Check fails before
// a plan exists. Plan is either absent or one complete sealed result group.
type BundleFailureIdentity struct {
	PackID                   string                     `json:"pack_id"`
	SourceIDs                []string                   `json:"source_ids"`
	RegistrationBundleSHA256 string                     `json:"registration_bundle_sha256"`
	ProposedVersion          string                     `json:"proposed_version"`
	ProposedManifestSHA256   string                     `json:"proposed_manifest_sha256"`
	Members                  []BundleFailureMember      `json:"members"`
	Plan                     *BundleFailurePlanIdentity `json:"plan,omitempty"`
	ContainsSecrets          bool                       `json:"contains_secrets"`
	ContainsUpstreamBytes    bool                       `json:"contains_upstream_bytes"`
}

type BundleFailureArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	BundleFailureIdentity
	Blockers []string `json:"blockers"`
	Recovery []string `json:"recovery"`
}

type BundlePublicationArtifact struct {
	SchemaVersion int `json:"schema_version"`
	BundleArtifactIdentity
	HeadSHA                 string          `json:"head_sha"`
	ResultTreeSHA           string          `json:"result_tree_sha"`
	BranchName              string          `json:"branch_name"`
	PRNumber                int             `json:"pr_number"`
	PRStateSHA256           string          `json:"pr_state_sha256"`
	ProvenanceSHA256        string          `json:"provenance_sha256"`
	ManagedTitle            string          `json:"managed_title"`
	ManagedMetadataHash     string          `json:"managed_metadata_hash"`
	Validation              ValidationGates `json:"validation"`
	DecisionReady           bool            `json:"decision_ready"`
	AutoMerge               bool            `json:"auto_merge"`
	ManualMergeRequired     bool            `json:"manual_merge_required"`
	UpstreamContentExecuted bool            `json:"upstream_content_executed"`
	InvalidationConditions  []string        `json:"invalidation_conditions"`
}

// BundlePreparationArtifact proves the complete read-only prefix of v3
// publication without carrying pull-request or decision-readiness authority.
type BundlePreparationArtifact struct {
	SchemaVersion int `json:"schema_version"`
	BundleArtifactIdentity
	HeadSHA                 string          `json:"head_sha"`
	ResultTreeSHA           string          `json:"result_tree_sha"`
	BranchName              string          `json:"branch_name"`
	ProvenanceSHA256        string          `json:"provenance_sha256"`
	ManagedTitle            string          `json:"managed_title"`
	ManagedMetadataHash     string          `json:"managed_metadata_hash"`
	ObservedBaseSHA         string          `json:"observed_base_sha"`
	Validation              ValidationGates `json:"validation"`
	ObservationsStable      bool            `json:"observations_stable"`
	RepositoryMutated       bool            `json:"repository_mutated"`
	DecisionReady           bool            `json:"decision_ready"`
	UpstreamContentExecuted bool            `json:"upstream_content_executed"`
}

func validateV3(schema int, identity BundleArtifactIdentity) error {
	if schema != 3 {
		return errors.New("bundle artifact requires schema version 3")
	}
	return identity.Validate()
}

func (a BundleInspectionArtifact) Validate() error {
	return validateV3(a.SchemaVersion, a.BundleArtifactIdentity)
}
func (a BundleClassificationArtifact) Validate() error {
	if err := validateV3(a.SchemaVersion, a.BundleArtifactIdentity); err != nil {
		return err
	}
	return requireSHA256("classification", a.ClassificationSHA256)
}
func (a BundleValidationArtifact) Validate() error {
	if err := validateV3(a.SchemaVersion, a.BundleArtifactIdentity); err != nil {
		return err
	}
	if requireFullSHA("result tree", a.ResultTreeSHA) != nil || !a.Validation.Complete() {
		return errors.New("v3 bundle validation gates are incomplete")
	}
	return nil
}
func (a BundleFailureArtifact) Validate() error {
	if a.SchemaVersion != 3 {
		return errors.New("bundle artifact requires schema version 3")
	}
	if err := a.BundleFailureIdentity.Validate(); err != nil {
		return err
	}
	if a.State != "blocked" || !validUniqueStrings(a.Blockers) || !validUniqueStrings(a.Recovery) {
		return errors.New("v3 bundle failure is incomplete")
	}
	return nil
}

func (a BundlePreparationArtifact) Validate() error {
	if err := validateV3(a.SchemaVersion, a.BundleArtifactIdentity); err != nil {
		return err
	}
	if requireFullSHA("head", a.HeadSHA) != nil || requireFullSHA("result tree", a.ResultTreeSHA) != nil ||
		a.BranchName != "sync/"+a.PackID || requireSHA256("provenance", a.ProvenanceSHA256) != nil ||
		a.ManagedTitle == "" || requireSHA256("managed metadata", a.ManagedMetadataHash) != nil ||
		a.ObservedBaseSHA != a.BaseSHA || !a.Validation.Complete() || !a.ObservationsStable ||
		a.RepositoryMutated || a.DecisionReady || a.UpstreamContentExecuted {
		return errors.New("v3 bundle preparation is incomplete or claims publication authority")
	}
	return nil
}

func (identity BundleFailureIdentity) Validate() error {
	if !ValidSourceID(identity.PackID) || len(identity.SourceIDs) < 2 ||
		len(identity.SourceIDs) != len(identity.Members) ||
		requireSHA256("registration bundle", identity.RegistrationBundleSHA256) != nil ||
		identity.ProposedVersion == "" || requireSHA256("proposed manifest", identity.ProposedManifestSHA256) != nil ||
		identity.ContainsSecrets || identity.ContainsUpstreamBytes {
		return errors.New("v3 bundle failure identity is incomplete or contradictory")
	}
	previous := ""
	for i, member := range identity.Members {
		if identity.SourceIDs[i] != member.SourceID || member.SourceID <= previous ||
			requireFullSHA("candidate", member.CandidateSHA) != nil ||
			member.LegalEvidenceRef == "" ||
			requireSHA256("legal evidence", member.LegalEvidenceSHA256) != nil {
			return errors.New("v3 bundle failure members are incomplete, mixed, or out of order")
		}
		previous = member.SourceID
	}
	if identity.Plan != nil {
		_, err := identity.PlannedIdentity()
		return err
	}
	return nil
}

// PlannedIdentity reconstructs the exact common artifact identity when Check
// produced a plan. It rejects partial plan/result groups.
func (identity BundleFailureIdentity) PlannedIdentity() (BundleArtifactIdentity, error) {
	if identity.Plan == nil {
		return BundleArtifactIdentity{}, errors.New("v3 bundle failure has no sealed plan identity")
	}
	planned := BundleArtifactIdentity{
		PackID: identity.PackID, SourceIDs: append([]string(nil), identity.SourceIDs...),
		RegistrationBundleSHA256: identity.RegistrationBundleSHA256,
		ProposedVersion:          identity.ProposedVersion, ProposedManifestSHA256: identity.ProposedManifestSHA256,
		Members: append([]BundleArtifactMember(nil), identity.Plan.Members...),
		PlanID:  identity.Plan.PlanID, BaseSHA: identity.Plan.BaseSHA,
		ConfigSHA256: identity.Plan.ConfigSHA256, ManifestsSHA256: identity.Plan.ManifestsSHA256,
		LockSetSHA256: identity.Plan.LockSetSHA256, ResultBundleSHA256: identity.Plan.ResultBundleSHA256,
		ContainsSecrets: identity.ContainsSecrets, ContainsUpstreamBytes: identity.ContainsUpstreamBytes,
	}
	if err := planned.Validate(); err != nil {
		return BundleArtifactIdentity{}, errors.New("v3 bundle failure plan identity is partial")
	}
	return planned, nil
}

func (identity BundleFailureIdentity) Matches(expected BundleArtifactIdentity) error {
	planned, err := identity.PlannedIdentity()
	if err != nil {
		return err
	}
	return planned.Matches(expected)
}
func (a BundlePublicationArtifact) Validate() error {
	if err := validateV3(a.SchemaVersion, a.BundleArtifactIdentity); err != nil {
		return err
	}
	if requireFullSHA("head", a.HeadSHA) != nil || requireFullSHA("result tree", a.ResultTreeSHA) != nil ||
		a.BranchName != "sync/"+a.PackID || a.PRNumber < 1 ||
		requireSHA256("pull request state", a.PRStateSHA256) != nil ||
		requireSHA256("provenance", a.ProvenanceSHA256) != nil || a.ManagedTitle == "" ||
		requireSHA256("managed metadata", a.ManagedMetadataHash) != nil || !a.Validation.Complete() ||
		!a.DecisionReady || a.AutoMerge || !a.ManualMergeRequired || a.UpstreamContentExecuted ||
		!validInvalidationConditions(a.InvalidationConditions) {
		return errors.New("v3 bundle publication is not decision ready")
	}
	return nil
}
