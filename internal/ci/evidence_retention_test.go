package ci_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestEvidenceRetentionBeforeCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := repositoryRoot(t)
	command := exec.CommandContext(ctx, "python3", "-B", filepath.Join(root, "scripts/probes/evidence-retention.py"))
	command.Dir = root
	command.Env = testprocess.Env(t, "PYTHONDONTWRITEBYTECODE=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("evidence retention regression: %v\n%s", err, output)
	}
}
