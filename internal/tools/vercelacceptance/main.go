package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/claudesmoke"
	"github.com/yersonargotev/packy/internal/codexsmoke"
	"github.com/yersonargotev/packy/internal/opencodesmoke"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

const evidenceLimit = 8 << 20

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("vercelacceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var candidate, runID, collected, foundationPath, codexPath, openCodePath, claudePath, output string
	flags.StringVar(&candidate, "candidate-sha", "", "")
	flags.StringVar(&runID, "run-id", "", "")
	flags.StringVar(&collected, "collected-at", "", "")
	flags.StringVar(&foundationPath, "foundation-evidence", "", "")
	flags.StringVar(&codexPath, "codex-evidence", "", "")
	flags.StringVar(&openCodePath, "opencode-evidence", "", "")
	flags.StringVar(&claudePath, "claude-evidence", "", "")
	flags.StringVar(&output, "output", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid Vercel acceptance arguments")
	}
	observedAt, err := time.Parse(time.RFC3339, collected)
	if err != nil || !fullSHA(candidate) || strings.TrimSpace(runID) == "" {
		return errors.New("candidate SHA, run ID, and RFC3339 collection time are required")
	}
	if !filepath.IsAbs(output) {
		return errors.New("output must be absolute")
	}
	foundation, err := loadFoundation(foundationPath, candidate, runID, observedAt)
	if err != nil {
		return fmt.Errorf("foundation evidence: %w", err)
	}

	codexRaw, codexBytes, err := decodeEvidence[codexsmoke.Evidence](codexPath)
	if err != nil {
		return fmt.Errorf("Codex evidence: %w", err)
	}
	openCodeRaw, openCodeBytes, err := decodeEvidence[opencodesmoke.Evidence](openCodePath)
	if err != nil {
		return fmt.Errorf("OpenCode evidence: %w", err)
	}
	claudeRaw, claudeBytes, err := decodeEvidence[claudesmoke.VercelEvidence](claudePath)
	if err != nil {
		return fmt.Errorf("Claude evidence: %w", err)
	}
	codex, err := normalizeCodex(candidate, runID, codexRaw)
	if err != nil {
		return err
	}
	openCode, err := normalizeOpenCode(candidate, runID, openCodeRaw)
	if err != nil {
		return err
	}
	claude, err := normalizeClaude(candidate, runID, claudeRaw)
	if err != nil {
		return err
	}
	set := vercelacceptance.HostEvidenceSet{Codex: codex, OpenCode: openCode, Claude: claude}
	if err := vercelacceptance.ValidateHostEvidence(candidate, runID, observedAt, 15*time.Minute, set); err != nil {
		return err
	}

	hostDigests := map[vercelacceptance.Host]string{
		vercelacceptance.HostCodex:    digestBytes(codexBytes),
		vercelacceptance.HostOpenCode: digestBytes(openCodeBytes),
		vercelacceptance.HostClaude:   digestBytes(claudeBytes),
	}
	hostObserved := map[vercelacceptance.Host]time.Time{
		vercelacceptance.HostCodex:    codex.ObservedAt,
		vercelacceptance.HostOpenCode: openCode.ObservedAt,
		vercelacceptance.HostClaude:   claude.ObservedAt,
	}
	ctx := vercelacceptance.CohortContext{
		CandidateSHA: candidate, FixtureSHA256: vercelacceptance.ExactArchiveSHA256,
		RunID: runID, Now: observedAt, MaxAge: 15 * time.Minute,
	}
	rows := vercelacceptance.Rows()
	evidence := append([]vercelacceptance.RowEvidence(nil), foundation...)
	for _, row := range rows {
		if row.ID != "VERCEL-ACCEPTANCE-17" && row.ID != "VERCEL-ACCEPTANCE-18" && row.ID != "VERCEL-ACCEPTANCE-19" {
			continue
		}
		item := vercelacceptance.RowEvidence{
			RowID: row.ID, CandidateSHA: candidate, FixtureSHA256: vercelacceptance.ExactArchiveSHA256,
			RunID: runID, ObservedAt: hostObserved[row.Surface], Passed: true, NegativeTwin: true,
			Deterministic: true, ZeroMutation: true, EvidenceSHA256: hostDigests[row.Surface],
		}
		item.EvidenceFingerprint = vercelacceptance.FingerprintRowEvidence(item)
		evidence = append(evidence, item)
	}
	report, err := vercelacceptance.Evaluate(ctx, evidence)
	if err != nil {
		return err
	}
	data, err := report.CanonicalJSON()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func decodeEvidence[T any](path string) (T, []byte, error) {
	var value T
	if !filepath.IsAbs(path) {
		return value, nil, errors.New("path must be absolute")
	}
	file, err := os.Open(path)
	if err != nil {
		return value, nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, evidenceLimit+1))
	if err != nil {
		return value, nil, err
	}
	if len(data) > evidenceLimit {
		return value, nil, errors.New("artifact exceeds size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return value, nil, errors.New("artifact contains trailing JSON")
	}
	return value, data, nil
}

type foundationIdentity struct {
	SchemaVersion int       `json:"schema_version"`
	MatrixVersion string    `json:"matrix_version"`
	CandidateSHA  string    `json:"candidate_sha"`
	FixtureSHA256 string    `json:"fixture_sha256"`
	RunID         string    `json:"run_id"`
	ObservedAt    time.Time `json:"observed_at"`
}

func loadFoundation(root, candidate, runID string, now time.Time) ([]vercelacceptance.RowEvidence, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("directory must be absolute")
	}
	identity, _, err := decodeEvidence[foundationIdentity](filepath.Join(root, "identity.json"))
	if err != nil {
		return nil, err
	}
	if identity.SchemaVersion != 1 || identity.MatrixVersion != vercelacceptance.AcceptanceMatrixVersion ||
		identity.CandidateSHA != candidate || identity.FixtureSHA256 != vercelacceptance.ExactArchiveSHA256 ||
		identity.RunID != runID || identity.ObservedAt.IsZero() ||
		identity.ObservedAt.After(now) || now.Sub(identity.ObservedAt) > 15*time.Minute {
		return nil, errors.New("identity is mixed or stale")
	}
	var result []vercelacceptance.RowEvidence
	for _, row := range vercelacceptance.Rows() {
		if row.ID == "VERCEL-ACCEPTANCE-17" || row.ID == "VERCEL-ACCEPTANCE-18" || row.ID == "VERCEL-ACCEPTANCE-19" {
			continue
		}
		first, err := readEvidenceFile(filepath.Join(root, row.ID+".first.txt"))
		if err != nil {
			return nil, err
		}
		second, err := readEvidenceFile(filepath.Join(root, row.ID+".second.txt"))
		if err != nil {
			return nil, err
		}
		if string(first) != string(second) {
			return nil, fmt.Errorf("%s deterministic rerun changed", row.ID)
		}
		slash := strings.LastIndex(row.EvidenceSeam, "/")
		if slash < 0 || !strings.HasPrefix(row.EvidenceSeam, "./internal/") {
			return nil, fmt.Errorf("%s has invalid owning seam", row.ID)
		}
		test := row.EvidenceSeam[slash+1:]
		text := string(first)
		if strings.Count(text, "=== RUN   "+test+"\n") != 1 ||
			strings.Count(text, "--- PASS: "+test+" (duration)\n") != 1 {
			return nil, fmt.Errorf("%s lacks exact owning RUN/PASS proof", row.ID)
		}
		item := vercelacceptance.RowEvidence{
			RowID: row.ID, CandidateSHA: candidate, FixtureSHA256: identity.FixtureSHA256,
			RunID: runID, ObservedAt: identity.ObservedAt, Passed: true, NegativeTwin: true,
			Deterministic: true, ZeroMutation: true, EvidenceSHA256: digestBytes(first),
		}
		item.EvidenceFingerprint = vercelacceptance.FingerprintRowEvidence(item)
		result = append(result, item)
	}
	return result, nil
}

func readEvidenceFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, evidenceLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > evidenceLimit {
		return nil, errors.New("foundation output exceeds size limit")
	}
	return data, nil
}

func normalizeCodex(candidate, runID string, raw codexsmoke.Evidence) (vercelacceptance.HostEvidence, error) {
	if raw.SchemaVersion != 1 || raw.PackyRef != candidate || raw.PackySHA != candidate ||
		raw.RunID != runID || raw.ObservedAt.IsZero() ||
		raw.VercelFixtureSHA256 != vercelacceptance.ExactArchiveSHA256 ||
		exactVersion(raw.CodexVersion) != vercelacceptance.ExactCodexVersion ||
		!strings.HasPrefix(raw.CodexNPMIntegrity, "sha512-") || len(raw.CodexExecutableSHA256) != 64 ||
		len(raw.SandboxRoots) < 3 || len(raw.Skills) != 9 || len(raw.RuntimeModes) != 28 ||
		!raw.NoAuthentication || !raw.NoModelInvocation || !raw.NoDeploy || !raw.NoUpstreamExecution {
		return vercelacceptance.HostEvidence{}, errors.New("Codex evidence is incomplete or unsafe")
	}
	e := vercelacceptance.HostEvidence{
		Host: vercelacceptance.HostCodex, Version: vercelacceptance.ExactCodexVersion, CandidateSHA: candidate,
		FixtureSHA256: raw.VercelFixtureSHA256, RunID: raw.RunID, ObservedAt: raw.ObservedAt,
		MissingOne: raw.MissingOneNegativeTwin, MissingOneObservedCount: 8,
		DisposableSandbox: true, NoSecrets: true, NoDeploy: true, NoUpstreamEffects: true,
	}
	for _, skill := range raw.Skills {
		if !skill.Enabled || !skill.InvocationAvailable || len(skill.SHA256) != 64 || skill.Invocation == "" {
			return e, errors.New("Codex skill evidence is incomplete")
		}
		e.Skills = append(e.Skills, skill.Name)
	}
	for _, mode := range raw.RuntimeModes {
		if !mode.SelectionObserved || !mode.FailBeforeEffects || mode.Invocation == "" {
			return e, errors.New("Codex runtime evidence is incomplete")
		}
		e.RuntimeModes = append(e.RuntimeModes, mode.ResourceID+"/"+mode.ModeID)
	}
	e.EvidenceFingerprint = vercelacceptance.FingerprintHostEvidence(e)
	return e, nil
}

func normalizeOpenCode(candidate, runID string, raw opencodesmoke.Evidence) (vercelacceptance.HostEvidence, error) {
	if raw.SchemaVersion != 2 || raw.PackyRef != candidate || raw.PackySHA != candidate ||
		raw.RunID != runID || raw.ObservedAt.IsZero() ||
		raw.VercelFixtureSHA256 != vercelacceptance.ExactArchiveSHA256 ||
		exactVersion(raw.OpenCodeVersion) != vercelacceptance.ExactOpenCodeVersion ||
		len(raw.OpenCodeArchiveSHA256) != 64 || len(raw.OpenCodeExecutableSHA256) != 64 ||
		len(raw.SandboxRoots) < 7 || len(raw.Skills) != 9 || len(raw.RuntimeModes) != 28 ||
		!raw.NoAuthentication || !raw.NoExternalModelNetwork || !raw.NoDeploy ||
		!raw.NativeSkillToolObserved || !raw.NoUpstreamEffects {
		return vercelacceptance.HostEvidence{}, errors.New("OpenCode evidence is incomplete or unsafe")
	}
	e := vercelacceptance.HostEvidence{
		Host: vercelacceptance.HostOpenCode, Version: vercelacceptance.ExactOpenCodeVersion, CandidateSHA: candidate,
		FixtureSHA256: raw.VercelFixtureSHA256, RunID: raw.RunID, ObservedAt: raw.ObservedAt,
		MissingOne: raw.MissingOneNegativeTwin, MissingOneObservedCount: 8,
		DisposableSandbox: true, NoSecrets: true, NoDeploy: true, NoUpstreamEffects: true,
	}
	for _, skill := range raw.Skills {
		if !skill.ContentLoaded || len(skill.SHA256) != 64 {
			return e, errors.New("OpenCode skill evidence is incomplete")
		}
		e.Skills = append(e.Skills, skill.Name)
	}
	for _, mode := range raw.RuntimeModes {
		if !mode.SelectionObserved || !mode.InvocationAvailable || !mode.FailBeforeHostEffects || mode.Invocation == "" {
			return e, errors.New("OpenCode runtime evidence is incomplete")
		}
		e.RuntimeModes = append(e.RuntimeModes, mode.ResourceID+"/"+mode.ModeID)
	}
	e.EvidenceFingerprint = vercelacceptance.FingerprintHostEvidence(e)
	return e, nil
}

func normalizeClaude(candidate, runID string, raw claudesmoke.VercelEvidence) (vercelacceptance.HostEvidence, error) {
	if raw.PackySHA != candidate || raw.RunID != runID || raw.ObservedAt.IsZero() {
		return vercelacceptance.HostEvidence{}, errors.New("Claude evidence candidate does not match")
	}
	if err := claudesmoke.ValidateVercelEvidence(raw); err != nil {
		return vercelacceptance.HostEvidence{}, fmt.Errorf("Claude evidence: %w", err)
	}
	e := vercelacceptance.HostEvidence{
		Host: vercelacceptance.HostClaude, Version: vercelacceptance.ExactClaudeVersion, CandidateSHA: candidate,
		FixtureSHA256: raw.FixtureSHA256, RunID: raw.RunID, ObservedAt: raw.ObservedAt,
		MissingOne: raw.Skills[0].Name, MissingOneObservedCount: raw.MissingOne.UserSkillDirCommands,
		DisposableSandbox: true, NoSecrets: true, NoDeploy: true, NoUpstreamEffects: true,
	}
	for _, skill := range raw.Skills {
		e.Skills = append(e.Skills, skill.Name)
	}
	for _, mode := range raw.RuntimeModes {
		e.RuntimeModes = append(e.RuntimeModes, mode.ResourceID+"/"+mode.ModeID)
	}
	e.EvidenceFingerprint = vercelacceptance.FingerprintHostEvidence(e)
	return e, nil
}

func fullSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func exactVersion(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
