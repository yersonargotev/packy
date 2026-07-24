package addyacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/governancedrift"
)

const AcceptanceRunReportSchema = "addy-acceptance-run.v1"

type AcceptanceRunRow struct {
	ID             string `json:"id"`
	Package        string `json:"package"`
	OwningTest     string `json:"owning_test"`
	Result         string `json:"result"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type AcceptanceRunReport struct {
	Schema         string             `json:"schema"`
	Synthetic      bool               `json:"synthetic"`
	Repository     string             `json:"repository"`
	CommitSHA      string             `json:"commit_sha"`
	WorkflowDigest string             `json:"workflow_digest"`
	RunID          string             `json:"run_id"`
	Qualified      bool               `json:"qualified"`
	Rows           []AcceptanceRunRow `json:"rows"`
}

func (r AcceptanceRunReport) Validate(context PromotionValidationContext) error {
	if r.Schema != AcceptanceRunReportSchema || r.Synthetic || !r.Qualified {
		return errors.New("acceptance report must be a qualified non-synthetic production run")
	}
	if r.Repository != context.Repository || r.CommitSHA != contextCommit(context) ||
		r.WorkflowDigest != context.WorkflowDigest || r.RunID != context.RunID {
		return errors.New("acceptance report does not match trusted evaluated candidate")
	}
	rows := PromotionRows()
	if len(r.Rows) != len(rows) {
		return errors.New("acceptance report must contain every promotion row exactly once")
	}
	seen := map[string]bool{}
	for i, got := range r.Rows {
		want := rows[i]
		if seen[got.ID] || got.ID != want.ID || got.Package != "./internal/addyacceptance" ||
			got.OwningTest != want.OwningTest || got.Result != PromotionPassed ||
			!validAuthorityDigest(got.EvidenceSHA256) {
			return fmt.Errorf("acceptance report row %d has invalid identity, owner, result, or evidence", i+1)
		}
		seen[got.ID] = true
	}
	return nil
}

func (r AcceptanceRunReport) CanonicalJSON(context PromotionValidationContext) ([]byte, error) {
	if err := r.Validate(context); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

type ProductionPromotionInputs struct {
	Acceptance            AcceptanceRunReport
	Qualification         ProductionQualification
	GovernanceEvaluation  governancedrift.Evaluation
	GovernanceDecision    governancedrift.GateDecision
	WorkflowBlobSHA       string
	DisposableHarnessRoot string
}

type ProductionQualification struct {
	Synthetic              bool      `json:"synthetic"`
	Repository             string    `json:"repository"`
	Workflow               string    `json:"workflow"`
	WorkflowDigest         string    `json:"workflow_digest"`
	RunID                  string    `json:"run_id"`
	Commit                 string    `json:"commit"`
	CollectedAt            time.Time `json:"collected_at"`
	PackySHA               string    `json:"packy_sha"`
	PackyExecutableDigest  string    `json:"packy_executable_sha256"`
	RequestedClaudeVersion string    `json:"requested_claude_version"`
	ResolvedClaudeVersion  string    `json:"resolved_claude_version"`
	ClaudeIntegrity        string    `json:"claude_npm_integrity"`
	ClaudeDigest           string    `json:"claude_executable_sha256"`
	AtomicityMaterial      any       `json:"atomicity_material"`
}

// BuildProductionPromotionEvidence owns admission policy for all independent
// production authorities and produces the exact aggregate evidence.
func BuildProductionPromotionEvidence(context PromotionValidationContext, in ProductionPromotionInputs) (PromotionEvidence, error) {
	if err := in.Acceptance.Validate(context); err != nil {
		return PromotionEvidence{}, err
	}
	q := in.Qualification
	if q.Synthetic || q.Repository != context.Repository || q.Workflow != context.Workflow ||
		q.WorkflowDigest != context.WorkflowDigest || q.RunID != context.RunID ||
		q.Commit != contextCommit(context) || q.PackySHA != contextCommit(context) ||
		q.RequestedClaudeVersion != "2.1.203" || q.ResolvedClaudeVersion != "2.1.203" ||
		!validAuthorityDigest(q.PackyExecutableDigest) || !validAuthorityDigest(q.ClaudeDigest) ||
		strings.TrimSpace(q.ClaudeIntegrity) == "" || q.AtomicityMaterial == nil {
		return PromotionEvidence{}, errors.New("qualification does not match exact trusted run, commit, workflow, and Claude floor")
	}
	if q.CollectedAt.After(context.Now) || context.Now.Sub(q.CollectedAt) > 24*time.Hour {
		return PromotionEvidence{}, errors.New("qualification evidence is stale or future-dated")
	}
	e, g := in.GovernanceEvaluation, in.GovernanceDecision
	if e.State != governancedrift.StateClean || len(e.Findings) != 0 || !g.Allowed || len(g.Reasons) != 0 {
		return PromotionEvidence{}, errors.New("governance evidence is dirty or gate decision is disallowed")
	}
	i := e.Identity
	if i.Repository != context.Repository || i.Ref != "refs/heads/main" ||
		i.CommitSHA != contextCommit(context) || i.WorkflowSHA != in.WorkflowBlobSHA {
		return PromotionEvidence{}, errors.New("governance identity does not match protected evaluated candidate")
	}
	if i.CollectedAt.After(context.Now) || context.Now.Sub(i.CollectedAt) > time.Hour {
		return PromotionEvidence{}, errors.New("governance evidence is stale or future-dated")
	}
	acceptanceSHA256, err := canonicalAuthorityDigest(in.Acceptance)
	if err != nil {
		return PromotionEvidence{}, err
	}
	qualificationSHA256, err := canonicalAuthorityDigest(in.Qualification)
	if err != nil {
		return PromotionEvidence{}, err
	}
	governanceSHA256, err := canonicalAuthorityDigest(struct {
		Evaluation governancedrift.Evaluation   `json:"evaluation"`
		Decision   governancedrift.GateDecision `json:"decision"`
	}{e, g})
	if err != nil {
		return PromotionEvidence{}, err
	}
	atomicitySHA256, err := canonicalAuthorityDigest(q.AtomicityMaterial)
	if err != nil {
		return PromotionEvidence{}, err
	}
	prepublication := digestBytes([]byte(acceptanceSHA256 + governanceSHA256))
	authority, err := NewProductionPromotionAuthority(context, in.Acceptance.Rows, acceptanceSHA256, qualificationSHA256, governanceSHA256, prepublication)
	if err != nil {
		return PromotionEvidence{}, err
	}
	report, err := (PromotionHarness{
		Root: in.DisposableHarnessRoot, Context: context, Mode: PromotionHarnessExactCandidate,
		Evaluate: ProductionPromotionRowEvaluator(authority),
	}).Run()
	if err != nil {
		return PromotionEvidence{}, err
	}
	claudeIdentities := []string{
		"version:" + q.ResolvedClaudeVersion,
		"npm-integrity:" + q.ClaudeIntegrity,
		"executable-sha256:" + q.ClaudeDigest,
	}
	sort.Strings(claudeIdentities)
	return report.BuildAggregate(context, PromotionAggregateCandidate{
		PackageCandidate: q.PackyExecutableDigest,
		ClaudeIdentities: claudeIdentities,
		AtomicitySHA256:  atomicitySHA256,
	})
}

func canonicalAuthorityDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func NewAcceptanceRunReport(repository, commit, workflowDigest, runID string, rows []AcceptanceRunRow) AcceptanceRunReport {
	return AcceptanceRunReport{
		Schema: AcceptanceRunReportSchema, Synthetic: false, Repository: strings.TrimSpace(repository),
		CommitSHA: commit, WorkflowDigest: workflowDigest, RunID: strings.TrimSpace(runID),
		Qualified: true, Rows: append([]AcceptanceRunRow(nil), rows...),
	}
}
