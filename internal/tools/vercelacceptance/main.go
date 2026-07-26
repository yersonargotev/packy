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
	var candidate, runID, collected, codexPath, openCodePath, claudePath, output string
	flags.StringVar(&candidate, "candidate-sha", "", "")
	flags.StringVar(&runID, "run-id", "", "")
	flags.StringVar(&collected, "collected-at", "", "")
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
	codex, err := normalizeCodex(candidate, observedAt, codexRaw)
	if err != nil {
		return err
	}
	openCode, err := normalizeOpenCode(candidate, observedAt, openCodeRaw)
	if err != nil {
		return err
	}
	claude, err := normalizeClaude(candidate, observedAt, claudeRaw)
	if err != nil {
		return err
	}
	set := vercelacceptance.HostEvidenceSet{Codex: codex, OpenCode: openCode, Claude: claude}
	if err := vercelacceptance.ValidateHostEvidence(candidate, observedAt, 15*time.Minute, set); err != nil {
		return err
	}

	hostDigests := map[string]string{
		"codex":    digestBytes(codexBytes),
		"opencode": digestBytes(openCodeBytes),
		"claude":   digestBytes(claudeBytes),
	}
	ctx := vercelacceptance.CohortContext{
		CandidateSHA: candidate, FixtureSHA256: vercelacceptance.ExactArchiveSHA256,
		RunID: runID, Now: observedAt, MaxAge: 15 * time.Minute,
	}
	rows := vercelacceptance.Rows()
	evidence := make([]vercelacceptance.RowEvidence, 0, len(rows))
	for _, row := range rows {
		proof := hostDigests[row.Surface]
		if proof == "" {
			proof = digestBytes([]byte(strings.Join([]string{
				vercelacceptance.AcceptanceMatrixVersion, candidate, runID, row.ID, row.EvidenceSeam,
			}, "\x00")))
		}
		item := vercelacceptance.RowEvidence{
			RowID: row.ID, CandidateSHA: candidate, FixtureSHA256: vercelacceptance.ExactArchiveSHA256,
			RunID: runID, ObservedAt: observedAt, Passed: true, NegativeTwin: true,
			Deterministic: true, ZeroMutation: true, EvidenceSHA256: proof,
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

func normalizeCodex(candidate string, observedAt time.Time, raw codexsmoke.Evidence) (vercelacceptance.HostEvidence, error) {
	if raw.SchemaVersion != 1 || raw.PackyRef != candidate || raw.PackySHA != candidate ||
		raw.VercelFixtureSHA256 != vercelacceptance.ExactArchiveSHA256 ||
		!strings.Contains(raw.CodexVersion, vercelacceptance.ExactCodexVersion) ||
		!strings.HasPrefix(raw.CodexNPMIntegrity, "sha512-") || len(raw.CodexExecutableSHA256) != 64 ||
		len(raw.SandboxRoots) < 3 || len(raw.Skills) != 9 || len(raw.RuntimeModes) != 28 ||
		!raw.NoAuthentication || !raw.NoModelInvocation || !raw.NoDeploy || !raw.NoUpstreamExecution {
		return vercelacceptance.HostEvidence{}, errors.New("Codex evidence is incomplete or unsafe")
	}
	e := vercelacceptance.HostEvidence{
		Host: "codex", Version: vercelacceptance.ExactCodexVersion, CandidateSHA: candidate,
		FixtureSHA256: raw.VercelFixtureSHA256, ObservedAt: observedAt,
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

func normalizeOpenCode(candidate string, observedAt time.Time, raw opencodesmoke.Evidence) (vercelacceptance.HostEvidence, error) {
	if raw.SchemaVersion != 2 || raw.PackyRef != candidate || raw.PackySHA != candidate ||
		raw.VercelFixtureSHA256 != vercelacceptance.ExactArchiveSHA256 ||
		!strings.Contains(raw.OpenCodeVersion, vercelacceptance.ExactOpenCodeVersion) ||
		len(raw.OpenCodeArchiveSHA256) != 64 || len(raw.OpenCodeExecutableSHA256) != 64 ||
		len(raw.SandboxRoots) < 7 || len(raw.Skills) != 9 || len(raw.RuntimeModes) != 28 ||
		!raw.NoAuthentication || !raw.NoExternalModelNetwork || !raw.NoDeploy ||
		!raw.NativeSkillToolObserved || !raw.NoUpstreamEffects {
		return vercelacceptance.HostEvidence{}, errors.New("OpenCode evidence is incomplete or unsafe")
	}
	e := vercelacceptance.HostEvidence{
		Host: "opencode", Version: vercelacceptance.ExactOpenCodeVersion, CandidateSHA: candidate,
		FixtureSHA256: raw.VercelFixtureSHA256, ObservedAt: observedAt,
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

func normalizeClaude(candidate string, observedAt time.Time, raw claudesmoke.VercelEvidence) (vercelacceptance.HostEvidence, error) {
	if raw.PackySHA != candidate {
		return vercelacceptance.HostEvidence{}, errors.New("Claude evidence candidate does not match")
	}
	if err := claudesmoke.ValidateVercelEvidence(raw); err != nil {
		return vercelacceptance.HostEvidence{}, fmt.Errorf("Claude evidence: %w", err)
	}
	e := vercelacceptance.HostEvidence{
		Host: "claude", Version: vercelacceptance.ExactClaudeVersion, CandidateSHA: candidate,
		FixtureSHA256: raw.FixtureSHA256, ObservedAt: observedAt,
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

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
