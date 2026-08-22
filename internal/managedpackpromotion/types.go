package managedpackpromotion

import (
	"context"
	"fmt"

	"github.com/yersonargotev/packy/internal/managedpack"
)

// Status is one observable Managed Pack Promotion outcome.
type Status string

const (
	StatusNoChange Status = "no-change"
	StatusRejected Status = "rejected"
	StatusProposal Status = "proposal"
)

// Gate identifies the admission policy that rejected a promotion.
type Gate string

const (
	GateRegistration       Gate = "registration"
	GateRelease            Gate = "release"
	GateValidation         Gate = "validation"
	GateOrigins            Gate = "origins"
	GateExactCopies        Gate = "exact-copies"
	GateNotices            Gate = "notices"
	GateOwnership          Gate = "cross-pack-ownership"
	GateSemVer             Gate = "semver"
	GateCompatibilityFloor Gate = "compatibility-floor"
	GateGeneratedDocs      Gate = "generated-docs"
	GateResourceSurfaces   Gate = "resource-surfaces"
	GatePackySuite         Gate = "packy-suite"
	GateProposalOwnership  Gate = "proposal-ownership"
	GateFreshness          Gate = "freshness"
	GatePublication        Gate = "publication"
)

// Rejection is the stable, typed explanation for a policy failure.
type Rejection struct {
	Gate   Gate
	Reason string
}

// RejectionError lets adapters report deterministic policy failures without
// turning them into operational errors at the Module interface.
type RejectionError struct {
	Gate   Gate
	Reason string
}

func (rejection *RejectionError) Error() string {
	return fmt.Sprintf("Managed Pack Promotion rejected at %s: %s", rejection.Gate, rejection.Reason)
}

// Reject constructs an error that Promote converts into StatusRejected.
func Reject(gate Gate, reason string) error {
	return &RejectionError{Gate: gate, Reason: reason}
}

// Request contains the Packy checkout and exact release coordinate to promote.
type Request struct {
	RepositoryRoot string
	Coordinate     Coordinate
}

// Result is exactly one no-change, rejection, or proposal outcome.
type Result struct {
	Status    Status
	Reason    string
	Rejection *Rejection
	Proposal  *Proposal
}

// GitObjectType is a type in a peeled release-tag chain.
type GitObjectType string

const (
	GitObjectTag    GitObjectType = "tag"
	GitObjectCommit GitObjectType = "commit"
)

// GitObject identifies the object referenced by the release tag ref.
type GitObject struct {
	SHA  string
	Type GitObjectType
}

// TagObject records one annotated tag and its immediate target.
type TagObject struct {
	SHA        string
	TargetSHA  string
	TargetType GitObjectType
}

// Release is the complete immutable release and Git identity acquired from the
// registered Managed Pack Project.
type Release struct {
	Project      string
	RepositoryID int64
	ReleaseID    int64
	Public       bool
	Published    bool
	Stable       bool
	Draft        bool
	Prerelease   bool
	Immutable    bool
	Tag          string
	TagRef       GitObject
	TagObjects   []TagObject
	CommitSHA    string
	RootTreeSHA  string
}

// Acquisition contains only local public trees plus their sealed release
// identity. Cleanup belongs to the Module and is invoked exactly once after a
// successful acquisition.
type Acquisition struct {
	Release     Release
	ProjectRoot string
	OriginRoots map[string]string
	Cleanup     func() error
}

// Candidate is a fully gated local proposal candidate. ID is adapter-owned
// sealed identity and must include Summary; Summary is the complete proposal
// body input so Publisher never reparses Managed Pack content. The remaining
// fields carry the exact repository state the publisher is authorized to
// propose.
type Candidate struct {
	ID             string
	Summary        string
	Coordinate     Coordinate
	Project        string
	RepositoryRoot string
	BaseSHA        string
	HeadSHA        string
	ResultTreeSHA  string
	Branch         string
}

// CandidatePreparation contains either one fully gated candidate or a
// deterministic no-change result.
type CandidatePreparation struct {
	Candidate      *Candidate
	NoChangeReason string
	Cleanup        func() error
}

// Proposal identifies one protected automation-owned pull-request proposal.
type Proposal struct {
	Branch  string
	Number  int
	URL     string
	HeadSHA string
}

// Publication contains either one proposal or a deterministic no-change
// result produced while adopting an existing proposal.
type Publication struct {
	Proposal       *Proposal
	NoChangeReason string
}

// Acquirer resolves the registered project and exact public release without
// Packy write credentials.
type Acquirer interface {
	Acquire(context.Context, string, Coordinate) (Acquisition, error)
}

// OfflineValidator validates acquired public trees in the isolated offline
// process and returns their sealed Declared Pack Closure.
type OfflineValidator interface {
	Validate(context.Context, Acquisition) (managedpack.Validation, error)
}

// CandidatePreparer runs every repository admission gate and materializes one
// complete local candidate, without branch or pull-request mutation.
type CandidatePreparer interface {
	Prepare(context.Context, string, Acquisition, managedpack.Validation) (CandidatePreparation, error)
}

// Publisher is the only port authorized to create or adopt a proposal.
type Publisher interface {
	Publish(context.Context, Candidate) (Publication, error)
}
