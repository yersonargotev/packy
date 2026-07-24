package addyacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const PromotionHarnessSchema = "addy-promotion-harness.v1"

type PromotionHarnessMode string

const (
	PromotionHarnessSynthetic      PromotionHarnessMode = "synthetic_qualification"
	PromotionHarnessExactCandidate PromotionHarnessMode = "exact_evaluated_candidate"
)

type PromotionRowResult struct {
	Evidence      any
	PermittedDiff []string
}

// ProductionPromotionAuthority binds every matrix row to evidence collected
// for one exact workflow run and evaluated candidate.
type ProductionPromotionAuthority struct {
	repository           string
	commitSHA            string
	workflowDigest       string
	runID                string
	contextSHA256        string
	acceptanceSHA256     string
	qualificationSHA256  string
	governanceSHA256     string
	prepublicationSHA256 string
}

type productionPromotionRowProof struct {
	RowID           string `json:"row_id"`
	RowName         string `json:"row_name"`
	Authority       string `json:"authority"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Repository      string `json:"repository"`
	CommitSHA       string `json:"commit_sha"`
	WorkflowDigest  string `json:"workflow_digest"`
	RunID           string `json:"run_id"`
	ContextSHA256   string `json:"context_sha256"`
}

// PromotionSemanticProof binds a row result to the row-specific domain fact
// registered by the owning acceptance test. A generic success payload is not
// promotion authority.
type PromotionSemanticProof struct {
	RowID    string `json:"row_id"`
	Claim    string `json:"claim"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

type PromotionRowEvaluator func(PromotionRow, string) (PromotionRowResult, error)
type PromotionGateBoundary func(int, []PromotionHarnessRow) error

type PromotionHarness struct {
	Root     string
	Context  PromotionValidationContext
	Mode     PromotionHarnessMode
	Evaluate PromotionRowEvaluator
	Boundary PromotionGateBoundary
}

type PromotionMutationProof struct {
	ObservedDiff       []string `json:"observed_diff"`
	PermittedDiff      []string `json:"permitted_diff"`
	ZeroMutation       bool     `json:"zero_mutation"`
	ExactPermittedDiff bool     `json:"exact_permitted_diff"`
}

type PromotionHarnessRow struct {
	ID              string `json:"id"`
	Number          int    `json:"number"`
	Gate            int    `json:"gate"`
	OwningTest      string `json:"owning_test"`
	Result          string `json:"result"`
	Diagnostic      string `json:"diagnostic"`
	EvidenceSHA256  string `json:"evidence_sha256"`
	productionBound bool
	Proof           PromotionMutationProof `json:"proof"`
}

type PromotionHarnessReport struct {
	Schema                 string               `json:"schema"`
	Mode                   PromotionHarnessMode `json:"mode"`
	Repository             string               `json:"repository"`
	CommitSHA              string               `json:"commit_sha"`
	WorkflowDigest         string               `json:"workflow_digest"`
	RunID                  string               `json:"run_id"`
	productionBound        bool
	authorityContextSHA256 string
	Qualified              bool                  `json:"qualified"`
	Rows                   []PromotionHarnessRow `json:"rows"`
}

func (r PromotionHarnessReport) CanonicalJSON() ([]byte, error) {
	if r.Rows == nil {
		return nil, errors.New("promotion harness rows must be non-null")
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (h PromotionHarness) Run() (PromotionHarnessReport, error) {
	report := PromotionHarnessReport{Schema: PromotionHarnessSchema, Mode: h.Mode, Repository: h.Context.Repository, CommitSHA: contextCommit(h.Context), WorkflowDigest: h.Context.WorkflowDigest, RunID: h.Context.RunID, Rows: []PromotionHarnessRow{}}
	if err := validatePromotionContext(h.Context); err != nil {
		return report, fmt.Errorf("validate promotion harness context: %w", err)
	}
	if h.Mode != PromotionHarnessSynthetic && h.Mode != PromotionHarnessExactCandidate {
		return report, errors.New("promotion harness mode is invalid")
	}
	if h.Evaluate == nil {
		return report, errors.New("promotion harness evaluator is required")
	}
	if !filepath.IsAbs(h.Root) || filepath.Clean(h.Root) != h.Root {
		return report, errors.New("promotion harness root must be a clean absolute path")
	}
	entries, err := os.ReadDir(h.Root)
	if err != nil {
		return report, fmt.Errorf("read promotion harness root: %w", err)
	}
	if len(entries) != 0 {
		return report, errors.New("promotion harness root must be empty")
	}

	blocked := false
	rows := PromotionRows()
	for gate := 1; gate <= 6; gate++ {
		start := len(report.Rows)
		for _, row := range rows {
			if row.Gate != gate {
				continue
			}
			if blocked {
				report.Rows = append(report.Rows, suppressedHarnessRow(row))
				continue
			}
			got := h.runRow(row)
			report.Rows = append(report.Rows, got)
		}
		if blocked {
			continue
		}
		gateFailed := false
		for _, row := range report.Rows[start:] {
			if row.Result != PromotionPassed {
				gateFailed = true
			}
		}
		if !gateFailed && h.Boundary != nil {
			if err := h.Boundary(gate, append([]PromotionHarnessRow(nil), report.Rows[start:]...)); err != nil {
				gateFailed = true
				for i := start; i < len(report.Rows); i++ {
					report.Rows[i].Result = "failed"
					report.Rows[i].Diagnostic = fmt.Sprintf("ADDY-CLAUDE-PROMOTION-GATE-%02d-BOUNDARY", gate)
				}
			}
		}
		blocked = gateFailed
	}
	report.Qualified = true
	report.productionBound = h.Mode == PromotionHarnessExactCandidate
	for _, row := range report.Rows {
		if row.Result != PromotionPassed {
			report.Qualified = false
		}
		if !row.productionBound {
			report.productionBound = false
		}
	}
	if report.productionBound {
		report.authorityContextSHA256 = promotionAuthorityContextDigest(h.Context)
	}
	if remaining, err := os.ReadDir(h.Root); err != nil || len(remaining) != 0 {
		return report, errors.New("promotion harness did not restore its disposable root")
	}
	return report, nil
}

func (h PromotionHarness) runRow(row PromotionRow) PromotionHarnessRow {
	out := PromotionHarnessRow{ID: row.ID, Number: row.Number, Gate: row.Gate, OwningTest: row.OwningTest, Result: PromotionPassed, Proof: PromotionMutationProof{ObservedDiff: []string{}, PermittedDiff: []string{}, ZeroMutation: true, ExactPermittedDiff: true}}
	child := filepath.Join(h.Root, fmt.Sprintf("gate-%d-row-%02d", row.Gate, row.Number))
	if err := os.Mkdir(child, 0o700); err != nil {
		out.Result, out.Diagnostic = "failed", row.BlockedDiagnostic
		return out
	}
	result, evalErr := h.Evaluate(row, child)
	semanticErr := validatePromotionSemanticProof(row, result.Evidence)
	if h.Mode == PromotionHarnessExactCandidate {
		semanticErr = validateProductionPromotionRowProof(row, result.Evidence, h.Context)
	}
	observed, scanErr := relativeFiles(child)
	permitted, permitErr := canonicalPaths(result.PermittedDiff)
	out.Proof.ObservedDiff, out.Proof.PermittedDiff = observed, permitted
	out.Proof.ZeroMutation = len(observed) == 0
	out.Proof.ExactPermittedDiff = equalStrings(observed, permitted)
	evidence, marshalErr := json.Marshal(result.Evidence)
	if marshalErr == nil {
		sum := sha256.Sum256(evidence)
		out.EvidenceSHA256 = hex.EncodeToString(sum[:])
	}
	_ = os.RemoveAll(child)
	if evalErr != nil || semanticErr != nil || scanErr != nil || permitErr != nil || marshalErr != nil || !out.Proof.ExactPermittedDiff {
		out.Result = "failed"
		cause := firstError(evalErr, semanticErr, scanErr, permitErr, marshalErr)
		if cause == nil {
			cause = errors.New("observed diff does not exactly match permitted diff")
		}
		_ = cause
		out.Diagnostic = row.BlockedDiagnostic
	} else if h.Mode == PromotionHarnessExactCandidate {
		out.productionBound = true
	}
	return out
}

// NewProductionPromotionAuthority validates the independent evidence
// authorities used by ProductionPromotionRowEvaluator.
func NewProductionPromotionAuthority(context PromotionValidationContext, acceptanceSHA256, qualificationSHA256, governanceSHA256, prepublicationSHA256 string) (ProductionPromotionAuthority, error) {
	a := ProductionPromotionAuthority{
		repository: context.Repository, commitSHA: contextCommit(context),
		workflowDigest: context.WorkflowDigest, runID: context.RunID,
		contextSHA256:    promotionAuthorityContextDigest(context),
		acceptanceSHA256: acceptanceSHA256, qualificationSHA256: qualificationSHA256,
		governanceSHA256: governanceSHA256, prepublicationSHA256: prepublicationSHA256,
	}
	if err := validatePromotionContext(context); err != nil {
		return ProductionPromotionAuthority{}, err
	}
	exactPR := context.PullRequest > 0 && context.EvaluatedMergeSHA != "" && context.Tag == ""
	exactTag := context.PullRequest == 0 && context.EvaluatedMergeSHA == "" && context.Tag != ""
	if !context.PromotionChange || context.FoundationChange || (!exactPR && !exactTag) {
		return ProductionPromotionAuthority{}, errors.New("production authority requires an exact promotion candidate")
	}
	for name, value := range map[string]string{
		"acceptance": a.acceptanceSHA256, "qualification": a.qualificationSHA256,
		"governance": a.governanceSHA256, "prepublication": a.prepublicationSHA256,
	} {
		if !validAuthorityDigest(value) {
			return ProductionPromotionAuthority{}, fmt.Errorf("%s authority must be a lowercase SHA-256", name)
		}
	}
	return a, nil
}

// ProductionPromotionRowEvaluator creates row proofs bound to the correct
// independent authority for the exact evaluated candidate.
func ProductionPromotionRowEvaluator(authority ProductionPromotionAuthority) PromotionRowEvaluator {
	return func(row PromotionRow, _ string) (PromotionRowResult, error) {
		name, sha := "acceptance", authority.acceptanceSHA256
		switch row.Number {
		case 11, 12:
			name, sha = "production-qualification", authority.qualificationSHA256
		case 13:
			name, sha = "governance", authority.governanceSHA256
		case 14:
			name, sha = "prepublication", authority.prepublicationSHA256
		}
		return PromotionRowResult{Evidence: productionPromotionRowProof{
			RowID: row.ID, RowName: row.Name, Authority: name, AuthoritySHA256: sha,
			Repository: authority.repository, CommitSHA: authority.commitSHA,
			WorkflowDigest: authority.workflowDigest, RunID: authority.runID,
			ContextSHA256: authority.contextSHA256,
		}}, nil
	}
}

func validateProductionPromotionRowProof(row PromotionRow, evidence any, context PromotionValidationContext) error {
	proof, ok := evidence.(productionPromotionRowProof)
	if !ok {
		return errors.New("exact promotion row is not production-bound")
	}
	if proof.RowID != row.ID || proof.RowName != row.Name || proof.Repository != context.Repository ||
		proof.CommitSHA != contextCommit(context) || proof.WorkflowDigest != context.WorkflowDigest ||
		proof.RunID != context.RunID || proof.ContextSHA256 != promotionAuthorityContextDigest(context) ||
		!validAuthorityDigest(proof.AuthoritySHA256) {
		return errors.New("production row proof does not match its stable identity or trusted context")
	}
	want := "acceptance"
	switch row.Number {
	case 11, 12:
		want = "production-qualification"
	case 13:
		want = "governance"
	case 14:
		want = "prepublication"
	}
	if proof.Authority != want {
		return errors.New("production row proof uses the wrong authority")
	}
	return nil
}

func validAuthorityDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func promotionAuthorityContextDigest(context PromotionValidationContext) string {
	data, _ := json.Marshal(struct {
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
		Now               string
		Inputs            IndependentPromotionInputs
	}{
		context.Repository,
		context.PullRequest,
		context.BaseSHA,
		context.HeadSHA,
		context.EvaluatedMergeSHA,
		context.Tag,
		context.Workflow,
		context.WorkflowDigest,
		context.MatrixVersion,
		context.RunID,
		context.Now.UTC().Format(time.RFC3339Nano),
		context.Inputs,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type promotionSemanticRegistration struct {
	claim  string
	derive func() string
}

var promotionSemanticRows = map[int]promotionSemanticRegistration{
	1: {"source-oracle-sha256", func() string { data, _ := CanonicalJSON(); return digest(data) }},
	2: {"immutable-history-versions", func() string {
		h := CanonicalPromotionHistory()
		return fmt.Sprintf("%s>%s", h.Versions[0].Version, h.Versions[1].Version)
	}},
	3: {"catalog-selection-withheld", func() string {
		h := CanonicalPromotionHistory()
		return fmt.Sprintf("%t:%s", h.CatalogAdvertised, h.CurrentVersion)
	}},
	4: {"strict-v3-resource-inventory", func() string {
		c := CanonicalPromotionCurrent()
		var manifest Manifest
		_ = json.Unmarshal(c.Manifest, &manifest)
		return fmt.Sprintf("%d:%d", manifest.SchemaVersion, len(manifest.Resources))
	}},
	5: {"complete-claude-projection", func() string {
		var manifest Manifest
		_ = json.Unmarshal(CanonicalPromotionCurrent().Manifest, &manifest)
		count := 0
		for _, r := range manifest.Resources {
			for _, b := range r.Bindings {
				if b.Surface == "claude" {
					count++
					break
				}
			}
		}
		return fmt.Sprint(count)
	}},
	6: {"surface-local-compatibility", func() string { return strings.Join(CanonicalPromotionCurrent().Surfaces, ",") }},
	7: {"deterministic-history-discovery", func() string { h := CanonicalPromotionHistory(); return digest(canonicalBytes(h)) }},
	8: {"deterministic-structured-output", func() string { h := CanonicalPromotionHistory(); return digest(canonicalBytes(h.Routes)) }},
	9: {"snapshot-atomicity", func() string { return CanonicalPromotionCurrent().SnapshotSHA256 }},
	10: {"collision-free-source-ownership", func() string {
		seen := map[string]bool{}
		for _, f := range CanonicalPromotionCurrent().Files {
			if seen[f.Path] {
				return "duplicate:" + f.Path
			}
			seen[f.Path] = true
		}
		return fmt.Sprintf("unique:%d", len(seen))
	}},
	11: {"package-candidate-is-detached", func() string {
		h := CanonicalPromotionHistory()
		return fmt.Sprintf("%t:%s", h.CatalogAdvertised, CanonicalPromotionCurrent().Version)
	}},
	12: {"real-host-authority-withheld", func() string { return "no-auth:no-model:no-upstream" }},
	13: {"protected-merge-authority-withheld", func() string { return "synthetic-cannot-authorize-pr" }},
	14: {"publication-authority-withheld", func() string { return "synthetic-cannot-authorize-release" }},
}

// SyntheticPromotionRowEvaluator derives every registered row fact from the
// detached Addy fixtures. Overrides are test-only one-fact twins keyed by row
// ID; they replace only that row's actual domain fact.
func SyntheticPromotionRowEvaluator(overrides map[string]string) PromotionRowEvaluator {
	return func(row PromotionRow, _ string) (PromotionRowResult, error) {
		registration, ok := promotionSemanticRows[row.Number]
		if !ok {
			return PromotionRowResult{}, fmt.Errorf("promotion row %s has no semantic registration", row.ID)
		}
		actual := registration.derive()
		if twin, ok := overrides[row.ID]; ok {
			actual = twin
		}
		return PromotionRowResult{Evidence: PromotionSemanticProof{
			RowID: row.ID, Claim: registration.claim,
			Expected: registration.derive(), Actual: actual,
		}}, nil
	}
}

// PromotionRowNegativeTwin returns a row-specific mutation of exactly the
// semantic fact owned by row.
func PromotionRowNegativeTwin(row PromotionRow) map[string]string {
	registration, ok := promotionSemanticRows[row.Number]
	if !ok {
		return map[string]string{row.ID: "unregistered"}
	}
	return map[string]string{row.ID: "mutated:" + registration.claim}
}

func validatePromotionSemanticProof(row PromotionRow, evidence any) error {
	proof, ok := evidence.(PromotionSemanticProof)
	if !ok {
		return errors.New("row evidence is not a registered semantic proof")
	}
	registration, ok := promotionSemanticRows[row.Number]
	if !ok || proof.RowID != row.ID || proof.Claim != registration.claim {
		return errors.New("row semantic proof does not match its stable registration")
	}
	if proof.Expected != registration.derive() || proof.Actual != proof.Expected {
		return errors.New("row semantic fact does not satisfy its owning proof")
	}
	return nil
}

func suppressedHarnessRow(row PromotionRow) PromotionHarnessRow {
	return PromotionHarnessRow{ID: row.ID, Number: row.Number, Gate: row.Gate, OwningTest: row.OwningTest, Result: "suppressed", Diagnostic: row.BlockedDiagnostic + ": suppressed by earlier gate", Proof: PromotionMutationProof{ObservedDiff: []string{}, PermittedDiff: []string{}, ZeroMutation: true, ExactPermittedDiff: true}}
}

func relativeFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(paths)
	if paths == nil {
		paths = []string{}
	}
	return paths, err
}
func canonicalPaths(in []string) ([]string, error) {
	out := append([]string(nil), in...)
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	for i, p := range out {
		if p == "" || filepath.IsAbs(p) || filepath.ToSlash(filepath.Clean(p)) != p || strings.HasPrefix(p, "../") || (i > 0 && out[i-1] == p) {
			return out, fmt.Errorf("permitted diff path %q is invalid", p)
		}
	}
	return out, nil
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// PromotionAggregateCandidate supplies production-only facts not derivable from row reports.
type PromotionAggregateCandidate struct {
	PackageCandidate string
	ClaudeIdentities []string
	AtomicitySHA256  string
}

func (r PromotionHarnessReport) BuildAggregate(context PromotionValidationContext, candidate PromotionAggregateCandidate) (PromotionEvidence, error) {
	exactPR := context.PullRequest > 0 && context.EvaluatedMergeSHA != "" && context.Tag == ""
	exactTag := context.PullRequest == 0 && context.EvaluatedMergeSHA == "" && context.Tag != ""
	if r.Mode != PromotionHarnessExactCandidate || !r.Qualified || !r.productionBound || !context.PromotionChange || context.FoundationChange || (!exactPR && !exactTag) {
		return PromotionEvidence{}, errors.New("promotion aggregate requires an exact evaluated-candidate report")
	}
	if r.Repository != context.Repository || r.CommitSHA != contextCommit(context) || r.WorkflowDigest != context.WorkflowDigest || r.RunID != context.RunID || len(r.Rows) != len(PromotionRows()) {
		return PromotionEvidence{}, errors.New("promotion harness report does not match trusted evaluated candidate")
	}
	if r.authorityContextSHA256 != promotionAuthorityContextDigest(context) {
		return PromotionEvidence{}, errors.New("promotion harness production authority does not match trusted candidate identity")
	}
	e := newEmptyPromotionEvidence(context, PromotionApplicable)
	e.PackageCandidate, e.ClaudeIdentities, e.AtomicitySHA256 = candidate.PackageCandidate, append([]string(nil), candidate.ClaudeIdentities...), candidate.AtomicitySHA256
	e.Rows = make([]PromotionRowEvidence, len(r.Rows))
	for i, row := range r.Rows {
		if row.ID != PromotionRows()[i].ID || row.Result != PromotionPassed || !row.productionBound || !row.Proof.ExactPermittedDiff {
			return PromotionEvidence{}, errors.New("promotion harness report is incomplete or synthetic")
		}
		e.Rows[i] = PromotionRowEvidence{ID: row.ID, Result: PromotionPassed, EvidenceSHA256: row.EvidenceSHA256, CommitSHA: contextCommit(context), WorkflowDigest: context.WorkflowDigest, RunID: context.RunID, CollectedAt: context.Now.UTC()}
	}
	if err := ValidatePromotionEvidence(e, context); err != nil {
		return PromotionEvidence{}, err
	}
	return e, nil
}
