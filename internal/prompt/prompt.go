package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	codexPackySectionID = "skills-router"
	packyRulesSectionID = "rules"
	dotsRulesOpen       = "<!-- dots:rules -->"
	dotsRulesClose      = "<!-- /dots:rules -->"
)

type RulesDisposition string

const (
	RulesNoExternalProvider        RulesDisposition = "packy-projected"
	RulesExternallySatisfied       RulesDisposition = "externally-satisfied"
	RulesExternalDrift             RulesDisposition = "external-drift"
	RulesMalformedExternalProvider RulesDisposition = "malformed-external-provider"
)

type RulesObservation struct {
	Disposition RulesDisposition
	Fingerprint string
}

type WriteResult struct {
	Warnings []string
}

type Inspection struct {
	HasPackySection bool
}

func CodexContent() string {
	return strings.TrimSpace(`## Packy global workflow

- Global skills live in ~/.agents/skills. When a task matches a skill, read that skill's SKILL.md before acting.
- Use ask-matt at ~/.agents/skills/ask-matt as the router when you are unsure which skill or workflow applies.
- Use Engram memory tools when available: search before past-work or project-sensitive tasks; save decisions, discoveries, bug fixes, and conventions; summarize sessions before finishing.
- Apply host delegation rules when this Codex session exposes subagent/delegation tools. If unavailable, proceed inline and mention that delegation was unavailable.`) + "\n"
}

func RulesContent() string {
	return strings.TrimSpace(`## Packy Agent Rules

| Boundary | Rule |
| --- | --- |
| Always | Keep diffs surgical: every changed line must trace to the user request; mention unrelated issues instead of fixing them silently. |
| Always | Choose the simplest change that satisfies the request; avoid speculative abstractions, configurability, or features not explicitly needed. |
| Always | Plan before editing: think through the target behavior, inspect existing patterns, and state the smallest intended change before coding. |
| Always | Verify before declaring success: use focused checks while iterating, then run the repo-required checks when the task is complete. |
| Always | Use sandboxed HOME/config paths for dotfiles behavior; never validate by writing to the operator's real home config. |
| Ask first | Stop when the safe path is unclear, the scope would broaden, or an action could mutate real user configuration. |

## Delegation

For non-trivial work, load the delegation skill when available. Use it for Delegation Preflight, safe slice selection, skip reasons, and final reporting. Keep external project state in the main agent.`) + "\n"
}

func RulesFingerprint() string {
	return rulesFingerprint(RulesContent())
}

func InspectRulesContract(content string) RulesObservation {
	openCount := strings.Count(content, dotsRulesOpen)
	closeCount := strings.Count(content, dotsRulesClose)
	if openCount == 0 && closeCount == 0 {
		return RulesObservation{Disposition: RulesNoExternalProvider}
	}
	if openCount == 0 || openCount != closeCount {
		return RulesObservation{Disposition: RulesMalformedExternalProvider}
	}

	remaining := content
	for range openCount {
		openIndex := strings.Index(remaining, dotsRulesOpen)
		closeIndex := strings.Index(remaining, dotsRulesClose)
		if openIndex < 0 || closeIndex < openIndex {
			return RulesObservation{Disposition: RulesMalformedExternalProvider}
		}
		bodyStart := openIndex + len(dotsRulesOpen)
		body := remaining[bodyStart:closeIndex]
		fingerprint := rulesFingerprint(body)
		if fingerprint != RulesFingerprint() {
			return RulesObservation{Disposition: RulesExternalDrift, Fingerprint: fingerprint}
		}
		remaining = remaining[closeIndex+len(dotsRulesClose):]
	}
	return RulesObservation{Disposition: RulesExternallySatisfied, Fingerprint: RulesFingerprint()}
}

func rulesFingerprint(content string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "## ") {
		lines[0] = "## Agent Rules"
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func RulesSectionContent() string {
	return sectionBlock(openMarker(packyRulesSectionID), closeMarker(packyRulesSectionID), RulesContent())
}

func WriteCodex(path string) (WriteResult, error) {
	existing, err := readOptionalFile(path)
	if err != nil {
		return WriteResult{}, err
	}
	result := WriteResult{Warnings: DetectExternalManagedBlocks(existing)}
	updated := upsertSection(existing, codexPackySectionID, CodexContent())
	if InspectRulesContract(existing).Disposition == RulesExternallySatisfied {
		updated = removeSection(updated, packyRulesSectionID)
	} else {
		updated = upsertSection(updated, packyRulesSectionID, RulesContent())
	}
	if updated == existing {
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return WriteResult{}, fmt.Errorf("create Codex config directory %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return WriteResult{}, fmt.Errorf("write Codex Packy prompt %s: %w", path, err)
	}
	return result, nil
}

func InspectCodex(path string) (Inspection, error) {
	existing, err := readOptionalFile(path)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{HasPackySection: strings.Contains(existing, openMarker(codexPackySectionID)) || strings.Contains(existing, openMarker(packyRulesSectionID))}, nil
}

func RemoveCodex(path string) error {
	existing, err := readOptionalFile(path)
	if err != nil {
		return err
	}
	updated := removeSection(existing, codexPackySectionID)
	updated = removeSection(updated, packyRulesSectionID)
	if updated == existing {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("remove Codex Packy prompt %s: %w", path, err)
	}
	return nil
}

func DetectExternalManagedBlocks(content string) []string {
	var warnings []string
	switch InspectRulesContract(content).Disposition {
	case RulesExternallySatisfied:
		warnings = append(warnings, "Codex baseline rules are externally satisfied by exact dots:rules; Packy preserved the external block and removed its redundant rules contribution")
	case RulesExternalDrift:
		warnings = append(warnings, "Codex dots:rules differs from the Packy baseline; Packy projected its baseline and preserved the external block; align the external provider contract before retrying")
	case RulesMalformedExternalProvider:
		warnings = append(warnings, "Codex dots:rules markers are malformed; Packy projected its baseline and preserved the external content; repair the external provider markers before retrying")
	}
	if strings.Contains(content, "<!-- gentle-ai:") || strings.Contains(content, "<!-- /gentle-ai:") {
		warnings = append(warnings, "Codex prompt contains gentle-ai managed blocks; Packy preserved them and only updated Packy markers")
	}
	if containsEngramMarker(content) {
		warnings = append(warnings, "Codex prompt contains Engram managed instructions; Packy preserved them and only updated Packy markers")
	}
	return warnings
}

func containsEngramMarker(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--") && strings.HasSuffix(trimmed, "-->") && strings.Contains(strings.ToLower(trimmed), "engram") {
			return true
		}
	}
	return false
}

func readOptionalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func upsertSection(existing, sectionID, content string) string {
	return mergeSection(existing, sectionID, content)
}

func removeSection(existing, sectionID string) string {
	return mergeSection(existing, sectionID, "")
}

func mergeSection(existing, sectionID, content string) string {
	open := openMarker(sectionID)
	close := closeMarker(sectionID)
	block := sectionBlock(open, close, content)
	var out strings.Builder
	inserted := false
	for {
		openIdx := strings.Index(existing, open)
		if openIdx < 0 {
			out.WriteString(existing)
			break
		}
		closeRelIdx := strings.Index(existing[openIdx+len(open):], close)
		if closeRelIdx < 0 {
			out.WriteString(existing[:openIdx])
			existing = existing[openIdx+len(open):]
			continue
		}

		closeEnd := openIdx + len(open) + closeRelIdx + len(close)
		out.WriteString(existing[:openIdx])
		if content != "" && !inserted {
			out.WriteString(block)
			inserted = true
		}
		existing = existing[closeEnd:]
	}
	if content != "" && !inserted {
		out.WriteString(block)
	}
	return out.String()
}

func openMarker(sectionID string) string  { return "<!-- packy:" + sectionID + " -->" }
func closeMarker(sectionID string) string { return "<!-- /packy:" + sectionID + " -->" }

func sectionBlock(open, close, content string) string {
	if content == "" {
		return ""
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return open + "\n" + content + close
}
