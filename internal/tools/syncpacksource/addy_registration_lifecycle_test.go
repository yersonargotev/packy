package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/addyacceptance"
	"github.com/yersonargotev/packy/internal/packsync"
)

func removeConfiguredAddyForTracer(t *testing.T, bundleRoot string, fixture addyacceptance.Fixture) {
	t.Helper()
	configPath := filepath.Join(bundleRoot, "sources.json")
	var config packsync.Config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	sources := config.Sources[:0]
	for _, source := range config.Sources {
		if source.ID != "addy" {
			sources = append(sources, source)
		}
	}
	config.Sources = sources
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(bundleRoot, "sources", "addy.lock.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(bundleRoot, "history", "addy")); err != nil {
		t.Fatal(err)
	}
	for _, resource := range fixture.Manifest.Resources {
		if err := os.RemoveAll(filepath.Join(bundleRoot, filepath.FromSlash(resource.Source))); err != nil {
			t.Fatal(err)
		}
	}
}

type exactAddySource struct {
	candidate    packsync.Candidate
	acquisitions int
	executions   int
}

func (source *exactAddySource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return []packsync.Release{*source.candidate.Release}, nil
}
func (source *exactAddySource) ResolveRelease(_ context.Context, _ packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	candidate := source.candidate
	candidate.Release = &release
	return candidate, nil
}
func (source *exactAddySource) ResolveCommit(_ context.Context, _ packsync.SourceConfig, sha string) (packsync.Candidate, error) {
	if sha != source.candidate.Commit {
		return packsync.Candidate{}, errors.New("unexpected Addy commit")
	}
	return source.candidate, nil
}
func (source *exactAddySource) WithSnapshot(_ context.Context, _ packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	source.acquisitions++
	snapshot := filepath.Join(temporaryRoot, "exact-addy")
	if err := addyacceptance.WriteExactAcquisition(snapshot); err != nil {
		return err
	}
	err := visit(snapshot)
	cleanup := os.RemoveAll(snapshot)
	if err != nil {
		return err
	}
	return cleanup
}

type exactAddyValidator struct {
	bundleCalls, suiteCalls int
}

func (validator *exactAddyValidator) ValidateBundle(ctx context.Context, repositoryRoot, bundleRoot string) error {
	validator.bundleCalls++
	if err := validateExactAddyResult(bundleRoot); err != nil {
		return err
	}
	return (contentValidator{}).ValidateBundle(ctx, repositoryRoot, bundleRoot)
}
func (validator *exactAddyValidator) Validate(ctx context.Context, repositoryRoot string) error {
	validator.suiteCalls++
	if err := validateExactAddyResult(filepath.Join(repositoryRoot, "bundle")); err != nil {
		return err
	}
	return (contentValidator{}).Validate(ctx, repositoryRoot)
}
func validateExactAddyResult(bundleRoot string) error {
	var manifest addyacceptance.Manifest
	data, err := os.ReadFile(filepath.Join(bundleRoot, "packs", "addy", "pack.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.ID != "addy" || manifest.Version != addyacceptance.PackVersion || addyResourceCounts(addyacceptance.Fixture{Manifest: manifest}) != [5]int{24, 4, 8, 7, 1} {
		return errors.New("Packy suite rejected incomplete Addy result")
	}
	return nil
}

func exactHumanEvidence(t *testing.T, plan packsync.Plan) packsync.ClassificationEvidenceSet {
	t.Helper()
	inspectionID, err := packsync.HumanInspectionID(plan)
	if err != nil {
		t.Fatal(err)
	}
	set := packsync.ClassificationEvidenceSet{SchemaVersion: 1, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, Candidate: plan.Candidate, HumanInspectionID: inspectionID}
	for _, impact := range plan.AffectedPacks {
		evidence := packsync.ClassificationEvidence{PackID: impact.PackID, Classifier: packsync.ClassifierIdentity{Type: packsync.ClassifierHuman, ID: "maintainer"}, Rationale: "Exact Addy 0.6.4 inventory is admitted as the complete 1.0.0 introduction.", CurrentVersion: impact.CurrentVersion, ProposedVersion: addyacceptance.PackVersion, ChangedAspects: []string{"complete Addy workflow pack introduction"}, MechanicalFloor: impact.MechanicalFloor, FinalLevel: packsync.LevelMajor, Migration: "Introduce the previously absent Addy pack.", RequiredActions: []string{"Review both projected surfaces before activation."}}
		set.Evidence = append(set.Evidence, evidence)
	}
	return set
}

func exactAddyCandidate(fixture addyacceptance.Fixture) packsync.Candidate {
	verifiedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	signature, payload := strings.Repeat("a", 64), strings.Repeat("b", 64)
	return packsync.Candidate{Repository: fixture.Provenance.Repository, RepositoryID: fixture.Provenance.RepositoryID, RepositoryNodeID: "R_kgDONPcQgA", RepositoryHTML: "https://github.com/" + fixture.Provenance.Repository, RepositoryClone: "https://github.com/" + fixture.Provenance.Repository + ".git", RepositoryAPI: "https://api.github.com/repos/" + fixture.Provenance.Repository, Visibility: "public", Owner: "addyosmani", OwnerID: fixture.Provenance.OwnerID, OwnerNodeID: "MDQ6VXNlcjExMDk1Mw==", Public: true, Release: &packsync.Release{ID: 1, NodeID: "RE_addy064", Tag: fixture.Provenance.Release, Name: fixture.Provenance.Release, Target: "main", CreatedAt: verifiedAt, PublishedAt: verifiedAt, Author: packsync.Actor{Login: "addyosmani", ID: fixture.Provenance.OwnerID, NodeID: "MDQ6VXNlcjExMDk1Mw=="}}, TagRefName: "refs/tags/" + fixture.Provenance.Release, TagRefType: "tag", TagRefSHA: fixture.Provenance.TagSHA, TagObjects: []packsync.TagObject{{SHA: fixture.Provenance.TagSHA, Name: fixture.Provenance.Release, TargetSHA: fixture.Provenance.Commit, TargetType: "commit", Verification: packsync.Verification{Reason: fixture.Provenance.TagVerification.Reason}}}, Commit: fixture.Provenance.Commit, CommitNodeID: "C_addy064", Tree: fixture.Provenance.Tree, Parents: append([]string(nil), fixture.Provenance.CommitParents...), CommitVerify: packsync.Verification{Verified: true, Reason: fixture.Provenance.CommitVerification.Reason, VerifiedAt: &verifiedAt, SignatureSHA256: &signature, PayloadSHA256: &payload}, ArchiveSHA256: fixture.Provenance.ArchiveSHA256}
}

func addyResourceCounts(fixture addyacceptance.Fixture) [5]int {
	var counts [5]int
	for _, resource := range fixture.Manifest.Resources {
		switch resource.Kind {
		case "skill":
			counts[0]++
		case "agent":
			counts[1]++
		case "command":
			counts[2]++
		case "asset":
			counts[3]++
		case "notice":
			counts[4]++
		}
	}
	return counts
}

func addySurfaceBindingCounts(fixture addyacceptance.Fixture) [2]int {
	var counts [2]int
	for _, resource := range fixture.Manifest.Resources {
		for _, binding := range resource.Bindings {
			switch binding.Surface {
			case "codex":
				counts[0]++
			case "opencode":
				counts[1]++
			}
		}
	}
	return counts
}

func assertSecretFreeArtifacts(t *testing.T, root string) {
	t.Helper()
	forbidden := [][]byte{[]byte("GITHUB_TOKEN"), []byte("Authorization: Bearer"), addyacceptance.ExactArchive()}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, needle := range forbidden {
			if len(needle) != 0 && bytes.Contains(data, needle) {
				return errors.New("artifact contains secret or upstream archive bytes")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
