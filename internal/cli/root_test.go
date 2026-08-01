package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/skillbundle"
	packyversion "github.com/yersonargotev/packy/internal/version"
)

func TestDoctorJSONHealthyWarningsAndFailures(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
			return setuphealth.Report{SchemaVersion: 1, Kind: "doctor", Checks: []setuphealth.Check{{Severity: setuphealth.Pass, Name: "fixture", Detail: "healthy"}}, Summary: setuphealth.Summary{Status: "healthy", Passes: 1}}, nil
		}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		var doc struct {
			SchemaVersion int    `json:"schema_version"`
			Report        string `json:"report"`
			Checks        []struct{ Name, Severity, Detail string }
			Summary       setupHealthJSONSummary `json:"summary"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
		if doc.SchemaVersion != 1 || doc.Report != "doctor" || doc.Summary.Status != "healthy" || len(doc.Checks) == 0 {
			t.Fatalf("unexpected report: %#v", doc)
		}
		if strings.Contains(out, "HOME=") || strings.Contains(out, "PASS ") {
			t.Fatalf("human output mixed into JSON: %s", out)
		}
	})
	t.Run("warnings", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
			return setuphealth.Report{SchemaVersion: 1, Kind: "doctor", Checks: []setuphealth.Check{{Severity: setuphealth.Warn, Name: "fixture", Detail: "warning"}}, Summary: setuphealth.Summary{Status: "warnings", Warnings: 1}}, nil
		}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		var doc struct {
			Summary setupHealthJSONSummary `json:"summary"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Summary.Status != "warnings" || doc.Summary.Warnings == 0 {
			t.Fatalf("warning report: %#v err=%v", doc, err)
		}
	})
	t.Run("failures emit full report before error", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
			return setuphealth.Report{SchemaVersion: 1, Kind: "doctor", Checks: []setuphealth.Check{
				{Severity: setuphealth.Fail, Name: "failed", Detail: "failure"},
				{Severity: setuphealth.Warn, Name: "later", Detail: "complete report"},
			}, Summary: setuphealth.Summary{Status: "failures", Warnings: 1, Failures: 1}}, nil
		}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if !errors.Is(err, ErrDoctorUnhealthy) {
			t.Fatalf("error=%v", err)
		}
		var doc struct {
			Checks  []struct{ Name, Severity string }
			Summary setupHealthJSONSummary `json:"summary"`
		}
		if json.Unmarshal([]byte(out), &doc) != nil || doc.Summary.Failures == 0 || len(doc.Checks) < 2 {
			t.Fatalf("incomplete report: %s", out)
		}
	})
}

func TestDoctorReportsOnlyActivePackHealthWithoutSideEffects(t *testing.T) {
	t.Run("converged active pack", func(t *testing.T) {
		terminal := &fakeTerminal{interactive: true, approve: true}
		opts, home, _ := packActivationOptions(t, terminal)
		opts.SurfaceAdapters = alwaysUsableAdapters(t, opts)
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
			t.Fatalf("activate: %v\n%s", err, out)
		}
		before := snapshotTree(t, home)
		prompts := terminal.calls

		human, err := executeCommand(t, NewRootCommand(opts), "doctor")
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, human)
		}
		for _, want := range []string{"PASS packy-core", "PASS pack-matty-codex", "converged and ready", "SUMMARY status=healthy passes=2"} {
			if !strings.Contains(human, want) {
				t.Fatalf("human doctor missing %q:\n%s", want, human)
			}
		}

		jsonOutput, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if err != nil {
			t.Fatalf("doctor JSON: %v\n%s", err, jsonOutput)
		}
		var report struct {
			Checks  []setupHealthJSONCheck `json:"checks"`
			Summary setupHealthJSONSummary `json:"summary"`
		}
		if err := json.Unmarshal([]byte(jsonOutput), &report); err != nil || len(report.Checks) != 2 || report.Checks[1].Name != "pack-matty-codex" || report.Summary.Status != "healthy" {
			t.Fatalf("doctor JSON = %+v err=%v\n%s", report, err, jsonOutput)
		}
		if snapshotTree(t, home) != before || terminal.calls != prompts {
			t.Fatal("doctor mutated sandbox state or requested approval")
		}
	})

	t.Run("active drift fails with current remediation", func(t *testing.T) {
		terminal := &fakeTerminal{interactive: true, approve: true}
		opts, home, _ := packActivationOptions(t, terminal)
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
			t.Fatalf("activate: %v\n%s", err, out)
		}
		if err := os.Remove(filepath.Join(home, ".codex", "AGENTS.md")); err != nil {
			t.Fatal(err)
		}
		before := snapshotTree(t, home)
		prompts := terminal.calls

		out, err := executeCommand(t, NewRootCommand(opts), "doctor")
		if !errors.Is(err, ErrDoctorUnhealthy) {
			t.Fatalf("doctor error=%v\n%s", err, out)
		}
		for _, want := range []string{"FAIL pack-matty-codex", "projection findings", "packy pack reconcile matty --surface codex", "packy pack status matty --surface codex"} {
			if !strings.Contains(out, want) {
				t.Fatalf("drift doctor missing %q:\n%s", want, out)
			}
		}
		for _, removed := range []string{"packy install", "packy uninstall", "packy update"} {
			if strings.Contains(out, removed) {
				t.Fatalf("doctor named removed command %q:\n%s", removed, out)
			}
		}
		if snapshotTree(t, home) != before || terminal.calls != prompts {
			t.Fatal("drift diagnosis mutated sandbox state or requested approval")
		}
	})

	t.Run("pending human action warns", func(t *testing.T) {
		terminal := &fakeTerminal{interactive: true, approve: true}
		opts, home, _ := packActivationOptions(t, terminal)
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
			t.Fatalf("activate: %v\n%s", err, out)
		}
		before := snapshotTree(t, home)
		out, err := executeCommand(t, NewRootCommand(opts), "doctor")
		if err != nil || !strings.Contains(out, "WARN pack-matty-codex") || !strings.Contains(out, "readiness is pending") {
			t.Fatalf("pending doctor: %v\n%s", err, out)
		}
		if snapshotTree(t, home) != before {
			t.Fatal("pending diagnosis mutated sandbox state")
		}
	})
}

type fakeCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []fakeCall
	path  map[string]string
	fail  map[string]error
	after map[string]func()
}

type countingEnv struct {
	values map[string]string
	calls  map[string]int
}

func (e *countingEnv) Getenv(key string) string {
	e.calls[key]++
	return e.values[key]
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if f.path != nil {
		if path, ok := f.path[name]; ok {
			return path, nil
		}
	}
	return "", os.ErrNotExist
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, fakeCall{name: name, args: append([]string(nil), args...)})
	key := strings.Join(append([]string{name}, args...), " ")
	if f.fail != nil {
		if err, ok := f.fail[key]; ok {
			return err
		}
	}
	if f.after != nil {
		if after, ok := f.after[key]; ok {
			after()
		}
	}
	return nil
}

func executeCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func sandboxOptions(t *testing.T) (Options, *fakeRunner, string) {
	t.Helper()
	home := t.TempDir()
	sourceRoot := createSkillSource(t)
	homebrewPrefix := filepath.Join(t.TempDir(), "homebrew")
	homebrewBin := filepath.Join(homebrewPrefix, "bin")
	engram := writeEngramExecutable(t, homebrewBin, "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": engram}}
	return Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg-config"),
			"XDG_CACHE_HOME":      filepath.Join(home, "xdg-cache"),
			"CODEX_HOME":          filepath.Join(home, ".codex"),
			"PATH":                homebrewBin,
			"HOMEBREW_PREFIX":     homebrewPrefix,
			"PACKY_SKILLS_SOURCE": sourceRoot,
		},
		Runner: runner,
		EngramFacts: engrambin.Facts{
			Version:        func(string) (string, error) { return "1.19.0", nil },
			ServeProcesses: func() ([]engrambin.Process, error) { return nil, nil },
		},
	}, runner, home
}

func expandHomebrewEngramCalls(t *testing.T, opts Options, calls []string) []string {
	t.Helper()
	engram := newCLITestFixture(t, opts).engram.ExpectedPath()
	expanded := make([]string, 0, len(calls))
	for _, call := range calls {
		expanded = append(expanded, strings.ReplaceAll(call, "<homebrew-engram>", engram))
	}
	return expanded
}

func engramSetupCallStrings(t *testing.T, opts Options) []string {
	t.Helper()
	engram := newCLITestFixture(t, opts).engram.ExpectedPath()
	return []string{engram + " setup codex", engram + " setup opencode"}
}

func engramUpdateCallStrings(t *testing.T, opts Options) []string {
	t.Helper()
	return append([]string{"brew update", "brew upgrade engram"}, engramSetupCallStrings(t, opts)...)
}

func createSkillSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	createSkillSourceAt(t, root)
	return root
}

func createSkillSourceAt(t *testing.T, root string) {
	t.Helper()
	for _, rel := range testSkillSourceRels() {
		dir := filepath.Join(root, rel)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir skill source: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+filepath.Base(dir)+"\n---\n"), 0o600); err != nil {
			t.Fatalf("write skill source: %v", err)
		}
	}
}

func testSkillSourceRels() []string {
	return []string{
		"engineering/ask-matt",
		"engineering/codebase-design",
		"productivity/grilling",
		"productivity/handoff",
		"in-progress/loop-me",
		"engineering/wayfinder",
	}
}

func testSkillNames() []string {
	names := make([]string, 0, len(testSkillSourceRels()))
	for _, rel := range testSkillSourceRels() {
		names = append(names, filepath.Base(rel))
	}
	return names
}

func createUnmanagedSkillSymlinks(t *testing.T, skills skillbundle.GlobalLayout, targetRoot string) {
	t.Helper()
	if err := os.MkdirAll(skills.Root(), 0o700); err != nil {
		t.Fatalf("mkdir agent skills: %v", err)
	}
	if err := os.MkdirAll(targetRoot, 0o700); err != nil {
		t.Fatalf("mkdir unmanaged target root: %v", err)
	}
	for _, name := range testSkillNames() {
		target := filepath.Join(targetRoot, name)
		if err := os.Symlink(target, skills.Skill(name)); err != nil {
			t.Fatalf("write unmanaged symlink %s: %v", name, err)
		}
	}
}

func installedSkillSourceRoot(home string) string {
	return skillbundle.SourceRoot(bootstrap.DefaultInstalledSourceRoot(home))
}

func createRepoCheckoutSkillSource(t *testing.T) (string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	skillSource := skillbundle.SourceRoot(repoRoot)
	createSkillSourceAt(t, skillSource)
	return repoRoot, skillSource
}

func withVersion(t *testing.T, value string) {
	t.Helper()
	previous := packyversion.Value
	packyversion.Value = value
	t.Cleanup(func() {
		packyversion.Value = previous
	})
}

func TestHelpRendersForRootAndV0Subcommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{"--help"}, want: []string{"Manage Packy capability packs and sources", "version", "init", "doctor", "pack"}},
		{name: "version", args: []string{"version", "--help"}, want: []string{"Print the Packy version"}},
		{name: "doctor", args: []string{"doctor", "--help"}, want: []string{"Check Packy setup"}},
		{name: "pack", args: []string{"pack", "--help"}, want: []string{"Discover and manage opt-in capability packs"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, _ := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), tt.args...)
			if err != nil {
				t.Fatalf("help command failed: %v\n%s", err, out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("help output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestVersionOutput(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "default dev", version: "dev"},
		{name: "injected release", version: "v0.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withVersion(t, tt.version)
			opts, _, _ := sandboxOptions(t)

			for _, args := range [][]string{{"--version"}, {"version"}} {
				out, err := executeCommand(t, NewRootCommand(opts), args...)
				if err != nil {
					t.Fatalf("version command %v failed: %v\n%s", args, err, out)
				}
				want := "packy version " + tt.version + "\n"
				if out != want {
					t.Fatalf("version output for %v = %q, want %q", args, out, want)
				}
			}
		})
	}
}

func TestHelpAndVersionDoNotResolveWorkstation(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"init", "--help"}, {"version", "--help"}, {"--version"}, {"version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env := &countingEnv{values: map[string]string{}, calls: map[string]int{}}
			getwdCalls := 0
			opts := Options{
				Env: env,
				Getwd: func() (string, error) {
					getwdCalls++
					return "", errors.New("must not capture cwd")
				},
			}
			if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			if getwdCalls != 0 || len(env.calls) != 0 {
				t.Fatalf("workstation captured for %v: getwd=%d env=%v", args, getwdCalls, env.calls)
			}
		})
	}
}

func chdirTempOutsideRepo(t *testing.T) string {
	t.Helper()
	cwd := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	return cwd
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func callStrings(calls []fakeCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, strings.Join(append([]string{call.name}, call.args...), " "))
	}
	return out
}

func fixedTestTime() time.Time { return time.Unix(0, 0).UTC() }

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, fmt.Sprintf("%s symlink %s mode=%s mod=%d", rel, target, info.Mode(), info.ModTime().UnixNano()))
		case entry.IsDir():
			entries = append(entries, fmt.Sprintf("%s dir mode=%s mod=%d", rel, info.Mode(), info.ModTime().UnixNano()))
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entries = append(entries, fmt.Sprintf("%s file mode=%s mod=%d size=%d %s", rel, info.Mode(), info.ModTime().UnixNano(), info.Size(), string(data)))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return strings.Join(entries, "\n")
}

func TestInitClonesDefaultInstalledSourceAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	if !exists(filepath.Join(sourceRoot, "bundle", "skills")) {
		t.Fatalf("init did not clone bundle/skills into %s", sourceRoot)
	}
	for _, want := range []string{"cloning Installed Source into " + sourceRoot, "initialized Installed Source"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, sourceRoot) {
		t.Fatalf("init output did not report initialized source:\n%s", out)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("second init failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already initialized") {
		t.Fatalf("second init did not report idempotent state:\n%s", out)
	}
}

func TestInitCapturesOneWorkstationSnapshot(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	env := &countingEnv{
		values: map[string]string{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, "xdg-config"),
			"PATH":            "/sandbox/bin",
			"HOMEBREW_PREFIX": "/sandbox/homebrew",
		},
		calls: map[string]int{},
	}
	getwdCalls := 0
	opts := Options{
		Env: env,
		Getwd: func() (string, error) {
			getwdCalls++
			return t.TempDir(), nil
		},
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if getwdCalls != 1 {
		t.Fatalf("getwd calls = %d; want 1", getwdCalls)
	}
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "PATH", "HOMEBREW_PREFIX"} {
		if env.calls[key] != 1 {
			t.Fatalf("%s calls = %d; want 1 (all calls: %v)", key, env.calls[key], env.calls)
		}
	}
}

func TestInitPreservesMissingHomeError(t *testing.T) {
	out, err := executeCommand(t, NewRootCommand(Options{
		Env:   MapEnv{},
		Getwd: func() (string, error) { return "", errors.New("cwd unavailable") },
	}), "init")
	if err == nil || err.Error() != "HOME is required" {
		t.Fatalf("error = %v; want HOME is required\n%s", err, out)
	}
}

func TestInitWithAbsoluteSourceDoesNotRequireCurrentDirectory(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	sourceRoot := filepath.Join(t.TempDir(), "installed")
	opts := Options{
		Env:   MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")},
		Getwd: func() (string, error) { return "", errors.New("cwd unavailable") },
	}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--source-root", sourceRoot, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(sourceRoot, "bundle", "skills")) {
		t.Fatalf("init did not use absolute source root")
	}
}

func TestInitRejectsMalformedExistingInstalledSourceWithoutMutation(t *testing.T) {
	home := t.TempDir()
	installedRoot := bootstrap.DefaultInstalledSourceRoot(home)
	manifest := filepath.Join(installedRoot, "bundle", "skills", "engineering", "ask-matt", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, home)
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init")
	if err == nil {
		t.Fatalf("expected malformed Installed Source error, got output:\n%s", out)
	}
	for _, want := range []string{"not a valid Packy checkout", "Move it aside", "--source-root"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("init mutated malformed Installed Source\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestInitDoesNotPublishMalformedClonedSource(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	manifest := filepath.Join(repo, "bundle", "skills", "engineering", "ask-matt", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "add", ".")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "malformed")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err == nil {
		t.Fatalf("expected malformed cloned source error, got output:\n%s", out)
	}
	for _, want := range []string{"invalid skill bundle", "productivity"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if exists(bootstrap.DefaultInstalledSourceRoot(home)) {
		t.Fatalf("init published malformed source at %s", bootstrap.DefaultInstalledSourceRoot(home))
	}
}

func TestInitDoesNotReplaceValidInstalledSourceWithMalformedRef(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	if err := os.RemoveAll(filepath.Join(repo, "bundle", "skills", "productivity")); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repo, "add", "-A")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "malformed")
	runGitCommand(t, repo, "tag", "v0.2.0")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}
	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.1.0"); err != nil {
		t.Fatalf("initialize valid source: %v\n%s", err, out)
	}
	installedRoot := bootstrap.DefaultInstalledSourceRoot(home)
	beforeHead := strings.TrimSpace(runGitCommand(t, installedRoot, "rev-parse", "HEAD"))
	beforeBundle := snapshotTree(t, filepath.Join(installedRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.2.0")
	if err == nil {
		t.Fatalf("expected malformed ref error, got output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "invalid skill bundle") || !strings.Contains(err.Error(), "productivity") {
		t.Fatalf("malformed ref error is not actionable: %v", err)
	}
	if afterHead := strings.TrimSpace(runGitCommand(t, installedRoot, "rev-parse", "HEAD")); afterHead != beforeHead {
		t.Fatalf("Installed Source HEAD changed from %s to %s", beforeHead, afterHead)
	}
	if afterBundle := snapshotTree(t, filepath.Join(installedRoot, "bundle")); afterBundle != beforeBundle {
		t.Fatalf("malformed ref replaced valid bundle\nbefore:\n%s\nafter:\n%s", beforeBundle, afterBundle)
	}
}

func TestInitReportsUpdateProgress(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	if err := os.WriteFile(filepath.Join(repo, "UPDATED"), []byte("updated"), 0o600); err != nil {
		t.Fatalf("write update fixture: %v", err)
	}
	runGitCommand(t, repo, "add", "UPDATED")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "updated")
	runGitCommand(t, repo, "tag", "v0.2.0")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.1.0"); err != nil {
		t.Fatalf("initial init failed: %v\n%s", err, out)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.2.0")
	if err != nil {
		t.Fatalf("update init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	for _, want := range []string{"updating Installed Source at " + sourceRoot + " to v0.2.0", "updated Installed Source"} {
		if !strings.Contains(out, want) {
			t.Fatalf("update output missing %q:\n%s", want, out)
		}
	}
}

func TestInitReportsProgressAndGitContextWhenCloneFails(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", filepath.Join(t.TempDir(), "missing-repository"))
	if err == nil {
		t.Fatalf("expected clone failure, got output:\n%s", out)
	}
	if !strings.Contains(out, "cloning Installed Source into "+sourceRoot) {
		t.Fatalf("clone failure output did not include progress:\n%s", out)
	}
	for _, want := range []string{"clone Packy Source of Truth", "git clone", "failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("clone failure missing %q: %v", want, err)
		}
	}
}

func TestInitSupportsHomeFlag(t *testing.T) {
	envHome := t.TempDir()
	flagHome := t.TempDir()
	repo := createPackySourceRepo(t)
	opts := Options{Env: MapEnv{"HOME": envHome, "XDG_CONFIG_HOME": filepath.Join(flagHome, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--home", flagHome, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(flagHome, ".local", "share", "packy", "bundle", "skills")) {
		t.Fatalf("init did not use --home for default Installed Source")
	}
	if exists(filepath.Join(envHome, ".local", "share", "packy")) {
		t.Fatalf("init --home unexpectedly wrote Env HOME")
	}
}

func TestInitSupportsExplicitSourceRoot(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	sourceRoot := filepath.Join(t.TempDir(), "custom-source")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--source-root", sourceRoot, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(sourceRoot, "bundle", "skills")) {
		t.Fatalf("init did not clone into explicit source root")
	}
	if exists(filepath.Join(home, ".local", "share", "packy")) {
		t.Fatalf("init with --source-root unexpectedly wrote default Installed Source")
	}
}

func TestInitNormalizesRelativeSourceRootFromCapturedDirectory(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	repo := createPackySourceRepo(t)
	opts := Options{
		Env:   MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")},
		Getwd: func() (string, error) { return cwd, nil },
	}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--source-root", filepath.Join("relative", "source"), "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(cwd, "relative", "source", "bundle", "skills")) {
		t.Fatalf("init did not resolve relative --source-root from captured cwd")
	}
}

func TestInitRejectsInvalidNonEmptyDestination(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "README.md"), []byte("not packy"), 0o600); err != nil {
		t.Fatalf("write invalid destination: %v", err)
	}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err == nil {
		t.Fatalf("expected invalid destination error, got output:\n%s", out)
	}
	for _, want := range []string{"not a valid Packy checkout", "Move it aside", "--source-root"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if !exists(filepath.Join(sourceRoot, "README.md")) {
		t.Fatalf("init removed user data from invalid destination")
	}
}

func TestInitDefaultsReleaseVersionAsRepositoryRef(t *testing.T) {
	withVersion(t, "v0.2.3")
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.2.3")
	runGitCommand(t, repo, "checkout", "-b", "next")
	if err := os.WriteFile(filepath.Join(repo, "UNRELEASED"), []byte("main only"), 0o600); err != nil {
		t.Fatalf("write unreleased file: %v", err)
	}
	runGitCommand(t, repo, "add", "UNRELEASED")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "unreleased")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	if exists(filepath.Join(sourceRoot, "UNRELEASED")) {
		t.Fatalf("release init cloned repository HEAD instead of release tag")
	}
	got := strings.TrimSpace(runGitCommand(t, sourceRoot, "rev-parse", "HEAD"))
	want := strings.TrimSpace(runGitCommand(t, repo, "rev-parse", "v0.2.3^{commit}"))
	if got != want {
		t.Fatalf("cloned HEAD = %s, want release tag commit %s", got, want)
	}
}

func createPackySourceRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, rel := range []string{
		"bundle/skills/engineering/ask-matt/SKILL.md",
		"bundle/skills/engineering/codebase-design/SKILL.md",
		"bundle/skills/productivity/grilling/SKILL.md",
		"bundle/skills/productivity/handoff/SKILL.md",
		"bundle/skills/in-progress/loop-me/SKILL.md",
		"bundle/skills/engineering/wayfinder/SKILL.md",
	} {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir repo fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
			t.Fatalf("write repo fixture: %v", err)
		}
	}
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "add", ".")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "initial")
	return repo
}

func runGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	home := t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg-config"), "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func writeEngramExecutable(t *testing.T, dir, versionOutput string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir executable dir: %v", err)
	}
	path := filepath.Join(dir, "engram")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo '" + versionOutput + "'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}
