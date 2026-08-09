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
		filepath.Join(root, "docs", "adr", "0033-make-the-tui-the-primary-interactive-interface.md"),
		filepath.Join(root, "docs", "adr", "0034-make-packy-root-namespace-pack-oriented.md"),
		filepath.Join(root, "docs", "adr", "0035-make-pack-readiness-capability-driven.md"),
	}
	if strings.Join(adrs, "\n") != strings.Join(wantADR, "\n") {
		t.Fatalf("current ADRs = %v, want %v", adrs, wantADR)
	}

	wantResearch := filepath.Join(root, "docs", "research", "evidence", "pack-readiness-architecture.md")
	var research []string
	if err := filepath.WalkDir(filepath.Join(root, "docs", "research"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			research = append(research, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(research, "\n") != wantResearch {
		t.Fatalf("current research = %v, want [%s]", research, wantResearch)
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
	requireDocumentationText(t, root, "docs/adr/0033-make-the-tui-the-primary-interactive-interface.md", []string{
		"Bubble Tea v2 TUI", "minimum Go version to 1.25", "same `internal/capabilitypack` behavior",
		"one Pack, surface", "global or project", "preview, phase consent", "verification boundary",
		"defers exit until the operation returns", "fresh inspection",
	})
	requireDocumentationText(t, root, "docs/adr/0034-make-packy-root-namespace-pack-oriented.md", []string{
		"root namespace is Pack-oriented", "direct root commands", "obsolete `pack` grouping command is absent",
		"alias, forwarding command, fallback, or deprecation path", "`internal/cli` adapter boundary",
		"Pack lifecycle behavior remains in `internal/capabilitypack`", "structured-output report identifiers",
		"Cobra's ordinary unknown command behavior", "issue 589",
	})
	requireDocumentationText(t, root, "docs/adr/0035-make-pack-readiness-capability-driven.md", []string{
		"capability-pack domain owns readiness policy", "three-valued result", "false` dominates",
		"global and project Pack lifecycle", "strict usability gate requires fresh true usability",
		"controlled runtime check", "only in Packy Home", "informational conditions",
		"closed, reviewed surface-capability vocabulary", "issues 619 through 625",
	})
	requireDocumentationText(t, root, "CONTEXT.md", []string{
		"Readiness obligation", "Readiness condition", "Readiness dimensions", "Controlled runtime check",
		"configured, authorized, or usable", "true, false, or unknown",
	})
	requireDocumentationText(t, root, "docs/research/evidence/pack-readiness-architecture.md", []string{
		"primary-source evidence", "Cockburn's original ports-and-adapters", "`True`,",
		"GitHub App manifest", "does not define additional Pack behavior or contracts",
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

func TestReadinessAdaptersDoNotDispatchOnPackOrResourceIdentity(t *testing.T) {
	root := repositoryRoot(t)
	identityDispatch := regexp.MustCompile(`\b(?:pack|resource)\.ID\s*(?:==|!=)`)
	for _, relative := range []string{"internal/codex/surface.go", "internal/opencode/surface.go"} {
		text := readFile(t, filepath.Join(root, filepath.FromSlash(relative)))
		start := strings.Index(text, "func (a *SurfaceAdapter) inspectReadiness")
		if start < 0 {
			t.Fatalf("%s readiness observation boundary is missing", relative)
		}
		end := strings.Index(text[start:], "func (a *SurfaceAdapter) inspectDesired")
		if end < 0 {
			t.Fatalf("%s readiness observation boundary is missing", relative)
		}
		readiness := text[start : start+end]
		if identityDispatch.MatchString(readiness) {
			t.Errorf("%s dispatches readiness observation by Pack or resource identity", relative)
		}
	}
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
