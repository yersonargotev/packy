package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassicLifecycleCommandsAndAuthorityAreAbsent(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	root := NewRootCommand(opts)

	help, err := executeCommand(t, root, "--help")
	if err != nil {
		t.Fatalf("root help: %v\n%s", err, help)
	}
	for _, command := range []string{"install", "update", "uninstall"} {
		if strings.Contains(help, "\n  "+command+" ") {
			t.Fatalf("root help retained classic command %q:\n%s", command, help)
		}

		out, err := executeCommand(t, NewRootCommand(opts), command)
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("%s command error = %v, want unknown command\n%s", command, err, out)
		}
	}

	if matches, err := filepath.Glob("../corelifecycle/*.go"); err != nil || len(matches) != 0 {
		t.Fatalf("classic lifecycle authority still has Go files: %v, %v", matches, err)
	}
}

func TestCoreOperationsWithoutActivationPreserveEverySurface(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(repositoryRoot, "bundle", "skills")
	configHome := opts.Env.(MapEnv)["XDG_CONFIG_HOME"]
	legacy := map[string]string{
		filepath.Join(home, ".packy", "config.json"):           "{classic state must remain unread}\n",
		filepath.Join(home, ".codex", "AGENTS.md"):             "codex-user-bytes\n",
		filepath.Join(configHome, "opencode", "opencode.json"): "{\"user\":true}\n",
		filepath.Join(configHome, "opencode", "packy.md"):      "opencode-user-bytes\n",
		filepath.Join(home, ".claude", "CLAUDE.md"):            "claude-user-bytes\n",
		filepath.Join(home, ".claude", "settings.json"):        "{\"user\":true}\n",
		filepath.Join(home, ".agents", "skills", "legacy"):     "shared-skill-user-bytes\n",
	}
	for path, content := range legacy {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	installedSource := createPackySourceRepo(t)
	before := snapshotTree(t, home)
	commands := [][]string{
		{"--help"},
		{"version"},
		{"init", "--source-root", installedSource},
		{"doctor"},
		{"pack", "list"},
		{"pack", "show", "engram"},
		{"pack", "status"},
	}
	for _, args := range commands {
		if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("packy %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	if after := snapshotTree(t, home); after != before {
		t.Fatalf("core operations without activation mutated a CLI surface:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("core operations without activation ran external effects: %#v", runner.calls)
	}
}

func TestPackActivationPreservesClassicArtifactsWithoutAdoptingThem(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	legacyStatePath := filepath.Join(home, ".packy", "config.json")
	legacySkillPath := filepath.Join(home, ".agents", "skills", "ask-matt", "SKILL.md")
	legacyState := []byte("{classic state must remain unread}\n")
	legacySkill := []byte("classic operator-owned skill\n")
	for path, content := range map[string][]byte{
		legacyStatePath: legacyState,
		legacySkillPath: legacySkill,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotTree(t, home)
	packID := "mat" + "ty"

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", "codex")
	if err == nil {
		t.Fatalf("activation unexpectedly adopted a classic artifact:\n%s", out)
	}
	for _, want := range []string{"Cannot apply activation", "ownership", "ask-matt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("blocked activation omitted %q:\n%s\nerror: %v", want, out, err)
		}
	}
	if terminal.calls != 0 {
		t.Fatalf("blocked activation prompted %d times", terminal.calls)
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("blocked activation mutated classic artifacts:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if got, readErr := os.ReadFile(legacyStatePath); readErr != nil || string(got) != string(legacyState) {
		t.Fatalf("classic state changed: %q, %v", got, readErr)
	}
	if got, readErr := os.ReadFile(legacySkillPath); readErr != nil || string(got) != string(legacySkill) {
		t.Fatalf("classic skill changed or gained ownership: %q, %v", got, readErr)
	}
}
