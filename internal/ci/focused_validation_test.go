package ci_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestFocusedValidationRoutesChecksAndPropagatesFailures(t *testing.T) {
	for _, scenario := range []struct {
		name, path, mode string
		checkExit        string
	}{
		{name: "unchanged", mode: "empty", checkExit: "0"},
		{name: "release notes", path: "docs/release-notes/next.md", mode: "documentation", checkExit: "0"},
		{name: "readme", path: "README.md", mode: "documentation", checkExit: "0"},
		{name: "document check fails", path: "docs/release-notes/next.md", mode: "documentation", checkExit: "17"},
		{name: "go owner", path: "internal/cli/root.go", mode: "go", checkExit: "0"},
		{name: "unknown path", path: "unclassified.txt", mode: "general", checkExit: "0"},
		{name: "general check fails", path: "unclassified.txt", mode: "general", checkExit: "17"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			script := readFile(t, filepath.Join(repositoryRoot(t), "scripts", "validate-changed.sh"))
			writeValidationFixture(t, root, "scripts/validate-changed.sh", script, 0o755)
			writeValidationFixture(t, root, "scripts/validate-packy.sh", "#!/usr/bin/env bash\necho general-validation\nexit \"$PACKY_CHECK_EXIT\"\n", 0o755)
			writeValidationFixture(t, root, "README.md", "original\n", 0o644)
			writeValidationFixture(t, root, "docs/release-notes/next.md", "original\n", 0o644)
			writeValidationFixture(t, root, "internal/cli/root.go", "package cli\n", 0o644)
			writeValidationFixture(t, root, "bin/go", `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  list) echo github.com/yersonargotev/packy/internal/cli ;;
  test)
    printf '%s\n' "$@" > "$PACKY_GO_CALLS"
    printf '%s\n' "$HOME" "$XDG_CONFIG_HOME" "$GOCACHE" "$GOMODCACHE" "$GOPATH" > "$PACKY_GO_ENV"
    exit "$PACKY_CHECK_EXIT" ;;
  *) exit 99 ;;
esac
`, 0o755)
			calls := filepath.Join(root, "go-calls")
			observedEnv := filepath.Join(root, "go-environment")
			cache := filepath.Join(root, "preserved-cache")
			environment := testprocess.Env(t,
				"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
				"PACKY_GO_CALLS="+calls, "PACKY_GO_ENV="+observedEnv,
				"PACKY_CHECK_EXIT="+scenario.checkExit,
				"GOCACHE="+cache, "GOMODCACHE="+cache, "GOPATH="+cache)
			for _, args := range [][]string{
				{"init", "-q"}, {"add", "."},
				{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "baseline"},
			} {
				if output, err := runValidationProcess(t, root, environment, "git", args...); err != nil {
					t.Fatalf("fixture git %v: %v\n%s", args, err, output)
				}
			}
			if scenario.path != "" {
				contents := "changed\n"
				if scenario.mode == "go" {
					contents = "package cli\n\nconst Changed = true\n"
				}
				writeValidationFixture(t, root, scenario.path, contents, 0o644)
			}
			output, err := runValidationProcess(t, root, environment, "bash", "scripts/validate-changed.sh", "HEAD")
			if scenario.checkExit == "0" && err != nil {
				t.Fatalf("validation: %v\n%s", err, output)
			}
			if scenario.checkExit != "0" {
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 17 {
					t.Fatalf("validation did not propagate check exit 17: %v\n%s", err, output)
				}
			}
			switch scenario.mode {
			case "empty", "general":
				if _, err := os.Stat(calls); !os.IsNotExist(err) {
					t.Fatalf("%s unexpectedly ran focused Go checks: %v", scenario.mode, err)
				}
				want := "scope=empty"
				if scenario.mode == "general" {
					want = "general-validation"
				}
				if !strings.Contains(output, want) {
					t.Fatalf("missing %q: %s", want, output)
				}
			case "documentation", "go":
				arguments := readFile(t, calls)
				if scenario.mode == "documentation" {
					if !strings.Contains(output, "scope=documentation-only") || !strings.Contains(arguments, "./internal/ci\n-run\n") {
						t.Fatalf("documentation contracts were not dispatched: %s\n%s", output, arguments)
					}
					args := strings.Split(strings.TrimSpace(arguments), "\n")
					selector, err := regexp.Compile(args[len(args)-1])
					if err != nil {
						t.Fatal(err)
					}
					for _, name := range []string{
						"TestCurrentDocumentationDescribesOnlyCurrentArchitecture",
						"TestCurrentDocumentationLocalLinksResolve",
						"TestDeveloperDocumentationExplainsStaticAnalysis",
						"TestPublicDocumentationUsesCanonicalPackDiscovery",
						"TestClaudeDocumentationContractStaysCurrent",
						"TestCatalogAdoptionDocumentsOrderedCleanHandoffAndPreservation",
						"TestIssue422OperatorPathsTeachInitializationThenExplicitActivation",
						"TestReleaseGateRequiresFreshDecisionEvidence",
					} {
						if !selector.MatchString(name) {
							t.Errorf("documentation selector omits %s", name)
						}
					}
					if selector.MatchString("TestCatalogExecutable") {
						t.Error("documentation-only checks include the full executable contract")
					}
				} else if strings.Contains(arguments, "-run\n") || !strings.Contains(output, "scope=go") {
					t.Fatalf("Go owner checks were replaced by document checks: %s\n%s", output, arguments)
				}
				values := strings.Split(strings.TrimSpace(readFile(t, observedEnv)), "\n")
				if len(values) != 5 || values[2] != cache || values[3] != cache || values[4] != cache {
					t.Fatalf("Go caches were not preserved: %v", values)
				}
				if !strings.Contains(values[0], "packy-validation.") || !strings.Contains(values[1], "packy-validation.") {
					t.Fatalf("check roots were not sandboxed: %v", values)
				}
				for _, path := range values[:2] {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("validation sandbox was not cleaned: %s: %v", path, err)
					}
				}
			}
		})
	}
}

func runValidationProcess(t *testing.T, root string, environment []string, executable string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir, command.Env = root, environment
	output, err := command.CombinedOutput()
	return string(output), err
}

func writeValidationFixture(t *testing.T, root, name, contents string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}
