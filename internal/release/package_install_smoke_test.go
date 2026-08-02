package release_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPackageInstallSmokeRequiresExplicitPackActivation(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and executes a local release binary")
	}

	root := repoRoot(t)
	assertSourceRepositoryExcludesExternalReferenceTrees(t, root)
	sandbox := t.TempDir()
	home := filepath.Join(sandbox, "home")
	xdgConfigHome := filepath.Join(sandbox, "xdg")
	outsideCheckout := filepath.Join(sandbox, "outside-checkout")
	stubBin := filepath.Join(sandbox, "bin")
	externalLog := filepath.Join(sandbox, "external-calls.log")
	for _, dir := range []string{home, xdgConfigHome, outsideCheckout, stubBin} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	binary := buildLocalReleaseBinary(t, root, sandbox, "v0.99.0")
	sourceRepo := createSmokeSourceRepo(t, root, sandbox, "v0.99.0")
	writeSmokeStub(t, stubBin, "engram", externalLog)
	writeSmokeStub(t, stubBin, "brew", externalLog)
	writeSmokeStub(t, stubBin, "claude", externalLog)
	env := append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+xdgConfigHome,
		"PATH="+stubBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOMEBREW_PREFIX="+sandbox,
		"GIT_CONFIG_NOSYSTEM=1",
	)

	legacy := map[string]string{
		filepath.Join(home, ".packy", "config.json"):                   "{classic state remains unread}\n",
		filepath.Join(home, ".codex", "AGENTS.md"):                     "codex-user-bytes\n",
		filepath.Join(xdgConfigHome, "opencode", "opencode.json"):      "{\"user\":true}\n",
		filepath.Join(xdgConfigHome, "opencode", "packy.md"):           "opencode-user-bytes\n",
		filepath.Join(home, ".claude", "CLAUDE.md"):                    "claude-user-bytes\n",
		filepath.Join(home, ".claude", "settings.json"):                "{\"user\":true}\n",
		filepath.Join(home, ".agents", "skills", "legacy", "SKILL.md"): "shared-skill-user-bytes\n",
	}
	for path, content := range legacy {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	help := runSmokeCommand(t, binary, outsideCheckout, env, "--help")
	for _, want := range []string{"Manage Packy capability packs and sources", "version", "init", "doctor", "pack"} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help output missing %q:\n%s", want, help)
		}
	}
	for _, removed := range []string{"install", "update", "uninstall"} {
		out, err := runSmokeCommandAllowError(t, binary, outsideCheckout, env, removed)
		if err == nil || !strings.Contains(out, "unknown command") {
			t.Fatalf("removed command %s did not fail as unknown: %v\n%s", removed, err, out)
		}
	}

	versionFlag := runSmokeCommand(t, binary, outsideCheckout, env, "--version")
	versionCommand := runSmokeCommand(t, binary, outsideCheckout, env, "version")
	if want := "packy version v0.99.0\n"; versionFlag != want || versionCommand != want {
		t.Fatalf("version outputs = flag %q, command %q; want %q", versionFlag, versionCommand, want)
	}
	runSmokeCommand(t, binary, outsideCheckout, env, "init", "--repository-url", sourceRepo)
	assertInitializedSourceExcludesExternalReferenceTrees(t, home)

	doctorJSON := runSmokeCommand(t, binary, outsideCheckout, env, "doctor", "--json")
	var doctor struct {
		SchemaVersion int               `json:"schema_version"`
		Report        string            `json:"report"`
		Checks        []json.RawMessage `json:"checks"`
	}
	if err := json.Unmarshal([]byte(doctorJSON), &doctor); err != nil {
		t.Fatalf("doctor --json emitted invalid JSON: %v\n%s", err, doctorJSON)
	}
	if doctor.SchemaVersion != 2 || doctor.Report != "doctor" || len(doctor.Checks) == 0 {
		t.Fatalf("doctor --json emitted unexpected shape: %#v", doctor)
	}

	for path, want := range legacy {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("core operation changed legacy surface %s: %q, %v", path, data, err)
		}
	}
	assertSmokePathExists(t, filepath.Join(home, ".local", "share", "packy", "bundle", "skills"), "init should create only the Installed Source substrate")
	assertSmokeExternalCalls(t, externalLog, nil)

	projectionPaths := map[string]string{
		"codex":    filepath.Join(home, ".agents", "skills", "api-and-interface-design"),
		"opencode": filepath.Join(home, ".agents", "skills", "api-and-interface-design"),
		"claude":   filepath.Join(home, ".claude", "skills", "api-and-interface-design"),
	}
	packID := "addy"
	for _, surface := range []string{"codex", "opencode", "claude"} {
		var observedReadOnlyCommands []string
		if surface == "claude" {
			observedReadOnlyCommands = []string{"claude --version"}
		}
		preview := runSmokeCommand(t, binary, outsideCheckout, env, "pack", "activate", packID, "--surface", surface, "--dry-run")
		if !strings.Contains(preview, "Activation dry-run plan") || !strings.Contains(preview, "Surface: "+surface) {
			t.Fatalf("%s activation preview omitted explicit surface intent:\n%s", surface, preview)
		}
		for path, want := range legacy {
			data, err := os.ReadFile(path)
			if err != nil || string(data) != want {
				t.Fatalf("%s activation preview changed legacy surface %s: %q, %v", surface, path, data, err)
			}
		}
		assertSmokeExternalCalls(t, externalLog, observedReadOnlyCommands)

		activation := runSmokeInteractiveCommand(t, binary, outsideCheckout, env, "y\n", "pack", "activate", packID, "--surface", surface)
		if !strings.Contains(activation, "Verified plan") || !strings.Contains(activation, "Apply result facts: verified=yes") {
			t.Fatalf("%s explicit activation did not report a verified Apply:\n%s", surface, activation)
		}
		assertSmokePathExists(t, projectionPaths[surface], "explicit activation should project the representative skill")
		if surface == "claude" {
			observedReadOnlyCommands = []string{"claude --version", "claude --version", "claude --version", "claude --version"}
		}
		assertSmokeExternalCalls(t, externalLog, observedReadOnlyCommands)

		status := runSmokeCommand(t, binary, outsideCheckout, env, "pack", "status", packID, "--surface", surface)
		for _, want := range []string{"configured", "authorized", "usable"} {
			if !strings.Contains(status, want) {
				t.Fatalf("%s status did not report readiness dimension %q separately from Apply:\n%s", surface, want, status)
			}
		}
	}
}
func buildLocalReleaseBinary(t *testing.T, root, sandbox, version string) string {
	t.Helper()
	binary := filepath.Join(sandbox, "packy")
	cmd := exec.Command("go", "build", "-ldflags", "-X github.com/yersonargotev/packy/internal/version.Value="+version, "-o", binary, "./cmd/packy")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+filepath.Join(sandbox, "go-home"),
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, "go-xdg"),
		"GOCACHE="+filepath.Join(sandbox, "go-cache"),
		"GOMODCACHE="+goEnv(t, "GOMODCACHE"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build local release binary: %v\n%s", err, output)
	}
	return binary
}

func createSmokeSourceRepo(t *testing.T, root, sandbox, version string) string {
	t.Helper()
	repo := filepath.Join(sandbox, "source-repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatalf("mkdir source repo fixture: %v", err)
	}
	if err := os.CopyFS(filepath.Join(repo, "bundle"), os.DirFS(filepath.Join(root, "bundle"))); err != nil {
		t.Fatalf("copy source bundle fixture: %v", err)
	}
	runSmokeGit(t, repo, sandbox, "init")
	runSmokeGit(t, repo, sandbox, "add", ".")
	runSmokeGit(t, repo, sandbox, "-c", "user.name=Packy Smoke", "-c", "user.email=packy-smoke@example.test", "commit", "-m", "fixture source")
	runSmokeGit(t, repo, sandbox, "tag", version)
	return repo
}

func runSmokeGit(t *testing.T, repo, sandbox string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(),
		"HOME="+filepath.Join(sandbox, "git-home"),
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, "git-xdg"),
		"GIT_CONFIG_NOSYSTEM=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func writeSmokeStub(t *testing.T, dir, name, logPath string) {
	t.Helper()
	script := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> " + shellQuote(logPath) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write %s stub: %v", name, err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func runSmokeCommand(t *testing.T, binary, dir string, env []string, args ...string) string {
	t.Helper()
	output, err := runSmokeCommandAllowError(t, binary, dir, env, args...)
	if err != nil {
		t.Fatalf("packy %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func runSmokeCommandAllowError(t *testing.T, binary, dir string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runSmokeInteractiveCommand(t *testing.T, binary, dir string, env []string, input string, args ...string) string {
	t.Helper()
	script, err := exec.LookPath("script")
	if err != nil {
		t.Fatalf("find script for pseudo-terminal smoke: %v", err)
	}
	var scriptArgs []string
	if runtime.GOOS == "darwin" {
		scriptArgs = append([]string{"-q", "/dev/null", binary}, args...)
	} else {
		command := shellQuote(binary)
		for _, arg := range args {
			command += " " + shellQuote(arg)
		}
		scriptArgs = []string{"-q", "-e", "-c", command, "/dev/null"}
	}
	cmd := exec.Command(script, scriptArgs...)
	cmd.Dir = dir
	cmd.Env = env
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pseudo-terminal input pipe: %v", err)
	}
	cmd.Stdin = stdinRead
	go func() {
		_, _ = stdinWrite.WriteString(input)
	}()
	output, err := cmd.CombinedOutput()
	_ = stdinWrite.Close()
	_ = stdinRead.Close()
	if err != nil {
		t.Fatalf("interactive packy %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func assertSourceRepositoryExcludesExternalReferenceTrees(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"engram", "gentle-ai", "skills"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("Packy source must not track external reference tree %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect external reference tree %s: %v", name, err)
		}
	}
}

func assertInitializedSourceExcludesExternalReferenceTrees(t *testing.T, home string) {
	t.Helper()
	root := filepath.Join(home, ".local", "share", "packy")
	for _, name := range []string{"engram", "gentle-ai", "skills"} {
		assertSmokePathMissing(t, filepath.Join(root, name), "initialized source must exclude external reference tree "+name)
	}
}

func assertSmokeExternalCalls(t *testing.T, logPath string, want []string) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if len(want) == 0 {
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Fatalf("read external call log: %v", err)
		}
		if strings.TrimSpace(string(data)) != "" {
			t.Fatalf("expected no external calls, got:\n%s", data)
		}
		return
	}
	if err != nil {
		t.Fatalf("read external call log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("external calls mismatch\nwant:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
}

func assertSmokePathExists(t *testing.T, path, reason string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s: %s: %v", reason, path, err)
	}
}

func assertSmokePathMissing(t *testing.T, path, reason string) {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		t.Fatalf("%s: %s exists", reason, path)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("%s: stat %s: %v", reason, path, err)
	}
}
