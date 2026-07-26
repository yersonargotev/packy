package vercelacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const AcceptanceMatrixVersion = "vercel-acceptance-v1"

type Gate uint8
type Host string

const (
	HostCodex    Host = "codex"
	HostOpenCode Host = "opencode"
	HostClaude   Host = "claude"
)

const (
	GateAdmission Gate = iota + 1
	GateContractClosure
	GateLifecycleSafety
	GateSurfaceConformance
	GateIndependentReadiness
	GateReproduciblePublication
)

type AcceptanceRow struct {
	ID           string `json:"id"`
	Gate         Gate   `json:"gate"`
	Surface      Host   `json:"surface,omitempty"`
	Name         string `json:"name"`
	EvidenceSeam string `json:"positive_seam"`
	NegativeSeam string `json:"negative_seam"`
	OracleSeam   string `json:"oracle_seam"`
	NegativeFact string `json:"negative_fact"`
	Oracle       string `json:"oracle"`
}

var acceptanceRows = []AcceptanceRow{
	{"VERCEL-ACCEPTANCE-01", Gate(1), "", "exact-composite-candidate", "./internal/packsync/TestVercelLegalAdmissionEvidence", "./internal/packsync/TestValidateLegalAdmissionRejectsOneFactNegativeTwins", "./internal/packsync/TestCompositeLegalAdmissionRequiresDurableExactEvidence", "one source identity differs", "zero candidate-byte diff"},
	{"VERCEL-ACCEPTANCE-02", Gate(1), "", "source-provenance-and-locks", "./internal/tools/syncpacksource/TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope", "./internal/packsync/TestCompositeRejectsInvalidSetsAndStaleFactsBeforeWrites", "./internal/packsync/TestRevalidateCompositeCandidatesRequiresEveryExactMember", "one provenance fact differs", "exact lock-set digest"},
	{"VERCEL-ACCEPTANCE-03", Gate(1), "", "primary-legal-admission", "./internal/packsync/TestVercelLegalAdmissionEvidence", "./internal/packsync/TestValidateLegalAdmissionRejectsOneFactNegativeTwins", "./internal/packsync/TestCompositeLegalAdmissionRequiresDurableExactEvidence", "primary authority is absent or stale", "exact documentary digest"},
	{"VERCEL-ACCEPTANCE-04", Gate(1), "", "secondary-notices", "./internal/vercelacceptance/TestExactSelectedTreesAreCompleteInertAndSealed", "./internal/vercelacceptance/TestNegativeTwinsFailDeterministicallyWithoutMutation", "./internal/vercelacceptance/TestValidateRejectsEverySealedFixtureGroup", "one MIT notice is absent", "exact notice bytes"},
	{"VERCEL-ACCEPTANCE-05", Gate(2), "", "complete-skill-inventory", "./internal/vercelacceptance/TestExactSelectedTreesAreCompleteInertAndSealed", "./internal/vercelacceptance/TestNegativeTwinsFailDeterministicallyWithoutMutation", "./internal/vercelacceptance/TestCanonicalExactClosureAndV4RoundTrip", "one selected skill tree is absent", "exact nine-tree inventory"},
	{"VERCEL-ACCEPTANCE-06", Gate(2), "", "native-three-surface-bindings", "./internal/vercelacceptance/TestExactSelectedTreesAreCompleteInertAndSealed", "./internal/vercelacceptance/TestNegativeTwinsFailDeterministicallyWithoutMutation", "./internal/vercelacceptance/TestCanonicalExactClosureAndV4RoundTrip", "one native binding is absent", "exact 27-binding set"},
	{"VERCEL-ACCEPTANCE-07", Gate(2), "", "sealed-loader-closure", "./internal/vercelacceptance/TestExactSelectedTreesAreCompleteInertAndSealed", "./internal/vercelacceptance/TestNegativeTwinsFailDeterministicallyWithoutMutation", "./internal/vercelacceptance/TestGuidelineAdaptationsAndSealedIdentities", "one loader uses a moving reference", "zero runtime network reads"},
	{"VERCEL-ACCEPTANCE-08", Gate(2), "", "runtime-mode-contract", "./internal/vercelacceptance/TestCanonicalRuntimeContractHasFreshExactCodexPreflight", "./internal/capabilitypack/TestEvaluateRuntimeModesRequiresExactCoverageAndSecretSafeDiagnostics", "./internal/capabilitypack/TestRuntimeModeFingerprintIgnoresFreshTimestampButSealsSemanticChanges", "one mode fact is omitted", "exact 28-mode set"},
	{"VERCEL-ACCEPTANCE-09", Gate(3), HostCodex, "codex-lifecycle-safety", "./internal/codex/TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly", "./internal/capabilitypack/TestVercelCollisionRequiresExplicitAliasBeforeMutation", "./internal/capabilitypack/TestVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe", "one write boundary fails", "exact permitted Codex diff"},
	{"VERCEL-ACCEPTANCE-10", Gate(3), HostOpenCode, "opencode-lifecycle-safety", "./internal/opencode/TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly", "./internal/capabilitypack/TestVercelCollisionRequiresExplicitAliasBeforeMutation", "./internal/capabilitypack/TestVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe", "one write boundary fails", "exact permitted OpenCode diff"},
	{"VERCEL-ACCEPTANCE-11", Gate(3), HostClaude, "claude-lifecycle-safety", "./internal/claudecode/TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly", "./internal/capabilitypack/TestVercelCollisionRequiresExplicitAliasBeforeMutation", "./internal/capabilitypack/TestVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe", "one write boundary fails", "exact permitted Claude diff"},
	{"VERCEL-ACCEPTANCE-12", Gate(3), "", "ownership-and-cross-surface-isolation", "./internal/capabilitypack/TestVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe", "./internal/capabilitypack/TestVercelCollisionRequiresExplicitAliasBeforeMutation", "./internal/capabilitypack/TestVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe", "foreign content or another surface changes", "zero foreign/cross-surface mutation"},
	{"VERCEL-ACCEPTANCE-13", Gate(4), HostCodex, "codex-disclosure-and-preflight", "./internal/vercelacceptance/TestCanonicalRuntimeContractHasFreshExactCodexPreflight", "./internal/capabilitypack/TestEvaluateRuntimeModesRequiresExactCoverageAndSecretSafeDiagnostics", "./internal/codexsmoke/TestExactFixtureProjectsNineCodexSkills", "one indispensable input is missing", "fail before effects"},
	{"VERCEL-ACCEPTANCE-14", Gate(4), HostOpenCode, "opencode-disclosure-and-preflight", "./internal/opencodesmoke/TestPreflightEveryVercelModeFailsBeforeHostEffects", "./internal/opencodesmoke/TestPreflightEveryVercelModeFailsBeforeHostEffects", "./internal/opencodesmoke/TestPreflightEveryVercelModeFailsBeforeHostEffects", "one indispensable input is missing", "fail before effects"},
	{"VERCEL-ACCEPTANCE-15", Gate(4), HostClaude, "claude-disclosure-and-preflight", "./internal/claudesmoke/TestVercelRuntimeEvidenceCoversExactTwentyEightModesSafely", "./internal/claudesmoke/TestValidateVercelEvidenceExactNamesCountsAndSafety", "./internal/claudesmoke/TestVercelRuntimeEvidenceCoversExactTwentyEightModesSafely", "one indispensable input is missing", "fail before effects"},
	{"VERCEL-ACCEPTANCE-16", Gate(4), "", "secret-free-tristate-contract", "./internal/capabilitypack/TestRuntimeEvidenceIsTriStateDeterministicAndSecretSafe", "./internal/capabilitypack/TestEvaluateRuntimeModesRequiresExactCoverageAndSecretSafeDiagnostics", "./internal/capabilitypack/TestRuntimeModeFingerprintIgnoresFreshTimestampButSealsSemanticChanges", "one sensitive value is present", "zero secret-bearing fields"},
	{"VERCEL-ACCEPTANCE-17", Gate(5), HostCodex, "codex-exact-host-readiness", "host-artifact/codex-0.145.0", "host-artifact/codex-missing-one", "host-artifact/codex-exact-inventory", "one skill is removed", "nine skills and 28 modes"},
	{"VERCEL-ACCEPTANCE-18", Gate(5), HostOpenCode, "opencode-exact-host-readiness", "host-artifact/opencode-1.18.5", "host-artifact/opencode-missing-one", "host-artifact/opencode-exact-inventory", "one skill is removed", "nine skills and 28 modes"},
	{"VERCEL-ACCEPTANCE-19", Gate(5), HostClaude, "claude-exact-host-readiness", "host-artifact/claude-2.1.203", "host-artifact/claude-missing-one", "host-artifact/claude-exact-inventory", "one skill is removed", "nine skills and 28 modes"},
	{"VERCEL-ACCEPTANCE-20", Gate(5), "", "compatibility-classification", "./internal/vercelacceptance/TestExactSelectedTreesAreCompleteInertAndSealed", "./internal/packsync/TestCanonicalCompositePackManifestParityAndSemanticChange", "./internal/vercelacceptance/TestCompatibilityAndAliasPolicyMatchTheFirstContract", "one observable contract fact changes", "maximum impact classification"},
	{"VERCEL-ACCEPTANCE-21", Gate(6), "", "independent-reacquisition", "./internal/tools/syncpacksource/TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope", "./internal/packsync/TestRevalidateCompositeCandidatesRequiresEveryExactMember", "./internal/tools/syncpacksource/TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope", "one acquired byte differs", "identical candidate digest"},
	{"VERCEL-ACCEPTANCE-22", Gate(6), "", "publication-reproduction", "./internal/tools/syncpacksource/TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope", "./internal/packsync/TestCompositeRejectsInvalidSetsAndStaleFactsBeforeWrites", "./internal/tools/syncpacksource/TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope", "one plan or bundle byte differs", "identical plan and bundle"},
	{"VERCEL-ACCEPTANCE-23", Gate(6), "", "proposal-safety", "./internal/ci/TestSyncWorkflowIsManualPinnedLeastPrivilegeAndPhaseSeparated", "./internal/ci/TestWorkflowTrustBoundaryMutationsFailClosed", "./internal/ci/TestSyncWorkflowIsManualPinnedLeastPrivilegeAndPhaseSeparated", "proposal state or privilege broadens", "least privilege and one proposal"},
	{"VERCEL-ACCEPTANCE-24", Gate(6), "", "complete-fresh-cohort", "./internal/vercelacceptance/TestCanonicalCohortReportAndDeterministicRerun", "./internal/vercelacceptance/TestCohortRejectsMissingDuplicateMixedStaleTamperedAndFailedEvidence", "./internal/vercelacceptance/TestCohortSuppressesEveryRowAfterFirstFailure", "one prior row is stale", "all 24 fresh fingerprints"},
}

func Rows() []AcceptanceRow { return append([]AcceptanceRow(nil), acceptanceRows...) }

type CohortContext struct {
	CandidateSHA  string
	FixtureSHA256 string
	RunID         string
	Now           time.Time
	MaxAge        time.Duration
}

type RowEvidence struct {
	RowID               string    `json:"row_id"`
	CandidateSHA        string    `json:"candidate_sha"`
	FixtureSHA256       string    `json:"fixture_sha256"`
	RunID               string    `json:"run_id"`
	ObservedAt          time.Time `json:"observed_at"`
	Passed              bool      `json:"passed"`
	NegativeTwin        bool      `json:"negative_twin"`
	Deterministic       bool      `json:"deterministic_rerun"`
	ZeroMutation        bool      `json:"zero_mutation_or_allowed_diff"`
	EvidenceSHA256      string    `json:"evidence_sha256"`
	EvidenceFingerprint string    `json:"evidence_fingerprint"`
}

type RowResult struct {
	AcceptanceRow
	Status   string            `json:"status"`
	Evidence *AcceptedEvidence `json:"evidence,omitempty"`
}

// AcceptedEvidence retains the detached proof facts that were validated for a
// report row without repeating the row, candidate, fixture, or run identity.
type AcceptedEvidence struct {
	ObservedAt          time.Time `json:"observed_at"`
	Passed              bool      `json:"passed"`
	NegativeTwin        bool      `json:"negative_twin"`
	Deterministic       bool      `json:"deterministic_rerun"`
	ZeroMutation        bool      `json:"zero_mutation_or_allowed_diff"`
	EvidenceSHA256      string    `json:"evidence_sha256"`
	EvidenceFingerprint string    `json:"evidence_fingerprint"`
}

type Report struct {
	SchemaVersion int         `json:"schema_version"`
	MatrixVersion string      `json:"matrix_version"`
	CandidateSHA  string      `json:"candidate_sha"`
	FixtureSHA256 string      `json:"fixture_sha256"`
	RunID         string      `json:"run_id"`
	Rows          []RowResult `json:"rows"`
	Fingerprint   string      `json:"fingerprint"`
}

func FingerprintRowEvidence(e RowEvidence) string {
	e.EvidenceFingerprint = ""
	return digest(e)
}

func Evaluate(ctx CohortContext, evidence []RowEvidence) (Report, error) {
	report := Report{SchemaVersion: 1, MatrixVersion: AcceptanceMatrixVersion, CandidateSHA: ctx.CandidateSHA, FixtureSHA256: ctx.FixtureSHA256, RunID: ctx.RunID}
	if ctx.CandidateSHA == "" || ctx.FixtureSHA256 == "" || ctx.RunID == "" || ctx.Now.IsZero() || ctx.MaxAge <= 0 {
		return report, errors.New("candidate, fixture, run ID, current time, and positive freshness window are required")
	}
	byID := make(map[string]RowEvidence, len(evidence))
	for _, item := range evidence {
		if _, exists := byID[item.RowID]; exists {
			return report, fmt.Errorf("duplicate row evidence %s", item.RowID)
		}
		byID[item.RowID] = item
	}
	var problems []string
	blocked := false
	for _, row := range acceptanceRows {
		status := "passed"
		item, ok := byID[row.ID]
		var accepted *AcceptedEvidence
		if blocked {
			status = "suppressed"
		} else if !ok {
			status, blocked = "failed", true
			problems = append(problems, row.ID+": missing evidence")
		} else if err := validateRow(ctx, row, item); err != nil {
			status, blocked = "failed", true
			problems = append(problems, row.ID+": "+err.Error())
		}
		if ok && status != "suppressed" {
			accepted = &AcceptedEvidence{
				ObservedAt: item.ObservedAt, Passed: item.Passed,
				NegativeTwin: item.NegativeTwin, Deterministic: item.Deterministic,
				ZeroMutation: item.ZeroMutation, EvidenceSHA256: item.EvidenceSHA256,
				EvidenceFingerprint: item.EvidenceFingerprint,
			}
		}
		report.Rows = append(report.Rows, RowResult{AcceptanceRow: row, Status: status, Evidence: accepted})
	}
	for id := range byID {
		if !isRow(id) {
			problems = append(problems, id+": unknown row")
		}
	}
	report.Fingerprint = reportDigest(report)
	if len(problems) > 0 {
		sort.Strings(problems)
		return report, errors.New(strings.Join(problems, "; "))
	}
	return report, nil
}

func validateRow(ctx CohortContext, row AcceptanceRow, item RowEvidence) error {
	if item.CandidateSHA != ctx.CandidateSHA || item.FixtureSHA256 != ctx.FixtureSHA256 || item.RunID != ctx.RunID {
		return errors.New("mixed candidate, fixture, or run evidence")
	}
	age := ctx.Now.Sub(item.ObservedAt)
	if item.ObservedAt.IsZero() || age < 0 || age > ctx.MaxAge {
		return errors.New("stale evidence")
	}
	if !item.Passed || !item.NegativeTwin || !item.Deterministic || !item.ZeroMutation {
		return errors.New("failed or incomplete evidence")
	}
	if !lowerHexDigest(item.EvidenceSHA256) {
		return errors.New("invalid owning evidence digest")
	}
	if item.EvidenceFingerprint == "" || item.EvidenceFingerprint != FingerprintRowEvidence(item) {
		return errors.New("tampered evidence fingerprint")
	}
	return nil
}

func (r Report) CanonicalJSON() ([]byte, error) {
	if r.SchemaVersion != 1 || r.MatrixVersion != AcceptanceMatrixVersion || r.CandidateSHA == "" || r.FixtureSHA256 == "" || r.RunID == "" || len(r.Rows) != len(acceptanceRows) {
		return nil, errors.New("invalid report identity or row count")
	}
	if r.Fingerprint == "" || r.Fingerprint != reportDigest(r) {
		return nil, errors.New("invalid report fingerprint")
	}
	b, err := json.MarshalIndent(r, "", "  ")
	return append(b, '\n'), err
}

func reportDigest(r Report) string {
	r.Fingerprint = ""
	return digest(r)
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func lowerHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	if err != nil {
		return false
	}
	for _, c := range value {
		if c >= 'A' && c <= 'F' {
			return false
		}
	}
	return true
}

func isRow(id string) bool {
	for _, row := range acceptanceRows {
		if row.ID == id {
			return true
		}
	}
	return false
}
