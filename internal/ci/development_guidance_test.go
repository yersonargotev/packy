package ci_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentGuidanceHasNoRetiredDeliveryRoute(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{".agents/skills/deliver-issue", "workflows/issue-delivery.md"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("retired development orchestrator remains at %s", path)
		}
	}
	for _, path := range []string{
		"AGENTS.md", "NOTES.md", "docs/agents/development.md", "docs/agents/issue-tracker.md",
		".agents/skills/release-packy/SKILL.md", "workflows/packy-release.md", "docs/release.md",
	} {
		for _, retired := range []string{"deliver-issue", "issue-delivery"} {
			if strings.Contains(readFile(t, filepath.Join(root, path)), retired) {
				t.Errorf("%s still routes to %s", path, retired)
			}
		}
	}
}
