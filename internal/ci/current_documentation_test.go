package ci_test

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestCurrentDocumentationDescribesOnlyCurrentArchitecture(t *testing.T) {
	root := repositoryRoot(t)

	adrs, err := filepath.Glob(filepath.Join(root, "docs", "adr", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	wantADR := []string{
		filepath.Join(root, "docs", "adr", "0031-simplify-packy-around-reviewed-packs.md"),
		filepath.Join(root, "docs", "adr", "0032-admit-single-source-packs-atomically.md"),
	}
	if strings.Join(adrs, "\n") != strings.Join(wantADR, "\n") {
		t.Fatalf("current ADRs = %v, want %v", adrs, wantADR)
	}

	research, err := filepath.Glob(filepath.Join(root, "docs", "research", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(research) != 0 {
		t.Fatalf("retired current-tree research remains: %v", research)
	}
	if err := filepath.WalkDir(filepath.Join(root, ".scratch"), func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return filepath.SkipDir
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			t.Errorf("retired current-tree planning artifact remains: %s", relativePath(root, path))
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	requireDocumentationText(t, root, "docs/adr/0031-simplify-packy-around-reviewed-packs.md", []string{
		"`v0.2.0`", "`addy`", "`argote`", "`engram`", "`matty`", "Pack version `1.0.0`",
		"installed Pack receipt", "branch protection", "CodeQL", "human merge", "immutable",
	})
	requireDocumentationText(t, root, "docs/adr/0032-admit-single-source-packs-atomically.md", []string{
		"immutable Pack Source v2.3.0", "single-source Pack admission", "immutable initial history",
		"fail closed", "previous bundle", "two or more sources", "`yersonargotev/orchestrate-skill`",
		"canonical exact-content source", "Eric Provencher",
	})
	requireDocumentationText(t, root, "bundle/pack-template/README.md", []string{
		"Copy this directory", "one `pack.json`", "maintainer-selected SemVer", "reviewed content",
		"./scripts/validate-pack-content.sh <pack-id>",
	})
	requireDocumentationText(t, root, "docs/release.md", []string{
		"version tag", "Darwin", "Linux", "SHA256SUMS", "GitHub Release", "Homebrew formula", "newer version",
	})
	requireDocumentationText(t, root, "docs/reset-v0.2.md", []string{
		"Warning", "brew uninstall packy", "Back up", "~/.packy", "~/.local/share/packy",
		"packy.json", "packy.lock.json", "PACKY-NOTICES.md", "brew install yersonargotev/tap/packy",
		"packy init", "packy install", "no automatic", "migration command",
	})
}

func TestCurrentDocumentationLocalLinksResolve(t *testing.T) {
	root := repositoryRoot(t)
	paths := []string{filepath.Join(root, "README.md"), filepath.Join(root, "CONTEXT.md"), filepath.Join(root, "AGENTS.md")}
	for _, directory := range []string{"docs", "workflows"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(paths)

	link := regexp.MustCompile(`\[[^]]*\]\(([^)]+)\)`)
	for _, path := range paths {
		text := readFile(t, path)
		for _, match := range link.FindAllStringSubmatch(text, -1) {
			target := strings.Trim(strings.Fields(match[1])[0], "<>")
			if strings.HasPrefix(target, "#") {
				continue
			}
			parsed, err := url.Parse(target)
			if err != nil || parsed.Scheme != "" || parsed.Host != "" {
				continue
			}
			resolved, err := url.PathUnescape(parsed.Path)
			if err != nil {
				t.Errorf("%s has invalid local link %q: %v", relativePath(root, path), target, err)
				continue
			}
			if resolved == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), filepath.FromSlash(resolved))); err != nil {
				t.Errorf("%s has unresolved local link %q", relativePath(root, path), target)
			}
		}
	}
}

func requireDocumentationText(t *testing.T, root, path string, required []string) {
	t.Helper()
	text := readFile(t, filepath.Join(root, filepath.FromSlash(path)))
	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Errorf("%s missing current documentation contract text %q", path, want)
		}
	}
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
