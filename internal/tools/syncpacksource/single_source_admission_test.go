package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func TestInspectRoutesV23RegisterToSingleSourceAdmission(t *testing.T) {
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

	snapshot := t.TempDir()
	writeToolFile(t, filepath.Join(snapshot, "skill", "SKILL.md"), "# Coordinate\n")
	writeToolFile(t, filepath.Join(snapshot, "LICENSE"), "MIT\n")
	candidate := toolCandidate()
	evidenceReference := "docs/evidence/admission.json"
	evidence := toolLegalEvidence(t, evidenceReference, candidate)
	writeToolFile(t, filepath.Join(repository, filepath.FromSlash(evidenceReference)), string(evidence))
	registration := packsync.SourceConfig{
		ID: "new-source", Provider: "github", Repository: candidate.Repository,
		Selector: packsync.Selector{Mode: packsync.SelectorStableRelease},
		Resources: []packsync.Binding{
			{PackID: "new-pack", Kind: "notice", ResourceID: "mit", UpstreamPath: "LICENSE"},
			{PackID: "new-pack", Kind: "skill", ResourceID: "coordinate", UpstreamPath: "skill"},
		},
	}
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}
	manifest, manifestDigest, err := packsyncworkflow.CanonicalProposedManifest(json.RawMessage(`{"id":"new-pack","version":"1.0.0","description":"Coordinate work","selectable":true,"surfaces":["codex"],"external_requirements":[],"resources":[{"kind":"notice","id":"mit","source":"notices/mit","license":"MIT","attribution":"Example","requires":[],"conflicts":[],"bindings":[],"surface_exclusions":[]},{"kind":"skill","id":"coordinate","source":"skills/coordinate","requires":[],"conflicts":[],"notices":["notice:mit"],"bindings":[{"surface":"codex","projection":"skill","name":"coordinate","invocation":"$coordinate","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]}],"exclusions":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	request := packsyncworkflow.DispatchRequest{
		SchemaVersion: 2, Operation: packsyncworkflow.OperationRegister, SourceID: registration.ID,
		Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "admit exact source",
		Registration: &registration, RegistrationSHA256: registrationDigest,
		ProposedVersion: "1.0.0", ProposedManifest: manifest, ProposedManifestSHA256: manifestDigest,
		LegalAdmission: &packsync.CompositeLegalAdmission{EvidenceReference: evidenceReference, EvidenceSHA256: fmt.Sprintf("%x", sha256.Sum256(evidence)), Disposition: packsync.RedistributableDisposition},
	}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	requestBytes, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, requestPath, string(requestBytes)+"\n")
	outputDir := t.TempDir()

	previousSource, previousValidator := workflowSourceFactory, workflowValidatorFactory
	workflowSourceFactory = func() packsync.Source { return &toolSource{root: snapshot, candidate: candidate} }
	workflowValidatorFactory = func() phaseValidator { return toolValidator{} }
	t.Cleanup(func() {
		workflowSourceFactory, workflowValidatorFactory = previousSource, previousValidator
	})
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", repository, "--request", requestPath, "--output", outputDir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var plan packsync.SingleSourceAdmissionPlan
	if err := readJSON(filepath.Join(outputDir, "plan.json"), &plan); err != nil || !plan.VerifySeal() || plan.PackID != "new-pack" {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	if _, err := os.Stat(filepath.Join(repository, "bundle", "packs", "new-pack")); !os.IsNotExist(err) {
		t.Fatalf("inspect wrote proposed Pack: %v", err)
	}
	if err := writeFailureArtifact(options{repositoryRoot: repository, requestPath: requestPath, planPath: filepath.Join(outputDir, "plan.json"), outputDir: outputDir}, fmt.Errorf("later phase blocked")); err != nil {
		t.Fatal(err)
	}
	var artifact packsyncworkflow.FailureArtifact
	if err := readJSON(filepath.Join(outputDir, "operational-artifact.json"), &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.PlanID != plan.PlanID || artifact.PackID != plan.PackID || artifact.ProposedVersion != plan.ProposedVersion ||
		artifact.ProposedManifestSHA256 != plan.ProposedManifestSHA256 || artifact.LegalEvidenceReference != plan.LegalAdmission.EvidenceReference ||
		artifact.LegalEvidenceSHA256 != plan.LegalAdmission.EvidenceSHA256 || artifact.ResultBundleSHA256 != plan.ResultBundleSHA256 {
		t.Fatalf("failure artifact lost initial-admission identity: %#v", artifact)
	}
}

type toolValidator struct{}

func (toolValidator) ValidateBundle(context.Context, string, string) error { return nil }
func (toolValidator) Validate(context.Context, string) error               { return nil }
func (toolValidator) ValidateApplied(context.Context, string) error        { return nil }

type toolSource struct {
	root      string
	candidate packsync.Candidate
}

func (source *toolSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return []packsync.Release{*source.candidate.Release}, nil
}
func (source *toolSource) ResolveRelease(_ context.Context, _ packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	candidate := source.candidate
	candidate.Release = &release
	return candidate, nil
}
func (source *toolSource) ResolveCommit(context.Context, packsync.SourceConfig, string) (packsync.Candidate, error) {
	return packsync.Candidate{}, fmt.Errorf("unexpected commit resolution")
}
func (source *toolSource) WithSnapshot(_ context.Context, _ packsync.Candidate, root string, visit func(string) error) error {
	target := filepath.Join(root, "snapshot")
	if err := copyToolTree(source.root, target); err != nil {
		return err
	}
	err := visit(target)
	cleanupErr := os.RemoveAll(target)
	if err != nil {
		return err
	}
	return cleanupErr
}

func toolCandidate() packsync.Candidate {
	when := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	signature, payload := strings.Repeat("a", 64), strings.Repeat("b", 64)
	commit, tag, tree := strings.Repeat("c", 40), strings.Repeat("d", 40), strings.Repeat("e", 40)
	return packsync.Candidate{
		Repository: "example/new-source", RepositoryID: 1, RepositoryNodeID: "repo", RepositoryHTML: "https://github.com/example/new-source", RepositoryClone: "https://github.com/example/new-source.git", RepositoryAPI: "https://api.github.com/repos/example/new-source", Visibility: "public",
		Owner: "example", OwnerID: 2, OwnerNodeID: "owner", Public: true,
		Release:    &packsync.Release{ID: 3, NodeID: "release", Tag: "v1.0.0", Name: "v1.0.0", Target: "main", CreatedAt: when, PublishedAt: when.Add(time.Minute), Author: packsync.Actor{Login: "maintainer", ID: 4, NodeID: "actor"}},
		TagRefName: "refs/tags/v1.0.0", TagRefType: "tag", TagRefSHA: tag,
		TagObjects: []packsync.TagObject{{SHA: tag, Name: "v1.0.0", TargetSHA: commit, TargetType: "commit", Verification: packsync.Verification{Reason: "unsigned"}}},
		Commit:     commit, CommitNodeID: "commit", Tree: tree, Parents: []string{strings.Repeat("f", 40)},
		CommitVerify: packsync.Verification{Verified: true, Reason: "valid", VerifiedAt: &when, SignatureSHA256: &signature, PayloadSHA256: &payload},
	}
}

type toolLegalEvidenceDocument struct {
	SchemaVersion    int                              `json:"schema_version"`
	EvidenceID       string                           `json:"evidence_id"`
	DurableReference string                           `json:"durable_reference"`
	Issuer           string                           `json:"issuer"`
	EvidenceOrigin   string                           `json:"evidence_origin"`
	Decision         string                           `json:"decision"`
	Candidate        packsync.LegalAdmissionCandidate `json:"candidate"`
	Disposition      string                           `json:"disposition"`
	Rights           []string                         `json:"rights"`
	Obligations      []string                         `json:"obligations"`
	Disclosures      []string                         `json:"disclosures"`
	Scope            packsync.LegalAdmissionScope     `json:"scope"`
	Validity         string                           `json:"validity"`
	Invalidation     string                           `json:"invalidation"`
}

func toolLegalEvidence(t *testing.T, reference string, candidate packsync.Candidate) []byte {
	t.Helper()
	document := toolLegalEvidenceDocument{SchemaVersion: 1, EvidenceID: "fixture", DurableReference: reference, Issuer: "fixture", EvidenceOrigin: "LICENSE", Decision: "admit", Candidate: packsync.LegalAdmissionCandidate{Repository: candidate.Repository, Commit: candidate.Commit, READMEBlob: strings.Repeat("a", 40), READMELength: 1, READMESHA256: strings.Repeat("b", 64)}, Disposition: packsync.RedistributableDisposition, Rights: []string{"copy"}, Obligations: []string{"notice"}, Disclosures: []string{"fixture"}, Scope: packsync.LegalAdmissionScope{SelectedRoots: []string{"LICENSE", "skill"}, Exclusions: []string{}}, Validity: "exact", Invalidation: "change"}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func writeToolFile(t *testing.T, name, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyToolTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
