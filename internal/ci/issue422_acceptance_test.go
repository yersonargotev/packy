package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue422OperatorPathsTeachInitializationThenExplicitActivation(t *testing.T) {
	root := repositoryRoot(t)
	contracts := map[string][]string{
		"README.md": {
			"packy init",
			"packy pack activate engram --surface codex --dry-run",
			"packy pack activate engram --surface codex",
			"To activate only selected resource roots",
			"--resource <kind>:<id>",
			"discovery is not activation on that surface",
		},
		"docs/product/packy-v0.md": {
			"Initialization leaves every Codex/OpenCode/Claude Code surface unchanged.",
			"it does not create activation intent for those surfaces",
		},
		"docs/release-notes/next.md": {
			"packy init",
			"packy pack activate engram --surface codex",
		},
	}
	for path, wants := range contracts {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(strings.Fields(string(data)), " ")
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not teach %q", path, want)
			}
		}
	}
}

func TestIssue422ReleaseGuideMatchesPackageAndReleaseSmoke(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "docs", "release.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(data)), " ")
	for _, want := range []string{
		"source initialization caused zero surface changes",
		"packy pack activate addy --surface claude --dry-run",
		"packy pack activate addy --surface claude",
		"packy pack status addy --surface claude",
		"on each supported surface",
		"`schema_version: 3`",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("release guide does not bind smoke behavior %q", want)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
