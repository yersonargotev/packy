package capabilitypack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"unicode"
)

const AgentPluginsFormat = "agent-plugins"

// PluginExports is independent of native bindings and on-demand compatibility.
// An empty set deliberately declares no eligible formats.
type PluginExports []PluginExport

type PluginExport struct {
	Source        string            `json:"source,omitempty"`
	Origin        *ResourceOrigin   `json:"origin,omitempty"`
	Format        string            `json:"format"`
	Name          string            `json:"name"`
	Prerequisites []string          `json:"prerequisites"`
	References    []PluginReference `json:"references"`
	Roles         []PluginRole      `json:"roles"`
}

// PluginReference replaces only its exact authored {{packy.export:<id>}} marker.
// Invocation syntax is supplied by the reviewer, never inferred from the format.
type PluginReference struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Value  string `json:"value,omitempty"`
}

type PluginRole struct {
	Resource    string `json:"resource"`
	Resolution  string `json:"resolution"`
	Description string `json:"description"`
}

func (contracts *PluginExports) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("plugin_exports must not be null")
	}
	type plain PluginExports
	var value plain
	if err := strictDecode(data, &value); err != nil {
		return err
	}
	*contracts = PluginExports(value)
	return nil
}

func clonePluginExports(contracts PluginExports) PluginExports {
	result := slices.Clone(contracts)
	for i := range result {
		if result[i].Origin != nil {
			origin := *result[i].Origin
			result[i].Origin = &origin
		}
		result[i].Prerequisites = slices.Clone(result[i].Prerequisites)
		result[i].References = slices.Clone(result[i].References)
		result[i].Roles = slices.Clone(result[i].Roles)
	}
	return result
}

func pluginContract(resource Resource, format string) *PluginExport {
	for _, contract := range resource.PluginExports {
		if contract.Format == format {
			return &contract
		}
	}
	return nil
}

func validatePluginExports(resource Resource) error {
	if resource.PluginExports != nil && resource.Kind != "skill" {
		return fmt.Errorf("plugin_exports is only applicable to skills")
	}
	for i, contract := range resource.PluginExports {
		if contract.Source != "" && !validPluginPath(contract.Source) {
			return fmt.Errorf("plugin export source must be a portable Pack-relative path")
		}
		if contract.Source != "" && resource.Origin != nil && contract.Origin == nil {
			return fmt.Errorf("plugin adaptation of imported source requires explicit origin")
		}
		if contract.Origin != nil && contract.Source == "" {
			return fmt.Errorf("plugin export origin requires an explicit source")
		}
		if contract.Format != AgentPluginsFormat {
			return fmt.Errorf("unsupported plugin export format %q", contract.Format)
		}
		if i > 0 && resource.PluginExports[i-1].Format >= contract.Format {
			return fmt.Errorf("plugin_exports must be sorted by format without duplicates")
		}
		if !idPattern.MatchString(contract.Name) || len(contract.Name) > 64 {
			return fmt.Errorf("plugin export name must be lowercase kebab-case, at most 64 characters")
		}
		if contract.Prerequisites == nil || !sortedPortableSet(contract.Prerequisites, nonemptyPluginDeclaration) {
			return fmt.Errorf("plugin export prerequisites must be a non-null sorted set of nonempty declarations")
		}
		if contract.References == nil || contract.Roles == nil {
			return fmt.Errorf("plugin export references and roles must be non-null arrays")
		}
		for j, ref := range contract.References {
			if !idPattern.MatchString(ref.ID) || j > 0 && contract.References[j-1].ID >= ref.ID {
				return fmt.Errorf("plugin references must have sorted unique kebab-case ids")
			}
			if !validPluginPath(ref.File) {
				return fmt.Errorf("plugin reference file must be a portable skill-relative path")
			}
			target, err := ParseResourceIdentity(ref.Target)
			if err != nil || target.Kind != "skill" && target.Kind != "asset" && target.Kind != "notice" {
				return fmt.Errorf("invalid plugin reference target %q", ref.Target)
			}
			if ref.Target != "skill:"+resource.ID && !slices.Contains(resource.Requires, ref.Target) && !slices.Contains(resource.Notices, ref.Target) {
				return fmt.Errorf("plugin reference %s target %s needs a direct native dependency or notice edge", ref.ID, ref.Target)
			}
			switch ref.Kind {
			case "path":
				if ref.Value != "" {
					return fmt.Errorf("plugin path reference forbids value")
				}
			case "name":
				if target.Kind != "skill" || ref.Value != "" {
					return fmt.Errorf("plugin name reference requires a skill and forbids value")
				}
			case "invocation":
				if target.Kind != "skill" || !nonemptyPluginDeclaration(ref.Value) || strings.Contains(ref.Value, "{{packy.") {
					return fmt.Errorf("plugin invocation reference requires a skill and explicit reviewed value")
				}
			default:
				return fmt.Errorf("unsupported plugin reference kind %q", ref.Kind)
			}
		}
		for j, role := range contract.Roles {
			identity, err := ParseResourceIdentity(role.Resource)
			if err != nil || identity.Kind != "agent" || j > 0 && contract.Roles[j-1].Resource >= role.Resource {
				return fmt.Errorf("plugin roles must be sorted unique agent identities")
			}
			if role.Resolution != "external" && role.Resolution != "adapted" || !nonemptyPluginDeclaration(role.Description) {
				return fmt.Errorf("plugin role requires reviewed external or adapted resolution and description")
			}
			if role.Resolution == "adapted" && contract.Source == "" {
				return fmt.Errorf("adapted plugin role requires an explicit reviewed plugin source")
			}
			if !slices.Contains(resource.Requires, role.Resource) {
				return fmt.Errorf("plugin role %s needs a direct native dependency", role.Resource)
			}
		}
	}
	return nil
}

func nonemptyPluginDeclaration(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

// PluginExportBodies returns additional reviewed source trees, retaining their
// owning effective resource's native dependencies and legal obligations.
func PluginExportBodies(resource Resource) []Resource {
	result := []Resource{}
	for _, body := range DeclaredResourceBodies(resource) {
		for _, contract := range body.PluginExports {
			if contract.Source == "" {
				continue
			}
			copy := body
			copy.Source, copy.Origin = contract.Source, contract.Origin
			copy.PluginExports = nil
			copy.OnDemand = nil
			result = append(result, copy)
		}
	}
	return result
}

func (contract *PluginExport) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if containsVariantNull(raw) {
		return fmt.Errorf("plugin export fields must not be null")
	}
	type plain PluginExport
	var decoded plain
	if err := strictDecode(data, &decoded); err != nil {
		return err
	}
	if fields, ok := raw.(map[string]any); ok {
		if _, present := fields["source"]; present && decoded.Source == "" {
			return fmt.Errorf("plugin export source must not be empty")
		}
	}
	*contract = PluginExport(decoded)
	return nil
}

func (reference *PluginReference) UnmarshalJSON(data []byte) error {
	type plain PluginReference
	var decoded plain
	if err := strictDecode(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, present := fields["value"]; present && decoded.Kind != "invocation" {
		return fmt.Errorf("only plugin invocation references accept value")
	}
	*reference = PluginReference(decoded)
	return nil
}

// Reviewed supporting filenames may contain spaces. Only normalized contained
// paths are accepted; contract identifiers keep their separate kebab-case rules.
func validPluginPath(value string) bool {
	return value != "." && fs.ValidPath(value) && !strings.ContainsAny(value, "\\:") && strings.IndexFunc(value, unicode.IsControl) < 0
}
