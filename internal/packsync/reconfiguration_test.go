package packsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestMetadataOnlyReconfigurationPublishesNewGenerationWithoutChangingProvenance(t *testing.T) {
	repository, _, admission, source := singleSourceAdmissionFixture(t)
	engine := Engine{Source: source, Validate: acceptingBundleValidator()}
	admissionPlan, err := engine.CheckSingleSourceAdmission(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	admission.AcquisitionDir = t.TempDir()
	if _, err := engine.ApplySingleSourceAdmission(context.Background(), SingleSourceAdmissionApplyRequest{
		SingleSourceAdmissionCheckRequest: admission,
		Plan:                              admissionPlan,
		ClassificationEvidence:            singleSourceAdmissionClassification(admissionPlan),
	}); err != nil {
		t.Fatal(err)
	}
	commitFixtureRepository(t, repository)

	bundle := filepath.Join(repository, "bundle")
	previousHistory := filepath.Join(bundle, "history", "orchestrate", "1.0.0")
	previousHistorySHA256, err := treeHash(previousHistory)
	if err != nil {
		t.Fatal(err)
	}
	previousLock := mustReadFile(t, sourceLockPath(repository, "orchestrate-source"))
	previousConfig := mustReadFile(t, filepath.Join(bundle, "sources.json"))

	var proposed map[string]any
	if err := json.Unmarshal(admission.ProposedManifest, &proposed); err != nil {
		t.Fatal(err)
	}
	proposed["description"] = "Coordinate focused Codex subagents across one reviewed task"
	proposedManifest, err := json.Marshal(proposed)
	if err != nil {
		t.Fatal(err)
	}
	reconfiguration := admission.Registration
	check := CheckRequest{
		RepositoryRoot:   repository,
		SourceID:         reconfiguration.ID,
		AcquisitionDir:   t.TempDir(),
		Reconfiguration:  &reconfiguration,
		ProposedManifest: proposedManifest,
	}
	plan, err := engine.Check(context.Background(), check)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "review-required" || len(plan.AffectedPacks) != 1 || plan.AffectedPacks[0].MechanicalFloor != LevelNone {
		t.Fatalf("metadata-only inspection = %#v", plan)
	}
	if !reflect.DeepEqual(plan.ProposedLock, admissionPlan.ProposedLock) {
		t.Fatalf("metadata-only inspection changed selected upstream provenance:\nbefore=%#v\nafter=%#v", admissionPlan.ProposedLock, plan.ProposedLock)
	}
	for _, change := range plan.Changes {
		if change.Kind != "manifest-reconfigured" {
			t.Fatalf("metadata-only inspection reported non-manifest change: %#v", change)
		}
	}

	evidence := metadataOnlyClassification(plan, "1.0.1")
	check.AcquisitionDir = t.TempDir()
	result, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: check, Plan: plan, ClassificationEvidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.Changed {
		t.Fatalf("result = %#v", result)
	}
	if got, err := treeHash(previousHistory); err != nil || got != previousHistorySHA256 {
		t.Fatalf("previous sealed generation changed: got=%s want=%s err=%v", got, previousHistorySHA256, err)
	}
	if got := mustReadFile(t, sourceLockPath(repository, "orchestrate-source")); !reflect.DeepEqual(got, previousLock) {
		t.Fatalf("metadata-only generation changed source lock:\nbefore=%s\nafter=%s", previousLock, got)
	}
	if got := mustReadFile(t, filepath.Join(bundle, "sources.json")); !reflect.DeepEqual(got, previousConfig) {
		t.Fatalf("metadata-only generation changed source configuration:\nbefore=%s\nafter=%s", previousConfig, got)
	}
	for _, path := range []string{
		filepath.Join(bundle, "packs", "orchestrate", "pack.json"),
		filepath.Join(bundle, "history", "orchestrate", "1.0.1", "pack.json"),
	} {
		data := mustReadFile(t, path)
		if !containsJSONFields(t, data, map[string]any{"version": "1.0.1", "description": proposed["description"]}) {
			t.Fatalf("new metadata generation missing from %s: %s", path, data)
		}
	}
	artifact := mustReadFile(t, filepath.Join(bundle, "history", "orchestrate", "1.0.1", "artifact.json"))
	if !containsJSONFields(t, artifact, map[string]any{"pack_id": "orchestrate", "pack_version": "1.0.1"}) {
		t.Fatalf("new generation artifact has wrong identity: %s", artifact)
	}
}

func TestMetadataOnlyReconfigurationRejectsProtectedProvenanceMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"source reference", func(manifest map[string]any) {
			manifest["source_reference"].(map[string]any)["revision"] = "different"
		}},
		{"attribution", func(manifest map[string]any) {
			manifestResources(manifest)[1]["attribution"] = "Copyright somebody else"
		}},
		{"license", func(manifest map[string]any) {
			manifestResources(manifest)[1]["license"] = "Apache-2.0"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, _, admission, source := singleSourceAdmissionFixture(t)
			var admitted map[string]any
			if err := json.Unmarshal(admission.ProposedManifest, &admitted); err != nil {
				t.Fatal(err)
			}
			admitted["source_reference"] = map[string]any{
				"repository": "https://github.com/yersonargotev/orchestrate-skill.git",
				"revision":   "1.0.0",
			}
			proposedManifest, err := json.Marshal(admitted)
			if err != nil {
				t.Fatal(err)
			}
			admission.ProposedManifest = proposedManifest
			canonical, err := canonicalJSON(admission.ProposedManifest)
			if err != nil {
				t.Fatal(err)
			}
			admission.ProposedManifestSHA256 = hashBytes(canonical)
			engine := Engine{Source: source, Validate: acceptingBundleValidator()}
			admissionPlan, err := engine.CheckSingleSourceAdmission(context.Background(), admission)
			if err != nil {
				t.Fatal(err)
			}
			admission.AcquisitionDir = t.TempDir()
			if _, err := engine.ApplySingleSourceAdmission(context.Background(), SingleSourceAdmissionApplyRequest{
				SingleSourceAdmissionCheckRequest: admission,
				Plan:                              admissionPlan,
				ClassificationEvidence:            singleSourceAdmissionClassification(admissionPlan),
			}); err != nil {
				t.Fatal(err)
			}

			var proposed map[string]any
			if err := json.Unmarshal(admission.ProposedManifest, &proposed); err != nil {
				t.Fatal(err)
			}
			test.mutate(proposed)
			proposedManifest, err = json.Marshal(proposed)
			if err != nil {
				t.Fatal(err)
			}
			reconfiguration := admission.Registration
			_, err = engine.Check(context.Background(), CheckRequest{
				RepositoryRoot: repository, SourceID: reconfiguration.ID, AcquisitionDir: t.TempDir(),
				Reconfiguration: &reconfiguration, ProposedManifest: proposedManifest,
			})
			if err == nil {
				t.Fatal("reconfiguration changed protected provenance metadata")
			}
		})
	}
}

func TestMetadataOnlyReconfigurationRecoveryExposesOnlyCompleteGenerations(t *testing.T) {
	for _, point := range []FaultPoint{FaultBeforeSwap, FaultAfterFirstRename, FaultAfterSecondRename, FaultDuringCleanup} {
		t.Run(string(point), func(t *testing.T) {
			repository, engine, check, plan := metadataReconfigurationFixture(t)
			bundle := filepath.Join(repository, "bundle")
			previousHistory := filepath.Join(bundle, "history", "orchestrate", "1.0.0")
			previousHistorySHA256, err := treeHash(previousHistory)
			if err != nil {
				t.Fatal(err)
			}
			engine.Fault = failOnce(point)
			check.AcquisitionDir = t.TempDir()
			_, err = engine.Apply(context.Background(), ApplyRequest{
				CheckRequest: check, Plan: plan, ClassificationEvidence: metadataOnlyClassification(plan, "1.0.1"),
			})
			if err == nil {
				t.Fatal("fault did not interrupt metadata generation")
			}
			if point != FaultBeforeSwap {
				if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := treeHash(previousHistory); err != nil || got != previousHistorySHA256 {
				t.Fatalf("recovery changed previous generation: got=%s want=%s err=%v", got, previousHistorySHA256, err)
			}
			current := mustReadFile(t, filepath.Join(bundle, "packs", "orchestrate", "pack.json"))
			var manifest map[string]any
			if err := json.Unmarshal(current, &manifest); err != nil {
				t.Fatal(err)
			}
			newHistory := filepath.Join(bundle, "history", "orchestrate", "1.0.1")
			switch manifest["version"] {
			case "1.0.0":
				if _, err := os.Stat(newHistory); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("old generation exposed partial new history: %v", err)
				}
			case "1.0.1":
				for _, path := range []string{"pack.json", "artifact.json"} {
					if _, err := os.Stat(filepath.Join(newHistory, path)); err != nil {
						t.Fatalf("new generation is incomplete: %s: %v", path, err)
					}
				}
			default:
				t.Fatalf("recovery exposed invalid generation: %s", current)
			}
			if _, err := os.Stat(recoveryMarkerPath(repository)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recovery marker remains: %v", err)
			}
		})
	}
}

func TestCheckedInOrchestrateSupportsMetadataOnlyReconfiguration(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	repository := t.TempDir()
	if err := copyTreeExact(filepath.Join(root, "bundle"), filepath.Join(repository, "bundle")); err != nil {
		t.Fatal(err)
	}
	commitFixtureRepository(t, repository)

	configBytes := mustReadFile(t, filepath.Join(repository, "bundle", "sources.json"))
	config, err := LoadConfig(bytes.NewReader(configBytes))
	if err != nil {
		t.Fatal(err)
	}
	reconfiguration, err := selectSource(config, "orchestrate-source")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, present, err := readLock(sourceLockPath(repository, reconfiguration.ID))
	if err != nil || !present {
		t.Fatalf("read Orchestrate lock: present=%t err=%v", present, err)
	}
	snapshot := t.TempDir()
	for _, resource := range lock.Resources {
		sourcePath := filepath.Join(repository, filepath.FromSlash(resource.VendoredPath))
		targetPath := filepath.Join(snapshot, filepath.FromSlash(resource.UpstreamPath))
		if err := copyTreeExact(sourcePath, targetPath); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(repository, "bundle", "packs", "orchestrate", "pack.json")
	var proposed map[string]any
	if err := json.Unmarshal(mustReadFile(t, manifestPath), &proposed); err != nil {
		t.Fatal(err)
	}
	proposed["description"] = "Coordinate focused Codex subagents across one reviewed task"
	proposedManifest, err := json.Marshal(proposed)
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{
		Source: &fixtureSource{root: snapshot, candidate: lock.Candidate},
		Validate: BundleValidatorFunc(func(_ context.Context, _, bundleRoot string) error {
			return ValidateContent(bundleRoot)
		}),
	}
	check := CheckRequest{RepositoryRoot: repository, SourceID: reconfiguration.ID, AcquisitionDir: t.TempDir(), Reconfiguration: &reconfiguration, ProposedManifest: proposedManifest}
	plan, err := engine.Check(context.Background(), check)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "review-required" || len(plan.AffectedPacks) != 1 || plan.AffectedPacks[0].MechanicalFloor != LevelNone || !reflect.DeepEqual(plan.ProposedLock, lock) {
		t.Fatalf("checked-in Orchestrate metadata inspection = %#v", plan)
	}
	check.AcquisitionDir = t.TempDir()
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: check, Plan: plan, ClassificationEvidence: metadataOnlyClassification(plan, "1.0.1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, "bundle", "history", "orchestrate", "1.0.1", "artifact.json")); err != nil {
		t.Fatal(err)
	}
}

func metadataReconfigurationFixture(t *testing.T) (string, Engine, CheckRequest, Plan) {
	t.Helper()
	repository, _, admission, source := singleSourceAdmissionFixture(t)
	engine := Engine{Source: source, Validate: acceptingBundleValidator()}
	admissionPlan, err := engine.CheckSingleSourceAdmission(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	admission.AcquisitionDir = t.TempDir()
	if _, err := engine.ApplySingleSourceAdmission(context.Background(), SingleSourceAdmissionApplyRequest{
		SingleSourceAdmissionCheckRequest: admission,
		Plan:                              admissionPlan,
		ClassificationEvidence:            singleSourceAdmissionClassification(admissionPlan),
	}); err != nil {
		t.Fatal(err)
	}
	commitFixtureRepository(t, repository)
	var proposed map[string]any
	if err := json.Unmarshal(admission.ProposedManifest, &proposed); err != nil {
		t.Fatal(err)
	}
	proposed["description"] = "Coordinate focused Codex subagents across one reviewed task"
	proposedManifest, err := json.Marshal(proposed)
	if err != nil {
		t.Fatal(err)
	}
	reconfiguration := admission.Registration
	check := CheckRequest{RepositoryRoot: repository, SourceID: reconfiguration.ID, AcquisitionDir: t.TempDir(), Reconfiguration: &reconfiguration, ProposedManifest: proposedManifest}
	plan, err := engine.Check(context.Background(), check)
	if err != nil {
		t.Fatal(err)
	}
	return repository, engine, check, plan
}

func manifestResources(manifest map[string]any) []map[string]any {
	raw := manifest["resources"].([]any)
	resources := make([]map[string]any, len(raw))
	for index := range raw {
		resources[index] = raw[index].(map[string]any)
	}
	return resources
}

func metadataOnlyClassification(plan Plan, version string) ClassificationEvidenceSet {
	impact := plan.AffectedPacks[0]
	return ClassificationEvidenceSet{
		SchemaVersion: 1,
		PlanID:        plan.PlanID,
		BaseSHA:       plan.Preconditions.BaseCommit,
		Candidate:     plan.Candidate,
		Evidence: []ClassificationEvidence{{
			PackID: impact.PackID, Classifier: ClassifierIdentity{Type: ClassifierAI, ID: "fixture-model"},
			Rationale: "Only descriptive Pack-owned metadata changed.", CurrentVersion: impact.CurrentVersion,
			ProposedVersion: version, ChangedAspects: []string{"Pack description"},
			MechanicalFloor: impact.MechanicalFloor, FinalLevel: LevelPatch,
		}},
	}
}

func containsJSONFields(t *testing.T, data []byte, want map[string]any) bool {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for key, value := range want {
		if !reflect.DeepEqual(got[key], value) {
			return false
		}
	}
	return true
}

func mustReadFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func commitFixtureRepository(t *testing.T, repository string) {
	t.Helper()
	repo, err := git.PlainInit(repository, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "Packy Test", Email: "test@example.com", When: time.Unix(1, 0)}}); err != nil {
		t.Fatal(err)
	}
}
