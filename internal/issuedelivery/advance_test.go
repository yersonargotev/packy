package issuedelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type fakeGitObserver struct {
	mu    sync.Mutex
	value GitObservation
	calls int
}

func (f *fakeGitObserver) ObserveGit(_ context.Context, _ string) (GitObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.value, nil
}

type fakeGitHubObserver struct {
	mu    sync.Mutex
	value TrackerObservation
	calls int
	hook  func(int)
}

func (f *fakeGitHubObserver) ObserveIssue(_ context.Context, _ GitObservation, issue int) (TrackerObservation, error) {
	f.mu.Lock()
	f.calls++
	hook := f.hook
	value := f.value
	f.mu.Unlock()
	if hook != nil {
		hook(issue)
	}
	value.Issue.Number = issue
	return value, nil
}

type fakeClock struct {
	mu   sync.Mutex
	next time.Time
	step time.Duration
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	got := f.next
	f.next = f.next.Add(f.step)
	return got
}

func moduleFixture(t *testing.T, issue int) (*Module, *fakeGitObserver, *fakeGitHubObserver) {
	t.Helper()
	common := filepath.Join(t.TempDir(), "common")
	if err := os.MkdirAll(common, 0700); err != nil {
		t.Fatal(err)
	}
	git := &fakeGitObserver{value: GitObservation{
		CommonDir: common, Worktree: filepath.Join(filepath.Dir(common), "worktree"),
		OriginURL: "git@github.com:yersonargotev/packy.git", Owner: "yersonargotev", Name: "packy",
		StartingBaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40),
		TreeSHA: strings.Repeat("c", 40), WorkspaceClean: true, Branch: "chore/issue-356-test",
	}}
	tracker := &fakeGitHubObserver{value: TrackerObservation{
		Repository: deliveryevidence.RepositoryIdentity{Owner: "yersonargotev", Name: "packy", NodeID: "R1"},
		Issue:      deliveryevidence.IssueIdentity{Number: issue, NodeID: "I1"},
		Title:      "Qualify one low-risk run",
		Body:       "explicit authority",
		State:      "OPEN",
		Labels:     []string{"type:chore", "status:approved"},
		Criteria: []AuthorityItem{
			{Text: "Persist one self-contained low-risk run.", EvidenceLink: "issue#356:criterion-1"},
			{Text: "Resume without creating a duplicate run.", EvidenceLink: "issue#356:criterion-2"},
		},
		Exclusions:   []AuthorityItem{{Text: "Do not push a branch.", EvidenceLink: "issue#356:out-of-scope"}},
		Dependencies: []DependencyObservation{},
		References:   []ReferenceObservation{},
		Ambiguities:  []AuthorityItem{},
	}}
	clock := &fakeClock{next: time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC), step: time.Second}
	module, err := New(Config{
		Git: git, GitHub: tracker, Clock: clock, DeclaredProfile: deliveryevidence.RiskLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	return module, git, tracker
}

func TestAdvanceCreatesAndResumesSelfContainedLowRiskRun(t *testing.T) {
	module, git, tracker := moduleFixture(t, 356)
	request := Request{RepositoryPath: "/ignored/by-fake", IssueNumber: 356}

	first, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID == "" || first.State != StateNeedsReview || first.Evidence == nil {
		t.Fatalf("fresh outcome = %#v", first)
	}
	if first.Evidence.Schema != deliveryevidence.SchemaV2 ||
		first.Evidence.Authority.Kind != deliveryevidence.AuthoritySelfContainedIssue ||
		first.Evidence.RiskProfile != deliveryevidence.RiskLow ||
		first.Evidence.Spec != (deliveryevidence.SpecIdentity{}) {
		t.Fatalf("fresh evidence = %#v", first.Evidence)
	}
	if len(first.Evidence.AcceptanceMatrix) != 2 || len(first.Evidence.Scope.OwnedNow) != 2 ||
		len(first.Evidence.Scope.Forbidden) != 1 || len(first.Timing) != 1 {
		t.Fatalf("compiled run = %#v", first)
	}
	if first.Observations.CommitSHA != strings.Repeat("b", 40) ||
		first.Observations.TreeSHA != strings.Repeat("c", 40) ||
		!first.Observations.WorkspaceClean {
		t.Fatalf("persisted observations = %#v", first.Observations)
	}

	git.value.HeadSHA = strings.Repeat("d", 40)
	git.value.TreeSHA = strings.Repeat("e", 40)
	second, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.RunID != first.RunID || second.State != StateNeedsReview ||
		len(second.Timing) != len(first.Timing) {
		t.Fatalf("resume changed run: first=%#v second=%#v", first, second)
	}
	if second.Observations.CommitSHA != strings.Repeat("d", 40) ||
		second.Observations.TreeSHA != strings.Repeat("e", 40) {
		t.Fatalf("resume returned stale observations: %#v", second.Observations)
	}
	if git.calls != 2 || tracker.calls != 2 {
		t.Fatalf("observations were not reacquired: git=%d github=%d", git.calls, tracker.calls)
	}
}

func TestAdvanceSupersedesChangedAuthorityWithoutRewritingPriorRun(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	request := Request{RepositoryPath: "/repo", IssueNumber: 356}
	originalCriteria := append([]AuthorityItem(nil), tracker.value.Criteria...)
	first, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(
		module.storePathForTest(t, 356), "runs", first.RunID+".json",
	)
	oldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}

	tracker.mu.Lock()
	tracker.value.Criteria = append(tracker.value.Criteria, AuthorityItem{
		Text: "Record automatic phase timing.", EvidenceLink: "issue#356:criterion-3",
	})
	tracker.mu.Unlock()
	second, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.RunID == first.RunID || second.SupersedesRunID != first.RunID ||
		len(second.Evidence.AcceptanceMatrix) != 3 {
		t.Fatalf("requalified outcome = %#v", second)
	}
	gotOldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotOldBytes, oldBytes) {
		t.Fatal("superseding authority rewrote the prior run")
	}

	tracker.mu.Lock()
	tracker.value.Criteria = originalCriteria
	tracker.mu.Unlock()
	third, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if third.RunID == first.RunID || third.RunID == second.RunID ||
		third.SupersedesRunID != second.RunID {
		t.Fatalf("A-B-A requalification reused history: first=%s second=%s third=%#v", first.RunID, second.RunID, third)
	}
	gotOldBytes, err = os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotOldBytes, oldBytes) {
		t.Fatal("A-B-A requalification rewrote the original run")
	}
}

func TestAdvanceCompilesStableProfileIDsFromNormalizedAuthority(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	tracker.value.References = []ReferenceObservation{{
		Identity: "ADR 0020", URL: "docs/adr/0020-adopt-risk-adjusted-issue-delivery-orchestration.md",
	}}
	first, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	firstIDs := acceptanceIDs(first)

	tracker.mu.Lock()
	tracker.value.Criteria = []AuthorityItem{
		{Text: "  Resume without creating a duplicate run. ", EvidenceLink: "issue#356:criterion-2"},
		{Text: "Persist   one self-contained low-risk run.", EvidenceLink: "issue#356:criterion-1"},
	}
	tracker.mu.Unlock()
	second, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	if got := acceptanceIDs(second); strings.Join(got, ",") != strings.Join(firstIDs, ",") {
		t.Fatalf("stable IDs changed: first=%v second=%v", firstIDs, got)
	}
	if len(second.Evidence.Scope.Prerequisites) != 1 ||
		!strings.HasPrefix(second.Evidence.Scope.Prerequisites[0].Identity, "reference-") {
		t.Fatalf("authority references were not compiled: %#v", second.Evidence.Scope.Prerequisites)
	}
	for _, row := range second.Evidence.AcceptanceMatrix {
		if row.OwningSeam == "" || row.PositiveEvidence == "" || row.NegativeEvidence == "" ||
			row.FailureEvidence == "" || row.MutationEvidence == "" ||
			row.CompatibilityEvidence == "" || row.PreservationEvidence == "" ||
			row.MigrationEvidence == "" {
			t.Fatalf("profile-shaped row is incomplete: %#v", row)
		}
	}
}

func TestAdvancePausesForTypedDecisionAndResumesWithMatchingAnswer(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	tracker.value.Ambiguities = []AuthorityItem{{
		Text:         "Should compatibility with v1 be part of this issue?",
		EvidenceLink: "issue#356:question-1",
	}}
	request := Request{RepositoryPath: "/repo", IssueNumber: 356}
	pending, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != StateNeedsDecision || pending.Decision == nil || pending.Evidence != nil {
		t.Fatalf("pending outcome = %#v", pending)
	}

	_, err = module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 356,
		Decision: &Decision{RequestID: "wrong", Disposition: DecisionForbidden, Requirement: "No v1 changes.", EvidenceLink: "caller:decision"},
	})
	var mismatch *DecisionMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("mismatched decision error = %v", err)
	}

	resolved, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 356,
		Decision: &Decision{
			RequestID: pending.Decision.ID, Disposition: DecisionForbidden,
			Requirement: "Do not change the v1 workflow.", EvidenceLink: "caller:decision-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.State != StateNeedsReview || resolved.Evidence == nil ||
		resolved.SupersedesRunID != pending.RunID || len(resolved.Evidence.Scope.Forbidden) != 2 {
		t.Fatalf("resolved outcome = %#v", resolved)
	}
	resumed, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.RunID != resolved.RunID || resumed.State != StateNeedsReview {
		t.Fatalf("resolved decision was not resumed: resolved=%#v resumed=%#v", resolved, resumed)
	}
}

func TestAdvanceResolvesEveryMaterialAmbiguityBeforeReview(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	tracker.value.Ambiguities = []AuthorityItem{
		{Text: "Include v1 compatibility?", EvidenceLink: "issue#356:question-1"},
		{Text: "Include CLI wiring?", EvidenceLink: "issue#356:question-2"},
	}
	request := Request{RepositoryPath: "/repo", IssueNumber: 356}
	outcome, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for sequence := 0; sequence < 2; sequence++ {
		if outcome.State != StateNeedsDecision || outcome.Decision == nil {
			t.Fatalf("ambiguity %d was skipped: %#v", sequence+1, outcome)
		}
		outcome, err = module.Advance(context.Background(), Request{
			RepositoryPath: "/repo", IssueNumber: 356,
			Decision: &Decision{
				RequestID: outcome.Decision.ID, Disposition: DecisionForbidden,
				Requirement: "Exclude " + outcome.Decision.Evidence, EvidenceLink: "caller:decision-" + strconv.Itoa(sequence+1),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if outcome.State != StateNeedsReview || len(outcome.Evidence.Scope.Forbidden) != 3 {
		t.Fatalf("all ambiguities resolved outcome = %#v", outcome)
	}
}

func TestAdvanceSupersedesChangedAmbiguityEvidence(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	tracker.value.Ambiguities = []AuthorityItem{{
		Text: "Include v1 compatibility?", EvidenceLink: "issue#356:question-1",
	}}
	first, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	tracker.value.Ambiguities[0].Text = "Include both v1 compatibility and migration?"
	second, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	if second.RunID == first.RunID || second.SupersedesRunID != first.RunID ||
		second.Decision.ID == first.Decision.ID {
		t.Fatalf("changed ambiguity resumed stale prompt: first=%#v second=%#v", first, second)
	}
}

func TestAdvanceAdvertisesOnlySupportedDecisionsAndRejectsUnsolicitedInput(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	tracker.value.Criteria = nil
	pending, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Decision.Kind != DecisionSupplyCriterion ||
		len(pending.Decision.Options) != 1 || pending.Decision.Options[0] != DecisionOwnedNow {
		t.Fatalf("criterion decision options = %#v", pending.Decision)
	}

	cleanModule, _, _ := moduleFixture(t, 357)
	_, err = cleanModule.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		Decision: &Decision{
			RequestID: "unsolicited", Disposition: DecisionOwnedNow,
			Requirement: "Invented criterion", EvidenceLink: "caller:unsolicited",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not requested") {
		t.Fatalf("unsolicited decision error = %v", err)
	}
}

func TestAdvanceRejectsSemanticallyInvalidPersistedRun(t *testing.T) {
	module, _, _ := moduleFixture(t, 356)
	request := Request{RepositoryPath: "/repo", IssueNumber: 356}
	outcome, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	runPath := filepath.Join(module.storePathForTest(t, 356), "runs", outcome.RunID+".json")
	data, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	var wire runWire
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	wire.State = StateCompleted
	data, err = json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Advance(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "timing does not reach current state") {
		t.Fatalf("invalid persisted run error = %v", err)
	}
}

func TestAdvanceRecoversRunPersistedWithoutActivePointer(t *testing.T) {
	module, _, _ := moduleFixture(t, 356)
	request := Request{RepositoryPath: "/repo", IssueNumber: 356}
	first, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(module.storePathForTest(t, 356), "active.json")
	if err := os.Remove(activePath); err != nil {
		t.Fatal(err)
	}

	recovered, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.RunID != first.RunID || !reflect.DeepEqual(recovered.Timing, first.Timing) {
		t.Fatalf("orphaned run was regenerated: first=%#v recovered=%#v", first, recovered)
	}
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active pointer was not recovered: %v", err)
	}
}

func TestAdvanceRejectsSymlinkedStateDirectory(t *testing.T) {
	module, git, _ := moduleFixture(t, 356)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(git.value.CommonDir, "packy")); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 356,
	}); err == nil {
		t.Fatal("Advance accepted a symlinked state directory")
	}
}

func TestAdvanceUsesLockedDirectoryForEveryStateOperation(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	issueDir := module.storePathForTest(t, 356)
	movedDir := issueDir + "-moved"
	tracker.hook = func(issue int) {
		if issue != 356 {
			return
		}
		if err := os.Rename(issueDir, movedDir); err != nil {
			t.Error(err)
			return
		}
		if err := os.Mkdir(issueDir, 0700); err != nil {
			t.Error(err)
		}
	}
	outcome, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 356,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(movedDir, "runs", outcome.RunID+".json")); err != nil {
		t.Fatalf("run escaped the locked directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(movedDir, "active.json")); err != nil {
		t.Fatalf("active pointer escaped the locked directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(issueDir, "active.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement directory received state: %v", err)
	}
}

func TestAdvanceReportsBlockedDependency(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	tracker.value.Dependencies = []DependencyObservation{{
		Identity: "issue-355", Number: 355, Title: "Admit v2 evidence",
		State: "OPEN", URL: "https://github.com/yersonargotev/packy/issues/355",
	}}
	outcome, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != StateBlocked ||
		outcome.Evidence.Authority.DependencyDisposition[0].Disposition != deliveryevidence.DependencyBlocking {
		t.Fatalf("blocked outcome = %#v", outcome)
	}
}

func TestAdvanceSerializesOneIssueButAllowsDifferentIssues(t *testing.T) {
	module, _, tracker := moduleFixture(t, 356)
	entered := make(chan struct{})
	release := make(chan struct{})
	tracker.hook = func(issue int) {
		if issue == 356 {
			close(entered)
			<-release
		}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
		firstDone <- err
	}()
	<-entered

	waiting, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != StateWaiting {
		t.Fatalf("same issue contention = %#v", waiting)
	}
	other, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 357})
	if err != nil {
		t.Fatal(err)
	}
	if other.State != StateNeedsReview {
		t.Fatalf("different issue outcome = %#v", other)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestAdvanceRecordsAutomaticPhaseTiming(t *testing.T) {
	module, _, _ := moduleFixture(t, 356)
	outcome, err := module.Advance(context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 356})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Timing) != 1 {
		t.Fatalf("timing = %#v", outcome.Timing)
	}
	timing := outcome.Timing[0]
	if timing.Sequence != 1 || timing.Phase != "qualification" ||
		timing.StartedAt != "2026-07-30T01:00:00.000000000Z" ||
		timing.CompletedAt != "2026-07-30T01:00:01.000000000Z" {
		t.Fatalf("automatic timing = %#v", timing)
	}
}

func (m *Module) storePathForTest(t *testing.T, issue int) string {
	t.Helper()
	git := m.git.(*fakeGitObserver)
	return filepath.Join(git.value.CommonDir, "packy", "issue-delivery", "issue-"+strconv.Itoa(issue))
}

func acceptanceIDs(outcome Outcome) []string {
	ids := make([]string, 0, len(outcome.Evidence.AcceptanceMatrix))
	for _, row := range outcome.Evidence.AcceptanceMatrix {
		ids = append(ids, row.Identity)
	}
	return ids
}
