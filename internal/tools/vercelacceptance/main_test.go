package main

import (
	"fmt"
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

func TestLoadFoundationRequiresEveryExactDeterministicOwningProof(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	candidate := strings.Repeat("a", 40)
	observedAt := now.Add(-time.Minute)
	var manifestRows []vercelacceptance.FoundationDigestRow
	for _, row := range vercelacceptance.FoundationRows() {
		var rowDigests []string
		for _, proof := range row.FoundationProofs() {
			seam := proof.Seam
			test := seam[strings.LastIndex(seam, "/")+1:]
			output := []byte(fmt.Sprintf("@identity\t%s\trun-1\t%s\t%s\n=== RUN   %s\n--- PASS: %s (duration)\n",
				candidate, observedAt.Format(time.RFC3339), seam, test, test))
			rowDigests = append(rowDigests, digestBytes(output))
			for _, rerun := range []string{"first", "second"} {
				name, err := vercelacceptance.FoundationProofFilename(row.ID, proof.Kind, rerun)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, name), output, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		manifestRows = append(manifestRows, vercelacceptance.FoundationDigestRow{
			RowID: row.ID, Digests: [3]string{rowDigests[0], rowDigests[1], rowDigests[2]},
		})
	}
	manifest, err := vercelacceptance.CanonicalFoundationManifest(vercelacceptance.FoundationContext{
		CandidateSHA: candidate, RunID: "run-1", ObservedAt: observedAt, Now: now, MaxAge: 15 * time.Minute,
	}, manifestRows)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.tsv"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	evidence, err := loadFoundation(root, candidate, "run-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 21 {
		t.Fatalf("foundation rows = %d, want 21", len(evidence))
	}
	if err := os.WriteFile(filepath.Join(root, "VERCEL-ACCEPTANCE-01.positive.second.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFoundation(root, candidate, "run-1", now); err == nil {
		t.Fatal("changed rerun passed")
	}
}

func TestNormalizeCodexBindsExactCandidateInventoryAndSafety(t *testing.T) {
	candidate := strings.Repeat("a", 40)
	raw := codexsmoke.Evidence{
		SchemaVersion: 1, PackyRef: candidate, PackySHA: candidate,
		RunID: "run-1", ObservedAt: time.Now().UTC(),
		VercelFixtureSHA256: vercelacceptance.ExactArchiveSHA256,
		CodexVersion:        "codex-cli " + vercelacceptance.ExactCodexVersion,
		CodexNPMIntegrity:   "sha512-exact", CodexExecutableSHA256: strings.Repeat("b", 64),
		SandboxRoots:           []string{"$SANDBOX/home", "$SANDBOX/bundle", "$SANDBOX/work"},
		SemanticRerun:          vercelacceptance.SemanticRerunEvidence{FirstSHA256: strings.Repeat("d", 64), SecondSHA256: strings.Repeat("d", 64), ExactMatch: true},
		Mutation:               vercelacceptance.MutationObservation{Root: "$SANDBOX/bundle", BeforeSHA256: strings.Repeat("e", 64), AfterSHA256: strings.Repeat("e", 64), AllowedChanges: []string{}, ChangedPaths: []string{}, ZeroMutationExact: true},
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
	got, err := normalizeCodex(candidate, "run-1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "codex" || len(got.Skills) != 9 || len(got.RuntimeModes) != 28 ||
		!got.SemanticRerun.Valid() || !got.Mutation.Valid() || got.EvidenceFingerprint == "" {
		t.Fatalf("normalized Codex evidence = %#v", got)
	}
	raw.CodexVersion = "codex-cli " + vercelacceptance.ExactCodexVersion + "-dev"
	if _, err := normalizeCodex(candidate, "run-1", raw); err == nil {
		t.Fatal("non-exact Codex version passed")
	}
	raw.CodexVersion = "codex-cli 9.9.9 " + vercelacceptance.ExactCodexVersion
	if _, err := normalizeCodex(candidate, "run-1", raw); err == nil {
		t.Fatal("Codex version suffix passed")
	}
	raw.CodexVersion = "codex-cli " + vercelacceptance.ExactCodexVersion
	raw.NoDeploy = false
	if _, err := normalizeCodex(candidate, "run-1", raw); err == nil {
		t.Fatal("unsafe Codex evidence passed")
	}
}

func TestNormalizeOpenCodeBindsExactCandidateInventoryAndSafety(t *testing.T) {
	candidate := strings.Repeat("a", 40)
	raw := opencodesmoke.Evidence{
		SchemaVersion: 2, PackyRef: candidate, PackySHA: candidate,
		RunID: "run-1", ObservedAt: time.Now().UTC(),
		VercelFixtureSHA256:   vercelacceptance.ExactArchiveSHA256,
		OpenCodeVersion:       vercelacceptance.ExactOpenCodeVersion,
		OpenCodeArchiveSHA256: strings.Repeat("b", 64), OpenCodeExecutableSHA256: strings.Repeat("c", 64),
		SandboxRoots:           []string{"home", "xdg", "data", "cache", "state", "bundle", "work"},
		SemanticRerun:          vercelacceptance.SemanticRerunEvidence{FirstSHA256: strings.Repeat("e", 64), SecondSHA256: strings.Repeat("e", 64), ExactMatch: true},
		Mutation:               vercelacceptance.MutationObservation{Root: "$SANDBOX/bundle", BeforeSHA256: strings.Repeat("f", 64), AfterSHA256: strings.Repeat("f", 64), AllowedChanges: []string{}, ChangedPaths: []string{}, ZeroMutationExact: true},
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
	got, err := normalizeOpenCode(candidate, "run-1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "opencode" || len(got.Skills) != 9 || len(got.RuntimeModes) != 28 ||
		!got.SemanticRerun.Valid() || !got.Mutation.Valid() || got.EvidenceFingerprint == "" {
		t.Fatalf("normalized OpenCode evidence = %#v", got)
	}
	raw.OpenCodeVersion = vercelacceptance.ExactOpenCodeVersion + "-dev"
	if _, err := normalizeOpenCode(candidate, "run-1", raw); err == nil {
		t.Fatal("non-exact OpenCode version passed")
	}
	raw.OpenCodeVersion = "opencode-dev " + vercelacceptance.ExactOpenCodeVersion
	if _, err := normalizeOpenCode(candidate, "run-1", raw); err == nil {
		t.Fatal("OpenCode version suffix passed")
	}
	raw.OpenCodeVersion = vercelacceptance.ExactOpenCodeVersion
	raw.NativeSkillToolObserved = false
	if _, err := normalizeOpenCode(candidate, "run-1", raw); err == nil {
		t.Fatal("non-native OpenCode evidence passed")
	}
}
