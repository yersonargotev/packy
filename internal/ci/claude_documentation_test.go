package ci_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/claudecode"
)

func TestClaudeDocumentationContractStaysCurrent(t *testing.T) {
	root := repositoryRoot(t)
	documents := map[string][]string{
		"CONTEXT.md":                 {"### CLI surface", "The supported surfaces are Codex", "OpenCode", "Claude Code", "Antigravity and GitHub Copilot CLI remain outside"},
		"README.md":                  {"Claude Code", "docs/claude-code.md", claudecode.MinimumSupportedVersion},
		"docs/claude-code.md":        {"Prerequisite", "Global projections", "Explicit activation", "Preservation and cleanup", "Readiness", "No authentication or model calls", claudecode.MinimumSupportedVersion},
		"docs/product/packy-v0.md":   {"Claude Code", claudecode.MinimumSupportedVersion},
		"docs/roadmap.md":            {"Claude Code", claudecode.MinimumSupportedVersion},
		"docs/capability-packs.md":   {"one `pack.json` manifest", "Claude", "packy pack list", "packy pack show <pack>"},
		"docs/structured-output.md":  {"`schema_version`", "Current-state contract"},
		"docs/release.md":            {"./scripts/validate-packy.sh", "Publication boundaries", "newer version"},
		"docs/release-notes/next.md": {"{{TAG}}", claudecode.MinimumSupportedVersion, "Addy", "Argote", "Engram", "Matty", "SHA256SUMS"},
	}

	staleSupportClaim := regexp.MustCompile(`(?is)(?:supports? only.{0,80}(?:codex.{0,30}opencode|opencode.{0,30}codex)|both supported surfaces|two[- ]surface support|cli surfaces\s*\|\s*codex and opencode only|initial supported cli surfaces are codex and opencode|claude code,\s*antigravity,\s*and github copilot cli are future candidates)`)
	versionLiteral := regexp.MustCompile(`\d+\.\d+\.\d+`)
	for path, required := range documents {
		text := readFile(t, filepath.Join(root, path))
		for _, want := range required {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing documentation contract text %q", path, want)
			}
		}
		if staleSupportClaim.MatchString(text) {
			t.Errorf("%s retains a stale CLI-surface support claim: %q", path, staleSupportClaim.FindString(text))
		}
		for _, line := range strings.Split(text, "\n") {
			lower := strings.ToLower(line)
			if !strings.Contains(lower, "floor") && !strings.Contains(lower, "prerequisite") && !strings.Contains(lower, "or newer") && !strings.Contains(line, "+") {
				continue
			}
			for _, version := range versionLiteral.FindAllString(line, -1) {
				if version != claudecode.MinimumSupportedVersion {
					t.Errorf("%s publishes Claude floor %s, want code authority %s", path, version, claudecode.MinimumSupportedVersion)
				}
			}
		}
	}
}
