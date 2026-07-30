package issuedelivery

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildTimingReportUsesCanonicalCategories(t *testing.T) {
	timings := []Timing{
		timingFixture(1, "qualification", StateNeedsReview, 0),
		timingFixture(2, "branch-push", StateWaiting, 2),
		timingFixture(3, "review", StateNeedsReview, 4),
		timingFixture(4, "adjudication", StateNeedsDecision, 6),
		timingFixture(5, "exhaustive-validation", StateWaiting, 8),
		timingFixture(6, "ci-wait", StateNeedsReview, 10),
		timingFixture(7, "merge", StateWaiting, 12),
		timingFixture(8, "completion", StateCompleted, 14),
	}

	report, err := BuildTimingReport(timings, timingInstant(20))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Categories) != 8 {
		t.Fatalf("categories = %#v", report.Categories)
	}
	for i, want := range timingCategoryOrder {
		got := report.Categories[i]
		if got.Category != want || got.DurationNanoseconds != int64(time.Second) {
			t.Fatalf("category %d = %#v, want %q for one second", i, got, want)
		}
	}
	if report.LowRisk.EndToEndNanoseconds != int64(15*time.Second) ||
		report.LowRisk.CIWaitNanoseconds != int64(time.Second) {
		t.Fatalf("low-risk measurement = %#v", report.LowRisk)
	}
	if report.LowRisk.PRReadinessObjectiveNanoseconds != int64(25*time.Minute) ||
		report.LowRisk.EndToEndObjectiveMinNanoseconds != int64(25*time.Minute) ||
		report.LowRisk.EndToEndObjectiveMaxNanoseconds != int64(35*time.Minute) ||
		report.LowRisk.CIWaitAssumptionNanoseconds != int64(10*time.Minute) {
		t.Fatalf("low-risk objectives = %#v", report.LowRisk)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "passed") || strings.Contains(string(data), "gate") {
		t.Fatalf("low-risk measurement became a gate: %s", data)
	}
}

func TestBuildTimingReportMeasuresQualificationToPRReadiness(t *testing.T) {
	timings := []Timing{
		timingFixture(1, "qualification", StateNeedsReview, 0),
		timingFixture(2, "pull-request", StateWaiting, 8),
		timingFixture(3, "ci-wait", StateNeedsReview, 12),
	}
	report, err := BuildTimingReport(timings, timingInstant(20))
	if err != nil {
		t.Fatal(err)
	}
	if report.LowRisk.QualificationToPRReadinessNanoseconds == nil ||
		*report.LowRisk.QualificationToPRReadinessNanoseconds != int64(9*time.Second) {
		t.Fatalf("qualification-to-PR readiness = %#v", report.LowRisk.QualificationToPRReadinessNanoseconds)
	}
}

func TestTimingCategoryForPhaseCoversCurrentVocabulary(t *testing.T) {
	phases := map[string]TimingCategory{
		"qualification": TimingQualification,

		"implementation":      TimingImplementation,
		"non-local-freshness": TimingImplementation, "non-local-authorization": TimingImplementation,
		"non-local-observation": TimingImplementation, "branch-push": TimingImplementation,
		"pull-request": TimingImplementation,

		"review": TimingReview, "specialist-review": TimingReview,
		"qualification-review": TimingReview,
		"repair":               TimingRepair, "adjudication": TimingRepair, "ci-candidate-failure": TimingRepair,
		"qualification-correction": TimingRepair,

		"risk-observation": TimingValidation, "focused-validation": TimingValidation,
		"boundary-validation": TimingValidation, "exhaustive-validation": TimingValidation,
		"local-readiness": TimingValidation, "merge-readiness": TimingValidation,
		"integration-verification": TimingValidation, "post-merge-observation": TimingValidation,

		"ci-wait": TimingCIWait, "ci-success": TimingCIWait, "ci-retry": TimingCIWait,
		"merge": TimingMerge, "merge-adoption": TimingMerge,

		"remote-cleanup": TimingCleanup, "local-cleanup": TimingCleanup,
		"worktree-cleanup": TimingCleanup, "local-branch-cleanup": TimingCleanup,
		"main-synchronization": TimingCleanup, "completion": TimingCleanup,
	}
	for phase, want := range phases {
		got, ok := timingCategoryForPhase(phase)
		if !ok || got != want {
			t.Fatalf("phase %q = %q, %v; want %q", phase, got, ok, want)
		}
	}
}

func TestBuildTimingReportIncludesOnlyAttributableOpenExternalWait(t *testing.T) {
	timings := []Timing{
		timingFixture(1, "qualification", StateNeedsReview, 0),
		timingFixture(2, "ci-wait", StateWaiting, 2),
	}
	report, err := BuildTimingReport(timings, timingInstant(8))
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Categories[5].DurationNanoseconds; got != int64(6*time.Second) {
		t.Fatalf("ci-wait = %s", time.Duration(got))
	}

	timings[1] = timingFixture(2, "review", StateWaiting, 2)
	report, err = BuildTimingReport(timings, timingInstant(8))
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Categories[2].DurationNanoseconds; got != int64(time.Second) {
		t.Fatalf("ambiguous open review wait = %s", time.Duration(got))
	}
}

func TestBuildTimingReportRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		timings []Timing
		now     time.Time
		want    string
	}{
		{
			name:    "unknown phase",
			timings: []Timing{timingFixture(1, "future-phase", StateWaiting, 0)},
			now:     timingInstant(2),
			want:    "unknown timing phase",
		},
		{
			name: "invalid timestamp",
			timings: []Timing{{
				Sequence: 1, Phase: "qualification", To: StateNeedsReview,
				StartedAt: "not-a-time", CompletedAt: timingInstant(1).Format(timeFormat),
			}},
			now:  timingInstant(2),
			want: "invalid started_at",
		},
		{
			name: "reversed timestamp",
			timings: []Timing{{
				Sequence: 1, Phase: "qualification", To: StateNeedsReview,
				StartedAt: timingInstant(2).Format(timeFormat), CompletedAt: timingInstant(1).Format(timeFormat),
			}},
			now:  timingInstant(3),
			want: "completes before it starts",
		},
		{
			name: "noncanonical chronology",
			timings: []Timing{
				timingFixture(1, "qualification", StateNeedsReview, 2),
				timingFixture(2, "review", StateNeedsReview, 0),
			},
			now:  timingInstant(4),
			want: "starts before the previous phase completes",
		},
		{
			name:    "open wait from the future",
			timings: []Timing{timingFixture(1, "ci-wait", StateWaiting, 2)},
			now:     timingInstant(2),
			want:    "precedes open ci-wait",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildTimingReport(test.timings, test.now)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func timingFixture(sequence int, phase string, to State, offsetSeconds int) Timing {
	return Timing{
		Sequence: sequence, Phase: phase, To: to,
		StartedAt:   timingInstant(offsetSeconds).Format(timeFormat),
		CompletedAt: timingInstant(offsetSeconds + 1).Format(timeFormat),
	}
}

func timingInstant(offsetSeconds int) time.Time {
	return time.Date(2026, 7, 30, 12, 0, offsetSeconds, 0, time.UTC)
}
