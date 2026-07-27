package deliveryevidence

import (
	"reflect"
	"strings"
	"testing"
)

func reviewFixture() Bundle {
	b := fixture()
	first := Iteration{Sequence: 1, Identity: "iteration-1", BaseSHA: b.StartingBaseSHA, HeadSHA: strings.Repeat("d", 40), EvidenceSHA256: strings.Repeat("1", 64)}
	second := Iteration{Sequence: 2, Identity: "iteration-2", BaseSHA: first.HeadSHA, HeadSHA: strings.Repeat("e", 40), EvidenceSHA256: strings.Repeat("2", 64)}
	b.Iterations = []Iteration{first, second}
	return b
}

func receipt(b Bundle, iteration string, axis ReviewAxis, findings ...ReviewFinding) ReviewReceipt {
	var it Iteration
	for _, candidate := range b.Iterations {
		if candidate.Identity == iteration {
			it = candidate
		}
	}
	if findings == nil {
		findings = []ReviewFinding{}
	}
	return ReviewReceipt{IssueNumber: b.Issue.Number, Iteration: iteration, BaseSHA: it.BaseSHA, HeadSHA: it.HeadSHA, Axis: axis, Findings: findings}
}

func finding(id string, axis ReviewAxis) ReviewFinding {
	authority := AuthoritySpecRequirement
	if axis == ReviewStandards {
		authority = AuthorityDocumentedStandard
	}
	return ReviewFinding{ID: id, Axis: axis, Severity: SeverityP2, Authority: authority, Citation: "docs/authority.md#rule", Location: "internal/example.go:12", Evidence: "the delta contradicts the cited rule"}
}

func TestCleanPairedReviewsCoverIterationDeltas(t *testing.T) {
	b := reviewFixture()
	for _, it := range b.Iterations {
		b.ReviewReceipts = append(b.ReviewReceipts, receipt(b, it.Identity, ReviewStandards), receipt(b, it.Identity, ReviewSpec))
	}
	middle := strings.Repeat("f", 40)
	commits := []string{b.StartingBaseSHA, middle, b.Iterations[0].HeadSHA, b.Iterations[1].HeadSHA}
	status, err := QueryReviews(b, b.Iterations[1].HeadSHA, commits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(status, ReviewStatus{}) {
		t.Fatalf("unexpected gaps: %#v", status)
	}
	report, err := RenderReviewReport(b, b.Iterations[1].HeadSHA, commits)
	if err != nil || !strings.Contains(report, "Uncovered commits: none") ||
		!strings.Contains(report, "Receipt: iteration=iteration-1 axis=spec base="+b.StartingBaseSHA+" head="+b.Iterations[0].HeadSHA) {
		t.Fatalf("report: %q %v", report, err)
	}
}

func TestAcceptedFindingRequiresPairedReviewOfRepair(t *testing.T) {
	b := reviewFixture()
	f := finding("standards-stable-id", ReviewStandards)
	b.ReviewReceipts = []ReviewReceipt{
		receipt(b, "iteration-1", ReviewStandards, f),
		receipt(b, "iteration-1", ReviewSpec),
	}
	b.Adjudications = []Adjudication{{Sequence: 1, FindingID: f.ID, Disposition: DispositionAccepted, Evidence: "maintainer accepted the cited defect", RepairIteration: "iteration-2"}}
	if err := Validate(b); err != nil {
		t.Fatalf("accepted finding should remain unresolved while repair reviews are pending: %v", err)
	}
	b.Adjudications = append(b.Adjudications, Adjudication{Sequence: 2, FindingID: f.ID, Disposition: DispositionRepairedByLaterIteration, Evidence: "the later delta repairs the cited location", RepairIteration: "iteration-2"})
	if err := Validate(b); err == nil {
		t.Fatal("repair resolved without paired reviews")
	}
	b.ReviewReceipts = append(b.ReviewReceipts, receipt(b, "iteration-2", ReviewStandards), receipt(b, "iteration-2", ReviewSpec))
	if err := Validate(b); err != nil {
		t.Fatal(err)
	}
	status, err := QueryReviews(b, b.Iterations[1].HeadSHA, []string{b.StartingBaseSHA, b.Iterations[0].HeadSHA, b.Iterations[1].HeadSHA})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.UnresolvedFindings) != 0 {
		t.Fatalf("repaired finding remains unresolved: %#v", status)
	}
	report, err := RenderReviewReport(b, b.Iterations[1].HeadSHA, []string{b.StartingBaseSHA, b.Iterations[0].HeadSHA, b.Iterations[1].HeadSHA})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Finding: id=standards-stable-id axis=standards severity=P2 authority=documented-standard citation=docs/authority.md#rule location=internal/example.go:12 evidence=the delta contradicts the cited rule",
		"Adjudication: sequence=1 finding=standards-stable-id disposition=accepted evidence=maintainer accepted the cited defect repair_iteration=iteration-2",
		"Adjudication: sequence=2 finding=standards-stable-id disposition=repaired-by-later-iteration evidence=the later delta repairs the cited location repair_iteration=iteration-2",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRejectedFindingIsResolvedWithDurableEvidence(t *testing.T) {
	b := reviewFixture()
	f := finding("spec-stable-id", ReviewSpec)
	b.ReviewReceipts = []ReviewReceipt{receipt(b, "iteration-1", ReviewStandards), receipt(b, "iteration-1", ReviewSpec, f)}
	b.Adjudications = []Adjudication{{Sequence: 1, FindingID: f.ID, Disposition: DispositionRejectedWithEvidence, Evidence: "the cited requirement applies only to another seam"}}
	status, err := QueryReviews(b, b.Iterations[1].HeadSHA, []string{b.StartingBaseSHA, b.Iterations[0].HeadSHA, b.Iterations[1].HeadSHA})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.UnresolvedFindings) != 0 || !reflect.DeepEqual(status.UnreviewedDeltas, []string{"iteration-2"}) {
		t.Fatalf("unexpected review status: %#v", status)
	}
}

func TestReviewValidationRejectsStaleCrossIssueDuplicateAndInvalidEvidence(t *testing.T) {
	tests := map[string]func(*Bundle){
		"stale head": func(b *Bundle) {
			b.ReviewReceipts = []ReviewReceipt{receipt(*b, "iteration-1", ReviewStandards)}
			b.ReviewReceipts[0].HeadSHA = b.Iterations[1].HeadSHA
		},
		"cross issue": func(b *Bundle) {
			b.ReviewReceipts = []ReviewReceipt{receipt(*b, "iteration-1", ReviewStandards)}
			b.ReviewReceipts[0].IssueNumber++
		},
		"duplicate axis receipt": func(b *Bundle) {
			r := receipt(*b, "iteration-1", ReviewStandards)
			b.ReviewReceipts = []ReviewReceipt{r, r}
		},
		"duplicate finding": func(b *Bundle) {
			f := finding("same-id", ReviewStandards)
			b.ReviewReceipts = []ReviewReceipt{receipt(*b, "iteration-1", ReviewStandards, f), receipt(*b, "iteration-2", ReviewStandards, f)}
		},
		"missing axis": func(b *Bundle) {
			f := finding("missing-axis", ReviewStandards)
			f.Axis = ""
			b.ReviewReceipts = []ReviewReceipt{receipt(*b, "iteration-1", ReviewStandards, f)}
		},
		"invalid authority": func(b *Bundle) {
			f := finding("bad-authority", ReviewStandards)
			f.Authority = AuthoritySpecRequirement
			b.ReviewReceipts = []ReviewReceipt{receipt(*b, "iteration-1", ReviewStandards, f)}
		},
		"invalid disposition": func(b *Bundle) {
			f := finding("bad-disposition", ReviewSpec)
			b.ReviewReceipts = []ReviewReceipt{receipt(*b, "iteration-1", ReviewSpec, f)}
			b.Adjudications = []Adjudication{{Sequence: 1, FindingID: f.ID, Disposition: "waived", Evidence: "not an allowed outcome"}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			b := reviewFixture()
			mutate(&b)
			if err := Validate(b); err == nil {
				t.Fatal("invalid review evidence accepted")
			}
		})
	}
}

func TestRecordOperationsPreserveAppendOnlyHistory(t *testing.T) {
	b := fixture()
	it := Iteration{Sequence: 1, Identity: "iteration-1", BaseSHA: b.StartingBaseSHA, HeadSHA: strings.Repeat("d", 40), EvidenceSHA256: strings.Repeat("1", 64)}
	var err error
	if b, err = RecordIteration(b, it); err != nil {
		t.Fatal(err)
	}
	f := finding("finding-1", ReviewStandards)
	scoped := Adjudication{Sequence: 1, FindingID: f.ID, Disposition: DispositionScoped, Evidence: "a separately approved issue owns this location", ScopeIdentity: "D1"}
	if b, err = RecordReview(b, receipt(b, it.Identity, ReviewStandards, f), scoped); err != nil {
		t.Fatal(err)
	}
	if _, err = RecordReview(b, receipt(b, it.Identity, ReviewStandards)); err == nil {
		t.Fatal("duplicate review receipt appended")
	}
}

func TestAcceptedFindingCanNameFutureRepairIteration(t *testing.T) {
	b := fixture()
	first := Iteration{Sequence: 1, Identity: "iteration-1", BaseSHA: b.StartingBaseSHA, HeadSHA: strings.Repeat("d", 40), EvidenceSHA256: strings.Repeat("1", 64)}
	var err error
	if b, err = RecordIteration(b, first); err != nil {
		t.Fatal(err)
	}
	f := finding("future-repair", ReviewStandards)
	accepted := Adjudication{Sequence: 1, FindingID: f.ID, Disposition: DispositionAccepted, Evidence: "repair is required in the next iteration", RepairIteration: "iteration-2"}
	if b, err = RecordReview(b, receipt(b, first.Identity, ReviewStandards, f), accepted); err != nil {
		t.Fatalf("accepted finding could not name a future repair iteration: %v", err)
	}
	second := Iteration{Sequence: 2, Identity: "iteration-2", BaseSHA: first.HeadSHA, HeadSHA: strings.Repeat("e", 40), EvidenceSHA256: strings.Repeat("2", 64)}
	if b, err = RecordIteration(b, second); err != nil {
		t.Fatal(err)
	}
	if b, err = RecordReview(b, receipt(b, second.Identity, ReviewStandards)); err != nil {
		t.Fatal(err)
	}
	if b, err = RecordReview(b, receipt(b, second.Identity, ReviewSpec)); err != nil {
		t.Fatal(err)
	}
	repaired := Adjudication{Sequence: 2, FindingID: f.ID, Disposition: DispositionRepairedByLaterIteration, Evidence: "paired reviews verify the repair", RepairIteration: second.Identity}
	if _, err = RecordAdjudication(b, repaired); err != nil {
		t.Fatal(err)
	}
}

func TestFindingRequiresAtomicInitialAdjudication(t *testing.T) {
	b := reviewFixture()
	f := finding("needs-disposition", ReviewSpec)
	r := receipt(b, "iteration-1", ReviewSpec, f)
	if _, err := RecordReview(b, r); err == nil {
		t.Fatal("finding without disposition was accepted")
	}
	rejected := Adjudication{Sequence: 1, FindingID: f.ID, Disposition: DispositionRejectedWithEvidence, Evidence: "the requirement applies to another seam"}
	if _, err := RecordReview(b, r, rejected); err != nil {
		t.Fatalf("atomic receipt and adjudication failed: %v", err)
	}
}

func TestScopedDispositionUsesOnlyPrequalifiedScope(t *testing.T) {
	for name, identity := range map[string]string{
		"missing":   "",
		"unknown":   "not-qualified",
		"owned-now": "O1",
	} {
		t.Run(name, func(t *testing.T) {
			b := reviewFixture()
			f := finding("scoped-"+name, ReviewStandards)
			event := Adjudication{Sequence: 1, FindingID: f.ID, Disposition: DispositionScoped, Evidence: "the qualified scope ledger owns this work", ScopeIdentity: identity}
			if _, err := RecordReview(b, receipt(b, "iteration-1", ReviewStandards, f), event); err == nil {
				t.Fatal("invalid scope authority accepted")
			}
		})
	}
	for _, identity := range []string{"D1", "F1"} {
		t.Run(identity, func(t *testing.T) {
			b := reviewFixture()
			f := finding("scoped-"+identity, ReviewStandards)
			event := Adjudication{Sequence: 1, FindingID: f.ID, Disposition: DispositionScoped, Evidence: "the qualified scope ledger owns this work", ScopeIdentity: identity}
			if _, err := RecordReview(b, receipt(b, "iteration-1", ReviewStandards, f), event); err != nil {
				t.Fatalf("pre-qualified scope rejected: %v", err)
			}
		})
	}
}
