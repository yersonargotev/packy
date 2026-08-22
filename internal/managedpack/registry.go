package managedpack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Registration assigns one Pack identity to one public Managed Pack Project.
type Registration struct {
	PackID  string `json:"pack_id"`
	Project string `json:"project"`
}

// Registry is Packy's reviewed Managed Pack Registry.
type Registry struct {
	SchemaVersion int            `json:"schema_version"`
	Packs         []Registration `json:"packs"`
}

// LoadRegistry reads and validates one reviewed registry document.
func LoadRegistry(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, fmt.Errorf("read Managed Pack Registry: %w", err)
	}
	return DecodeRegistry(data)
}

// DecodeRegistry strictly decodes one reviewed registry document.
func DecodeRegistry(data []byte) (Registry, error) {
	var registry Registry
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, fmt.Errorf("decode Managed Pack Registry: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Registry{}, fmt.Errorf("decode Managed Pack Registry: %w", err)
	}
	if registry.SchemaVersion != SchemaVersion {
		return Registry{}, fmt.Errorf("Managed Pack Registry schema_version must be %d", SchemaVersion)
	}
	if registry.Packs == nil || len(registry.Packs) == 0 {
		return Registry{}, fmt.Errorf("Managed Pack Registry packs must be a non-empty array")
	}
	projects := map[string]string{}
	for i, registration := range registry.Packs {
		if !idPattern.MatchString(registration.PackID) {
			return Registry{}, fmt.Errorf("Managed Pack Registry pack_id %q must be lowercase kebab-case", registration.PackID)
		}
		if i > 0 && registry.Packs[i-1].PackID >= registration.PackID {
			return Registry{}, fmt.Errorf("Managed Pack Registry packs must be sorted by pack_id without duplicates")
		}
		if !repositoryPattern.MatchString(registration.Project) {
			return Registry{}, fmt.Errorf("Managed Pack Registry project %q must be an owner/name identity", registration.Project)
		}
		key := strings.ToLower(registration.Project)
		if owner, exists := projects[key]; exists {
			return Registry{}, fmt.Errorf("Managed Pack Project %q already owns Pack %q", registration.Project, owner)
		}
		projects[key] = registration.PackID
	}
	return registry, nil
}
