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
	unowned := NewSurfaceAdapter("", l, filepath.Join(home, "s0"), "claude", &recordingRunner{}, OwnershipSnapshot{})
	if err := unowned.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{action}); err == nil {
		t.Fatal("foreign collision overwritten")
	}
	mustFile(t, target, "foreign")
	owned := NewSurfaceAdapter("", l, filepath.Join(home, "s1"), "claude", &recordingRunner{}, NewOwnershipSnapshot(OwnershipRecord{ID: action.ID, Fingerprint: Fingerprint([]byte("foreign")), DeletionAuthorized: true, Contributors: []string{"one"}}))
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
	for name, s := range map[string]OwnershipSnapshot{"shared": NewOwnershipSnapshot(OwnershipRecord{ID: x.ID, Fingerprint: fp, DeletionAuthorized: true, Contributors: []string{"a", "b"}}), "ambiguous": NewOwnershipSnapshot(OwnershipRecord{ID: x.ID, Fingerprint: fp, DeletionAuthorized: true}, OwnershipRecord{ID: x.ID, Fingerprint: fp, DeletionAuthorized: true})} {
		t.Run(name, func(t *testing.T) {
			a := NewSurfaceAdapter("", l, filepath.Join(home, name), "claude", &recordingRunner{}, s)
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
	a := NewSurfaceAdapter("", l, "", "claude", &recordingRunner{}, OwnershipSnapshot{})
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
	a := NewSurfaceAdapter("", l, filepath.Join(home, "state"), "claude", r, NewOwnershipSnapshot(OwnershipRecord{ID: "mcp_server:memory", DeletionAuthorized: true, Contributors: []string{"one"}}))
	identity := NewMCPIdentity("memory", "engram", []string{"mcp"}, map[string]string{})
	add := capabilitypack.ProjectionAction{ID: "mcp_server:memory", Kind: ActionUserMCP, Command: "claude", Args: []string{"mcp", "add", "memory", "--scope", "user", "--", "engram", "mcp"}, Content: canonicalFingerprint(identity)}
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{add}); err != nil {
		t.Fatal(err)
	}
	remove := capabilitypack.ProjectionAction{ID: add.ID, Kind: ActionUserMCP, Command: "claude", Args: []string{"mcp", "remove", "memory", "--scope", "user"}, Mode: capabilitypack.ProjectionDeleteTarget}
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
