package version

// Value is the semantic version reported by the CLI.
//
// Release builds override this with:
//
//	go build -ldflags "-X github.com/yersonargotev/matty/internal/version.Value=v0.x.y"
var Value = "dev"
