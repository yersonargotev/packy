package packsync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCheckSingleSourceAdmissionProducesDeterministicMutationFreePlan(t *testing.T) {
	repository, snapshot, request, source := singleSourceAdmissionFixture(t)
	before, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Source: source, Validate: acceptingBundleValidator()}

	first, err := engine.CheckSingleSourceAdmission(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.AcquisitionDir = t.TempDir()
	second, err := engine.CheckSingleSourceAdmission(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	after, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("inspection mutated bundle: before=%s after=%s", before, after)
	}
	if entries, err := os.ReadDir(request.AcquisitionDir); err != nil || len(entries) != 0 {
		t.Fatalf("acquisition area not cleaned: entries=%v err=%v", entries, err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plan is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if !first.VerifySeal() || first.Status != "review-required" || first.PlanID == "" {
		t.Fatalf("plan is not sealed and decision-ready: %#v", first)
	}
	if first.PackID != "orchestrate" || first.ProposedVersion != "1.0.0" || first.ProposedManifestSHA256 != request.ProposedManifestSHA256 {
		t.Fatalf("proposed Pack contract missing: %#v", first)
	}
	if first.Registration.ID != "orchestrate-source" || first.Candidate.Commit != acceptedCandidate().Commit || first.LegalAdmission.Disposition != RedistributableDisposition {
		t.Fatalf("source or legal evidence missing: %#v", first)
	}
	if first.ProposedLock.SourceID != "orchestrate-source" || first.SourceLockSHA256 == "" || first.LockSetSHA256 == "" || first.ResultBundleSHA256 == "" {
		t.Fatalf("resulting provenance missing: %#v", first)
	}
	if first.Classification.PackID != "orchestrate" || first.Classification.CurrentVersion != "0.0.0" || first.Classification.MechanicalFloor != LevelMajor || !first.Classification.SemanticEvidenceRequired {
		t.Fatalf("initial classification missing: %#v", first.Classification)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "orchestrate", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSingleSourceAdmissionRequiresBidirectionalBindingEquality(t *testing.T) {
	_, _, request, source := singleSourceAdmissionFixture(t)
	var manifest map[string]any
	if err := json.Unmarshal(request.ProposedManifest, &manifest); err != nil {
		t.Fatal(err)
	}
	resources := manifest["resources"].([]any)
	manifest["resources"] = append(resources, map[string]any{
		"kind": "skill", "id": "unexpected", "source": "skills/unexpected",
		"requires": []any{}, "conflicts": []any{}, "notices": []any{},
		"bindings":           []any{map[string]any{"surface": "codex", "projection": "skill", "name": "unexpected", "invocation": "$unexpected", "mode": "native", "sharing": "exclusive"}},
		"surface_exclusions": []any{},
	})
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	request.ProposedManifest = raw
	canonical, err := canonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	request.ProposedManifestSHA256 = hashBytes(canonical)

	_, err = (Engine{Source: source, Validate: acceptingBundleValidator()}).CheckSingleSourceAdmission(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "bidirectionally") {
		t.Fatalf("binding mismatch error = %v", err)
	}
}

func TestCheckSingleSourceAdmissionRejectsInvalidOrConflictingRequests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *SingleSourceAdmissionCheckRequest, **fixtureSource)
		want   string
	}{
		{"existing Pack", func(t *testing.T, repository string, _ *SingleSourceAdmissionCheckRequest, _ **fixtureSource) {
			writeFile(t, filepath.Join(repository, "bundle", "packs", "orchestrate", "pack.json"), "{}\n")
		}, "already exists"},
		{"existing source", func(t *testing.T, repository string, request *SingleSourceAdmissionCheckRequest, _ **fixtureSource) {
			data, err := os.ReadFile(filepath.Join(repository, "bundle", "sources.json"))
			if err != nil {
				t.Fatal(err)
			}
			var config Config
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			existingIdentity := request.Registration
			existingIdentity.Resources = []Binding{{PackID: "other", Kind: "skill", ResourceID: "other", UpstreamPath: "other"}}
			config.Sources = append(config.Sources, existingIdentity)
			encoded, err := canonicalConfigAfterValidation(config)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(repository, "bundle", "sources.json"), string(encoded))
		}, "already configured"},
		{"malformed manifest", func(_ *testing.T, _ string, request *SingleSourceAdmissionCheckRequest, _ **fixtureSource) {
			request.ProposedManifest = json.RawMessage(`{`)
		}, "proposed manifest"},
		{"unavailable candidate", func(_ *testing.T, _ string, _ *SingleSourceAdmissionCheckRequest, source **fixtureSource) {
			(*source).candidate.Release = nil
		}, "no published stable release"},
		{"invalid legal evidence", func(_ *testing.T, _ string, request *SingleSourceAdmissionCheckRequest, _ **fixtureSource) {
			request.LegalAdmission.EvidenceSHA256 = strings.Repeat("f", 64)
		}, "legal admission"},
		{"unexpected selector", func(t *testing.T, _ string, request *SingleSourceAdmissionCheckRequest, _ **fixtureSource) {
			request.Registration.Selector = Selector{Mode: SelectorCommit, Ref: strings.Repeat("a", 40)}
			_, digest, err := canonicalRegistration(request.Registration)
			if err != nil {
				t.Fatal(err)
			}
			request.RegistrationSHA256 = digest
		}, "stable release selector"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, _, request, source := singleSourceAdmissionFixture(t)
			test.mutate(t, repository, &request, &source)
			_, err := (Engine{Source: source, Validate: acceptingBundleValidator()}).CheckSingleSourceAdmission(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCheckSingleSourceAdmissionRejectsChangedBaseFacts(t *testing.T) {
	repository, _, request, source := singleSourceAdmissionFixture(t)
	gated := &gatedSingleSource{fixtureSource: *source, acquired: make(chan struct{}), proceed: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := (Engine{Source: gated, Validate: acceptingBundleValidator()}).CheckSingleSourceAdmission(context.Background(), request)
		result <- err
	}()
	<-gated.acquired
	writeFile(t, filepath.Join(repository, "bundle", "changed-during-check"), "changed\n")
	close(gated.proceed)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "base facts changed") {
		t.Fatalf("changed base error = %v", err)
	}
}

func TestCheckSingleSourceAdmissionRejectsChangedCandidate(t *testing.T) {
	_, _, request, source := singleSourceAdmissionFixture(t)
	gated := &gatedSingleSource{fixtureSource: *source, acquired: make(chan struct{}), proceed: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := (Engine{Source: gated, Validate: acceptingBundleValidator()}).CheckSingleSourceAdmission(context.Background(), request)
		result <- err
	}()
	<-gated.acquired
	gated.candidate.Commit = strings.Repeat("9", 40)
	close(gated.proceed)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "candidate changed") {
		t.Fatalf("changed candidate error = %v", err)
	}
}

type gatedSingleSource struct {
	fixtureSource
	acquired chan struct{}
	proceed  chan struct{}
}

func (source *gatedSingleSource) WithSnapshot(ctx context.Context, candidate Candidate, temporaryRoot string, visit func(string) error) error {
	snapshot := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeError(source.root, snapshot); err != nil {
		return err
	}
	close(source.acquired)
	<-source.proceed
	err := visit(snapshot)
	cleanupErr := os.RemoveAll(snapshot)
	if err != nil {
		return err
	}
	return cleanupErr
}

func singleSourceAdmissionFixture(t *testing.T) (string, string, SingleSourceAdmissionCheckRequest, *fixtureSource) {
	t.Helper()
	repository, bootstrapSnapshot := tinyRepository(t)
	bootstrapSource := &fixtureSource{root: bootstrapSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	if _, err := (Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}).Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}

	snapshot := t.TempDir()
	writeFile(t, filepath.Join(snapshot, "orchestrate", "SKILL.md"), "# Orchestrate\n")
	writeFile(t, filepath.Join(snapshot, "LICENSE"), "MIT\n")
	candidate := acceptedCandidateFor("yersonargotev/orchestrate-skill")
	source := &fixtureSource{root: snapshot, candidate: candidate}
	registration := SourceConfig{
		ID: "orchestrate-source", Provider: "github", Repository: candidate.Repository,
		Selector: Selector{Mode: SelectorStableRelease},
		Resources: []Binding{
			{PackID: "orchestrate", Kind: "notice", ResourceID: "mit", UpstreamPath: "LICENSE"},
			{PackID: "orchestrate", Kind: "skill", ResourceID: "orchestrate", UpstreamPath: "orchestrate"},
		},
	}
	manifest := json.RawMessage(`{
  "id": "orchestrate",
  "version": "1.0.0",
  "description": "Coordinate focused Codex subagents",
  "selectable": true,
  "surfaces": ["codex"],
  "external_requirements": [],
  "resources": [
	{"kind":"lifecycle","id":"coordinate-session","requires":[],"conflicts":[],"bindings":[{"surface":"codex","projection":"lifecycle","name":"coordinate-session","invocation":"coordinate-session","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]},
    {"kind":"notice","id":"mit","source":"notices/mit","license":"MIT","attribution":"Copyright Eric Provencher","requires":[],"conflicts":[],"bindings":[],"surface_exclusions":[]},
	{"kind":"skill","id":"orchestrate","source":"skills/orchestrate","requires":[],"conflicts":[],"notices":["notice:mit"],"bindings":[{"surface":"codex","projection":"skill","name":"orchestrate","invocation":"$orchestrate","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]}
  ],
  "exclusions": []
}`)
	canonicalManifest, err := canonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	evidenceReference := "docs/evidence/orchestrate-admission.json"
	evidence := legalEvidenceJSON(t, evidenceReference, candidate)
	writeFile(t, filepath.Join(repository, filepath.FromSlash(evidenceReference)), string(evidence))
	_, registrationDigest, err := canonicalRegistration(registration)
	if err != nil {
		t.Fatal(err)
	}
	return repository, snapshot, SingleSourceAdmissionCheckRequest{
		RepositoryRoot:         repository,
		AcquisitionDir:         t.TempDir(),
		Registration:           registration,
		RegistrationSHA256:     registrationDigest,
		ProposedVersion:        "1.0.0",
		ProposedManifest:       manifest,
		ProposedManifestSHA256: hashBytes(canonicalManifest),
		LegalAdmission: CompositeLegalAdmission{
			EvidenceReference: evidenceReference,
			EvidenceSHA256:    hashBytes(evidence),
			Disposition:       RedistributableDisposition,
		},
	}, source
}

func legalEvidenceJSON(t *testing.T, reference string, candidate Candidate) []byte {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("README")))
	value := legalAdmissionEvidence{
		SchemaVersion: 1, EvidenceID: "orchestrate-mit", DurableReference: reference,
		Issuer: "Packy maintainer", EvidenceOrigin: "upstream LICENSE", Decision: "admit exact candidate",
		Candidate:   LegalAdmissionCandidate{Repository: candidate.Repository, Commit: candidate.Commit, READMEBlob: digest, READMELength: 6, READMESHA256: digest},
		Disposition: RedistributableDisposition,
		Rights:      []string{"redistribute"}, Obligations: []string{"preserve MIT notice"}, Disclosures: []string{"exact candidate only"},
		Scope:    LegalAdmissionScope{SelectedRoots: []string{"LICENSE", "orchestrate"}, Exclusions: []string{}},
		Validity: "digest-bound", Invalidation: "candidate or digest change",
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}
