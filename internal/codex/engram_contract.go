package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

const (
	engramInstructionsFingerprint = "74176fb0847b06fb725ae8992c9a5fa12022ff347ca3ee2ef3e77c6d318d5fb3"
	engramCompactFingerprint      = "c779d9584c8ca16331ebb31a753f7fbb5bcb8193b229572a54da189ffaa97fd1"
)

func hasEngramCodexSetupResources(pack capabilitypack.Pack) bool {
	hasInstruction, hasMCP := false, false
	for _, resource := range pack.Resources {
		hasInstruction = hasInstruction || resource.Kind == "instruction" && resource.ID == "engram-memory"
		hasMCP = hasMCP || resource.Kind == "mcp_server" && resource.ID == "engram"
	}
	return hasInstruction && hasMCP
}

func isEngramOwnedResource(resource capabilitypack.Resource) bool {
	return resource.Kind == "instruction" && resource.ID == "engram-memory" || resource.Kind == "mcp_server" && resource.ID == "engram"
}

func (a *SurfaceAdapter) inspectEngramContract(config string, resolutions []capabilitypack.ExecutableResolution) ([]capabilitypack.ObservedProjection, error) {
	dir := filepath.Dir(a.configFile)
	instructionsPath := filepath.Join(dir, "engram-instructions.md")
	compactPath := filepath.Join(dir, "engram-compact-prompt.md")
	instructions, instructionsExist, err := readOptionalFileWithExistence(instructionsPath)
	if err != nil {
		return nil, err
	}
	compact, compactExists, err := readOptionalFileWithExistence(compactPath)
	if err != nil {
		return nil, err
	}
	command := capabilitypack.ResolvedExecutablePath("engram", resolutions)
	checks := []struct {
		id, target string
		kind       capabilitypack.ProjectionActionKind
		valid      bool
		present    bool
		exact      string
	}{
		{id: "mcp", target: a.configFile, kind: capabilitypack.ActionCodexMCPConfig, valid: tomlSectionHas(config, "mcp_servers.engram", map[string]string{"command": command, "args": `["mcp", "--tools=agent"]`}), present: tomlSectionExists(config, "mcp_servers.engram"), exact: tomlSectionContent(config, "mcp_servers.engram")},
		{id: "instructions-config", target: a.configFile, kind: capabilitypack.ActionCodexMCPConfig, valid: tomlSectionHas(config, "", map[string]string{"model_instructions_file": instructionsPath}), present: tomlTopLevelKeyExists(config, "model_instructions_file"), exact: tomlTopLevelValue(config, "model_instructions_file")},
		{id: "instructions-file", target: instructionsPath, kind: capabilitypack.ActionCodexAssetFile, valid: instructionsExist && localprojection.FingerprintBytes([]byte(instructions)) == engramInstructionsFingerprint, present: instructionsExist, exact: instructions},
		{id: "compact-config", target: a.configFile, kind: capabilitypack.ActionCodexMCPConfig, valid: tomlSectionHas(config, "", map[string]string{"experimental_compact_prompt_file": compactPath}), present: tomlTopLevelKeyExists(config, "experimental_compact_prompt_file"), exact: tomlTopLevelValue(config, "experimental_compact_prompt_file")},
		{id: "compact-file", target: compactPath, kind: capabilitypack.ActionCodexAssetFile, valid: compactExists && localprojection.FingerprintBytes([]byte(compact)) == engramCompactFingerprint, present: compactExists, exact: compact},
		{id: "marketplace", target: a.configFile, kind: capabilitypack.ActionCodexMCPConfig, valid: tomlSectionHas(config, "marketplaces.engram", map[string]string{"source_type": "git", "source": "https://github.com/Gentleman-Programming/engram.git", "ref": "main"}), present: tomlSectionExists(config, "marketplaces.engram"), exact: tomlSectionContent(config, "marketplaces.engram")},
		{id: "plugin", target: a.configFile, kind: capabilitypack.ActionCodexMCPConfig, valid: tomlSectionHas(config, `plugins."engram@engram"`, map[string]string{"enabled": "true"}), present: tomlSectionExists(config, `plugins."engram@engram"`), exact: tomlSectionContent(config, `plugins."engram@engram"`)},
	}
	result := make([]capabilitypack.ObservedProjection, 0, len(checks))
	for _, check := range checks {
		id := "external_setup:engram:codex:" + check.id
		desired := localprojection.FingerprintBytes([]byte("engram-codex-contract-v1:" + check.id))
		exact := "missing"
		if check.present {
			exact = localprojection.FingerprintBytes([]byte(check.exact))
		}
		observed := exact
		if check.valid {
			observed = desired
		}
		result = append(result, capabilitypack.ObservedProjection{
			ID: id, Exists: check.present, ObservedFingerprint: observed, ExactFingerprint: exact, DesiredFingerprint: desired, ExternallyManaged: true,
			Action: capabilitypack.ProjectionAction{ID: id, Kind: check.kind, Target: check.target, Description: fmt.Sprintf("observe Engram-owned Codex %s configuration", check.id)},
		})
	}
	return result, nil
}

func readOptionalFileWithExistence(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), true, nil
}

func tomlSectionExists(content, section string) bool {
	return tomlSectionContent(content, section) != ""
}

func tomlSectionContent(content, section string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start := -1
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "["+section+"]" {
			start = i
			continue
		}
		if start >= 0 && strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			return strings.TrimSpace(strings.Join(lines[start:i], "\n"))
		}
	}
	if start >= 0 {
		return strings.TrimSpace(strings.Join(lines[start:], "\n"))
	}
	return ""
}

func tomlTopLevelKeyExists(content, key string) bool { return tomlTopLevelValue(content, key) != "" }

func tomlTopLevelValue(content, key string) string {
	current := ""
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = line
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if current == "" && ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func removeTOMLSection(content, section string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start, end := -1, len(lines)
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "["+section+"]" {
			start = i
			continue
		}
		if start >= 0 && strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			end = i
			break
		}
	}
	if start < 0 {
		return content
	}
	updated := append(append([]string(nil), lines[:start]...), lines[end:]...)
	return strings.TrimSpace(strings.Join(updated, "\n")) + "\n"
}

func removeTOMLTopLevelKey(content, key string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	current := ""
	kept := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = line
		}
		name, _, ok := strings.Cut(line, "=")
		if current == "" && ok && strings.TrimSpace(name) == key {
			continue
		}
		kept = append(kept, raw)
	}
	return strings.TrimSpace(strings.Join(kept, "\n")) + "\n"
}

func tomlSectionHas(content, section string, expected map[string]string) bool {
	found := map[string]string{}
	current := ""
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			continue
		}
		if current != section {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			found[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	for key, value := range expected {
		if found[key] != value {
			return false
		}
	}
	return true
}
