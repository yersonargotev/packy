package claudecode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type claudeAgentSource struct {
	Name, Description string
	Body              []byte
}

func decodeClaudeAgentSource(source []byte) (claudeAgentSource, error) {
	if !utf8.Valid(source) {
		return claudeAgentSource{}, errors.New("Claude agent source is not valid UTF-8")
	}
	if !bytes.HasPrefix(source, []byte("---\n")) {
		return claudeAgentSource{}, errors.New("Claude agent source must start with frontmatter")
	}
	end := bytes.Index(source[4:], []byte("\n---\n"))
	if end < 0 {
		return claudeAgentSource{}, errors.New("Claude agent source has unterminated frontmatter")
	}
	end += 4
	header := source[4:end]
	body := append([]byte(nil), source[end+5:]...)
	values := map[string]string{}
	for _, line := range strings.Split(string(header), "\n") {
		key, raw, ok := strings.Cut(line, ":")
		if !ok || key == "" || strings.TrimSpace(key) != key || strings.TrimSpace(raw) == "" {
			return claudeAgentSource{}, fmt.Errorf("malformed Claude agent frontmatter line %q", line)
		}
		if key != "name" && key != "description" {
			return claudeAgentSource{}, fmt.Errorf("unknown Claude agent frontmatter key %q", key)
		}
		if _, duplicate := values[key]; duplicate {
			return claudeAgentSource{}, fmt.Errorf("duplicate Claude agent frontmatter key %q", key)
		}
		value, err := decodeClaudeAgentScalar(strings.TrimSpace(raw))
		if err != nil {
			return claudeAgentSource{}, fmt.Errorf("decode Claude agent %s: %w", key, err)
		}
		values[key] = value
	}
	if values["name"] == "" || values["description"] == "" || len(values) != 2 {
		return claudeAgentSource{}, errors.New("Claude agent frontmatter requires exactly name and description")
	}
	return claudeAgentSource{Name: values["name"], Description: values["description"], Body: body}, nil
}

func decodeClaudeAgentScalar(raw string) (string, error) {
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return "", err
		}
		if value == "" {
			return "", errors.New("empty value")
		}
		return value, nil
	}
	if strings.ContainsAny(raw, "[]{}&*!|>'\"#\t\r") || strings.TrimSpace(raw) != raw {
		return "", errors.New("unsupported YAML scalar")
	}
	return raw, nil
}

func renderClaudeAgentDocument(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, source []byte) ([]byte, error) {
	capability, ok := resource.SurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilityClaudeAgentDocument)
	if !ok || capability.ClaudeAgentDocument == nil {
		return nil, fmt.Errorf("Claude agent %s does not declare an agent document capability", resource.ID)
	}
	decoded, err := decodeClaudeAgentSource(source)
	if err != nil {
		return nil, fmt.Errorf("decode Claude agent %s: %w", resource.ID, err)
	}
	if decoded.Name != resource.ID {
		return nil, fmt.Errorf("Claude agent source name %q does not match portable name %q", decoded.Name, resource.ID)
	}
	dependencies, err := claudeAgentSkillDependencies(pack, resource, capability.ClaudeAgentDocument.Skills)
	if err != nil {
		return nil, err
	}
	authority := capability.ClaudeAgentDocument.Authority
	if authority.PermissionMode != "default" {
		return nil, fmt.Errorf("Claude Claude agent %s requires permissionMode default", resource.ID)
	}
	toolSet := map[string]bool{}
	records := append([]capabilitypack.AuthorityRecord(nil), authority.Authorities...)
	sort.Slice(records, func(i, j int) bool { return records[i].Portable < records[j].Portable })
	contract := make([]string, 0, len(records))
	for _, record := range records {
		for _, tool := range record.ClaudeTools {
			toolSet[tool] = true
		}
		declarations, tools := strings.Join(record.Declarations, ", "), strings.Join(record.ClaudeTools, ", ")
		if declarations == "" {
			declarations = "none"
		}
		if tools == "" {
			tools = "none"
		}
		contract = append(contract, fmt.Sprintf("- %s: declarations=[%s]; outcome=%s; claude_tools=[%s]; fallback=%s", record.Portable, declarations, record.Outcome, tools, record.Fallback))
	}
	tools := make([]string, 0, len(toolSet))
	for tool := range toolSet {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	var out bytes.Buffer
	fmt.Fprintf(&out, "---\nname: %s\ndescription: %s\npermissionMode: default\ntools: %s\nskills:\n", binding.Name, yamlScalar(decoded.Description), strings.Join(tools, ", "))
	for _, dependency := range dependencies {
		fmt.Fprintf(&out, "  - %s\n", dependency)
	}
	fmt.Fprintf(&out, "---\n\n## Packy authority contract\n\n- permission_mode: default\n%s\n", strings.Join(contract, "\n"))
	out.Write(decoded.Body)
	return out.Bytes(), nil
}

func claudeAgentSkillDependencies(pack capabilitypack.Pack, agent capabilitypack.Resource, requested []capabilitypack.ResourceIdentity) ([]string, error) {
	result := make([]string, 0, len(requested))
	for _, dependency := range requested {
		identity := dependency.Kind + ":" + dependency.ID
		required := false
		for _, requirement := range agent.Requires {
			required = required || requirement == identity
		}
		if !required {
			return nil, fmt.Errorf("Claude agent %s does not require declared skill %s", agent.ID, identity)
		}
		resource, err := uniqueResource(pack, dependency.Kind, dependency.ID)
		if err != nil {
			return nil, err
		}
		matches := []string{}
		for _, candidate := range resource.Bindings {
			if candidate.Surface == capabilitypack.SurfaceClaude && candidate.Projection == "skill" && candidate.Name != "" {
				matches = append(matches, candidate.Name)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("Claude agent dependency %s has no unique effective Claude skill binding", identity)
		}
		result = append(result, matches[0])
	}
	return result, nil
}
