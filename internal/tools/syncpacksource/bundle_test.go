package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/packclassification"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope(t *testing.T) {
	root := repositoryRootForTest(t)
	base := t.TempDir()
	copyTreeForTest(t, filepath.Join(root, "bundle"), filepath.Join(base, "bundle"))
	first, second := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(first, "skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "skill", "SKILL.md"), []byte("composite skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "notice.md"), []byte("composite notice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var lock packsync.Lock
	readJSONForTest(t, filepath.Join(base, "bundle", "sources", "mattpocock-skills.lock.json"), &lock)
	candidateFor := func(commit string) packsync.Candidate {
		candidate := lock.Candidate
		candidate.Commit, candidate.Tree = commit, strings.Repeat(commit[:1], 40)
		candidate.CommitNodeID, candidate.Release, candidate.TagRefName, candidate.TagRefSHA, candidate.TagObjects = "node-"+commit[:1], nil, "", "", nil
		return candidate
	}
	shaA, shaB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	source := &compositeAdapterSource{
		roots: map[string]string{shaA: first, shaB: second},
		candidates: map[string]packsync.Candidate{
			"source-a": candidateFor(shaA), "source-b": candidateFor(shaB),
		},
		resolves: map[string]int{},
	}
	manifestBytes := []byte("{\n  \"schema_version\": 1,\n  \"id\": \"adapter-composite\",\n  \"version\": \"1.0.0\",\n  \"resources\": [\n    {\n      \"kind\": \"skill\",\n      \"id\": \"one\",\n      \"source\": \"skills/one\"\n    },\n    {\n      \"kind\": \"reference\",\n      \"id\": \"two\",\n      \"source\": \"references/two.md\"\n    }\n  ]\n}\n")
	manifestBytes, err := packsync.CanonicalCompositePackManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	members := []packsyncworkflow.BundleRegistration{
		{Registration: packsync.SourceConfig{ID: "source-a", Provider: "github", Repository: lock.Candidate.Repository, Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: shaA}, Resources: []packsync.Binding{{PackID: "adapter-composite", Kind: "skill", ResourceID: "one", UpstreamPath: "skill"}}}, LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "evidence/a.json", Disposition: packsync.RedistributableDisposition}},
		{Registration: packsync.SourceConfig{ID: "source-b", Provider: "github", Repository: lock.Candidate.Repository, Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: shaB}, Resources: []packsync.Binding{{PackID: "adapter-composite", Kind: "reference", ResourceID: "two", UpstreamPath: "notice.md"}}}, LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "evidence/b.json", Disposition: packsync.RedistributableDisposition}},
	}
	for i := range members {
		members[i].LegalAdmission.EvidenceSHA256 = writeAdapterLegalEvidence(t, base, members[i])
	}
	digest, err := packsyncworkflow.CanonicalRegistrationBundleSHA256("adapter-composite", members)
	if err != nil {
		t.Fatal(err)
	}
	encodedMembers, _ := json.Marshal(members)
	gitForTest(t, base, "init", "-q")
	gitForTest(t, base, "config", "user.name", "fixture")
	gitForTest(t, base, "config", "user.email", "fixture@example.com")
	gitForTest(t, base, "add", ".")
	gitForTest(t, base, "commit", "-qm", "base")
	baseSHA := strings.TrimSpace(gitForTest(t, base, "rev-parse", "HEAD"))
	validateRepo, publishRepo := filepath.Join(t.TempDir(), "validate"), filepath.Join(t.TempDir(), "publish")
	gitForTest(t, filepath.Dir(validateRepo), "clone", "-q", base, validateRepo)
	gitForTest(t, filepath.Dir(publishRepo), "clone", "-q", base, publishRepo)

	oldSource, oldValidator, oldGateway, oldClassifier := workflowSourceFactory, workflowValidatorFactory, workflowGatewayFactory, bundleClassificationAttempt
	workflowSourceFactory = func() packsync.Source { return source }
	validator := &sandboxValidator{}
	workflowValidatorFactory = func() phaseValidator { return validator }
	fakeGitHub := &fakeGitHubCommands{baseHead: baseSHA, sourceID: "adapter-composite"}
	workflowGatewayFactory = func(repositoryRoot string, plan packsync.Plan) *githubGateway {
		return &githubGateway{repositoryRoot: repositoryRoot, repository: "owner/repo", plan: plan, retry: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Nanosecond, Sleeper: noWaitSleeper{}}, run: fakeGitHub.run}
	}
	bundleClassificationAttempt = func(_ context.Context, request packclassification.Request) (packsync.ClassificationEvidence, error) {
		return packsync.ClassificationEvidence{PackID: request.PackID, Classifier: packsync.ClassifierIdentity{Type: packsync.ClassifierAI, ID: "synthetic"}, Rationale: "initial composite admission", CurrentVersion: "0.0.0", ProposedVersion: "1.0.0", ChangedAspects: []string{"initial complete Pack generation"}, MechanicalFloor: packsync.LevelMajor, FinalLevel: packsync.LevelMajor, Migration: "no predecessor", RequiredActions: []string{"review"}}, nil
	}
	t.Cleanup(func() {
		workflowSourceFactory, workflowValidatorFactory, workflowGatewayFactory, bundleClassificationAttempt = oldSource, oldValidator, oldGateway, oldClassifier
	})
	for key, value := range map[string]string{
		"GITHUB_REPOSITORY": "owner/repo", "GITHUB_ACTOR": "maintainer", "GITHUB_RUN_ID": "37", "GITHUB_RUN_ATTEMPT": "1", "GITHUB_SERVER_URL": "https://github.com",
		"PACKY_OPERATION": "register_bundle", "PACKY_PACK_ID": "adapter-composite", "PACKY_PROPOSED_VERSION": "1.0.0",
		"PACKY_PROPOSED_MANIFEST_JSON": string(manifestBytes), "PACKY_PROPOSED_MANIFEST_SHA256": sha256Text(string(manifestBytes)),
		"PACKY_REGISTRATIONS_JSON": string(encodedMembers), "PACKY_REGISTRATION_BUNDLE_SHA256": digest, "PACKY_CLASSIFICATION_MODE": "ai", "PACKY_REQUEST_REASON": "synthetic tracer",
	} {
		t.Setenv(key, value)
	}
	artifacts := t.TempDir()
	inspectDir := filepath.Join(artifacts, "inspect")
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", base, "--output", inspectDir}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	classifyDir := filepath.Join(artifacts, "classify")
	if err := run(context.Background(), []string{"--phase", "classify", "--repository-root", base, "--request", filepath.Join(inspectDir, "request.json"), "--plan", filepath.Join(inspectDir, "plan.json"), "--output", classifyDir}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	validateDir := filepath.Join(artifacts, "validate")
	if err := run(context.Background(), []string{"--phase", "validate", "--repository-root", validateRepo, "--request", filepath.Join(inspectDir, "request.json"), "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", filepath.Join(classifyDir, "classification-evidence.json"), "--output", validateDir}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	publishDir := filepath.Join(artifacts, "publish")
	if err := run(context.Background(), []string{"--phase", "publish", "--repository-root", publishRepo, "--request", filepath.Join(inspectDir, "request.json"), "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", filepath.Join(classifyDir, "classification-evidence.json"), "--validation", filepath.Join(validateDir, "validation.json"), "--output", publishDir}, &bytes.Buffer{}); err != nil {
		t.Fatalf("%#v fake=%#v", err, fakeGitHub)
	}
	if source.resolves["source-a"] < 3 || source.resolves["source-b"] < 3 {
		t.Fatalf("each phase did not independently reacquire all members: %#v", source.resolves)
	}
	var publication packsyncworkflow.BundlePublicationArtifact
	readJSONForTest(t, filepath.Join(publishDir, "publication.json"), &publication)
	if !publication.DecisionReady || publication.BranchName != "sync/adapter-composite" || publication.ResultTreeSHA == "" || fakeGitHub.createCalls != 1 {
		t.Fatalf("composite publication = %#v, creates=%d", publication, fakeGitHub.createCalls)
	}
	if fakeGitHub.pr == nil || !strings.Contains(fakeGitHub.pr.body, `"schema_version": 3`) ||
		!strings.Contains(fakeGitHub.pr.body, `"pack_id": "adapter-composite"`) ||
		strings.Contains(fakeGitHub.pr.body, `"operation": "synchronize"`) {
		t.Fatalf("composite PR evidence is not v3-native: %#v", fakeGitHub.pr)
	}

	evidencePath := filepath.Join(classifyDir, "classification-evidence.json")
	artifactPath := filepath.Join(classifyDir, "classification.json")
	originalEvidence, _ := os.ReadFile(evidencePath)
	originalArtifact, _ := os.ReadFile(artifactPath)
	assertRejectedWithoutWrite := func(name string, invoke func(string) error, want string) {
		t.Helper()
		repository := filepath.Join(t.TempDir(), name)
		gitForTest(t, filepath.Dir(repository), "clone", "-q", base, repository)
		before := strings.TrimSpace(gitForTest(t, repository, "rev-parse", "HEAD^{tree}"))
		err := invoke(repository)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s error = %v, want %q", name, err, want)
		}
		after := strings.TrimSpace(gitForTest(t, repository, "write-tree"))
		status := strings.TrimSpace(gitForTest(t, repository, "status", "--porcelain"))
		if before != after || status != "" {
			t.Fatalf("%s changed repository tree before rejection", name)
		}
	}
	if err := os.WriteFile(evidencePath, append(originalEvidence, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejectedWithoutWrite("stale-digest", func(repository string) error {
		return run(context.Background(), []string{"--phase", "validate", "--repository-root", repository, "--request", filepath.Join(inspectDir, "request.json"), "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", evidencePath, "--output", filepath.Join(artifacts, "stale-output")}, &bytes.Buffer{})
	}, "digest")
	if err := os.WriteFile(evidencePath, originalEvidence, 0o600); err != nil {
		t.Fatal(err)
	}
	var mixed packsyncworkflow.BundleClassificationArtifact
	if err := json.Unmarshal(originalArtifact, &mixed); err != nil {
		t.Fatal(err)
	}
	mixed.Members[1].CandidateSHA = mixed.Members[0].CandidateSHA
	if err := writeCanonical(artifactPath, mixed); err != nil {
		t.Fatal(err)
	}
	assertRejectedWithoutWrite("mixed-member", func(repository string) error {
		return run(context.Background(), []string{"--phase", "validate", "--repository-root", repository, "--request", filepath.Join(inspectDir, "request.json"), "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", evidencePath, "--output", filepath.Join(artifacts, "mixed-output")}, &bytes.Buffer{})
	}, "stale or mixed")
	if err := os.WriteFile(artifactPath, originalArtifact, 0o600); err != nil {
		t.Fatal(err)
	}
	moved := source.candidates["source-b"]
	moved.Commit = strings.Repeat("d", 40)
	source.candidates["source-b"] = moved
	pushesBefore := fakeGitHub.pushCalls
	assertRejectedWithoutWrite("moved-member", func(repository string) error {
		return run(context.Background(), []string{"--phase", "publish", "--repository-root", repository, "--request", filepath.Join(inspectDir, "request.json"), "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", evidencePath, "--validation", filepath.Join(validateDir, "validation.json"), "--output", filepath.Join(artifacts, "moved-output")}, &bytes.Buffer{})
	}, "changed after Check")
	if fakeGitHub.pushCalls != pushesBefore {
		t.Fatal("moved member reached GitHub mutation")
	}
}

type compositeAdapterLegalEvidence struct {
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

func writeAdapterLegalEvidence(t *testing.T, repository string, member packsyncworkflow.BundleRegistration) string {
	t.Helper()
	selected := make([]string, 0, len(member.Registration.Resources))
	for _, binding := range member.Registration.Resources {
		selected = append(selected, binding.UpstreamPath)
	}
	evidence := compositeAdapterLegalEvidence{
		SchemaVersion: 1, EvidenceID: "synthetic-" + member.Registration.ID,
		DurableReference: member.LegalAdmission.EvidenceReference,
		Issuer:           "Packy synthetic fixture", EvidenceOrigin: "issue-256 adapter tracer",
		Decision: "synthetic redistribution admitted",
		Candidate: packsync.LegalAdmissionCandidate{
			Repository: member.Registration.Repository, Commit: member.Registration.Selector.Ref,
			READMEBlob: strings.Repeat("c", 40), READMELength: 1, READMESHA256: strings.Repeat("d", 64),
		},
		Disposition: packsync.RedistributableDisposition,
		Rights:      []string{"copy"}, Obligations: []string{"preserve notice"}, Disclosures: []string{"synthetic fixture only"},
		Scope:    packsync.LegalAdmissionScope{SelectedRoots: selected, Exclusions: []string{}},
		Validity: "exact candidate and selected roots", Invalidation: "candidate, scope, or evidence digest changes",
	}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	target := filepath.Join(repository, filepath.FromSlash(member.LegalAdmission.EvidenceReference))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return sha256Text(string(raw))
}

type compositeAdapterSource struct {
	roots      map[string]string
	candidates map[string]packsync.Candidate
	resolves   map[string]int
}

func (s *compositeAdapterSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return nil, nil
}
func (s *compositeAdapterSource) ResolveRelease(context.Context, packsync.SourceConfig, packsync.Release) (packsync.Candidate, error) {
	return packsync.Candidate{}, nil
}
func (s *compositeAdapterSource) ResolveCommit(_ context.Context, registration packsync.SourceConfig, _ string) (packsync.Candidate, error) {
	s.resolves[registration.ID]++
	return s.candidates[registration.ID], nil
}
func (s *compositeAdapterSource) WithSnapshot(_ context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	target := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeErrorForTest(s.roots[candidate.Commit], target); err != nil {
		return err
	}
	err := visit(target)
	cleanup := os.RemoveAll(target)
	if err != nil {
		return err
	}
	return cleanup
}

func TestBundleDispatchDecodesV3EnvironmentWithoutCrossDecoding(t *testing.T) {
	sha, hash := strings.Repeat("a", 40), strings.Repeat("b", 64)
	var manifestValue any
	if err := json.Unmarshal([]byte(`{"schema_version":1,"id":"composite","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/one"},{"kind":"reference","id":"two","source":"references/two.md"}]}`), &manifestValue); err != nil {
		t.Fatal(err)
	}
	manifestBytes, _ := json.MarshalIndent(manifestValue, "", "  ")
	manifest := append(json.RawMessage(nil), append(manifestBytes, '\n')...)
	members := []packsyncworkflow.BundleRegistration{
		{Registration: packsync.SourceConfig{ID: "source-a", Provider: "github", Repository: "example/a", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: sha}, Resources: []packsync.Binding{{PackID: "composite", Kind: "skill", ResourceID: "one", UpstreamPath: "skill"}}}, LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "evidence/a", EvidenceSHA256: hash, Disposition: packsync.RedistributableDisposition}},
		{Registration: packsync.SourceConfig{ID: "source-b", Provider: "github", Repository: "example/b", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: sha}, Resources: []packsync.Binding{{PackID: "composite", Kind: "reference", ResourceID: "two", UpstreamPath: "notice.md"}}}, LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "evidence/b", EvidenceSHA256: hash, Disposition: packsync.RedistributableDisposition}},
	}
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationBundleSHA256("composite", members)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256Text(string(manifest))
	encoded, _ := json.Marshal(members)
	t.Setenv("PACKY_OPERATION", "register_bundle")
	t.Setenv("PACKY_PACK_ID", "composite")
	t.Setenv("PACKY_PROPOSED_VERSION", "1.0.0")
	t.Setenv("PACKY_PROPOSED_MANIFEST_JSON", string(manifest))
	t.Setenv("PACKY_PROPOSED_MANIFEST_SHA256", manifestDigest)
	t.Setenv("PACKY_REGISTRATIONS_JSON", string(encoded))
	t.Setenv("PACKY_REGISTRATION_BUNDLE_SHA256", registrationDigest)
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "synthetic tracer")
	request, err := bundleDispatch(options{})
	if err != nil {
		t.Fatal(err)
	}
	if request.SchemaVersion != 3 || request.PackID != "composite" || len(request.Registrations) != 2 {
		t.Fatalf("v3 environment dispatch = %#v", request)
	}

	v2 := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(v2, []byte(`{"schema_version":2,"operation":"synchronize","source_id":"source","selector":"latest-stable","classification_mode":"ai","request_reason":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := isBundleDispatch(options{requestPath: v2})
	if err != nil || bundle {
		t.Fatalf("v2 request crossed into v3 dispatch: bundle=%v err=%v", bundle, err)
	}
}
