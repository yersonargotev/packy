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
	AcceptanceSHA256      string
	Qualification         ProductionQualification
	QualificationSHA256   string
	GovernanceEvaluation  governancedrift.Evaluation
	GovernanceDecision    governancedrift.GateDecision
	GovernanceSHA256      string
	WorkflowBlobSHA       string
	DisposableHarnessRoot string
}

type ProductionQualification struct {
	Synthetic              bool
	Repository             string
	Workflow               string
	WorkflowDigest         string
	RunID                  string
	Commit                 string
	CollectedAt            time.Time
	PackySHA               string
	PackyExecutableDigest  string
	RequestedClaudeVersion string
	ResolvedClaudeVersion  string
	ClaudeIntegrity        string
	ClaudeDigest           string
	AtomicitySHA256        string
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
		strings.TrimSpace(q.ClaudeIntegrity) == "" || !validAuthorityDigest(q.AtomicitySHA256) {
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
	for name, value := range map[string]string{
		"acceptance": in.AcceptanceSHA256, "qualification": in.QualificationSHA256,
		"governance": in.GovernanceSHA256,
	} {
		if !validAuthorityDigest(value) {
			return PromotionEvidence{}, fmt.Errorf("%s evidence digest is invalid", name)
		}
	}
	prepublication := digestBytes([]byte(in.AcceptanceSHA256 + in.GovernanceSHA256))
	authority, err := NewProductionPromotionAuthority(context, in.Acceptance.Rows, in.AcceptanceSHA256, in.QualificationSHA256, in.GovernanceSHA256, prepublication)
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
		AtomicitySHA256:  q.AtomicitySHA256,
	})
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
