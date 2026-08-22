package managedpackpromotion_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestPromoteReturnsOneProposalForARegisteredValidatedImmutableRelease(t *testing.T) {
	repositoryRoot := registryFixture(t, "addy", "owner/addy")
	coordinate := mustCoordinate(t, "addy@1.2.3")
	acquisition := validAcquisition()
	acquisition.Release.Project = "OWNER/ADDY"
	cleanupCalls := 0
	acquisition.Cleanup = func() error { cleanupCalls++; return nil }

	acquirer := &acquirerStub{acquisition: acquisition}
	validator := &validatorStub{validation: managedpack.Validation{Manifest: managedpack.Manifest{ID: "addy", Version: "1.2.3"}}}
	preparer := &preparerStub{result: managedpackpromotion.CandidatePreparation{
		Candidate: &managedpackpromotion.Candidate{ID: "sealed-candidate", Summary: "sealed promotion summary"},
		Cleanup:   func() error { return nil },
	}}
	publisher := &publisherStub{result: managedpackpromotion.Publication{
		Proposal: &managedpackpromotion.Proposal{Branch: "promote/addy-1.2.3", Number: 42, URL: "https://example.test/pull/42", HeadSHA: strings.Repeat("9", 40)},
	}}
	module := managedpackpromotion.NewModule(acquirer, validator, preparer, publisher)

	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: repositoryRoot,
		Coordinate:     coordinate,
	})
	if err != nil {
		t.Fatalf("Promote returned an error: %v", err)
	}
	if result.Status != managedpackpromotion.StatusProposal || result.Proposal == nil || result.Proposal.Number != 42 {
		t.Fatalf("Promote result = %#v, want proposal 42", result)
	}
	if acquirer.project != "owner/addy" || acquirer.coordinate != coordinate {
		t.Fatalf("acquisition input = %q %#v", acquirer.project, acquirer.coordinate)
	}
	if preparer.acquisition.Release.Project != "owner/addy" {
		t.Fatalf("candidate project = %q, want canonical registry identity", preparer.acquisition.Release.Project)
	}
	if publisher.calls != 1 {
		t.Fatalf("Publisher calls = %d, want 1", publisher.calls)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
}

func TestPromoteAcceptsALightweightTagThatDirectlyIdentifiesTheCommit(t *testing.T) {
	acquisition := validAcquisition()
	acquisition.Release.TagRef = managedpackpromotion.GitObject{
		SHA: acquisition.Release.CommitSHA, Type: managedpackpromotion.GitObjectCommit,
	}
	acquisition.Release.TagObjects = nil
	module := managedpackpromotion.NewModule(
		&acquirerStub{acquisition: acquisition},
		validValidator(),
		&preparerStub{result: validCandidatePreparation()},
		&publisherStub{result: managedpackpromotion.Publication{Proposal: &managedpackpromotion.Proposal{Number: 7}}},
	)
	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
		Coordinate:     mustCoordinate(t, "addy@1.2.3"),
	})
	if err != nil || result.Status != managedpackpromotion.StatusProposal {
		t.Fatalf("Promote = %#v, %v; want proposal", result, err)
	}
}

func TestPromoteAcceptsACompleteNestedAnnotatedTagChain(t *testing.T) {
	acquisition := validAcquisition()
	secondTag := strings.Repeat("b", 40)
	acquisition.Release.TagObjects = []managedpackpromotion.TagObject{
		{SHA: acquisition.Release.TagRef.SHA, TargetSHA: secondTag, TargetType: managedpackpromotion.GitObjectTag},
		{SHA: secondTag, TargetSHA: acquisition.Release.CommitSHA, TargetType: managedpackpromotion.GitObjectCommit},
	}
	module := managedpackpromotion.NewModule(
		&acquirerStub{acquisition: acquisition}, validValidator(),
		&preparerStub{result: validCandidatePreparation()},
		&publisherStub{result: managedpackpromotion.Publication{Proposal: &managedpackpromotion.Proposal{Number: 8}}},
	)
	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
		Coordinate:     mustCoordinate(t, "addy@1.2.3"),
	})
	if err != nil || result.Status != managedpackpromotion.StatusProposal {
		t.Fatalf("Promote = %#v, %v; want proposal", result, err)
	}
}

func TestPromoteRejectsAPrereleaseCoordinateEvenWhenRemoteFlagsClaimStable(t *testing.T) {
	acquisition := validAcquisition()
	acquisition.Release.Tag = "pack-v1.2.3-beta.1"
	publisher := &publisherStub{}
	module := managedpackpromotion.NewModule(
		&acquirerStub{acquisition: acquisition}, &validatorStub{}, &preparerStub{}, publisher,
	)
	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
		Coordinate:     mustCoordinate(t, "addy@1.2.3-beta.1"),
	})
	if err != nil {
		t.Fatalf("Promote returned an error: %v", err)
	}
	if result.Status != managedpackpromotion.StatusRejected || result.Rejection == nil || result.Rejection.Gate != managedpackpromotion.GateRelease {
		t.Fatalf("Promote result = %#v, want release rejection", result)
	}
	if publisher.calls != 0 {
		t.Fatalf("Publisher calls = %d, want 0", publisher.calls)
	}
}

func TestPromoteRequiresCompleteAcquiredLocalTreesAndCleanup(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*managedpackpromotion.Acquisition)
	}{
		{"project root", func(value *managedpackpromotion.Acquisition) { value.ProjectRoot = "" }},
		{"origin roots", func(value *managedpackpromotion.Acquisition) { value.OriginRoots = nil }},
		{"cleanup", func(value *managedpackpromotion.Acquisition) { value.Cleanup = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			acquisition := validAcquisition()
			test.mutate(&acquisition)
			publisher := &publisherStub{}
			validator := validValidator()
			module := managedpackpromotion.NewModule(
				&acquirerStub{acquisition: acquisition}, validator, &preparerStub{}, publisher,
			)
			result, err := module.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
				Coordinate:     mustCoordinate(t, "addy@1.2.3"),
			})
			if err == nil {
				t.Fatalf("Promote result = %#v, want malformed acquisition error", result)
			}
			if result.Status != "" || validator.calls != 0 || publisher.calls != 0 {
				t.Fatalf("Promote result=%#v validator calls=%d publisher calls=%d after malformed acquisition", result, validator.calls, publisher.calls)
			}
		})
	}
}

func TestPromoteCleansAPreparedNoChangeWithoutCallingPublisher(t *testing.T) {
	repositoryRoot := registryFixture(t, "addy", "owner/addy")
	acquisitionCleanup := 0
	acquisition := validAcquisition()
	acquisition.Cleanup = func() error { acquisitionCleanup++; return nil }
	candidateCleanup := 0
	publisher := &publisherStub{}
	module := managedpackpromotion.NewModule(
		&acquirerStub{acquisition: acquisition},
		&validatorStub{validation: managedpack.Validation{Manifest: managedpack.Manifest{ID: "addy", Version: "1.2.3"}}},
		&preparerStub{result: managedpackpromotion.CandidatePreparation{
			NoChangeReason: "exact generation is already admitted",
			Cleanup:        func() error { candidateCleanup++; return nil },
		}},
		publisher,
	)

	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: repositoryRoot,
		Coordinate:     mustCoordinate(t, "addy@1.2.3"),
	})
	if err != nil {
		t.Fatalf("Promote returned an error: %v", err)
	}
	if result.Status != managedpackpromotion.StatusNoChange || result.Reason != "exact generation is already admitted" {
		t.Fatalf("Promote result = %#v, want candidate no-change", result)
	}
	if publisher.calls != 0 {
		t.Fatalf("Publisher calls = %d, want 0", publisher.calls)
	}
	if acquisitionCleanup != 1 || candidateCleanup != 1 {
		t.Fatalf("cleanup calls acquisition=%d candidate=%d, want 1 each", acquisitionCleanup, candidateCleanup)
	}
}

func TestPromoteReturnsPublisherNoChangeAndCleansTheCandidate(t *testing.T) {
	candidateCleanup := 0
	preparation := validCandidatePreparation()
	preparation.Cleanup = func() error { candidateCleanup++; return nil }
	publisher := &publisherStub{result: managedpackpromotion.Publication{NoChangeReason: "owned proposal already has the exact head"}}
	module := managedpackpromotion.NewModule(
		&acquirerStub{acquisition: validAcquisition()}, validValidator(),
		&preparerStub{result: preparation}, publisher,
	)
	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
		Coordinate:     mustCoordinate(t, "addy@1.2.3"),
	})
	if err != nil {
		t.Fatalf("Promote returned an error: %v", err)
	}
	if result.Status != managedpackpromotion.StatusNoChange || result.Reason != "owned proposal already has the exact head" {
		t.Fatalf("Promote result = %#v, want publisher no-change", result)
	}
	if publisher.calls != 1 || candidateCleanup != 1 {
		t.Fatalf("publisher calls=%d candidate cleanup=%d, want 1 each", publisher.calls, candidateCleanup)
	}
}

func TestPromoteRefusesAnUnsealedCandidateSummaryBeforePublication(t *testing.T) {
	candidateCleanup := 0
	preparation := validCandidatePreparation()
	preparation.Candidate.Summary = "  "
	preparation.Cleanup = func() error { candidateCleanup++; return nil }
	publisher := &publisherStub{result: managedpackpromotion.Publication{Proposal: &managedpackpromotion.Proposal{Number: 9}}}
	module := managedpackpromotion.NewModule(
		&acquirerStub{acquisition: validAcquisition()}, validValidator(),
		&preparerStub{result: preparation}, publisher,
	)
	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
		Coordinate:     mustCoordinate(t, "addy@1.2.3"),
	})
	if err == nil {
		t.Fatalf("Promote result = %#v, want unsealed candidate error", result)
	}
	if result.Status != "" || publisher.calls != 0 || candidateCleanup != 1 {
		t.Fatalf("result=%#v publisher calls=%d candidate cleanup=%d", result, publisher.calls, candidateCleanup)
	}
}

func TestPromoteSurfacesCleanupFailuresAsOperationalErrors(t *testing.T) {
	cleanupFailure := errors.New("cleanup failed")
	for _, test := range []struct {
		name        string
		acquisition managedpackpromotion.Acquisition
		preparation managedpackpromotion.CandidatePreparation
	}{
		{
			name: "acquisition cleanup",
			acquisition: func() managedpackpromotion.Acquisition {
				value := validAcquisition()
				value.Cleanup = func() error { return cleanupFailure }
				return value
			}(),
			preparation: validCandidatePreparation(),
		},
		{
			name: "candidate cleanup", acquisition: validAcquisition(),
			preparation: managedpackpromotion.CandidatePreparation{
				NoChangeReason: "already admitted", Cleanup: func() error { return cleanupFailure },
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			module := managedpackpromotion.NewModule(
				&acquirerStub{acquisition: test.acquisition}, validValidator(),
				&preparerStub{result: test.preparation},
				&publisherStub{result: managedpackpromotion.Publication{Proposal: &managedpackpromotion.Proposal{Number: 1}}},
			)
			result, err := module.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
				Coordinate:     mustCoordinate(t, "addy@1.2.3"),
			})
			if !errors.Is(err, cleanupFailure) {
				t.Fatalf("Promote error = %v, want cleanup failure", err)
			}
			if result.Status != "" {
				t.Fatalf("Promote result = %#v alongside cleanup failure", result)
			}
		})
	}
}

func TestPromoteRejectsEveryInvalidReleaseIdentityWithoutPublication(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*managedpackpromotion.Acquisition)
	}{
		{"wrong registered project", func(value *managedpackpromotion.Acquisition) { value.Release.Project = "owner/other" }},
		{"private project", func(value *managedpackpromotion.Acquisition) { value.Release.Public = false }},
		{"unpublished release", func(value *managedpackpromotion.Acquisition) { value.Release.Published = false }},
		{"unstable release", func(value *managedpackpromotion.Acquisition) { value.Release.Stable = false }},
		{"draft release", func(value *managedpackpromotion.Acquisition) { value.Release.Draft = true }},
		{"prerelease", func(value *managedpackpromotion.Acquisition) { value.Release.Prerelease = true }},
		{"mutable release", func(value *managedpackpromotion.Acquisition) { value.Release.Immutable = false }},
		{"wrong tag", func(value *managedpackpromotion.Acquisition) { value.Release.Tag = "v1.2.3" }},
		{"missing repository ID", func(value *managedpackpromotion.Acquisition) { value.Release.RepositoryID = 0 }},
		{"missing release ID", func(value *managedpackpromotion.Acquisition) { value.Release.ReleaseID = 0 }},
		{"invalid tag ref SHA", func(value *managedpackpromotion.Acquisition) { value.Release.TagRef.SHA = "short" }},
		{"invalid tag ref type", func(value *managedpackpromotion.Acquisition) { value.Release.TagRef.Type = "tree" }},
		{"missing annotated tag object", func(value *managedpackpromotion.Acquisition) { value.Release.TagObjects = nil }},
		{"moved first tag object", func(value *managedpackpromotion.Acquisition) {
			value.Release.TagObjects[0].SHA = strings.Repeat("b", 40)
		}},
		{"moved peeled target", func(value *managedpackpromotion.Acquisition) {
			value.Release.TagObjects[0].TargetSHA = strings.Repeat("e", 40)
		}},
		{"contradictory target type", func(value *managedpackpromotion.Acquisition) {
			value.Release.TagObjects[0].TargetType = managedpackpromotion.GitObjectTag
		}},
		{"disconnected nested tag chain", func(value *managedpackpromotion.Acquisition) {
			value.Release.TagObjects = []managedpackpromotion.TagObject{
				{SHA: value.Release.TagRef.SHA, TargetSHA: strings.Repeat("b", 40), TargetType: managedpackpromotion.GitObjectTag},
				{SHA: strings.Repeat("e", 40), TargetSHA: value.Release.CommitSHA, TargetType: managedpackpromotion.GitObjectCommit},
			}
		}},
		{"tag cycle", func(value *managedpackpromotion.Acquisition) {
			value.Release.TagObjects = []managedpackpromotion.TagObject{
				{SHA: value.Release.TagRef.SHA, TargetSHA: value.Release.TagRef.SHA, TargetType: managedpackpromotion.GitObjectTag},
				{SHA: value.Release.TagRef.SHA, TargetSHA: value.Release.CommitSHA, TargetType: managedpackpromotion.GitObjectCommit},
			}
		}},
		{"invalid commit SHA", func(value *managedpackpromotion.Acquisition) { value.Release.CommitSHA = "short" }},
		{"invalid tree SHA", func(value *managedpackpromotion.Acquisition) { value.Release.RootTreeSHA = "short" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repositoryRoot := registryFixture(t, "addy", "owner/addy")
			acquisition := validAcquisition()
			test.mutate(&acquisition)
			cleanupCalls := 0
			acquisition.Cleanup = func() error { cleanupCalls++; return nil }
			validator := &validatorStub{validation: managedpack.Validation{Manifest: managedpack.Manifest{ID: "addy", Version: "1.2.3"}}}
			publisher := &publisherStub{}
			module := managedpackpromotion.NewModule(
				&acquirerStub{acquisition: acquisition},
				validator,
				&preparerStub{result: managedpackpromotion.CandidatePreparation{Candidate: &managedpackpromotion.Candidate{ID: "candidate", Summary: "sealed promotion summary"}}},
				publisher,
			)

			result, err := module.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: repositoryRoot,
				Coordinate:     mustCoordinate(t, "addy@1.2.3"),
			})
			if err != nil {
				t.Fatalf("Promote returned an operational error: %v", err)
			}
			if result.Status != managedpackpromotion.StatusRejected || result.Rejection == nil || result.Rejection.Gate != managedpackpromotion.GateRelease {
				t.Fatalf("Promote result = %#v, want release rejection", result)
			}
			if publisher.calls != 0 {
				t.Fatalf("Publisher calls = %d after failed release gate, want 0", publisher.calls)
			}
			if cleanupCalls != 1 {
				t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
			}
		})
	}
}

func TestPromoteValidatesRequestBeforeAcquisition(t *testing.T) {
	validRoot := registryFixture(t, "addy", "owner/addy")
	tests := []struct {
		name    string
		request managedpackpromotion.Request
	}{
		{"empty repository root", managedpackpromotion.Request{Coordinate: mustCoordinate(t, "addy@1.2.3")}},
		{"missing repository root", managedpackpromotion.Request{RepositoryRoot: filepath.Join(t.TempDir(), "missing"), Coordinate: mustCoordinate(t, "addy@1.2.3")}},
		{"repository root is a file", managedpackpromotion.Request{RepositoryRoot: filepath.Join(validRoot, "managed-packs", "registry.json"), Coordinate: mustCoordinate(t, "addy@1.2.3")}},
		{"invalid manual Pack ID", managedpackpromotion.Request{RepositoryRoot: validRoot, Coordinate: managedpackpromotion.Coordinate{PackID: "Addy", Version: "1.2.3"}}},
		{"invalid manual version", managedpackpromotion.Request{RepositoryRoot: validRoot, Coordinate: managedpackpromotion.Coordinate{PackID: "addy", Version: "v1.2.3"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			acquirer := &acquirerStub{}
			publisher := &publisherStub{}
			module := managedpackpromotion.NewModule(acquirer, &validatorStub{}, &preparerStub{}, publisher)
			if _, err := module.Promote(context.Background(), test.request); err == nil {
				t.Fatal("Promote succeeded")
			}
			if acquirer.calls != 0 || publisher.calls != 0 {
				t.Fatalf("external calls after invalid request: acquire=%d publish=%d", acquirer.calls, publisher.calls)
			}
		})
	}
}

func TestPromoteRejectsAnUnregisteredPackBeforeAcquisition(t *testing.T) {
	acquirer := &acquirerStub{}
	publisher := &publisherStub{}
	module := managedpackpromotion.NewModule(acquirer, &validatorStub{}, &preparerStub{}, publisher)
	result, err := module.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
		Coordinate:     mustCoordinate(t, "argote@1.2.3"),
	})
	if err != nil {
		t.Fatalf("Promote returned an error: %v", err)
	}
	if result.Status != managedpackpromotion.StatusRejected || result.Rejection == nil || result.Rejection.Gate != managedpackpromotion.GateRegistration {
		t.Fatalf("Promote result = %#v, want registration rejection", result)
	}
	if acquirer.calls != 0 || publisher.calls != 0 {
		t.Fatalf("external calls after registration rejection: acquire=%d publish=%d", acquirer.calls, publisher.calls)
	}
}

func TestPromoteRejectsManifestIdentityDriftWithoutPublication(t *testing.T) {
	for _, test := range []struct {
		name       string
		validation managedpack.Validation
	}{
		{"Pack ID", managedpack.Validation{Manifest: managedpack.Manifest{ID: "argote", Version: "1.2.3"}}},
		{"version", managedpack.Validation{Manifest: managedpack.Manifest{ID: "addy", Version: "1.2.4"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher := &publisherStub{}
			module := managedpackpromotion.NewModule(
				&acquirerStub{acquisition: validAcquisition()},
				&validatorStub{validation: test.validation},
				&preparerStub{},
				publisher,
			)
			result, err := module.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
				Coordinate:     mustCoordinate(t, "addy@1.2.3"),
			})
			if err != nil {
				t.Fatalf("Promote returned an error: %v", err)
			}
			if result.Status != managedpackpromotion.StatusRejected || result.Rejection == nil || result.Rejection.Gate != managedpackpromotion.GateValidation {
				t.Fatalf("Promote result = %#v, want validation rejection", result)
			}
			if publisher.calls != 0 {
				t.Fatalf("Publisher calls = %d, want 0", publisher.calls)
			}
		})
	}
}

func TestPromoteConvertsOnlyTypedPortPolicyFailuresIntoRejections(t *testing.T) {
	tests := []struct {
		name           string
		gate           managedpackpromotion.Gate
		acquirer       *acquirerStub
		validator      *validatorStub
		preparer       *preparerStub
		publisher      *publisherStub
		publisherCalls int
	}{
		{
			name: "acquisition policy", gate: managedpackpromotion.GateRelease,
			acquirer:  &acquirerStub{err: managedpackpromotion.Reject(managedpackpromotion.GateRelease, "release disappeared")},
			validator: &validatorStub{}, preparer: &preparerStub{}, publisher: &publisherStub{},
		},
		{
			name: "offline validation policy", gate: managedpackpromotion.GateExactCopies,
			acquirer:  &acquirerStub{acquisition: validAcquisition()},
			validator: &validatorStub{err: managedpackpromotion.Reject(managedpackpromotion.GateExactCopies, "origin drift")},
			preparer:  &preparerStub{}, publisher: &publisherStub{},
		},
		{
			name: "candidate gate policy", gate: managedpackpromotion.GatePackySuite,
			acquirer:  &acquirerStub{acquisition: validAcquisition()},
			validator: validValidator(),
			preparer:  &preparerStub{err: managedpackpromotion.Reject(managedpackpromotion.GatePackySuite, "tests failed")},
			publisher: &publisherStub{},
		},
		{
			name: "publication policy", gate: managedpackpromotion.GateFreshness,
			acquirer:       &acquirerStub{acquisition: validAcquisition()},
			validator:      validValidator(),
			preparer:       &preparerStub{result: validCandidatePreparation()},
			publisher:      &publisherStub{err: managedpackpromotion.Reject(managedpackpromotion.GateFreshness, "base moved")},
			publisherCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			module := managedpackpromotion.NewModule(test.acquirer, test.validator, test.preparer, test.publisher)
			result, err := module.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
				Coordinate:     mustCoordinate(t, "addy@1.2.3"),
			})
			if err != nil {
				t.Fatalf("Promote returned an operational error: %v", err)
			}
			if result.Status != managedpackpromotion.StatusRejected || result.Rejection == nil || result.Rejection.Gate != test.gate {
				t.Fatalf("Promote result = %#v, want %s rejection", result, test.gate)
			}
			if result.Rejection.Reason == "" {
				t.Fatal("rejection lost its stable reason")
			}
			if test.publisher.calls != test.publisherCalls {
				t.Fatalf("Publisher calls = %d, want %d", test.publisher.calls, test.publisherCalls)
			}
		})
	}
}

func TestPromoteKeepsOperationalAndCancellationFailuresAsErrors(t *testing.T) {
	sentinel := errors.New("transport failed")
	for _, test := range []struct {
		name      string
		acquirer  *acquirerStub
		validator *validatorStub
		preparer  *preparerStub
		publisher *publisherStub
		want      error
	}{
		{"acquirer", &acquirerStub{err: sentinel}, &validatorStub{}, &preparerStub{}, &publisherStub{}, sentinel},
		{"validator cancellation", &acquirerStub{acquisition: validAcquisition()}, &validatorStub{err: context.Canceled}, &preparerStub{}, &publisherStub{}, context.Canceled},
		{"preparer", &acquirerStub{acquisition: validAcquisition()}, validValidator(), &preparerStub{err: sentinel}, &publisherStub{}, sentinel},
		{"publisher", &acquirerStub{acquisition: validAcquisition()}, validValidator(), &preparerStub{result: validCandidatePreparation()}, &publisherStub{err: sentinel}, sentinel},
	} {
		t.Run(test.name, func(t *testing.T) {
			module := managedpackpromotion.NewModule(test.acquirer, test.validator, test.preparer, test.publisher)
			result, err := module.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: registryFixture(t, "addy", "owner/addy"),
				Coordinate:     mustCoordinate(t, "addy@1.2.3"),
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Promote error = %v, want %v", err, test.want)
			}
			if result.Status != "" {
				t.Fatalf("Promote result = %#v alongside operational error", result)
			}
		})
	}
}

type acquirerStub struct {
	acquisition managedpackpromotion.Acquisition
	err         error
	project     string
	coordinate  managedpackpromotion.Coordinate
	calls       int
}

func (stub *acquirerStub) Acquire(_ context.Context, project string, coordinate managedpackpromotion.Coordinate) (managedpackpromotion.Acquisition, error) {
	stub.calls++
	stub.project = project
	stub.coordinate = coordinate
	return stub.acquisition, stub.err
}

type validatorStub struct {
	validation managedpack.Validation
	err        error
	calls      int
}

func (stub *validatorStub) Validate(context.Context, managedpackpromotion.Acquisition) (managedpack.Validation, error) {
	stub.calls++
	return stub.validation, stub.err
}

type preparerStub struct {
	result      managedpackpromotion.CandidatePreparation
	err         error
	calls       int
	acquisition managedpackpromotion.Acquisition
}

func (stub *preparerStub) Prepare(_ context.Context, _ string, acquisition managedpackpromotion.Acquisition, _ managedpack.Validation) (managedpackpromotion.CandidatePreparation, error) {
	stub.calls++
	stub.acquisition = acquisition
	return stub.result, stub.err
}

type publisherStub struct {
	result managedpackpromotion.Publication
	err    error
	calls  int
}

func (stub *publisherStub) Publish(context.Context, managedpackpromotion.Candidate) (managedpackpromotion.Publication, error) {
	stub.calls++
	return stub.result, stub.err
}

func registryFixture(t *testing.T, packID, project string) string {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, "managed-packs")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	document := `{"schema_version":1,"packs":[{"pack_id":"` + packID + `","project":"` + project + `"}]}`
	if err := os.WriteFile(filepath.Join(directory, "registry.json"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func mustCoordinate(t *testing.T, value string) managedpackpromotion.Coordinate {
	t.Helper()
	coordinate, err := managedpackpromotion.ParseCoordinate(value)
	if err != nil {
		t.Fatal(err)
	}
	return coordinate
}

func validValidator() *validatorStub {
	return &validatorStub{validation: managedpack.Validation{Manifest: managedpack.Manifest{ID: "addy", Version: "1.2.3"}}}
}

func validCandidatePreparation() managedpackpromotion.CandidatePreparation {
	return managedpackpromotion.CandidatePreparation{
		Candidate: &managedpackpromotion.Candidate{ID: "candidate", Summary: "sealed promotion summary"},
		Cleanup:   func() error { return nil },
	}
}

func validAcquisition() managedpackpromotion.Acquisition {
	commit := strings.Repeat("c", 40)
	return managedpackpromotion.Acquisition{
		Release: managedpackpromotion.Release{
			Project: "owner/addy", RepositoryID: 11, ReleaseID: 22,
			Public: true, Published: true, Stable: true, Immutable: true,
			Tag:    "pack-v1.2.3",
			TagRef: managedpackpromotion.GitObject{SHA: strings.Repeat("a", 40), Type: managedpackpromotion.GitObjectTag},
			TagObjects: []managedpackpromotion.TagObject{{
				SHA: strings.Repeat("a", 40), TargetSHA: commit, TargetType: managedpackpromotion.GitObjectCommit,
			}},
			CommitSHA: commit, RootTreeSHA: strings.Repeat("d", 40),
		},
		ProjectRoot: "/acquired/project",
		OriginRoots: map[string]string{},
		Cleanup:     func() error { return nil },
	}
}
