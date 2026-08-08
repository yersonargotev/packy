package packsync

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type compositeFixtureSource struct {
	roots        map[string]string
	candidates   map[string]Candidate
	fail         string
	failSnapshot string
}

func (source *compositeFixtureSource) Releases(context.Context, SourceConfig) ([]Release, error) {
	return nil, nil
}
func (source *compositeFixtureSource) ResolveRelease(context.Context, SourceConfig, Release) (Candidate, error) {
	return Candidate{}, errors.New("composite fixtures use exact commits")
}
func (source *compositeFixtureSource) ResolveCommit(_ context.Context, config SourceConfig, sha string) (Candidate, error) {
	if config.ID == source.fail {
		return Candidate{}, errors.New("synthetic member failure")
	}
	candidate := source.candidates[config.ID]
	candidate.Commit = sha
	candidate.Release = nil
	candidate.TagObjects = nil
	candidate.TagRefSHA = ""
	return candidate, nil
}
func (source *compositeFixtureSource) WithSnapshot(_ context.Context, candidate Candidate, temporaryRoot string, visit func(string) error) error {
	if candidate.Repository == source.failSnapshot {
		return errors.New("synthetic snapshot failure")
	}
	root := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeError(source.roots[candidate.Repository], root); err != nil {
		return err
	}
	err := visit(root)
	cleanupErr := os.RemoveAll(root)
	if err != nil {
		return err
	}
	return cleanupErr
}

func TestCompositeCheckApplyMaterializesCompleteCrossSourcePack(t *testing.T) {
	repository, provider, request := compositeFixture(t)
	validator := BundleValidatorFunc(func(_ context.Context, _, bundle string) error {
		first, err := os.ReadFile(filepath.Join(bundle, "skills", "first", "SKILL.md"))
		if errors.Is(err, os.ErrNotExist) {
			if _, secondErr := os.Stat(filepath.Join(bundle, "notices", "second.md")); errors.Is(secondErr, os.ErrNotExist) {
				return nil
			}
		}
		if err != nil {
			return err
		}
		second, err := os.ReadFile(filepath.Join(bundle, "notices", "second.md"))
		if err != nil {
			return err
		}
		if !strings.Contains(string(first), "notices/second.md") || string(second) != "closure\n" {
			return errors.New("cross-source dependency closure is incomplete")
		}
		return nil
	})
	engine := Engine{Source: provider, Validate: validator}
	plan, err := engine.CheckComposite(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.VerifySeal() || len(plan.Members) != 2 || plan.RegistrationBundleSHA256 == "" || plan.LockSetSHA256 == "" {
		t.Fatalf("incomplete composite plan: %#v", plan)
	}
	result, err := engine.ApplyComposite(context.Background(), CompositeApplyRequest{CompositeCheckRequest: request, Plan: plan, ClassificationEvidence: compositeClassification(plan)})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatalf("result = %#v", result)
	}
	configBytes, err := os.ReadFile(filepath.Join(repository, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(strings.NewReader(string(configBytes)))
	if err != nil || len(config.Sources) != 3 {
		t.Fatalf("complete sources: %#v, %v", config.Sources, err)
	}
	for _, id := range []string{"source-a", "source-b"} {
		if _, err := os.Stat(sourceLockPath(repository, id)); err != nil {
			t.Fatalf("member %s lock absent: %v", id, err)
		}
	}
}

func TestCompositeCheckDoesNotBootstrapMissingSourceConfiguration(t *testing.T) {
	repository, provider, request := compositeFixture(t)
	removeSourceProvenance(t, repository)

	_, err := (Engine{Source: provider, Validate: acceptingBundleValidator()}).CheckComposite(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), filepath.Join("bundle", "sources.json")) {
		t.Fatalf("missing composite source configuration error = %v", err)
	}
}

func TestCompositeCheckPreservesAndSealsSchemaV4Manifest(t *testing.T) {
	_, provider, request := compositeFixture(t)
	request.ProposedManifest = json.RawMessage(`{
	  "schema_version": 4,
	  "id": "composite",
	  "version": "1.0.0",
	  "description": "synthetic v4 composite",
	  "surfaces": ["codex"],
	  "provides": [],
	  "requires": {"capabilities": [], "tools": []},
	  "conflicts": [],
	  "resources": [
	    {"kind":"skill","id":"first","source":"skills/first","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]},
	    {"kind":"notice","id":"second","source":"notices/second.md","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]}
	  ]
	}`)
	plan, err := (Engine{Source: provider, Validate: acceptingBundleValidator()}).CheckComposite(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.VerifySeal() || plan.ProposedManifestSHA256 != hashBytes(plan.ProposedManifest) || !strings.Contains(string(plan.ProposedManifest), `"schema_version": 4`) || !strings.Contains(string(plan.ProposedManifest), `"surfaces"`) {
		t.Fatalf("v4 manifest was not retained and sealed: %s", plan.ProposedManifest)
	}
}

func TestRevalidateCompositeCandidatesRequiresEveryExactMember(t *testing.T) {
	_, provider, request := compositeFixture(t)
	engine := Engine{Source: provider, Validate: acceptingBundleValidator()}
	plan, err := engine.CheckComposite(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.RevalidateCompositeCandidates(context.Background(), plan); err != nil {
		t.Fatalf("exact complete candidate set failed revalidation: %v", err)
	}
	moved := provider.candidates["source-b"]
	moved.Tree = strings.Repeat("c", 40)
	provider.candidates["source-b"] = moved
	if err := engine.RevalidateCompositeCandidates(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "source-b") || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("one-member movement error = %v", err)
	}
}

func TestCompositeCheckRejectsIncompleteCompleteResultWithoutWriting(t *testing.T) {
	repository, provider, request := compositeFixture(t)
	before, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	validator := BundleValidatorFunc(func(_ context.Context, _, bundle string) error {
		if _, err := os.Stat(filepath.Join(bundle, "notices", "required-third.md")); errors.Is(err, os.ErrNotExist) {
			return errors.New("complete Pack closure is missing required-third")
		}
		return err
	})
	if _, err := (Engine{Source: provider, Validate: validator}).CheckComposite(context.Background(), request); err == nil || !strings.Contains(err.Error(), "required-third") {
		t.Fatalf("incomplete result error = %v", err)
	}
	after, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("failed complete-result validation changed the old bundle")
	}
}

func TestCanonicalCompositePackManifestParityAndSemanticChange(t *testing.T) {
	compact := json.RawMessage(`{"schema_version":4,"id":"composite","version":"1.0.0","provides":[],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"skill","id":"first","source":"skills/first","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]}]}`)
	formatted := json.RawMessage("{\n  \"resources\": [{\"capability_conflicts\":[],\"requires_tools\":[],\"requires_capabilities\":[],\"provides_capabilities\":[],\"source\":\"skills/first\",\"id\":\"first\",\"kind\":\"skill\"}],\n  \"conflicts\":[], \"requires\":{\"tools\":[],\"capabilities\":[]}, \"provides\":[],\n  \"version\":\"1.0.0\", \"id\":\"composite\", \"schema_version\":4\n}\n")
	left, err := CanonicalCompositePackManifest(compact)
	if err != nil {
		t.Fatal(err)
	}
	right, err := CanonicalCompositePackManifest(formatted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) || hashBytes(left) != hashBytes(right) {
		t.Fatal("semantically equal manifests produced different workflow/core identities")
	}
	changed, err := CanonicalCompositePackManifest(json.RawMessage(`{"schema_version":4,"id":"composite","version":"1.0.1","provides":[],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"skill","id":"first","source":"skills/first","provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(left) == hashBytes(changed) {
		t.Fatal("semantic manifest change retained the old digest")
	}
}

func TestCompositeRejectsInvalidSetsAndStaleFactsBeforeWrites(t *testing.T) {
	repository, provider, request := compositeFixture(t)
	engine := Engine{Source: provider, Validate: acceptingBundleValidator()}
	before, _ := treeHash(filepath.Join(repository, "bundle"))
	duplicate := request
	duplicate.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	duplicate.Members[1].Registration.ID = duplicate.Members[0].Registration.ID
	if _, err := engine.CheckComposite(context.Background(), duplicate); err == nil {
		t.Fatal("duplicate member was accepted")
	}
	outside := request
	outside.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	outside.Members[0].Registration.Resources = append([]Binding(nil), outside.Members[0].Registration.Resources...)
	outside.Members[0].Registration.Resources[0].PackID = "other"
	if _, err := engine.CheckComposite(context.Background(), outside); err == nil {
		t.Fatal("outside-Pack binding was accepted")
	}
	incomplete := request
	incomplete.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	incomplete.Members[0].LegalAdmission.EvidenceSHA256 = ""
	if _, err := engine.CheckComposite(context.Background(), incomplete); err == nil {
		t.Fatal("incomplete legal evidence was accepted")
	}
	partial := request
	partial.Members = append([]CompositeRegistrationMember(nil), request.Members[:1]...)
	if _, err := engine.CheckComposite(context.Background(), partial); err == nil {
		t.Fatal("partial composite was accepted")
	}
	unsafe := request
	unsafe.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	unsafe.Members[0].Registration.ID = "../unsafe"
	if _, err := engine.CheckComposite(context.Background(), unsafe); err == nil {
		t.Fatal("unsafe source id was accepted")
	}
	mixedSelector := request
	mixedSelector.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	mixedSelector.Members[0].Registration.Selector = Selector{Mode: SelectorStableRelease}
	if _, err := engine.CheckComposite(context.Background(), mixedSelector); err == nil {
		t.Fatal("mixed selector set was accepted")
	}
	mixedVersion := request
	mixedVersion.ProposedVersion = "2.0.0"
	if _, err := engine.CheckComposite(context.Background(), mixedVersion); err == nil {
		t.Fatal("manifest/request version mismatch was accepted")
	}
	duplicateOwner := request
	duplicateOwner.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	duplicateOwner.Members[1].Registration.Resources = append([]Binding(nil), request.Members[1].Registration.Resources...)
	duplicateOwner.Members[1].Registration.Resources[0] = request.Members[0].Registration.Resources[0]
	if _, err := engine.CheckComposite(context.Background(), duplicateOwner); err == nil {
		t.Fatal("duplicate cross-source ownership was accepted")
	}
	existingPack := request
	writeFile(t, filepath.Join(repository, "bundle", "packs", "composite", "pack.json"), string(request.ProposedManifest))
	if _, err := engine.CheckComposite(context.Background(), existingPack); err == nil {
		t.Fatal("existing Pack was accepted")
	}
	if err := os.RemoveAll(filepath.Join(repository, "bundle", "packs", "composite")); err != nil {
		t.Fatal(err)
	}
	existingSource := request
	existingSource.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	existingSource.Members[0].Registration.ID = "mattpocock-skills"
	if _, err := engine.CheckComposite(context.Background(), existingSource); err == nil {
		t.Fatal("already-present member source was accepted")
	}
	plan, err := engine.CheckComposite(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository, "bundle", "unrelated"), "stale\n")
	if _, err := engine.ApplyComposite(context.Background(), CompositeApplyRequest{CompositeCheckRequest: request, Plan: plan, ClassificationEvidence: compositeClassification(plan)}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale apply error = %v", err)
	}
	_ = os.Remove(filepath.Join(repository, "bundle", "unrelated"))
	provider.fail = "source-b"
	if _, err := engine.CheckComposite(context.Background(), request); err == nil || !strings.Contains(err.Error(), "source-b") {
		t.Fatalf("member failure = %v", err)
	}
	provider.fail = ""
	provider.failSnapshot = "example/b"
	if _, err := engine.CheckComposite(context.Background(), request); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("member snapshot failure = %v", err)
	}
	after, _ := treeHash(filepath.Join(repository, "bundle"))
	if before != after {
		t.Fatal("rejected composite operation changed the bundle")
	}
}

func TestCompositeReplacementFaultRecoversOnlyCompleteTrees(t *testing.T) {
	for _, point := range []FaultPoint{FaultBeforeSwap, FaultAfterFirstRename, FaultAfterSecondRename, FaultDuringCleanup} {
		t.Run(string(point), func(t *testing.T) {
			repository, provider, request := compositeFixture(t)
			engine := Engine{Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(point)}
			plan, err := engine.CheckComposite(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.ApplyComposite(context.Background(), CompositeApplyRequest{CompositeCheckRequest: request, Plan: plan, ClassificationEvidence: compositeClassification(plan)}); err == nil {
				t.Fatal("fault did not interrupt replacement")
			}
			if point != FaultBeforeSwap {
				if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); err != nil {
					t.Fatal(err)
				}
			}
			assertCompositeGeneration(t, repository)
		})
	}
}

func assertCompositeGeneration(t *testing.T, repository string) {
	t.Helper()
	present := 0
	for _, path := range []string{
		sourceLockPath(repository, "source-a"),
		sourceLockPath(repository, "source-b"),
		filepath.Join(repository, "bundle", "packs", "composite", "pack.json"),
		filepath.Join(repository, "bundle", "history", "composite", "1.0.0", "artifact.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			present++
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	if present != 0 && present != 4 {
		t.Fatalf("recovery exposed partial composite generation: %d/4 members", present)
	}
}

func compositeFixture(t *testing.T) (string, *compositeFixtureSource, CompositeCheckRequest) {
	t.Helper()
	repository, bootstrapSnapshot := tinyRepository(t)
	bootstrapSource := &fixtureSource{root: bootstrapSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	if _, err := (Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}).Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}
	first, second := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(first, "skill", "SKILL.md"), "uses notices/second.md\n")
	writeFile(t, filepath.Join(second, "notice.md"), "closure\n")
	shaA := strings.Repeat("a", 40)
	shaB := strings.Repeat("b", 40)
	members := []CompositeRegistrationMember{
		{Registration: SourceConfig{ID: "source-a", Provider: "github", Repository: "example/a", Selector: Selector{Mode: SelectorCommit, Ref: shaA}, Resources: []Binding{{PackID: "composite", Kind: "skill", ResourceID: "first", UpstreamPath: "skill"}}}, LegalAdmission: CompositeLegalAdmission{EvidenceReference: "evidence/a.json", EvidenceSHA256: strings.Repeat("1", 64), Disposition: RedistributableDisposition}},
		{Registration: SourceConfig{ID: "source-b", Provider: "github", Repository: "example/b", Selector: Selector{Mode: SelectorCommit, Ref: shaB}, Resources: []Binding{{PackID: "composite", Kind: "notice", ResourceID: "second", UpstreamPath: "notice.md"}}}, LegalAdmission: CompositeLegalAdmission{EvidenceReference: "evidence/b.json", EvidenceSHA256: strings.Repeat("2", 64), Disposition: RedistributableDisposition}},
	}
	provider := &compositeFixtureSource{
		roots:      map[string]string{"example/a": first, "example/b": second},
		candidates: map[string]Candidate{"source-a": acceptedCandidateFor("example/a"), "source-b": acceptedCandidateFor("example/b")},
	}
	for i := range members {
		candidate := provider.candidates[members[i].Registration.ID]
		candidate.Commit = members[i].Registration.Selector.Ref
		members[i].LegalAdmission.EvidenceSHA256 = writeCompositeLegalEvidence(t, repository, members[i], candidate)
	}
	manifest := json.RawMessage(`{"schema_version":1,"id":"composite","version":"1.0.0","resources":[{"kind":"skill","id":"first","source":"skills/first"},{"kind":"notice","id":"second","source":"notices/second.md"}]}`)
	return repository, provider, CompositeCheckRequest{RepositoryRoot: repository, AcquisitionDir: t.TempDir(), PackID: "composite", ProposedVersion: "1.0.0", ProposedManifest: manifest, Members: members}
}

func writeCompositeLegalEvidence(t *testing.T, repository string, member CompositeRegistrationMember, candidate Candidate) string {
	t.Helper()
	selectedRoots := make([]string, 0, len(member.Registration.Resources))
	for _, binding := range member.Registration.Resources {
		selectedRoots = append(selectedRoots, binding.UpstreamPath)
	}
	sort.Strings(selectedRoots)
	evidence := legalAdmissionEvidence{
		SchemaVersion: 1, EvidenceID: "synthetic-" + member.Registration.ID,
		DurableReference: member.LegalAdmission.EvidenceReference,
		Issuer:           "Packy synthetic fixture", EvidenceOrigin: "issue-256 synthetic fixture",
		Decision: "synthetic redistribution admitted", Candidate: LegalAdmissionCandidate{
			Repository: candidate.Repository, Commit: candidate.Commit,
			READMEBlob: strings.Repeat("c", 40), READMELength: 1, READMESHA256: strings.Repeat("d", 64),
		},
		Disposition: RedistributableDisposition,
		Rights:      []string{"copy"}, Obligations: []string{"preserve notice"}, Disclosures: []string{"synthetic fixture only"},
		Scope:    LegalAdmissionScope{SelectedRoots: selectedRoots, Exclusions: []string{}},
		Validity: "exact candidate and selected roots", Invalidation: "candidate, scope, or evidence digest changes",
	}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	writeFile(t, filepath.Join(repository, filepath.FromSlash(member.LegalAdmission.EvidenceReference)), string(raw))
	return hashBytes(raw)
}

func TestCompositeLegalAdmissionRequiresDurableExactEvidence(t *testing.T) {
	repository, provider, request := compositeFixture(t)
	engine := Engine{Source: provider, Validate: acceptingBundleValidator()}
	evidencePath := filepath.Join(repository, filepath.FromSlash(request.Members[0].LegalAdmission.EvidenceReference))
	original, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(evidencePath); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CheckComposite(context.Background(), request); err == nil || !strings.Contains(err.Error(), "durable evidence") {
		t.Fatalf("missing evidence error = %v", err)
	}
	if err := os.WriteFile(evidencePath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	wrongDigest := request
	wrongDigest.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	wrongDigest.Members[0].LegalAdmission.EvidenceSHA256 = strings.Repeat("f", 64)
	if _, err := engine.CheckComposite(context.Background(), wrongDigest); !errors.Is(err, ErrLegalAdmissionDigest) {
		t.Fatalf("wrong evidence digest error = %v", err)
	}
	var mismatched legalAdmissionEvidence
	if err := json.Unmarshal(original, &mismatched); err != nil {
		t.Fatal(err)
	}
	mismatched.Candidate.Repository = "example/other"
	mismatchedRaw, err := json.MarshalIndent(mismatched, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	mismatchedRaw = append(mismatchedRaw, '\n')
	if err := os.WriteFile(evidencePath, mismatchedRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	mismatchedRequest := request
	mismatchedRequest.Members = append([]CompositeRegistrationMember(nil), request.Members...)
	mismatchedRequest.Members[0].LegalAdmission.EvidenceSHA256 = hashBytes(mismatchedRaw)
	if _, err := engine.CheckComposite(context.Background(), mismatchedRequest); !errors.Is(err, ErrLegalAdmissionBinding) {
		t.Fatalf("candidate-mismatched evidence error = %v", err)
	}
	if err := os.WriteFile(evidencePath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := engine.CheckComposite(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, append(original, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ApplyComposite(context.Background(), CompositeApplyRequest{CompositeCheckRequest: request, Plan: plan, ClassificationEvidence: compositeClassification(plan)}); !errors.Is(err, ErrLegalAdmissionDigest) {
		t.Fatalf("changed durable evidence error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "bundle", "packs", "composite")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("changed evidence wrote target Pack: %v", err)
	}
}

func compositeClassification(plan CompositePlan) CompositeClassificationEvidence {
	return CompositeClassificationEvidence{SchemaVersion: 1, PlanID: plan.PlanID, PackID: plan.PackID, Evidence: ClassificationEvidence{
		PackID: plan.PackID, Classifier: ClassifierIdentity{Type: ClassifierAI, ID: "synthetic"}, Rationale: "initial composite Pack admission",
		CurrentVersion: "0.0.0", ProposedVersion: plan.ProposedVersion, ChangedAspects: []string{"initial complete Pack generation"},
		MechanicalFloor: LevelMajor, FinalLevel: LevelMajor,
		Migration: "initial generation has no predecessor", RequiredActions: []string{"review initial complete Pack contract"},
	}}
}
