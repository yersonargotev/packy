package addyacceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	PromotionEvidenceSchema = "addy-promotion-evidence.v1"
	PromotionMatrixVersion  = "addy-claude-promotion.v1"
	PromotionCheckName      = "Addy 1.1.0 promotion gate"

	PromotionNotApplicable = "not_applicable"
	PromotionApplicable    = "applicable"
	PromotionPassed        = "passed"
)

// PromotionRow is one immutable identity in the Addy 1.1.0 promotion matrix.
type PromotionRow struct {
	ID                string `json:"id"`
	Number            int    `json:"number"`
	Gate              int    `json:"gate"`
	Name              string `json:"name"`
	OwningTest        string `json:"owning_test"`
	BlockedDiagnostic string `json:"blocked_diagnostic"`
}

var promotionRows = []PromotionRow{
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-01", Number: 1, Gate: 1, Name: "exact-source-and-content-invariance", OwningTest: "TestAddyPromotionIndependentInputs", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-01-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-02", Number: 2, Gate: 1, Name: "immutable-1.0.0-history", OwningTest: "TestAddyPromotionIndependentInputs", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-02-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-03", Number: 3, Gate: 1, Name: "exact-atomic-catalog-delta", OwningTest: "TestAddyPromotionIndependentInputs", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-03-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-04", Number: 4, Gate: 2, Name: "strict-v3-inventory-contract", OwningTest: "TestAddyPromotionAuthorityFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-04-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-05", Number: 5, Gate: 2, Name: "complete-claude-projection-and-authority", OwningTest: "TestAddyPromotionAuthorityFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-05-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-06", Number: 6, Gate: 2, Name: "surface-local-compatibility-and-intent", OwningTest: "TestAddyPromotionAuthorityFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-06-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-07", Number: 7, Gate: 3, Name: "deterministic-discovery-and-history-output", OwningTest: "TestAddyPromotionLifecycleFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-07-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-08", Number: 8, Gate: 3, Name: "deterministic-lifecycle-structured-output", OwningTest: "TestAddyPromotionLifecycleFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-08-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-09", Number: 9, Gate: 4, Name: "plan-approval-apply-and-recovery-atomicity", OwningTest: "TestAddyPromotionLifecycleFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-09-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-10", Number: 10, Gate: 4, Name: "collision-alias-ownership-and-removal-safety", OwningTest: "TestAddyPromotionLifecycleFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-10-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-11", Number: 11, Gate: 5, Name: "package-installed-parity", OwningTest: "TestAddyPromotionRealHostFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-11-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-12", Number: 12, Gate: 5, Name: "real-claude-code-addy-smoke", OwningTest: "TestAddyPromotionRealHostFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-12-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-13", Number: 13, Gate: 6, Name: "protected-promotion-pr", OwningTest: "TestAddyPromotionEvidenceFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-13-BLOCKED"},
	{ID: "ADDY-CLAUDE-PROMOTION-ROW-14", Number: 14, Gate: 6, Name: "exact-tag-release-publication", OwningTest: "TestAddyPromotionEvidenceFoundations", BlockedDiagnostic: "ADDY-CLAUDE-PROMOTION-ROW-14-BLOCKED"},
}

func PromotionRows() []PromotionRow {
	return append([]PromotionRow(nil), promotionRows...)
}

// ImmutableVersionFixture records reconstructed artifact bytes without making
// either version a catalog selection.
type ImmutableVersionFixture struct {
	Version        string   `json:"version"`
	SchemaVersion  int      `json:"schema_version"`
	Surfaces       []string `json:"surfaces"`
	ManifestSHA256 string   `json:"manifest_sha256"`
	SnapshotSHA256 string   `json:"snapshot_sha256"`
}

type PromotionHistoryFixture struct {
	CatalogAdvertised bool                      `json:"catalog_advertised"`
	CurrentVersion    string                    `json:"current_version"`
	Versions          []ImmutableVersionFixture `json:"versions"`
	Routes            []VersionRoute            `json:"routes"`
}

// IndependentPromotionInputs are reconstructed by the gate outside candidate
// evidence. The candidate may report these hashes but cannot choose their
// trusted values.
type IndependentPromotionInputs struct {
	BaseSHA256    string `json:"base_sha256"`
	HeadSHA256    string `json:"head_sha256"`
	HistorySHA256 string `json:"history_sha256"`
	DiffSHA256    string `json:"diff_sha256"`
}

type PromotionRowEvidence struct {
	ID             string    `json:"id"`
	Result         string    `json:"result"`
	EvidenceSHA256 string    `json:"evidence_sha256"`
	CommitSHA      string    `json:"commit_sha"`
	WorkflowDigest string    `json:"workflow_digest"`
	RunID          string    `json:"run_id"`
	CollectedAt    time.Time `json:"collected_at"`
}

type PromotionProof struct {
	IndependentPromotionInputs
	SyncAddyParticipated bool `json:"sync_addy_participated"`
}

type PromotionEvidence struct {
	Schema            string                 `json:"schema"`
	Disposition       string                 `json:"disposition"`
	Repository        string                 `json:"repository"`
	PullRequest       int                    `json:"pull_request,omitempty"`
	BaseSHA           string                 `json:"base_sha,omitempty"`
	HeadSHA           string                 `json:"head_sha,omitempty"`
	EvaluatedMergeSHA string                 `json:"evaluated_merge_sha,omitempty"`
	Tag               string                 `json:"tag,omitempty"`
	Workflow          string                 `json:"workflow"`
	WorkflowDigest    string                 `json:"workflow_digest"`
	MatrixVersion     string                 `json:"matrix_version"`
	RunID             string                 `json:"run_id"`
	CollectedAt       time.Time              `json:"collected_at"`
	Rows              []PromotionRowEvidence `json:"rows"`
	Proof             PromotionProof         `json:"proof"`
	PackageCandidate  string                 `json:"package_candidate"`
	ClaudeIdentities  []string               `json:"claude_identities"`
	AtomicitySHA256   string                 `json:"atomicity_sha256"`
}

// PromotionValidationContext contains trusted workflow and reconstruction
// facts. It is intentionally separate from the candidate-controlled evidence.
type PromotionValidationContext struct {
	PromotionChange   bool
	Repository        string
	PullRequest       int
	BaseSHA           string
	HeadSHA           string
	EvaluatedMergeSHA string
	Tag               string
	Workflow          string
	WorkflowDigest    string
	MatrixVersion     string
	RunID             string
	Now               time.Time
	MaxAge            time.Duration
	Inputs            IndependentPromotionInputs
}

func CanonicalPromotionHistory() PromotionHistoryFixture {
	fixture := Canonical()
	base := canonicalBytes(fixture.Manifest)
	candidate := canonicalBytes(struct {
		SchemaVersion int      `json:"schema_version"`
		ID            string   `json:"id"`
		Version       string   `json:"version"`
		Surfaces      []string `json:"surfaces"`
		Synthetic     bool     `json:"synthetic_non_catalog_fixture"`
	}{SchemaVersion: 3, ID: "addy", Version: "1.1.0", Surfaces: []string{"claude", "codex", "opencode"}, Synthetic: true})
	return PromotionHistoryFixture{
		CatalogAdvertised: false,
		CurrentVersion:    PackVersion,
		Versions: []ImmutableVersionFixture{
			{Version: PackVersion, SchemaVersion: 2, Surfaces: []string{"codex", "opencode"}, ManifestSHA256: digest(base), SnapshotSHA256: fixture.Provenance.SnapshotSHA256},
			{Version: "1.1.0", SchemaVersion: 3, Surfaces: []string{"claude", "codex", "opencode"}, ManifestSHA256: digest(candidate), SnapshotSHA256: fixture.Provenance.SnapshotSHA256},
		},
		Routes: []VersionRoute{{From: PackVersion, To: "1.1.0", Kind: "update", Migration: []string{}, Actions: []string{"project-complete-surface"}}},
	}
}

func CanonicalIndependentPromotionInputs() IndependentPromotionInputs {
	base, _ := CanonicalJSON()
	history := canonicalBytes(CanonicalPromotionHistory())
	diff := canonicalBytes(struct {
		AddySourceChanged    bool     `json:"addy_source_changed"`
		SyncAddyParticipated bool     `json:"sync_addy_participated"`
		AllowedKinds         []string `json:"allowed_kinds"`
	}{AllowedKinds: []string{"catalog-selection", "history", "manifest-v3", "promotion-gate"}, AddySourceChanged: false, SyncAddyParticipated: false})
	return IndependentPromotionInputs{
		BaseSHA256:    digest(base),
		HeadSHA256:    digest(base),
		HistorySHA256: digest(history),
		DiffSHA256:    digest(diff),
	}
}

func NewNotApplicablePromotionEvidence(context PromotionValidationContext) PromotionEvidence {
	return PromotionEvidence{
		Schema:            PromotionEvidenceSchema,
		Disposition:       PromotionNotApplicable,
		Repository:        context.Repository,
		PullRequest:       context.PullRequest,
		BaseSHA:           context.BaseSHA,
		HeadSHA:           context.HeadSHA,
		EvaluatedMergeSHA: context.EvaluatedMergeSHA,
		Tag:               context.Tag,
		Workflow:          context.Workflow,
		WorkflowDigest:    context.WorkflowDigest,
		MatrixVersion:     context.MatrixVersion,
		RunID:             context.RunID,
		CollectedAt:       context.Now.UTC(),
		Rows:              []PromotionRowEvidence{},
		Proof:             PromotionProof{IndependentPromotionInputs: context.Inputs},
		ClaudeIdentities:  []string{},
	}
}

func (e PromotionEvidence) CanonicalJSON() ([]byte, error) {
	if e.Rows == nil || e.ClaudeIdentities == nil {
		return nil, errors.New("promotion evidence arrays must be non-null")
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func DecodePromotionEvidence(data []byte) (PromotionEvidence, error) {
	var evidence PromotionEvidence
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return PromotionEvidence{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return PromotionEvidence{}, errors.New("promotion evidence contains multiple JSON values")
		}
		return PromotionEvidence{}, err
	}
	return evidence, nil
}

func ValidateCanonicalPromotionEvidence(data []byte, context PromotionValidationContext) (PromotionEvidence, error) {
	evidence, err := DecodePromotionEvidence(data)
	if err != nil {
		return PromotionEvidence{}, err
	}
	if err := ValidatePromotionEvidence(evidence, context); err != nil {
		return PromotionEvidence{}, err
	}
	canonical, err := evidence.CanonicalJSON()
	if err != nil {
		return PromotionEvidence{}, err
	}
	if !bytes.Equal(data, canonical) {
		return PromotionEvidence{}, errors.New("promotion evidence is not canonical JSON")
	}
	return evidence, nil
}

func ValidatePromotionEvidence(e PromotionEvidence, context PromotionValidationContext) error {
	if err := validatePromotionContext(context); err != nil {
		return err
	}
	if e.Schema != PromotionEvidenceSchema || e.Repository != context.Repository || e.PullRequest != context.PullRequest ||
		e.BaseSHA != context.BaseSHA || e.HeadSHA != context.HeadSHA || e.EvaluatedMergeSHA != context.EvaluatedMergeSHA ||
		e.Tag != context.Tag || e.Workflow != context.Workflow || e.WorkflowDigest != context.WorkflowDigest ||
		e.MatrixVersion != context.MatrixVersion || e.RunID != context.RunID {
		return errors.New("promotion evidence identity does not match trusted workflow context")
	}
	if stale(e.CollectedAt, context.Now, context.MaxAge) {
		return errors.New("promotion evidence is stale or future-dated")
	}
	if e.Proof.IndependentPromotionInputs != context.Inputs || e.Proof.SyncAddyParticipated {
		return errors.New("promotion evidence invariance, history, or diff proof does not match independent reconstruction")
	}
	if e.Rows == nil || e.ClaudeIdentities == nil {
		return errors.New("promotion evidence arrays must be non-null")
	}
	if e.Disposition == PromotionNotApplicable {
		if context.PromotionChange {
			return errors.New("promotion change cannot be not_applicable")
		}
		if len(e.Rows) != 0 || e.PackageCandidate != "" || len(e.ClaudeIdentities) != 0 || e.AtomicitySHA256 != "" {
			return errors.New("not_applicable promotion evidence contains applicable-only facts")
		}
		return nil
	}
	if e.Disposition != PromotionApplicable || !context.PromotionChange {
		return errors.New("promotion evidence disposition is missing or ambiguous")
	}
	if (e.PullRequest > 0) == (e.Tag != "") || e.PullRequest > 0 && e.EvaluatedMergeSHA == "" {
		return errors.New("promotion evidence must identify exactly one evaluated pull request or exact tag")
	}
	if !validSHA256(e.PackageCandidate) || !validSHA256(e.AtomicitySHA256) || len(e.ClaudeIdentities) == 0 || !sortedUnique(e.ClaudeIdentities) {
		return errors.New("promotion evidence package, Claude, or atomicity identity is incomplete")
	}
	known := make(map[string]bool, len(promotionRows))
	for _, row := range promotionRows {
		known[row.ID] = true
	}
	seen := make(map[string]bool, len(e.Rows))
	for _, row := range e.Rows {
		if !known[row.ID] {
			return fmt.Errorf("promotion evidence contains unknown row %q", row.ID)
		}
		if seen[row.ID] {
			return fmt.Errorf("promotion evidence contains duplicate row %q", row.ID)
		}
		seen[row.ID] = true
		if row.Result != PromotionPassed {
			return fmt.Errorf("promotion evidence row %s has ambiguous result %q", row.ID, row.Result)
		}
		if !validSHA256(row.EvidenceSHA256) || row.CommitSHA != contextCommit(context) || row.WorkflowDigest != context.WorkflowDigest || row.RunID != context.RunID {
			return fmt.Errorf("promotion evidence row %s is cross-commit, cross-workflow, or cross-run", row.ID)
		}
		if stale(row.CollectedAt, context.Now, context.MaxAge) {
			return fmt.Errorf("promotion evidence row %s is stale or future-dated", row.ID)
		}
	}
	if len(seen) != len(promotionRows) {
		return fmt.Errorf("promotion evidence has %d rows, want %d", len(seen), len(promotionRows))
	}
	return nil
}

func validatePromotionContext(context PromotionValidationContext) error {
	if strings.TrimSpace(context.Repository) == "" || context.Workflow == "" || !validSHA256(context.WorkflowDigest) ||
		context.MatrixVersion != PromotionMatrixVersion || context.RunID == "" || context.Now.IsZero() || context.MaxAge <= 0 {
		return errors.New("trusted promotion validation context is incomplete")
	}
	if !validGitSHA(context.BaseSHA) || !validGitSHA(context.HeadSHA) || context.EvaluatedMergeSHA != "" && !validGitSHA(context.EvaluatedMergeSHA) {
		return errors.New("trusted promotion commit identity is invalid")
	}
	if (context.PullRequest > 0) == (context.Tag != "") {
		return errors.New("trusted promotion context must identify exactly one pull request or tag")
	}
	for _, value := range []string{context.Inputs.BaseSHA256, context.Inputs.HeadSHA256, context.Inputs.HistorySHA256, context.Inputs.DiffSHA256} {
		if !validSHA256(value) {
			return errors.New("trusted independent promotion input is invalid")
		}
	}
	return nil
}

func contextCommit(context PromotionValidationContext) string {
	if context.EvaluatedMergeSHA != "" {
		return context.EvaluatedMergeSHA
	}
	return context.HeadSHA
}

func stale(value, now time.Time, maxAge time.Duration) bool {
	return value.IsZero() || value.After(now) || now.Sub(value) > maxAge
}

func validGitSHA(value string) bool {
	return (len(value) == 40 || len(value) == 64) && lowercaseHex(value)
}

func validSHA256(value string) bool {
	return len(value) == 64 && lowercaseHex(value)
}

func lowercaseHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func sortedUnique(values []string) bool {
	return sort.StringsAreSorted(values) && !hasDuplicate(values)
}

func hasDuplicate(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] == values[i] {
			return true
		}
	}
	return false
}

func canonicalBytes(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
