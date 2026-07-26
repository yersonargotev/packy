package vercelacceptance

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

const (
	ExactCodexVersion    = "0.145.0"
	ExactOpenCodeVersion = "1.18.5"
	ExactClaudeVersion   = "2.1.203"
)

type HostEvidenceSet struct {
	Codex    HostEvidence `json:"codex"`
	OpenCode HostEvidence `json:"opencode"`
	Claude   HostEvidence `json:"claude"`
}

// HostEvidence is the intentionally narrow, portable projection of a host
// smoke artifact. Each adapter must populate it from that host's own artifact.
type HostEvidence struct {
	Host                    Host      `json:"host"`
	Version                 string    `json:"version"`
	CandidateSHA            string    `json:"candidate_sha"`
	FixtureSHA256           string    `json:"fixture_sha256"`
	RunID                   string    `json:"run_id"`
	ObservedAt              time.Time `json:"observed_at"`
	Skills                  []string  `json:"skills"`
	RuntimeModes            []string  `json:"runtime_modes"`
	MissingOne              string    `json:"missing_one_negative_twin"`
	MissingOneObservedCount int       `json:"missing_one_observed_count"`
	DisposableSandbox       bool      `json:"disposable_sandbox"`
	NoSecrets               bool      `json:"no_secrets"`
	NoDeploy                bool      `json:"no_deploy"`
	NoUpstreamEffects       bool      `json:"no_upstream_effects"`
	EvidenceFingerprint     string    `json:"evidence_fingerprint"`
}

func FingerprintHostEvidence(e HostEvidence) string {
	e.EvidenceFingerprint = ""
	return digest(e)
}

func ValidateHostEvidence(candidateSHA, runID string, now time.Time, maxAge time.Duration, set HostEvidenceSet) error {
	if candidateSHA == "" || runID == "" || now.IsZero() || maxAge <= 0 {
		return errors.New("candidate, run ID, current time, and positive freshness window are required")
	}
	checks := []struct {
		name    Host
		version string
		value   HostEvidence
	}{{HostCodex, ExactCodexVersion, set.Codex}, {HostOpenCode, ExactOpenCodeVersion, set.OpenCode}, {HostClaude, ExactClaudeVersion, set.Claude}}
	for _, check := range checks {
		if err := validateHost(candidateSHA, runID, now, maxAge, check.name, check.version, check.value); err != nil {
			return fmt.Errorf("%s evidence: %w", check.name, err)
		}
	}
	return nil
}

func validateHost(candidate, runID string, now time.Time, maxAge time.Duration, host Host, version string, e HostEvidence) error {
	if e.Host != host || e.Version != version || e.CandidateSHA != candidate ||
		e.FixtureSHA256 != ExactArchiveSHA256 || e.RunID != runID {
		return errors.New("identity mismatch")
	}
	age := now.Sub(e.ObservedAt)
	if e.ObservedAt.IsZero() || age < 0 || age > maxAge {
		return errors.New("stale observation")
	}
	if !e.DisposableSandbox || !e.NoSecrets || !e.NoDeploy || !e.NoUpstreamEffects {
		return errors.New("unsafe host evidence")
	}
	if e.EvidenceFingerprint == "" || e.EvidenceFingerprint != FingerprintHostEvidence(e) {
		return errors.New("tampered or cross-host evidence")
	}
	wantSkills, wantModes := expectedHostInventory()
	if !sameSet(e.Skills, wantSkills) || !sameSet(e.RuntimeModes, wantModes) {
		return errors.New("incomplete or duplicate skill/mode evidence")
	}
	found := false
	for _, skill := range wantSkills {
		found = found || skill == e.MissingOne
	}
	if !found || e.MissingOneObservedCount != len(wantSkills)-1 {
		return errors.New("invalid missing-one negative twin")
	}
	return nil
}

func expectedHostInventory() ([]string, []string) {
	pack := Canonical().Pack
	var skills, modes []string
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface == "codex" {
				skills = append(skills, binding.Name)
			}
		}
		for _, mode := range resource.RuntimeModes {
			modes = append(modes, resource.ID+"/"+mode.ID)
		}
	}
	sort.Strings(skills)
	sort.Strings(modes)
	return skills, modes
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	copyGot := append([]string(nil), got...)
	sort.Strings(copyGot)
	for i := range want {
		if copyGot[i] != want[i] || (i > 0 && copyGot[i] == copyGot[i-1]) {
			return false
		}
	}
	return true
}
