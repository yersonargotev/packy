package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packclassification"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func TestClassifyBundlePersistsClassifierTrace(t *testing.T) {
	repository, provider, request := bundleClassificationFixture(t)
	requestPath := filepath.Join(t.TempDir(), "request.json")
	raw, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, requestPath, string(raw)+"\n")

	previousSource, previousValidator := workflowSourceFactory, workflowValidatorFactory
	workflowSourceFactory = func() packsync.Source { return provider }
	workflowValidatorFactory = func() phaseValidator { return toolValidator{} }
	t.Cleanup(func() {
		workflowSourceFactory, workflowValidatorFactory = previousSource, previousValidator
	})
	inspection := t.TempDir()
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", repository, "--request", requestPath, "--output", inspection}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var plan packsync.CompositePlan
	if err := readJSON(filepath.Join(inspection, "plan.json"), &plan); err != nil {
		t.Fatal(err)
	}

	previousAttempt := bundleClassificationAttempt
	bundleClassificationAttempt = func(_ context.Context, classificationRequest packclassification.Request) (packsync.ClassificationEvidence, []packsyncworkflow.ClassifierTrace, error) {
		evidence := packsync.ClassificationEvidence{
			PackID: classificationRequest.PackID, Classifier: packsync.ClassifierIdentity{Type: packsync.ClassifierAI, ID: "fixture-v3"},
			Rationale: "initial composite generation", CurrentVersion: "0.0.0", ProposedVersion: plan.ProposedVersion,
			ChangedAspects: []string{"initial complete composite generation"}, MechanicalFloor: packsync.LevelMajor, FinalLevel: packsync.LevelMajor,
			Migration: "no predecessor", RequiredActions: []string{"review initial contract"},
		}
		trace := packsyncworkflow.ClassifierTrace{PackID: classificationRequest.PackID, Model: "fixture-v3", PromptSHA256: strings.Repeat("a", 64), CanonicalInputSHA256: strings.Repeat("b", 64), StructuredOutputSHA256: strings.Repeat("c", 64)}
		return evidence, []packsyncworkflow.ClassifierTrace{trace}, nil
	}
	t.Cleanup(func() { bundleClassificationAttempt = previousAttempt })

	classification := t.TempDir()
	if err := run(context.Background(), []string{"--phase", "classify", "--repository-root", repository, "--request", requestPath, "--plan", filepath.Join(inspection, "plan.json"), "--output", classification}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var artifact packsyncworkflow.BundleClassificationArtifact
	if err := readJSON(filepath.Join(classification, "classification.json"), &artifact); err != nil || artifact.Validate() != nil || artifact.PlanID != plan.PlanID {
		t.Fatalf("classification artifact=%#v err=%v", artifact, err)
	}
	var traces []packsyncworkflow.ClassifierTrace
	if err := readJSON(filepath.Join(classification, "classifier-trace.json"), &traces); err != nil || len(traces) != 1 || traces[0].Model != "fixture-v3" || traces[0].PromptSHA256 != strings.Repeat("a", 64) || traces[0].CanonicalInputSHA256 != strings.Repeat("b", 64) || traces[0].StructuredOutputSHA256 != strings.Repeat("c", 64) {
		t.Fatalf("v3 classifier traces=%#v err=%v", traces, err)
	}
}

type bundleToolSource struct {
	roots      map[string]string
	candidates map[string]packsync.Candidate
}

func (source *bundleToolSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return nil, errors.New("bundle fixture uses exact commits")
}

func (source *bundleToolSource) ResolveRelease(context.Context, packsync.SourceConfig, packsync.Release) (packsync.Candidate, error) {
	return packsync.Candidate{}, errors.New("bundle fixture uses exact commits")
}

func (source *bundleToolSource) ResolveCommit(_ context.Context, config packsync.SourceConfig, sha string) (packsync.Candidate, error) {
	candidate, ok := source.candidates[config.Repository]
	if !ok || candidate.Commit != sha {
		return packsync.Candidate{}, errors.New("unexpected bundle candidate")
	}
	return candidate, nil
}

func (source *bundleToolSource) WithSnapshot(_ context.Context, candidate packsync.Candidate, root string, visit func(string) error) error {
	target := filepath.Join(root, "snapshot")
	if err := copyToolTree(source.roots[candidate.Repository], target); err != nil {
		return err
	}
	err := visit(target)
	cleanupErr := os.RemoveAll(target)
	if err != nil {
		return err
	}
	return cleanupErr
}

func bundleClassificationFixture(t *testing.T) (string, *bundleToolSource, packsyncworkflow.BundleDispatchRequest) {
	t.Helper()
	repository := t.TempDir()
	existingBinding := packsync.Binding{PackID: "existing", Kind: "skill", ResourceID: "existing", UpstreamPath: "existing", VendoredPath: "bundle/skills/existing"}
	writeToolFile(t, filepath.Join(repository, "bundle", "sources.json"), `{"schema_version":1,"sources":[{"id":"existing-source","provider":"github","repository":"example/existing","selector":{"mode":"stable-release"},"resources":[{"pack_id":"existing","kind":"skill","resource_id":"existing","upstream_path":"existing"}]}]}`)
	writeToolFile(t, filepath.Join(repository, "bundle", "packs", "existing", "pack.json"), `{"schema_version":1,"id":"existing","version":"1.0.0","resources":[{"kind":"skill","id":"existing","source":"skills/existing"}]}`)
	writeToolFile(t, filepath.Join(repository, "bundle", "skills", "existing", "SKILL.md"), "existing\n")
	existingLock := packsync.Lock{SchemaVersion: 1, SourceID: "existing-source", Resources: []packsync.ResourceEvidence{{Binding: existingBinding, Files: []packsync.FileEvidence{}}}}
	lockBytes, _, err := packsync.CanonicalSourceLock(existingLock)
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, filepath.Join(repository, "bundle", "sources", "existing-source.lock.json"), string(lockBytes))

	provider := &bundleToolSource{roots: map[string]string{}, candidates: map[string]packsync.Candidate{}}
	registrations := make([]packsyncworkflow.BundleRegistration, 0, 2)
	for index, sourceID := range []string{"source-a", "source-b"} {
		repositoryID := "example/" + sourceID
		root := t.TempDir()
		writeToolFile(t, filepath.Join(root, "skill", "SKILL.md"), "# "+sourceID+"\n")
		writeToolFile(t, filepath.Join(root, "LICENSE"), "MIT\n")
		candidate := toolCandidate()
		candidate.Repository = repositoryID
		candidate.RepositoryID = int64(10 + index)
		candidate.RepositoryNodeID = "repo-" + sourceID
		candidate.RepositoryHTML = "https://github.com/" + repositoryID
		candidate.RepositoryClone = candidate.RepositoryHTML + ".git"
		candidate.RepositoryAPI = "https://api.github.com/repos/" + repositoryID
		candidate.Commit = strings.Repeat(string(rune('1'+index)), 40)
		candidate.CommitNodeID = "commit-" + sourceID
		candidate.Tree = strings.Repeat(string(rune('3'+index)), 40)
		candidate.Release, candidate.TagObjects, candidate.TagRefName, candidate.TagRefType, candidate.TagRefSHA = nil, nil, "", "", ""
		provider.roots[repositoryID] = root
		provider.candidates[repositoryID] = candidate
		evidenceReference := filepath.ToSlash(filepath.Join("docs", "evidence", sourceID+".json"))
		evidence := toolLegalEvidence(t, evidenceReference, candidate)
		writeToolFile(t, filepath.Join(repository, filepath.FromSlash(evidenceReference)), string(evidence))
		evidenceDigest := sha256.Sum256(evidence)
		resourceSuffix := string(rune('a' + index))
		registration := packsync.SourceConfig{
			ID: sourceID, Provider: "github", Repository: repositoryID, Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: candidate.Commit},
			Resources: []packsync.Binding{
				{PackID: "composite", Kind: "notice", ResourceID: "mit-" + resourceSuffix, UpstreamPath: "LICENSE"},
				{PackID: "composite", Kind: "skill", ResourceID: "skill-" + resourceSuffix, UpstreamPath: "skill"},
			},
		}
		registrations = append(registrations, packsyncworkflow.BundleRegistration{Registration: registration, LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: evidenceReference, EvidenceSHA256: fmt.Sprintf("%x", evidenceDigest), Disposition: packsync.RedistributableDisposition}})
	}
	initializeToolRepository(t, repository)

	manifest, err := packsync.CanonicalCompositePackManifest(json.RawMessage(`{"schema_version":4,"id":"composite","version":"1.0.0","description":"fixture","surfaces":["codex"],"provides":[],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"notice","id":"mit-a","source":"notices/mit-a","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]},{"kind":"notice","id":"mit-b","source":"notices/mit-b","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]},{"kind":"skill","id":"skill-a","source":"skills/skill-a","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]},{"kind":"skill","id":"skill-b","source":"skills/skill-b","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(manifest)
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationBundleSHA256("composite", registrations)
	if err != nil {
		t.Fatal(err)
	}
	request := packsyncworkflow.BundleDispatchRequest{
		SchemaVersion: 3, Operation: packsyncworkflow.OperationRegisterBundle, PackID: "composite", ProposedVersion: "1.0.0",
		ProposedManifest: manifest, ProposedManifestSHA256: fmt.Sprintf("%x", manifestDigest), Registrations: registrations,
		RegistrationBundleSHA256: registrationDigest, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "exercise v3 classifier traces",
	}
	return repository, provider, request
}
