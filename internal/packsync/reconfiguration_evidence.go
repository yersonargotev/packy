package packsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

type changedSelectionEvidence struct {
	SchemaVersion            int      `json:"schema_version"`
	PackID                   string   `json:"pack_id"`
	FromVersion              string   `json:"from_version"`
	ToVersion                string   `json:"to_version"`
	SourceID                 string   `json:"source_id"`
	Before                   []string `json:"before_bindings"`
	After                    []string `json:"after_bindings"`
	Added                    []string `json:"added"`
	Removed                  []string `json:"removed"`
	BeforeManifestSHA256     string   `json:"before_manifest_sha256"`
	AfterManifestSHA256      string   `json:"after_manifest_sha256"`
	PlanID                   string   `json:"plan_id"`
	BaseSHA                  string   `json:"base_sha"`
	CandidateSHA             string   `json:"candidate_sha"`
	FinalVersion             string   `json:"final_version"`
	ClaimsUpstreamProvenance bool     `json:"claims_upstream_provenance"`
	ReplacesSourceLock       bool     `json:"replaces_source_lock"`
}

func materializeChangedSelectionEvidence(staged string, plan Plan, set ClassificationEvidenceSet) error {
	if plan.Reconfiguration == nil {
		return nil
	}
	if len(plan.AffectedPacks) != 1 || len(set.Evidence) != 1 {
		return errors.New("reconfiguration requires exactly one classified affected Pack")
	}
	impact, classification := plan.AffectedPacks[0], set.Evidence[0]
	before := bindingKeysForPack(plan.PreviousBindings, impact.PackID)
	after := bindingKeysForPack(plan.Reconfiguration.Resources, impact.PackID)
	added, removed := stringSetDelta(before, after)
	evidence := changedSelectionEvidence{
		SchemaVersion: 3, PackID: impact.PackID, FromVersion: impact.CurrentVersion,
		ToVersion: classification.ProposedVersion, SourceID: plan.SourceID,
		Before: before, After: after, Added: added, Removed: removed,
		BeforeManifestSHA256: plan.PreviousManifestSHA256, AfterManifestSHA256: plan.ProposedManifestSHA256,
		PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, CandidateSHA: plan.Candidate.Commit,
		FinalVersion: classification.ProposedVersion,
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := validateChangedSelectionEvidence(data, plan, set); err != nil {
		return err
	}
	directory := filepath.Join(staged, "compatibility", impact.PackID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, impact.CurrentVersion+"-to-"+classification.ProposedVersion+".json"), data, 0o644)
}

func validateChangedSelectionEvidence(data []byte, plan Plan, set ClassificationEvidenceSet) error {
	var evidence changedSelectionEvidence
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return fmt.Errorf("changed-selection evidence is invalid: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}
	if plan.Reconfiguration == nil || len(plan.AffectedPacks) != 1 || len(set.Evidence) != 1 {
		return errors.New("changed-selection evidence contradicts the sealed operation")
	}
	impact, classification := plan.AffectedPacks[0], set.Evidence[0]
	before := bindingKeysForPack(plan.PreviousBindings, impact.PackID)
	after := bindingKeysForPack(plan.Reconfiguration.Resources, impact.PackID)
	added, removed := stringSetDelta(before, after)
	if evidence.SchemaVersion != 3 || evidence.PackID != impact.PackID || evidence.FromVersion != impact.CurrentVersion || evidence.ToVersion != classification.ProposedVersion || evidence.FinalVersion != classification.ProposedVersion || evidence.SourceID != plan.SourceID || evidence.PlanID != plan.PlanID || evidence.BaseSHA != plan.Preconditions.BaseCommit || evidence.CandidateSHA != plan.Candidate.Commit || evidence.BeforeManifestSHA256 != plan.PreviousManifestSHA256 || evidence.AfterManifestSHA256 != plan.ProposedManifestSHA256 || evidence.ClaimsUpstreamProvenance || evidence.ReplacesSourceLock || !reflect.DeepEqual(evidence.Before, before) || !reflect.DeepEqual(evidence.After, after) || !reflect.DeepEqual(evidence.Added, added) || !reflect.DeepEqual(evidence.Removed, removed) {
		return errors.New("changed-selection evidence is stale, partial, or contradictory")
	}
	return nil
}

func bindingKeysForPack(bindings []Binding, packID string) []string {
	var keys []string
	for _, binding := range bindings {
		if binding.PackID == packID {
			keys = append(keys, bindingKey(binding))
		}
	}
	sort.Strings(keys)
	return keys
}

func stringSetDelta(before, after []string) (added, removed []string) {
	old, next := map[string]bool{}, map[string]bool{}
	for _, value := range before {
		old[value] = true
	}
	for _, value := range after {
		next[value] = true
	}
	for _, value := range after {
		if !old[value] {
			added = append(added, value)
		}
	}
	for _, value := range before {
		if !next[value] {
			removed = append(removed, value)
		}
	}
	return added, removed
}

func verifyStagedProposedManifest(bundle string, plan Plan, set ClassificationEvidenceSet) error {
	if plan.Reconfiguration == nil {
		return nil
	}
	if len(plan.AffectedPacks) != 1 || len(set.Evidence) != 1 {
		return errors.New("staged reconfiguration classification is incomplete")
	}
	impact := plan.AffectedPacks[0]
	data, err := os.ReadFile(filepath.Join(bundle, "packs", impact.PackID, "pack.json"))
	if err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil || manifest["version"] != set.Evidence[0].ProposedVersion {
		return errors.New("staged proposed manifest has the wrong classified version")
	}
	manifest["version"] = impact.CurrentVersion
	canonical, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	canonical = append(canonical, '\n')
	if hashBytes(canonical) != plan.ProposedManifestSHA256 {
		return errors.New("staged proposed manifest changed from the sealed plan")
	}
	evidencePath := filepath.Join(bundle, "compatibility", impact.PackID, impact.CurrentVersion+"-to-"+set.Evidence[0].ProposedVersion+".json")
	evidence, err := os.ReadFile(evidencePath)
	if err != nil {
		return errors.New("staged changed-selection compatibility evidence is missing")
	}
	return validateChangedSelectionEvidence(evidence, plan, set)
}
