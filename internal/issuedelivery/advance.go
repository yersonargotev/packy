package issuedelivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

func (m *Module) Advance(ctx context.Context, request Request) (Outcome, error) {
	if ctx == nil {
		return Outcome{}, errors.New("Advance requires a context")
	}
	if request.IssueNumber <= 0 || strings.TrimSpace(request.RepositoryPath) == "" {
		return Outcome{}, errors.New("Advance requires a repository path and positive issue number")
	}
	git, err := m.git.ObserveGit(ctx, request.RepositoryPath)
	if err != nil {
		return Outcome{}, fmt.Errorf("observe Git: %w", err)
	}

	var outcome Outcome
	err = m.store.withIssueLock(ctx, git.CommonDir, request.IssueNumber, func() error {
		tracker, err := m.github.ObserveIssue(ctx, git, request.IssueNumber)
		if err != nil {
			return fmt.Errorf("observe GitHub issue: %w", err)
		}
		if tracker.Issue.Number != request.IssueNumber {
			return fmt.Errorf("GitHub observer returned issue #%d for requested issue #%d", tracker.Issue.Number, request.IssueNumber)
		}
		activeID, activeData, found, err := m.store.loadActive(git.CommonDir, request.IssueNumber)
		if err != nil {
			return err
		}
		var active runRecord
		if found {
			active, err = decodeRun(activeData)
			if err != nil {
				return err
			}
			if active.ID != activeID || active.Repository != tracker.Repository || active.Issue != tracker.Issue {
				return errors.New("active issue delivery run identity does not match current authority")
			}
		}
		compiled, err := compileAuthority(git, tracker, active.Decisions, request.Decision)
		if err != nil {
			return err
		}
		if found {
			if active.AuthoritySHA256 == compiled.hash {
				outcome = outcomeFromRecord(active)
				outcome.Observations = observationsFrom(git, tracker, compiled.hash)
				return nil
			}
		}

		nowStarted := m.clock.Now().UTC()
		supersedes := ""
		if found {
			supersedes = active.ID
		}
		runID := runIdentity(tracker.Repository, tracker.Issue, compiled.hash, supersedes)
		nowCompleted := m.clock.Now().UTC()
		record := runRecord{
			Schema: runSchema, ID: runID, Repository: tracker.Repository, Issue: tracker.Issue,
			AuthoritySHA256: compiled.hash, State: compiled.state, Reason: compiled.reason,
			Evidence: &compiled.evidence, PendingDecision: compiled.pending,
			Decisions:    append([]Decision{}, compiled.decisions...),
			Observations: observationsFrom(git, tracker, compiled.hash),
			Timing: []Timing{{
				Sequence: 1, Phase: "qualification", To: compiled.state,
				StartedAt: nowStarted.Format(timeFormat), CompletedAt: nowCompleted.Format(timeFormat),
			}},
			CreatedAt: nowStarted.Format(timeFormat), UpdatedAt: nowCompleted.Format(timeFormat),
		}
		if compiled.pending != nil {
			record.Evidence = nil
		}
		if found {
			record.SupersedesRunID = active.ID
		}
		data, err := encodeRun(record)
		if err != nil {
			return err
		}
		if err := m.store.storeAndActivate(git.CommonDir, request.IssueNumber, runID, data); err != nil {
			return err
		}
		outcome = outcomeFromRecord(record)
		return nil
	})
	if errors.Is(err, errIssueRunActive) {
		return Outcome{State: StateWaiting, Reason: "another Advance call is active for this issue"}, nil
	}
	return outcome, err
}

const timeFormat = "2006-01-02T15:04:05.000000000Z"

func runIdentity(
	repository deliveryevidence.RepositoryIdentity,
	issue deliveryevidence.IssueIdentity,
	authorityHash, supersedes string,
) string {
	sum := sha256.Sum256([]byte(
		repository.NodeID + "\x00" + issue.NodeID + "\x00" + authorityHash + "\x00" + supersedes,
	))
	return hex.EncodeToString(sum[:])
}

func outcomeFromRecord(record runRecord) Outcome {
	return Outcome{
		RunID: record.ID, State: record.State, Reason: record.Reason,
		SupersedesRunID: record.SupersedesRunID, Decision: record.PendingDecision,
		Evidence: record.Evidence, Observations: record.Observations,
		Timing: append([]Timing(nil), record.Timing...),
	}
}

func observationsFrom(git GitObservation, tracker TrackerObservation, authoritySHA256 string) Observations {
	return Observations{
		Repository: tracker.Repository, Issue: tracker.Issue, AuthoritySHA256: authoritySHA256,
		CommitSHA: git.HeadSHA, TreeSHA: git.TreeSHA, WorkspaceClean: git.WorkspaceClean,
	}
}
