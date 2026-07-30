package issuedelivery

import (
	"fmt"
	"time"
)

type TimingCategory string

const (
	TimingQualification  TimingCategory = "qualification"
	TimingImplementation TimingCategory = "implementation"
	TimingReview         TimingCategory = "review"
	TimingRepair         TimingCategory = "repair"
	TimingValidation     TimingCategory = "validation"
	TimingCIWait         TimingCategory = "ci-wait"
	TimingMerge          TimingCategory = "merge"
	TimingCleanup        TimingCategory = "cleanup"
)

var timingCategoryOrder = [...]TimingCategory{
	TimingQualification,
	TimingImplementation,
	TimingReview,
	TimingRepair,
	TimingValidation,
	TimingCIWait,
	TimingMerge,
	TimingCleanup,
}

type TimingCategoryDuration struct {
	Category            TimingCategory `json:"category"`
	DurationNanoseconds int64          `json:"duration_nanoseconds"`
}

// TimingReport is observational telemetry. LowRiskDeliveryNanoseconds is a
// measurable lead-time value, not a delivery gate.
type TimingReport struct {
	Categories []TimingCategoryDuration `json:"categories"`
	LowRisk    LowRiskTimingTelemetry   `json:"low_risk"`
}

type LowRiskTimingTelemetry struct {
	QualificationToPRReadinessNanoseconds *int64 `json:"qualification_to_pr_readiness_nanoseconds,omitempty"`
	EndToEndNanoseconds                   int64  `json:"end_to_end_nanoseconds"`
	CIWaitNanoseconds                     int64  `json:"ci_wait_nanoseconds"`
	PRReadinessObjectiveNanoseconds       int64  `json:"pr_readiness_objective_nanoseconds"`
	EndToEndObjectiveMinNanoseconds       int64  `json:"end_to_end_objective_min_nanoseconds"`
	EndToEndObjectiveMaxNanoseconds       int64  `json:"end_to_end_objective_max_nanoseconds"`
	CIWaitAssumptionNanoseconds           int64  `json:"ci_wait_assumption_nanoseconds"`
}

func BuildTimingReport(timings []Timing, now time.Time) (TimingReport, error) {
	totals := make(map[TimingCategory]time.Duration, len(timingCategoryOrder))
	var previousCompleted time.Time
	for i, timing := range timings {
		if timing.Sequence != i+1 {
			return TimingReport{}, fmt.Errorf("timing sequence %d is not canonical", timing.Sequence)
		}
		category, ok := timingCategoryForPhase(timing.Phase)
		if !ok {
			return TimingReport{}, fmt.Errorf("unknown timing phase %q", timing.Phase)
		}
		started, err := time.Parse(timeFormat, timing.StartedAt)
		if err != nil {
			return TimingReport{}, fmt.Errorf("timing phase %q has invalid started_at: %w", timing.Phase, err)
		}
		completed, err := time.Parse(timeFormat, timing.CompletedAt)
		if err != nil {
			return TimingReport{}, fmt.Errorf("timing phase %q has invalid completed_at: %w", timing.Phase, err)
		}
		if completed.Before(started) {
			return TimingReport{}, fmt.Errorf("timing phase %q completes before it starts", timing.Phase)
		}
		if i > 0 && started.Before(previousCompleted) {
			return TimingReport{}, fmt.Errorf("timing phase %q starts before the previous phase completes", timing.Phase)
		}
		totals[category] += completed.Sub(started)
		previousCompleted = completed
	}

	if len(timings) > 0 {
		last := timings[len(timings)-1]
		if last.To == StateWaiting && last.Phase == "ci-wait" {
			completed, _ := time.Parse(timeFormat, last.CompletedAt)
			if now.Before(completed) {
				return TimingReport{}, fmt.Errorf("timing report time precedes open ci-wait")
			}
			totals[TimingCIWait] += now.Sub(completed)
		}
	}

	report := TimingReport{
		Categories: make([]TimingCategoryDuration, 0, len(timingCategoryOrder)),
		LowRisk: LowRiskTimingTelemetry{
			PRReadinessObjectiveNanoseconds: int64(25 * time.Minute),
			EndToEndObjectiveMinNanoseconds: int64(25 * time.Minute),
			EndToEndObjectiveMaxNanoseconds: int64(35 * time.Minute),
			CIWaitAssumptionNanoseconds:     int64(10 * time.Minute),
		},
	}
	for _, category := range timingCategoryOrder {
		duration := totals[category]
		report.Categories = append(report.Categories, TimingCategoryDuration{
			Category:            category,
			DurationNanoseconds: duration.Nanoseconds(),
		})
	}
	report.LowRisk.CIWaitNanoseconds = totals[TimingCIWait].Nanoseconds()
	if len(timings) > 0 {
		qualifiedAt, _ := time.Parse(timeFormat, timings[0].StartedAt)
		endAt, _ := time.Parse(timeFormat, timings[len(timings)-1].CompletedAt)
		if timings[len(timings)-1].To == StateWaiting && timings[len(timings)-1].Phase == "ci-wait" {
			endAt = now
		}
		report.LowRisk.EndToEndNanoseconds = endAt.Sub(qualifiedAt).Nanoseconds()
		for _, timing := range timings {
			if timing.Phase != "pull-request" {
				continue
			}
			readyAt, _ := time.Parse(timeFormat, timing.CompletedAt)
			elapsed := readyAt.Sub(qualifiedAt).Nanoseconds()
			report.LowRisk.QualificationToPRReadinessNanoseconds = &elapsed
		}
	}
	return report, nil
}

func timingCategoryForPhase(phase string) (TimingCategory, bool) {
	switch phase {
	case "qualification":
		return TimingQualification, true
	case "implementation", "non-local-freshness", "non-local-authorization", "non-local-observation",
		"branch-push", "pull-request":
		return TimingImplementation, true
	case "review", "specialist-review":
		return TimingReview, true
	case "repair", "adjudication", "ci-candidate-failure":
		return TimingRepair, true
	case "risk-observation", "focused-validation", "boundary-validation",
		"exhaustive-validation", "local-readiness", "merge-readiness",
		"integration-verification", "post-merge-observation":
		return TimingValidation, true
	case "ci-wait", "ci-success":
		return TimingCIWait, true
	case "merge", "merge-adoption":
		return TimingMerge, true
	case "remote-cleanup", "local-cleanup", "worktree-cleanup",
		"local-branch-cleanup", "main-synchronization", "completion":
		return TimingCleanup, true
	case "ci-retry":
		return TimingCIWait, true
	default:
		return "", false
	}
}
