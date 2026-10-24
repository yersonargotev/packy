package capabilitypack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/yersonargotev/packy/internal/skilltemplate"
)

// OnDemandDependency names reviewed instructions without native file ownership.
type OnDemandDependency struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

// OnDemandSkill is a reviewed file-tree compatibility contract. Prerequisites
// are declarations, never runtime evidence. Access edges do not grant ownership.
type OnDemandSkill struct {
	Dependencies   []OnDemandDependency `json:"dependencies,omitempty"`
	EntryDocument  string               `json:"entry_document,omitempty"`
	ReferenceFiles []string             `json:"reference_files,omitempty"`
	Protocol       string               `json:"protocol"`
	Prerequisites  []string             `json:"prerequisites"`
}

// DecodeOnDemandSkill preserves the distinction between an omitted contract
// and an explicitly invalid null declaration at both manifest boundaries.
func DecodeOnDemandSkill(data json.RawMessage) (*OnDemandSkill, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("on_demand must be an object, not null")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("decode on_demand: %w", err)
	}
	for _, name := range []string{"entry_document", "reference_files", "dependencies"} {
		if raw, ok := fields[name]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("on_demand %s must not be null", name)
		}
	}
	var value OnDemandSkill
	if err := strictDecode(data, &value); err != nil {
		return nil, fmt.Errorf("decode on_demand: %w", err)
	}
	return &value, nil
}

func cloneOnDemand(value *OnDemandSkill) *OnDemandSkill {
	if value == nil {
		return nil
	}
	result := *value
	result.Dependencies = slices.Clone(value.Dependencies)
	result.Prerequisites = slices.Clone(value.Prerequisites)
	result.ReferenceFiles = slices.Clone(value.ReferenceFiles)
	return &result
}

func validateOnDemand(resource Resource) error {
	contract := resource.OnDemand
	if contract == nil {
		return nil
	}
	if resource.Kind != "skill" {
		return fmt.Errorf("on_demand is only applicable to skill resources")
	}
	if contract.Protocol != "skill-tree-v1" {
		return fmt.Errorf("unsupported on_demand protocol %q; expected skill-tree-v1", contract.Protocol)
	}
	if contract.Prerequisites == nil || !sortedPortableSet(contract.Prerequisites, func(value string) bool {
		return strings.TrimSpace(value) == value && value != ""
	}) {
		return fmt.Errorf("on_demand prerequisites must be a non-null sorted set of nonempty declarations")
	}
	if contract.EntryDocument != "" && !validAccessPath(contract.EntryDocument) {
		return fmt.Errorf("on_demand entry_document must be a portable skill-relative file path")
	}
	if !sortedPortableSet(contract.ReferenceFiles, func(value string) bool { return validAccessPath(value) && strings.HasSuffix(value, ".md") }) {
		return fmt.Errorf("on_demand reference_files must be sorted unique portable skill-relative Markdown file paths")
	}

	for i, dependency := range contract.Dependencies {
		if dependency.Type != "skill" || !ValidQualifiedSkill(dependency.Target) {
			return fmt.Errorf("on_demand dependencies require type skill and a qualified <pack>:<skill> target; received %q %q", dependency.Type, dependency.Target)
		}
		if i > 0 && contract.Dependencies[i-1].Target >= dependency.Target {
			return fmt.Errorf("on_demand dependencies must be sorted unique qualified skill targets")
		}
	}
	return nil
}

// ValidQualifiedSkill recognizes the public Pack-qualified skill identity.
func ValidQualifiedSkill(value string) bool {
	parts := strings.Split(value, ":")
	return len(parts) == 2 && idPattern.MatchString(parts[0]) && idPattern.MatchString(parts[1])
}

func validAccessPath(value string) bool {
	return !strings.ContainsRune(value, 0) && value != "" && value != "." && path.Clean(value) == value && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\\:") && strings.IndexFunc(value, unicode.IsSpace) < 0
}

// AccessEntry identifies the reviewed standalone instruction document.
func (contract OnDemandSkill) AccessEntry() string {
	if contract.EntryDocument != "" {
		return contract.EntryDocument
	}
	return "SKILL.md"
}

// RenderAccessReferences changes only authored access tokens in explicitly
// reviewed files. Targets are effective, supported qualified entry documents.
func RenderAccessReferences(packID string, resource Resource, relative string, data []byte, targets map[string]string) ([]byte, error) {
	if resource.OnDemand == nil || !slices.Contains(resource.OnDemand.ReferenceFiles, relative) {
		return data, nil
	}
	references, err := skilltemplate.ReferencedAccessSkills(data)
	if err != nil {
		return nil, err
	}
	result := string(data)
	for _, id := range references {
		qualified := id
		if !strings.Contains(id, ":") {
			qualified = packID + ":" + id
		}
		target, ok := targets[qualified]
		if !ok {
			return nil, fmt.Errorf("access reference to absent, excluded, or incompatible skill %q", id)
		}
		declared := qualified == packID+":"+resource.ID || strings.HasPrefix(qualified, packID+":") && slices.Contains(resource.Requires, "skill:"+strings.TrimPrefix(qualified, packID+":"))
		for _, dependency := range resource.OnDemand.Dependencies {
			declared = declared || dependency.Target == qualified
		}
		if !declared {
			return nil, fmt.Errorf("access reference to skill %q requires a direct requires or on_demand dependency", id)
		}
		from := path.Dir(path.Join("packs", packID, resource.Source, relative))
		// Paths include their owning Pack; keep links portable across prepared roots.
		link, err := filepath.Rel(filepath.FromSlash(from), filepath.FromSlash(target))
		if err != nil {
			return nil, err
		}
		segments := strings.Split(filepath.ToSlash(link), "/")
		for i, segment := range segments {
			segments[i] = url.PathEscape(segment)
		}
		result = strings.ReplaceAll(result, "{{packy.access:"+id+"}}", strings.Join(segments, "/"))
	}
	return []byte(result), nil
}
