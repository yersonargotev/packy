package release_test

import (
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
)

func TestVerifyRetainedRecoveryBindsOriginalCandidate(t *testing.T) {
	candidate := mustCandidate(t, fixtureObservation())
	plan := release.RecoveryPublicationPlan{SchemaVersion: 1, Tag: candidate.Version, TargetCommit: candidate.Commit, Draft: true, SourceRunID: "12345", AttestationSourceRef: "refs/tags/" + candidate.Version, CandidateID: candidate.ID, CandidateAssets: candidate.Subjects}
	plan.Attestation = "attestation.bundle.jsonl"
	plan.Homebrew.Repository, plan.Homebrew.Path, plan.Homebrew.SHA256 = "yersonargotev/homebrew-tap", "Formula/packy.rb", strings.Repeat("a", 64)
	draft := release.RecoveryDraftBase{SchemaVersion: 1, CandidateID: candidate.ID, Provenance: release.ProvenanceFor(candidate), TargetCommit: candidate.Commit, SourceRunID: "12345", AttestationSourceRef: "refs/tags/" + candidate.Version, PublicationPlan: plan}
	base := release.RetainedRecoveryObservation{Tag: candidate.Version, Commit: candidate.Commit, OriginalRunID: "12345", Candidate: candidate, Provenance: release.ProvenanceFor(candidate), PublicationPlan: plan, DraftBase: draft, Subjects: candidate.Subjects}
	if err := release.VerifyRetainedRecovery(base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*release.RetainedRecoveryObservation)
	}{
		{"run", func(o *release.RetainedRecoveryObservation) { o.OriginalRunID = "99999" }},
		{"plan", func(o *release.RetainedRecoveryObservation) { o.PublicationPlan.TargetCommit = strings.Repeat("e", 40) }},
		{"draft", func(o *release.RetainedRecoveryObservation) { o.DraftBase.CandidateID = strings.Repeat("e", 64) }},
		{"digest", func(o *release.RetainedRecoveryObservation) {
			o.Subjects = append([]release.Subject(nil), o.Subjects...)
			o.Subjects[0].SHA256 = strings.Repeat("e", 64)
		}},
		{"attestation name", func(o *release.RetainedRecoveryObservation) { o.PublicationPlan.Attestation = "other.bundle" }},
		{"homebrew repository", func(o *release.RetainedRecoveryObservation) { o.PublicationPlan.Homebrew.Repository = "attacker/tap" }},
		{"homebrew path", func(o *release.RetainedRecoveryObservation) { o.PublicationPlan.Homebrew.Path = "Formula/other.rb" }},
		{"homebrew digest", func(o *release.RetainedRecoveryObservation) {
			o.PublicationPlan.Homebrew.SHA256 = strings.Repeat("A", 64)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observed := base
			tc.mutate(&observed)
			if release.VerifyRetainedRecovery(observed) == nil {
				t.Fatal("divergence accepted")
			}
		})
	}
}
