package vercelacceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestCanonicalIndependentHostEvidence(t *testing.T) {
	now, set := canonicalHosts()
	if err := ValidateHostEvidence(strings.Repeat("a", 40), "run-1", now, time.Hour, set); err != nil {
		t.Fatal(err)
	}
}

func TestHostEvidenceRejectsEveryIndependentNegative(t *testing.T) {
	now, good := canonicalHosts()
	tests := []struct {
		name string
		edit func(*HostEvidenceSet)
	}{
		{"exact version", func(s *HostEvidenceSet) { s.Codex.Version = "0.146.0" }},
		{"no cross host inference", func(s *HostEvidenceSet) {
			s.Claude = s.Codex
			s.Claude.Host = "claude"
			s.Claude.Version = ExactClaudeVersion
		}},
		{"nine skills", func(s *HostEvidenceSet) { s.OpenCode.Skills = s.OpenCode.Skills[:8] }},
		{"28 modes", func(s *HostEvidenceSet) { s.Claude.RuntimeModes = s.Claude.RuntimeModes[:27] }},
		{"duplicate", func(s *HostEvidenceSet) { s.Codex.Skills[8] = s.Codex.Skills[0] }},
		{"missing one", func(s *HostEvidenceSet) { s.OpenCode.MissingOneObservedCount = 9 }},
		{"fresh", func(s *HostEvidenceSet) { s.Claude.ObservedAt = now.Add(-2 * time.Hour) }},
		{"same run", func(s *HostEvidenceSet) { s.Claude.RunID = "other-run" }},
		{"semantic rerun", func(s *HostEvidenceSet) { s.Codex.SemanticRerun.SecondSHA256 = strings.Repeat("d", 64) }},
		{"zero mutation", func(s *HostEvidenceSet) { s.OpenCode.Mutation.AfterSHA256 = strings.Repeat("e", 64) }},
		{"sandbox", func(s *HostEvidenceSet) { s.Codex.DisposableSandbox = false }},
		{"no secret", func(s *HostEvidenceSet) { s.OpenCode.NoSecrets = false }},
		{"no deploy", func(s *HostEvidenceSet) { s.Claude.NoDeploy = false }},
		{"no upstream effects", func(s *HostEvidenceSet) { s.Codex.NoUpstreamEffects = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := cloneHosts(good)
			tt.edit(&set)
			if err := ValidateHostEvidence(strings.Repeat("a", 40), "run-1", now, time.Hour, set); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestHostEvidenceRejectsSealedInvalidRerunAndMutationProofs(t *testing.T) {
	now, good := canonicalHosts()
	for _, edit := range []func(*HostEvidence){
		func(e *HostEvidence) { e.SemanticRerun.SecondSHA256 = strings.Repeat("d", 64) },
		func(e *HostEvidence) { e.Mutation.AfterSHA256 = strings.Repeat("e", 64) },
	} {
		set := cloneHosts(good)
		edit(&set.Codex)
		set.Codex.EvidenceFingerprint = FingerprintHostEvidence(set.Codex)
		if err := ValidateHostEvidence(strings.Repeat("a", 40), "run-1", now, time.Hour, set); err == nil {
			t.Fatal("sealed invalid host proof passed")
		}
	}
}

func TestMutationObservationDerivesConcreteExactTreeChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "nested", "SKILL.md")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := localprojection.SnapshotExactTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	after, err := localprojection.SnapshotExactTree(root)
	if err != nil {
		t.Fatal(err)
	}
	observation := NewMutationObservation("$SANDBOX/bundle", before, after)
	if got, want := strings.Join(observation.ChangedPaths, ","), "empty,nested/SKILL.md"; got != want {
		t.Fatalf("changed paths = %q, want %q", got, want)
	}
	if observation.ZeroMutationExact || observation.Valid() {
		t.Fatal("changed exact tree was reported as zero mutation")
	}
	unchanged := NewMutationObservation("$SANDBOX/bundle", before, before)
	if !unchanged.Valid() {
		t.Fatal("identical exact snapshots did not prove zero mutation")
	}
}

func TestHostRowsDeriveRerunAndMutationFactsFromArtifacts(t *testing.T) {
	_, set := canonicalHosts()
	digests := map[Host]string{
		HostCodex: strings.Repeat("d", 64), HostOpenCode: strings.Repeat("e", 64), HostClaude: strings.Repeat("f", 64),
	}
	rows, err := HostRowEvidence(strings.Repeat("a", 40), "run-1", set, digests)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("host rows = %d, want 3", len(rows))
	}
	for _, row := range rows {
		if !row.NegativeTwin || !row.Deterministic || !row.ZeroMutation || row.EvidenceFingerprint == "" {
			t.Fatalf("host row did not derive artifact facts: %#v", row)
		}
	}
	set.Codex.SemanticRerun.ExactMatch = false
	rows, err = HostRowEvidence(strings.Repeat("a", 40), "run-1", set, digests)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Deterministic {
		t.Fatal("host row fabricated deterministic rerun")
	}
}

func canonicalHosts() (time.Time, HostEvidenceSet) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	skills, modes := expectedHostInventory()
	makeHost := func(host Host, version string) HostEvidence {
		e := HostEvidence{
			Host: host, Version: version, CandidateSHA: strings.Repeat("a", 40), FixtureSHA256: ExactArchiveSHA256,
			RunID: "run-1", ObservedAt: now.Add(-time.Minute), Skills: append([]string(nil), skills...),
			RuntimeModes: append([]string(nil), modes...), MissingOne: skills[0], MissingOneObservedCount: 8,
			SemanticRerun: SemanticRerunEvidence{FirstSHA256: strings.Repeat("b", 64), SecondSHA256: strings.Repeat("b", 64), ExactMatch: true},
			Mutation: MutationObservation{
				Root: "$SANDBOX/bundle", BeforeSHA256: strings.Repeat("c", 64), AfterSHA256: strings.Repeat("c", 64),
				AllowedChanges: []string{}, ChangedPaths: []string{}, ZeroMutationExact: true,
			},
			DisposableSandbox: true, NoSecrets: true, NoDeploy: true, NoUpstreamEffects: true,
		}
		e.EvidenceFingerprint = FingerprintHostEvidence(e)
		return e
	}
	return now, HostEvidenceSet{Codex: makeHost(HostCodex, ExactCodexVersion), OpenCode: makeHost(HostOpenCode, ExactOpenCodeVersion), Claude: makeHost(HostClaude, ExactClaudeVersion)}
}

func cloneHosts(in HostEvidenceSet) HostEvidenceSet {
	clone := func(e HostEvidence) HostEvidence {
		e.Skills = append([]string(nil), e.Skills...)
		e.RuntimeModes = append([]string(nil), e.RuntimeModes...)
		e.Mutation.AllowedChanges = append([]string(nil), e.Mutation.AllowedChanges...)
		e.Mutation.ChangedPaths = append([]string(nil), e.Mutation.ChangedPaths...)
		return e
	}
	return HostEvidenceSet{Codex: clone(in.Codex), OpenCode: clone(in.OpenCode), Claude: clone(in.Claude)}
}
