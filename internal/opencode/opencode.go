package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteResult reports non-fatal conditions Matty noticed while preserving user
// OpenCode config.
type WriteResult struct {
	Warnings []string
}

func promptContent() string {
	return strings.TrimSpace(`## Matty global workflow

- Global skills live in ~/.agents/skills. When a task matches a skill, read that skill's SKILL.md before acting.
- Use ask-matt at ~/.agents/skills/ask-matt as the router when you are unsure which skill or workflow applies.
- Use Engram memory tools when available: search before past-work or project-sensitive tasks; save decisions, discoveries, bug fixes, and conventions; summarize sessions before finishing.
- Apply host delegation rules when this OpenCode session exposes subagent/delegation tools. If unavailable, proceed inline and mention that delegation was unavailable.`) + "\n"
}

func Write(configPath, promptPath string) (WriteResult, error) {
	existing, err := readOptionalFile(configPath)
	if err != nil {
		return WriteResult{}, err
	}
	result := WriteResult{Warnings: detectExternalManagedConfig(existing)}
	config, err := mergeInstruction(existing, configPath, promptPath)
	if err != nil {
		return WriteResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o700); err != nil {
		return WriteResult{}, fmt.Errorf("create OpenCode config directory %s: %w", filepath.Dir(promptPath), err)
	}
	if err := os.WriteFile(promptPath, []byte(promptContent()), 0o600); err != nil {
		return WriteResult{}, fmt.Errorf("write OpenCode Matty prompt %s: %w", promptPath, err)
	}
	if config != existing {
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			return WriteResult{}, fmt.Errorf("write OpenCode config %s: %w", configPath, err)
		}
	}
	return result, nil
}

func Remove(configPath, promptPath string) error {
	existing, err := readOptionalFile(configPath)
	if err != nil {
		return err
	}
	config, err := removeInstruction(existing, configPath, promptPath)
	if err != nil {
		return err
	}
	if config != existing {
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			return fmt.Errorf("remove OpenCode Matty config %s: %w", configPath, err)
		}
	}
	if err := os.Remove(promptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove OpenCode Matty prompt %s: %w", promptPath, err)
	}
	return nil
}

func detectExternalManagedConfig(content string) []string {
	if strings.Contains(content, "gentle-ai") {
		return []string{"OpenCode config contains gentle-ai references; Matty preserved them and only updated Matty instruction entries"}
	}
	return nil
}

func mergeInstruction(existing, configPath, promptPath string) (string, error) {
	return updateInstructions(existing, configPath, promptPath, true)
}

func removeInstruction(existing, configPath, promptPath string) (string, error) {
	return updateInstructions(existing, configPath, promptPath, false)
}

func updateInstructions(existing, configPath, promptPath string, add bool) (string, error) {
	config := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		jsonData, err := jsoncToJSON(existing)
		if err != nil {
			return "", fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
		}
		if err := json.Unmarshal(jsonData, &config); err != nil {
			return "", fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
		}
	}

	instructions, err := instructionStrings(config["instructions"])
	if err != nil {
		return "", err
	}
	instructions = removeString(instructions, promptPath)
	if add {
		instructions = append(instructions, promptPath)
	}
	if len(instructions) == 0 {
		delete(config, "instructions")
	} else {
		config["instructions"] = instructions
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode OpenCode config: %w", err)
	}
	return string(append(data, '\n')), nil
}

func jsoncToJSON(content string) ([]byte, error) {
	withoutComments, err := stripJSONCComments(content)
	if err != nil {
		return nil, err
	}
	return []byte(stripTrailingCommas(withoutComments)), nil
}

func stripJSONCComments(content string) (string, error) {
	var out strings.Builder
	inString := false
	escaped := false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
			out.WriteByte(ch)
		case '/':
			if i+1 >= len(content) {
				out.WriteByte(ch)
				continue
			}
			next := content[i+1]
			switch next {
			case '/':
				i += 2
				for i < len(content) && content[i] != '\n' && content[i] != '\r' {
					i++
				}
				if i < len(content) {
					out.WriteByte(content[i])
				}
			case '*':
				i += 2
				closed := false
				for i+1 < len(content) {
					if content[i] == '*' && content[i+1] == '/' {
						closed = true
						i++
						break
					}
					if content[i] == '\n' || content[i] == '\r' {
						out.WriteByte(content[i])
					}
					i++
				}
				if !closed {
					return "", fmt.Errorf("unterminated block comment")
				}
			default:
				out.WriteByte(ch)
			}
		default:
			out.WriteByte(ch)
		}
	}
	if inString {
		return "", fmt.Errorf("unterminated string")
	}
	return out.String(), nil
}

func stripTrailingCommas(content string) string {
	var out strings.Builder
	inString := false
	escaped := false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(content) && isJSONWhitespace(content[j]) {
				j++
			}
			if j < len(content) && (content[j] == '}' || content[j] == ']') {
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func isJSONWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
}

func instructionStrings(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("read OpenCode config: instructions must be an array of strings")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("read OpenCode config: instructions must be an array of strings")
		}
		out = append(out, s)
	}
	return out, nil
}

func removeString(items []string, value string) []string {
	out := items[:0]
	for _, item := range items {
		if item != value {
			out = append(out, item)
		}
	}
	return out
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
