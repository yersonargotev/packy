package ci_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestReleaseObserver(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(python, "-B", filepath.Join("..", "..", "scripts", "tests", "observe-release.py"))
	command.Env = testprocess.Env(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("release observer: %v\n%s", err, output)
	}
}
