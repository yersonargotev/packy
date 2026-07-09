package cli

import "github.com/yersonargotev/matty/internal/engrambin"

// HomebrewEngramInstalled checks whether the canonical Homebrew-managed Engram
// binary is already available. A non-Homebrew engram on PATH deliberately does
// not satisfy this check: Matty delegates ownership to Homebrew and setup uses
// the Homebrew binary path directly.
func HomebrewEngramInstalled(paths Paths, runner Runner) bool {
	canonical := engrambin.DiscoverHomebrew(paths.HomebrewPrefixEnv)
	if canonical != nil {
		return true
	}
	resolved, err := runner.LookPath("engram")
	if err != nil {
		return false
	}
	return engrambin.IsExpectedHomebrewPath(resolved, engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv))
}
