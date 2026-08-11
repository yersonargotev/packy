package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/skillbundle"
	"github.com/yersonargotev/packy/internal/workstation"
)

type alwaysUsableAdapter struct{ delegate capabilitypack.SurfaceAdapter }

type controlledCheckHostAdapter struct {
	delegate    capabilitypack.SurfaceAdapter
	hostVersion string
}

func (a *controlledCheckHostAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	inspection, err := a.delegate.InspectSurface(ctx, transition)
	inspection.ControlledCheck.HostVersion = a.hostVersion
	return inspection, err
}

func (a *controlledCheckHostAdapter) ApplyProjections(ctx context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	return a.delegate.ApplyProjections(ctx, actions)
}

func (a alwaysUsableAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	inspection, err := a.delegate.InspectSurface(ctx, transition)
	inspection.Readiness = capabilitypack.ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true, Evidence: []string{"fake runtime loaded capability"}}
	return inspection, err
}

func TestParseSurfaceAliasesAcceptsRepeatableQualifiedAliases(t *testing.T) {
	if aliases, err := parseSurfaceAliases(nil); err != nil || aliases != nil {
		t.Fatalf("omitted aliases = %+v, err=%v; want nil intent-preserving input", aliases, err)
	}
	aliases, err := parseSurfaceAliases([]string{"command:build=addy-build", "agent:reviewer=addy-reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	want := []capabilitypack.SurfaceAlias{{Kind: "command", ID: "build", Name: "addy-build"}, {Kind: "agent", ID: "reviewer", Name: "addy-reviewer"}}
	if !reflect.DeepEqual(aliases, want) {
		t.Fatalf("aliases = %+v, want %+v", aliases, want)
	}
}

func TestParseSurfaceAliasesRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{"build", "command:build", "command:=name", ":build=name", "command:build=", "command:build=name=extra"} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseSurfaceAliases([]string{input}); err == nil || !strings.Contains(err.Error(), "--alias") {
				t.Fatalf("parse error = %v", err)
			}
		})
	}
}

func TestReadinessValuePreservesUnknown(t *testing.T) {
	for _, value := range []capabilitypack.ReadinessValue{capabilitypack.ReadinessTrue, capabilitypack.ReadinessFalse, capabilitypack.ReadinessUnknown} {
		if got := readinessValue(value); got != string(value) {
			t.Fatalf("readinessValue(%q) = %q, want %q", value, got, value)
		}
	}
}

func TestPackLifecycleJSONPreviewUsesCanonicalStructuredContract(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "activate", "ma"+"tty", "--surface", "codex", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report capabilitypack.JSONLifecyclePlan
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid lifecycle JSON: %v\n%s", err, out)
	}
	if report.SchemaVersion != capabilitypack.LifecycleJSONSchemaVersion || report.Report != "pack-lifecycle-preview" || !report.DryRun || report.Operation != capabilitypack.OperationActivate || report.Contract.Bindings == nil || report.Contract.Exclusions == nil || report.Contract.OptionalModes == nil || report.Contract.PromptAuthorities == nil || report.Aliases == nil || report.Phases == nil || report.Blockers == nil || report.PendingHumanActions == nil || report.Evidence == nil || report.PendingEvidence == nil {
		t.Fatalf("incomplete lifecycle contract: %#v", report)
	}
	if terminal.calls != 0 || snapshotTree(t, home) != before {
		t.Fatal("JSON dry-run prompted or mutated sandbox")
	}
}

func TestPackLifecycleJSONFailureIsStructuredAndEffectFree(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "activate", "ma"+"tty", "--surface", "codex", "--alias", "command:missing=alias", "--json")
	if err == nil {
		t.Fatal("invalid alias unexpectedly succeeded")
	}
	var failure capabilitypack.JSONLifecycleFailure
	if json.Unmarshal([]byte(out), &failure) != nil || failure.Report != "pack-lifecycle-failure" || failure.Stage != "preview" || failure.ActionsExecuted == nil || *failure.ActionsExecuted != 0 || failure.ApprovalRequested == nil || *failure.ApprovalRequested {
		t.Fatalf("failure contract=%#v\n%s", failure, out)
	}
	if terminal.calls != 0 || snapshotTree(t, home) != before {
		t.Fatal("blocked JSON preview prompted or mutated sandbox")
	}
}

func TestPackLifecycleJSONCancellationReportsRequestedApprovalAndZeroEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: false}
	opts, home, _ := packActivationOptions(t, terminal)
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "activate", "ma"+"tty", "--surface", "codex", "--json")
	if err == nil {
		t.Fatal("cancelled activation unexpectedly succeeded")
	}
	decoder := json.NewDecoder(strings.NewReader(out))
	var preview capabilitypack.JSONLifecyclePlan
	var failure capabilitypack.JSONLifecycleFailure
	if decoder.Decode(&preview) != nil || decoder.Decode(&failure) != nil || failure.Stage != "approval" || failure.ApprovalRequested == nil || !*failure.ApprovalRequested || failure.ActionsExecuted == nil || *failure.ActionsExecuted != 0 {
		t.Fatalf("cancellation events: preview=%#v failure=%#v\n%s", preview, failure, out)
	}
	if terminal.calls != 1 || snapshotTree(t, home) != before {
		t.Fatal("cancelled JSON activation mutated sandbox or requested extra approval")
	}
}

func (a alwaysUsableAdapter) ApplyProjections(ctx context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	return a.delegate.ApplyProjections(ctx, actions)
}

func alwaysUsableAdapters(t *testing.T, opts Options) map[capabilitypack.Surface]capabilitypack.SurfaceAdapter {
	t.Helper()
	layout := resolvePackTestLayout(t, opts.Env)
	bundleRoot := skillbundle.BundleRoot(opts.Env.Getenv("PACKY_SKILLS_SOURCE"))
	return map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
		capabilitypack.SurfaceCodex:    alwaysUsableAdapter{delegate: codex.NewSurfaceAdapterWithConfig(bundleRoot, layout.skills.Root(), layout.codex.PromptFile(), layout.codex.ConfigFile())},
		capabilitypack.SurfaceOpenCode: alwaysUsableAdapter{delegate: opencode.NewSurfaceAdapter(bundleRoot, layout.skills.Root(), layout.openCode.ConfigFile(), layout.openCode.PromptFile())},
	}
}

type packTestLayout struct {
	packyHome string
	state     capabilitypack.StateLayout
	skills    skillbundle.GlobalLayout
	codex     codex.CanonicalLayout
	openCode  opencode.CanonicalLayout
}

func resolvePackTestLayout(t *testing.T, env Env) packTestLayout {
	t.Helper()
	snapshot, err := workstation.Resolve(workstation.Inputs{
		Home:              env.Getenv("HOME"),
		ConfigurationHome: env.Getenv("XDG_CONFIG_HOME"),
	}, workstation.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return packTestLayout{
		packyHome: snapshot.PackyHome(),
		state:     capabilitypack.NewStateLayout(snapshot.PackyHome()),
		skills:    skillbundle.NewGlobalLayout(snapshot.Home()),
		codex:     codex.NewCanonicalLayout(snapshot.Home()),
		openCode:  opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
	}
}

func TestPackVerbHelpUsesFlatCommandPaths(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	for _, verb := range []string{"list", "show", "check", "activate", "install", "update", "status", "deactivate", "uninstall"} {
		t.Run(verb, func(t *testing.T) {
			out, err := executeCommand(t, NewRootCommand(opts), verb, "--help")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "packy "+verb) || strings.Contains(out, "packy pack") {
				t.Fatalf("%s help does not use the flat path:\n%s", verb, out)
			}
			if strings.Contains(out, "Project installation writes the shared") {
				t.Fatalf("%s help duplicates shared root lifecycle guidance:\n%s", verb, out)
			}
		})
	}
}

func TestControlledRuntimeCheckIsExplicitPersonalEvidenceAndSatisfiesStrictStatus(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	layout := resolvePackTestLayout(t, opts.Env)
	bundleRoot := skillbundle.BundleRoot(opts.Env.Getenv("PACKY_SKILLS_SOURCE"))
	adapter := &controlledCheckHostAdapter{
		delegate:    codex.NewSurfaceAdapterWithConfig(bundleRoot, layout.skills.Root(), layout.codex.PromptFile(), layout.codex.ConfigFile()),
		hostVersion: "codex/v1",
	}
	opts.SurfaceAdapters = map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: adapter}
	if _, err := executeCommand(t, NewRootCommand(opts), "activate", "orchestrate", "--surface", "codex"); err != nil {
		t.Fatalf("activate orchestrate: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(home, ".packy", "packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := executeCommand(t, NewRootCommand(opts), "status", "orchestrate", "--surface", "codex")
	if err != nil || !strings.Contains(unknown, "usable=unknown") || !strings.Contains(unknown, "Controlled runtime check: unknown") {
		t.Fatalf("unknown status: err=%v\n%s", err, unknown)
	}
	focusedUnknown, err := executeCommand(t, NewRootCommand(opts), "status", "orchestrate", "--surface", "codex", "--resource", "skill:orchestrate", "--require", "usable")
	if err == nil || !strings.Contains(focusedUnknown, "Focused resource: skill:orchestrate configured=true authorized=true usable=unknown") {
		t.Fatalf("focused unknown strict status: err=%v\n%s", err, focusedUnknown)
	}
	preview, err := executeCommand(t, NewRootCommand(opts), "check", "orchestrate", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CONTROLLED RUNTIME CHECK DRY-RUN", "Selected resource closure:", "Projection revision:", "Adapter version:", "Observable host version:", "Check instructions:"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("controlled check preview omitted %q:\n%s", want, preview)
		}
	}
	if _, err := executeCommand(t, NewRootCommand(opts), "check", "orchestrate", "--surface", "codex", "--result", "positive"); err != nil {
		t.Fatalf("record positive controlled check: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(home, ".packy", "packs.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("controlled check changed installed receipts: err=%v", err)
	}
	if info, err := os.Stat(filepath.Join(home, ".packy", "controlled-checks.json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("personal evidence file: info=%v err=%v", info, err)
	}
	current, err := executeCommand(t, NewRootCommand(opts), "status", "orchestrate", "--surface", "codex", "--require", "usable")
	if err != nil || !strings.Contains(current, "usable=true") || !strings.Contains(current, "Controlled runtime check: current result=true") {
		t.Fatalf("current positive strict status: err=%v\n%s", err, current)
	}
	focusedCurrent, err := executeCommand(t, NewRootCommand(opts), "status", "orchestrate", "--surface", "codex", "--resource", "skill:orchestrate", "--require", "usable", "--json")
	if err != nil || !strings.Contains(focusedCurrent, `"satisfied":true`) || !strings.Contains(focusedCurrent, `"reason":"runtime-confirmed"`) || !strings.Contains(focusedCurrent, `"controlled-check:`) {
		t.Fatalf("focused current strict JSON status: err=%v\n%s", err, focusedCurrent)
	}
	adapter.hostVersion = "codex/v2"
	focusedStale, err := executeCommand(t, NewRootCommand(opts), "status", "orchestrate", "--surface", "codex", "--resource", "skill:orchestrate", "--require", "usable", "--json")
	if err == nil || !strings.Contains(focusedStale, `"usable":"unknown"`) || !strings.Contains(focusedStale, `"satisfied":false`) || !strings.Contains(focusedStale, `"reason":"runtime-check-stale"`) {
		t.Fatalf("focused stale strict JSON status: err=%v\n%s", err, focusedStale)
	}
	if _, err := executeCommand(t, NewRootCommand(opts), "check", "orchestrate", "--surface", "codex", "--result", "negative"); err != nil {
		t.Fatalf("record negative controlled check: %v", err)
	}
	focusedNegative, err := executeCommand(t, NewRootCommand(opts), "status", "orchestrate", "--surface", "codex", "--resource", "skill:orchestrate", "--require", "usable", "--json")
	if err == nil || !strings.Contains(focusedNegative, `"usable":"false"`) || !strings.Contains(focusedNegative, `"satisfied":false`) || !strings.Contains(focusedNegative, `"reason":"runtime-rejected"`) || !strings.Contains(focusedNegative, "rerun the check") {
		t.Fatalf("focused negative strict JSON status: err=%v\n%s", err, focusedNegative)
	}
}

func TestProjectControlledRuntimeCheckDoesNotEnterProjectArtifacts(t *testing.T) {
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	opts.Getwd = func() (string, error) { return project, nil }
	if _, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex", "--resource", "skill:ask-matt"); err != nil {
		t.Fatalf("install project matty: %v", err)
	}
	manifestBefore, err := os.ReadFile(filepath.Join(project, "packy.json"))
	if err != nil {
		t.Fatal(err)
	}
	lockBefore, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkOutput, err := executeCommand(t, NewRootCommand(opts), "check", "matty", "--surface", "codex", "--project", "--result", "negative")
	if err != nil {
		t.Fatalf("record project controlled check: %v", err)
	}
	manifestAfter, _ := os.ReadFile(filepath.Join(project, "packy.json"))
	lockAfter, _ := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if !bytes.Equal(manifestBefore, manifestAfter) || !bytes.Equal(lockBefore, lockAfter) {
		t.Fatal("controlled check entered project manifest or lock")
	}
	status, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--project")
	if err != nil || !strings.Contains(status, "Controlled runtime check: current result=false") || !strings.Contains(status, "usable=false") {
		t.Fatalf("project negative status: err=%v\ncheck:\n%s\nstatus:\n%s", err, checkOutput, status)
	}
}

func TestObsoletePackRouteIsRejected(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "list")
	if err == nil || !strings.Contains(err.Error(), `unknown command "pack"`) {
		t.Fatalf("obsolete route error = %v, output:\n%s", err, out)
	}
}

func TestRootCompletionOffersFlatPackVerbsWithoutPackGroup(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "__complete", "")
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "Completion ended with directive:") {
			continue
		}
		command, _, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("malformed completion line %q", line)
		}
		commands = append(commands, command)
	}
	want := []string{"activate", "check", "completion", "deactivate", "doctor", "help", "init", "install", "list", "show", "status", "uninstall", "update", "version"}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("root completion commands = %q, want %q\n%s", commands, want, out)
	}
}

func TestPackListUsesOneCapturedWorkstationForSkillSource(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	captures := 0
	opts := Options{
		Env: MapEnv{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, "xdg"),
		},
		Getwd: func() (string, error) {
			captures++
			return repoRoot, nil
		},
	}

	out, err := executeCommand(t, NewRootCommand(opts), "list")
	if err != nil {
		t.Fatalf("pack list: %v\n%s", err, out)
	}
	if captures != 1 {
		t.Fatalf("workstation captures = %d, want 1", captures)
	}
	if !strings.Contains(out, "matty") || !strings.Contains(out, "engram") {
		t.Fatalf("pack list did not use repository Skill Source:\n%s", out)
	}
}

func TestPackLifecycleRejectsInvalidBundleResourceBeforeMutation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	malformedSkill := filepath.Join(bundle, "skills", "engineering", "unlisted-broken")
	if err := os.MkdirAll(malformedSkill, 0o700); err != nil {
		t.Fatal(err)
	}
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	runner := opts.Runner.(*fakeRunner)
	before := snapshotTree(t, home)

	out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex")
	if err == nil {
		t.Fatalf("expected invalid bundle resource error, got output:\n%s", out)
	}
	for _, want := range []string{"malformed", "unlisted-broken", "missing SKILL.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if terminal.calls != 0 || len(runner.calls) != 0 {
		t.Fatalf("invalid bundle prompted or ran external effects: prompts=%d calls=%#v", terminal.calls, runner.calls)
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("invalid bundle mutated HOME\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

type fakeTerminal struct {
	interactive bool
	approve     bool
	calls       int
	onApprove   func()
	answers     []bool
	prompts     []string
}

type versionOutputRunner struct{ output string }

func (r versionOutputRunner) LookPath(name string) (string, error)         { return "/bin/" + name, nil }
func (r versionOutputRunner) Run(context.Context, string, ...string) error { return nil }
func (r versionOutputRunner) RunOutput(context.Context, string, ...string) (string, string, int, error) {
	return r.output, "", 0, nil
}

func TestObservableSurfaceVersionUsesSanitizedHostVersion(t *testing.T) {
	if got := observableSurfaceVersion(context.Background(), versionOutputRunner{output: "codex-cli 2.4.1\n"}, "codex"); got != "codex/2.4.1" {
		t.Fatalf("host version = %q", got)
	}
	if got := observableSurfaceVersion(context.Background(), versionOutputRunner{output: "TOKEN=secret"}, "codex"); got != "unobservable" {
		t.Fatalf("secret-shaped host output = %q", got)
	}
	if got := observableSurfaceVersion(context.Background(), versionOutputRunner{output: "/Users/alice/codex build"}, "codex"); got != "unobservable" {
		t.Fatalf("path-shaped host output = %q", got)
	}
}

func (f *fakeTerminal) Interactive(io.Reader) bool                   { return f.interactive }
func (f *fakeTerminal) InteractiveSession(io.Reader, io.Writer) bool { return f.interactive }
func (f *fakeTerminal) Approve(_ io.Reader, _ io.Writer, prompt string) (bool, error) {
	f.calls++
	f.prompts = append(f.prompts, prompt)
	if f.onApprove != nil {
		f.onApprove()
	}
	if len(f.answers) >= f.calls {
		return f.answers[f.calls-1], nil
	}
	return f.approve, nil
}

func packActivationOptions(t *testing.T, terminal Terminal) (Options, string, string) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	return Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}, Runner: &fakeRunner{}, Terminal: terminal}, home, repoRoot
}

func checkedInMattyFacts(t *testing.T) (string, int) {
	t.Helper()
	var manifest struct {
		Version   string            `json:"version"`
		Resources []json.RawMessage `json:"resources"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "bundle", "packs", "matty", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest.Version, len(manifest.Resources)
}

func currentPackActivationOptions(t *testing.T, terminal Terminal) (Options, string, string) {
	t.Helper()
	return packActivationOptions(t, terminal)
}

func engramActivationOptions(t *testing.T, terminal Terminal) (Options, string, string, *fakeRunner) {
	t.Helper()
	opts, home, repoRoot := currentPackActivationOptions(t, terminal)
	prefix := filepath.Join(t.TempDir(), "homebrew")
	engram := writeEngramExecutable(t, filepath.Join(prefix, "bin"), "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": engram}}
	opts.Runner = runner
	opts.EngramFormulaInspector = func(_ context.Context, formula string) (engrambin.FormulaMetadata, error) {
		return engrambin.FormulaMetadata{Source: formula, Version: "0.4.2"}, nil
	}
	env := opts.Env.(MapEnv)
	env["HOMEBREW_PREFIX"] = prefix
	env["PATH"] = filepath.Dir(engram)
	env["OPENCODE_CONFIG"] = ""
	env["OPENCODE_CONFIG_CONTENT"] = ""
	env["OPENCODE_CONFIG_DIR"] = ""
	return opts, home, repoRoot, runner
}

func TestPackActivateCodexDryRunIsCompletelySideEffectFree(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	_, resources := checkedInMattyFacts(t)
	for _, want := range []string{"Activation dry-run plan plan-", "Digest:", "Phase: reversible-local", "link skill ask-matt", fmt.Sprintf("Logical resources: %d skill, 0 instruction", resources)} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, retired := range []string{"matty-guidance", "matty-workflow-conventions", "write instruction"} {
		if strings.Contains(out, retired) {
			t.Fatalf("retired Matty instruction %q entered Codex preview:\n%s", retired, out)
		}
	}
	if terminal.calls != 0 {
		t.Fatalf("dry-run requested approval %d times", terminal.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("dry-run mutated HOME:\n%s", got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("dry-run mutated source bundle")
	}
}

func TestArgoteActivationPreviewIsApplicableOnEverySurface(t *testing.T) {
	for _, surface := range []string{"claude", "codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, terminal)
			beforeHome := snapshotTree(t, home)

			out, err := executeCommand(t, NewRootCommand(opts), "activate", "argote", "--surface", surface, "--dry-run")
			if err != nil {
				t.Fatalf("Argote %s dry-run failed: %v\n%s", surface, err, out)
			}
			for _, want := range []string{"Plan disposition: applicable", "Logical resources: 1 skill, 1 instruction", "instruction:guidance", "skill:espera-que"} {
				if !strings.Contains(out, want) {
					t.Fatalf("Argote %s preview missing %q:\n%s", surface, want, out)
				}
			}
			if strings.Contains(out, "target-collision") {
				t.Fatalf("Argote %s preview contains a target collision:\n%s", surface, out)
			}
			if terminal.calls != 0 || snapshotTree(t, home) != beforeHome {
				t.Fatalf("Argote %s dry-run prompted or mutated HOME", surface)
			}
		})
	}
}

func TestArgoteCodexActivationSurvivesReceiptReloadAndCanBeDeactivated(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)

	if out, err := executeCommand(t, NewRootCommand(opts), "activate", "argote", "--surface", "codex"); err != nil {
		t.Fatalf("activate Argote: %v\n%s", err, out)
	}
	status, err := executeCommand(t, NewRootCommand(opts), "status", "argote", "--surface", "codex")
	if err != nil {
		t.Fatalf("inspect Argote: %v\n%s", err, status)
	}
	for _, want := range []string{"Lifecycle state: active", "Readiness: configured=true", "Projections: 2 verified; 0 drifted; 0 ambiguous; 0 missing; 0 unmanaged"} {
		if !strings.Contains(status, want) {
			t.Fatalf("active Argote status missing %q:\n%s", want, status)
		}
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "deactivate", "argote", "--surface", "codex"); err != nil {
		t.Fatalf("deactivate Argote: %v\n%s", err, out)
	}
	if prompt, err := os.ReadFile(filepath.Join(home, ".codex", "AGENTS.md")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	} else if strings.Contains(string(prompt), "packy:pack:guidance") || strings.Contains(string(prompt), "# Argote guidance") {
		t.Fatalf("deactivation retained Argote instruction block:\n%s", prompt)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "espera-que")); !os.IsNotExist(err) {
		t.Fatalf("deactivation retained Argote skill: %v", err)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "activate", "argote", "--surface", "codex"); err != nil {
		t.Fatalf("reactivate Argote: %v\n%s", err, out)
	}
	status, err = executeCommand(t, NewRootCommand(opts), "status", "argote", "--surface", "codex")
	if err != nil || !strings.Contains(status, "Projections: 2 verified; 0 drifted; 0 ambiguous; 0 missing; 0 unmanaged") {
		t.Fatalf("reactivated Argote is not verified: %v\n%s", err, status)
	}
}

func TestPackActivateCodexSelectsOneV4ResourceThroughLifecycle(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := currentPackActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")

	before := snapshotTree(t, home)
	dryRun, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex", "--resource", "skill:ask-matt", "--resource", "skill:ask-matt", "--dry-run")
	if err != nil {
		t.Fatalf("selected dry-run failed: %v\n%s", err, dryRun)
	}
	for _, want := range []string{"Activation dry-run plan", "Selection mode: custom", "Selection root: skill:ask-matt", "link skill ask-matt"} {
		if !strings.Contains(dryRun, want) {
			t.Fatalf("selected dry-run missing %q:\n%s", want, dryRun)
		}
	}
	if strings.Contains(dryRun, "write instruction matty-guidance") {
		t.Fatalf("unselected resource entered dry-run:\n%s", dryRun)
	}
	if got := snapshotTree(t, home); got != before {
		t.Fatalf("selected dry-run mutated sandbox HOME:\n%s", got)
	}

	applied, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex", "--resource", "skill:ask-matt")
	if err != nil {
		t.Fatalf("selected activation failed: %v\n%s", err, applied)
	}
	if target, err := os.Readlink(filepath.Join(home, ".agents", "skills", "ask-matt")); err != nil || !strings.HasSuffix(target, "/skills/engineering/ask-matt") {
		t.Fatalf("selected ask-matt link = %q err=%v", target, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("unselected instructions were projected: %v", err)
	}
	state := readFileString(t, filepath.Join(home, ".packy", "packs.json"))
	for _, want := range []string{`"mode": "custom"`, `"kind": "skill"`, `"id": "ask-matt"`} {
		if !strings.Contains(state, want) {
			t.Fatalf("persisted selection missing %q:\n%s", want, state)
		}
	}

	status, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("selected status failed: %v\n%s", err, status)
	}
	for _, want := range []string{"Selection mode: custom", "Resource selection: skill:ask-matt role=root dependency_chain=skill:ask-matt", "role=unselected dependency_chain=none"} {
		if !strings.Contains(status, want) {
			t.Fatalf("selected status missing %q:\n%s", want, status)
		}
	}

	jsonStatus, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--json")
	if err != nil {
		t.Fatalf("selected JSON status failed: %v\n%s", err, jsonStatus)
	}
	var report capabilitypack.JSONStatusReport
	if err := json.Unmarshal([]byte(jsonStatus), &report); err != nil {
		t.Fatalf("invalid selected JSON status: %v\n%s", err, jsonStatus)
	}
	if len(report.Entries) != 1 || report.Entries[0].Intent.Selection.Mode != capabilitypack.SelectionCustom {
		t.Fatalf("selected JSON intent = %#v", report.Entries)
	}
	var selected, unselected bool
	for _, resource := range report.Entries[0].ResourceSelections {
		selected = selected || resource.Resource.String() == "skill:ask-matt" && resource.Selected &&
			resource.Role == capabilitypack.ResourceRoleRoot && len(resource.DependencyChain) == 1
		unselected = unselected || !resource.Selected && resource.Role == capabilitypack.ResourceRoleUnselected &&
			len(resource.DependencyChain) == 0
	}
	if !selected || !unselected {
		t.Fatalf("selected JSON resources = %#v", report.Entries[0].ResourceSelections)
	}

	additive, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex", "--resource", "skill:code-review")
	if err != nil {
		t.Fatalf("additive selection failed: %v\n%s", err, additive)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "code-review")); err != nil {
		t.Fatalf("additive skill was not projected: %v", err)
	}
	state = readFileString(t, filepath.Join(home, ".packy", "packs.json"))
	for _, want := range []string{`"id": "ask-matt"`, `"id": "code-review"`} {
		if !strings.Contains(state, want) {
			t.Fatalf("additive selection missing %q:\n%s", want, state)
		}
	}

	deactivated, err := executeCommand(t, NewRootCommand(opts), "deactivate", "matty", "--surface", "codex", "--resource", "skill:ask-matt")
	if err != nil {
		t.Fatalf("resource-scoped deactivation failed: %v\n%s", err, deactivated)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "ask-matt")); !os.IsNotExist(err) {
		t.Fatalf("removed skill projection remains: %v\n%s", err, deactivated)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "code-review")); err != nil {
		t.Fatalf("remaining skill projection was removed: %v", err)
	}
}

func TestPackStatusFocusesSelectedResourceAndRequiresFreshUsability(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	opts.SurfaceAdapters = alwaysUsableAdapters(t, opts)

	if out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex", "--resource", "skill:ask-matt"); err != nil {
		t.Fatalf("activate selected resource: %v\n%s", err, out)
	}
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--resource", "skill:ask-matt", "--require", "usable")
	if err != nil {
		t.Fatalf("focused usable status: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Resource readiness: skill:ask-matt role=root",
		"configured=true authorized=true usable=true",
		"Focused resource: skill:ask-matt configured=true authorized=true usable=true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("focused human status missing %q:\n%s", want, out)
		}
	}
	if got := snapshotTree(t, home); got != before {
		t.Fatalf("focused status mutated sandbox HOME:\nbefore:\n%s\nafter:\n%s", before, got)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--resource", "skill:ask-matt", "--require", "usable", "--json")
	if err != nil {
		t.Fatalf("focused JSON status: %v\n%s", err, out)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatalf("focused JSON decode: %v\n%s", err, out)
	}
	focused, _ := document["focused_resource"].(map[string]any)
	requirement, _ := document["requirement"].(map[string]any)
	if focused["role"] != "root" || requirement["readiness"] != "usable" || requirement["satisfied"] != true {
		t.Fatalf("focused JSON facts = focused:%#v requirement:%#v", focused, requirement)
	}

	for _, resource := range []string{"malformed", "instruction:matty-guidance"} {
		if _, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--resource", resource); err == nil {
			t.Fatalf("focused status unexpectedly accepted %q", resource)
		}
	}
}

func TestPackActivateCodexSelectedV4ResourceRejectsStalePlanWithoutEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	terminal.onApprove = func() {
		target := filepath.Join(home, ".agents", "skills", "ask-matt")
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "operator-owned"), []byte("preserve\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex", "--resource", "skill:ask-matt")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "stale") {
		t.Fatalf("selected stale error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".packy", "packs.json")); !os.IsNotExist(err) {
		t.Fatalf("selected stale plan wrote state: %v", err)
	}
	if got := readFileString(t, filepath.Join(home, ".agents", "skills", "ask-matt", "operator-owned")); got != "preserve\n" {
		t.Fatalf("selected stale plan changed operator content: %q", got)
	}
}

func TestPackActivateCodexRejectsNonTTYBeforeEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: false, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if terminal.calls != 0 {
		t.Fatalf("non-TTY requested approval")
	}
	if _, err := os.Stat(filepath.Join(home, ".packy", "packs.json")); !os.IsNotExist(err) {
		t.Fatalf("state written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("skills changed: %v", err)
	}
}

func TestPackActivateCodexAppliesApprovedPlanAndRepeatIsNoOp(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := currentPackActivationOptions(t, terminal)
	_, resourceCount := checkedInMattyFacts(t)
	out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("activate failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "Verified plan") || !strings.Contains(out, fmt.Sprintf("%d Codex projections", resourceCount)) {
		t.Fatalf("unexpected interaction/output: calls=%d\n%s", terminal.calls, out)
	}
	if target, err := os.Readlink(filepath.Join(home, ".agents", "skills", "ask-matt")); err != nil || !strings.HasSuffix(target, "/skills/engineering/ask-matt") {
		t.Fatalf("ask-matt link = %q err=%v", target, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("skills-only Matty Pack projected instructions: %v", err)
	}
	state, err := os.ReadFile(filepath.Join(home, ".packy", "packs.json"))
	if err != nil || !strings.Contains(string(state), `"receipts": [`) || strings.Contains(string(state), "contributors") || strings.Contains(string(state), "applying_journal") {
		t.Fatalf("state = %s err=%v", state, err)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("repeat failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "Already converged") {
		t.Fatalf("repeat was not approval-free no-op: calls=%d\n%s", terminal.calls, out)
	}
}

func TestPackActivateCodexStalePlanExecutesNoActions(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := currentPackActivationOptions(t, terminal)
	terminal.onApprove = func() {
		target := filepath.Join(home, ".agents", "skills", "ask-matt")
		_ = os.MkdirAll(target, 0o755)
		_ = os.WriteFile(filepath.Join(target, "operator-owned"), []byte("concurrent change\n"), 0o600)
	}

	_, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".packy", "packs.json")); !os.IsNotExist(err) {
		t.Fatalf("stale plan wrote state: %v", err)
	}
	if got := readFileString(t, filepath.Join(home, ".agents", "skills", "ask-matt", "operator-owned")); got != "concurrent change\n" {
		t.Fatalf("stale plan changed concurrent content: %q", got)
	}
}

func TestPackListAndShowAreSideEffectFree(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	runner := &fakeRunner{}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}, Runner: runner}
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))
	out, err := executeCommand(t, NewRootCommand(opts), "list")
	if err != nil {
		t.Fatalf("list failed: %v\n%s", err, out)
	}
	for _, want := range []string{"PACK", "argote", "engram", "matty", "Yerson Argote's engineering and communication guidance", "Selective durable memory", "codex"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list missing %q:\n%s", want, out)
		}
	}
	show, err := executeCommand(t, NewRootCommand(opts), "show", "engram")
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, show)
	}
	for _, want := range []string{"Requires global tools: engram", "1 skill, 0 instruction, 0 mcp_server, 0 lifecycle"} {
		if !strings.Contains(show, want) {
			t.Fatalf("show missing %q:\n%s", want, show)
		}
	}
	argoteShow, err := executeCommand(t, NewRootCommand(opts), "show", "argote")
	if err != nil {
		t.Fatalf("show Argote failed: %v\n%s", err, argoteShow)
	}
	for _, want := range []string{
		"argote 1.0.2", "Supported CLI surfaces: claude, codex, opencode", "Resources: 1 skill, 1 instruction",
		"Resource: instruction:guidance — Defines default engineering principles and neutral-Spanish communication guidance; role=operational dependencies=none notices=none",
		"Resource: skill:espera-que — Re-explains the last point in neutral Spanish when it did not land; role=operational dependencies=none notices=none",
	} {
		if !strings.Contains(argoteShow, want) {
			t.Fatalf("Argote show missing %q:\n%s", want, argoteShow)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %v", runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("HOME changed\nbefore:\n%s\nafter:\n%s", beforeHome, got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("bundle changed during discovery")
	}
	if _, err := os.Stat(filepath.Join(home, ".packy", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("state file exists: %v", err)
	}
}

func TestPackShowRejectsUnknownPack(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}}
	opts.Env.(MapEnv)["PATH"] = ""
	_, err := executeCommand(t, NewRootCommand(opts), "show", "mobile")
	if err == nil || !strings.Contains(err.Error(), "unknown capability pack") {
		t.Fatalf("error = %v", err)
	}
}

func TestPackStatusRendersBaselineWithoutSideEffects(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	runner := &fakeRunner{}
	opts := Options{Env: MapEnv{
		"HOME": home, "XDG_CONFIG_HOME": xdg, "PATH": "",
		"PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills"),
	}, Runner: runner}
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	overview, err := executeCommand(t, NewRootCommand(opts), "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, overview)
	}
	lines := strings.Split(strings.TrimSpace(overview), "\n")
	if len(lines) == 0 || !reflect.DeepEqual(strings.Fields(lines[0]), []string{"PACK", "SURFACE", "INTENT", "CONFIGURED", "AUTHORIZED", "USABLE", "ACTION"}) {
		t.Fatalf("status header is not semantic:\n%s", overview)
	}
	wantRows := expectedPackSurfaceRows(t, repoRoot)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) != 7 {
			t.Fatalf("malformed status row %q", line)
		}
		key := fields[0] + "/" + fields[1]
		if !wantRows[key] {
			t.Fatalf("unexpected or duplicate status row %q", line)
		}
		delete(wantRows, key)
		if fields[2] != "inactive" || fields[3] != "false" || !isReadinessValue(fields[4]) || !isReadinessValue(fields[5]) || fields[6] != "none" {
			t.Fatalf("status row changed semantics: %q", line)
		}
	}
	if len(wantRows) != 0 {
		t.Fatalf("status omitted Pack/surface rows: %#v\n%s", wantRows, overview)
	}

	detail, err := executeCommand(t, NewRootCommand(opts), "status", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("targeted status failed: %v\n%s", err, detail)
	}
	for _, want := range []string{
		"engram 2.0.0 on codex", "Intent: inactive", "Resources: 0 selected", "Receipt ownership: 0 projected paths", "Drift: 0 projections",
		"Readiness: configured=false, authorized=true, usable=false",
		"Projections: 0 verified; 0 drifted; 0 ambiguous", "Pending human actions: none",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail missing %q:\n%s", want, detail)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %v", runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("HOME changed\nbefore:\n%s\nafter:\n%s", beforeHome, got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("bundle changed during status")
	}
	if _, err := os.Stat(filepath.Join(home, ".packy", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("state file exists: %v", err)
	}
}

func isReadinessValue(value string) bool {
	return value == "true" || value == "false" || value == "unknown"
}

func TestPackStatusJSONOverviewAndTargetedAbsenceAreStable(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}}
	overview, err := executeCommand(t, NewRootCommand(opts), "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report capabilitypack.JSONStatusReport
	if err := json.Unmarshal([]byte(overview), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, overview)
	}
	wantRows := expectedPackSurfaceRows(t, repoRoot)
	if report.SchemaVersion != capabilitypack.StatusSchemaVersion || report.Report != "pack-status-overview" || len(report.Entries) != len(wantRows) {
		t.Fatalf("report=%#v", report)
	}
	for i, entry := range report.Entries {
		if i > 0 && (report.Entries[i-1].Pack > entry.Pack || report.Entries[i-1].Pack == entry.Pack && report.Entries[i-1].Surface > entry.Surface) {
			t.Fatalf("entries not sorted: %#v", report.Entries)
		}
		key := entry.Pack + "/" + string(entry.Surface)
		if !wantRows[key] {
			t.Fatalf("unexpected or duplicate JSON status entry: %#v", entry)
		}
		delete(wantRows, key)
	}
	if len(wantRows) != 0 {
		t.Fatalf("JSON status omitted Pack/surface entries: %#v", wantRows)
	}
	detail, err := executeCommand(t, NewRootCommand(opts), "status", "engram", "--surface", "codex", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(detail), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	entry := report.Entries[0]
	if report.Report != "pack-status" || entry.Intent.State != "absent" || entry.Intent.Active != nil || entry.Readiness.Authorized != capabilitypack.ReadinessTrue || entry.Readiness.Usable != capabilitypack.ReadinessFalse || entry.Blockers == nil || entry.Evidence == nil || entry.PendingHumanActions == nil {
		t.Fatalf("absence contract: %#v", entry)
	}
	if strings.Contains(detail, "Intent:") {
		t.Fatalf("human output mixed into JSON: %s", detail)
	}
}

func expectedPackSurfaceRows(t *testing.T, repoRoot string) map[string]bool {
	t.Helper()
	catalog, err := capabilitypack.Discover(filepath.Join(repoRoot, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	packs, err := catalog.ListCurrent()
	if err != nil {
		t.Fatal(err)
	}
	rows := make(map[string]bool)
	for _, pack := range packs {
		for _, surface := range pack.Surfaces {
			rows[pack.ID+"/"+string(surface)] = true
		}
	}
	return rows
}

func TestPackStatusJSONRequireEmitsDocumentBeforeGateError(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	out, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--require", "usable", "--json")
	if err == nil || !strings.Contains(err.Error(), "not freshly observed usable") {
		t.Fatalf("gate error=%v", err)
	}
	var report capabilitypack.JSONStatusReport
	if json.Unmarshal([]byte(out), &report) != nil || len(report.Entries) != 1 {
		t.Fatalf("missing JSON before gate: %s", out)
	}
	opts.SurfaceAdapters = alwaysUsableAdapters(t, opts)
	if activation, activateErr := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex"); activateErr != nil {
		t.Fatalf("activate: %v\n%s", activateErr, activation)
	}
	out, err = executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--require", "usable", "--json")
	if err != nil || json.Unmarshal([]byte(out), &report) != nil || report.Entries[0].Readiness.Usable != capabilitypack.ReadinessTrue {
		t.Fatalf("successful JSON gate: err=%v\n%s", err, out)
	}
}

func TestPackStatusRequiresCompleteTarget(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
		"PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills"),
	}}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"status", "engram"}, "--surface is required"},
		{[]string{"status", "--surface", "codex"}, "a pack is required"},
		{[]string{"status", "engram", "--surface", "vscode"}, "does not support CLI surface"},
		{[]string{"status", "missing", "--surface", "codex"}, "unknown capability pack"},
	} {
		_, err := executeCommand(t, NewRootCommand(opts), tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v error = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestPackStatusRequireUsableIsIndependentNonInteractiveGate(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	opts.SurfaceAdapters = alwaysUsableAdapters(t, opts)
	if _, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--require", "usable"); err == nil || !strings.Contains(err.Error(), "not freshly observed usable") {
		t.Fatalf("inactive gate error=%v", err)
	}
	if terminal.calls != 0 || exists(filepath.Join(home, ".packy", "packs.json")) {
		t.Fatal("failed status gate prompted or persisted")
	}
	if _, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex"); err != nil {
		t.Fatal(err)
	}
	prompts := terminal.calls
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--require", "usable")
	if err != nil || !strings.Contains(out, "configured=true, authorized=true, usable=true") {
		t.Fatalf("gate err=%v\n%s", err, out)
	}
	if terminal.calls != prompts || snapshotTree(t, home) != before {
		t.Fatal("successful status gate prompted or mutated files")
	}
	for _, args := range [][]string{{"status", "--require", "usable"}, {"status", "matty", "--surface", "codex", "--require", "authorized"}} {
		if _, err := executeCommand(t, NewRootCommand(opts), args...); err == nil || !strings.Contains(err.Error(), "valid only") {
			t.Fatalf("%v error=%v", args, err)
		}
	}
}

func TestPackActivatePackyAndFreshStatusAgreeRuntimeUsabilityIsPending(t *testing.T) {
	for _, surface := range []string{"codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, _, _ := packActivationOptions(t, terminal)
			out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", surface)
			if err != nil {
				t.Fatalf("activate: %v\n%s", err, out)
			}
			for _, want := range []string{"Readiness: configured=true, authorized=true, usable=unknown", "Pending evidence:"} {
				if !strings.Contains(out, want) {
					t.Fatalf("activate output missing %q:\n%s", want, out)
				}
			}
			status, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", surface, "--require", "usable")
			if err == nil || !strings.Contains(status, "Readiness: configured=true, authorized=true, usable=unknown") {
				t.Fatalf("usable gate: err=%v\n%s", err, status)
			}
		})
	}
}

func TestPackActivateOpenCodeDryRunIsCompletelySideEffectFree(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	opts.Env.(MapEnv)["OPENCODE_CONFIG"] = ""
	opts.Env.(MapEnv)["OPENCODE_CONFIG_CONTENT"] = ""
	opts.Env.(MapEnv)["OPENCODE_CONFIG_DIR"] = ""
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "opencode", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	_, resources := checkedInMattyFacts(t)
	for _, want := range []string{"Activation dry-run plan plan-", "Surface: opencode", "link OpenCode skill ask-matt", fmt.Sprintf("Logical resources: %d skill, 0 instruction", resources)} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, retired := range []string{"matty-guidance", "matty-workflow-conventions", "write OpenCode instruction", "add OpenCode instruction reference"} {
		if strings.Contains(out, retired) {
			t.Fatalf("retired Matty instruction %q entered OpenCode preview:\n%s", retired, out)
		}
	}
	if terminal.calls != 0 {
		t.Fatalf("dry-run requested approval")
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("dry-run mutated HOME:\n%s", got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("dry-run mutated source bundle")
	}
}

func TestCurrentMattyActivationProjectsSurfaceCapabilities(t *testing.T) {
	for _, surface := range []string{"codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, terminal)
			out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", surface)
			if err != nil {
				t.Fatalf("activate current Matty: %v\n%s", err, out)
			}
			if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "ask-matt")); err != nil {
				t.Fatalf("Matty skill was not projected: %v", err)
			}
			for _, retired := range []string{"matty-guidance", "matty-workflow-conventions"} {
				if strings.Contains(out, retired) {
					t.Fatalf("retired instruction %q entered activation output:\n%s", retired, out)
				}
			}
			retiredPaths := []string{filepath.Join(home, "xdg", "opencode", "matty-workflow-conventions.md")}
			if surface == "codex" {
				retiredPaths = append(retiredPaths, filepath.Join(home, ".codex", "AGENTS.md"))
			} else {
				prompt := filepath.Join(home, "xdg", "opencode", "packy.md")
				if data, err := os.ReadFile(prompt); err != nil || !strings.Contains(string(data), "Matty OpenCode skill trees") {
					t.Fatalf("OpenCode primary prompt = %q, %v", data, err)
				}
				config := readFileString(t, filepath.Join(home, "xdg", "opencode", "opencode.json"))
				if !strings.Contains(config, prompt) {
					t.Fatalf("OpenCode config omitted primary prompt %s:\n%s", prompt, config)
				}
			}
			for _, path := range retiredPaths {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("current Matty wrote retired instruction target %s: %v", path, err)
				}
			}
		})
	}
}

func TestPackActivateOpenCodeRejectsNonTTYBeforeEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: false, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	_, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "opencode")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("error = %v", err)
	}
	if terminal.calls != 0 {
		t.Fatal("non-TTY requested approval")
	}
	for _, path := range []string{filepath.Join(home, ".packy", "packs.json"), filepath.Join(home, ".agents"), filepath.Join(home, "xdg", "opencode")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("non-TTY wrote %s: %v", path, err)
		}
	}
}

func TestPackActivateOpenCodePreservesUnmanagedContentAndDoesNotMutateCodex(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := currentPackActivationOptions(t, terminal)
	xdg := filepath.Join(home, "xdg", "opencode")
	if err := os.MkdirAll(xdg, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(xdg, "opencode.json")
	existing := "// keep host syntax\n{\n  \"model\": \"anthropic/test\",\n  \"mcp\": {\"jira\": {\"enabled\": true,},},\n  \"instructions\": [\"CONTRIBUTING.md\",],\n}\n"
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	codexPath := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexPath, []byte("unmanaged Codex guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "opencode")
	if err != nil {
		t.Fatalf("activate failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "Verified plan") {
		t.Fatalf("interaction/output calls=%d\n%s", terminal.calls, out)
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// keep host syntax", `"model": "anthropic/test"`, `"jira"`, `"CONTRIBUTING.md"`} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("OpenCode config lost %q:\n%s", want, updated)
		}
	}
	codex, err := os.ReadFile(codexPath)
	if err != nil || string(codex) != "unmanaged Codex guidance\n" {
		t.Fatalf("Codex mutated: %q err=%v", codex, err)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "opencode")
	if err != nil {
		t.Fatalf("repeat failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "Already converged") {
		t.Fatalf("repeat not no-op: calls=%d\n%s", terminal.calls, out)
	}
}

func TestPackActivateEngramDryRunShowsOnlyReviewedSkillAndNoEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot, runner := engramActivationOptions(t, terminal)
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "activate", "engram", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Pack: engram 2.0.0", "Phase: reversible-local", "Logical resources: 1 skill", "link skill engram-memory"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"tool-host-setup", "engram setup", "host-follow-up", "mcp_servers.engram", "engram-instructions", "engram-compact", "engram@engram", "marketplaces.engram", "/hooks"} {
		if strings.Contains(strings.ToLower(out), forbidden) {
			t.Fatalf("output contains retired setup behavior %q:\n%s", forbidden, out)
		}
	}
	if terminal.calls != 0 || len(runner.calls) != 0 {
		t.Fatalf("dry-run requested effects: prompts=%d calls=%v", terminal.calls, runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("dry-run mutated HOME:\n%s", got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("dry-run mutated source bundle")
	}
}

func TestPackActivateEngramInstallsOnlyTheReviewedSkill(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _, runner := engramActivationOptions(t, terminal)
	out, err := executeCommand(t, NewRootCommand(opts), "activate", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("activate failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || len(terminal.prompts) != 1 || !strings.Contains(terminal.prompts[0], "reversible-local") {
		t.Fatalf("prompts = %#v calls=%d", terminal.prompts, terminal.calls)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %#v", runner.calls)
	}
	for _, want := range []string{"Readiness: configured=true, authorized=true, usable=unknown", "Verified plan"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	target := filepath.Join(home, ".agents", "skills", "engram-memory")
	if link, err := os.Readlink(target); err != nil || !strings.HasSuffix(link, "/skills/engram-memory") {
		t.Fatalf("Engram skill link = %q, %v", link, err)
	}
	for _, retired := range []string{
		filepath.Join(home, ".codex", "engram-instructions.md"),
		filepath.Join(home, ".codex", "engram-compact-prompt.md"),
		filepath.Join(home, ".codex", "config.toml"),
		filepath.Join(home, ".codex", "hooks"),
	} {
		if _, err := os.Stat(retired); !os.IsNotExist(err) {
			t.Fatalf("activation created retired setup artifact %s: %v", retired, err)
		}
	}
}

func TestPackActivateEngramAcquiresOnlyWhenExecutableIsMissing(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := currentPackActivationOptions(t, terminal)
	prefix := filepath.Join(t.TempDir(), "homebrew")
	runner := &fakeRunner{}
	opts.Runner = runner
	opts.EngramFormulaInspector = func(_ context.Context, formula string) (engrambin.FormulaMetadata, error) {
		return engrambin.FormulaMetadata{Source: formula, Version: "0.4.2"}, nil
	}
	env := opts.Env.(MapEnv)
	env["HOMEBREW_PREFIX"] = prefix
	env["PATH"] = ""
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "activate", "engram", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Phase: reversible-local", "Phase: executable-external", "brew install gentleman-programming/tap/engram"} {
		if !strings.Contains(out, want) {
			t.Fatalf("acquisition preview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "tool-host-setup") || strings.Contains(out, "engram setup") || terminal.calls != 0 || len(runner.calls) != 0 {
		t.Fatalf("acquisition preview included setup or effects: prompts=%d calls=%v\n%s", terminal.calls, runner.calls, out)
	}
	if snapshotTree(t, home) != beforeHome || snapshotTree(t, filepath.Join(repoRoot, "bundle")) != beforeBundle {
		t.Fatal("acquisition dry-run mutated sandbox state")
	}
	runner.after = map[string]func(){
		"brew install gentleman-programming/tap/engram": func() {
			engram := writeEngramExecutable(t, filepath.Join(prefix, "bin"), "engram version 1.19.0")
			runner.path = map[string]string{"engram": engram}
		},
	}
	applied, err := executeCommand(t, NewRootCommand(opts), "activate", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("activation with acquisition failed: %v\n%s", err, applied)
	}
	if terminal.calls != 2 || len(runner.calls) != 1 || callStrings(runner.calls)[0] != "brew install gentleman-programming/tap/engram" {
		t.Fatalf("acquisition effects prompts=%d calls=%v", terminal.calls, runner.calls)
	}
	if _, err := os.Readlink(filepath.Join(home, ".agents", "skills", "engram-memory")); err != nil {
		t.Fatalf("Engram skill missing after acquisition: %v", err)
	}

	unsupported, err := executeCommand(t, NewRootCommand(opts), "activate", "engram", "--surface", "opencode", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "lifecycle plan is not actionable") || !strings.Contains(unsupported, "declares no outcome on opencode") {
		t.Fatalf("OpenCode selective mode unexpectedly supported: %v\n%s", err, unsupported)
	}
}

func copyPackBundleForUpdate(t *testing.T, repoRoot string) string {
	t.Helper()
	root := t.TempDir()
	copyProductionCatalogBundle(t, root, repoRoot)
	return root
}

func copyProductionCatalogBundle(t *testing.T, target, repoRoot string) {
	t.Helper()
	for _, dir := range []string{"skills", "instructions", "agents", "commands", "references", "notices", "packs"} {
		if err := os.CopyFS(filepath.Join(target, dir), os.DirFS(filepath.Join(repoRoot, "bundle", dir))); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "bundle", "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "LICENSE"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPackDeactivateDryRunApplyAndInactiveNoOpOnBothSurfaces(t *testing.T) {
	currentVersion, _ := checkedInMattyFacts(t)
	for _, surface := range []string{"codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, terminal)
			if out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", surface); err != nil {
				t.Fatalf("seed: %v\n%s", err, out)
			}
			before := snapshotTree(t, home)
			prompts := terminal.calls
			out, err := executeCommand(t, NewRootCommand(opts), "deactivate", "matty", "--surface", surface, "--dry-run")
			if err != nil {
				t.Fatalf("dry-run: %v\n%s", err, out)
			}
			for _, want := range []string{"Deactivation dry-run plan plan-", "Active version: " + currentVersion, "Intent revision:", "Phase: destructive-cleanup"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q:\n%s", want, out)
				}
			}
			if terminal.calls != prompts || snapshotTree(t, home) != before {
				t.Fatal("deactivation dry-run prompted or mutated HOME")
			}
			out, err = executeCommand(t, NewRootCommand(opts), "deactivate", "matty", "--surface", surface)
			if err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("apply: %v\n%s", err, out)
			}
			out, err = executeCommand(t, NewRootCommand(opts), "deactivate", "matty", "--surface", surface)
			if err != nil || !strings.Contains(out, "Already converged") {
				t.Fatalf("no-op: %v\n%s", err, out)
			}
		})
	}
}

func TestPackDeactivateCancellationAndNonTTYHaveZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal *fakeTerminal
	}{{"cancel", &fakeTerminal{interactive: true, approve: false}}, {"non-tty", &fakeTerminal{interactive: false, approve: true}}} {
		t.Run(tc.name, func(t *testing.T) {
			seed := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, seed)
			if out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex"); err != nil {
				t.Fatalf("seed: %v\n%s", err, out)
			}
			opts.Terminal = tc.terminal
			before := snapshotTree(t, home)
			_, err := executeCommand(t, NewRootCommand(opts), "deactivate", "matty", "--surface", "codex")
			if err == nil {
				t.Fatal("unsafe deactivation succeeded")
			}
			if snapshotTree(t, home) != before {
				t.Fatal("cancel/non-TTY deactivation caused effects")
			}
			if tc.name == "cancel" && (len(tc.terminal.prompts) != 1 || !strings.Contains(tc.terminal.prompts[0], "destructive-cleanup")) {
				t.Fatalf("prompts=%v", tc.terminal.prompts)
			}
		})
	}
}
