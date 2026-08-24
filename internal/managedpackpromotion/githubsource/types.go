// Package githubsource reads immutable release and Git tree evidence from
// public GitHub repositories for Managed Pack Promotion. It treats every
// upstream object as inert data and never executes project content.
package githubsource

import "time"

// Release is the GitHub release evidence required by promotion policy.
type Release struct {
	ID          int64
	Tag         string
	Immutable   bool
	PublishedAt time.Time
	Draft       bool
	Prerelease  bool
}

// TagObject records one edge in an annotated tag chain.
type TagObject struct {
	SHA        string
	TargetSHA  string
	TargetType string
}

// Candidate is the exact repository, release, commit, and tree identity used
// by acquisition. Fields not consumed by promotion are intentionally omitted.
type Candidate struct {
	Repository   string
	RepositoryID int64
	Public       bool
	Release      *Release
	TagRefName   string
	TagRefType   string
	TagRefSHA    string
	TagObjects   []TagObject
	Commit       string
	Tree         string
}
