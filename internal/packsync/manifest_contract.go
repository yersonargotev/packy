package packsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func applyManifestContracts(root string, manifests map[string]packManifest, targetPacks map[string]bool, impacts []PackImpact, existingPacks map[string]bool, registration bool, blockers *[]string) []PackImpact {
	byPack := make(map[string]int, len(impacts))
	for i := range impacts {
		byPack[impacts[i].PackID] = i
	}
	for id, current := range manifests {
		if current.SchemaVersion != 4 || !targetPacks[id] {
			continue
		}
		baselineWire, baselineCanonical, err := readHistoricalManifestBaseline(root, id, current.Version)
		if err != nil {
			if registration && !existingPacks[id] {
				continue
			}
			*blockers = append(*blockers, fmt.Sprintf("schema-v4 pack historical same-version baseline is invalid: %s@%s: %v", id, current.Version, err))
			continue
		}
		var currentWire manifestV4Wire
		var currentRaw, baselineRaw any
		currentCanonical, err := normalizedManifestV4(current.canonicalV4, &currentWire)
		if err != nil || json.Unmarshal(currentCanonical, &currentRaw) != nil || json.Unmarshal(baselineCanonical, &baselineRaw) != nil {
			*blockers = append(*blockers, fmt.Sprintf("schema-v4 pack historical same-version baseline is invalid: %s@%s", id, current.Version))
			continue
		}
		changed := !reflect.DeepEqual(currentRaw, baselineRaw)
		index, affected := byPack[id]
		if changed && !affected {
			impacts = append(impacts, PackImpact{PackID: id, CurrentVersion: current.Version, MechanicalFloor: LevelNone, SemanticEvidenceRequired: true})
			index = len(impacts) - 1
			byPack[id] = index
			affected = true
		}
		if !affected {
			continue
		}
		floor, reasons := manifestFloor(baselineWire, currentWire)
		if changed && len(reasons) == 0 {
			reasons = []string{"manifest observable contract changed"}
		}
		raiseImpact(&impacts[index], floor, reasons...)
		impacts[index].Contract = contractEvidence(current, currentCanonical, baselineCanonical, currentWire)
	}
	for i := range impacts {
		sort.Strings(impacts[i].Reasons)
		impacts[i].Reasons = unique(impacts[i].Reasons)
	}
	sort.Slice(impacts, func(i, j int) bool { return impacts[i].PackID < impacts[j].PackID })
	return impacts
}

type manifestV4Wire struct {
	SchemaVersion  int                         `json:"schema_version"`
	ID             string                      `json:"id"`
	Version        string                      `json:"version"`
	Surfaces       []string                    `json:"surfaces"`
	Provides       []string                    `json:"provides"`
	Requires       capabilitypack.Requirements `json:"requires"`
	Conflicts      []string                    `json:"conflicts"`
	Resources      []resourceV4Wire            `json:"resources"`
	RootMigrations []migrationV4Wire           `json:"root_migrations"`
}

type resourceV4Wire struct {
	Kind                 string              `json:"kind"`
	ID                   string              `json:"id"`
	Source               string              `json:"source"`
	Command              string              `json:"command"`
	Args                 []string            `json:"args"`
	Mode                 string              `json:"mode"`
	Tools                []string            `json:"tools"`
	Permissions          []string            `json:"permissions"`
	Requires             []string            `json:"requires"`
	Conflicts            []string            `json:"conflicts"`
	Notices              []string            `json:"notices"`
	ProvidesCapabilities []string            `json:"provides_capabilities"`
	RequiresCapabilities []string            `json:"requires_capabilities"`
	RequiresTools        []string            `json:"requires_tools"`
	CapabilityConflicts  []string            `json:"capability_conflicts"`
	Bindings             []map[string]any    `json:"bindings"`
	Arguments            map[string]any      `json:"arguments"`
	SurfaceExclusions    []map[string]any    `json:"surface_exclusions"`
	RuntimeModes         []runtimeModeV4Wire `json:"runtime_modes"`
}

type runtimeModeV4Wire struct {
	ID            string           `json:"id"`
	Role          string           `json:"role"`
	Requirements  []map[string]any `json:"requirements"`
	Authorities   []map[string]any `json:"authorities"`
	Effects       []map[string]any `json:"effects"`
	Fallback      map[string]any   `json:"fallback"`
	OnUnavailable string           `json:"on_unavailable"`
}

type migrationV4Wire struct {
	From json.RawMessage `json:"from"`
	To   json.RawMessage `json:"to"`
}

func manifestFloor(before, after manifestV4Wire) (ClassificationLevel, []string) {
	floor := LevelNone
	var reasons []string
	raise := func(level ClassificationLevel, reason string) {
		if classificationRank(level) > classificationRank(floor) {
			floor = level
		}
		reasons = append(reasons, reason)
	}
	if lostStrings(before.Surfaces, after.Surfaces) {
		raise(LevelMajor, "manifest surface support removed")
	}
	if !reflect.DeepEqual(before.Provides, after.Provides) {
		raise(LevelMajor, "manifest provided capabilities changed")
	}
	if addedStrings(before.Requires.Capabilities, after.Requires.Capabilities) ||
		addedStrings(before.Requires.Tools, after.Requires.Tools) ||
		addedStrings(before.Conflicts, after.Conflicts) {
		raise(LevelMajor, "manifest mandatory dependency or conflict added")
	}
	old := resourceMap(before.Resources)
	now := resourceMap(after.Resources)
	for key, prior := range old {
		current, ok := now[key]
		if !ok {
			raise(LevelMajor, "manifest resource removed or renamed")
			continue
		}
		if addedStrings(prior.Requires, current.Requires) || addedStrings(prior.Conflicts, current.Conflicts) ||
			addedStrings(prior.RequiresCapabilities, current.RequiresCapabilities) || addedStrings(prior.RequiresTools, current.RequiresTools) ||
			addedStrings(prior.CapabilityConflicts, current.CapabilityConflicts) ||
			runtimeMandatoryAdded(prior.RuntimeModes, current.RuntimeModes) || projectionBehaviorChanged(prior, current) {
			raise(LevelMajor, "manifest mandatory graph, authority, requirement, or projection changed")
		}
	}
	for key, resource := range now {
		if _, ok := old[key]; ok {
			continue
		}
		if resourceMandatory(resource) {
			raise(LevelMajor, "manifest resource added with mandatory graph, authority, or requirement")
		} else {
			raise(LevelMinor, "isolated manifest resource added")
		}
	}
	if !reflect.DeepEqual(before, after) && len(reasons) == 0 {
		reasons = append(reasons, "manifest observable contract changed")
	}
	return floor, reasons
}

func projectionBehaviorChanged(before, after resourceV4Wire) bool {
	return before.Source != after.Source || before.Command != after.Command ||
		!reflect.DeepEqual(before.Args, after.Args) || before.Mode != after.Mode ||
		!reflect.DeepEqual(before.Tools, after.Tools) || !reflect.DeepEqual(before.Permissions, after.Permissions) ||
		!reflect.DeepEqual(before.ProvidesCapabilities, after.ProvidesCapabilities) ||
		!reflect.DeepEqual(before.Bindings, after.Bindings) || !reflect.DeepEqual(before.Arguments, after.Arguments) ||
		!reflect.DeepEqual(before.SurfaceExclusions, after.SurfaceExclusions) ||
		!reflect.DeepEqual(before.RuntimeModes, after.RuntimeModes)
}

func resourceMap(resources []resourceV4Wire) map[string]resourceV4Wire {
	result := make(map[string]resourceV4Wire, len(resources))
	for _, resource := range resources {
		result[resource.Kind+":"+resource.ID] = resource
	}
	return result
}

func resourceMandatory(r resourceV4Wire) bool {
	if len(r.Requires)+len(r.Conflicts)+len(r.RequiresCapabilities)+len(r.RequiresTools)+len(r.CapabilityConflicts) > 0 {
		return true
	}
	for _, mode := range r.RuntimeModes {
		if len(mode.Requirements)+len(mode.Authorities) > 0 {
			return true
		}
	}
	return false
}

func runtimeMandatoryAdded(before, after []runtimeModeV4Wire) bool {
	old := make(map[string]runtimeModeV4Wire, len(before))
	for _, mode := range before {
		old[mode.ID] = mode
	}
	for _, mode := range after {
		prior, ok := old[mode.ID]
		if !ok {
			if len(mode.Requirements)+len(mode.Authorities) > 0 {
				return true
			}
			continue
		}
		if canonicalItemsAdded(prior.Requirements, mode.Requirements) || canonicalItemsAdded(prior.Authorities, mode.Authorities) {
			return true
		}
	}
	return false
}

func canonicalItemsAdded(before, after []map[string]any) bool {
	old := make(map[string]bool, len(before))
	for _, item := range before {
		encoded, _ := json.Marshal(item)
		old[string(encoded)] = true
	}
	for _, item := range after {
		encoded, _ := json.Marshal(item)
		if !old[string(encoded)] {
			return true
		}
	}
	return false
}

func exclusionsAdded(before, after []map[string]any) bool {
	return canonicalItemsAdded(before, after)
}

func addedStrings(before, after []string) bool {
	set := make(map[string]bool, len(before))
	for _, value := range before {
		set[value] = true
	}
	for _, value := range after {
		if !set[value] {
			return true
		}
	}
	return false
}

func lostStrings(before, after []string) bool { return addedStrings(after, before) }

func raiseImpact(impact *PackImpact, floor ClassificationLevel, reasons ...string) {
	if classificationRank(floor) > classificationRank(impact.MechanicalFloor) {
		impact.MechanicalFloor = floor
	}
	impact.SemanticEvidenceRequired = true
	impact.Reasons = append(impact.Reasons, reasons...)
}

func contractEvidence(current packManifest, currentCanonical, baseline []byte, wire manifestV4Wire) *ManifestContractEvidence {
	evidence := &ManifestContractEvidence{
		SchemaVersion: 4, CurrentVersion: current.Version,
		CurrentManifestSHA256: hashBytes(currentCanonical), BaselineManifestSHA256: hashBytes(baseline),
		RootMigrations: []MigrationIdentity{}, NoticeAssociations: []NoticeAssociation{},
	}
	for _, migration := range wire.RootMigrations {
		evidence.RootMigrations = append(evidence.RootMigrations, MigrationIdentity{
			From: canonicalMigrationIdentity(migration.From),
			To:   canonicalMigrationIdentity(migration.To),
		})
	}
	for _, resource := range wire.Resources {
		for _, notice := range resource.Notices {
			evidence.NoticeAssociations = append(evidence.NoticeAssociations, NoticeAssociation{Resource: resource.Kind + ":" + resource.ID, Notice: notice})
		}
	}
	sort.Slice(evidence.RootMigrations, func(i, j int) bool {
		return evidence.RootMigrations[i].From+evidence.RootMigrations[i].To < evidence.RootMigrations[j].From+evidence.RootMigrations[j].To
	})
	sort.Slice(evidence.NoticeAssociations, func(i, j int) bool {
		return evidence.NoticeAssociations[i].Resource+"\x00"+evidence.NoticeAssociations[i].Notice < evidence.NoticeAssociations[j].Resource+"\x00"+evidence.NoticeAssociations[j].Notice
	})
	return evidence
}

type historicalManifestArtifact struct {
	SchemaVersion   int             `json:"schema_version"`
	PackID          string          `json:"pack_id"`
	PackVersion     string          `json:"pack_version"`
	Manifest        FileEvidence    `json:"manifest"`
	Resources       json.RawMessage `json:"resources"`
	AggregateSHA256 string          `json:"aggregate_sha256"`
}

func readHistoricalManifestBaseline(root, packID, version string) (manifestV4Wire, []byte, error) {
	historyRoot := filepath.Join(root, "bundle", "history", packID, version)
	artifactBytes, err := os.ReadFile(filepath.Join(historyRoot, "artifact.json"))
	if err != nil {
		return manifestV4Wire{}, nil, fmt.Errorf("read artifact: %w", err)
	}
	var artifact historicalManifestArtifact
	if err := strictManifestJSON(artifactBytes, &artifact); err != nil {
		return manifestV4Wire{}, nil, fmt.Errorf("decode artifact: %w", err)
	}
	if artifact.SchemaVersion != 1 || artifact.PackID != packID || artifact.PackVersion != version ||
		artifact.Manifest.Path != "pack.json" || artifact.Manifest.Mode != 0o644 || artifact.Manifest.SHA256 == "" {
		return manifestV4Wire{}, nil, errors.New("artifact manifest identity is invalid")
	}
	manifestPath := filepath.Join(historyRoot, artifact.Manifest.Path)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifestV4Wire{}, nil, fmt.Errorf("read manifest: %w", err)
	}
	info, err := os.Stat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != artifact.Manifest.Mode {
		return manifestV4Wire{}, nil, errors.New("artifact manifest size, mode, or sha256 is invalid")
	}
	sealedBytes := data
	if int64(len(sealedBytes)) != artifact.Manifest.Size || hashBytes(sealedBytes) != artifact.Manifest.SHA256 {
		sealedBytes, err = historicalSealedManifestBytes(data)
		if err != nil || int64(len(sealedBytes)) != artifact.Manifest.Size || hashBytes(sealedBytes) != artifact.Manifest.SHA256 {
			return manifestV4Wire{}, nil, errors.New("artifact manifest size, mode, or sha256 is invalid")
		}
	}
	var wire manifestV4Wire
	canonical, err := normalizedManifestV4(data, &wire)
	if err != nil || wire.SchemaVersion != 4 || wire.ID != packID || wire.Version != version {
		return manifestV4Wire{}, nil, errors.New("historical manifest identity is invalid")
	}
	return wire, canonical, nil
}

// Historical manifests received required empty-array normalizations without
// rewriting their immutable artifact seals. Reconstruct only that legacy sealed
// representation; any non-empty or unrelated change remains hash-incompatible.
func historicalSealedManifestBytes(data []byte) ([]byte, error) {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if migrations, ok := value["root_migrations"].([]any); ok && len(migrations) == 0 {
		delete(value, "root_migrations")
	}
	if resources, ok := value["resources"].([]any); ok {
		for _, raw := range resources {
			resource, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("historical manifest resource is malformed")
			}
			if notices, ok := resource["notices"].([]any); ok && len(notices) == 0 {
				delete(resource, "notices")
			}
		}
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func normalizedManifestV4(data []byte, wire *manifestV4Wire) ([]byte, error) {
	var value map[string]any
	if err := strictManifestJSON(data, &value); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	normalizeArray := func(object map[string]any, key string) {
		if object[key] == nil {
			object[key] = []any{}
		}
	}
	for _, key := range []string{"surfaces", "provides", "conflicts", "resources", "root_migrations"} {
		normalizeArray(value, key)
	}
	if requirements, ok := value["requires"].(map[string]any); ok {
		normalizeArray(requirements, "capabilities")
		normalizeArray(requirements, "tools")
	}
	if resources, ok := value["resources"].([]any); ok {
		for _, raw := range resources {
			resource, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("historical manifest resource is malformed")
			}
			for _, key := range []string{"args", "tools", "permissions", "requires", "conflicts", "notices", "provides_capabilities", "requires_capabilities", "requires_tools", "capability_conflicts", "bindings", "surface_exclusions", "runtime_modes"} {
				normalizeArray(resource, key)
			}
			if modes, ok := resource["runtime_modes"].([]any); ok {
				for _, rawMode := range modes {
					mode, ok := rawMode.(map[string]any)
					if !ok {
						return nil, errors.New("historical manifest runtime mode is malformed")
					}
					normalizeArray(mode, "requirements")
					normalizeArray(mode, "authorities")
					normalizeArray(mode, "effects")
				}
			}
		}
	}
	if contract, ok := value["contract"].(map[string]any); ok {
		normalizeArray(contract, "exclusions")
	}
	return json.Marshal(value)
}

func strictManifestJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func canonicalMigrationIdentity(raw json.RawMessage) string {
	var object struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if json.Unmarshal(raw, &object) == nil && object.Kind != "" && object.ID != "" {
		return object.Kind + ":" + object.ID
	}
	var encoded string
	_ = json.Unmarshal(raw, &encoded)
	return encoded
}
