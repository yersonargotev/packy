package vercelacceptance

import (
	"strings"
	"testing"
	"time"
)

func TestCanonicalIndependentHostEvidence(t *testing.T) {
	now, set := canonicalHosts()
	if err := ValidateHostEvidence(strings.Repeat("a", 40), now, time.Hour, set); err != nil {
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
		{"sandbox", func(s *HostEvidenceSet) { s.Codex.DisposableSandbox = false }},
		{"no secret", func(s *HostEvidenceSet) { s.OpenCode.NoSecrets = false }},
		{"no deploy", func(s *HostEvidenceSet) { s.Claude.NoDeploy = false }},
		{"no upstream effects", func(s *HostEvidenceSet) { s.Codex.NoUpstreamEffects = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := cloneHosts(good)
			tt.edit(&set)
			if err := ValidateHostEvidence(strings.Repeat("a", 40), now, time.Hour, set); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func canonicalHosts() (time.Time, HostEvidenceSet) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	skills, modes := expectedHostInventory()
	makeHost := func(host, version string) HostEvidence {
		e := HostEvidence{Host: host, Version: version, CandidateSHA: strings.Repeat("a", 40), FixtureSHA256: ExactArchiveSHA256, ObservedAt: now.Add(-time.Minute), Skills: append([]string(nil), skills...), RuntimeModes: append([]string(nil), modes...), MissingOne: skills[0], MissingOneObservedCount: 8, DisposableSandbox: true, NoSecrets: true, NoDeploy: true, NoUpstreamEffects: true}
		e.EvidenceFingerprint = FingerprintHostEvidence(e)
		return e
	}
	return now, HostEvidenceSet{Codex: makeHost("codex", ExactCodexVersion), OpenCode: makeHost("opencode", ExactOpenCodeVersion), Claude: makeHost("claude", ExactClaudeVersion)}
}

func cloneHosts(in HostEvidenceSet) HostEvidenceSet {
	clone := func(e HostEvidence) HostEvidence {
		e.Skills = append([]string(nil), e.Skills...)
		e.RuntimeModes = append([]string(nil), e.RuntimeModes...)
		return e
	}
	return HostEvidenceSet{Codex: clone(in.Codex), OpenCode: clone(in.OpenCode), Claude: clone(in.Claude)}
}
