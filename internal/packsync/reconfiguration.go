package packsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

func canonicalProposedManifest(repositoryRoot string, raw json.RawMessage, source SourceConfig, current map[string]packManifest) (packManifest, []byte, error) {
	if len(raw) == 0 {
		return packManifest{}, nil, errors.New("reconfiguration requires a complete proposed manifest")
	}
	packIDs := map[string]bool{}
	for _, binding := range source.Resources {
		packIDs[binding.PackID] = true
	}
	if len(packIDs) != 1 {
		return packManifest{}, nil, errors.New("reconfiguration must own exactly one affected Pack")
	}
	var packID string
	for id := range packIDs {
		packID = id
	}
	before, ok := current[packID]
	if !ok {
		return packManifest{}, nil, fmt.Errorf("reconfiguration references unknown runtime pack: %s", packID)
	}
	manifest, canonical, err := validateSingleSourceAdmissionManifest(repositoryRoot, packID, before.Version, raw)
	if err != nil {
		return packManifest{}, nil, err
	}
	if !singleSourceBindingsMatchManifest(source.Resources, manifest.Resources) {
		return packManifest{}, nil, errors.New("proposed manifest resources must exactly match the complete proposed binding set")
	}
	if err := validateProtectedManifestMetadata(repositoryRoot, packID, canonical); err != nil {
		return packManifest{}, nil, err
	}
	return manifest, canonical, nil
}

func validateProtectedManifestMetadata(repositoryRoot, packID string, proposed []byte) error {
	current, err := os.ReadFile(filepath.Join(repositoryRoot, "bundle", "packs", packID, "pack.json"))
	if err != nil {
		return err
	}
	var before, after map[string]any
	if json.Unmarshal(current, &before) != nil || json.Unmarshal(proposed, &after) != nil {
		return errors.New("compare current and proposed Pack provenance metadata")
	}
	beforeReference, beforeHasReference := before["source_reference"].(map[string]any)
	afterReference, afterHasReference := after["source_reference"].(map[string]any)
	if beforeHasReference != afterHasReference ||
		(beforeHasReference && beforeReference["repository"] != afterReference["repository"]) {
		return errors.New("proposed manifest must preserve source reference repository")
	}
	beforeResources, err := manifestResourceObjects(before)
	if err != nil {
		return err
	}
	afterResources, err := manifestResourceObjects(after)
	if err != nil {
		return err
	}
	for identity, currentResource := range beforeResources {
		proposedResource, ok := afterResources[identity]
		if !ok {
			continue
		}
		for _, field := range []string{"source", "license", "attribution"} {
			if !reflect.DeepEqual(currentResource[field], proposedResource[field]) {
				return fmt.Errorf("proposed manifest must preserve resource %s field %s", identity, field)
			}
		}
	}
	return nil
}

func validateSelectedReleaseRevision(before, after []byte, candidate Candidate) error {
	var current, proposed map[string]any
	if json.Unmarshal(before, &current) != nil || json.Unmarshal(after, &proposed) != nil {
		return errors.New("compare Pack source reference revision")
	}
	currentReference, currentOK := current["source_reference"].(map[string]any)
	proposedReference, proposedOK := proposed["source_reference"].(map[string]any)
	if currentOK != proposedOK || !currentOK {
		return nil
	}
	if candidate.Release != nil && proposedReference["revision"] != candidate.Release.Tag {
		return errors.New("proposed source reference revision must equal the exact selected release tag")
	}
	if candidate.Release == nil && currentReference["revision"] != proposedReference["revision"] {
		return errors.New("proposed source reference revision requires a selected release tag")
	}
	return nil
}

func manifestReconfigurationFloor(before, after []byte) (ClassificationLevel, []string, error) {
	var oldManifest, newManifest map[string]any
	if err := json.Unmarshal(before, &oldManifest); err != nil {
		return LevelNone, nil, fmt.Errorf("decode current manifest transition: %w", err)
	}
	if err := json.Unmarshal(after, &newManifest); err != nil {
		return LevelNone, nil, fmt.Errorf("decode proposed manifest transition: %w", err)
	}
	originalOld, originalNew := cloneManifestMap(oldManifest), cloneManifestMap(newManifest)
	delete(oldManifest, "version")
	delete(newManifest, "version")
	delete(oldManifest, "description")
	delete(newManifest, "description")
	delete(oldManifest, "source_reference")
	delete(newManifest, "source_reference")
	oldResources, err := manifestResourceObjects(oldManifest)
	if err != nil {
		return LevelNone, nil, err
	}
	newResources, err := manifestResourceObjects(newManifest)
	if err != nil {
		return LevelNone, nil, err
	}
	delete(oldManifest, "resources")
	delete(newManifest, "resources")
	for _, resource := range oldResources {
		delete(resource, "description")
	}
	for _, resource := range newResources {
		delete(resource, "description")
	}
	floor := LevelNone
	var reasons []string
	raise := func(level ClassificationLevel, reason string) {
		if classificationRank(level) > classificationRank(floor) {
			floor = level
		}
		reasons = append(reasons, reason)
	}
	if !reflect.DeepEqual(oldManifest, newManifest) {
		raise(LevelMajor, "manifest Pack-level contract changed")
	}
	for key, beforeResource := range oldResources {
		afterResource, ok := newResources[key]
		if !ok {
			raise(LevelMajor, "manifest resource removed or renamed")
			continue
		}
		if !reflect.DeepEqual(beforeResource, afterResource) {
			raise(LevelMajor, "manifest resource projection, authority, or requirement changed")
		}
	}
	for key, resource := range newResources {
		if _, ok := oldResources[key]; ok {
			continue
		}
		if manifestResourceIsMandatory(resource) {
			raise(LevelMajor, "manifest resource added with mandatory requirement")
		} else {
			raise(LevelMinor, "isolated manifest resource added")
		}
	}
	delete(originalOld, "version")
	delete(originalNew, "version")
	if floor == LevelNone && !reflect.DeepEqual(originalOld, originalNew) {
		reasons = append(reasons, "Pack-owned descriptive metadata changed")
	}
	return floor, unique(reasons), nil
}

func cloneManifestMap(value map[string]any) map[string]any {
	encoded, _ := json.Marshal(value)
	var clone map[string]any
	_ = json.Unmarshal(encoded, &clone)
	return clone
}

func manifestResourceObjects(manifest map[string]any) (map[string]map[string]any, error) {
	resources, ok := manifest["resources"].([]any)
	if !ok {
		return nil, errors.New("manifest transition resources are incomplete")
	}
	result := make(map[string]map[string]any, len(resources))
	for _, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("manifest transition resource is invalid")
		}
		kind, kindOK := resource["kind"].(string)
		id, idOK := resource["id"].(string)
		key := kind + ":" + id
		if !kindOK || !idOK || result[key] != nil {
			return nil, errors.New("manifest transition resource identity is invalid or duplicated")
		}
		result[key] = resource
	}
	return result, nil
}

func manifestResourceIsMandatory(resource map[string]any) bool {
	for _, field := range []string{"requires", "conflicts", "requires_capabilities", "requires_tools", "capability_conflicts"} {
		if values, ok := resource[field].([]any); ok && len(values) > 0 {
			return true
		}
	}
	if modes, ok := resource["runtime_modes"].([]any); ok {
		for _, raw := range modes {
			mode, _ := raw.(map[string]any)
			for _, field := range []string{"requirements", "authorities"} {
				if values, ok := mode[field].([]any); ok && len(values) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func validateCurrentHistoricalGeneration(repositoryRoot, packID, version string, manifest packManifest, currentManifest []byte) error {
	history := filepath.Join(repositoryRoot, "bundle", "history", packID, version)
	historicalManifest, err := os.ReadFile(filepath.Join(history, "pack.json"))
	if err != nil || !bytes.Equal(historicalManifest, currentManifest) {
		return errors.New("current Pack immutable history does not match the sealed manifest")
	}
	artifactBytes, err := os.ReadFile(filepath.Join(history, "artifact.json"))
	if err != nil {
		return errors.New("current Pack immutable history artifact is missing")
	}
	var artifact compositeHistoricalArtifact
	decoder := json.NewDecoder(bytes.NewReader(artifactBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil || ensureEOF(decoder) != nil || artifact.SchemaVersion != 1 || artifact.PackID != packID || artifact.PackVersion != version || len(artifact.Resources) != len(manifest.Resources) {
		return errors.New("current Pack immutable history artifact is invalid")
	}
	manifestFiles, err := inventory(filepath.Join(history, "pack.json"))
	if err != nil || len(manifestFiles) != 1 {
		return errors.New("inspect current historical manifest")
	}
	manifestFiles[0].Path = "pack.json"
	if !reflect.DeepEqual(artifact.Manifest, manifestFiles[0]) {
		return errors.New("current historical manifest evidence is invalid")
	}
	for i, resource := range manifest.Resources {
		evidence := artifact.Resources[i]
		if evidence.Kind != resource.Kind || evidence.ID != resource.ID || evidence.Source != resource.Source {
			return errors.New("current historical resource identity is invalid")
		}
		files := []FileEvidence{}
		if resource.Source != "" {
			files, err = inventory(filepath.Join(history, filepath.FromSlash(resource.Source)))
			if err != nil {
				return fmt.Errorf("inspect current historical resource: %w", err)
			}
			for j := range files {
				relative, relErr := filepath.Rel(history, filepath.Join(history, filepath.FromSlash(resource.Source), filepath.FromSlash(files[j].Path)))
				if relErr != nil {
					return relErr
				}
				files[j].Path = filepath.ToSlash(relative)
			}
		}
		if !reflect.DeepEqual(evidence.Files, files) || evidence.SHA256 != resourceHash(files) {
			return errors.New("current historical resource evidence is invalid")
		}
	}
	if artifact.AggregateSHA256 == "" || artifact.AggregateSHA256 != compositeHistoricalAggregate(artifact) {
		return errors.New("current historical artifact aggregate seal is invalid")
	}
	return nil
}
