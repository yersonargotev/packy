package release

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
)

var workflowRunIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

type RecoveryPublicationPlan struct {
	SchemaVersion        int       `json:"schema_version"`
	Tag                  string    `json:"tag"`
	TargetCommit         string    `json:"target_commit"`
	Draft                bool      `json:"draft"`
	SourceRunID          string    `json:"source_run_id"`
	AttestationSourceRef string    `json:"attestation_source_ref"`
	CandidateID          string    `json:"candidate_id"`
	CandidateAssets      []Subject `json:"candidate_assets"`
	Attestation          string    `json:"attestation"`
	Homebrew             struct {
		Repository string `json:"repository"`
		Path       string `json:"path"`
		SHA256     string `json:"sha256"`
	} `json:"homebrew"`
}

type RecoveryDraftBase struct {
	SchemaVersion        int                     `json:"schema_version"`
	CandidateID          string                  `json:"candidate_id"`
	Provenance           Provenance              `json:"provenance"`
	TargetCommit         string                  `json:"target_commit"`
	SourceRunID          string                  `json:"source_run_id"`
	AttestationSourceRef string                  `json:"attestation_source_ref"`
	PublicationPlan      RecoveryPublicationPlan `json:"publication_plan"`
}

type RetainedRecoveryObservation struct {
	Tag             string
	Commit          string
	OriginalRunID   string
	Candidate       Candidate
	Provenance      Provenance
	PublicationPlan RecoveryPublicationPlan
	DraftBase       RecoveryDraftBase
	Subjects        []Subject
}

func VerifyRetainedRecovery(observed RetainedRecoveryObservation) error {
	if !workflowRunIDPattern.MatchString(observed.OriginalRunID) {
		return errors.New("original retained candidate run ID is invalid")
	}
	if err := VerifyProvenance(observed.Candidate, observed.Provenance); err != nil {
		return fmt.Errorf("retained provenance: %w", err)
	}
	wantRef := "refs/tags/" + observed.Tag
	plan := observed.PublicationPlan
	draft := observed.DraftBase
	if observed.Candidate.Version != observed.Tag || observed.Candidate.Commit != observed.Commit ||
		plan.SchemaVersion != 1 || plan.Tag != observed.Tag || plan.TargetCommit != observed.Commit || !plan.Draft ||
		plan.SourceRunID != observed.OriginalRunID || plan.AttestationSourceRef != wantRef ||
		plan.CandidateID != observed.Candidate.ID || !reflect.DeepEqual(plan.CandidateAssets, observed.Candidate.Subjects) ||
		plan.Attestation != "attestation.bundle.jsonl" || plan.Homebrew.Repository != "yersonargotev/homebrew-tap" ||
		plan.Homebrew.Path != "Formula/packy.rb" || !digestPattern.MatchString(plan.Homebrew.SHA256) {
		return errors.New("retained publication plan diverges from the sealed recovery identity or original run")
	}
	if draft.SchemaVersion != 1 || draft.CandidateID != observed.Candidate.ID ||
		!reflect.DeepEqual(draft.Provenance, observed.Provenance) || draft.TargetCommit != observed.Commit ||
		draft.SourceRunID != observed.OriginalRunID || draft.AttestationSourceRef != wantRef ||
		!reflect.DeepEqual(draft.PublicationPlan, plan) {
		return errors.New("retained draft base diverges from the sealed recovery identity or original run")
	}
	actual := append([]Subject(nil), observed.Subjects...)
	sort.Slice(actual, func(i, j int) bool { return actual[i].Name < actual[j].Name })
	if !reflect.DeepEqual(actual, observed.Candidate.Subjects) {
		return errors.New("retained candidate subject set or digest diverges")
	}
	return nil
}
