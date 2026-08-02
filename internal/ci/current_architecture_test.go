package ci

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentContractsDoNotTeachClassicDesiredState(t *testing.T) {
	root := cutoverRepoRoot(t)
	paths := currentArchitectureContractPaths(t, root)
	for _, path := range paths {
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(file)
		for lineNumber := 1; scanner.Scan(); lineNumber++ {
			line := strings.ToLower(scanner.Text())
			if currentContractRetainsClassicAssumption(path, line) {
				t.Errorf("%s:%d retains a current classic desired-state assumption: %s", path, lineNumber, strings.TrimSpace(scanner.Text()))
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("scan %s: %v", path, err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
	}
}

func currentContractRetainsClassicAssumption(path, line string) bool {
	if strings.TrimSpace(line) == "### classic lifecycle" && path == "CONTEXT.md" {
		return false
	}
	historicalContext := false
	for _, marker := range []string{
		"removed", "former", "historical", "superseded", "legacy", "leftover",
		"unowned", "preserv", "ignored", "does not", " not ", "must not", "reject", "unknown command",
	} {
		if strings.Contains(line, marker) {
			historicalContext = true
			break
		}
	}
	for _, token := range []string{"managed_skills", "created_containers", "claude_ownership"} {
		if strings.Contains(line, token) && !historicalContext {
			return true
		}
	}
	if strings.Contains(line, ".packy/config.json") && !historicalContext {
		return true
	}
	if strings.Contains(line, "classic lifecycle") && !historicalContext {
		return true
	}
	for _, command := range []string{"packy install", "packy update", "packy uninstall"} {
		if containsCommandInvocation(line, command) && !historicalContext {
			return true
		}
	}
	return false
}

func containsCommandInvocation(line, command string) bool {
	for offset := 0; ; {
		index := strings.Index(line[offset:], command)
		if index < 0 {
			return false
		}
		end := offset + index + len(command)
		if end == len(line) || line[end] < 'a' || line[end] > 'z' {
			return true
		}
		offset = end
	}
}

func TestCurrentArchitectureCommandInvocationDetection(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "packy update", want: true},
		{line: "run `packy update --dry-run`", want: true},
		{line: "packy updates exact owned projections", want: false},
		{line: "the skills packy installs are selected explicitly", want: false},
	}
	for _, test := range tests {
		if got := containsCommandInvocation(test.line, "packy update"); got != test.want {
			t.Errorf("containsCommandInvocation(%q) = %t, want %t", test.line, got, test.want)
		}
	}
}

func TestCurrentArchitectureHistoricalPathsAreExplicitlyAllowlisted(t *testing.T) {
	for _, path := range []string{
		"docs/adr/0003-core-lifecycle-deep-module.md",
		"docs/cutover/packy-v0.1.7/evidence/issue-65/README.md",
		"docs/governance/evidence/issue-168/baseline.json",
		"docs/release-notes/v0.1.7.md",
		"docs/research/claude-code-implementation-ready-specification.md",
	} {
		if !historicalArchitecturePath(path) {
			t.Errorf("historical architecture path %q is not explicitly allowlisted", path)
		}
	}
	for _, path := range []string{"README.md", "docs/capability-packs.md", "docs/claude-code.md", "docs/release-notes/next.md", "internal/cli/root.go"} {
		if historicalArchitecturePath(path) {
			t.Errorf("current architecture path %q is hidden by the historical allowlist", path)
		}
	}
}

func TestCurrentArchitectureContractsCoverProductionAndTests(t *testing.T) {
	root := cutoverRepoRoot(t)
	paths := currentArchitectureContractPaths(t, root)
	covered := make(map[string]bool, len(paths))
	for _, path := range paths {
		covered[path] = true
	}
	for _, path := range []string{
		"cmd/packy/main.go",
		"internal/capabilitypack/activation.go",
		"internal/cli/issue418_classic_cutover_test.go",
		"internal/setuphealth/setuphealth_test.go",
	} {
		if !covered[path] {
			t.Errorf("current architecture contracts do not cover %s", path)
		}
	}
}

func currentArchitectureContractPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel != "." && historicalArchitecturePath(rel+"/") {
				return filepath.SkipDir
			}
			return nil
		}
		if historicalArchitecturePath(rel) {
			return nil
		}

		switch {
		case rel == "README.md", rel == "CONTEXT.md":
			paths = append(paths, rel)
		case strings.HasPrefix(rel, "docs/") && filepath.Ext(rel) == ".md":
			paths = append(paths, rel)
		case (strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "internal/")) && filepath.Ext(rel) == ".go" && !architectureEnforcementPath(rel):
			paths = append(paths, rel)
		case strings.HasPrefix(rel, ".github/workflows/") && (filepath.Ext(rel) == ".yml" || filepath.Ext(rel) == ".yaml"):
			paths = append(paths, rel)
		case strings.HasPrefix(rel, "scripts/"):
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func architectureEnforcementPath(path string) bool {
	return path == "internal/ci/current_architecture_test.go" || path == "internal/cli/architecture_test.go"
}

// Historical decisions, research, immutable cutover records, and release
// evidence describe the superseded architecture. Keeping these paths explicit
// prevents a current-contract check from pressuring maintainers to rewrite the
// record of why ADR 0022 made the incompatible cutover.
func historicalArchitecturePath(path string) bool {
	if path == "docs/release-notes/next.md" {
		return false
	}
	for _, prefix := range []string{
		".scratch/",
		"docs/adr/",
		"docs/cutover/",
		"docs/governance/evidence/",
		"docs/release-notes/",
		"docs/research/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
