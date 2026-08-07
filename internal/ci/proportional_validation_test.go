package ci_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneralValidationIsSandboxedAndProportional(t *testing.T) {
	script := readFile(t, filepath.Join(repositoryRoot(t), "scripts", "validate-packy.sh"))

	for _, required := range []string{"export HOME=", "export XDG_CONFIG_HOME=", "gofmt -l", "go vet ./...", `go test "${test_packages[@]}"`} {
		if !strings.Contains(script, required) {
			t.Errorf("general validation is missing %q", required)
		}
	}
	for _, retired := range []string{"validate-addy-acceptance.sh", "validate-vercel-acceptance.sh", "governanceauth", "governancedrift"} {
		if strings.Contains(script, retired) {
			t.Errorf("general validation retains disproportionate phase %q", retired)
		}
	}
}

func TestOrdinaryPullRequestCIContainsOnlyGeneralValidation(t *testing.T) {
	workflow := readFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", "ci.yml"))
	if strings.Count(workflow, "\n  validate:") != 1 || strings.Count(workflow, "./scripts/validate-packy.sh --ci") != 1 {
		t.Fatal("ordinary CI must expose one general validation job and invoke it once")
	}
	for _, retired := range []string{"floor-smoke", "vercel-acceptance-gate", "run-claude-smoke.sh", "run-codex-smoke.sh", "run-opencode-smoke.sh", "upload-artifact"} {
		if strings.Contains(workflow, retired) {
			t.Errorf("ordinary CI retains release or supported-CLI work %q", retired)
		}
	}
	script := readFile(t, filepath.Join(repositoryRoot(t), "scripts", "validate-packy.sh"))
	if strings.Count(script, "go test -race") != 1 || !strings.Contains(script, "go test -race ./internal/capabilitypack") {
		t.Fatal("CI validation must race-test only the concurrent capability state store package")
	}
	for _, concurrent := range []string{`"$ci" == true && "$package" == github.com/yersonargotev/packy/internal/capabilitypack`, `go test "${test_packages[@]}" &`, "go test -race ./internal/capabilitypack &", `wait "$tests_pid"`, `wait "$race_pid"`} {
		if !strings.Contains(script, concurrent) {
			t.Errorf("CI validation does not cover each package once while running regular and race tests concurrently: missing %q", concurrent)
		}
	}
}

func TestCustomGovernanceWorkflowsAreRemoved(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{"governance.yml", "governance-drift.yml"} {
		if _, err := os.Stat(filepath.Join(root, ".github", "workflows", name)); !os.IsNotExist(err) {
			t.Errorf("custom governance workflow %s still exists", name)
		}
	}
}
