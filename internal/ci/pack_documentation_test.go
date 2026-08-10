package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedPackDocumentationIsCurrent(t *testing.T) {
	root := repositoryRoot(t)
	command := exec.Command("go", "run", "./internal/tools/packdocs", "--check", "--root", root)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated Pack documentation is not current: %v\n%s", err, output)
	}
}

func TestPackDocumentationCheckRejectsDriftedOutput(t *testing.T) {
	root := repositoryRoot(t)
	for name, mutate := range map[string]func(*testing.T, string){
		"missing": func(t *testing.T, generatedRoot string) {
			if err := os.Remove(filepath.Join(generatedRoot, "addy.md")); err != nil {
				t.Fatal(err)
			}
		},
		"modified": func(t *testing.T, generatedRoot string) {
			path := filepath.Join(generatedRoot, "index.md")
			if err := os.WriteFile(path, []byte("modified\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"obsolete": func(t *testing.T, generatedRoot string) {
			if err := os.WriteFile(filepath.Join(generatedRoot, "retired.md"), []byte("obsolete\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			temporaryRoot := t.TempDir()
			copyTree(t, filepath.Join(root, "bundle"), filepath.Join(temporaryRoot, "bundle"))
			generatedRoot := filepath.Join(temporaryRoot, "docs", "packs")
			copyTree(t, filepath.Join(root, "docs", "packs"), generatedRoot)
			mutate(t, generatedRoot)

			command := exec.Command("go", "run", "./internal/tools/packdocs", "--check", "--root", temporaryRoot)
			command.Dir = root
			if output, err := command.CombinedOutput(); err == nil {
				t.Fatalf("Pack documentation check accepted %s output:\n%s", name, output)
			}
		})
	}
}

func TestGeneratedPackDocumentationOmitsInternalFields(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "docs", "packs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		text := readFile(t, filepath.Join(root, "docs", "packs", entry.Name()))
		for _, forbidden := range []string{"source_paths", "sha256", "fingerprint", "projection"} {
			if strings.Contains(strings.ToLower(text), forbidden) {
				t.Errorf("docs/packs/%s exposes internal field %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestGeneratedPackPagesExposeDecisionUsefulInventory(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "docs", "packs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "index.md" {
			continue
		}
		text := readFile(t, filepath.Join(root, "docs", "packs", entry.Name()))
		for _, required := range []string{
			"## Purpose", "- Version:", "- Supported surfaces:", "- Readiness obligations:", "- External requirements:",
			"## Resources", "  - Role:", "  - Dependencies:", "  - Notices:",
			"## Pack exclusions", "## Inspect and preview", "packy show", "--dry-run",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("docs/packs/%s is missing %q", entry.Name(), required)
			}
		}
	}
}

func TestGeneratedPackIndexDescribesTheCompleteAuthoringVocabulary(t *testing.T) {
	text := readFile(t, filepath.Join(repositoryRoot(t), "docs", "packs", "index.md"))
	for _, required := range []string{
		"## Pack authoring vocabulary", "Readiness obligations:", "`runtime-usability`", "`surface-authorization`",
		"Surface capabilities:", "`claude-agent-document`", "`claude-composite-skill`", "`external-host-setup`", "`opencode-primary-prompt`", "`project-instruction`",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("generated Pack index is missing %q", required)
		}
	}
}

func copyTree(t *testing.T, source, target string) {
	t.Helper()
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(target, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
}
