package version

// Value is the semantic version reported by the CLI.
//
// Release builds override this with:
//
//	go build -ldflags "-X github.com/yersonargotev/packy/internal/version.Value=v0.x.y"
var Value = "dev"

// SourceCommit is the exact source revision embedded by release builds.
// Unstamped development binaries report unknown rather than claiming a release identity.
var SourceCommit = "unknown"
