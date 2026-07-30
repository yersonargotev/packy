package issuedelivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

func (m *Module) advanceAssurance(
	ctx context.Context,
	store lockedIssueStore,
	record runRecord,
	git GitObservation,
	tracker TrackerObservation,
	compiled compiledAuthority,
	request Request,
) (Outcome, error) {
	if request.Repair != nil {
		return m.applyRepairDecision(store, record, *request.Repair)
	}
	if record.PendingRepair != nil {
		return outcomeFromRecord(record), nil
	}
	if request.NonLocal != nil && m.nonlocal == nil {
		return Outcome{}, errors.New("non-local authorization requires a configured non-local gateway")
	}
	if request.NonLocal != nil && record.LocalReadiness == nil {
		return Outcome{}, errors.New("non-local authorization requires exact local readiness")
	}

	candidate := latestCandidate(&record)
	if candidate == nil || candidate.CommitSHA != git.HeadSHA || candidate.TreeSHA != git.TreeSHA {
		if candidate != nil && hasAcceptedFindings(candidate.RepairDecision) {
			// The changed tree is the declared repair batch.
		} else if candidate != nil && unresolvedFindingIDs(candidate) != nil {
			return Outcome{}, errors.New("candidate changed before its review findings were adjudicated")
		}
		activityPhase := "implementation"
		if candidate != nil {
			activityPhase = "repair"
		}
		if err := m.appendElapsedTiming(&record, activityPhase, m.clock.Now().UTC()); err != nil {
			return Outcome{}, err
		}
		next := newCandidate(record, candidate, git)
		if record.Schema != legacyRunSchema {
			if _, err := m.observeCandidateRisk(ctx, &record, &next, candidate, request.RepositoryPath); err != nil {
				return m.persistAssuranceTransition(
					store, record, StateBlocked, "candidate risk observation is incomplete or invalid",
					"risk-observation",
				)
			}
		}
		if err := validateSandboxRoot(m.sandboxRoot); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "configured validation sandbox is not physically isolated",
				"focused-validation",
			)
		}
		result, err := m.validation.Focused(ctx, m.validationRequest(record, next))
		if err != nil {
			return Outcome{}, fmt.Errorf("run focused validation: %w", err)
		}
		if err := m.validateValidationResult(result, next, false); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "focused validation sandbox or exact candidate evidence is invalid",
				"focused-validation",
			)
		}
		next.Focused = &ValidationProof{
			Kind: "focused", Result: result, CompletedAt: m.clock.Now().UTC().Format(timeFormat),
		}
		record.Candidates = append(record.Candidates, next)
		record.LocalReadiness = nil
		record.NonLocal = nil
		invalidateAcceptance(record.Evidence, record.QualificationCorrections)
		return m.persistAssuranceTransition(
			store, record, StateNeedsReview, "focused candidate evidence is ready for review", "focused-validation",
		)
	}

	if record.NonLocal != nil && record.NonLocal.CandidateFailure != nil &&
		record.NonLocal.Authorization.CandidateID == candidate.ID {
		return outcomeWithReason(
			record, StateNeedsReview,
			"change-attributable CI failure requires a candidate-changing repair",
		), nil
	}

	if record.Schema != legacyRunSchema {
		riskChanged, err := m.observeCandidateRisk(ctx, &record, candidate, nil, request.RepositoryPath)
		if err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "candidate risk observation is incomplete or invalid",
				"risk-observation",
			)
		}
		if riskChanged {
			invalidateForProfileEscalation(&record, candidate)
			return m.persistAssuranceTransition(
				store, record, StateNeedsReview, "candidate assurance profile or sensitive boundaries escalated",
				"risk-observation",
			)
		}
	}

	if record.LocalReadiness != nil &&
		record.LocalReadiness.CandidateID == candidate.ID &&
		record.LocalReadiness.CommitSHA == git.HeadSHA &&
		record.LocalReadiness.TreeSHA == git.TreeSHA {
		if record.LocalReadiness.Branch == git.Branch &&
			git.WorkspaceClean && deliveryBranch(git.Branch, tracker.Issue.Number) {
			if m.nonlocal != nil {
				return m.advanceNonLocal(ctx, store, record, git, tracker, compiled, request)
			}
			if record.State == StateWaiting {
				return outcomeFromRecord(record), nil
			}
			return m.persistAssuranceTransition(
				store, record, StateWaiting, "exact local readiness is proved; awaiting non-local delivery authority",
				"local-readiness",
			)
		}
		return m.persistAssuranceTransition(
			store, record, StateBlocked, "local readiness no longer matches the current branch or workspace",
			"local-readiness",
		)
	}

	missing := missingReviewAxes(candidate)
	if len(missing) > 0 {
		reviews, err := m.executeReviews(ctx, record, *candidate, missing)
		if err != nil {
			return Outcome{}, err
		}
		for _, review := range reviews {
			if review.Completed {
				candidate.Reviews = append(candidate.Reviews, review)
			}
		}
		sort.Slice(candidate.Reviews, func(i, j int) bool {
			return candidate.Reviews[i].Axis < candidate.Reviews[j].Axis
		})
		for _, review := range reviews {
			if !review.Completed {
				return m.persistAssuranceTransition(
					store, record, StateWaiting, "independent candidate review is still running", "review",
				)
			}
		}
		if ids := unresolvedFindingIDs(candidate); len(ids) > 0 {
			record.PendingRepair = repairDecisionRequest(candidate.ID, ids)
			return m.persistAssuranceTransition(
				store, record, StateNeedsDecision, "review findings require one batch adjudication", "review",
			)
		}
		return m.persistAssuranceTransition(
			store, record, StateNeedsReview, "required candidate reviews completed without findings", "review",
		)
	}

	if ids := unresolvedFindingIDs(candidate); len(ids) > 0 {
		record.PendingRepair = repairDecisionRequest(candidate.ID, ids)
		return m.persistAssuranceTransition(
			store, record, StateNeedsDecision, "review findings require one batch adjudication", "review",
		)
	}
	if missing := missingSpecialistBoundaries(candidate); len(missing) > 0 {
		reviews, err := m.executeSpecialistReviews(ctx, record, *candidate, missing)
		if err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "required high-risk specialist review is unavailable",
				"specialist-review",
			)
		}
		for _, review := range reviews {
			if review.Completed {
				candidate.SpecialistReviews = append(candidate.SpecialistReviews, review)
			}
		}
		sort.Slice(candidate.SpecialistReviews, func(i, j int) bool {
			return candidate.SpecialistReviews[i].Boundary < candidate.SpecialistReviews[j].Boundary
		})
		for _, review := range reviews {
			if !review.Completed {
				return m.persistAssuranceTransition(
					store, record, StateWaiting, "high-risk specialist review is still running",
					"specialist-review",
				)
			}
		}
		if ids := unresolvedFindingIDs(candidate); len(ids) > 0 {
			record.PendingRepair = repairDecisionRequest(candidate.ID, ids)
			return m.persistAssuranceTransition(
				store, record, StateNeedsDecision, "review findings require one batch adjudication",
				"specialist-review",
			)
		}
		return m.persistAssuranceTransition(
			store, record, StateNeedsReview, "required high-risk specialist reviews completed",
			"specialist-review",
		)
	}
	if hasAcceptedFindings(candidate.RepairDecision) {
		return outcomeWithReason(record, StateNeedsReview, "accepted findings must be repaired as one candidate batch"), nil
	}
	if missing := missingBoundaryProofs(candidate); len(missing) > 0 {
		proofs, err := m.executeBoundaryProofs(ctx, record, *candidate, missing)
		if err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "required sandboxed high-risk boundary proof is invalid",
				"boundary-validation",
			)
		}
		candidate.BoundaryProofs = append(candidate.BoundaryProofs, proofs...)
		sort.Slice(candidate.BoundaryProofs, func(i, j int) bool {
			return candidate.BoundaryProofs[i].Result.Boundary < candidate.BoundaryProofs[j].Result.Boundary
		})
		return m.persistAssuranceTransition(
			store, record, StateNeedsReview, "required sandboxed high-risk boundary proofs completed",
			"boundary-validation",
		)
	}
	if candidate.Exhaustive == nil {
		if err := validateSandboxRoot(m.sandboxRoot); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "configured validation sandbox is not physically isolated",
				"exhaustive-validation",
			)
		}
		result, err := m.validation.Exhaustive(ctx, m.validationRequest(record, *candidate))
		if err != nil {
			return Outcome{}, fmt.Errorf("run exhaustive validation: %w", err)
		}
		if err := m.validateValidationResult(result, *candidate, true); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "exhaustive validation sandbox or exact candidate evidence is invalid",
				"exhaustive-validation",
			)
		}
		nextEvidence := *record.Evidence
		nextEvidence.AcceptanceMatrix = append(
			[]deliveryevidence.AcceptanceRow(nil), record.Evidence.AcceptanceMatrix...,
		)
		if err := admitAcceptanceProofs(&nextEvidence, candidate, result.Acceptance); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "exhaustive validation lacks exact acceptance traceability",
				"exhaustive-validation",
			)
		}
		completedAt := m.clock.Now().UTC().Format(time.RFC3339Nano)
		nextEvidence, err = deliveryevidence.RecordExhaustiveValidation(
			nextEvidence,
			deliveryevidence.ExhaustiveValidationResult{
				Observation: deliveryevidence.ValidationObservation{
					Repository: record.Repository, CheckoutSHA256: result.CheckoutSHA256,
					CommitSHA: result.CommitSHA, TreeSHA: result.TreeSHA, WorkspaceClean: result.WorkspaceClean,
					ValidatorIdentity: result.ValidatorIdentity, ValidatorSHA256: result.ValidatorSHA256,
					ValidatorIdentityExpiresAt: result.ValidatorIdentityExpiresAt,
					RequiredCommand:            result.Command,
					Sandbox: deliveryevidence.SandboxFacts{
						HomeRoot: result.HomeRoot, ConfigHomeRoot: result.ConfigRoot, Sandboxed: result.Sandboxed,
					},
				},
				CompletedAt: completedAt, Succeeded: result.Succeeded, Completed: result.Completed,
			},
		)
		if err != nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "canonical exhaustive validation receipt is invalid",
				"exhaustive-validation",
			)
		}
		candidate.Exhaustive = &ValidationProof{
			Kind: "exhaustive", Result: result, CompletedAt: completedAt,
		}
		candidate.Acceptance = append([]AcceptanceProof(nil), result.Acceptance...)
		record.Evidence = &nextEvidence
	}
	freshGit, err := m.git.ObserveGit(ctx, git.Worktree)
	if err != nil {
		return Outcome{}, fmt.Errorf("reobserve Git after exhaustive validation: %w", err)
	}
	freshTracker, err := m.github.ObserveIssue(ctx, freshGit, tracker.Issue.Number)
	if err != nil {
		return Outcome{}, fmt.Errorf("reobserve GitHub after exhaustive validation: %w", err)
	}
	freshAuthority, err := compileAuthority(
		freshGit, freshTracker, record.Decisions, nil, record.Evidence.RiskProfile,
	)
	if err != nil {
		return Outcome{}, err
	}
	if freshAuthority.hash != compiled.hash || freshGit.HeadSHA != candidate.CommitSHA ||
		freshGit.TreeSHA != candidate.TreeSHA || !freshGit.WorkspaceClean ||
		!deliveryBranch(freshGit.Branch, tracker.Issue.Number) {
		return m.persistAssuranceTransition(
			store, record, StateBlocked, "fresh authority or exact HEAD changed during exhaustive validation",
			"local-readiness",
		)
	}
	record.LocalReadiness = &LocalReadiness{
		CandidateID: candidate.ID, CommitSHA: candidate.CommitSHA, TreeSHA: candidate.TreeSHA,
		AuthorityHash: compiled.hash, Branch: freshGit.Branch, ReadyAt: m.clock.Now().UTC().Format(timeFormat),
	}
	return m.persistAssuranceTransition(
		store, record, StateWaiting, "exact local readiness is proved; awaiting non-local delivery authority",
		"local-readiness",
	)
}

func (m *Module) applyRepairDecision(
	store lockedIssueStore,
	record runRecord,
	decision RepairDecision,
) (Outcome, error) {
	candidate := latestCandidate(&record)
	if candidate == nil || record.PendingRepair == nil ||
		decision.CandidateID != candidate.ID || decision.CandidateID != record.PendingRepair.CandidateID {
		return Outcome{}, errors.New("repair decision does not match the pending candidate")
	}
	if decision.Class != RepairBounded && decision.Class != RepairCandidateChanging {
		return Outcome{}, fmt.Errorf("invalid repair class %q", decision.Class)
	}
	expected := unresolvedFindingIDs(candidate)
	if len(decision.Findings) != len(expected) {
		return Outcome{}, errors.New("repair decision must adjudicate every finding as one batch")
	}
	got := make(map[string]bool, len(decision.Findings))
	for _, item := range decision.Findings {
		if got[item.FindingID] || strings.TrimSpace(item.Evidence) == "" ||
			(item.Disposition != FindingAccepted && item.Disposition != FindingRejected) {
			return Outcome{}, errors.New("repair decision contains an invalid finding adjudication")
		}
		got[item.FindingID] = true
	}
	for _, id := range expected {
		if !got[id] {
			return Outcome{}, errors.New("repair decision must adjudicate every finding as one batch")
		}
	}
	if decision.Class == RepairBounded && !hasAcceptedFindings(&decision) {
		return Outcome{}, errors.New("bounded repair requires at least one accepted finding")
	}
	decision.Findings = append([]FindingDecision(nil), decision.Findings...)
	sort.Slice(decision.Findings, func(i, j int) bool {
		return decision.Findings[i].FindingID < decision.Findings[j].FindingID
	})
	candidate.RepairDecision = &decision
	record.PendingRepair = nil
	reason := "review findings were adjudicated without an accepted repair"
	if hasAcceptedFindings(&decision) {
		reason = "accepted findings must be repaired as one candidate batch"
	}
	return m.persistAssuranceTransition(store, record, StateNeedsReview, reason, "adjudication")
}

func (m *Module) executeReviews(
	ctx context.Context,
	record runRecord,
	candidate Candidate,
	axes []deliveryevidence.ReviewAxis,
) ([]CandidateReview, error) {
	type result struct {
		review CandidateReview
		err    error
	}
	results := make(chan result, len(axes))
	var group sync.WaitGroup
	for _, axis := range axes {
		axis := axis
		group.Add(1)
		go func() {
			defer group.Done()
			review, err := m.review.Review(ctx, ReviewRequest{
				RunID: record.ID, CandidateID: candidate.ID, Repository: record.Repository, Issue: record.Issue,
				Axis: axis, BaseSHA: candidate.BaseSHA, CommitSHA: candidate.CommitSHA, TreeSHA: candidate.TreeSHA,
			})
			results <- result{review: review, err: err}
		}()
	}
	group.Wait()
	close(results)
	out := make([]CandidateReview, 0, len(axes))
	for result := range results {
		if result.err != nil {
			return nil, fmt.Errorf("execute candidate review: %w", result.err)
		}
		if err := validateCandidateReview(result.review, candidate, axes); err != nil {
			return nil, err
		}
		out = append(out, result.review)
	}
	if err := validateReviewBatch(candidate, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *Module) persistAssuranceTransition(
	store lockedIssueStore,
	record runRecord,
	state State,
	reason, phase string,
) (Outcome, error) {
	started, err := time.Parse(timeFormat, record.UpdatedAt)
	if err != nil {
		return Outcome{}, fmt.Errorf("parse issue delivery transition start: %w", err)
	}
	previous := record.State
	record.State, record.Reason = state, reason
	completed := m.clock.Now().UTC()
	if completed.Before(started) {
		return Outcome{}, errors.New("issue delivery transition clock moved backwards")
	}
	record.Timing = append(record.Timing, Timing{
		Sequence: len(record.Timing) + 1, Phase: phase, From: previous, To: state,
		StartedAt: started.Format(timeFormat), CompletedAt: completed.Format(timeFormat),
	})
	record.UpdatedAt = completed.Format(timeFormat)
	adoptLegacyNullQualificationFindings(&record)
	data, err := encodeRun(record)
	if err != nil {
		return Outcome{}, err
	}
	if _, err := store.storeRevisionAndActivate(record.ID, data); err != nil {
		return Outcome{}, err
	}
	return outcomeFromRecord(record), nil
}

func (m *Module) appendElapsedTiming(record *runRecord, phase string, completed time.Time) error {
	started, err := time.Parse(timeFormat, record.UpdatedAt)
	if err != nil {
		return fmt.Errorf("parse issue delivery activity start: %w", err)
	}
	if completed.Before(started) {
		return errors.New("issue delivery activity clock moved backwards")
	}
	record.Timing = append(record.Timing, Timing{
		Sequence: len(record.Timing) + 1, Phase: phase, From: record.State, To: record.State,
		StartedAt: started.Format(timeFormat), CompletedAt: completed.Format(timeFormat),
	})
	record.UpdatedAt = completed.Format(timeFormat)
	return nil
}

func newCandidate(record runRecord, previous *Candidate, git GitObservation) Candidate {
	base, class := record.Evidence.StartingBaseSHA, RepairClass("")
	required := bothReviewAxes()
	if previous != nil {
		class = RepairCandidateChanging
		if hasAcceptedFindings(previous.RepairDecision) {
			class = previous.RepairDecision.Class
			if class == RepairBounded {
				required = acceptedFindingAxes(*previous)
			}
		}
	}
	return Candidate{
		ID:      candidateIdentity(record.ID, base, git.HeadSHA, git.TreeSHA),
		BaseSHA: base, CommitSHA: git.HeadSHA, TreeSHA: git.TreeSHA, RepairClass: class,
		RequiredReviews: required, Reviews: []CandidateReview{},
		Effects: []EffectObservation{}, Boundaries: []SensitiveBoundary{},
		RequiredSpecialists: []SensitiveBoundary{}, SpecialistReviews: []SpecialistReview{},
		BoundaryProofs: []BoundaryProof{},
	}
}

func candidateIdentity(runID, base, commit, tree string) string {
	sum := sha256.Sum256([]byte(runID + "\x00" + base + "\x00" + commit + "\x00" + tree))
	return hex.EncodeToString(sum[:])
}

func (m *Module) validationRequest(record runRecord, candidate Candidate) ValidationRequest {
	return ValidationRequest{
		RunID: record.ID, CandidateID: candidate.ID, Repository: record.Repository, Issue: record.Issue,
		CommitSHA: candidate.CommitSHA, TreeSHA: candidate.TreeSHA,
		HomeRoot: filepath.Join(m.sandboxRoot, "home"), ConfigRoot: filepath.Join(m.sandboxRoot, "config"),
		AcceptanceRows: append([]deliveryevidence.AcceptanceRow(nil), record.Evidence.AcceptanceMatrix...),
		Profile:        candidate.Profile, Boundaries: append([]SensitiveBoundary(nil), candidate.Boundaries...),
	}
}

func (m *Module) validateValidationResult(result ValidationResult, candidate Candidate, exhaustive bool) error {
	if err := validateSandboxRoot(m.sandboxRoot); err != nil {
		return err
	}
	if result.CommitSHA != candidate.CommitSHA || result.TreeSHA != candidate.TreeSHA ||
		!result.Sandboxed || !result.Succeeded || !result.Completed ||
		strings.TrimSpace(result.Command) == "" {
		return errors.New("validation result does not prove the exact sandboxed candidate")
	}
	for _, root := range []string{result.HomeRoot, result.ConfigRoot} {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return errors.New("validation sandbox roots must be absolute and canonical")
		}
	}
	if result.HomeRoot == result.ConfigRoot {
		return errors.New("validation sandbox roots must be distinct")
	}
	if result.HomeRoot != filepath.Join(m.sandboxRoot, "home") ||
		result.ConfigRoot != filepath.Join(m.sandboxRoot, "config") {
		return errors.New("validation result does not use the configured sandbox")
	}
	if exhaustive {
		expires, err := time.Parse(time.RFC3339Nano, result.ValidatorIdentityExpiresAt)
		if result.Command != "./scripts/validate-packy.sh" ||
			result.ValidatorIdentity != "scripts/validate-packy.sh" ||
			!runIDPattern.MatchString(result.CheckoutSHA256) ||
			!runIDPattern.MatchString(result.ValidatorSHA256) ||
			err != nil || !m.clock.Now().UTC().Before(expires) || !result.WorkspaceClean {
			return errors.New("exhaustive result does not prove the repository validation authority")
		}
	}
	return nil
}

func validateCandidateReview(review CandidateReview, candidate Candidate, requested []deliveryevidence.ReviewAxis) error {
	if review.CandidateID != candidate.ID || !containsAxis(requested, review.Axis) || review.Findings == nil {
		return errors.New("candidate review does not match its exact request")
	}
	if !review.Completed && len(review.Findings) != 0 {
		return errors.New("incomplete candidate review cannot contain findings")
	}
	for _, finding := range review.Findings {
		if finding.Axis != review.Axis || strings.TrimSpace(finding.ID) == "" {
			return errors.New("candidate review contains an invalid finding")
		}
	}
	return nil
}

func validateReviewBatch(candidate Candidate, reviews []CandidateReview) error {
	seen := make(map[string]bool)
	for _, review := range candidate.Reviews {
		for _, finding := range review.Findings {
			seen[finding.ID] = true
		}
	}
	for _, review := range reviews {
		for _, finding := range review.Findings {
			if seen[finding.ID] {
				return fmt.Errorf("duplicate candidate review finding ID %q", finding.ID)
			}
			seen[finding.ID] = true
		}
	}
	return nil
}

func invalidateAcceptance(
	evidence *deliveryevidence.Bundle,
	corrections []QualificationCorrection,
) {
	if len(corrections) > 0 {
		evidence.AcceptanceMatrix = append(
			[]deliveryevidence.AcceptanceRow(nil),
			corrections[len(corrections)-1].AcceptanceMatrix...,
		)
	}
	for index := range evidence.AcceptanceMatrix {
		row := &evidence.AcceptanceMatrix[index]
		row.State = deliveryevidence.AcceptancePlanned
	}
	evidence.ValidationReceipts = []deliveryevidence.ValidationReceipt{}
}

func admitAcceptanceProofs(
	evidence *deliveryevidence.Bundle,
	candidate *Candidate,
	proofs []AcceptanceProof,
) error {
	if len(proofs) != len(evidence.AcceptanceMatrix) {
		return errors.New("every acceptance row requires one exact proof")
	}
	byID := make(map[string]AcceptanceProof, len(proofs))
	required := map[string]bool{"positive": true, "failure": true, "mutation": true}
	if candidate.Profile == "" {
		for _, name := range []string{
			"positive", "negative", "failure", "mutation", "compatibility", "preservation", "migration",
		} {
			required[name] = true
		}
	} else if candidate.Profile == deliveryevidence.RiskStandard || candidate.Profile == deliveryevidence.RiskHigh {
		required["negative"] = true
		required["compatibility"] = true
		required["preservation"] = true
	}
	if candidateRequiresMigrationEvidence(candidate) {
		required["migration"] = true
	}
	for _, proof := range proofs {
		if byID[proof.Identity].Identity != "" {
			return fmt.Errorf("duplicate acceptance proof %q", proof.Identity)
		}
		for name, value := range map[string]string{
			"positive": proof.PositiveEvidence, "negative": proof.NegativeEvidence,
			"failure": proof.FailureEvidence, "mutation": proof.MutationEvidence,
			"compatibility": proof.CompatibilityEvidence, "preservation": proof.PreservationEvidence,
			"migration": proof.MigrationEvidence,
		} {
			value = strings.TrimSpace(value)
			if value == "" ||
				(required[name] && !strings.Contains(value, candidate.ID)) ||
				(!required[name] && !strings.Contains(value, candidate.ID) &&
					!strings.HasPrefix(value, "not-applicable:")) {
				return fmt.Errorf(
					"acceptance %s evidence is insufficient for %s candidate %s",
					name, candidate.Profile, candidate.ID,
				)
			}
		}
		byID[proof.Identity] = proof
	}
	next := append([]deliveryevidence.AcceptanceRow(nil), evidence.AcceptanceMatrix...)
	for index := range next {
		row := &next[index]
		proof, found := byID[row.Identity]
		if !found {
			return fmt.Errorf("acceptance row %q lacks proof", row.Identity)
		}
		row.PositiveEvidence = proof.PositiveEvidence
		row.NegativeEvidence = proof.NegativeEvidence
		row.FailureEvidence = proof.FailureEvidence
		row.MutationEvidence = proof.MutationEvidence
		row.CompatibilityEvidence = proof.CompatibilityEvidence
		row.PreservationEvidence = proof.PreservationEvidence
		row.MigrationEvidence = proof.MigrationEvidence
		row.State = deliveryevidence.AcceptanceProved
	}
	evidence.AcceptanceMatrix = next
	return nil
}

func candidateRequiresMigrationEvidence(candidate *Candidate) bool {
	for _, observation := range candidate.Effects {
		if observation.Effect == EffectMigration || observation.Effect == EffectPersistentFormat {
			return true
		}
	}
	return false
}

func latestCandidate(record *runRecord) *Candidate {
	if len(record.Candidates) == 0 {
		return nil
	}
	return &record.Candidates[len(record.Candidates)-1]
}

func missingReviewAxes(candidate *Candidate) []deliveryevidence.ReviewAxis {
	have := map[deliveryevidence.ReviewAxis]bool{}
	for _, review := range candidate.Reviews {
		if review.Completed {
			have[review.Axis] = true
		}
	}
	var missing []deliveryevidence.ReviewAxis
	for _, axis := range candidate.RequiredReviews {
		if !have[axis] {
			missing = append(missing, axis)
		}
	}
	return missing
}

func unresolvedFindingIDs(candidate *Candidate) []string {
	decided := map[string]bool{}
	if candidate.RepairDecision != nil {
		for _, item := range candidate.RepairDecision.Findings {
			decided[item.FindingID] = true
		}
	}
	var ids []string
	for _, review := range candidate.Reviews {
		for _, finding := range review.Findings {
			if !decided[finding.ID] {
				ids = append(ids, finding.ID)
			}
		}
	}
	for _, review := range candidate.SpecialistReviews {
		for _, finding := range review.Findings {
			if !decided[finding.ID] {
				ids = append(ids, finding.ID)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

func repairDecisionRequest(candidateID string, findingIDs []string) *RepairDecisionRequest {
	ids := append([]string(nil), findingIDs...)
	return &RepairDecisionRequest{
		ID:          stableID("repair-decision", candidateID+"\x00"+strings.Join(ids, "\x00")),
		CandidateID: candidateID, FindingIDs: ids,
		Options: []RepairClass{RepairBounded, RepairCandidateChanging},
	}
}

func hasAcceptedFindings(decision *RepairDecision) bool {
	if decision == nil {
		return false
	}
	for _, item := range decision.Findings {
		if item.Disposition == FindingAccepted {
			return true
		}
	}
	return false
}

func acceptedFindingAxes(candidate Candidate) []deliveryevidence.ReviewAxis {
	accepted := map[string]bool{}
	for _, decision := range candidate.RepairDecision.Findings {
		if decision.Disposition == FindingAccepted {
			accepted[decision.FindingID] = true
		}
	}
	axes := map[deliveryevidence.ReviewAxis]bool{}
	for _, review := range candidate.Reviews {
		for _, finding := range review.Findings {
			if accepted[finding.ID] {
				axes[review.Axis] = true
			}
		}
	}
	var out []deliveryevidence.ReviewAxis
	for _, axis := range bothReviewAxes() {
		if axes[axis] {
			out = append(out, axis)
		}
	}
	return out
}

func bothReviewAxes() []deliveryevidence.ReviewAxis {
	return []deliveryevidence.ReviewAxis{deliveryevidence.ReviewStandards, deliveryevidence.ReviewSpec}
}

func containsAxis(axes []deliveryevidence.ReviewAxis, target deliveryevidence.ReviewAxis) bool {
	for _, axis := range axes {
		if axis == target {
			return true
		}
	}
	return false
}

func deliveryBranch(branch string, issue int) bool {
	for _, prefix := range []string{"chore/", "feat/", "fix/"} {
		if strings.HasPrefix(branch, prefix+"issue-"+fmt.Sprint(issue)+"-") {
			return true
		}
	}
	return false
}

func outcomeWithReason(record runRecord, state State, reason string) Outcome {
	outcome := outcomeFromRecord(record)
	outcome.State, outcome.Reason = state, reason
	return outcome
}
