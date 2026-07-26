package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/codexsmoke"
	"github.com/yersonargotev/packy/internal/opencodesmoke"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

func TestRunRejectsIncompleteIdentityBeforeReadingArtifacts(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("empty invocation passed")
	}
	if err := run([]string{"--candidate-sha", strings.Repeat("a", 40), "--run-id", "run", "--collected-at", "not-a-time", "--output", filepath.Join(t.TempDir(), "out.json")}); err == nil {
		t.Fatal("invalid collection time passed")
	}
}

func TestDecodeEvidenceRejectsUnknownAndTrailingJSON(t *testing.T) {
	root := t.TempDir()
	unknown := filepath.Join(root, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"schema_version":1,"unknown":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeEvidence[codexsmoke.Evidence](unknown); err == nil {
		t.Fatal("unknown field passed")
	}
	trailing := filepath.Join(root, "trailing.json")
	if err := os.WriteFile(trailing, []byte(`{} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeEvidence[codexsmoke.Evidence](trailing); err == nil {
		t.Fatal("trailing JSON passed")
	}
}

func TestNormalizeCodexBindsExactCandidateInventoryAndSafety(t *testing.T) {
	candidate := strings.Repeat("a", 40)
	raw := codexsmoke.Evidence{
		SchemaVersion: 1, PackyRef: candidate, PackySHA: candidate,
		VercelFixtureSHA256: vercelacceptance.ExactArchiveSHA256,
		CodexVersion:        "codex-cli " + vercelacceptance.ExactCodexVersion,
		CodexNPMIntegrity:   "sha512-exact", CodexExecutableSHA256: strings.Repeat("b", 64),
		SandboxRoots:           []string{"$SANDBOX/home", "$SANDBOX/bundle", "$SANDBOX/work"},
		MissingOneNegativeTwin: "deploy-to-vercel",
		NoAuthentication:       true, NoModelInvocation: true, NoDeploy: true, NoUpstreamExecution: true,
	}
	for _, resource := range vercelacceptance.Canonical().Pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		var name, invocation string
		for _, binding := range resource.Bindings {
			if binding.Surface == "codex" {
				name, invocation = binding.Name, binding.Invocation
			}
		}
		raw.Skills = append(raw.Skills, codexsmoke.SkillEvidence{
			Name: name, SHA256: strings.Repeat("c", 64), Invocation: invocation,
			Enabled: true, InvocationAvailable: true,
		})
		for _, mode := range resource.RuntimeModes {
			raw.RuntimeModes = append(raw.RuntimeModes, codexsmoke.RuntimeModeEvidence{
				ResourceID: resource.ID, ModeID: mode.ID, Invocation: invocation + " " + mode.ID,
				SelectionObserved: true, FailBeforeEffects: true,
			})
		}
	}
	got, err := normalizeCodex(candidate, time.Now().UTC(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "codex" || len(got.Skills) != 9 || len(got.RuntimeModes) != 28 || got.EvidenceFingerprint == "" {
		t.Fatalf("normalized Codex evidence = %#v", got)
	}
	raw.NoDeploy = false
	if _, err := normalizeCodex(candidate, time.Now().UTC(), raw); err == nil {
		t.Fatal("unsafe Codex evidence passed")
	}
}

func TestNormalizeOpenCodeBindsExactCandidateInventoryAndSafety(t *testing.T) {
	candidate := strings.Repeat("a", 40)
	raw := opencodesmoke.Evidence{
		SchemaVersion: 2, PackyRef: candidate, PackySHA: candidate,
		VercelFixtureSHA256:   vercelacceptance.ExactArchiveSHA256,
		OpenCodeVersion:       vercelacceptance.ExactOpenCodeVersion,
		OpenCodeArchiveSHA256: strings.Repeat("b", 64), OpenCodeExecutableSHA256: strings.Repeat("c", 64),
		SandboxRoots:           []string{"home", "xdg", "data", "cache", "state", "bundle", "work"},
		MissingOneNegativeTwin: "deploy-to-vercel",
		NoAuthentication:       true, NoExternalModelNetwork: true, NoDeploy: true,
		NativeSkillToolObserved: true, NoUpstreamEffects: true,
	}
	for _, resource := range vercelacceptance.Canonical().Pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		var name, invocation string
		for _, binding := range resource.Bindings {
			if binding.Surface == "opencode" {
				name, invocation = binding.Name, binding.Invocation
			}
		}
		raw.Skills = append(raw.Skills, opencodesmoke.SkillEvidence{
			Name: name, SHA256: strings.Repeat("d", 64), ContentLoaded: true,
		})
		for _, mode := range resource.RuntimeModes {
			raw.RuntimeModes = append(raw.RuntimeModes, opencodesmoke.RuntimeModeEvidence{
				ResourceID: resource.ID, ModeID: mode.ID, Invocation: invocation + " " + mode.ID,
				SelectionObserved: true, InvocationAvailable: true, FailBeforeHostEffects: true,
			})
		}
	}
	got, err := normalizeOpenCode(candidate, time.Now().UTC(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "opencode" || len(got.Skills) != 9 || len(got.RuntimeModes) != 28 || got.EvidenceFingerprint == "" {
		t.Fatalf("normalized OpenCode evidence = %#v", got)
	}
	raw.NativeSkillToolObserved = false
	if _, err := normalizeOpenCode(candidate, time.Now().UTC(), raw); err == nil {
		t.Fatal("non-native OpenCode evidence passed")
	}
}
