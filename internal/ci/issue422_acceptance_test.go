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
			"Repeat `--resource` to select specific resource roots",
			"--resource <kind>:<id>",
			"Inspection and `--dry-run` do not mutate Pack state or CLI surfaces",
		},
		"docs/product/packy-v0.md": {
			"`packy init`, catalog inspection, `doctor`, and `--dry-run` do not change a CLI surface.",
			"Every mutation names one Pack and uses a fresh preview.",
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

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
