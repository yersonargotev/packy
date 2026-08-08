// Package packsyncworkflow owns the manual synchronization operation around
// packsync. It owns dispatch, retry, publication, artifact, and readiness
// policy, but never candidate admission, compatibility floors, plans, Apply,
// or Recover.
package packsyncworkflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yersonargotev/packy/internal/packsync"
)

type Selector string

const (
	SelectorLatestStable Selector = "latest-stable"
	SelectorPrerelease   Selector = "prerelease"
	SelectorCommit       Selector = "commit"
)

type ClassificationMode string

const (
	ClassificationAI    ClassificationMode = "ai"
	ClassificationHuman ClassificationMode = "human"
)

type HumanDispatchPhase string

const (
	HumanInspect  HumanDispatchPhase = "inspection"
	HumanEvidence HumanDispatchPhase = "evidence"
)

type DispatchOperation string

const (
	OperationSynchronize DispatchOperation = "synchronize"
	OperationRegister    DispatchOperation = "register"
	OperationReconfigure DispatchOperation = "reconfigure"
)

type DispatchRequest struct {
	SchemaVersion          int                               `json:"schema_version"`
	Operation              DispatchOperation                 `json:"operation,omitempty"`
	SourceID               string                            `json:"source_id"`
	Selector               Selector                          `json:"selector"`
	SelectorRef            string                            `json:"selector_ref,omitempty"`
	ClassificationMode     ClassificationMode                `json:"classification_mode"`
	RequestReason          string                            `json:"request_reason"`
	RetryOfRun             string                            `json:"retry_of_run,omitempty"`
	ExpectedPlanID         string                            `json:"expected_plan_id,omitempty"`
	ExpectedBaseSHA        string                            `json:"expected_base_sha,omitempty"`
	HumanEvidence          json.RawMessage                   `json:"human_evidence,omitempty"`
	Registration           *packsync.SourceConfig            `json:"registration,omitempty"`
	RegistrationSHA256     string                            `json:"registration_sha256,omitempty"`
	Reconfiguration        *packsync.SourceConfig            `json:"reconfiguration,omitempty"`
	ReconfigurationSHA256  string                            `json:"reconfiguration_sha256,omitempty"`
	ProposedManifest       json.RawMessage                   `json:"proposed_manifest,omitempty"`
	ProposedManifestSHA256 string                            `json:"proposed_manifest_sha256,omitempty"`
	ProposedVersion        string                            `json:"proposed_version,omitempty"`
	LegalAdmission         *packsync.CompositeLegalAdmission `json:"legal_admission,omitempty"`
	ClosingIssue           string                            `json:"closing_issue,omitempty"`
}

func (request DispatchRequest) IsSingleSourceAdmission() bool {
	return request.Operation == OperationRegister && request.ProposedVersion != "" && len(request.ProposedManifest) != 0 && request.ProposedManifestSHA256 != "" && request.LegalAdmission != nil
}

// Digest returns the stable request identity used only to attach maintainers to
// an existing workflow run. It does not add authority or become a dispatch
// field; the canonical request remains the source of truth.
func (request DispatchRequest) Digest() (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(canonical, '\n'))
	return fmt.Sprintf("%x", sum), nil
}

func (request *DispatchRequest) UnmarshalJSON(data []byte) error {
	type dispatchRequest DispatchRequest
	var decoded dispatchRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{"schema_version": true, "source_id": true, "selector": true, "selector_ref": true, "classification_mode": true, "request_reason": true, "retry_of_run": true, "expected_plan_id": true, "expected_base_sha": true, "human_evidence": true}
	if decoded.SchemaVersion == 2 {
		allowed["operation"] = true
		allowed["registration"] = true
		allowed["registration_sha256"] = true
		allowed["reconfiguration"] = true
		allowed["reconfiguration_sha256"] = true
		allowed["proposed_manifest"] = true
		allowed["proposed_manifest_sha256"] = true
		allowed["proposed_version"] = true
		allowed["legal_admission"] = true
		allowed["closing_issue"] = true
	}
	for name := range fields {
		if !allowed[name] {
			return fmt.Errorf("dispatch contains unknown field %q", name)
		}
	}
	for _, name := range []string{"operation", "selector_ref", "retry_of_run", "expected_plan_id", "expected_base_sha", "registration_sha256", "reconfiguration_sha256", "proposed_manifest_sha256", "proposed_version", "closing_issue"} {
		value, present := fields[name]
		if !present {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil || text == "" {
			return fmt.Errorf("dispatch field %s must be a non-empty string when present", name)
		}
	}
	if raw, present := fields["registration"]; present {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var registration packsync.SourceConfig
		if err := decoder.Decode(&registration); err != nil {
			return fmt.Errorf("decode registration: %w", err)
		}
		decoded.Registration = &registration
	}
	if raw, present := fields["reconfiguration"]; present {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var reconfiguration packsync.SourceConfig
		if err := decoder.Decode(&reconfiguration); err != nil {
			return fmt.Errorf("decode reconfiguration: %w", err)
		}
		decoded.Reconfiguration = &reconfiguration
	}
	if raw, present := fields["legal_admission"]; present {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var legal packsync.CompositeLegalAdmission
		if err := decoder.Decode(&legal); err != nil {
			return fmt.Errorf("decode legal admission: %w", err)
		}
		decoded.LegalAdmission = &legal
	}
	*request = DispatchRequest(decoded)
	return nil
}

// CanonicalRegistrationSHA256 returns the identity sealed by a registration
// dispatch. The canonical bytes are the validated, binding-sorted SourceConfig
// encoded as two-space-indented JSON with one trailing LF.
func CanonicalRegistrationSHA256(registration packsync.SourceConfig) (string, error) {
	normalized, err := normalizeRegistration(registration)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func CanonicalReconfigurationSHA256(reconfiguration packsync.SourceConfig) (string, error) {
	return canonicalSourceConfigSHA256(reconfiguration, "reconfiguration")
}

func CanonicalProposedManifest(proposed json.RawMessage) (json.RawMessage, string, error) {
	if len(proposed) == 0 {
		return nil, "", errors.New("proposed manifest is required")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(proposed))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, "", fmt.Errorf("decode proposed manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, "", errors.New("proposed manifest contains multiple JSON values")
		}
		return nil, "", fmt.Errorf("decode trailing proposed manifest: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, "", err
	}
	data = append(data, '\n')
	sum := sha256.Sum256(data)
	return data, fmt.Sprintf("%x", sum), nil
}

func canonicalSourceConfigSHA256(source packsync.SourceConfig, label string) (string, error) {
	normalized, err := normalizeSourceConfig(source, label)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func normalizeRegistration(registration packsync.SourceConfig) (packsync.SourceConfig, error) {
	return normalizeSourceConfig(registration, "registration")
}

func normalizeSourceConfig(registration packsync.SourceConfig, label string) (packsync.SourceConfig, error) {
	if registration.Resources == nil {
		return packsync.SourceConfig{}, fmt.Errorf("%s resources is a required array", label)
	}
	data, err := json.Marshal(packsync.Config{SchemaVersion: 1, Sources: []packsync.SourceConfig{registration}})
	if err != nil {
		return packsync.SourceConfig{}, err
	}
	config, err := packsync.LoadConfig(bytes.NewReader(data))
	if err != nil {
		return packsync.SourceConfig{}, fmt.Errorf("invalid %s: %w", label, err)
	}
	if !sourceIDPattern.MatchString(config.Sources[0].ID) {
		return packsync.SourceConfig{}, fmt.Errorf("%s source id is not canonical", label)
	}
	return config.Sources[0], nil
}

type ValidationArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	SourceID      string `json:"source_id"`
	PlanID        string `json:"plan_id"`
	BaseSHA       string `json:"base_sha"`
	CandidateSHA  string `json:"candidate_sha"`
	ArtifactProvenance
	InitialAdmissionArtifactIdentity
	ResultTreeSHA string `json:"result_tree_sha,omitempty"`
	PackySuite    bool   `json:"packy_suite"`
	Apply         bool   `json:"apply"`
	UpstreamBytes bool   `json:"contains_upstream_bytes"`
	ClosingIssue  string `json:"closing_issue,omitempty"`
}

func (artifact ValidationArtifact) Validate() error {
	if artifact.SchemaVersion != 1 && artifact.SchemaVersion != 2 {
		return errors.New("sandbox validation schema is not supported")
	}
	if !sourceIDPattern.MatchString(artifact.SourceID) || artifact.PlanID == "" || requireFullSHA("base", artifact.BaseSHA) != nil || requireFullSHA("candidate", artifact.CandidateSHA) != nil || !artifact.PackySuite || !artifact.Apply || artifact.UpstreamBytes {
		return errors.New("sandbox validation proof is incomplete or contradictory")
	}
	if artifact.SchemaVersion == 1 {
		if !artifact.ArtifactProvenance.empty() || !artifact.InitialAdmissionArtifactIdentity.empty() || artifact.ResultTreeSHA != "" || artifact.ClosingIssue != "" {
			return errors.New("v1 sandbox validation proof forbids v2 provenance")
		}
		return nil
	}
	if !artifact.ArtifactProvenance.valid() || requireFullSHA("result tree", artifact.ResultTreeSHA) != nil {
		return errors.New("v2 sandbox validation proof lacks complete provenance")
	}
	if !artifact.InitialAdmissionArtifactIdentity.empty() && !artifact.InitialAdmissionArtifactIdentity.valid(true) {
		return errors.New("v2 sandbox validation proof has incomplete initial-admission identity")
	}
	if artifact.ClosingIssue != "" {
		if _, _, ok := ParseClosingIssue(artifact.ClosingIssue); !ok {
			return errors.New("v2 sandbox validation proof has an invalid closing issue")
		}
	}
	return nil
}

// ArtifactProvenance binds a workflow artifact to both the target source lock
// and the complete configured bundle observed by the workflow.
type ArtifactProvenance struct {
	SourceLockSHA256 string `json:"source_lock_sha256,omitempty"`
	LockSetSHA256    string `json:"lock_set_sha256,omitempty"`
	ConfigSHA256     string `json:"config_sha256,omitempty"`
	ManifestsSHA256  string `json:"manifests_sha256,omitempty"`
}

// InitialAdmissionArtifactIdentity carries the complete one-source initial
// generation identity through v2.3 workflow artifacts.
type InitialAdmissionArtifactIdentity struct {
	PackID                 string `json:"pack_id,omitempty"`
	ProposedVersion        string `json:"proposed_version,omitempty"`
	ProposedManifestSHA256 string `json:"proposed_manifest_sha256,omitempty"`
	LegalEvidenceReference string `json:"legal_evidence_reference,omitempty"`
	LegalEvidenceSHA256    string `json:"legal_evidence_sha256,omitempty"`
	ResultBundleSHA256     string `json:"result_bundle_sha256,omitempty"`
}

func (identity InitialAdmissionArtifactIdentity) empty() bool {
	return identity == (InitialAdmissionArtifactIdentity{})
}

func (identity InitialAdmissionArtifactIdentity) valid(requireResult bool) bool {
	if !ValidSourceID(identity.PackID) || identity.ProposedVersion == "" ||
		requireSHA256("proposed manifest", identity.ProposedManifestSHA256) != nil ||
		identity.LegalEvidenceReference == "" || requireSHA256("legal evidence", identity.LegalEvidenceSHA256) != nil {
		return false
	}
	if requireResult {
		return requireSHA256("result bundle", identity.ResultBundleSHA256) == nil
	}
	return identity.ResultBundleSHA256 == "" || requireSHA256("result bundle", identity.ResultBundleSHA256) == nil
}

func (provenance ArtifactProvenance) valid() bool {
	return requireSHA256("source lock", provenance.SourceLockSHA256) == nil &&
		requireSHA256("lock set", provenance.LockSetSHA256) == nil &&
		requireSHA256("configuration", provenance.ConfigSHA256) == nil &&
		requireSHA256("manifests", provenance.ManifestsSHA256) == nil
}

func (provenance ArtifactProvenance) empty() bool {
	return provenance == (ArtifactProvenance{})
}

type NoopArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	SourceID      string `json:"source_id"`
	PlanID        string `json:"plan_id"`
	BaseSHA       string `json:"base_sha"`
	CandidateSHA  string `json:"candidate_sha"`
	ArtifactProvenance
	ContainsSecrets       bool   `json:"contains_secrets"`
	ContainsUpstreamBytes bool   `json:"contains_upstream_bytes"`
	ClosingIssue          string `json:"closing_issue,omitempty"`
}

func (artifact NoopArtifact) Validate() error {
	if artifact.SchemaVersion != 2 || artifact.State != "no-op" || !ValidSourceID(artifact.SourceID) || artifact.PlanID == "" || requireFullSHA("base", artifact.BaseSHA) != nil || requireFullSHA("candidate", artifact.CandidateSHA) != nil || !artifact.ArtifactProvenance.valid() || artifact.ContainsSecrets || artifact.ContainsUpstreamBytes {
		return errors.New("v2 no-op artifact is incomplete or contradictory")
	}
	if artifact.ClosingIssue != "" {
		if _, _, ok := ParseClosingIssue(artifact.ClosingIssue); !ok {
			return errors.New("v2 no-op artifact has an invalid closing issue")
		}
	}
	return nil
}

var (
	sourceIDPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	fullSHAPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Pattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	prereleasePattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z][0-9A-Za-z.-]*$`)
	runIDPattern      = regexp.MustCompile(`^[0-9]+$`)
)

func (request DispatchRequest) Validate() error {
	if request.SchemaVersion != 1 && request.SchemaVersion != 2 {
		return errors.New("dispatch schema is not supported")
	}
	if !sourceIDPattern.MatchString(request.SourceID) || strings.TrimSpace(request.RequestReason) == "" || utf8.RuneCountInString(request.RequestReason) > 500 {
		return errors.New("dispatch schema, source id, and request reason are required")
	}
	if request.SchemaVersion == 1 {
		if request.Operation != "" || request.Registration != nil || request.RegistrationSHA256 != "" || request.Reconfiguration != nil || request.ReconfigurationSHA256 != "" || len(request.ProposedManifest) != 0 || request.ProposedManifestSHA256 != "" || request.ProposedVersion != "" || request.LegalAdmission != nil || request.ClosingIssue != "" {
			return errors.New("v1 dispatch forbids v2 operation fields")
		}
	} else {
		if request.ClosingIssue != "" {
			if _, _, ok := ParseClosingIssue(request.ClosingIssue); !ok {
				return errors.New("closing issue must be one canonical GitHub issue URL")
			}
		}
		switch request.Operation {
		case OperationSynchronize:
			if request.Registration != nil || request.RegistrationSHA256 != "" || request.Reconfiguration != nil || request.ReconfigurationSHA256 != "" || len(request.ProposedManifest) != 0 || request.ProposedManifestSHA256 != "" || request.ProposedVersion != "" || request.LegalAdmission != nil {
				return errors.New("synchronize dispatch forbids configuration proposals")
			}
		case OperationRegister:
			if request.Registration == nil || request.RegistrationSHA256 == "" || request.Reconfiguration != nil || request.ReconfigurationSHA256 != "" {
				return errors.New("register dispatch requires registration and its SHA-256")
			}
			if request.Registration.ID != request.SourceID {
				return errors.New("registration id must equal dispatch source id")
			}
			digest, err := CanonicalRegistrationSHA256(*request.Registration)
			if err != nil || digest != request.RegistrationSHA256 {
				return errors.New("registration SHA-256 does not match canonical registration")
			}
			initialFieldCount := 0
			for _, present := range []bool{request.ProposedVersion != "", len(request.ProposedManifest) != 0, request.ProposedManifestSHA256 != "", request.LegalAdmission != nil} {
				if present {
					initialFieldCount++
				}
			}
			if initialFieldCount != 0 && initialFieldCount != 4 {
				return errors.New("initial register dispatch requires proposed version, manifest, manifest digest, and legal admission together")
			}
			if initialFieldCount == 4 {
				_, manifestDigest, err := CanonicalProposedManifest(request.ProposedManifest)
				if err != nil || manifestDigest != request.ProposedManifestSHA256 {
					return errors.New("initial proposed manifest SHA-256 or canonical bytes do not match")
				}
				if request.Selector != SelectorLatestStable || request.Registration.Selector.Mode != packsync.SelectorStableRelease || request.LegalAdmission.Disposition != packsync.RedistributableDisposition || !sha256Pattern.MatchString(request.LegalAdmission.EvidenceSHA256) || request.LegalAdmission.EvidenceReference == "" {
					return errors.New("initial register dispatch requires latest stable selection and complete redistributable legal admission")
				}
			}
		case OperationReconfigure:
			if request.Registration != nil || request.RegistrationSHA256 != "" || request.Reconfiguration == nil || request.ReconfigurationSHA256 == "" || len(request.ProposedManifest) == 0 || request.ProposedManifestSHA256 == "" || request.ProposedVersion != "" || request.LegalAdmission != nil {
				return errors.New("reconfigure dispatch requires the complete reconfiguration and proposed manifest with their SHA-256 digests")
			}
			if request.Reconfiguration.ID != request.SourceID {
				return errors.New("reconfiguration id must equal dispatch source id")
			}
			digest, err := CanonicalReconfigurationSHA256(*request.Reconfiguration)
			if err != nil || digest != request.ReconfigurationSHA256 {
				return errors.New("reconfiguration SHA-256 does not match canonical reconfiguration")
			}
			_, manifestDigest, err := CanonicalProposedManifest(request.ProposedManifest)
			if err != nil || manifestDigest != request.ProposedManifestSHA256 {
				return errors.New("proposed manifest SHA-256 or canonical bytes do not match")
			}
		default:
			return errors.New("v2 dispatch operation must be synchronize, register, or reconfigure")
		}
	}
	switch request.Selector {
	case SelectorLatestStable:
		if request.SelectorRef != "" {
			return errors.New("latest-stable forbids a selector ref")
		}
	case SelectorPrerelease:
		if !prereleasePattern.MatchString(request.SelectorRef) {
			return errors.New("prerelease selector requires one exact published prerelease tag")
		}
	case SelectorCommit:
		if !fullSHAPattern.MatchString(request.SelectorRef) {
			return errors.New("commit selector requires one full lowercase commit SHA")
		}
	default:
		return errors.New("selector is not supported")
	}
	if request.ClassificationMode != ClassificationAI && request.ClassificationMode != ClassificationHuman {
		return errors.New("classification mode must be explicitly ai or human")
	}
	if request.RetryOfRun != "" && !runIDPattern.MatchString(request.RetryOfRun) {
		return errors.New("retry_of_run must identify one prior numeric workflow run")
	}
	hasHumanBinding := request.ExpectedPlanID != "" || request.ExpectedBaseSHA != "" || len(request.HumanEvidence) != 0
	if request.ClassificationMode == ClassificationAI && hasHumanBinding {
		return errors.New("AI dispatch contradicts human evidence binding")
	}
	if hasHumanBinding {
		if request.ClassificationMode != ClassificationHuman || request.Selector != SelectorCommit || request.ExpectedPlanID == "" || !fullSHAPattern.MatchString(request.ExpectedBaseSHA) || len(request.HumanEvidence) == 0 || !json.Valid(request.HumanEvidence) {
			return errors.New("human evidence dispatch requires exact commit, plan, base, and canonical evidence")
		}
	}
	return nil
}

func (request DispatchRequest) HumanPhase() (HumanDispatchPhase, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	if request.ClassificationMode != ClassificationHuman {
		return "", errors.New("human phase requires explicit human classification")
	}
	if len(request.HumanEvidence) == 0 {
		return HumanInspect, nil
	}
	return HumanEvidence, nil
}

type FailureArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	SourceID      string `json:"source_id"`
	PlanID        string `json:"plan_id,omitempty"`
	BaseSHA       string `json:"base_sha,omitempty"`
	CandidateSHA  string `json:"candidate_sha,omitempty"`
	ArtifactProvenance
	InitialAdmissionArtifactIdentity
	Blockers              []string `json:"blockers"`
	Recovery              []string `json:"recovery"`
	RunURL                string   `json:"run_url,omitempty"`
	ContainsSecrets       bool     `json:"contains_secrets"`
	ContainsUpstreamBytes bool     `json:"contains_upstream_bytes"`
	ClosingIssue          string   `json:"closing_issue,omitempty"`
}

func (artifact FailureArtifact) CanonicalJSON() ([]byte, error) {
	if artifact.SchemaVersion != 1 && artifact.SchemaVersion != 2 {
		return nil, errors.New("operational artifact schema is not supported")
	}
	if artifact.State != "blocked" || !ValidSourceID(artifact.SourceID) || !validOptionalSHA(artifact.BaseSHA) || !validOptionalSHA(artifact.CandidateSHA) || !validUniqueStrings(artifact.Blockers) || !validUniqueStrings(artifact.Recovery) || !validOptionalURI(artifact.RunURL) {
		return nil, errors.New("operational artifact is incomplete")
	}
	if artifact.SchemaVersion == 1 && (!artifact.ArtifactProvenance.empty() || !artifact.InitialAdmissionArtifactIdentity.empty()) {
		return nil, errors.New("v1 operational artifact forbids v2 provenance")
	}
	if artifact.ClosingIssue != "" {
		if artifact.SchemaVersion != 2 {
			return nil, errors.New("v1 operational artifact forbids a closing issue")
		}
		if _, _, ok := ParseClosingIssue(artifact.ClosingIssue); !ok {
			return nil, errors.New("operational artifact has an invalid closing issue")
		}
	}
	if artifact.SchemaVersion == 2 && ((artifact.PlanID != "" && !artifact.ArtifactProvenance.valid()) || (artifact.PlanID == "" && !artifact.ArtifactProvenance.empty())) {
		return nil, errors.New("v2 operational artifact provenance contradicts plan state")
	}
	if !artifact.InitialAdmissionArtifactIdentity.empty() && !artifact.InitialAdmissionArtifactIdentity.valid(false) {
		return nil, errors.New("v2 operational artifact has incomplete initial-admission identity")
	}
	if artifact.ContainsSecrets || artifact.ContainsUpstreamBytes {
		return nil, errors.New("operational artifacts must not contain secrets or upstream bytes")
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Indent(&compact, data, "", "  "); err != nil {
		return nil, err
	}
	compact.WriteByte('\n')
	return compact.Bytes(), nil
}

type PublicationArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	SourceID      string `json:"source_id"`
	PlanID        string `json:"plan_id"`
	BaseSHA       string `json:"base_sha"`
	CandidateSHA  string `json:"candidate_sha"`
	ArtifactProvenance
	InitialAdmissionArtifactIdentity
	ResultTreeSHA           string          `json:"result_tree_sha"`
	HeadSHA                 string          `json:"head_sha"`
	ProvenanceSHA256        string          `json:"provenance_sha256"`
	BranchName              string          `json:"branch_name"`
	PRNumber                int             `json:"pr_number"`
	PRStateSHA256           string          `json:"pr_state_sha256"`
	ManagedTitle            string          `json:"managed_title"`
	ManagedMetadataHash     string          `json:"managed_metadata_hash"`
	Validation              ValidationGates `json:"validation"`
	DecisionReady           bool            `json:"decision_ready"`
	AutoMerge               bool            `json:"auto_merge"`
	ManualMergeRequired     bool            `json:"manual_merge_required"`
	UpstreamContentExecuted bool            `json:"upstream_content_executed"`
	InvalidationConditions  []string        `json:"invalidation_conditions"`
	ClosingIssue            string          `json:"closing_issue,omitempty"`
}

func (artifact PublicationArtifact) Validate() error {
	if artifact.SchemaVersion != 2 || !ValidSourceID(artifact.SourceID) || artifact.PlanID == "" || requireFullSHA("base", artifact.BaseSHA) != nil || requireFullSHA("candidate", artifact.CandidateSHA) != nil || !artifact.ArtifactProvenance.valid() || requireFullSHA("result tree", artifact.ResultTreeSHA) != nil || requireFullSHA("head", artifact.HeadSHA) != nil || requireSHA256("provenance", artifact.ProvenanceSHA256) != nil || artifact.BranchName != "sync/"+artifact.SourceID || artifact.PRNumber < 1 || requireSHA256("pull request state", artifact.PRStateSHA256) != nil || artifact.ManagedTitle == "" || requireSHA256("managed metadata", artifact.ManagedMetadataHash) != nil {
		return errors.New("v2 publication identity is incomplete")
	}
	if !artifact.Validation.Complete() || !artifact.DecisionReady || artifact.AutoMerge || !artifact.ManualMergeRequired || artifact.UpstreamContentExecuted || !validInvalidationConditions(artifact.InvalidationConditions, artifact.ClosingIssue != "") {
		return errors.New("v2 publication is not decision ready")
	}
	if !artifact.InitialAdmissionArtifactIdentity.empty() && !artifact.InitialAdmissionArtifactIdentity.valid(true) {
		return errors.New("v2 publication has incomplete initial-admission identity")
	}
	if artifact.ClosingIssue != "" {
		if _, _, ok := ParseClosingIssue(artifact.ClosingIssue); !ok {
			return errors.New("v2 publication has an invalid closing issue")
		}
	}
	return nil
}

func validInvalidationConditions(values []string, closingIssue bool) bool {
	want := map[string]bool{"base_changed": true, "candidate_changed": true, "provenance_changed": true, "head_changed": true, "pr_state_changed": true}
	if closingIssue {
		want["issue_changed"] = true
	}
	if len(values) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !want[value] || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

// ValidSourceID reports whether value is safe for canonical workflow identity.
func ValidSourceID(value string) bool {
	return sourceIDPattern.MatchString(value)
}

func validOptionalSHA(value string) bool {
	return value == "" || fullSHAPattern.MatchString(value)
}

func validUniqueStrings(values []string) bool {
	if len(values) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validOptionalURI(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs()
}

func requireFullSHA(name, value string) error {
	if !fullSHAPattern.MatchString(value) {
		return fmt.Errorf("%s must be one full lowercase SHA", name)
	}
	return nil
}

func requireSHA256(name, value string) error {
	if !sha256Pattern.MatchString(value) {
		return fmt.Errorf("%s must be one lowercase hexadecimal SHA-256", name)
	}
	return nil
}
