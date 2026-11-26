package ci_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestProductionExecutableExcludesGitHubFake(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "./cmd/packy")
	command.Dir = repositoryRoot(t)
	command.Env = testprocess.GoOfflineEnv(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/packy: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "github.com/yersonargotev/packy/internal/catalogstore\n") {
		t.Fatalf("go list -deps ./cmd/packy omitted the production catalog store:\n%s", output)
	}
	if strings.Contains(string(output), "internal/githubfake") {
		t.Fatal("the production executable depends on internal/githubfake")
	}
}
