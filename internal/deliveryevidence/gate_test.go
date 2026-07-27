package deliveryevidence_test

import (
	"strings"
	"testing"

	de "github.com/yersonargotev/packy/internal/deliveryevidence"
)

const (
	base  = "1111111111111111111111111111111111111111"
	head1 = "2222222222222222222222222222222222222222"
	head2 = "3333333333333333333333333333333333333333"
	tree1 = "4444444444444444444444444444444444444444"
	tree2 = "5555555555555555555555555555555555555555"
)

func gateFixture(t *testing.T, multi bool) (de.Bundle, de.LocalGateObservation) {
	t.Helper()
	repo := de.RepositoryIdentity{Owner: "owner", Name: "repo", NodeID: "R_1"}
	issue := de.IssueIdentity{Number: 279, NodeID: "I_279"}
	spec := de.SpecIdentity{Number: 275, NodeID: "I_275"}
	row := de.AcceptanceRow{Identity: "AC-01", Criterion: "gate", OwningSeam: "delivery evidence", PositiveEvidence: "passes", NegativeEvidence: "foreign fails", FailureEvidence: "typed error", MutationEvidence: "read only", CompatibilityEvidence: "bundle v1", PreservationEvidence: "receipts", MigrationEvidence: "N/A additive", State: de.AcceptanceProved}
	b := de.Bundle{Schema: de.SchemaV1, Repository: repo, Issue: issue, Spec: spec, Authority: de.Authority{IssueSHA256: strings.Repeat("a", 64), SpecSHA256: strings.Repeat("b", 64), Labels: []string{"status:approved"}, DependencyDisposition: []de.DependencyDisposition{}, AcceptanceCriteria: []string{"AC-01"}}, Scope: de.ScopeLedger{OwnedNow: []de.LedgerEntry{}, Deferred: []de.DeferredEntry{}, Forbidden: []de.LedgerEntry{}, Prerequisites: []de.PrerequisiteEntry{}}, AcceptanceMatrix: []de.AcceptanceRow{row}, StartingBaseSHA: base, Iterations: []de.Iteration{{Sequence: 1, Identity: "iteration-1", BaseSHA: base, HeadSHA: head1, EvidenceSHA256: strings.Repeat("c", 64)}}, ReviewReceipts: []de.ReviewReceipt{{IssueNumber: 279, Iteration: "iteration-1", BaseSHA: base, HeadSHA: head1, Axis: de.ReviewStandards, Findings: []de.ReviewFinding{}}, {IssueNumber: 279, Iteration: "iteration-1", BaseSHA: base, HeadSHA: head1, Axis: de.ReviewSpec, Findings: []de.ReviewFinding{}}}, Adjudications: []de.Adjudication{}, ValidationReceipts: []de.ValidationReceipt{}, FocusedValidation: []de.FocusedValidationEvidence{}}
	finalHead, finalTree := head1, tree1
	commits := []string{base, head1}
	if multi {
		b.Iterations = append(b.Iterations, de.Iteration{Sequence: 2, Identity: "iteration-2", BaseSHA: head1, HeadSHA: head2, EvidenceSHA256: strings.Repeat("d", 64)})
		b.ReviewReceipts = append(b.ReviewReceipts, de.ReviewReceipt{IssueNumber: 279, Iteration: "iteration-2", BaseSHA: head1, HeadSHA: head2, Axis: de.ReviewStandards, Findings: []de.ReviewFinding{}}, de.ReviewReceipt{IssueNumber: 279, Iteration: "iteration-2", BaseSHA: head1, HeadSHA: head2, Axis: de.ReviewSpec, Findings: []de.ReviewFinding{}})
		finalHead, finalTree, commits = head2, tree2, []string{base, head1, head2}
	}
	validation := de.ValidationObservation{Repository: repo, CheckoutSHA256: strings.Repeat("e", 64), CommitSHA: finalHead, TreeSHA: finalTree, WorkspaceClean: true, ValidatorIdentity: "validate-packy-v1", ValidatorSHA256: strings.Repeat("f", 64), ValidatorIdentityExpiresAt: "2030-01-01T00:00:00Z", RequiredCommand: "./scripts/validate-packy.sh", Sandbox: de.SandboxFacts{HomeRoot: "/tmp/home", ConfigHomeRoot: "/tmp/config", Sandboxed: true}}
	b.ValidationReceipts = append(b.ValidationReceipts, de.ValidationReceipt{Schema: de.ValidationReceiptV1, ValidationObservation: validation, CompletedAt: "2026-07-27T12:00:00Z", Succeeded: true, Completed: true})
	o := de.LocalGateObservation{Repository: repo, Issue: issue, Spec: spec, IssueSHA256: b.Authority.IssueSHA256, SpecSHA256: b.Authority.SpecSHA256, IssueEligible: true, SpecEligible: true, ExpectedBranch: "feat/issue-279", CurrentBranch: "feat/issue-279", HeadSHA: finalHead, TreeSHA: finalTree, OrderedCommits: commits, Validation: validation, ObservedAt: "2026-07-27T12:01:00Z"}
	if err := de.Validate(b); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	return b, o
}

func TestEvaluateLocalGatePassesSingleAndMultiIteration(t *testing.T) {
	for _, multi := range []bool{false, true} {
		b, o := gateFixture(t, multi)
		r, err := de.EvaluateLocalGate(b, o)
		if err != nil {
			t.Fatalf("multi=%v: %v", multi, err)
		}
		if r.Iterations != len(b.Iterations) || r.Reviews != len(b.ReviewReceipts) || r.BundleSHA256 == "" {
			t.Fatalf("incomplete report: %+v", r)
		}
		if got, again := de.RenderLocalGateReport(r), de.RenderLocalGateReport(r); got != again || !strings.Contains(got, "Bundle SHA-256:") || !strings.Contains(got, "Repository: owner/repo") {
			t.Fatalf("non-canonical report: %q", got)
		}
	}
}

func TestEvaluateLocalGateFailureClasses(t *testing.T) {
	tests := []struct {
		name   string
		code   de.LocalGateErrorCode
		mutate func(*de.Bundle, *de.LocalGateObservation)
	}{
		{"foreign", de.LocalGateForeignEvidence, func(_ *de.Bundle, o *de.LocalGateObservation) { o.Issue.NodeID = "foreign" }},
		{"authority", de.LocalGateTrackerAuthorityChanged, func(_ *de.Bundle, o *de.LocalGateObservation) { o.IssueSHA256 = strings.Repeat("0", 64) }},
		{"qualification", de.LocalGateQualificationInvalid, func(b *de.Bundle, _ *de.LocalGateObservation) { b.Schema = "bad" }},
		{"acceptance", de.LocalGateAcceptanceUnproved, func(b *de.Bundle, _ *de.LocalGateObservation) { b.AcceptanceMatrix[0].State = de.AcceptanceImplemented }},
		{"review gap", de.LocalGateReviewGap, func(b *de.Bundle, _ *de.LocalGateObservation) { b.ReviewReceipts = b.ReviewReceipts[:1] }},
		{"stale validation", de.LocalGateStaleValidation, func(_ *de.Bundle, o *de.LocalGateObservation) { o.Validation.ValidatorSHA256 = strings.Repeat("0", 64) }},
		{"dirty", de.LocalGateDirtyWorkspace, func(_ *de.Bundle, o *de.LocalGateObservation) { o.Validation.WorkspaceClean = false }},
		{"wrong branch", de.LocalGateWrongBranch, func(_ *de.Bundle, o *de.LocalGateObservation) { o.CurrentBranch = "main" }},
		{"unrecorded", de.LocalGateUnrecordedDelta, func(_ *de.Bundle, o *de.LocalGateObservation) {
			o.HeadSHA = head2
			o.Validation.CommitSHA = head2
			o.OrderedCommits = append(o.OrderedCommits, head2)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, o := gateFixture(t, false)
			tt.mutate(&b, &o)
			_, err := de.EvaluateLocalGate(b, o)
			if !de.IsLocalGateError(err, tt.code) {
				t.Fatalf("want %s, got %v", tt.code, err)
			}
		})
	}
}

func TestEvaluateLocalGateRejectsUnresolvedFinding(t *testing.T) {
	b, o := gateFixture(t, true)
	finding := de.ReviewFinding{ID: "F-1", Axis: de.ReviewStandards, Severity: de.SeverityP1, Authority: de.AuthorityDocumentedStandard, Citation: "standard", Location: "gate.go", Evidence: "needs repair"}
	b.ReviewReceipts[0].Findings = []de.ReviewFinding{finding}
	b.Adjudications = []de.Adjudication{{Sequence: 1, FindingID: "F-1", Disposition: de.DispositionAccepted, Evidence: "repair next", RepairIteration: "iteration-2"}}
	if err := de.Validate(b); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	_, err := de.EvaluateLocalGate(b, o)
	if !de.IsLocalGateError(err, de.LocalGateUnresolvedFindings) {
		t.Fatalf("got %v", err)
	}
}
