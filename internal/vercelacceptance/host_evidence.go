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

type SemanticRerunEvidence struct {
	FirstSHA256  string `json:"first_sha256"`
	SecondSHA256 string `json:"second_sha256"`
	ExactMatch   bool   `json:"exact_match"`
}

func (e SemanticRerunEvidence) Valid() bool {
	return e.ExactMatch && lowerHexDigest(e.FirstSHA256) && e.FirstSHA256 == e.SecondSHA256
}

type MutationObservation struct {
	Root              string   `json:"root"`
	BeforeSHA256      string   `json:"before_sha256"`
	AfterSHA256       string   `json:"after_sha256"`
	AllowedChanges    []string `json:"allowed_changes"`
	ChangedPaths      []string `json:"changed_paths"`
	ZeroMutationExact bool     `json:"zero_mutation_exact"`
}

func (e MutationObservation) Valid() bool {
	return e.Root == "$SANDBOX/bundle" && e.ZeroMutationExact && lowerHexDigest(e.BeforeSHA256) &&
		e.BeforeSHA256 == e.AfterSHA256 && len(e.AllowedChanges) == 0 && len(e.ChangedPaths) == 0
}

// HostEvidence is the intentionally narrow, portable projection of a host
// smoke artifact. Each adapter must populate it from that host's own artifact.
type HostEvidence struct {
	Host                    Host                  `json:"host"`
	Version                 string                `json:"version"`
	CandidateSHA            string                `json:"candidate_sha"`
	FixtureSHA256           string                `json:"fixture_sha256"`
	RunID                   string                `json:"run_id"`
	ObservedAt              time.Time             `json:"observed_at"`
	Skills                  []string              `json:"skills"`
	RuntimeModes            []string              `json:"runtime_modes"`
	MissingOne              string                `json:"missing_one_negative_twin"`
	MissingOneObservedCount int                   `json:"missing_one_observed_count"`
	SemanticRerun           SemanticRerunEvidence `json:"semantic_rerun"`
	Mutation                MutationObservation   `json:"mutation_observation"`
	DisposableSandbox       bool                  `json:"disposable_sandbox"`
	NoSecrets               bool                  `json:"no_secrets"`
	NoDeploy                bool                  `json:"no_deploy"`
	NoUpstreamEffects       bool                  `json:"no_upstream_effects"`
	EvidenceFingerprint     string                `json:"evidence_fingerprint"`
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
	if !e.SemanticRerun.Valid() {
		return errors.New("invalid semantic rerun evidence")
	}
	if !e.Mutation.Valid() {
		return errors.New("invalid zero-mutation observation")
	}
	return nil
}

func HostRowEvidence(candidateSHA, runID string, set HostEvidenceSet, artifactDigests map[Host]string) ([]RowEvidence, error) {
	byHost := map[Host]HostEvidence{
		HostCodex: set.Codex, HostOpenCode: set.OpenCode, HostClaude: set.Claude,
	}
	var result []RowEvidence
	for _, row := range HostRows() {
		host, ok := byHost[row.Surface]
		digest, hasDigest := artifactDigests[row.Surface]
		if !ok || !hasDigest || !lowerHexDigest(digest) || host.CandidateSHA != candidateSHA || host.RunID != runID {
			return nil, fmt.Errorf("%s host row evidence is incomplete", row.ID)
		}
		item := RowEvidence{
			RowID: row.ID, CandidateSHA: candidateSHA, FixtureSHA256: ExactArchiveSHA256,
			RunID: runID, ObservedAt: host.ObservedAt, Passed: true,
			NegativeTwin:   host.MissingOne != "" && host.MissingOneObservedCount == len(host.Skills)-1,
			Deterministic:  host.SemanticRerun.Valid(),
			ZeroMutation:   host.Mutation.Valid(),
			EvidenceSHA256: digest,
		}
		item.EvidenceFingerprint = FingerprintRowEvidence(item)
		result = append(result, item)
	}
	return result, nil
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
