package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/githubfake"
)

func testIssue929TUI(t *testing.T, transport string) {
	fixtures, environment := startIssue929Catalogs(t, transport)
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("b", 40), githubfake.Pack{ID: "fixture", Version: "2.0.0"})
	if err := githubfake.WriteRepository(fixtures, issue929Acme, githubfake.Repository{ReleaseStatus: 500}); err != nil {
		t.Fatal(err)
	}

	refresh := driveIssue929TUI(t, transport, environment, issue929Acme, "c", "Internal Server Error", "q")
	assertIssue929TUI(t, refresh,
		`yersonargotev/packy-catalog\s*attested\s*a{40}`,
		`acme/packs\s*attested\s*c{40}`,
		`acme\s*1\.0\.0\s*acme/packs`,
		`fixture\s*1\.0\.0\s*yersonargotev/packy-catalog`,
		`\[OK\] yersonargotev/packy-catalog\s*selected Catalog Snapshot b{40}`,
		`\[FAIL\] acme/packs\s*failed and keeps Catalog Snapshot c{40}\s*\S[^\n]*GitHub returned 500`,
	)
	if official, acme := issue929Selected(t, environment, issue929OfficialCatalog), issue929Selected(t, environment, issue929Acme); official != strings.Repeat("b", 40) || acme != strings.Repeat("c", 40) {
		t.Fatalf("TUI refresh selected %s and %s", official, acme)
	}

	reopened := driveIssue929TUI(t, transport, environment, issue929Acme, "q")
	assertIssue929TUI(t, reopened,
		`yersonargotev/packy-catalog\s*attested\s*b{40}`,
		`acme/packs\s*attested\s*c{40}`,
		`fixture\s*2\.0\.0\s*yersonargotev/packy-catalog`,
		`acme\s*1\.0\.0\s*acme/packs`,
	)
}

// driveIssue929TUI opens the TUI of the transport binary in a private
// terminal through verify-packy's pty-drive.py and returns the text it showed.
// Each pair of steps waits for literal terminal text, then sends keys.
func driveIssue929TUI(t *testing.T, transport string, environment []string, steps ...string) string {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", ".agents", "skills", "verify-packy", "scripts", "pty-drive.py"))
	if err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "tui.raw")
	args := []string{script, transport, transcript}
	for i := 0; i+1 < len(steps); i += 2 {
		args = append(args, "--step", steps[i], steps[i+1])
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "python3", args...)
	command.Dir = t.TempDir()
	command.Env = append(append([]string(nil), environment...), "TERM=xterm-256color", "PYTHONDONTWRITEBYTECODE=1")
	out, err := command.CombinedOutput()
	raw, _ := os.ReadFile(transcript)
	if err != nil {
		t.Fatalf("pty-drive.py: %v\n%s\n%s", err, out, ansi.Strip(string(raw)))
	}
	return ansi.Strip(string(raw))
}

// assertIssue929TUI matches each pattern against the terminal text with panel
// borders blanked. The renderer moves the cursor across blank cells and
// redraws only changed cells, so a pattern allows any whitespace between words.
func assertIssue929TUI(t *testing.T, text string, patterns ...string) {
	t.Helper()
	text = strings.ReplaceAll(text, "│", " ")
	for _, pattern := range patterns {
		if !regexp.MustCompile(pattern).MatchString(text) {
			t.Fatalf("TUI did not show %s:\n%s", pattern, text)
		}
	}
}
