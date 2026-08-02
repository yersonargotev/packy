package claudesmoke

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestResolveSelector(t *testing.T) {
	for _, tc := range []struct {
		name, selector, metadata, version string
		wantErr                           bool
	}{
		{"floor", ExactFloor, `{"version":"2.1.203","dist.integrity":"sha512-floor"}`, "2.1.203", false},
		{"stable", "stable", `{"version":"2.2.0","dist.integrity":"sha512-stable"}`, "2.2.0", false},
		{"floor mismatch", ExactFloor, `{"version":"2.2.0","dist.integrity":"x"}`, "", true},
		{"forbidden", "latest", `{"version":"2.2.0","dist.integrity":"x"}`, "", true},
		{"malformed", "stable", `{`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := ResolveSelector(tc.selector, tc.metadata)
			if (err != nil) != tc.wantErr || got != tc.version {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestAllowedCommandRejectsInteractiveClaudeAndUnknownPacky(t *testing.T) {
	p, c := "/x/packy", "/x/claude"
	if !AllowedCommand(p, c, []string{c, "--version"}) {
		t.Fatal("version rejected")
	}
	for _, argv := range [][]string{
		{p, "pack", "list"},
		{p, "pack", "show", "addy"},
		{p, "pack", "activate", "addy", "--surface", "claude", "--dry-run"},
		{p, "pack", "activate", "addy", "--surface", "claude"},
		{p, "pack", "status", "addy", "--surface", "claude"},
	} {
		if !AllowedCommand(p, c, argv) {
			t.Fatalf("release activation command rejected: %#v", argv)
		}
	}
	for _, argv := range [][]string{{c}, {c, "--print", "hello"}, {c, "mcp", "list"}, {p, "pack", "show", "engram"}, {p, "doctor", "--json"}, {p, "install", "--dry-run"}, {"sh", "-c", "true"}} {
		if AllowedCommand(p, c, argv) {
			t.Fatalf("allowed %#v", argv)
		}
	}
}

func TestRunAllowedUsesCanonicalConfiguredPackyIdentity(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	for _, name := range []string{"packy", "packy_v0.1.9_darwin_amd64"} {
		configured := filepath.Join(root, name)
		if err := writeStub(configured, "#!/bin/sh\nexit 0\n"); err != nil {
			t.Fatal(err)
		}
		evidence := validEvidence()
		evidence.Commands[1] = runAllowed(context.Background(), root, root, RestrictedEnv(root, root), configured, filepath.Join(root, "claude"), []string{configured, "version"})
		if err := ValidateEvidence(evidence); err != nil {
			t.Fatalf("configured %s evidence was rejected: %v", name, err)
		}
	}

	configured := filepath.Join(root, "packy_v0.1.9_darwin_amd64")
	foreign := filepath.Join(root, "packy_v9.9.9_darwin_amd64")
	if err := writeStub(foreign, "#!/bin/sh\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	evidence := validEvidence()
	evidence.Commands[1] = runAllowed(context.Background(), root, root, RestrictedEnv(root, root), configured, filepath.Join(root, "claude"), []string{foreign, "version"})
	if err := ValidateEvidence(evidence); err == nil {
		t.Fatal("accepted substituted Packy executable")
	}
}

func TestRunInteractiveRestrictedProvidesTTYForExplicitActivation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec and script PTY are macOS-specific")
	}
	root := t.TempDir()
	packy := filepath.Join(root, "packy")
	if err := writeStub(packy, "#!/bin/sh\n[ -t 0 ] || exit 41\nread answer\n[ \"$answer\" = y ] || exit 42\nprintf 'Verified plan\\nApply result facts: verified=yes\\n'\n"); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(root, "claude")
	argv := []string{packy, "pack", "activate", "addy", "--surface", "claude"}
	evidence := runInteractiveRestricted(context.Background(), root, root, "", RestrictedEnv(root, root), packy, claude, "y\n", argv)
	if evidence.ExitCode != 0 || !strings.Contains(evidence.Stdout, "Verified plan") {
		t.Fatalf("interactive activation evidence = %#v", evidence)
	}
}

func TestRestrictedEnvIsAllowlistAndScrubsCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "super-secret")
	env := RestrictedEnv("/sandbox", "/sandbox/npm/bin")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "super-secret") || strings.Contains(joined, "ANTHROPIC") {
		t.Fatal("credential leaked")
	}
	for _, key := range []string{"HOME=/sandbox/home", "CLAUDE_CONFIG_DIR=/sandbox/home", "TMPDIR=/sandbox/tmp"} {
		if !strings.Contains(joined, key) {
			t.Fatalf("missing %s", key)
		}
	}
	for _, key := range []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1"} {
		if !strings.Contains(joined, key) {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestAcquisitionEnvUsesOnlyDisposableNPMState(t *testing.T) {
	t.Setenv("NPM_TOKEN", "operator-secret")
	env := strings.Join(acquisitionEnv("/sandbox", "/runtime/bin/npm"), "\n")
	for _, want := range []string{"HOME=/sandbox/acquisition/home", "XDG_CONFIG_HOME=/sandbox/acquisition/config", "NPM_CONFIG_CACHE=/sandbox/acquisition/cache", "NPM_CONFIG_USERCONFIG=/sandbox/acquisition/npmrc"} {
		if !strings.Contains(env, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(env, "operator-secret") || strings.Contains(env, "NPM_TOKEN") {
		t.Fatal("acquisition inherited npm credential")
	}
}

func TestAcquisitionCommandCannotObserveCallerNPMRC(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	caller := filepath.Join(root, "caller")
	acquisition := filepath.Join(root, "acquisition", "home")
	for _, dir := range []string{caller, acquisition, filepath.Join(root, "acquisition", "config"), filepath.Join(root, "acquisition", "cache"), filepath.Join(root, "acquisition", "tmp")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(caller, ".npmrc"), []byte("//registry/:_authToken=CALLER_SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "fake-npm")
	if err := writeStub(fake, "#!/bin/sh\npwd\n[ ! -e .npmrc ] || cat .npmrc\n"); err != nil {
		t.Fatal(err)
	}
	out, err := sandboxOutput(context.Background(), root, acquisition, acquisitionEnv(root, fake), fake, "view")
	if err != nil {
		t.Fatal(err)
	}
	realAcquisition, err := filepath.EvalSymlinks(acquisition)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "CALLER_SECRET") || strings.TrimSpace(out) != realAcquisition {
		t.Fatalf("caller npmrc observed: %q", out)
	}
}

func TestSandboxBoundaryAllowsOnlyConfiguredRootWrites(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	inside := filepath.Join(root, "inside")
	cmd, err := sandboxCommand(context.Background(), root, "/usr/bin/touch", inside)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("inside write: %v: %s", err, out)
	}
	cmd, err = sandboxCommand(context.Background(), root, "/usr/bin/touch", outside)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(); err == nil {
		t.Fatal("outside write escaped sandbox")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("outside file was created")
	}
}

func TestSandboxBoundaryDeniesCheckoutReads(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	checkout := t.TempDir()
	secret := filepath.Join(checkout, "fixture")
	if err := os.WriteFile(secret, []byte("must-not-be-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd, err := sandboxCommandDenyReads(context.Background(), root, checkout, "/bin/cat", secret)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("checkout fixture escaped read boundary: %s", output)
	}
}

func TestSandboxOutputKeepsSuccessDiagnosticsOutOfStructuredStdout(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	command := filepath.Join(root, "command")
	if err := writeStub(command, "#!/bin/sh\nprintf '%s' '{\"version\":\"2.1.203\"}'\nprintf '%s' 'npm notice update available' >&2\n"); err != nil {
		t.Fatal(err)
	}
	out, err := sandboxOutput(context.Background(), root, root, RestrictedEnv(root, filepath.Join(root, "bin")), command)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"version":"2.1.203"}` {
		t.Fatalf("structured stdout was contaminated: %q", out)
	}
}

func TestObserveAddyQualificationAcquiresInstalledSourceState(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	sandbox := t.TempDir()
	layout := newSandboxLayout(sandbox)
	for _, path := range append(layout.writableDirectories(), layout.SourceRepository) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	gitExecutable, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gitHome, gitConfig := t.TempDir(), t.TempDir()
	gitEnv := []string{
		"HOME=" + gitHome, "XDG_CONFIG_HOME=" + gitConfig,
		"PATH=" + filepath.Dir(gitExecutable) + ":/usr/bin:/bin",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(gitExecutable, args...)
		cmd.Dir = layout.InstalledSource
		cmd.Env = gitEnv
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	runGit("init")
	runGit("config", "user.name", "Smoke Test")
	runGit("config", "user.email", "smoke@example.invalid")
	if err := os.WriteFile(filepath.Join(layout.InstalledSource, "README"), []byte("proved\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README")
	runGit("commit", "-m", "proved source")
	commit := runGit("rev-parse", "HEAD")

	packy := filepath.Join(t.TempDir(), "packy")
	if err := writeStub(packy, "#!/bin/sh\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	checkout := t.TempDir()
	evidence := Evidence{
		InstalledSourceSHA: commit,
		Commands: []CommandEvidence{
			{Name: "claude", Args: []string{"--version"}, ExitCode: 0},
			{Name: "packy", Args: []string{"version"}, ExitCode: 0},
			{Name: "claude", Args: []string{"version"}, ExitCode: 0},
		},
		Safety: SafetyEvidence{CredentialsScrubbed: true, WriteBoundaryEnforced: true},
	}
	observation, err := observeAddyQualification(context.Background(), sandbox, checkout, packy, "", restrictedEnv(sandbox, filepath.Dir(gitExecutable)), layout, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.InstalledSourceClean || observation.InstalledSourceCommit != commit {
		t.Fatalf("clean Installed Source observation = %#v", observation)
	}
	if observation.WritableRoots.ClaudeConfig != layout.Home {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", observation.WritableRoots.ClaudeConfig, layout.Home)
	}
	if !observation.Safety.NoAuthentication || !observation.Safety.NoModelInvocation || !observation.Safety.NoPrint ||
		!observation.Safety.NoREPL || !observation.Safety.NoUpstreamExecute {
		t.Fatalf("safe direct and normalized Claude observations were rejected: %#v", observation.Safety)
	}
	evidence.Sandbox, evidence.Qualification = sandbox, observation
	encoded, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Evidence
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	bound, err := BindAddyQualification(AddyQualification{}, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if bound.InstalledSourceCommit != commit || bound.WritableRoots.ClaudeConfig != layout.Home {
		t.Fatalf("serialized observation was not bound: %#v", bound)
	}

	if err := os.WriteFile(filepath.Join(layout.InstalledSource, "untracked"), []byte("dirty"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty, err := observeAddyQualification(context.Background(), sandbox, checkout, packy, "", restrictedEnv(sandbox, filepath.Dir(gitExecutable)), layout, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if dirty.InstalledSourceClean {
		t.Fatal("dirty Installed Source was certified clean")
	}
}

func TestManifestDeterministicAndContentBound(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b"), []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	one, err := Manifest(root)
	if err != nil {
		t.Fatal(err)
	}
	two, err := Manifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, two) || one[0].Path != "a" || one[0].SHA256 == "" {
		t.Fatalf("non-deterministic: %#v %#v", one, two)
	}
}

func TestSurfaceManifestCoversEverySupportedAndSharedTarget(t *testing.T) {
	items := []FileEvidence{
		{Path: "home/.codex/AGENTS.md"},
		{Path: "config/opencode/opencode.json"},
		{Path: "home/.claude/CLAUDE.md"},
		{Path: "home/.claude.json"},
		{Path: "home/.agents/skills/example/SKILL.md"},
		{Path: "home/.packy/config.json"}, // Historical classic state is not a CLI surface.
	}
	got := surfaceManifest(items)
	if len(got) != 5 {
		t.Fatalf("surface manifest = %#v", got)
	}
	changed := append(append([]FileEvidence(nil), got...), FileEvidence{Path: "home/.codex/new-surface-file"})
	if reflect.DeepEqual(got, surfaceManifest(changed)) {
		t.Fatal("surface manifest ignored a newly created Codex artifact")
	}
}

func TestAddyProjectionMatchesInstalledSourceContent(t *testing.T) {
	root := t.TempDir()
	projection := filepath.Join(root, "home", ".claude", "skills", "api-and-interface-design", "SKILL.md")
	source := filepath.Join(root, "installed-source", "bundle", "history", "addy", "1.1.0", "skills", "api-and-interface-design", "SKILL.md")
	for _, path := range []string{projection, source} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("expected Addy skill\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !addyProjectionMatchesInstalledSource(root) {
		t.Fatal("matching representative projection was rejected")
	}
	if err := os.WriteFile(projection, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if addyProjectionMatchesInstalledSource(root) {
		t.Fatal("mismatched representative projection was accepted")
	}
}

func TestValidationFailureStillWritesDiagnosticEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.json")
	evidence := validEvidence()
	evidence.Assertions.NoActivationStateAfterInitialization = false
	if err := validateAndWriteEvidence(path, evidence); err == nil {
		t.Fatal("invalid evidence accepted")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"no_activation_state_after_initialization": false`)) {
		t.Fatalf("failed diagnostic evidence missing assertion: %s", b)
	}
}

func validEvidence() Evidence {
	sandbox := filepath.Join(string(filepath.Separator), "sandbox")
	args := [][]string{
		{"--version"}, {"version"},
		{"init", "--home", filepath.Join(sandbox, "home"), "--source-root", filepath.Join(sandbox, "installed-source"), "--repository-url", filepath.Join(sandbox, "source-repository"), "--repository-ref", syntheticSourceRef},
		{"doctor"}, {"pack", "list"}, {"pack", "show", "addy"},
		{"install"}, {"update"}, {"uninstall"},
		{"pack", "activate", "addy", "--surface", "claude", "--dry-run"},
		{"pack", "activate", "addy", "--surface", "claude"},
		{"pack", "status", "addy", "--surface", "claude"},
	}
	commands := make([]CommandEvidence, len(args))
	for i := range args {
		commands[i] = CommandEvidence{Name: "packy", Args: args[i], ExitCode: 0}
	}
	commands[0].Name = "claude"
	for i, name := range []string{"install", "update", "uninstall"} {
		commands[6+i].ExitCode = 1
		commands[6+i].Stderr = "unknown command \"" + name + "\" for \"packy\""
	}
	commands = append(commands, CommandEvidence{Name: "claude", Args: []string{"version"}, ExitCode: 0})
	sha := strings.Repeat("a", 40)
	manifest := []FileEvidence{{Path: "fixture", SHA256: strings.Repeat("c", 64), Mode: 0o600, Size: 1}}
	return Evidence{SchemaVersion: 3, PackyVersion: "v1", PackyRef: "v1", PackySHA: sha, InstalledSourceSHA: sha, RequestedClaudeVersion: ExactFloor, ResolvedClaudeVersion: ExactFloor, ClaudeIntegrity: "sha512-x", ClaudeDigest: strings.Repeat("b", 64), Sandbox: sandbox, Commands: commands, Before: manifest, After: manifest, Safety: SafetyEvidence{DisposableSandbox: true, AllowlistEnvironment: true, CredentialsScrubbed: true, CommandAllowlist: true, CheckoutUnchanged: true, ConfiguredWritableRootsConfined: true, EvidencePathOutsideSandbox: true, NoInteractiveClaude: true, WriteBoundaryEnforced: true}, Assertions: AssertionEvidence{InstalledSourceInitialized: true, DoctorReportedCoreHealthy: true, RemovedInstallRejected: true, RemovedUpdateRejected: true, RemovedUninstallRejected: true, ClassicStatePreserved: true, ClaudeInstructionPreserved: true, ClaudeMCPPreserved: true, SharedSkillSentinelPreserved: true, InitializationCausedNoSurfaceChange: true, ActivationPreviewCausedNoChange: true, RepresentativePackActivated: true, ReadinessInspectedSeparately: true, NoActivationStateAfterInitialization: true, NoClaudeMutationOperations: true, EngramStubProtocolVerified: true, SensitiveFixtureRedacted: true}}
}

func TestEvidenceSchemaV3ProvesInitializationThenExplicitActivation(t *testing.T) {
	data, err := json.Marshal(validEvidence())
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		SchemaVersion int             `json:"schema_version"`
		Assertions    map[string]bool `json:"assertions"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"installed_source_initialized", "doctor_reported_core_healthy",
		"removed_install_rejected", "removed_update_rejected", "removed_uninstall_rejected",
		"classic_state_preserved", "claude_instruction_preserved", "claude_mcp_preserved",
		"shared_skill_sentinel_preserved", "initialization_caused_no_surface_change",
		"activation_preview_caused_no_change", "representative_pack_activated",
		"readiness_inspected_separately", "no_activation_state_after_initialization",
		"no_claude_mutation_operations", "engram_stub_protocol_verified", "sensitive_fixture_redacted",
	}
	if document.SchemaVersion != 3 || len(document.Assertions) != len(want) {
		t.Fatalf("schema = %d, assertions = %#v", document.SchemaVersion, document.Assertions)
	}
	for _, name := range want {
		if !document.Assertions[name] {
			t.Fatalf("schema v3 omitted or failed %q: %#v", name, document.Assertions)
		}
	}
}

func TestLoggedStubCapturesExternalInvocation(t *testing.T) {
	root := t.TempDir()
	stub := filepath.Join(root, "engram")
	logPath := filepath.Join(root, "external.log")
	if err := writeLoggedStub(stub, logPath, "exit 0\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !externalLogEmpty(logPath) {
		t.Fatal("new external invocation log is not empty")
	}
	if err := exec.Command(stub, "setup").Run(); err != nil {
		t.Fatal(err)
	}
	if externalLogEmpty(logPath) {
		t.Fatal("external invocation was not detected")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "invoked\n" {
		t.Fatalf("external invocation log = %q", data)
	}
}

func TestValidateEvidenceRejectsTampering(t *testing.T) {
	e := validEvidence()
	if err := ValidateEvidence(e); err != nil {
		t.Fatal(err)
	}
	e.Commands[0].ExitCode = 1
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted failed command")
	}
	e = validEvidence()
	e.Safety.CheckoutUnchanged = false
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted checkout mutation")
	}
	e = validEvidence()
	e.Safety.ConfiguredWritableRootsConfined = false
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unproved sandbox confinement")
	}
	e = validEvidence()
	e.Commands[0].Stdout = "ANTHROPIC_API_KEY"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted credential marker")
	}
	e = validEvidence()
	e.Commands[6].Args = []string{"install", "--dry-run"}
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted tampered lifecycle sequence")
	}
	e = validEvidence()
	e.Commands[0].Name = "packy"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted tampered executable identity")
	}
	e = validEvidence()
	e.Commands[2].Args[3] = "--unexpected"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted tampered init flag")
	}
	e = validEvidence()
	e.Commands[2].Args[2] = filepath.Join(string(filepath.Separator), "outside")
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted init home outside recorded sandbox")
	}
	e = validEvidence()
	e.Commands[2].Args[6] = filepath.Join(e.Sandbox, "unrelated-repository")
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unrelated init repository")
	}
	e = validEvidence()
	e.Commands[2].Args[8] = e.PackyRef
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unproved init repository ref")
	}
	e = validEvidence()
	e.Commands = e.Commands[:len(e.Commands)-1]
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted missing normalized Claude operations")
	}
	e = validEvidence()
	e.Commands[len(e.Commands)-1].Args[0] = "mcp-list"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted tampered normalized Claude sequence")
	}
	e = validEvidence()
	e.Assertions.NoClaudeMutationOperations = false
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted incomplete explicit-activation assertions")
	}
	e = validEvidence()
	e.Commands[6].ExitCode = 0
	e.Commands[6].Stderr = ""
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted surviving classic install command")
	}
	e = validEvidence()
	e.Commands[7].Stderr = "different failure"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted removed command without unknown-command evidence")
	}
	e = validEvidence()
	e.SchemaVersion = 2
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted superseded lifecycle evidence schema")
	}
	e = validEvidence()
	e.Before = nil
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted missing before manifest")
	}
	e = validEvidence()
	e.After[0].Path = "../outside"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unsafe after manifest path")
	}
	e = validEvidence()
	e.After[0].SHA256 = ""
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted regular file without content digest")
	}
	e = validEvidence()
	e.After[0].Size = -1
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted negative manifest size")
	}
	e = validEvidence()
	e.After[0].Mode = uint32(os.ModeDevice | 0o600)
	e.After[0].SHA256 = ""
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unsupported manifest type")
	}
	e = validEvidence()
	e.PackySHA = strings.Repeat("z", 40)
	e.InstalledSourceSHA = e.PackySHA
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted malformed Packy SHA")
	}
	e = validEvidence()
	e.InstalledSourceSHA = strings.Repeat("c", 40)
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted Installed Source from a different commit")
	}
}

func TestClaudeInterposerRecordsSafeNestedCommands(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "reached")
	real := filepath.Join(root, "real-claude")
	if err := writeStub(real, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+marker+"\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "log")
	wrapper := filepath.Join(root, "claude")
	if err := createClaudeInterposer(wrapper, real, log); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--version"}, {"mcp", "list"}, {"mcp", "get", "engram"}, {"mcp", "add", "engram", "--scope", "user", "--", "engram", "mcp"}, {"mcp", "remove", "engram", "--scope", "user"}} {
		if out, err := exec.CommandContext(context.Background(), wrapper, args...).CombinedOutput(); err != nil {
			t.Fatalf("safe %v: %v: %s", args, err, out)
		}
	}
	got := readClaudeInvocations(log)
	if len(got) != 5 {
		t.Fatalf("nested evidence = %#v", got)
	}
	for _, command := range got {
		if command.Name != "claude" || command.ExitCode != 0 || len(command.Args) != 1 {
			t.Fatalf("unsafe evidence detail: %#v", command)
		}
	}
}

func TestClaudeInterposerBlocksForbiddenShapesBeforeRealBinary(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "reached")
	real := filepath.Join(root, "real-claude")
	if err := writeStub(real, "#!/bin/sh\ntouch "+marker+"\n"); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "claude")
	if err := createClaudeInterposer(wrapper, real, filepath.Join(root, "log")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"--print", "hello"}, {"login"}, {"auth"}, {"model", "opus"}, {"mcp", "add", "x", "--scope", "project", "--", "engram"}, {"mcp", "remove", "x"}, {"mcp", "list", "extra"}} {
		if err := exec.Command(wrapper, args...).Run(); err == nil {
			t.Fatalf("forbidden shape succeeded: %v", args)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("forbidden shape reached real binary: %v", args)
		}
	}
}

func TestPackyTriggeredClaudeInvocationIsRecorded(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	real := filepath.Join(root, "real-claude")
	if err := writeStub(real, "#!/bin/sh\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "claude.log")
	stubBin := filepath.Join(root, "stub-bin")
	if err := os.Mkdir(stubBin, 0700); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(stubBin, "claude")
	if err := createClaudeInterposer(claude, real, log); err != nil {
		t.Fatal(err)
	}
	packy := filepath.Join(root, "packy")
	if err := writeStub(packy, "#!/bin/sh\nclaude mcp list\n"); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + stubBin + ":/usr/bin:/bin"}
	outer := runAllowed(context.Background(), root, root, env, packy, claude, []string{packy, "install"})
	if outer.ExitCode != 0 {
		t.Fatalf("fake Packy failed: %#v", outer)
	}
	nested := readClaudeInvocations(log)
	if len(nested) != 1 || !reflect.DeepEqual(nested[0].Args, []string{"mcp-list"}) || nested[0].ExitCode != 0 {
		t.Fatalf("Packy-triggered evidence = %#v", nested)
	}
}

func TestPrepareInstallableSourceAdaptsFullSHAWithoutMutatingCheckout(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	gitHome, gitConfig := filepath.Join(root, "git-home"), filepath.Join(root, "git-config")
	if err := os.MkdirAll(gitConfig, 0700); err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gitEnv := []string{"HOME=" + gitHome, "XDG_CONFIG_HOME=" + gitConfig, "PATH=" + filepath.Dir(gitExecutable) + ":/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(gitExecutable, args...)
		cmd.Dir = dir
		cmd.Env = gitEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	runGit(source, "init")
	runGit(source, "config", "user.name", "Smoke Test")
	runGit(source, "config", "user.email", "smoke@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "README"), []byte("proved\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(source, "add", "README")
	runGit(source, "commit", "-m", "proved source")
	sha := runGit(source, "rev-parse", "HEAD")
	statusBefore := runGit(source, "status", "--porcelain=v1", "--untracked-files=all")
	if err := os.Mkdir(filepath.Join(root, "work"), 0700); err != nil {
		t.Fatal(err)
	}
	repository, ref, resolved, err := prepareInstallableSource(context.Background(), root, acquisitionEnv(root, "/usr/bin/npm"), source, sha, filepath.Join(root, "installable"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved != sha || ref == sha {
		t.Fatalf("repository=%q ref=%q sha=%q", repository, ref, resolved)
	}
	if got := runGit(repository, "rev-parse", ref+"^{commit}"); got != sha {
		t.Fatalf("synthetic ref = %q, want %q", got, sha)
	}
	if got := runGit(source, "status", "--porcelain=v1", "--untracked-files=all"); got != statusBefore {
		t.Fatalf("source checkout changed: before %q after %q", statusBefore, got)
	}
	if got := runGit(source, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("source HEAD changed to %q", got)
	}
}
