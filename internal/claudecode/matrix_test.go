package claudecode

import (
	"context"
	"errors"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionRunnerFailureMatrix(t *testing.T) {
	cases := []struct {
		o    VersionObservation
		want Compatibility
	}{{ObserveVersion(context.Background(), "", &recordingRunner{}), CompatibilityMissing}, {ObserveVersion(context.Background(), "claude", &recordingRunner{result: Result{TimedOut: true, Err: context.DeadlineExceeded}}), CompatibilityTimedOut}, {ObserveVersion(context.Background(), "claude", &recordingRunner{result: Result{Err: errors.New("boom")}}), CompatibilityFailed}}
	for _, tc := range cases {
		if got := ClassifyVersion(tc.o); got != tc.want {
			t.Errorf("%+v=%s want %s", tc.o, got, tc.want)
		}
	}
}

func TestInstructionObservationExactContentsAndInvalidMarkers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CLAUDE.md")
	os.WriteFile(p, []byte(instructionStart+"\n<!-- contributor:a -->\n  exact body  \n<!-- /contributor:a -->\n"+instructionEnd), 0600)
	o := ObserveInstructions(p)
	if o.Err != nil || o.Contributions["a"] != Fingerprint([]byte("exact body")) {
		t.Fatalf("%+v", o)
	}
	os.WriteFile(p, []byte(instructionStart+instructionStart+instructionEnd), 0600)
	if ObserveInstructions(p).Err == nil {
		t.Fatal("duplicate markers accepted")
	}
}

func TestHookObservationPolicyAndMalformedJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	h := CommandHookEntry{Type: "command", Event: "SessionStart", Command: "engram", Args: []string{"session"}, TimeoutSeconds: 5, Blocking: true, Failure: "block", Authorities: []string{}}
	b, _ := MergeCommandHook(nil, h, false)
	os.WriteFile(p, b, 0600)
	o := ObserveHooks(p, h, []byte(`{"disableAllHooks":true}`))
	if o.Err != nil || len(o.MatchingEntries) != 1 || !o.Disabled || !o.Shadowed {
		t.Fatalf("%+v", o)
	}
	os.WriteFile(p, []byte("{"), 0600)
	if ObserveHooks(p, h, nil).Err == nil {
		t.Fatal("malformed JSON accepted")
	}
}

func TestOwnershipCollisionDriftCleanupAndIdempotence(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	os.MkdirAll(l.AgentsDir, 0700)
	target := filepath.Join(l.AgentsDir, "coach.md")
	os.WriteFile(target, []byte("foreign"), 0600)
	action := capabilitypack.ProjectionAction{ID: "agent:coach", Kind: ActionAgentFile, Target: target, Content: "ours", Command: Fingerprint([]byte("foreign"))}
	unowned := NewSurfaceAdapter("", l, filepath.Join(home, "s0"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
	if err := unowned.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err == nil {
		t.Fatal("foreign collision overwritten")
	}
	mustFile(t, target, "foreign")
	owned := NewSurfaceAdapter("", l, filepath.Join(home, "s1"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "one", ID: action.ID, Kind: string(ActionAgentFile), Target: target, Fingerprint: Fingerprint([]byte("foreign")), DeletionAuthorized: true, Contributors: []string{"one"}})))
	remove := action
	remove.Mode = capabilitypack.ProjectionDeleteTarget
	os.WriteFile(target, []byte("changed"), 0600)
	if err := owned.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err == nil {
		t.Fatal("drifted target deleted")
	}
	mustFile(t, target, "changed")
	os.WriteFile(target, []byte("foreign"), 0600)
	if err := owned.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err != nil {
		t.Fatal(err)
	}
	if err := owned.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err != nil {
		t.Fatal("cleanup not idempotent", err)
	}
}

func TestLastContributorAndAmbiguousOwnershipPreserve(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	os.MkdirAll(l.SkillsDir, 0700)
	src := filepath.Join(home, "src")
	os.Mkdir(src, 0700)
	target := filepath.Join(l.SkillsDir, "x")
	os.Symlink(src, target)
	fp, _, _ := localprojection.FingerprintPath(target)
	x := capabilitypack.ProjectionAction{ID: "skill:x", Kind: ActionSkillLink, Target: target, Mode: capabilitypack.ProjectionDeleteTarget}
	for name, s := range map[string]OwnershipSnapshot{"shared": NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "a", ID: x.ID, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"a", "b"}}), "ambiguous": NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "a", ID: x.ID, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"a"}}, OwnershipRecord{StateOwner: "classic", ContributorID: "b", ID: x.ID, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"b"}})} {
		t.Run(name, func(t *testing.T) {
			a := NewSurfaceAdapter("", l, filepath.Join(home, name), "claude", &recordingRunner{}, StaticOwnershipSnapshot(s))
			if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{x}); err == nil {
				t.Fatal("unsafe deletion accepted")
			}
			if _, err := os.Lstat(target); err != nil {
				t.Fatal("target not preserved")
			}
		})
	}
}

func TestKindSpecificTargetsAndSharedDocuments(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	a := NewSurfaceAdapter("", l, "", "claude", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
	outside := filepath.Join(home, "elsewhere")
	bad := []capabilitypack.ProjectionAction{{ID: "skill:x", Kind: ActionSkillLink, Source: outside, Target: filepath.Join(l.AgentsDir, "x")}, {ID: "agent:x", Kind: ActionAgentFile, Target: filepath.Join(l.SkillsDir, "x.md")}, {ID: "instruction:x", Kind: ActionInstructionContribution, Target: outside}, {ID: "hook:x", Kind: ActionCommandHook, Target: outside}}
	for _, x := range bad {
		if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{x}); err == nil {
			t.Fatalf("invalid target accepted: %+v", x)
		}
	}
	shared := []capabilitypack.ProjectionAction{{ID: "instruction:a", Kind: ActionInstructionContribution, Target: l.InstructionsFile, Content: "one"}, {ID: "instruction:b", Kind: ActionInstructionContribution, Target: l.InstructionsFile, Content: "two"}}
	if err := a.ApplyProjections(context.Background(), shared); err == nil || !strings.Contains(err.Error(), "aggregated") {
		t.Fatalf("unaggregated shared actions=%v", err)
	}
}

type mcpStoreRunner struct {
	path       string
	definition []byte
	calls      []Command
}

func (r *mcpStoreRunner) Run(_ context.Context, c Command) Result {
	r.calls = append(r.calls, c)
	_, remove := mcpActionName(c.Args)
	if remove {
		os.WriteFile(r.path, []byte(`{"mcpServers":{}}`), 0600)
	} else {
		os.WriteFile(r.path, r.definition, 0600)
	}
	return Result{}
}
func TestMCPAddRemoveStaticVerification(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	def := []byte(`{"mcpServers":{"memory":{"command":"engram","args":["mcp"],"env":{}}}}`)
	r := &mcpStoreRunner{path: l.UserMCPFile, definition: def}
	identity := NewMCPIdentity("memory", "engram", []string{"mcp"}, map[string]string{})
	a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", r, StaticOwnershipSnapshot(NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "one", ID: "mcp_server:memory", Kind: string(ActionUserMCP), Target: "memory", Fingerprint: canonicalFingerprint(identity), DeletionAuthorized: true, Contributors: []string{"one"}})))
	add := capabilitypack.ProjectionAction{ID: "mcp_server:memory", Kind: ActionUserMCP, Command: "claude", Args: []string{"mcp", "add", "memory", "--scope", "user", "--", "engram", "mcp"}, Content: canonicalFingerprint(identity)}
	add.Target = "memory"
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{add}); err != nil {
		t.Fatal(err)
	}
	remove := capabilitypack.ProjectionAction{ID: add.ID, Kind: ActionUserMCP, Command: "claude", Args: []string{"mcp", "remove", "memory", "--scope", "user"}, Mode: capabilitypack.ProjectionDeleteTarget}
	remove.Target = "memory"
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 {
		t.Fatal("unexpected effect count")
	}
}

func mustFile(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || string(b) != want {
		t.Fatalf("file=%q err=%v want=%q", b, err, want)
	}
}

func TestCanonicalLayoutUsesOnlyInjectedHome(t *testing.T) {
	for _, home := range []string{filepath.Join(t.TempDir(), "home-a"), filepath.Join(t.TempDir(), "nested", "home-b")} {
		t.Run(filepath.Base(home), func(t *testing.T) {
			l := NewCanonicalLayout(home)
			want := map[string]string{"home": home, "config": filepath.Join(home, ".claude"), "skills": filepath.Join(home, ".claude", "skills"), "agents": filepath.Join(home, ".claude", "agents"), "instructions": filepath.Join(home, ".claude", "CLAUDE.md"), "settings": filepath.Join(home, ".claude", "settings.json"), "mcp": filepath.Join(home, ".claude.json")}
			got := map[string]string{"home": l.Home, "config": l.ConfigDir, "skills": l.SkillsDir, "agents": l.AgentsDir, "instructions": l.InstructionsFile, "settings": l.SettingsFile, "mcp": l.UserMCPFile}
			for k, v := range want {
				if got[k] != v {
					t.Errorf("%s=%q want %q", k, got[k], v)
				}
				if !strings.HasPrefix(got[k], home) {
					t.Errorf("%s escaped injected sandbox: %q", k, got[k])
				}
			}
		})
	}
}

type lockCheckingProvider struct {
	lock     string
	snapshot OwnershipSnapshot
	calls    int
}

func (p *lockCheckingProvider) ObserveOwnership(context.Context) (OwnershipSnapshot, error) {
	p.calls++
	if _, err := os.Stat(p.lock); err != nil {
		return OwnershipSnapshot{}, errors.New("ownership observed outside host-effect lock")
	}
	return p.snapshot, nil
}
func TestBatchPreflightUsesFreshOwnershipUnderLockAndMakesNoPartialEffect(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	os.MkdirAll(l.ConfigDir, 0700)
	os.WriteFile(l.InstructionsFile, []byte("changed"), 0600)
	state := filepath.Join(home, "state")
	p := &lockCheckingProvider{lock: filepath.Join(state, "claude-host-effect.lock")}
	a := NewSurfaceAdapter("", l, state, "claude", &recordingRunner{}, p)
	agent := filepath.Join(l.AgentsDir, "first.md")
	actions := []capabilitypack.ProjectionAction{{ID: "agent:first", Kind: ActionAgentFile, Target: agent, Content: "would mutate", Command: Fingerprint(nil)}, {ID: "instruction:stale", Kind: ActionInstructionContribution, Target: l.InstructionsFile, Content: "replacement", Command: Fingerprint([]byte("prior"))}}
	err := a.ApplyProjections(context.Background(), actions)
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("batch error=%v", err)
	}
	if p.calls != 1 {
		t.Fatalf("ownership observations=%d want 1", p.calls)
	}
	if _, err := os.Stat(agent); !os.IsNotExist(err) {
		t.Fatal("earlier batch effect executed before later stale detection")
	}
	mustFile(t, l.InstructionsFile, "changed")
}

func TestSurfaceInspectionUsesExactProjectionIdentityDomainsAndObservedAuthorization(t *testing.T) {
	home := t.TempDir()
	bundle := filepath.Join(home, "bundle")
	os.MkdirAll(filepath.Join(bundle, "agents"), 0700)
	os.MkdirAll(filepath.Join(bundle, "instructions"), 0700)
	os.WriteFile(filepath.Join(bundle, "agents", "coach.md"), []byte("coach"), 0600)
	os.WriteFile(filepath.Join(bundle, "instructions", "guide.md"), []byte("guide"), 0600)
	l := NewCanonicalLayout(filepath.Join(home, "sandbox-home"))
	os.MkdirAll(l.AgentsDir, 0700)
	os.WriteFile(filepath.Join(l.AgentsDir, "coach.md"), []byte("coach"), 0600)
	doc, _ := UpsertInstructionContribution("", InstructionContribution{ContributorID: "pack:p:guide", Content: "guide"})
	os.WriteFile(l.InstructionsFile, []byte(doc), 0600)
	hook := CommandHookEntry{Type: "command", Event: "SessionStart", Command: "engram", Args: []string{"session"}, TimeoutSeconds: 5, Blocking: true, Failure: "block", Authorities: []string{}}
	settings, _ := MergeCommandHook(nil, hook, false)
	os.WriteFile(l.SettingsFile, settings, 0600)
	if direct := ObserveHooks(l.SettingsFile, hook, nil); len(direct.MatchingEntries) != 1 {
		t.Fatalf("direct hook=%+v want=%s", direct, canonicalFingerprint(hookJSON(hook)))
	}
	pack := capabilitypack.Pack{ID: "p", Resources: []capabilitypack.Resource{
		{Kind: "agent", ID: "coach", Source: "agents/coach.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "coach"}}},
		{Kind: "instruction", ID: "guide", Source: "instructions/guide.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "instruction", Name: "guide"}}},
		{Kind: "lifecycle", ID: "session", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "command_hook", Name: "session", Hook: &capabilitypack.CommandHook{Type: "command", Event: "SessionStart", Command: "engram", Args: []string{"session"}, TimeoutSeconds: 5, Blocking: true, Failure: "block", Authorities: []string{}}}}},
	}}
	a := NewSurfaceAdapter(bundle, l, filepath.Join(home, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
	inspection, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range inspection.Projections {
		if p.ObservedFingerprint != p.DesiredFingerprint {
			t.Errorf("%s observed=%s desired=%s settings=%s", p.ID, p.ObservedFingerprint, p.DesiredFingerprint, settings)
		}
	}
	if !inspection.Readiness.AuthorizationObserved || !inspection.Readiness.Authorized {
		t.Fatalf("authorization=%+v", inspection.Readiness)
	}
}

func TestSurfaceInspectionPropagatesSharedStoreClassificationErrors(t *testing.T) {
	home := t.TempDir()
	bundle := filepath.Join(home, "bundle")
	os.MkdirAll(bundle, 0700)
	l := NewCanonicalLayout(filepath.Join(home, "sandbox"))
	binding := func(projection, name string) []capabilitypack.Binding {
		return []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: projection, Name: name}}
	}
	instruction := capabilitypack.Pack{ID: "p", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "x", Source: "x.md", Bindings: binding("instruction", "x")}}}
	hookBinding := binding("command_hook", "x")
	hookBinding[0].Hook = &capabilitypack.CommandHook{Type: "command", Event: "SessionStart", Command: "x", TimeoutSeconds: 1, Blocking: true, Failure: "block", Args: []string{}, Authorities: []string{}}
	settings := capabilitypack.Pack{ID: "p", Resources: []capabilitypack.Resource{{Kind: "lifecycle", ID: "x", Bindings: hookBinding}}}
	mcp := capabilitypack.Pack{ID: "p", Resources: []capabilitypack.Resource{{Kind: "mcp_server", ID: "x", Command: "x", Args: []string{}, Bindings: binding("mcp_server", "x")}}}
	tests := []struct {
		name    string
		pack    capabilitypack.Pack
		prepare func()
	}{
		{"instructions", instruction, func() {
			os.WriteFile(filepath.Join(bundle, "x.md"), []byte("x"), 0600)
			os.Mkdir(l.InstructionsFile, 0700)
		}},
		{"settings", settings, func() { os.WriteFile(l.SettingsFile, []byte("{"), 0600) }},
		{"mcp", mcp, func() { os.WriteFile(l.UserMCPFile, []byte(`{"mcpServers":[]}`), 0600) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.RemoveAll(l.ConfigDir)
			os.Remove(l.UserMCPFile)
			os.MkdirAll(l.ConfigDir, 0700)
			tc.prepare()
			a := NewSurfaceAdapter(bundle, l, "", "claude", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
			if _, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: tc.pack}); err == nil {
				t.Fatal("classification error was treated as empty")
			}
		})
	}
}

func TestExactCleanupMatrixForSkillInstructionHookAndMCP(t *testing.T) {
	t.Run("skill exact last contributor", func(t *testing.T) {
		home := t.TempDir()
		l := NewCanonicalLayout(home)
		os.MkdirAll(l.SkillsDir, 0700)
		src := filepath.Join(home, "src")
		os.Mkdir(src, 0700)
		target := filepath.Join(l.SkillsDir, "x")
		os.Symlink(src, target)
		fp, _, _ := localprojection.FingerprintPath(target)
		record := OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "pack:p:x", ID: "skill:x", Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"pack:p:x"}}
		a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(record)))
		action := capabilitypack.ProjectionAction{ID: record.ID, Kind: ActionSkillLink, Target: target, Mode: capabilitypack.ProjectionDeleteTarget}
		if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
			t.Fatal(err)
		}
		if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
			t.Fatal("repeat", err)
		}
	})
	t.Run("instruction contribution", func(t *testing.T) {
		doc := "foreign\n" + instructionStart + "\n<!-- contributor:classic -->\nkeep\n<!-- /contributor:classic -->\n<!-- contributor:pack:p:x -->\nremove\n<!-- /contributor:pack:p:x -->\n" + instructionEnd + "\ntail\n"
		got, err := RemoveInstructionContribution(doc, "pack:p:x")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "classic -->\nkeep") || !strings.HasPrefix(got, "foreign\n") || !strings.HasSuffix(got, "tail\n") || strings.Contains(got, "\nremove\n") {
			t.Fatalf("preservation failed:\n%s", got)
		}
		again, err := RemoveInstructionContribution(got, "pack:p:x")
		if err != nil || again != got {
			t.Fatal("not idempotent", err)
		}
	})
	t.Run("hook exact entry", func(t *testing.T) {
		hook := CommandHookEntry{Type: "command", Event: "SessionStart", Command: "x", Args: []string{}, TimeoutSeconds: 1, Blocking: true, Failure: "block", Authorities: []string{}}
		foreign := CommandHookEntry{Type: "command", Event: "SessionStart", Command: "foreign", Args: []string{}, TimeoutSeconds: 1, Blocking: true, Failure: "warn", Authorities: []string{}}
		settings, _ := MergeCommandHook(nil, foreign, false)
		settings, _ = MergeCommandHook(settings, hook, false)
		got, err := MergeCommandHook(settings, hook, true)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(got), `"command": "x"`) || !strings.Contains(string(got), `"command": "foreign"`) {
			t.Fatalf("hook cleanup=%s", got)
		}
		again, err := MergeCommandHook(got, hook, true)
		if err != nil || string(again) != string(got) {
			t.Fatal("not idempotent", err)
		}
	})
	t.Run("foreign MCP collision", func(t *testing.T) {
		home := t.TempDir()
		l := NewCanonicalLayout(home)
		os.WriteFile(l.UserMCPFile, []byte(`{"mcpServers":{"memory":{"command":"foreign","args":[],"env":{}}}}`), 0600)
		r := &recordingRunner{}
		a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", r, StaticOwnershipSnapshot(OwnershipSnapshot{}))
		x := capabilitypack.ProjectionAction{ID: "mcp_server:memory", Kind: ActionUserMCP, Target: "memory", Command: "claude", Args: []string{"mcp", "add", "memory", "--scope", "user", "--", "ours"}}
		if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{x}); err == nil {
			t.Fatal("foreign MCP overwritten")
		}
		if len(r.calls) != 0 {
			t.Fatal("MCP effect ran after collision")
		}
	})
}
