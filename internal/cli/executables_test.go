package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/testprocess"
)

var executablesRoot string

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "packy-cli-executables-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create shared executables directory:", err)
		os.Exit(1)
	}
	executablesRoot = root
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

type sharedExecutable struct {
	once   sync.Once
	path   string
	output []byte
	err    error
}

// Each build runs with GoOfflineEnv's fresh, empty Go cache, so building once
// per test process keeps executable acceptance tests within the CI budget.
func (e *sharedExecutable) get(t *testing.T, name, pkg string, flags ...string) string {
	t.Helper()
	e.once.Do(func() {
		e.path = filepath.Join(executablesRoot, name)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		build := exec.CommandContext(ctx, "go", append(append([]string{"build"}, flags...), "-o", e.path, pkg)...)
		build.Dir = filepath.Join("..", "..")
		build.Env = testprocess.GoOfflineEnv(t)
		e.output, e.err = build.CombinedOutput()
	})
	if e.err != nil {
		t.Fatalf("build %s: %v\n%s", name, e.err, e.output)
	}
	return e.path
}

var packyExecutable, issue929TransportExecutable sharedExecutable

func builtPackyExecutable(t *testing.T) string {
	t.Helper()
	return packyExecutable.get(t, "packy", "./cmd/packy")
}

func builtIssue929Transport(t *testing.T) string {
	t.Helper()
	return issue929TransportExecutable.get(t, "packy-transport", "./internal/cli/testdata/packytransport", "-ldflags=-X github.com/yersonargotev/packy/internal/version.SourceCommit="+strings.Repeat("2", 40))
}
