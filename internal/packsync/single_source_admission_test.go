package packsync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
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

func TestSingleSourceAdmissionApplyMaterializesCompleteGeneration(t *testing.T) {
	repository, _, request, source := singleSourceAdmissionFixture(t)
	engine := Engine{Source: source, Validate: acceptingBundleValidator()}
	plan, err := engine.CheckSingleSourceAdmission(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.AcquisitionDir = t.TempDir()
	result, err := engine.ApplySingleSourceAdmission(context.Background(), SingleSourceAdmissionApplyRequest{
		SingleSourceAdmissionCheckRequest: request,
		Plan:                              plan,
		ClassificationEvidence:            singleSourceAdmissionClassification(plan),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.Changed || result.PlanID != plan.PlanID {
		t.Fatalf("result = %#v", result)
	}

	configBytes, err := os.ReadFile(filepath.Join(repository, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(strings.NewReader(string(configBytes)))
	if err != nil || len(config.Sources) != 2 || config.Sources[1].ID != plan.Registration.ID {
		t.Fatalf("complete source configuration = %#v, %v", config.Sources, err)
	}
	for _, path := range []string{
		sourceLockPath(repository, plan.Registration.ID),
		filepath.Join(repository, "bundle", "skills", "orchestrate", "SKILL.md"),
		filepath.Join(repository, "bundle", "notices", "mit"),
		filepath.Join(repository, "bundle", "packs", "orchestrate", "pack.json"),
		filepath.Join(repository, "bundle", "history", "orchestrate", "1.0.0", "pack.json"),
		filepath.Join(repository, "bundle", "history", "orchestrate", "1.0.0", "artifact.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("complete generation path %s: %v", path, err)
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(repository, "bundle", "packs", "orchestrate", "pack.json"))
	if err != nil || !strings.Contains(string(manifestBytes), `"selectable": true`) {
		t.Fatalf("selectable manifest = %s, %v", manifestBytes, err)
	}
	if hash, err := treeHash(filepath.Join(repository, "bundle")); err != nil || hash != plan.ResultBundleSHA256 {
		t.Fatalf("result bundle hash = %s, %v; want %s", hash, err, plan.ResultBundleSHA256)
	}
	admitted, err := capabilitypack.LoadCurrentManifest(filepath.Join(repository, "bundle", "packs", "orchestrate", "pack.json"), filepath.Join(repository, "bundle"), true)
	if err != nil {
		t.Fatal(err)
	}
	if admitted.ID != "orchestrate" || admitted.Version != "1.0.0" || !admitted.Selectable {
		t.Fatalf("admitted Pack is not a selectable catalog entry: %#v", admitted)
	}
}

func singleSourceAdmissionClassification(plan SingleSourceAdmissionPlan) CompositeClassificationEvidence {
	return CompositeClassificationEvidence{SchemaVersion: 1, PlanID: plan.PlanID, PackID: plan.PackID, Evidence: ClassificationEvidence{
		PackID: plan.PackID, Classifier: ClassifierIdentity{Type: ClassifierAI, ID: "synthetic"}, Rationale: "initial single-source Pack admission",
		CurrentVersion: "0.0.0", ProposedVersion: plan.ProposedVersion, ChangedAspects: []string{"initial complete Pack generation"},
		MechanicalFloor: LevelMajor, FinalLevel: LevelMajor,
		Migration: "initial generation has no predecessor", RequiredActions: []string{"review initial complete Pack contract"},
	}}
}

func TestSingleSourceAdmissionClassificationIsPlanBoundAndProposalOnly(t *testing.T) {
	_, _, request, source := singleSourceAdmissionFixture(t)
	plan, err := (Engine{Source: source, Validate: acceptingBundleValidator()}).CheckSingleSourceAdmission(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, classifierType := range []ClassifierType{ClassifierAI, ClassifierHuman} {
		t.Run(string(classifierType), func(t *testing.T) {
			evidence := singleSourceAdmissionClassification(plan)
			evidence.Evidence.Classifier = ClassifierIdentity{Type: classifierType, ID: "fixture"}
			if err := ValidateSingleSourceAdmissionClassificationEvidence(plan, evidence); err != nil {
				t.Fatalf("plan-bound classification rejected: %v", err)
			}
			encoded, err := json.Marshal(evidence)
			if err != nil || strings.Contains(string(encoded), "decision_ready") || strings.Contains(string(encoded), "auto_merge") {
				t.Fatalf("classification gained publication authority: err=%v %s", err, encoded)
			}
			evidence.PlanID = "different-plan"
			if err := ValidateSingleSourceAdmissionClassificationEvidence(plan, evidence); err == nil {
				t.Fatal("classification from a different plan was accepted")
			}
		})
	}
}

func TestSingleSourceAdmissionApplyRejectsFreshnessAndValidationFailuresWithoutAdmission(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *SingleSourceAdmissionApplyRequest, **fixtureSource, *Engine)
		want   string
	}{
		{"changed request", func(_ *testing.T, _ string, apply *SingleSourceAdmissionApplyRequest, _ **fixtureSource, _ *Engine) {
			apply.ProposedVersion = "1.0.1"
		}, "generation changed"},
		{"changed candidate", func(_ *testing.T, _ string, _ *SingleSourceAdmissionApplyRequest, source **fixtureSource, _ *Engine) {
			(*source).candidate.Commit = strings.Repeat("9", 40)
		}, "candidate changed"},
		{"changed legal evidence", func(t *testing.T, repository string, apply *SingleSourceAdmissionApplyRequest, _ **fixtureSource, _ *Engine) {
			writeFile(t, filepath.Join(repository, filepath.FromSlash(apply.LegalAdmission.EvidenceReference)), "changed\n")
		}, "legal admission changed"},
		{"invalid classification", func(_ *testing.T, _ string, apply *SingleSourceAdmissionApplyRequest, _ **fixtureSource, _ *Engine) {
			apply.ClassificationEvidence.Evidence.ProposedVersion = "2.0.0"
		}, "classification evidence"},
		{"changed base generation", func(t *testing.T, repository string, _ *SingleSourceAdmissionApplyRequest, _ **fixtureSource, _ *Engine) {
			writeFile(t, filepath.Join(repository, "bundle", "changed-after-check"), "stale\n")
		}, "complete bundle changed"},
		{"source ownership conflict", func(t *testing.T, repository string, apply *SingleSourceAdmissionApplyRequest, _ **fixtureSource, _ *Engine) {
			data, err := os.ReadFile(filepath.Join(repository, "bundle", "sources.json"))
			if err != nil {
				t.Fatal(err)
			}
			var config Config
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			config.Sources = append(config.Sources, apply.Registration)
			encoded, err := canonicalConfigAfterValidation(config)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(repository, "bundle", "sources.json"), string(encoded))
		}, "already exists"},
		{"failed staged validation", func(_ *testing.T, _ string, _ *SingleSourceAdmissionApplyRequest, _ **fixtureSource, engine *Engine) {
			engine.Validate = BundleValidatorFunc(func(_ context.Context, _, bundle string) error {
				if _, err := os.Stat(filepath.Join(bundle, "packs", "orchestrate", "pack.json")); err == nil {
					return errors.New("synthetic complete Pack validation failure")
				}
				return nil
			})
		}, "validation failure"},
		{"changed result tree", func(t *testing.T, _ string, apply *SingleSourceAdmissionApplyRequest, _ **fixtureSource, _ *Engine) {
			apply.Plan.ResultBundleSHA256 = strings.Repeat("f", 64)
			planID, err := sealSingleSourceAdmissionPlan(apply.Plan)
			if err != nil {
				t.Fatal(err)
			}
			apply.Plan.PlanID = planID
			apply.ClassificationEvidence = singleSourceAdmissionClassification(apply.Plan)
		}, "result tree contradicts"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, _, request, source := singleSourceAdmissionFixture(t)
			plan, err := (Engine{Source: source, Validate: acceptingBundleValidator()}).CheckSingleSourceAdmission(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			before, err := treeHash(filepath.Join(repository, "bundle"))
			if err != nil {
				t.Fatal(err)
			}
			request.AcquisitionDir = t.TempDir()
			apply := SingleSourceAdmissionApplyRequest{SingleSourceAdmissionCheckRequest: request, Plan: plan, ClassificationEvidence: singleSourceAdmissionClassification(plan)}
			engine := Engine{Source: source, Validate: acceptingBundleValidator()}
			test.mutate(t, repository, &apply, &source, &engine)
			engine.Source = source
			_, err = engine.ApplySingleSourceAdmission(context.Background(), apply)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if _, err := os.Stat(filepath.Join(repository, "bundle", "packs", "orchestrate")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected Apply admitted Pack: %v", err)
			}
			if test.name != "changed base generation" && test.name != "source ownership conflict" {
				after, hashErr := treeHash(filepath.Join(repository, "bundle"))
				if hashErr != nil || after != before {
					t.Fatalf("rejected Apply changed bundle: before=%s after=%s err=%v", before, after, hashErr)
				}
			}
		})
	}
}

func TestSingleSourceAdmissionReplacementFaultRecoversOnlyCompleteGenerations(t *testing.T) {
	for _, point := range []FaultPoint{FaultBeforeSwap, FaultAfterFirstRename, FaultAfterSecondRename, FaultDuringCleanup} {
		t.Run(string(point), func(t *testing.T) {
			repository, _, request, source := singleSourceAdmissionFixture(t)
			engine := Engine{Source: source, Validate: acceptingBundleValidator(), Fault: failOnce(point)}
			plan, err := engine.CheckSingleSourceAdmission(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			request.AcquisitionDir = t.TempDir()
			_, err = engine.ApplySingleSourceAdmission(context.Background(), SingleSourceAdmissionApplyRequest{
				SingleSourceAdmissionCheckRequest: request, Plan: plan,
				ClassificationEvidence: singleSourceAdmissionClassification(plan),
			})
			if err == nil {
				t.Fatal("fault did not interrupt replacement")
			}
			if point != FaultBeforeSwap {
				if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); err != nil {
					t.Fatal(err)
				}
			}
			assertSingleSourceAdmissionGeneration(t, repository)
		})
	}
}

func assertSingleSourceAdmissionGeneration(t *testing.T, repository string) {
	t.Helper()
	present := 0
	for _, path := range []string{
		sourceLockPath(repository, "orchestrate-source"),
		filepath.Join(repository, "bundle", "skills", "orchestrate", "SKILL.md"),
		filepath.Join(repository, "bundle", "notices", "mit"),
		filepath.Join(repository, "bundle", "packs", "orchestrate", "pack.json"),
		filepath.Join(repository, "bundle", "history", "orchestrate", "1.0.0", "artifact.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			present++
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(repository, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	configured := false
	for _, source := range config.Sources {
		configured = configured || source.ID == "orchestrate-source"
	}
	if configured {
		present++
	}
	if present != 0 && present != 6 {
		t.Fatalf("recovery exposed partial single-source generation: %d/6 components", present)
	}
	if _, err := os.Stat(recoveryMarkerPath(repository)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery marker remains: %v", err)
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
