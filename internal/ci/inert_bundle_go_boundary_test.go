package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const bundleGoModule = "module github.com/yersonargotev/packy/bundle\n\ngo 1.25.0\n"

func TestBundleGoModuleKeepsAdmittedContentOutOfRootTooling(t *testing.T) {
	root := repositoryRoot(t)
	marker, err := os.ReadFile(filepath.Join(root, "bundle", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(marker); got != bundleGoModule {
		t.Fatalf("bundle/go.mod must remain the minimal nested-module boundary:\n%s", got)
	}

	fixture := t.TempDir()
	writeFixtureFile(t, fixture, "go.mod", "module example.test/root\n\ngo 1.25.0\n")
	writeFixtureFile(t, fixture, "safe/safe.go", "package safe\n")
	writeFixtureFile(t, fixture, "bundle/go.mod", string(marker))
	writeFixtureFile(t, fixture, "bundle/admitted/failing_test.go", `package admitted

import "testing"

func TestAdmittedProjectContentMustRemainInert(t *testing.T) {
	panic("root Go tooling executed admitted bundle content")
}
`)

	listed := runRootGoTool(t, fixture, "list", "./...")
	if got := strings.TrimSpace(listed); got != "example.test/root/safe" {
		t.Fatalf("root go list discovered content across the bundle boundary: %q", got)
	}
	runRootGoTool(t, fixture, "test", "./...")
}

func writeFixtureFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRootGoTool(t *testing.T, root string, args ...string) string {
	t.Helper()
	sandbox := t.TempDir()
	command := exec.Command("go", args...)
	command.Dir = root
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + filepath.Join(sandbox, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(sandbox, "xdg"),
		"GOCACHE=" + filepath.Join(sandbox, "cache"),
		"GOMODCACHE=" + filepath.Join(sandbox, "modcache"),
		"GOPATH=" + filepath.Join(sandbox, "gopath"),
		"GOENV=off",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
