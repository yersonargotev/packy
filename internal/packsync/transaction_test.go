package packsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

func TestInitialApplyBootstrapsTruthfulProvenanceWithoutSelectedContentChange(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy evidence\n")
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	plan := checkWith(t, repository, provider)
	selectedBefore := hashSelectedResources(t, repository, plan.ProposedLock)
	validated := 0
	engine := Engine{allowBootstrap: true, Source: provider, Validate: BundleValidatorFunc(func(_ context.Context, _, bundle string) error {
		validated++
		_, err := treeHash(bundle)
		return err
	})}
	request := ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}
	result, err := engine.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.Changed || validated != 1 {
		t.Fatalf("result=%#v validations=%d", result, validated)
	}
	if got := hashSelectedResources(t, repository, plan.ProposedLock); got != selectedBefore {
		t.Fatalf("selected content changed: %s -> %s", selectedBefore, got)
	}
	if _, err := os.Stat(filepath.Join(repository, "skills-lock.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy evidence still present: %v", err)
	}
	production, _, present, err := readLock(filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"))
	if err != nil || !present || lockDigest(production) != lockDigest(plan.ProposedLock) {
		t.Fatalf("production lock = %#v, present=%t, err=%v", production, present, err)
	}
	repeated := checkWith(t, repository, provider)
	if repeated.Status != "no-op" || !repeated.Authoritative || len(repeated.Changes) != 0 || len(repeated.Blockers) != 0 {
		t.Fatalf("post-Apply Check = %#v", repeated)
	}
	retry, err := engine.Apply(context.Background(), request)
	if err != nil || retry.Status != "no-op" || retry.Changed {
		t.Fatalf("repeated Apply = %#v, %v", retry, err)
	}
}

func TestApplyCommitsRegistrationConfigurationLockAndContributionAtomically(t *testing.T) {
	repository, existingSnapshot := tinyRepository(t)
	initializeFixtureGit(t, repository)
	bootstrapSource := &fixtureSource{root: existingSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	if _, err := (Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}).Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}
	legacyPackID := "ma" + "tty"
	writeFile(t, filepath.Join(repository, "bundle", "packs", legacyPackID, "pack.json"), fmt.Sprintf(`{"schema_version":1,"id":%q,"version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/engineering/one"},{"kind":"skill","id":"two","source":"skills/engineering/two"}]}`, legacyPackID))
	snapshot := t.TempDir()
	writeFile(t, filepath.Join(snapshot, "skills", "engineering", "two", "SKILL.md"), "new\n")
	registration := SourceConfig{ID: "addy", Provider: "github", Repository: "addyosmani/agent-skills", Selector: Selector{Mode: SelectorStableRelease}, Resources: []Binding{{PackID: legacyPackID, Kind: "skill", ResourceID: "two", UpstreamPath: "skills/engineering/two"}}}
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidateFor("addyosmani/agent-skills")}
	engine := Engine{Source: provider, Validate: acceptingBundleValidator()}
	request := CheckRequest{RepositoryRoot: repository, SourceID: "addy", Registration: &registration, AcquisitionDir: t.TempDir()}
	plan, err := engine.Check(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	evidence := classificationEvidenceForPlan(t, plan, ClassifierAI, "fixture-model", LevelMinor)
	result, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: request, Plan: plan, ClassificationEvidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.Changed {
		t.Fatalf("result = %#v", result)
	}
	config, err := os.Open(filepath.Join(repository, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer config.Close()
	parsed, err := LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Sources) != 2 || parsed.Sources[0].ID != "addy" {
		t.Fatalf("config = %#v", parsed)
	}
	if _, _, present, err := readLock(sourceLockPath(repository, "addy")); err != nil || !present {
		t.Fatalf("lock present=%t err=%v", present, err)
	}
	if string(mustReadFile(t, filepath.Join(repository, "bundle", "skills", "engineering", "two", "SKILL.md"))) != "new\n" {
		t.Fatal("contribution missing")
	}
	if retry, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: request, Plan: plan, ClassificationEvidence: evidence}); err == nil || retry.Status == "no-op" {
		t.Fatalf("initial registration replay = %#v, %v", retry, err)
	}
}

func TestApplyCommitsExistingSourceReconfigurationAsOneGeneration(t *testing.T) {
	sourceRepository := repositoryRoot(t)
	repository := t.TempDir()
	copyTree(t, filepath.Join(sourceRepository, "bundle"), filepath.Join(repository, "bundle"))
	initializeFixtureGit(t, repository)
	snapshot := realSnapshot(t, sourceRepository, false)
	copyTree(t, filepath.Join(snapshot, "skills", "productivity", "writing-great-skills"), filepath.Join(snapshot, "skills", "productivity", "writing-for-agents"))

	configFile, err := os.Open(filepath.Join(repository, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(configFile)
	configFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	var proposed SourceConfig
	for _, source := range config.Sources {
		if source.ID != "mattpocock-skills" {
			continue
		}
		proposed = source
		proposed.Resources = append([]Binding(nil), source.Resources...)
		for i := range proposed.Resources {
			if proposed.Resources[i].ResourceID == "writing-great-skills" {
				proposed.Resources[i].ResourceID = "writing-for-agents"
				proposed.Resources[i].UpstreamPath = "skills/productivity/writing-for-agents"
			}
		}
	}
	if proposed.ID == "" {
		t.Fatal("fixture source is absent")
	}

	manifestBytes := mustReadFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"))
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, raw := range manifest["resources"].([]any) {
		resource := raw.(map[string]any)
		if resource["id"] != "writing-great-skills" {
			continue
		}
		resource["id"] = "writing-for-agents"
		resource["source"] = "skills/productivity/writing-for-agents"
		for _, rawBinding := range resource["bindings"].([]any) {
			binding := rawBinding.(map[string]any)
			binding["name"] = "writing-for-agents"
			binding["invocation"] = "writing-for-agents"
		}
	}
	canonicalManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	canonicalManifest = append(canonicalManifest, '\n')

	locked, _, present, err := readLock(sourceLockPath(repository, "mattpocock-skills"))
	if err != nil || !present {
		t.Fatalf("fixture lock: present=%t err=%v", present, err)
	}
	provider := &fixtureSource{root: snapshot, candidate: locked.Candidate}
	engine := Engine{Source: provider, Validate: acceptingBundleValidator()}
	check := CheckRequest{RepositoryRoot: repository, SourceID: proposed.ID, AcquisitionDir: t.TempDir(), Reconfiguration: &proposed, ProposedManifest: canonicalManifest}
	plan, err := engine.Check(context.Background(), check)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "review-required" || plan.Reconfiguration == nil || !plan.VerifySeal() || len(plan.AffectedPacks) != 1 || plan.AffectedPacks[0].MechanicalFloor != LevelMajor {
		t.Fatalf("reconfiguration plan = %#v", plan)
	}
	assertChange(t, plan, "source-reconfigured")
	assertChange(t, plan, "manifest-reconfigured")
	assertChange(t, plan, "resource-added")
	assertChange(t, plan, "resource-removed")
	evidence := classificationEvidenceForPlan(t, plan, ClassifierHuman, "maintainer", LevelMajor)
	evidence.HumanInspectionID, err = HumanInspectionID(plan)
	if err != nil {
		t.Fatal(err)
	}
	apply := ApplyRequest{CheckRequest: CheckRequest{RepositoryRoot: repository, SourceID: proposed.ID, AcquisitionDir: t.TempDir(), Reconfiguration: &proposed, ProposedManifest: plan.ProposedManifest}, Plan: plan, ClassificationEvidence: evidence}
	result, err := engine.Apply(context.Background(), apply)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.Changed {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(repository, "bundle", "skills", "productivity", "writing-great-skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired skill remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "bundle", "skills", "productivity", "writing-for-agents", "SKILL.md")); err != nil {
		t.Fatalf("replacement skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "bundle", "history", "matty", "5.0.0", "artifact.json")); err != nil {
		t.Fatalf("classified history missing: %v", err)
	}
	changedEvidence := filepath.Join(repository, "bundle", "compatibility", "matty", "4.0.0-to-5.0.0.json")
	data, err := os.ReadFile(changedEvidence)
	if err != nil || validateChangedSelectionEvidence(data, plan, evidence) != nil {
		t.Fatalf("changed-selection evidence invalid: %v", err)
	}
	var partial map[string]any
	if err := json.Unmarshal(data, &partial); err != nil {
		t.Fatal(err)
	}
	delete(partial, "added")
	partialData, _ := json.Marshal(partial)
	if err := validateChangedSelectionEvidence(partialData, plan, evidence); err == nil {
		t.Fatal("partial changed-selection evidence was accepted")
	}
}

func acceptedCandidateFor(repository string) Candidate {
	candidate := acceptedCandidate()
	candidate.Repository = repository
	owner := strings.Split(repository, "/")[0]
	candidate.Owner = owner
	candidate.RepositoryHTML = "https://github.com/" + repository
	candidate.RepositoryClone = candidate.RepositoryHTML + ".git"
	candidate.RepositoryAPI = "https://api.github.com/repos/" + repository
	return candidate
}

func TestApplyFaultsAndRecoverDeterministically(t *testing.T) {
	for _, test := range []struct {
		point        FaultPoint
		wantBundle   string
		wantRecovery string
	}{
		{FaultBeforeSwap, "old", ""},
		{FaultAfterFirstRename, "missing", "rolled-back"},
		{FaultAfterSecondRename, "new", "completed"},
		{FaultDuringCleanup, "new", "completed"},
	} {
		t.Run(string(test.point), func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			oldHash, err := treeHash(filepath.Join(repository, "bundle"))
			if err != nil {
				t.Fatal(err)
			}
			engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(test.point)}
			_, applyErr := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan})
			if applyErr == nil {
				t.Fatal("faulted Apply unexpectedly succeeded")
			}
			bundle := filepath.Join(repository, "bundle")
			switch test.wantBundle {
			case "old":
				if got, err := treeHash(bundle); err != nil || got != oldHash {
					t.Fatalf("pre-swap bundle = %s, %v; want %s", got, err, oldHash)
				}
				assertNoTransactionEvidence(t, repository)
				return
			case "missing":
				if _, err := os.Stat(bundle); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("bundle exists between renames: %v", err)
				}
			case "new":
				if _, _, present, err := readLock(filepath.Join(bundle, "sources/mattpocock-skills.lock.json")); err != nil || !present {
					t.Fatalf("new bundle is not installed: present=%t err=%v", present, err)
				}
			}
			recovered, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
			if err != nil || recovered.Status != test.wantRecovery {
				t.Fatalf("Recover = %#v, %v", recovered, err)
			}
			assertNoTransactionEvidence(t, repository)
		})
	}
}

func TestRecoverFailsClosedForMissingManipulatedAndIncompatibleEvidence(t *testing.T) {
	repository := t.TempDir()
	engine := Engine{allowBootstrap: true, Validate: acceptingBundleValidator()}
	if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); !errors.Is(err, ErrRecoveryEvidence) {
		t.Fatalf("missing marker error = %v", err)
	}
	for _, marker := range []string{
		`{"schema_version":1}`,
		`{"schema_version":1,"plan_id":"bad","phase":"prepared","bundle":"/tmp/outside","backup":"/tmp/outside-backup","staged":"/tmp/outside-stage","old_sha256":"` + strings.Repeat("a", 64) + `","new_sha256":"` + strings.Repeat("b", 64) + `"}`,
	} {
		writeFile(t, recoveryMarkerPath(repository), marker+"\n")
		if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); !errors.Is(err, ErrRecoveryEvidence) {
			t.Fatalf("manipulated marker error = %v", err)
		}
		if err := os.Remove(recoveryMarkerPath(repository)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplyRejectsEverySealedFreshnessBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *fixtureSource, *Plan, *Engine)
		want   string
	}{
		{name: "plan", want: "sealed plan", mutate: func(_ *testing.T, _ string, _ *fixtureSource, plan *Plan, _ *Engine) {
			plan.Preconditions.ConfigSHA256 = strings.Repeat("f", 64)
		}},
		{name: "base", want: "repository base", mutate: func(t *testing.T, _ string, _ *fixtureSource, plan *Plan, _ *Engine) {
			plan.Preconditions.BaseCommit = strings.Repeat("a", 40)
			resealPlan(t, plan)
		}},
		{name: "candidate", want: "candidate provenance changed", mutate: func(_ *testing.T, _ string, source *fixtureSource, _ *Plan, _ *Engine) {
			source.candidate.RepositoryID++
		}},
		{name: "configuration", want: "source configuration changed", mutate: func(t *testing.T, repository string, _ *fixtureSource, _ *Plan, _ *Engine) {
			name := filepath.Join(repository, "bundle", "sources.json")
			writeFile(t, name, string(mustReadFile(t, name))+"\n")
		}},
		{name: "bundle-history-evidence", want: "bundle, history, or compatibility", mutate: func(t *testing.T, repository string, _ *fixtureSource, _ *Plan, _ *Engine) {
			name := filepath.Join(repository, "bundle", "skills", "engineering", "one", "SKILL.md")
			writeFile(t, name, "drift\n")
		}},
		{name: "production-lock", want: "source lock mattpocock-skills", mutate: func(t *testing.T, repository string, _ *fixtureSource, _ *Plan, _ *Engine) {
			writeFile(t, filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"), "{}\n")
		}},
		{name: "Packy-owned-suite", want: "validate staged bundle", mutate: func(_ *testing.T, _ string, _ *fixtureSource, _ *Plan, engine *Engine) {
			engine.Validate = BundleValidatorFunc(func(context.Context, string, string) error { return errors.New("suite rejected hostile content") })
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator()}
			test.mutate(t, repository, provider, &plan, &engine)
			_, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Apply error = %v, want %q", err, test.want)
			}
			if _, markerErr := os.Stat(recoveryMarkerPath(repository)); !errors.Is(markerErr, os.ErrNotExist) {
				t.Fatalf("stale Apply published recovery state: %v", markerErr)
			}
		})
	}
}

func TestPublicationProvenanceRevalidationRejectsMovedCandidate(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	plan := checkWith(t, repository, provider)
	if err := (Engine{allowBootstrap: true, Source: provider}).RevalidateCandidate(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	provider.candidate.RepositoryID++
	if err := (Engine{allowBootstrap: true, Source: provider}).RevalidateCandidate(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "provenance changed") {
		t.Fatalf("moved provenance error = %v", err)
	}
}

func TestRecoverPendingTreatsAbsentMarkerAsCleanState(t *testing.T) {
	result, pending, err := (Engine{allowBootstrap: true}).RecoverPending(context.Background(), t.TempDir())
	if err != nil || pending || result.Status != "" {
		t.Fatalf("clean recovery state = %#v pending=%v err=%v", result, pending, err)
	}
}

func TestRecoverRetainsEvidenceForIncompleteBackupAndAmbiguousSiblings(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string, recoveryMarker)
	}{
		{name: "incomplete-backup", mutate: func(t *testing.T, _ string, marker recoveryMarker) {
			name := filepath.Join(marker.Backup, "skills", "engineering", "one", "SKILL.md")
			writeFile(t, name, "tampered\n")
		}},
		{name: "incomplete-staging", mutate: func(t *testing.T, _ string, marker recoveryMarker) {
			name := filepath.Join(marker.Staged, "skills", "engineering", "one", "SKILL.md")
			writeFile(t, name, "tampered\n")
		}},
		{name: "ambiguous-sibling", mutate: func(t *testing.T, repository string, _ recoveryMarker) {
			if err := os.Mkdir(filepath.Join(repository, ".packy-bundle-unexpected.backup"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultAfterFirstRename)}
			if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
				t.Fatal("faulted Apply unexpectedly succeeded")
			}
			marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, repository, marker)
			if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); !errors.Is(err, ErrRecoveryEvidence) {
				t.Fatalf("Recover error = %v", err)
			}
			if _, err := os.Stat(recoveryMarkerPath(repository)); err != nil {
				t.Fatalf("recovery marker was not retained: %v", err)
			}
		})
	}
}

func TestApplyAndRecoverHoldSharedLockForEveryMutationAndRepairPhase(t *testing.T) {
	for _, point := range []FaultPoint{FaultBeforeSwap, FaultAfterFirstRename, FaultAfterSecondRename, FaultDuringCleanup} {
		t.Run(string(point), func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator(), Fault: func(observed FaultPoint) error {
				if observed != point {
					return nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
				if guard, err := bundletransaction.Acquire(ctx, repository); err == nil {
					guard.Release()
					return errors.New("mutation phase did not hold the shared lock")
				}
				return errors.New("injected while locked")
			}}
			if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil || !strings.Contains(err.Error(), "injected while locked") {
				t.Fatalf("Apply error = %v", err)
			}
			if point == FaultBeforeSwap {
				return
			}
			engine.Fault = nil
			engine.Validate = BundleValidatorFunc(func(ctx context.Context, _, _ string) error {
				wait, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
				if guard, err := bundletransaction.Acquire(wait, repository); err == nil {
					guard.Release()
					return errors.New("Recover validation did not hold the shared lock")
				}
				return nil
			})
			if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoverFinishesCleanupIdempotentlyAfterEffectsCompleted(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	plan := checkWith(t, repository, provider)
	engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultDuringCleanup)}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
		t.Fatal("cleanup fault unexpectedly succeeded")
	}
	marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanupCommitted(marker); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
	if err != nil || result.Status != "completed" {
		t.Fatalf("Recover = %#v, %v", result, err)
	}
	assertNoTransactionEvidence(t, repository)
}

func TestStagedSuiteFailureLeavesRepositoryUntouched(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	plan := checkWith(t, repository, provider)
	before, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	validations := 0
	engine := Engine{allowBootstrap: true, Source: provider, Validate: BundleValidatorFunc(func(context.Context, string, string) error {
		validations++
		if validations == 1 {
			return errors.New("staged suite failed")
		}
		return nil
	})}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil || !strings.Contains(err.Error(), "staged suite failed") {
		t.Fatalf("Apply error = %v", err)
	}
	after, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil || before != after {
		t.Fatalf("pre-swap failure changed bundle: %s -> %s, %v", before, after, err)
	}
	if _, err := os.Stat(filepath.Join(repository, "skills-lock.json")); err != nil {
		t.Fatalf("pre-swap failure removed legacy evidence: %v", err)
	}
	assertNoTransactionEvidence(t, repository)
}

func TestApplyMaterializesCompleteAuthoritativeCandidateBundle(t *testing.T) {
	repository, oldSnapshot := tinyRepository(t)
	upgradeTinyRepositoryToV4(t, repository)
	initializeFixtureGit(t, repository)
	oldCandidate := acceptedCandidate()
	bootstrapSource := &fixtureSource{root: oldSnapshot, candidate: oldCandidate}
	bootstrap := checkWith(t, repository, bootstrapSource)
	engine := Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}

	newSnapshot := t.TempDir()
	writeFile(t, filepath.Join(newSnapshot, "skills", "engineering", "one", "SKILL.md"), "updated\n")
	newCandidate := advancedCandidate(oldCandidate)
	source := &multiReleaseSource{root: newSnapshot, candidates: map[string]Candidate{oldCandidate.Release.Tag: oldCandidate, newCandidate.Release.Tag: newCandidate}}
	plan := checkWith(t, repository, source)
	if plan.Status != "review-required" || !plan.Authoritative || plan.Counts.Modified != 1 {
		t.Fatalf("authoritative update plan = %#v", plan)
	}
	engine.Source = source
	evidence := classificationEvidenceForPlan(t, plan, ClassifierAI, "fixture-model", LevelPatch)
	if len(plan.AffectedPacks) != 1 || plan.AffectedPacks[0].Contract == nil {
		t.Fatalf("v4 contract evidence = %#v", plan.AffectedPacks)
	}
	oldBundleHash, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	badPlan := plan
	badPlan.AffectedPacks = append([]PackImpact(nil), plan.AffectedPacks...)
	badContract := *plan.AffectedPacks[0].Contract
	badContract.CurrentManifestSHA256 = strings.Repeat("0", 64)
	badPlan.AffectedPacks[0].Contract = &badContract
	badPlan.PlanID, err = seal(badPlan)
	if err != nil {
		t.Fatal(err)
	}
	badEvidence := classificationEvidenceForPlan(t, badPlan, ClassifierAI, "fixture-model", LevelPatch)
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: badPlan, ClassificationEvidence: badEvidence}); err == nil ||
		!strings.Contains(err.Error(), "staged manifest contract") {
		t.Fatalf("invalid staged contract error = %v", err)
	}
	if after, err := treeHash(filepath.Join(repository, "bundle")); err != nil || after != oldBundleHash {
		t.Fatalf("invalid staged contract crossed swap boundary: hash=%s err=%v", after, err)
	}
	result, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan, ClassificationEvidence: evidence})
	if err != nil || result.Status != "applied" {
		t.Fatalf("Apply = %#v, %v", result, err)
	}
	updated := mustReadFile(t, filepath.Join(repository, "bundle", "skills", "engineering", "one", "SKILL.md"))
	if string(updated) != "updated\n" {
		t.Fatalf("selected candidate was not materialized: %q", updated)
	}
	manifest := mustReadFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"))
	if !strings.Contains(string(manifest), `"version": "1.0.1"`) {
		t.Fatalf("classified exact version was not materialized: %s", manifest)
	}
	packID := plan.AffectedPacks[0].PackID
	history := filepath.Join(repository, "bundle", "history", packID, "1.0.1")
	if historical := mustReadFile(t, filepath.Join(history, "pack.json")); string(historical) != string(manifest) {
		t.Fatal("classified immutable history manifest differs from the installed manifest")
	}
	if historical := mustReadFile(t, filepath.Join(history, "skills", "engineering", "one", "SKILL.md")); string(historical) != "updated\n" {
		t.Fatalf("classified immutable history omitted selected resource: %q", historical)
	}
	if _, _, err := readHistoricalManifestBaseline(repository, packID, "1.0.1"); err != nil {
		t.Fatalf("classified immutable history artifact is invalid: %v", err)
	}
	var artifact compositeHistoricalArtifact
	if err := json.Unmarshal(mustReadFile(t, filepath.Join(history, "artifact.json")), &artifact); err != nil {
		t.Fatal(err)
	}
	foundSourceLess := false
	for _, resource := range artifact.Resources {
		if resource.Source == "" {
			foundSourceLess = true
			if resource.Files == nil || len(resource.Files) != 0 || resource.SHA256 != resourceHash([]FileEvidence{}) {
				t.Fatalf("source-less historical evidence = %#v", resource)
			}
		}
	}
	if !foundSourceLess {
		t.Fatal("classified history omitted source-less v4 resource evidence")
	}
	lock, _, present, err := readLock(filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"))
	if err != nil || !present || lock.Candidate.Commit != newCandidate.Commit {
		t.Fatalf("updated lock = %#v, present=%t, err=%v", lock, present, err)
	}
	repeated := checkWith(t, repository, source)
	if repeated.Status != "no-op" || len(repeated.Changes) != 0 || len(repeated.Blockers) != 0 {
		t.Fatalf("repeated Check = %#v", repeated)
	}
	retryRequest := ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan, ClassificationEvidence: evidence}
	if retry, err := engine.Apply(context.Background(), retryRequest); err != nil || retry.Status != "no-op" || retry.Changed {
		t.Fatalf("idempotent classified Apply = %#v, %v", retry, err)
	}
	writeFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"), strings.Replace(string(manifest), `"version": "1.0.1"`, `"version": "9.9.9"`, 1))
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan, ClassificationEvidence: evidence}); err == nil || !strings.Contains(err.Error(), "classified pack versions") {
		t.Fatalf("tampered classified version error = %v", err)
	}
}

func upgradeTinyRepositoryToV4(t *testing.T, repository string) {
	t.Helper()
	existing, _, err := loadManifests(repository)
	if err != nil || len(existing) != 1 {
		t.Fatalf("derive sole tiny Pack identity: manifests=%#v err=%v", existing, err)
	}
	var packID string
	for packID = range existing {
	}
	raw := portableManifestV4Fixture
	raw = strings.Replace(raw, `"id": "example"`, fmt.Sprintf(`"id": %q`, packID), 1)
	raw = strings.Replace(raw, `"id": "example"`, `"id": "one"`, 1)
	raw = strings.Replace(raw, `"source": "skills/example.md"`, `"source": "skills/engineering/one"`, 1)
	raw = strings.Replace(raw, "  }],\n  \"contract\"", `  }, {
    "kind": "lifecycle",
    "id": "background",
    "requires": [],
    "conflicts": [],
    "notices": [],
    "provides_capabilities": [],
    "requires_capabilities": [],
    "requires_tools": [],
    "capability_conflicts": [],
    "bindings": [{
      "surface": "codex",
      "projection": "lifecycle",
      "name": "background",
      "invocation": "background",
      "mode": "native",
      "sharing": "exclusive"
    }],
    "surface_exclusions": []
  }],
  "contract"`, 1)
	var document map[string]any
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatal(err)
	}
	resources := document["resources"].([]any)
	resources[0], resources[1] = resources[1], resources[0]
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = string(encoded)
	path := filepath.Join(repository, "bundle", "packs", packID, "pack.json")
	writeFile(t, path, raw)
	manifests, _, err := loadManifests(repository)
	if err != nil {
		t.Fatal(err)
	}
	canonical := manifests[packID].canonicalV4
	writeFile(t, path, string(canonical))
	history := filepath.Join(repository, "bundle", "history", packID, "1.0.0")
	writeFile(t, filepath.Join(history, "pack.json"), string(canonical))
	for _, resource := range manifests[packID].Resources {
		if resource.Source == "" {
			continue
		}
		if err := copyTreeExact(
			filepath.Join(repository, "bundle", filepath.FromSlash(resource.Source)),
			filepath.Join(history, filepath.FromSlash(resource.Source)),
		); err != nil {
			t.Fatal(err)
		}
	}
	writeHistoricalManifestArtifact(t, history, packID, "1.0.0", canonical)
}

func TestApplyRejectsAffectedPlanWithoutCompleteClassificationEvidence(t *testing.T) {
	repository, oldSnapshot := tinyRepository(t)
	initializeFixtureGit(t, repository)
	bootstrapSource := &fixtureSource{root: oldSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	engine := Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}

	newSnapshot := t.TempDir()
	writeFile(t, filepath.Join(newSnapshot, "skills", "engineering", "one", "SKILL.md"), "updated\n")
	newCandidate := advancedCandidate(acceptedCandidate())
	source := &multiReleaseSource{root: newSnapshot, candidates: map[string]Candidate{acceptedCandidate().Release.Tag: acceptedCandidate(), newCandidate.Release.Tag: newCandidate}}
	plan := checkWith(t, repository, source)
	before, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	engine.Source = source
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil || !strings.Contains(err.Error(), "complete evidence coverage") {
		t.Fatalf("Apply error = %v", err)
	}
	after, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil || after != before {
		t.Fatalf("rejected classification changed bundle: %s -> %s, %v", before, after, err)
	}
}

func TestApplyRemovesObsoleteDestinationWhenManifestMovesBinding(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, provider)
	engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator()}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}
	oldDestination := filepath.Join(repository, "bundle", "skills", "engineering", "one")
	newDestination := filepath.Join(repository, "bundle", "skills", "engineering", "moved")
	copyTree(t, oldDestination, newDestination)
	manifest := filepath.Join(repository, "bundle", "packs", "matty", "pack.json")
	writeFile(t, manifest, strings.Replace(string(mustReadFile(t, manifest)), "skills/engineering/one", "skills/engineering/moved", 1))

	plan := checkWith(t, repository, provider)
	if plan.Status != "review-required" || !plan.Authoritative || plan.Counts.Moved != 1 {
		t.Fatalf("destination move plan = %#v", plan)
	}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDestination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete selected destination remains: %v", err)
	}
	if got := string(mustReadFile(t, filepath.Join(newDestination, "SKILL.md"))); got != "same\n" {
		t.Fatalf("new selected destination = %q", got)
	}
	repeated := checkWith(t, repository, provider)
	if repeated.Status != "no-op" || len(repeated.Changes) != 0 || len(repeated.Blockers) != 0 {
		t.Fatalf("repeated Check = %#v", repeated)
	}
}

func TestRecoverResumesAfterItsOwnRollbackAndCleanupEffects(t *testing.T) {
	t.Run("rollback rename already completed", func(t *testing.T) {
		repository, snapshot := tinyRepository(t)
		writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
		provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
		plan := checkWith(t, repository, provider)
		engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultAfterFirstRename)}
		if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
			t.Fatal("faulted Apply unexpectedly succeeded")
		}
		marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
		if err != nil {
			t.Fatal(err)
		}
		marker.Phase = "rolling-back"
		if err := writeRecoveryMarker(recoveryMarkerPath(repository), &marker); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(marker.Backup, marker.Bundle); err != nil {
			t.Fatal(err)
		}
		result, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
		if err != nil || result.Status != "rolled-back" {
			t.Fatalf("Recover = %#v, %v", result, err)
		}
		assertNoTransactionEvidence(t, repository)
	})

	t.Run("new bundle cleanup already completed", func(t *testing.T) {
		repository, snapshot := tinyRepository(t)
		writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
		provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
		plan := checkWith(t, repository, provider)
		engine := Engine{allowBootstrap: true, Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultAfterSecondRename)}
		if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
			t.Fatal("faulted Apply unexpectedly succeeded")
		}
		marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
		if err != nil {
			t.Fatal(err)
		}
		if err := cleanupCommitted(marker); err != nil {
			t.Fatal(err)
		}
		result, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
		if err != nil || result.Status != "completed" {
			t.Fatalf("Recover = %#v, %v", result, err)
		}
		assertNoTransactionEvidence(t, repository)
	})
}

func acceptingBundleValidator() BundleValidator {
	return BundleValidatorFunc(func(context.Context, string, string) error { return nil })
}

func resealPlan(t *testing.T, plan *Plan) {
	t.Helper()
	plan.PlanID = ""
	id, err := seal(*plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanID = id
}

func advancedCandidate(previous Candidate) Candidate {
	next := previous
	release := *previous.Release
	release.ID++
	release.NodeID = "new-release-node"
	release.Tag = "v1.2.0"
	release.Name = "v1.2.0"
	release.CreatedAt = release.CreatedAt.Add(time.Hour)
	release.PublishedAt = release.PublishedAt.Add(time.Hour)
	next.Release = &release
	next.TagRefName = "refs/tags/" + release.Tag
	next.TagRefSHA = strings.Repeat("c", 40)
	next.TagObjects = append([]TagObject(nil), previous.TagObjects...)
	next.TagObjects[0].SHA = next.TagRefSHA
	next.TagObjects[0].Name = release.Tag
	next.Commit = strings.Repeat("d", 40)
	next.CommitNodeID = "new-commit-node"
	next.Tree = strings.Repeat("e", 40)
	next.Parents = []string{previous.Commit}
	next.TagObjects[0].TargetSHA = next.Commit
	return next
}

func failOnce(point FaultPoint) FaultInjector {
	fired := false
	return func(observed FaultPoint) error {
		if observed == point && !fired {
			fired = true
			return errors.New("injected " + string(point))
		}
		return nil
	}
}

func newCheckRequest(t *testing.T, repository string) CheckRequest {
	t.Helper()
	return CheckRequest{RepositoryRoot: repository, AcquisitionDir: t.TempDir()}
}

func hashSelectedResources(t *testing.T, repository string, lock Lock) string {
	t.Helper()
	var evidence []FileEvidence
	for _, resource := range lock.Resources {
		files, err := inventory(filepath.Join(repository, filepath.FromSlash(resource.VendoredPath)))
		if err != nil {
			t.Fatal(err)
		}
		evidence = append(evidence, FileEvidence{Path: bindingKey(resource.Binding), Size: int64(len(files)), Mode: 0o600, SHA256: resourceHash(files)})
	}
	return resourceHash(evidence)
}

func assertNoTransactionEvidence(t *testing.T, repository string) {
	t.Helper()
	if _, err := os.Stat(recoveryMarkerPath(repository)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery marker remains: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(repository, ".packy-bundle-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("transaction siblings remain: %v, %v", matches, err)
	}
}
