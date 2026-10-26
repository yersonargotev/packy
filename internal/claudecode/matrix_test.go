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
	for name, s := range map[string]OwnershipSnapshot{"shared": NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "a", ID: x.ID, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"a", "b"}}), "ambiguous": NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "a", ID: x.ID, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"a"}}, OwnershipRecord{StateOwner: "other", ContributorID: "b", ID: x.ID, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"b"}})} {
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
	bad := []capabilitypack.ProjectionAction{{ID: "skill:x", Kind: ActionSkillLink, Source: outside, Target: filepath.Join(l.AgentsDir, "x")}, {ID: "agent:x", Kind: ActionAgentFile, Target: filepath.Join(l.SkillsDir, "x.md")}, {ID: "instruction:x", Kind: ActionInstructionContribution, Target: outside}}
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
			want := map[string]string{"home": home, "config": filepath.Join(home, ".claude"), "skills": filepath.Join(home, ".claude", "skills"), "agents": filepath.Join(home, ".claude", "agents"), "instructions": filepath.Join(home, ".claude", "CLAUDE.md")}
			got := map[string]string{"home": l.Home, "config": l.ConfigDir, "skills": l.SkillsDir, "agents": l.AgentsDir, "instructions": l.InstructionsFile}
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
	pack := capabilitypack.Pack{ID: "p", Resources: []capabilitypack.Resource{
		{Kind: "agent", ID: "coach", Source: "agents/coach.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "coach"}}},
		{Kind: "instruction", ID: "guide", Source: "instructions/guide.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "instruction", Name: "guide"}}},
	}}
	agentTarget := filepath.Join(l.AgentsDir, "coach.md")
	agentFP, _, _ := localprojection.FingerprintPath(agentTarget)
	ownership := NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "pack:p:coach", ID: "agent:coach", Kind: string(ActionAgentFile), Target: agentTarget, Fingerprint: agentFP, Contributors: []string{"pack:p:coach"}})
	a := NewSurfaceAdapterWithAuthorization(bundle, l, filepath.Join(home, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(ownership), AuthorizationObserverFunc(func(context.Context) AuthorizationObservation {
		return AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true}
	}))
	inspection, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range inspection.Projections {
		if p.ObservedFingerprint != p.DesiredFingerprint {
			t.Errorf("%s observed=%s desired=%s", p.ID, p.ObservedFingerprint, p.DesiredFingerprint)
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
	tests := []struct {
		name    string
		pack    capabilitypack.Pack
		prepare func()
	}{
		{"instructions", instruction, func() {
			os.WriteFile(filepath.Join(bundle, "x.md"), []byte("x"), 0600)
			os.Mkdir(l.InstructionsFile, 0700)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.RemoveAll(l.ConfigDir)
			os.MkdirAll(l.ConfigDir, 0700)
			tc.prepare()
			a := NewSurfaceAdapter(bundle, l, "", "claude", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
			if _, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: tc.pack}); err == nil {
				t.Fatal("classification error was treated as empty")
			}
		})
	}
}

func TestAuthorizationKnownNegativeAndUnknownSemantics(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	supported := &recordingRunner{result: Result{Stdout: "2.1.203"}}
	knownNegative := NewSurfaceAdapterWithAuthorization("", l, "", "claude", supported, StaticOwnershipSnapshot(OwnershipSnapshot{}), AuthorizationObserverFunc(func(context.Context) AuthorizationObservation {
		return AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true, Disabled: true}
	}))
	inspection, err := knownNegative.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: capabilitypack.Pack{ID: "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Readiness.AuthorizationObserved || inspection.Readiness.Authorized {
		t.Fatalf("known negative=%+v", inspection.Readiness)
	}
	unsupported := NewSurfaceAdapterWithAuthorization("", l, "", "", supported, StaticOwnershipSnapshot(OwnershipSnapshot{}), AuthorizationObserverFunc(func(context.Context) AuthorizationObservation {
		return AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true}
	}))
	inspection, err = unsupported.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: capabilitypack.Pack{ID: "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Readiness.AuthorizationObserved || inspection.Readiness.Authorized {
		t.Fatalf("known unsupported=%+v", inspection.Readiness)
	}
	unknown := NewSurfaceAdapterWithAuthorization("", l, "", "claude", supported, StaticOwnershipSnapshot(OwnershipSnapshot{}), AuthorizationObserverFunc(func(context.Context) AuthorizationObservation { return AuthorizationObservation{PolicyObserved: true} }))
	inspection, err = unknown.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: capabilitypack.Pack{ID: "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Readiness.AuthorizationObserved || inspection.Readiness.Authorized {
		t.Fatalf("unknown=%+v", inspection.Readiness)
	}
}

func TestPresentOwnedInstructionApplyCleanupStaleAndRepeat(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	os.MkdirAll(l.ConfigDir, 0700)
	doc, _ := UpsertInstructionContribution("foreign\n", InstructionContribution{ContributorID: "pack:p:i", Content: "owned"})
	os.WriteFile(l.InstructionsFile, []byte(doc), 0600)
	record := OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "pack:p:i", ID: "instruction:i", Kind: string(ActionInstructionContribution), Target: l.InstructionsFile, Fingerprint: Fingerprint([]byte("owned")), Contributors: []string{"pack:p:i"}}
	a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(record)))
	removed, _ := RemoveInstructionContribution(doc, "pack:p:i")
	stale := capabilitypack.ProjectionAction{ID: record.ID, Kind: ActionInstructionContribution, Target: l.InstructionsFile, Content: removed, Command: Fingerprint([]byte("stale")), Mode: capabilitypack.ProjectionRemoveContent}
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{stale}); err == nil {
		t.Fatal("stale instruction cleanup accepted")
	}
	mustFile(t, l.InstructionsFile, doc)
	stale.Command = Fingerprint([]byte(doc))
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{stale}); err != nil {
		t.Fatal(err)
	}
	mustFile(t, l.InstructionsFile, removed)
	stale.Command = Fingerprint([]byte(removed))
	stale.Content = removed
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{stale}); err != nil {
		t.Fatal("repeat", err)
	}
}

func TestSkillInstallUpdateDriftAndRemovalLifecycle(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	src := filepath.Join(home, "src")
	os.MkdirAll(src, 0700)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("v1"), 0600)
	target := filepath.Join(l.SkillsDir, "skill")
	snapshot := OwnershipSnapshot{}
	provider := OwnershipSnapshotFunc(func(context.Context) (OwnershipSnapshot, error) { return snapshot, nil })
	a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", &recordingRunner{}, provider)
	action := capabilitypack.ProjectionAction{ID: "skill:skill", Kind: ActionSkillLink, Source: src, Target: target}
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
		t.Fatal(err)
	}
	fp, _, _ := localprojection.FingerprintPath(target)
	resolvedSrc, _ := filepath.EvalSymlinks(src)
	snapshot = NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "pack:p:s", ID: action.ID, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, Skill: SkillIdentity{Surface: "claude", ProjectionID: action.ID, Path: target, SymlinkType: "directory", ResolvedTarget: resolvedSrc, ExpectedSource: resolvedSrc, SourceTreeFingerprint: fp}, DeletionAuthorized: true, Contributors: []string{"pack:p:s"}})
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
		t.Fatal("idempotent install", err)
	}
	foreign := filepath.Join(home, "foreign")
	os.Mkdir(foreign, 0700)
	os.Remove(target)
	os.Symlink(foreign, target)
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err == nil {
		t.Fatal("drifted skill update accepted")
	}
	os.Remove(target)
	os.Symlink(src, target)
	remove := action
	remove.Source = ""
	remove.Mode = capabilitypack.ProjectionDeleteTarget
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err != nil {
		t.Fatal("repeat removal", err)
	}
}

func TestAgentInstallUpdateStaleAndRepeatedRemovalLifecycle(t *testing.T) {
	home := t.TempDir()
	l := NewCanonicalLayout(home)
	target := filepath.Join(l.AgentsDir, "agent.md")
	snapshot := OwnershipSnapshot{}
	provider := OwnershipSnapshotFunc(func(context.Context) (OwnershipSnapshot, error) { return snapshot, nil })
	a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", &recordingRunner{}, provider)
	action := capabilitypack.ProjectionAction{ID: "agent:agent", Kind: ActionAgentFile, Target: target, Content: "v1", Command: Fingerprint(nil)}
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
		t.Fatal(err)
	}
	fp, _, _ := localprojection.FingerprintPath(target)
	snapshot = NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "pack:p:a", ID: action.ID, Kind: string(ActionAgentFile), Target: target, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"pack:p:a"}})
	action.Content = "v2"
	action.Command = Fingerprint([]byte("v1"))
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err != nil {
		t.Fatal(err)
	}
	mustFile(t, target, "v2")
	os.WriteFile(target, []byte("foreign"), 0600)
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err == nil {
		t.Fatal("stale agent update accepted")
	}
	os.WriteFile(target, []byte("v2"), 0600)
	snapshot = NewOwnershipSnapshot(OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "pack:p:a", ID: action.ID, Kind: string(ActionAgentFile), Target: target, Fingerprint: localprojection.FingerprintBytes([]byte("v2")), DeletionAuthorized: true, Contributors: []string{"pack:p:a"}})
	remove := action
	remove.Mode = capabilitypack.ProjectionDeleteTarget
	remove.Command = Fingerprint([]byte("v2"))
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{remove}); err != nil {
		t.Fatal("repeat", err)
	}
}

func TestSkillSameBytesWrongPathTypesNeverTransferOwnership(t *testing.T) {
	t.Run("directory collision", func(t *testing.T) {
		home := t.TempDir()
		l := NewCanonicalLayout(home)
		source := filepath.Join(home, "source")
		target := filepath.Join(l.SkillsDir, "skill")
		os.MkdirAll(source, 0700)
		os.MkdirAll(target, 0700)
		os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("same"), 0600)
		os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("same"), 0600)
		a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
		action := capabilitypack.ProjectionAction{ID: "skill:skill", Kind: ActionSkillLink, Source: source, Target: target}
		if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err == nil {
			t.Fatal("same-bytes directory accepted as owned symlink")
		}
		info, _ := os.Lstat(target)
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatal("directory collision was mutated")
		}
	})
	t.Run("foreign retarget", func(t *testing.T) {
		home := t.TempDir()
		l := NewCanonicalLayout(home)
		expected := filepath.Join(home, "expected")
		foreign := filepath.Join(home, "foreign")
		for _, p := range []string{expected, foreign} {
			os.MkdirAll(p, 0700)
			os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("same"), 0600)
		}
		target := filepath.Join(l.SkillsDir, "skill")
		os.MkdirAll(l.SkillsDir, 0700)
		os.Symlink(foreign, target)
		fp, _ := localprojection.FingerprintTree(expected)
		resolvedExpected, _ := filepath.EvalSymlinks(expected)
		record := OwnershipRecord{StateOwner: "capabilitypack", ContributorID: "pack:p:s", ID: "skill:skill", Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, Skill: SkillIdentity{Surface: "claude", ProjectionID: "skill:skill", Path: target, SymlinkType: "directory", ResolvedTarget: resolvedExpected, ExpectedSource: resolvedExpected, SourceTreeFingerprint: fp}, Contributors: []string{"pack:p:s"}}
		a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(record)))
		action := capabilitypack.ProjectionAction{ID: record.ID, Kind: ActionSkillLink, Source: expected, Target: target}
		if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err == nil {
			t.Fatal("same-bytes foreign retarget accepted")
		}
		resolved, _ := filepath.EvalSymlinks(target)
		resolvedForeign, _ := filepath.EvalSymlinks(foreign)
		if resolved != resolvedForeign {
			t.Fatalf("foreign symlink changed to %s", resolved)
		}
	})
}

func TestSurfaceAdapterAggregatesMultipleInstructionContributionsIntoOneSealedDocument(t *testing.T) {
	home := t.TempDir()
	bundle := filepath.Join(home, "bundle")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"one.md": "one", "two.md": "two"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bindings := func(name string) []capabilitypack.Binding {
		return []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "instruction", Name: name}}
	}
	pack := capabilitypack.Pack{ID: "p", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "one", Source: "one.md", Bindings: bindings("one")},
		{Kind: "instruction", ID: "two", Source: "two.md", Bindings: bindings("two")},
	}}
	layout := NewCanonicalLayout(home)
	a := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state"), "", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
	inspection, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 2 || inspection.Projections[0].Action.Content != inspection.Projections[1].Action.Content {
		t.Fatalf("instruction actions were not sealed to one shared document: %#v", inspection.Projections)
	}
	content := inspection.Projections[0].Action.Content
	if !strings.Contains(content, "pack:p:one") || !strings.Contains(content, "pack:p:two") {
		t.Fatalf("sealed document omitted a contribution:\n%s", content)
	}
	if actionErr := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{inspection.Projections[0].Action, inspection.Projections[1].Action}); actionErr != nil {
		t.Fatal(actionErr)
	}
	if got, err := os.ReadFile(layout.InstructionsFile); err != nil || string(got) != content {
		t.Fatalf("shared document write: err=%v\n%s", err, got)
	}
	removal, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: pack, Desired: capabilitypack.Pack{ID: "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(removal.Projections) != 2 || removal.Projections[0].Action.Content != removal.Projections[1].Action.Content {
		t.Fatalf("instruction removals were not sealed to one shared document: %#v", removal.Projections)
	}
	if removed := removal.Projections[0].Action.Content; strings.Contains(removed, "pack:p:one") || strings.Contains(removed, "pack:p:two") {
		t.Fatalf("sealed removal retained a contribution:\n%s", removed)
	}
}
