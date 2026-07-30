package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
	"github.com/yersonargotev/packy/internal/issuedelivery"
)

type advanceOptions struct {
	RepositoryPath          string
	IssueNumber             int
	SpecificationNumber     int
	SandboxRoot             string
	DeclaredProfile         deliveryevidence.DeliveryRiskProfile
	Decision                *issuedelivery.Decision
	Repair                  *issuedelivery.RepairDecision
	Reviews                 []issuedelivery.CandidateReview
	Specialists             []issuedelivery.SpecialistReview
	Acceptance              []issuedelivery.AcceptanceProof
	QualificationReview     *issuedelivery.QualificationReview
	QualificationCorrection *issuedelivery.QualificationCorrection
	CIFailureAttributions   []advanceCIFailureAttribution
	AuthorizeRemote         bool
}

type advanceFactory func(advanceOptions) (issueDeliveryAdvancer, error)

type advanceReviewContent struct {
	Reviews                 []issuedelivery.CandidateReview        `json:"reviews"`
	Specialists             []issuedelivery.SpecialistReview       `json:"specialist_reviews"`
	Acceptance              []issuedelivery.AcceptanceProof        `json:"acceptance_proofs"`
	QualificationReview     *issuedelivery.QualificationReview     `json:"qualification_review,omitempty"`
	QualificationCorrection *issuedelivery.QualificationCorrection `json:"qualification_correction,omitempty"`
}

type advanceCIFailureAttribution struct {
	CheckIdentity string                             `json:"check_identity"`
	RunID         int64                              `json:"run_id"`
	HeadSHA       string                             `json:"head_sha"`
	DetailsURL    string                             `json:"details_url"`
	Attribution   issuedelivery.CIFailureAttribution `json:"attribution"`
}

type advanceReport struct {
	RunID                    string                                        `json:"run_id"`
	State                    issuedelivery.State                           `json:"state"`
	Reason                   string                                        `json:"reason"`
	SupersedesRunID          string                                        `json:"supersedes_run_id,omitempty"`
	Decision                 *issuedelivery.DecisionRequest                `json:"decision,omitempty"`
	Repair                   *issuedelivery.RepairDecisionRequest          `json:"repair,omitempty"`
	QualificationCorrection  *issuedelivery.QualificationCorrectionRequest `json:"qualification_correction,omitempty"`
	QualificationApproved    bool                                          `json:"qualification_approved,omitempty"`
	QualificationReviews     []issuedelivery.QualificationReview           `json:"qualification_reviews,omitempty"`
	QualificationCorrections []issuedelivery.QualificationCorrection       `json:"qualification_corrections,omitempty"`
	Evidence                 *deliveryevidence.Bundle                      `json:"evidence,omitempty"`
	Observations             issuedelivery.Observations                    `json:"observations"`
	Candidate                *issuedelivery.Candidate                      `json:"candidate,omitempty"`
	LocalReadiness           *issuedelivery.LocalReadiness                 `json:"local_readiness,omitempty"`
	NonLocal                 *issuedelivery.NonLocalDelivery               `json:"non_local,omitempty"`
	Timing                   []issuedelivery.Timing                        `json:"timing"`
	TimingReport             issuedelivery.TimingReport                    `json:"timing_report"`
}

func (c command) advance(ctx context.Context, args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("deliveryevidence advance", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var options advanceOptions
	var profile, decisionPath, repairPath, reviewPath, ciAttributionPath string
	f.StringVar(&options.RepositoryPath, "repository", ".", "repository to observe")
	f.IntVar(&options.IssueNumber, "issue", 0, "approved Packy issue number")
	f.IntVar(&options.SpecificationNumber, "spec", 0, "approved governing specification issue number")
	f.StringVar(&profile, "risk-profile", string(deliveryevidence.RiskStandard), "declared low-risk, standard, or high-risk profile")
	f.StringVar(&decisionPath, "decision", "", "typed semantic qualification decision")
	f.StringVar(&repairPath, "repair", "", "typed finding adjudication and repair classification")
	f.StringVar(&reviewPath, "review-content", "", "candidate and specialist review content")
	f.StringVar(&ciAttributionPath, "ci-attribution", "", "typed attribution of exact failed CI runs")
	f.BoolVar(&options.AuthorizeRemote, "authorize-non-local", false, "authorize deterministic delivery effects after exact local readiness")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || options.IssueNumber <= 0 || strings.TrimSpace(options.RepositoryPath) == "" {
		return errors.New("issue and repository are required and positional arguments are forbidden")
	}
	if options.SpecificationNumber < 0 || options.SpecificationNumber == options.IssueNumber {
		return errors.New("specification must be a distinct positive issue number when supplied")
	}
	repositoryPath, err := filepath.Abs(options.RepositoryPath)
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	options.RepositoryPath, err = filepath.EvalSymlinks(repositoryPath)
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	options.RepositoryPath = filepath.Clean(options.RepositoryPath)
	options.DeclaredProfile = deliveryevidence.DeliveryRiskProfile(profile)
	if options.DeclaredProfile != deliveryevidence.RiskLow &&
		options.DeclaredProfile != deliveryevidence.RiskStandard &&
		options.DeclaredProfile != deliveryevidence.RiskHigh {
		return fmt.Errorf("risk profile %q is invalid", profile)
	}
	if err := decodeOptionalExactJSON(decisionPath, &options.Decision); err != nil {
		return fmt.Errorf("decode decision: %w", err)
	}
	if err := decodeOptionalExactJSON(repairPath, &options.Repair); err != nil {
		return fmt.Errorf("decode repair: %w", err)
	}
	if reviewPath != "" {
		var content advanceReviewContent
		if err := decodeExactJSONFile(reviewPath, &content); err != nil {
			return fmt.Errorf("decode review content: %w", err)
		}
		options.Reviews = content.Reviews
		options.Specialists = content.Specialists
		options.Acceptance = content.Acceptance
		options.QualificationReview = content.QualificationReview
		options.QualificationCorrection = content.QualificationCorrection
	}
	if ciAttributionPath != "" {
		if err := decodeExactJSONFile(ciAttributionPath, &options.CIFailureAttributions); err != nil {
			return fmt.Errorf("decode CI attribution: %w", err)
		}
		if options.CIFailureAttributions == nil {
			return errors.New("CI attribution requires an explicit array")
		}
	}
	if options.Decision != nil && options.Repair != nil {
		return errors.New("one Advance call cannot submit both a qualification decision and repair adjudication")
	}
	if c.AdvanceFactory == nil {
		return errors.New("Advance adapter is unavailable")
	}
	advancer, err := c.AdvanceFactory(options)
	if err != nil {
		return fmt.Errorf("configure Advance: %w", err)
	}
	request := issuedelivery.Request{
		RepositoryPath:          options.RepositoryPath,
		IssueNumber:             options.IssueNumber,
		Decision:                options.Decision,
		Repair:                  options.Repair,
		QualificationReview:     options.QualificationReview,
		QualificationCorrection: options.QualificationCorrection,
	}
	outcome, err := advancer.Advance(ctx, request)
	if err != nil {
		return err
	}
	if options.AuthorizeRemote && outcome.LocalReadiness != nil && outcome.NonLocal == nil {
		request.Decision, request.Repair = nil, nil
		request.QualificationReview, request.QualificationCorrection = nil, nil
		request.NonLocal = nonLocalAuthorization(outcome)
		outcome, err = advancer.Advance(ctx, request)
		if err != nil {
			return err
		}
	}
	report, err := reportFromOutcome(outcome, c.now())
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = stdout.Write(raw)
	return err
}

func nonLocalAuthorization(outcome issuedelivery.Outcome) *issuedelivery.NonLocalAuthorization {
	readiness := outcome.LocalReadiness
	if readiness == nil || outcome.Candidate == nil {
		return nil
	}
	return &issuedelivery.NonLocalAuthorization{
		RunID: outcome.RunID, CandidateID: readiness.CandidateID,
		CommitSHA: readiness.CommitSHA, TreeSHA: readiness.TreeSHA,
		Branch: readiness.Branch, LocalReadyAt: readiness.ReadyAt,
	}
}

func reportFromOutcome(outcome issuedelivery.Outcome, now time.Time) (advanceReport, error) {
	timingReport, err := issuedelivery.BuildTimingReport(outcome.Timing, now)
	if err != nil {
		return advanceReport{}, fmt.Errorf("build timing report: %w", err)
	}
	return advanceReport{
		RunID: outcome.RunID, State: outcome.State, Reason: outcome.Reason,
		SupersedesRunID: outcome.SupersedesRunID, Decision: outcome.Decision,
		Repair: outcome.Repair, Evidence: outcome.Evidence, Observations: outcome.Observations,
		QualificationCorrection:  outcome.QualificationCorrection,
		QualificationApproved:    outcome.QualificationApproved,
		QualificationReviews:     outcome.QualificationReviews,
		QualificationCorrections: outcome.QualificationCorrections,
		Candidate:                outcome.Candidate, LocalReadiness: outcome.LocalReadiness,
		NonLocal: outcome.NonLocal, Timing: outcome.Timing, TimingReport: timingReport,
	}, nil
}

func decodeOptionalExactJSON[T any](path string, target **T) error {
	if path == "" {
		return nil
	}
	var value T
	if err := decodeExactJSONFile(path, &value); err != nil {
		return err
	}
	*target = &value
	return nil
}

func decodeExactJSONFile(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("input must contain exactly one JSON value")
	}
	return nil
}
