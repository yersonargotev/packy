package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
	"github.com/yersonargotev/packy/internal/issuedelivery"
)

type productionPathReviewExecutor struct{}

func (productionPathReviewExecutor) Review(
	_ context.Context,
	request issuedelivery.ReviewRequest,
) (issuedelivery.CandidateReview, error) {
	return issuedelivery.CandidateReview{
		CandidateID: request.CandidateID, Axis: request.Axis,
		Findings: []deliveryevidence.ReviewFinding{}, Completed: true,
	}, nil
}

type productionPathRiskObserver struct{}

func (productionPathRiskObserver) ObserveCandidateRisk(
	_ context.Context,
	request issuedelivery.CandidateRiskRequest,
) (issuedelivery.CandidateRiskObservation, error) {
	return issuedelivery.CandidateRiskObservation{
		CandidateID: request.CandidateID, CommitSHA: request.CommitSHA, TreeSHA: request.TreeSHA,
		Effects: []issuedelivery.EffectObservation{{
			Effect: issuedelivery.EffectPassive, Evidence: "production-path test", Complete: true,
		}},
		Completed: true,
	}, nil
}

type productionValidationObservationRunner struct {
	commit string
	tree   string
}

type mutatingValidationRunner struct {
	mutate func() error
}

func (runner mutatingValidationRunner) Run(
	_ context.Context,
	_ string,
	_ deliveryevidence.SandboxFacts,
) error {
	return runner.mutate()
}

func (runner productionValidationObservationRunner) Output(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	if name != "git" {
		return nil, errors.New("unexpected validation observation command")
	}
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "rev-parse HEAD^{commit}"):
		return []byte(runner.commit + "\n"), nil
	case strings.Contains(joined, "rev-parse HEAD^{tree}"):
		return []byte(runner.tree + "\n"), nil
	case strings.Contains(joined, "status --porcelain"):
		return nil, nil
	default:
		return nil, errors.New("unexpected validation observation")
	}
}

func TestSandboxSnapshotDigestBindsActualDeclaredWrites(t *testing.T) {
	home, config := filepath.Join(t.TempDir(), "home"), filepath.Join(t.TempDir(), "config")
	for _, root := range []string{home, config} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	before, err := sandboxSnapshotDigest(home, config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(home, "cache"), []byte("written"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := sandboxSnapshotDigest(home, config)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("sandbox write was absent from the content-addressed manifest")
	}
}

func TestProductionBoundaryValidationRejectsProtectedRepositoryWrite(t *testing.T) {
	for _, relative := range []string{
		".git/config", ".git/hooks/pre-commit", ".git/refs/heads/main", "ignored.cache",
	} {
		t.Run(strings.ReplaceAll(relative, "/", "_"), func(t *testing.T) {
			repository, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			for _, directory := range []string{
				filepath.Join(repository, "scripts"), filepath.Join(repository, ".git", "objects"),
				filepath.Join(repository, ".git", "hooks"), filepath.Join(repository, ".git", "refs", "heads"),
			} {
				if err = os.MkdirAll(directory, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.WriteFile(
				filepath.Join(repository, "scripts", "validate-packy.sh"),
				[]byte("#!/bin/sh\n"), 0o700,
			); err != nil {
				t.Fatal(err)
			}
			home, config := filepath.Join(repository, "home"), filepath.Join(repository, "config")
			for _, root := range []string{home, config} {
				if err = os.MkdirAll(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			runner := &fakeRunner{outputs: [][]byte{
				nil, []byte("worktree /repo\n"), []byte(filepath.Join(repository, ".git") + "\n"),
				nil, []byte("worktree /repo\n"), []byte(filepath.Join(repository, ".git") + "\n"),
			}}
			executor := productionBoundaryExecutor{
				repository: repository, runner: runner,
				validation: mutatingValidationRunner{mutate: func() error {
					target := filepath.Join(repository, relative)
					if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
						return err
					}
					return os.WriteFile(target, []byte("changed"), 0o600)
				}},
				mu: &sync.Mutex{},
			}
			_, err = executor.ValidateBoundary(context.Background(), issuedelivery.BoundaryValidationRequest{
				CandidateID: "candidate", Boundary: issuedelivery.BoundarySecurity,
				CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40),
				HomeRoot: home, ConfigRoot: config,
			})
			if err == nil || !strings.Contains(err.Error(), "protected repository state changed") {
				t.Fatalf("protected write %q was accepted: %v", relative, err)
			}
		})
	}
}

func TestProductionValidationAdapterAdvancesRealModuleToLocalReadiness(t *testing.T) {
	repository := t.TempDir()
	var err error
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repository, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(repository, "scripts", "validate-packy.sh"),
		[]byte("#!/bin/sh\nexit 0\n"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	common, sandbox := filepath.Join(repository, ".git"), filepath.Join(repository, "sandbox")
	for _, path := range []string{
		common, sandbox, filepath.Join(sandbox, "home"), filepath.Join(sandbox, "config"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	commit, tree := strings.Repeat("b", 40), strings.Repeat("c", 40)
	git := commandGitObserver{observation: issuedelivery.GitObservation{
		CommonDir: common, Worktree: repository,
		OriginURL: "git@github.com:yersonargotev/packy.git",
		Owner:     "yersonargotev", Name: "packy", StartingBaseSHA: strings.Repeat("a", 40),
		HeadSHA: commit, TreeSHA: tree, WorkspaceClean: true,
		Branch: "chore/issue-361-production-validation",
	}}
	tracker := commandTrackerObserver{observation: issuedelivery.TrackerObservation{
		Repository: deliveryevidence.RepositoryIdentity{
			Owner: "yersonargotev", Name: "packy", NodeID: "R1",
		},
		Issue: deliveryevidence.IssueIdentity{Number: 361, NodeID: "I361"},
		Title: "Cut over", Body: "Approved authority.", State: "OPEN",
		Labels: []string{"status:approved", "type:chore"},
		Criteria: []issuedelivery.AuthorityItem{{
			Text: "Reach exact local readiness.", EvidenceLink: "issue#361:acceptance-1",
			Plan: &issuedelivery.QualificationPlan{
				OwningSeam:            "production validation adapter",
				PositiveEvidence:      "planned: exact candidate validation succeeds",
				NegativeEvidence:      "planned: mismatched candidate is rejected",
				FailureEvidence:       "planned: validation failure blocks readiness",
				MutationEvidence:      "planned: readiness receipt is persisted",
				CompatibilityEvidence: "planned: validation contract remains compatible",
				PreservationEvidence:  "planned: sandbox preserves operator configuration",
			},
		}},
		Exclusions: []issuedelivery.AuthorityItem{}, Dependencies: []issuedelivery.DependencyObservation{},
		References: []issuedelivery.ReferenceObservation{}, Ambiguities: []issuedelivery.AuthorityItem{},
	}}
	clock := &commandClock{now: time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)}
	focusedRunner, exhaustiveRunner := &fakeValidationRunner{}, &fakeValidationRunner{}
	validationAdapter := &productionValidationExecutor{
		repository: repository,
		runner:     productionValidationObservationRunner{commit: commit, tree: tree},
		focused:    focusedRunner, exhaustive: exhaustiveRunner, now: clock.Now,
	}
	module, err := issuedelivery.New(issuedelivery.Config{
		Git: git, GitHub: tracker, Review: productionPathReviewExecutor{},
		Validation: validationAdapter,
		Risk:       productionPathRiskObserver{}, Clock: clock, SandboxRoot: sandbox,
		DeclaredProfile: deliveryevidence.RiskLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := issuedelivery.Request{RepositoryPath: repository, IssueNumber: 361}
	var outcome issuedelivery.Outcome
	var lastCandidateID, acceptanceID string
	sawTraceabilityPreflight := false
	for attempts := 0; attempts < 8; attempts++ {
		outcome, err = module.Advance(context.Background(), request)
		if err != nil {
			if !strings.Contains(err.Error(), "acceptance review content") {
				t.Fatal(err)
			}
			sawTraceabilityPreflight = true
			validationAdapter.acceptance = []issuedelivery.AcceptanceProof{{
				Identity:              acceptanceID,
				PositiveEvidence:      lastCandidateID + ": TestProductionValidationAdapter positive result",
				NegativeEvidence:      lastCandidateID + ": reviewed negative-path test result",
				FailureEvidence:       lastCandidateID + ": reviewed failure-path test result",
				MutationEvidence:      lastCandidateID + ": reviewed persisted-mutation test result",
				CompatibilityEvidence: lastCandidateID + ": reviewed compatibility test result",
				PreservationEvidence:  lastCandidateID + ": reviewed preservation test result",
				MigrationEvidence:     lastCandidateID + ": reviewed non-migration applicability",
			}}
			continue
		}
		if outcome.LocalReadiness != nil {
			break
		}
		if outcome.Candidate != nil {
			lastCandidateID = outcome.Candidate.ID
		}
		if outcome.Evidence != nil && len(outcome.Evidence.AcceptanceMatrix) == 1 {
			acceptanceID = outcome.Evidence.AcceptanceMatrix[0].Identity
		}
	}
	if !sawTraceabilityPreflight || outcome.LocalReadiness == nil || outcome.State != issuedelivery.StateWaiting ||
		outcome.Evidence == nil ||
		outcome.Evidence.AcceptanceMatrix[0].State != deliveryevidence.AcceptanceProved ||
		len(focusedRunner.calls) != 1 || len(exhaustiveRunner.calls) != 1 {
		t.Fatalf("production-shaped path did not reach local readiness: %#v", outcome)
	}
}

func TestProductionTrackerObserverBindsSelectedSpecification(t *testing.T) {
	repository, _ := json.Marshal(map[string]any{
		"nameWithOwner": "yersonargotev/packy", "id": "R1",
	})
	issue, _ := json.Marshal(map[string]any{
		"number": 361, "id": "I361", "title": "Cut over", "body": "## Acceptance criteria\n- [ ] Use Advance.",
		"state": "OPEN", "url": "https://github.com/yersonargotev/packy/issues/361",
		"labels":    []map[string]string{{"name": "status:approved"}},
		"blockedBy": map[string]any{"nodes": []any{}, "totalCount": 0},
	})
	specification, _ := json.Marshal(map[string]any{
		"number": 354, "id": "I354", "title": "Delivery specification", "body": "Accepted orchestration.",
		"state": "OPEN", "url": "https://github.com/yersonargotev/packy/issues/354",
		"labels": []map[string]string{{"name": "type:spec"}, {"name": "status:approved"}},
	})
	runner := &fakeRunner{outputs: [][]byte{repository, issue, specification}}
	observer := productionTrackerObserver{runner: runner, specificationNumber: 354}

	observation, err := observer.ObserveIssue(context.Background(), issuedelivery.GitObservation{
		Owner: "yersonargotev", Name: "packy",
	}, 361)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Specification == nil ||
		observation.Specification.Identity != (deliveryevidence.SpecIdentity{Number: 354, NodeID: "I354"}) ||
		observation.Specification.URL != "https://github.com/yersonargotev/packy/issues/354" {
		t.Fatalf("specification observation = %#v", observation.Specification)
	}
	if len(runner.calls) != 3 || !strings.Contains(runner.calls[2], "issue view 354") {
		t.Fatalf("tracker calls = %v", runner.calls)
	}
}

func TestParseIssueAuthorityExtractsStableCriteriaExclusionsAmbiguitiesAndReferences(t *testing.T) {
	body := `## Parent

- #354

## Acceptance criteria

- [ ] Complete issue delivery invokes Advance.
- [ ] Choose one of two materially different outputs?

## Out of scope

- A public packy deliver command.
`
	criteria, exclusions, ambiguities, references := parseIssueAuthority(
		body,
		"https://github.com/yersonargotev/packy/issues/361",
		"yersonargotev/packy",
	)
	if len(criteria) != 1 || criteria[0].Text != "Complete issue delivery invokes Advance." ||
		criteria[0].EvidenceLink != "https://github.com/yersonargotev/packy/issues/361#acceptance-1" {
		t.Fatalf("criteria = %#v", criteria)
	}
	if len(ambiguities) != 1 || !strings.HasSuffix(ambiguities[0].Text, "?") {
		t.Fatalf("ambiguities = %#v", ambiguities)
	}
	if len(exclusions) != 1 || exclusions[0].Text != "A public packy deliver command." {
		t.Fatalf("exclusions = %#v", exclusions)
	}
	if len(references) != 1 || references[0].URL != "https://github.com/yersonargotev/packy/issues/354" {
		t.Fatalf("references = %#v", references)
	}
}

func TestSuppliedReviewExecutorsWaitUntilExactCandidateContentExists(t *testing.T) {
	reviewExecutor := suppliedReviewExecutor{reviews: []issuedelivery.CandidateReview{{
		CandidateID: "candidate-1", Axis: deliveryevidence.ReviewStandards, Completed: true,
	}}}
	waiting, err := reviewExecutor.Review(context.Background(), issuedelivery.ReviewRequest{
		CandidateID: "candidate-2", Axis: deliveryevidence.ReviewStandards,
	})
	if err != nil || waiting.Completed || waiting.Findings == nil ||
		waiting.CandidateID != "candidate-2" {
		t.Fatalf("waiting review = %#v, err=%v", waiting, err)
	}
	exact, err := reviewExecutor.Review(context.Background(), issuedelivery.ReviewRequest{
		CandidateID: "candidate-1", Axis: deliveryevidence.ReviewStandards,
	})
	if err != nil || !exact.Completed || exact.Findings == nil {
		t.Fatalf("exact review = %#v, err=%v", exact, err)
	}

	specialistExecutor := suppliedSpecialistExecutor{}
	specialist, err := specialistExecutor.ReviewSpecialist(
		context.Background(),
		issuedelivery.SpecialistReviewRequest{
			CandidateID: "candidate-1", Boundary: issuedelivery.BoundarySecurity,
			Specialist: "security-specialist",
		},
	)
	if err != nil || specialist.Completed || specialist.Findings == nil ||
		specialist.Boundary != issuedelivery.BoundarySecurity {
		t.Fatalf("waiting specialist = %#v, err=%v", specialist, err)
	}
}
