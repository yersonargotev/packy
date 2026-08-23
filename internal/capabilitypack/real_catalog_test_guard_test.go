package capabilitypack

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type realCatalogExceptionCategory string

const (
	realCatalogPublicContract    realCatalogExceptionCategory = "public contract"
	realCatalogGeneratedEvidence realCatalogExceptionCategory = "generated catalog/docs/admission"
	realCatalogIntegrationSmoke  realCatalogExceptionCategory = "one explicit integration smoke"
)

type realCatalogException struct {
	category      realCatalogExceptionCategory
	justification string
}

// Exceptions are deliberately scoped to a complete Test function. A new use of
// the live catalog must either fit the function's stated contract or receive its
// own reviewed exception; line-number exceptions become stale after harmless edits.
var realCatalogTestExceptions = map[string]realCatalogException{
	"internal/capabilitypack/argote_pack_test.go:TestCheckedInArgotePackHasCollisionFreeNativeRoots": {
		category:      realCatalogPublicContract,
		justification: "Protects Argote's checked-in version, resource roots, native surface bindings, and published guidance contract.",
	},
	"internal/capabilitypack/pstack_pack_test.go:TestCheckedInPstackPackPreservesCompatibilityMatrix": {
		category:      realCatalogPublicContract,
		justification: "Protects pstack's checked-in 81-case resource-selection and three-surface compatibility matrix.",
	},
	"internal/capabilitypack/content_validation_test.go:TestCheckedInCurrentManifestsOmitRetiredContractTerms": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects every checked-in manifest against retired schema and capability terms.",
	},
	"internal/ci/issue672_engram_source_test.go:TestIssue672EngramPackUsesExactUpstreamSkill": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the admitted Engram manifest and its exact generated upstream resource evidence.",
	},
	"internal/claudesmoke/runner_test.go:TestAllowedCommandRejectsInteractiveClaudeAndUnknownPacky": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the Addy qualification runner's exact release-command allowlist and rejection boundary.",
	},
	"internal/claudesmoke/runner_test.go:TestEvidenceSchemaV3ProvesInitializationThenExplicitActivation": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects Addy publication evidence ordering from initialization through explicit activation.",
	},
	"internal/claudesmoke/runner_test.go:TestRunAllowedUsesCanonicalConfiguredPackyIdentity": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects validation of the configured Packy executable identity in Addy qualification evidence.",
	},
	"internal/claudesmoke/runner_test.go:TestRunInteractiveRestrictedProvidesTTYForExplicitActivation": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the explicit interactive Addy activation required by the publication qualification flow.",
	},
	"internal/claudesmoke/runner_test.go:TestValidateEvidenceRejectsTampering": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects content-bound Addy qualification evidence against command and activation tampering.",
	},
	"internal/claudesmoke/runner_test.go:TestValidationFailureStillWritesDiagnosticEvidence": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects diagnostic Addy qualification evidence when release validation fails.",
	},
	"internal/claudesmoke/addy_qualification_test.go:TestAddyQualificationCanonicalOutput": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the canonical Addy qualification artifact emitted for release evidence.",
	},
	"internal/claudesmoke/addy_qualification_test.go:TestAddyQualificationProductionBoundary": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the production boundary that binds real Addy qualification observations.",
	},
	"internal/claudesmoke/addy_qualification_test.go:TestAddyQualificationRejectsOneFactSafetyFailures": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects Addy qualification rejection when any required release-safety fact fails.",
	},
	"internal/claudesmoke/addy_qualification_test.go:TestBindAddyQualificationUsesInSandboxObservations": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects binding of sandboxed Addy lifecycle observations into qualification evidence.",
	},
	"internal/claudesmoke/addy_qualification_test.go:TestProductionAddyQualificationRejectsStaleCollection": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects production Addy qualification against stale collected release evidence.",
	},
	"internal/claudesmoke/release_evidence_test.go:TestValidateReleaseAddyQualificationMatrixRequiresOneSyntheticRun": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the release evidence matrix combining real Addy qualification with its required synthetic run.",
	},
	"internal/claudesmoke/release_evidence_test.go:TestValidateReleaseEvidenceMatrixUsesCanonicalEvidence": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects canonical Addy qualification evidence in the complete release validation matrix.",
	},
	"internal/cli/claude_pack_tracer_test.go:TestClaudeMattyTracerActivatesStatusesAndDeactivatesInSandbox": {
		category:      realCatalogIntegrationSmoke,
		justification: "Runs the one end-to-end real-catalog smoke across Matty's Claude activation, status, update, and deactivation lifecycle.",
	},
	"internal/cli/issue579_pack_show_inventory_test.go:TestPackShowHumanRendersDeterministicDescriptiveInventory": {
		category:      realCatalogPublicContract,
		justification: "Protects Engram's checked-in descriptive resource inventory and deterministic human rendering.",
	},
	"internal/cli/issue579_pack_show_inventory_test.go:TestPackShowJSONV5IncludesDescriptiveInventory": {
		category:      realCatalogPublicContract,
		justification: "Protects Engram's checked-in resource inventory in the public structured show contract.",
	},
	"internal/cli/issue683_pack_list_json_test.go:TestPackListHumanOutputRemainsUnchanged": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the complete checked-in catalog's generated human list ordering, versions, descriptions, and surfaces.",
	},
	"internal/cli/issue683_pack_list_json_test.go:TestPackListJSONReportsValidatedCatalogInCanonicalOrder": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects the generated JSON catalog against the validated checked-in catalog and its canonical ordering.",
	},
	"internal/cli/pack_test.go:TestArgoteActivationPreviewIsApplicableOnEverySurface": {
		category:      realCatalogPublicContract,
		justification: "Protects Argote's reviewed all-surface activation and collision-free native projection contract.",
	},
	"internal/cli/pack_test.go:TestArgoteCodexActivationSurvivesReceiptReloadAndCanBeDeactivated": {
		category:      realCatalogPublicContract,
		justification: "Protects Argote's checked-in Codex projection, receipt reload, status, and deactivation contract.",
	},
	"internal/cli/pack_test.go:TestPackListUsesOneCapturedWorkstationForSkillSource": {
		category:      realCatalogPublicContract,
		justification: "Protects repository Skill Source discovery and the single captured-workstation boundary while enumerating the checked-in catalog.",
	},
	"internal/cli/pack_test.go:TestCurrentMattyActivationProjectsSurfaceCapabilities": {
		category:      realCatalogPublicContract,
		justification: "Protects Matty's current reviewed surface-capability projections rather than generic lifecycle behavior.",
	},
	"internal/cli/pack_test.go:TestMattyCodexActivationDryRunPreservesCurrentPublicContract": {
		category:      realCatalogPublicContract,
		justification: "Protects Matty's current checked-in Codex resource inventory and retired-projection exclusions.",
	},
	"internal/cli/pack_test.go:TestMattyOpenCodeActivationDryRunPreservesCurrentPublicContract": {
		category:      realCatalogPublicContract,
		justification: "Protects Matty's current checked-in OpenCode prompt and skill projection contract.",
	},
	"internal/cli/pack_test.go:TestPackActivateEngramAcquiresOnlyWhenExecutableIsMissing": {
		category:      realCatalogPublicContract,
		justification: "Protects Engram's reviewed external-executable acquisition capability and supported-surface contract.",
	},
	"internal/cli/pack_test.go:TestPackActivateEngramDryRunShowsOnlyReviewedSkillAndNoEffects": {
		category:      realCatalogPublicContract,
		justification: "Protects Engram's exact reviewed skill projection and retired setup exclusions.",
	},
	"internal/cli/pack_test.go:TestPackActivateEngramInstallsOnlyTheReviewedSkill": {
		category:      realCatalogPublicContract,
		justification: "Protects Engram's checked-in installed skill and absence of retired host-setup artifacts.",
	},
	"internal/cli/pack_test.go:TestRealPackCatalogListAndShowPreserveArgoteEngramMattyPublicContracts": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects reviewed catalog list/show descriptions, inventories, and supported surfaces for real Packs.",
	},
	"internal/cli/pstack_pack_test.go:TestPstackActivationPreviewsProjectThroughEverySurfaceAdapter": {
		category:      realCatalogPublicContract,
		justification: "Protects pstack's public all-surface projection and dependency-closing preview contract.",
	},
	"internal/cli/tui_backend_test.go:TestTUIProductionBackendUsesPackyOwnersWithoutMutatingState": {
		category:      realCatalogPublicContract,
		justification: "Protects the selectable Orchestrate catalog entry and its manifest-owned description, resources, and supported-surface matrix in the production TUI backend.",
	},
	"internal/packsync/reconfiguration_test.go:TestCheckedInIssueDeliveryReconfigurationAcceptsExactSelectedReleaseRevision": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects Issue Delivery's checked-in legacy reconfiguration and selected-release admission evidence.",
	},
	"internal/packsync/reconfiguration_test.go:TestCheckedInOrchestrateSupportsMetadataOnlyReconfiguration": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects Orchestrate's checked-in metadata-only reconfiguration admission contract.",
	},
	"internal/packsync/reconfiguration_test.go:TestMetadataOnlyReconfigurationPublishesNewGenerationWithoutChangingProvenance": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects legacy admission provenance while reconfiguring Orchestrate's checked-in manifest generation.",
	},
	"internal/packsync/reconfiguration_test.go:TestMetadataOnlyReconfigurationRecoveryExposesOnlyCompleteGenerations": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects complete-generation recovery for Orchestrate's checked-in legacy admission evidence.",
	},
	"internal/packsync/single_source_admission_test.go:TestCheckSingleSourceAdmissionRejectsInvalidOrConflictingRequests": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects rejection gates against Orchestrate's reviewed single-source admission manifest.",
	},
	"internal/packsync/single_source_admission_test.go:TestSingleSourceAdmissionApplyMaterializesCompleteGeneration": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects complete materialization of Orchestrate's reviewed single-source admission generation.",
	},
	"internal/packsync/single_source_admission_test.go:TestSingleSourceAdmissionApplyRejectsFreshnessAndValidationFailuresWithoutAdmission": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects atomic rejection of stale or invalid Orchestrate admission evidence.",
	},
	"internal/packsync/single_source_admission_test.go:TestSingleSourceAdmissionBootstrapsFirstSourceFromReviewedBundle": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects bootstrap from Orchestrate's reviewed bundle as legacy admission authority.",
	},
	"internal/packsync/single_source_admission_test.go:TestSingleSourceAdmissionReplacementFaultRecoversOnlyCompleteGenerations": {
		category:      realCatalogGeneratedEvidence,
		justification: "Protects atomic recovery of Orchestrate's reviewed single-source admission generation.",
	},
}

type realCatalogInventory struct {
	packIDs         map[string]struct{}
	resourceAliases map[realCatalogResourceAlias]struct{}
}

type realCatalogResourceAlias struct {
	kind string
	id   string
	name string
}

type realCatalogFinding struct {
	test   string
	detail string
}

func TestGenericTestsDoNotDependOnTheRealPackCatalog(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := loadRealCatalogInventory(filepath.Join(repositoryRoot, "bundle", "packs"))
	if err != nil {
		t.Fatal(err)
	}

	findings, err := scanRealCatalogDependencies(repositoryRoot, inventory)
	if err != nil {
		t.Fatal(err)
	}
	validateRealCatalogExceptions(t, findings)

	var unclassified []string
	for _, finding := range findings {
		if _, ok := realCatalogTestExceptions[finding.test]; !ok {
			unclassified = append(unclassified, finding.test+": "+finding.detail)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("generic tests depend on the live Pack catalog; replace each dependency with a synthetic fixture or add an exact justified exception:\n%s", strings.Join(unclassified, "\n"))
	}
}

func TestRealCatalogDependencyScannerAttributesHelperDependenciesToTests(t *testing.T) {
	source := `package sample
import (
	"os"
	"path/filepath"
	"testing"
)
const catalogPackID = "live-"+"pack"
var packagePackID = "live-pack"
func catalogHelper() string { return filepath.Join("bundle", "packs", "live-pack", "pack.json") }
func literalManifest() string { return "bundle/packs/live-pack/pack.json" }
func lifecycleArgs(packID string) []string { return []string{"activate", packID} }
func labels() []string { return []string{"activate", "live-pack"} }
func syntheticArgs(packID string) []string { _ = labels(); return []string{"activate", packID} }
func executeArgs(t *testing.T, args []string) { executeCommand(t, NewRootCommand(Options{}), args...) }
func identity(packID string) string { return packID }
func TestThroughHelper(t *testing.T) { _, _ = os.ReadFile(catalogHelper()) }
func TestDirectManifestLiteral(t *testing.T) { _, _ = os.ReadFile("bundle/packs/live-pack/pack.json") }
func TestLocalManifestLiteral(t *testing.T) { manifest := "bundle/packs/live-pack/pack.json"; _, _ = os.Open(manifest) }
func TestHelperManifestLiteral(t *testing.T) { _, _ = os.Open(literalManifest()) }
func TestSplitPackIDManifest(t *testing.T) { _, _ = os.ReadFile(filepath.Join("bundle", "packs", "live-"+"pack", "pack.json")) }
func TestSplitMattyManifest(t *testing.T) { _, _ = os.ReadFile(filepath.Join("bundle", "packs", "mat"+"ty", "pack.json")) }
func TestTemporaryManifest(t *testing.T) { _, _ = os.ReadFile(filepath.Join(t.TempDir(), "bundle", "packs", "live-pack", "pack.json")) }
func TestTypedLifecycle(t *testing.T) { _ = ActivationRequest{PackID: "live-pack"} }
func TestCLILifecycle(t *testing.T) { executeCommand(t, NewRootCommand(Options{}), "activate", "live-"+"pack", "--surface", "codex") }
func TestVariableCLILifecycle(t *testing.T) { packID := "live-"+"pack"; executeCommand(t, NewRootCommand(Options{}), "activate", packID, "--surface", "codex") }
func TestVariableTypedLifecycle(t *testing.T) { packID := "live-pack"; _ = ActivationRequest{PackID: packID} }
func TestVariableLookup(t *testing.T) { packID := "live-pack"; catalog, _ := Discover(ctx, "bundle"); catalog.Show(ctx, packID) }
func TestPackageConstantLookup(t *testing.T) { catalog, _ := Discover(ctx, "bundle"); catalog.Show(ctx, catalogPackID) }
func TestPackageVariableLookup(t *testing.T) { catalog, _ := Discover(ctx, "bundle"); catalog.Show(ctx, packagePackID) }
func TestVariableResourceAlias(t *testing.T) { kind, id, name := "skill", "guide", "live-guide"; _ = SurfaceAlias{Kind: kind, ID: id, Name: name} }
func TestVariableManifestPath(t *testing.T) { packID := "live-pack"; manifest := filepath.Join("bundle", "packs", packID, "pack.json"); os.ReadFile(manifest) }
func TestRealResourceAlias(t *testing.T) { _ = SurfaceAlias{Kind: "skill", ID: "guide", Name: "live-guide"} }
func TestRunClosure(t *testing.T) { t.Run("live", func(t *testing.T) { executeCommand(t, NewRootCommand(Options{}), "activate", "live-pack") }) }
func TestVariadicHelperReturn(t *testing.T) { executeArgs(t, lifecycleArgs("live-pack")) }
func TestTableRangeAndIndex(t *testing.T) {
	cases := []struct{ args []string }{{args: lifecycleArgs("live-pack")}}
	for index := range cases { executeArgs(t, cases[index].args) }
}
func TestBranchSelectedArguments(t *testing.T) {
	packID := "synthetic-pack"
	if t.Name() != "" { packID = "live-pack" }
	executeCommand(t, NewRootCommand(Options{}), "activate", packID)
}
func TestUnrelatedLiteral(t *testing.T) { _ = "live-pack" }
func TestUnrelatedVariable(t *testing.T) { packID := "live-pack"; _ = packID }
func TestSyntheticVariable(t *testing.T) { packID := "synthetic-pack"; executeCommand(t, NewRootCommand(Options{}), "activate", packID) }
func TestUnrelatedSlice(t *testing.T) { _ = []string{"activate", "live-pack"} }
func TestUnrelatedShow(t *testing.T) { unrelated := unrelatedView{}; unrelated.Show(ctx, "live-pack") }
func TestCallSensitiveLifecycle(t *testing.T) { _ = identity("live-pack"); executeCommand(t, NewRootCommand(Options{}), "activate", identity("synthetic-pack")) }
func TestDiscardedHelperSequence(t *testing.T) { executeArgs(t, syntheticArgs("synthetic-pack")) }
`
	inventory := realCatalogInventory{
		packIDs: map[string]struct{}{"live-pack": {}, "matty": {}},
		resourceAliases: map[realCatalogResourceAlias]struct{}{
			{kind: "skill", id: "guide", name: "live-guide"}: {},
		},
	}
	findings, err := scanRealCatalogSource("sample_test.go", []byte(source), inventory)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, finding := range findings {
		got[finding.test] = true
	}
	for _, want := range []string{
		"sample_test.go:TestThroughHelper",
		"sample_test.go:TestDirectManifestLiteral",
		"sample_test.go:TestLocalManifestLiteral",
		"sample_test.go:TestHelperManifestLiteral",
		"sample_test.go:TestSplitPackIDManifest",
		"sample_test.go:TestSplitMattyManifest",
		"sample_test.go:TestTypedLifecycle",
		"sample_test.go:TestCLILifecycle",
		"sample_test.go:TestVariableCLILifecycle",
		"sample_test.go:TestVariableTypedLifecycle",
		"sample_test.go:TestVariableLookup",
		"sample_test.go:TestPackageConstantLookup",
		"sample_test.go:TestPackageVariableLookup",
		"sample_test.go:TestVariableResourceAlias",
		"sample_test.go:TestVariableManifestPath",
		"sample_test.go:TestRealResourceAlias",
		"sample_test.go:TestRunClosure",
		"sample_test.go:TestVariadicHelperReturn",
		"sample_test.go:TestTableRangeAndIndex",
		"sample_test.go:TestBranchSelectedArguments",
	} {
		if !got[want] {
			t.Errorf("missing finding for %s: %+v", want, findings)
		}
	}
	if got["sample_test.go:TestUnrelatedLiteral"] {
		t.Fatalf("unrelated real-ID literal was classified as a dependency: %+v", findings)
	}
	for _, unwanted := range []string{
		"sample_test.go:TestUnrelatedVariable",
		"sample_test.go:TestSyntheticVariable",
		"sample_test.go:TestUnrelatedSlice",
		"sample_test.go:TestUnrelatedShow",
		"sample_test.go:TestCallSensitiveLifecycle",
		"sample_test.go:TestDiscardedHelperSequence",
		"sample_test.go:TestTemporaryManifest",
	} {
		if got[unwanted] {
			t.Fatalf("unrelated or synthetic variable was classified as a dependency: %s: %+v", unwanted, findings)
		}
	}
}

func TestRealCatalogDependencyScannerFollowsPackageHelpersAndMethodsAndClassifiesLiveListOnly(t *testing.T) {
	sources := map[string][]byte{
		"sample/helpers_test.go": []byte(`package sample
import (
	"path/filepath"
	"testing"
)
func crossFileManifest() string { return filepath.Join("bundle", "packs", "live-pack", "pack.json") }
func manifestFor(packID string) string { return filepath.Join("bundle", "packs", packID, "pack.json") }
func requestFor(packID string) ActivationRequest { return ActivationRequest{PackID: packID} }
func aliasFor(kind, id, name string) SurfaceAlias { return SurfaceAlias{Kind: kind, ID: id, Name: name} }
func showPack(catalog Catalog, packID string) { catalog.Show(ctx, packID) }
func executeLifecycle(t *testing.T, verb, packID string) { executeCommand(t, NewRootCommand(Options{}), verb, packID) }
type fixture struct{}
func (fixture) live() string { return filepath.Join("bundle", "packs", "live-pack", "pack.json") }
func (fixture) activate(t *testing.T, packID string) { executeLifecycle(t, "activate", packID) }
type syntheticFixture struct{}
func (syntheticFixture) live() string { return "temporary synthetic bundle" }
func newSyntheticFixture() syntheticFixture { return syntheticFixture{} }
func repositoryRoot() string { root, _ := filepath.Abs(filepath.Join("..", "..")); return root }
func liveOptions(root string) Options { return Options{Getwd: func() (string, error) { return root, nil }} }
func listWithOptions(t *testing.T, options Options) { executeCommand(t, NewRootCommand(options), "list") }
func optionsIdentity(options Options) Options { return options }
func listCatalog(catalog Catalog) { _, _ = catalog.ListCurrent(ctx) }
func syntheticOptions(t *testing.T) Options {
	bundleRoot := filepath.Join(t.TempDir(), "bundle")
	return Options{Env: MapEnv{"PACKY_SKILLS_SOURCE": filepath.Join(bundleRoot, "skills")}}
}`),
		"sample/scenarios_test.go": []byte(`package sample
import (
	"os"
	"path/filepath"
	"testing"
)
func TestCrossFileHelper(t *testing.T) { _, _ = os.ReadFile(crossFileManifest()) }
func TestParameterizedManifestHelper(t *testing.T) { _, _ = os.ReadFile(manifestFor("live-pack")) }
func TestParameterizedTypedRequestHelper(t *testing.T) { _ = requestFor("live-pack") }
func TestParameterizedAliasHelper(t *testing.T) { _ = aliasFor("skill", "guide", "live-guide") }
func TestParameterizedLookupHelper(t *testing.T) { catalog, _ := Discover(ctx, "bundle"); showPack(catalog, "live-pack") }
func TestParameterizedLifecycleHelper(t *testing.T) { executeLifecycle(t, "activate", "live-pack") }
func TestReceiverMethod(t *testing.T) { _, _ = os.ReadFile(fixture{}.live()) }
func TestVariableReceiverMethod(t *testing.T) { f := fixture{}; _, _ = os.ReadFile(f.live()) }
func TestParameterizedReceiverMethod(t *testing.T) { fixture{}.activate(t, "live-pack") }
func TestSyntheticFactoryReceiverMethod(t *testing.T) { _, _ = os.ReadFile(newSyntheticFixture().live()) }
func TestLiveCatalogList(t *testing.T) { root := repositoryRoot(); options := liveOptions(root); listWithOptions(t, options) }
func TestTypedLiveListCurrent(t *testing.T) { root := repositoryRoot(); catalog, _ := Discover(ctx, filepath.Join(root, "bundle")); listCatalog(catalog) }
func TestDirectTypedLiveListCurrent(t *testing.T) { catalog := Catalog{bundleRoot: filepath.Join(repositoryRoot(), "bundle")}; catalog.ListCurrent(ctx) }
func TestSyntheticCatalogList(t *testing.T) { executeCommand(t, NewRootCommand(syntheticOptions(t)), "list") }
func TestTypedSyntheticListCurrent(t *testing.T) { catalog, _ := Discover(ctx, filepath.Join(t.TempDir(), "bundle")); listCatalog(catalog) }
func TestDirectTypedSyntheticListCurrent(t *testing.T) { catalog := Catalog{bundleRoot: filepath.Join(t.TempDir(), "bundle")}; catalog.ListCurrent(ctx) }
func TestDiscoverLive(t *testing.T) { _, _ = Discover(ctx, filepath.Join(repositoryRoot(), "bundle")) }
func TestDiscoverTemporary(t *testing.T) { _, _ = Discover(ctx, filepath.Join(t.TempDir(), "bundle")) }
func TestDiscoverForDurableIntentsLive(t *testing.T) { _, _ = DiscoverForDurableIntents(ctx, filepath.Join(repositoryRoot(), "bundle")) }
func TestDiscoverForDurableIntentsTemporary(t *testing.T) { _, _ = DiscoverForDurableIntents(ctx, filepath.Join(t.TempDir(), "bundle")) }
func TestValidatePortableContentLive(t *testing.T) { _ = ValidatePortableContent(filepath.Join(repositoryRoot(), "bundle")) }
func TestValidatePortableContentTemporary(t *testing.T) { _ = ValidatePortableContent(filepath.Join(t.TempDir(), "bundle")) }
func TestCatalogListDetailsLive(t *testing.T) { catalog := Catalog{bundleRoot: filepath.Join(repositoryRoot(), "bundle")}; _, _ = catalog.ListDetails(ctx) }
func TestCatalogListDetailsTemporary(t *testing.T) { catalog := Catalog{bundleRoot: filepath.Join(t.TempDir(), "bundle")}; _, _ = catalog.ListDetails(ctx) }
func TestCatalogShowDetailLive(t *testing.T) { _, _ = (Catalog{}).ShowDetail(ctx, "live-pack") }
func TestCatalogShowDetailSynthetic(t *testing.T) { _, _ = (Catalog{}).ShowDetail(ctx, "synthetic-pack") }
func TestUnrelatedShowDetail(t *testing.T) { unrelated := unrelatedView{}; unrelated.ShowDetail(ctx, "live-pack") }
func TestLoadCurrentManifestLive(t *testing.T) {
	root := repositoryRoot()
	manifest := filepath.Join(root, "bundle", "packs", "live-pack", "pack.json")
	_, _ = LoadCurrentManifest(manifest, filepath.Join(root, "bundle"), true)
}
func TestLoadCurrentManifestTemporary(t *testing.T) {
	root := t.TempDir()
	_, _ = LoadCurrentManifest(filepath.Join(root, "bundle", "packs", "live-pack", "pack.json"), filepath.Join(root, "bundle"), true)
}
func TestLoadCurrentManifestSynthetic(t *testing.T) {
	root := repositoryRoot()
	_, _ = LoadCurrentManifest(filepath.Join(root, "bundle", "packs", "synthetic-pack", "pack.json"), filepath.Join(root, "bundle"), true)
}
func TestValidatePackContentLive(t *testing.T) { _, _ = ValidatePackContent(filepath.Join(repositoryRoot(), "bundle"), "live-pack") }
func TestValidatePackContentTemporary(t *testing.T) { _, _ = ValidatePackContent(filepath.Join(t.TempDir(), "bundle"), "live-pack") }
func TestValidatePackContentSynthetic(t *testing.T) { _, _ = ValidatePackContent(filepath.Join(repositoryRoot(), "bundle"), "synthetic-pack") }
func TestValidatePackContentLiveDirectory(t *testing.T) {
	bundleRoot := filepath.Join(repositoryRoot(), "bundle")
	_, _ = ValidatePackContent(bundleRoot, filepath.Join(bundleRoot, "packs", "live-pack"))
}
func TestValidatePackContentTemporaryDirectory(t *testing.T) {
	_ = repositoryRoot()
	bundleRoot := filepath.Join(t.TempDir(), "bundle")
	_, _ = ValidatePackContent(bundleRoot, filepath.Join(bundleRoot, "packs", "live-pack"))
}
func TestValidatePackContentSyntheticDirectory(t *testing.T) {
	bundleRoot := filepath.Join(repositoryRoot(), "bundle")
	_, _ = ValidatePackContent(bundleRoot, filepath.Join(bundleRoot, "packs", "synthetic-pack"))
}
func TestValidatePackContentRelativeDirectory(t *testing.T) {
	_, _ = ValidatePackContent(filepath.Join("..", "..", "bundle"), filepath.Join("..", "..", "bundle", "packs", "matty"))
}
func TestValidatePackContentSyntheticRelativeDirectory(t *testing.T) {
	_, _ = ValidatePackContent(filepath.Join("..", "..", "bundle"), filepath.Join("..", "..", "bundle", "packs", "synthetic-pack"))
}
func TestValidatePackContentUnrelatedRelativeDirectory(t *testing.T) {
	_, _ = ValidatePackContent(filepath.Join("fixtures", "bundle"), filepath.Join("fixtures", "bundle", "packs", "live-pack"))
}
func TestUnrelatedRootAndSyntheticCatalogList(t *testing.T) { _ = repositoryRoot(); listWithOptions(t, syntheticOptions(t)) }
func TestCallSensitiveOptionsList(t *testing.T) { root := repositoryRoot(); _ = optionsIdentity(liveOptions(root)); listWithOptions(t, optionsIdentity(syntheticOptions(t))) }
func TestReceiverShadowing(t *testing.T) {
	f := fixture{}
	_, _ = os.ReadFile(f.live())
	t.Run("synthetic", func(t *testing.T) { f := syntheticFixture{}; _, _ = os.ReadFile(f.live()) })
}
func TestReceiverShadowingDoesNotReuseOuterType(t *testing.T) {
	f := fixture{}
	t.Run("synthetic", func(t *testing.T) { f := syntheticFixture{}; _, _ = os.ReadFile(f.live()) })
	_ = f
}
`),
	}
	inventory := realCatalogInventory{
		packIDs: map[string]struct{}{"live-pack": {}, "matty": {}},
		resourceAliases: map[realCatalogResourceAlias]struct{}{
			{kind: "skill", id: "guide", name: "live-guide"}: {},
		},
	}
	findings, err := scanRealCatalogSources(sources, inventory)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, finding := range findings {
		got[finding.test] = true
	}
	for _, want := range []string{
		"sample/scenarios_test.go:TestCrossFileHelper",
		"sample/scenarios_test.go:TestParameterizedManifestHelper",
		"sample/scenarios_test.go:TestParameterizedTypedRequestHelper",
		"sample/scenarios_test.go:TestParameterizedAliasHelper",
		"sample/scenarios_test.go:TestParameterizedLookupHelper",
		"sample/scenarios_test.go:TestParameterizedLifecycleHelper",
		"sample/scenarios_test.go:TestReceiverMethod",
		"sample/scenarios_test.go:TestVariableReceiverMethod",
		"sample/scenarios_test.go:TestParameterizedReceiverMethod",
		"sample/scenarios_test.go:TestLiveCatalogList",
		"sample/scenarios_test.go:TestTypedLiveListCurrent",
		"sample/scenarios_test.go:TestDirectTypedLiveListCurrent",
		"sample/scenarios_test.go:TestDiscoverLive",
		"sample/scenarios_test.go:TestDiscoverForDurableIntentsLive",
		"sample/scenarios_test.go:TestValidatePortableContentLive",
		"sample/scenarios_test.go:TestCatalogListDetailsLive",
		"sample/scenarios_test.go:TestCatalogShowDetailLive",
		"sample/scenarios_test.go:TestLoadCurrentManifestLive",
		"sample/scenarios_test.go:TestValidatePackContentLive",
		"sample/scenarios_test.go:TestValidatePackContentLiveDirectory",
		"sample/scenarios_test.go:TestValidatePackContentRelativeDirectory",
		"sample/scenarios_test.go:TestReceiverShadowing",
	} {
		if !got[want] {
			t.Errorf("missing finding for %s: %+v", want, findings)
		}
	}
	if got["sample/scenarios_test.go:TestSyntheticCatalogList"] {
		t.Fatalf("synthetic temporary catalog list was classified as live: %+v", findings)
	}
	if got["sample/scenarios_test.go:TestTypedSyntheticListCurrent"] {
		t.Fatalf("synthetic Catalog.ListCurrent was classified as live: %+v", findings)
	}
	if got["sample/scenarios_test.go:TestDirectTypedSyntheticListCurrent"] {
		t.Fatalf("direct synthetic Catalog.ListCurrent was classified as live: %+v", findings)
	}
	for _, unwanted := range []string{
		"sample/scenarios_test.go:TestLoadCurrentManifestTemporary",
		"sample/scenarios_test.go:TestLoadCurrentManifestSynthetic",
		"sample/scenarios_test.go:TestValidatePackContentTemporary",
		"sample/scenarios_test.go:TestValidatePackContentSynthetic",
		"sample/scenarios_test.go:TestValidatePackContentTemporaryDirectory",
		"sample/scenarios_test.go:TestValidatePackContentSyntheticDirectory",
		"sample/scenarios_test.go:TestValidatePackContentSyntheticRelativeDirectory",
		"sample/scenarios_test.go:TestValidatePackContentUnrelatedRelativeDirectory",
		"sample/scenarios_test.go:TestDiscoverTemporary",
		"sample/scenarios_test.go:TestDiscoverForDurableIntentsTemporary",
		"sample/scenarios_test.go:TestValidatePortableContentTemporary",
		"sample/scenarios_test.go:TestCatalogListDetailsTemporary",
		"sample/scenarios_test.go:TestCatalogShowDetailSynthetic",
		"sample/scenarios_test.go:TestUnrelatedShowDetail",
	} {
		if got[unwanted] {
			t.Fatalf("synthetic content validation was classified as live: %s: %+v", unwanted, findings)
		}
	}
	if got["sample/scenarios_test.go:TestUnrelatedRootAndSyntheticCatalogList"] {
		t.Fatalf("unrelated repository root was correlated with synthetic list options: %+v", findings)
	}
	if got["sample/scenarios_test.go:TestCallSensitiveOptionsList"] {
		t.Fatalf("an unrelated live options helper call contaminated a synthetic list call: %+v", findings)
	}
	if got["sample/scenarios_test.go:TestSyntheticFactoryReceiverMethod"] {
		t.Fatalf("synthetic same-name receiver inherited the live method finding: %+v", findings)
	}
	if got["sample/scenarios_test.go:TestReceiverShadowingDoesNotReuseOuterType"] {
		t.Fatalf("inner synthetic receiver inherited the shadowed outer receiver type: %+v", findings)
	}
}

func TestRealCatalogDependencyScannerSelectsEveryTestPackage(t *testing.T) {
	workspace := t.TempDir()
	for path, source := range map[string]string{
		"candidate/scenario_test.go": `package candidate
func TestCandidate() { executeCommand(nil, nil, "activate", "live-pack") }
`,
		"unrelated/scenario_test.go": `package unrelated
func TestUnrelated() { values := []string{"temporary", "fixture"}; _ = values }
`,
	} {
		target := filepath.Join(workspace, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	patterns, err := repositoryTestPackagePatterns(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(patterns, ","), "./candidate,./unrelated"; got != want {
		t.Fatalf("repository test package patterns = %q, want %q", got, want)
	}
}

func loadRealCatalogInventory(packsRoot string) (realCatalogInventory, error) {
	inventory := realCatalogInventory{packIDs: map[string]struct{}{}, resourceAliases: map[realCatalogResourceAlias]struct{}{}}
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		return inventory, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(packsRoot, entry.Name(), "pack.json"))
		if err != nil {
			return inventory, err
		}
		var manifest struct {
			ID        string `json:"id"`
			Resources []struct {
				Kind     string `json:"kind"`
				ID       string `json:"id"`
				Bindings []struct {
					Name string `json:"name"`
				} `json:"bindings"`
			} `json:"resources"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return inventory, fmt.Errorf("decode %s: %w", entry.Name(), err)
		}
		if manifest.ID == "" || manifest.ID != entry.Name() {
			return inventory, fmt.Errorf("catalog manifest %s has mismatched id %q", entry.Name(), manifest.ID)
		}
		inventory.packIDs[manifest.ID] = struct{}{}
		for _, resource := range manifest.Resources {
			for _, binding := range resource.Bindings {
				if binding.Name != "" {
					inventory.resourceAliases[realCatalogResourceAlias{kind: resource.Kind, id: resource.ID, name: binding.Name}] = struct{}{}
				}
			}
		}
	}
	return inventory, nil
}

func scanRealCatalogDependencies(repositoryRoot string, inventory realCatalogInventory) ([]realCatalogFinding, error) {
	return scanRealCatalogWorkspace(repositoryRoot, inventory, false)
}

func scanRealCatalogSource(path string, source []byte, inventory realCatalogInventory) ([]realCatalogFinding, error) {
	return scanRealCatalogSources(map[string][]byte{path: source}, inventory)
}

func scanRealCatalogSources(sources map[string][]byte, inventory realCatalogInventory) ([]realCatalogFinding, error) {
	workspace, err := os.MkdirTemp("", "packy-real-catalog-ssa-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workspace)
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module github.com/yersonargotev/packy\n\ngo 1.25.0\n"), 0o600); err != nil {
		return nil, err
	}
	for path, source := range sources {
		target := filepath.Join(workspace, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, source, 0o600); err != nil {
			return nil, err
		}
	}
	stub := `package sample
import (
	"context"
	"testing"
)
var ctx = context.Background()
type Options struct { Env MapEnv; Getwd func() (string, error) }
type MapEnv map[string]string
type rootCommand struct{}
func NewRootCommand(Options) *rootCommand { return &rootCommand{} }
func executeCommand(*testing.T, *rootCommand, ...string) (string, error) { return "", nil }
type ActivationRequest struct { PackID string }
type SurfaceAlias struct { Kind, ID, Name string }
type Catalog struct{ bundleRoot string }
type Pack struct{}
type CatalogDetail struct{}
func Discover(_ context.Context, bundleRoot string) (Catalog, error) { return Catalog{bundleRoot: bundleRoot}, nil }
func DiscoverForDurableIntents(_ context.Context, bundleRoot string) (Catalog, error) { return Catalog{bundleRoot: bundleRoot}, nil }
func (Catalog) Show(context.Context, string) {}
func (Catalog) ListCurrent(context.Context) ([]string, error) { return nil, nil }
func (Catalog) ListDetails(context.Context) ([]CatalogDetail, error) { return nil, nil }
func (Catalog) ShowDetail(context.Context, string) (CatalogDetail, error) { return CatalogDetail{}, nil }
func LoadCurrentManifest(string, string, bool) (Pack, error) { return Pack{}, nil }
func ValidatePackContent(string, string) (Pack, error) { return Pack{}, nil }
func ValidatePortableContent(string) error { return nil }
type unrelatedView struct{}
func (unrelatedView) Show(context.Context, string) {}
func (unrelatedView) ShowDetail(context.Context, string) {}
`
	for path := range sources {
		directory := filepath.Dir(filepath.Join(workspace, filepath.FromSlash(path)))
		stubPath := filepath.Join(directory, "scanner_stubs_test.go")
		if _, err := os.Stat(stubPath); err == nil {
			continue
		}
		if err := os.WriteFile(stubPath, []byte(stub), 0o600); err != nil {
			return nil, err
		}
	}
	return scanRealCatalogWorkspace(workspace, inventory, true)
}

type realCatalogSSAAnalyzer struct {
	root         string
	fset         *token.FileSet
	inventory    realCatalogInventory
	fixture      bool
	storesByRoot map[ssaRootKey][]*ssa.Store
}

type realCatalogSSAContext struct {
	analyzer    *realCatalogSSAAnalyzer
	root        *ssa.Function
	functions   map[*ssa.Function]bool
	calls       map[*ssa.Function][]*ssa.CallCommon
	closures    map[*ssa.Function][]*ssa.MakeClosure
	activeCalls map[*ssa.Function]*ssa.CallCommon
}

type ssaRootKey struct {
	value  ssa.Value
	global string
}

func scanRealCatalogWorkspace(root string, inventory realCatalogInventory, fixture bool) ([]realCatalogFinding, error) {
	patterns, err := repositoryTestPackagePatterns(root)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	fset := token.NewFileSet()
	loaded, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax, Dir: root, Fset: fset, Tests: true}, patterns...)
	if err != nil {
		return nil, err
	}
	if count := packages.PrintErrors(loaded); count != 0 {
		return nil, fmt.Errorf("type-check real catalog scanner input: %d errors", count)
	}
	program := buildRealCatalogSSA(fset, loaded)
	analyzer := &realCatalogSSAAnalyzer{root: root, fset: fset, inventory: inventory, fixture: fixture, storesByRoot: map[ssaRootKey][]*ssa.Store{}}
	functions := ssautil.AllFunctions(program)
	// Index stores once because every test-root backward slice consults them.
	for function := range functions {
		if !analyzer.ownedFunction(function) {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if store, ok := instruction.(*ssa.Store); ok {
					key := ssaAddressRootKey(store.Addr)
					analyzer.storesByRoot[key] = append(analyzer.storesByRoot[key], store)
				}
			}
		}
	}
	var findings []realCatalogFinding
	for function := range functions {
		if !analyzer.isTestRoot(function) {
			continue
		}
		context := analyzer.contextFor(function)
		details := analyzer.findings(context)
		position := fset.Position(function.Pos())
		relative, err := filepath.Rel(root, position.Filename)
		if err != nil {
			return nil, err
		}
		for _, detail := range uniqueStrings(details) {
			findings = append(findings, realCatalogFinding{test: filepath.ToSlash(relative) + ":" + function.Name(), detail: detail})
		}
	}
	return findings, nil
}

func buildRealCatalogSSA(fset *token.FileSet, loaded []*packages.Package) *ssa.Program {
	program := ssa.NewProgram(fset, ssa.InstantiateGenerics|ssa.NaiveForm)
	selected := map[*types.Package]*packages.Package{}
	for _, pkg := range loaded {
		if pkg.Types != nil && len(pkg.Syntax) > 0 {
			selected[pkg.Types] = pkg
		}
	}
	created := map[*types.Package]*ssa.Package{}
	// Dependency packages provide type identity only; only selected repository
	// packages receive bodies, keeping the guard practical under -race.
	var create func(*packages.Package) *ssa.Package
	create = func(pkg *packages.Package) *ssa.Package {
		if pkg == nil || pkg.Types == nil {
			return nil
		}
		if existing := created[pkg.Types]; existing != nil {
			return existing
		}
		if initial := selected[pkg.Types]; initial != nil {
			pkg = initial
		}
		for _, imported := range pkg.Imports {
			create(imported)
		}
		var files []*ast.File
		var info *types.Info
		if selected[pkg.Types] != nil {
			files = pkg.Syntax
			info = pkg.TypesInfo
		}
		created[pkg.Types] = program.CreatePackage(pkg.Types, files, info, true)
		return created[pkg.Types]
	}
	for _, pkg := range loaded {
		create(pkg)
	}
	for packageType := range selected {
		created[packageType].Build()
	}
	return program
}

func repositoryTestPackagePatterns(root string) ([]string, error) {
	directories := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "real_catalog_test_guard_test.go" {
			return nil
		}
		directories[filepath.Dir(path)] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	patterns := make([]string, 0, len(directories))
	for directory := range directories {
		relative, err := filepath.Rel(root, directory)
		if err != nil {
			return nil, err
		}
		if relative == "." {
			patterns = append(patterns, ".")
		} else {
			patterns = append(patterns, "./"+filepath.ToSlash(relative))
		}
	}
	sort.Strings(patterns)
	return patterns, nil
}

func (analyzer *realCatalogSSAAnalyzer) ownedFunction(function *ssa.Function) bool {
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	filename := analyzer.fset.Position(function.Pos()).Filename
	if filename == "" {
		return function.Pkg != nil && function.Name() == "init" && strings.HasPrefix(function.Pkg.Pkg.Path(), "github.com/yersonargotev/packy")
	}
	relative, err := filepath.Rel(analyzer.root, filename)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (analyzer *realCatalogSSAAnalyzer) isTestRoot(function *ssa.Function) bool {
	if !analyzer.ownedFunction(function) || function.Object() == nil || !strings.HasPrefix(function.Name(), "Test") || function.Signature.Recv() != nil {
		return false
	}
	filename := analyzer.fset.Position(function.Pos()).Filename
	return strings.HasSuffix(filename, "_test.go") && filepath.Base(filename) != "real_catalog_test_guard_test.go"
}

func (analyzer *realCatalogSSAAnalyzer) contextFor(root *ssa.Function) *realCatalogSSAContext {
	context := &realCatalogSSAContext{
		analyzer: analyzer, root: root,
		functions: map[*ssa.Function]bool{}, calls: map[*ssa.Function][]*ssa.CallCommon{}, closures: map[*ssa.Function][]*ssa.MakeClosure{},
		activeCalls: map[*ssa.Function]*ssa.CallCommon{},
	}
	queue := []*ssa.Function{root}
	for len(queue) > 0 {
		function := queue[0]
		queue = queue[1:]
		if context.functions[function] || !analyzer.ownedFunction(function) {
			continue
		}
		filename := analyzer.fset.Position(function.Pos()).Filename
		if function != root && !strings.HasSuffix(filename, "_test.go") {
			continue
		}
		context.functions[function] = true
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if store, ok := instruction.(*ssa.Store); ok {
					queue = append(queue, possibleSSAFunctions(store.Val, map[ssa.Value]bool{})...)
				}
				if closure, ok := instruction.(*ssa.MakeClosure); ok {
					if target, ok := closure.Fn.(*ssa.Function); ok {
						context.closures[target] = append(context.closures[target], closure)
						queue = append(queue, target)
					}
				}
				callInstruction, ok := instruction.(ssa.CallInstruction)
				if !ok {
					continue
				}
				call := callInstruction.Common()
				for _, callee := range possibleSSAFunctions(call.Value, map[ssa.Value]bool{}) {
					context.calls[callee] = append(context.calls[callee], call)
					queue = append(queue, callee)
				}
				for _, argument := range call.Args {
					queue = append(queue, possibleSSAFunctions(argument, map[ssa.Value]bool{})...)
				}
			}
		}
	}
	return context
}

func possibleSSAFunctions(value ssa.Value, seen map[ssa.Value]bool) []*ssa.Function {
	if value == nil || seen[value] {
		return nil
	}
	seen[value] = true
	defer delete(seen, value)
	switch value := value.(type) {
	case *ssa.Function:
		return []*ssa.Function{value}
	case *ssa.MakeClosure:
		if function, ok := value.Fn.(*ssa.Function); ok {
			return []*ssa.Function{function}
		}
	case *ssa.Phi:
		var result []*ssa.Function
		for _, edge := range value.Edges {
			result = append(result, possibleSSAFunctions(edge, seen)...)
		}
		return result
	case *ssa.ChangeType:
		return possibleSSAFunctions(value.X, seen)
	case *ssa.MakeInterface:
		return possibleSSAFunctions(value.X, seen)
	}
	return nil
}

func (analyzer *realCatalogSSAAnalyzer) findings(context *realCatalogSSAContext) []string {
	return uniqueStrings(analyzer.functionFindings(context, context.root, map[*ssa.Function]bool{}))
}

func (analyzer *realCatalogSSAAnalyzer) functionFindings(context *realCatalogSSAContext, function *ssa.Function, path map[*ssa.Function]bool) []string {
	if function == nil || !context.functions[function] || path[function] {
		return nil
	}
	path[function] = true
	defer delete(path, function)

	var details []string
	aliasFields := map[ssa.Value]map[string]ssa.Value{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if callInstruction, ok := instruction.(ssa.CallInstruction); ok {
				call := callInstruction.Common()
				details = append(details, analyzer.callFindings(context, function, call)...)
				for _, callee := range possibleSSAFunctions(call.Value, map[ssa.Value]bool{}) {
					details = append(details, analyzer.calledFunctionFindings(context, callee, call, path)...)
				}
				for _, argument := range call.Args {
					for _, callee := range possibleSSAFunctions(argument, map[ssa.Value]bool{}) {
						details = append(details, analyzer.functionFindings(context, callee, path)...)
					}
				}
			}
			store, ok := instruction.(*ssa.Store)
			if !ok {
				continue
			}
			for _, callee := range possibleSSAFunctions(store.Val, map[ssa.Value]bool{}) {
				details = append(details, analyzer.functionFindings(context, callee, path)...)
			}
			owner, field, ok := ssaFieldIdentity(store.Addr)
			if !ok {
				continue
			}
			if (isLifecycleType(owner) || analyzer.fixture && lifecycleTypeName(owner.Obj().Name())) && field == "PackID" {
				for id := range context.stringValues(store.Val, map[ssa.Value]bool{}) {
					if _, real := analyzer.inventory.packIDs[id]; real {
						details = append(details, fmt.Sprintf("passes real Pack %q through %s.PackID", id, owner.Obj().Name()))
					}
				}
			}
			if owner.Obj().Name() == "CommandEvidence" && field == "Args" && strings.HasSuffix(owner.Obj().Pkg().Path(), "/internal/claudesmoke") {
				details = append(details, analyzer.commandFindings(context.sequences(store.Val))...)
			}
			if owner.Obj().Name() == "SurfaceAlias" {
				base := store.Addr.(*ssa.FieldAddr).X
				if aliasFields[base] == nil {
					aliasFields[base] = map[string]ssa.Value{}
				}
				aliasFields[base][field] = store.Val
			}
		}
	}
	for _, fields := range aliasFields {
		for kind := range context.stringValues(fields["Kind"], map[ssa.Value]bool{}) {
			for id := range context.stringValues(fields["ID"], map[ssa.Value]bool{}) {
				for name := range context.stringValues(fields["Name"], map[ssa.Value]bool{}) {
					alias := realCatalogResourceAlias{kind: kind, id: id, name: name}
					if _, real := analyzer.inventory.resourceAliases[alias]; real {
						details = append(details, fmt.Sprintf("uses live resource alias %s:%s=%s", kind, id, name))
					}
				}
			}
		}
	}
	return details
}

func (analyzer *realCatalogSSAAnalyzer) calledFunctionFindings(context *realCatalogSSAContext, function *ssa.Function, call *ssa.CallCommon, path map[*ssa.Function]bool) []string {
	if !context.functions[function] {
		return nil
	}
	previous, hadPrevious := context.activeCalls[function]
	context.activeCalls[function] = call
	details := analyzer.functionFindings(context, function, path)
	if hadPrevious {
		context.activeCalls[function] = previous
	} else {
		delete(context.activeCalls, function)
	}
	return details
}

func (analyzer *realCatalogSSAAnalyzer) callFindings(context *realCatalogSSAContext, caller *ssa.Function, call *ssa.CallCommon) []string {
	callee := call.StaticCallee()
	if callee == nil {
		return nil
	}
	var direct []string
	if analyzer.isFilePathSink(callee) && len(call.Args) > 0 {
		if detail := analyzer.manifestValueFinding(context, caller, call.Args[0], map[ssa.Value]bool{}); detail != "" {
			direct = append(direct, detail)
		}
	}
	if analyzer.isCapabilitypackFunction(callee, "LoadCurrentManifest") && len(call.Args) > 0 {
		if detail := analyzer.manifestValueFinding(context, caller, call.Args[0], map[ssa.Value]bool{}); detail != "" {
			direct = append(direct, detail)
		}
	}
	if analyzer.isCapabilitypackFunction(callee, "ValidatePackContent") && len(call.Args) >= 2 && context.isLiveBundleRoot(call.Args[0]) {
		if id := analyzer.validatePackContentID(context, call.Args[1]); id != "" {
			direct = append(direct, fmt.Sprintf("validates real Pack %q through ValidatePackContent", id))
		}
	}
	if analyzer.isCapabilitypackFunction(callee, "ValidatePortableContent") && len(call.Args) > 0 && context.isLiveBundleRoot(call.Args[0]) {
		direct = append(direct, "enumerates the checked-in Pack catalog through portable content validation")
	}
	if analyzer.isNamedFunction(callee, "/internal/cli", "executeCommand") || analyzer.fixture && callee.Name() == "executeCommand" {
		if len(call.Args) == 0 {
			return nil
		}
		sequences := context.sequences(call.Args[len(call.Args)-1])
		details := analyzer.commandFindings(sequences)
		if containsCommand(sequences, "list") && len(call.Args) > 1 && context.commandUsesLiveOptions(call.Args[1], map[ssa.Value]bool{}) {
			details = append(details, "enumerates the checked-in Pack catalog through implicit discovery/list")
		}
		return append(direct, details...)
	}
	if analyzer.isQualificationDriver(callee) && len(call.Args) > 0 {
		return append(direct, analyzer.commandFindings(context.sequences(call.Args[len(call.Args)-1]))...)
	}
	if analyzer.isCatalogLookup(callee) || analyzer.isKnownLookup(callee) {
		var details []string
		for _, argument := range call.Args {
			for id := range context.stringValues(argument, map[ssa.Value]bool{}) {
				if _, real := analyzer.inventory.packIDs[id]; real {
					details = append(details, fmt.Sprintf("looks up real Pack %q through %s", id, callee.Name()))
				}
			}
		}
		return append(direct, details...)
	}
	if analyzer.isCatalogEnumerator(callee) && len(call.Args) > 0 && context.catalogUsesLiveBundle(call.Args[0], map[ssa.Value]bool{}) {
		return append(direct, "enumerates the checked-in Pack catalog through implicit discovery/list")
	}
	if (analyzer.isCapabilitypackFunction(callee, "Discover") || analyzer.isCapabilitypackFunction(callee, "DiscoverForDurableIntents")) && len(call.Args) > 0 {
		for _, argument := range call.Args {
			if context.isLiveBundleRoot(argument) {
				return append(direct, "enumerates the checked-in Pack catalog through implicit discovery/list")
			}
		}
	}
	return direct
}

func ssaFunctionPackagePath(function *ssa.Function) string {
	for function != nil {
		if function.Pkg != nil {
			return function.Pkg.Pkg.Path()
		}
		function = function.Parent()
	}
	return ""
}

func (analyzer *realCatalogSSAAnalyzer) validatePackContentID(context *realCatalogSSAContext, value ssa.Value) string {
	values := context.stringValues(value, map[ssa.Value]bool{})
	for candidate := range values {
		if _, real := analyzer.inventory.packIDs[candidate]; real {
			return candidate
		}
	}
	hasRepositoryRoot := context.valueContainsRepositoryRoot(value, map[ssa.Value]bool{})
	for candidate := range values {
		parts := strings.Split(filepath.ToSlash(filepath.Clean(candidate)), "/")
		for index := 0; index+1 < len(parts); index++ {
			if parts[index] != "packs" {
				continue
			}
			bundlePrefix := strings.Join(parts[:index], "/")
			if _, real := analyzer.inventory.packIDs[parts[index+1]]; real && (hasRepositoryRoot || isRepositoryBundlePath(bundlePrefix)) {
				return parts[index+1]
			}
		}
	}
	return ""
}

func (analyzer *realCatalogSSAAnalyzer) commandFindings(sequences [][]string) []string {
	verbs := map[string]bool{"activate": true, "deactivate": true, "install": true, "uninstall": true, "update": true, "status": true, "show": true, "check": true}
	var details []string
	for _, sequence := range sequences {
		for index := 0; index+1 < len(sequence); index++ {
			if !verbs[sequence[index]] {
				continue
			}
			if _, real := analyzer.inventory.packIDs[sequence[index+1]]; real {
				details = append(details, fmt.Sprintf("passes real Pack %q to CLI lifecycle verb %q", sequence[index+1], sequence[index]))
			}
		}
	}
	return uniqueStrings(details)
}

func containsCommand(sequences [][]string, command string) bool {
	for _, sequence := range sequences {
		for index, value := range sequence {
			if value == command && !(index > 0 && sequence[index-1] == "pack") && !(index+1 < len(sequence) && sequence[index+1] == "--help") {
				return true
			}
		}
	}
	return false
}

func (analyzer *realCatalogSSAAnalyzer) isNamedFunction(function *ssa.Function, packageSuffix, name string) bool {
	if function == nil || function.Name() != name || function.Pkg == nil {
		return false
	}
	return strings.HasSuffix(function.Pkg.Pkg.Path(), packageSuffix)
}

func (analyzer *realCatalogSSAAnalyzer) isCapabilitypackFunction(function *ssa.Function, name string) bool {
	return analyzer.isNamedFunction(function, "/internal/capabilitypack", name) || analyzer.fixture && function != nil && function.Name() == name
}

func (analyzer *realCatalogSSAAnalyzer) isQualificationDriver(function *ssa.Function) bool {
	if function == nil || function.Pkg == nil || !strings.HasSuffix(function.Pkg.Pkg.Path(), "/internal/claudesmoke") {
		return false
	}
	return function.Name() == "AllowedCommand" || function.Name() == "runAllowed" || function.Name() == "runInteractiveRestricted"
}

func (analyzer *realCatalogSSAAnalyzer) isCatalogLookup(function *ssa.Function) bool {
	if function == nil || (function.Name() != "Show" && function.Name() != "ShowDetail") || function.Signature.Recv() == nil {
		return false
	}
	named := namedType(function.Signature.Recv().Type())
	if named == nil || named.Obj().Name() != "Catalog" {
		return false
	}
	return analyzer.fixture || strings.HasSuffix(named.Obj().Pkg().Path(), "/internal/capabilitypack")
}

func (analyzer *realCatalogSSAAnalyzer) isCatalogEnumerator(function *ssa.Function) bool {
	if function == nil || (function.Name() != "ListCurrent" && function.Name() != "ListDetails") || function.Signature.Recv() == nil {
		return false
	}
	named := namedType(function.Signature.Recv().Type())
	if named == nil || named.Obj().Name() != "Catalog" {
		return false
	}
	return analyzer.fixture || strings.HasSuffix(named.Obj().Pkg().Path(), "/internal/capabilitypack")
}

func (analyzer *realCatalogSSAAnalyzer) isFilePathSink(function *ssa.Function) bool {
	if function == nil || function.Pkg == nil || function.Pkg.Pkg.Path() != "os" {
		return false
	}
	switch function.Name() {
	case "ReadFile", "WriteFile", "Open", "OpenFile", "Readlink", "Stat", "Lstat":
		return true
	default:
		return false
	}
}

func (analyzer *realCatalogSSAAnalyzer) isKnownLookup(function *ssa.Function) bool {
	if function == nil || function.Pkg == nil || !strings.HasSuffix(function.Pkg.Pkg.Path(), "/internal/cli") {
		return false
	}
	return function.Name() == "findTUIPack" || function.Name() == "checkedInPackVersion"
}

func isLifecycleType(named *types.Named) bool {
	if named == nil || named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), "github.com/yersonargotev/packy/internal/") {
		return false
	}
	return lifecycleTypeName(named.Obj().Name())
}

func lifecycleTypeName(name string) bool {
	switch name {
	case "ActivationRequest", "UpdateRequest", "DeactivationRequest", "ReconcileRequest", "StatusRequest", "ControlledCheckRequest", "ProjectInstallRequest", "ProjectUpdateRequest", "ProjectUninstallRequest", "ProjectStatusRequest", "ProjectActivationRequest", "ActivationIntent":
		return true
	default:
		return false
	}
}

func namedType(value types.Type) *types.Named {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, _ := value.(*types.Named)
	return named
}

func ssaFieldIdentity(address ssa.Value) (*types.Named, string, bool) {
	fieldAddress, ok := address.(*ssa.FieldAddr)
	if !ok {
		return nil, "", false
	}
	named := namedType(fieldAddress.X.Type())
	if named == nil {
		return nil, "", false
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok || fieldAddress.Field >= structure.NumFields() {
		return nil, "", false
	}
	return named, structure.Field(fieldAddress.Field).Name(), true
}

func (analyzer *realCatalogSSAAnalyzer) manifestValueFinding(context *realCatalogSSAContext, caller *ssa.Function, value ssa.Value, seen map[ssa.Value]bool) string {
	if value == nil || seen[value] {
		return ""
	}
	seen[value] = true
	defer delete(seen, value)

	packSyncContract := strings.HasSuffix(ssaFunctionPackagePath(caller), "/internal/packsync")
	if packSyncContract {
		for candidate := range context.stringValues(value, map[ssa.Value]bool{}) {
			if detail := analyzer.manifestPathFinding(candidate); detail != "" {
				return detail
			}
		}
	}
	switch value := value.(type) {
	case *ssa.Const, *ssa.BinOp:
		for candidate := range context.stringValues(value, map[ssa.Value]bool{}) {
			if detail := analyzer.manifestPathFinding(candidate); detail != "" && (packSyncContract || isRepositoryRelativeManifestPath(candidate)) {
				return detail
			}
		}
	case *ssa.Call:
		call := value.Common()
		callee := call.StaticCallee()
		if analyzer.isNamedFunction(callee, "path/filepath", "Join") {
			return analyzer.manifestJoinFinding(context, call.Args, packSyncContract)
		}
		var detail string
		context.withCallBinding(call, func() {
			for _, returned := range context.callReturnValues(call, 0) {
				if detail == "" {
					detail = analyzer.manifestValueFinding(context, callee, returned, seen)
				}
			}
		})
		return detail
	case *ssa.Extract:
		if call, ok := value.Tuple.(*ssa.Call); ok {
			var detail string
			context.withCallBinding(call.Common(), func() {
				for _, returned := range context.callReturnValues(call.Common(), value.Index) {
					if detail == "" {
						detail = analyzer.manifestValueFinding(context, call.Common().StaticCallee(), returned, seen)
					}
				}
			})
			return detail
		}
		return analyzer.manifestValueFinding(context, caller, value.Tuple, seen)
	case *ssa.Parameter:
		for _, argument := range context.parameterArguments(value) {
			if detail := analyzer.manifestValueFinding(context, caller, argument, seen); detail != "" {
				return detail
			}
		}
	case *ssa.FreeVar:
		for _, binding := range context.freeVarBindings(value) {
			if detail := analyzer.manifestValueFinding(context, caller, binding, seen); detail != "" {
				return detail
			}
		}
	case *ssa.UnOp:
		if address, ok := value.X.(*ssa.IndexAddr); ok {
			if detail := analyzer.manifestValueFinding(context, caller, address.X, seen); detail != "" {
				return detail
			}
			for _, store := range context.analyzer.storesFor(addressRoot(address.X)) {
				if detail := analyzer.manifestValueFinding(context, caller, store.Val, seen); detail != "" {
					return detail
				}
			}
		}
		for _, stored := range context.rootStoredValues(value.X) {
			if detail := analyzer.manifestValueFinding(context, caller, stored, seen); detail != "" {
				return detail
			}
		}
	case *ssa.Field:
		for _, stored := range context.fieldValues(value.X, value.Field) {
			if detail := analyzer.manifestValueFinding(context, caller, stored, seen); detail != "" {
				return detail
			}
		}
	case *ssa.Index:
		for _, stored := range context.indexValues(value.X, value.Index) {
			if detail := analyzer.manifestValueFinding(context, caller, stored, seen); detail != "" {
				return detail
			}
		}
	case *ssa.Next:
		return analyzer.manifestValueFinding(context, caller, value.Iter, seen)
	case *ssa.Range:
		return analyzer.manifestValueFinding(context, caller, value.X, seen)
	case *ssa.Slice:
		return analyzer.manifestValueFinding(context, caller, value.X, seen)
	case *ssa.Alloc, *ssa.MakeSlice, *ssa.Global:
		for _, stored := range context.analyzer.storesFor(value) {
			if detail := analyzer.manifestValueFinding(context, caller, stored.Val, seen); detail != "" {
				return detail
			}
		}
	case *ssa.Phi:
		for _, edge := range value.Edges {
			if detail := analyzer.manifestValueFinding(context, caller, edge, seen); detail != "" {
				return detail
			}
		}
	case *ssa.ChangeType:
		return analyzer.manifestValueFinding(context, caller, value.X, seen)
	case *ssa.Convert:
		return analyzer.manifestValueFinding(context, caller, value.X, seen)
	case *ssa.MakeInterface:
		return analyzer.manifestValueFinding(context, caller, value.X, seen)
	case *ssa.ChangeInterface:
		return analyzer.manifestValueFinding(context, caller, value.X, seen)
	}
	return ""
}

func (analyzer *realCatalogSSAAnalyzer) manifestJoinFinding(context *realCatalogSSAContext, arguments []ssa.Value, packSyncContract bool) string {
	if len(arguments) == 1 {
		for _, sequence := range context.sequences(arguments[0]) {
			if detail := analyzer.manifestPartsFinding(sequence, packSyncContract); detail != "" {
				if packSyncContract || isRepositoryRelativeManifest(sequence) || context.valueContainsRepositoryRoot(arguments[0], map[ssa.Value]bool{}) {
					return detail
				}
			}
		}
	}
	parts := make([]map[string]struct{}, len(arguments))
	for index, argument := range arguments {
		parts[index] = context.stringValues(argument, map[ssa.Value]bool{})
	}
	for index := 0; index+2 < len(parts); index++ {
		if _, ok := parts[index]["packs"]; !ok {
			continue
		}
		if _, ok := parts[index+2]["pack.json"]; !ok {
			continue
		}
		for id := range parts[index+1] {
			if _, real := analyzer.inventory.packIDs[id]; real {
				if packSyncContract {
					return fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", id)
				}
				if index > 0 {
					if _, bundle := parts[index-1]["bundle"]; bundle && (index == 1 || anyValueContainsRepositoryRoot(context, arguments[:index-1])) {
						return fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", id)
					}
				}
			}
		}
	}
	return ""
}

func (analyzer *realCatalogSSAAnalyzer) manifestPartsFinding(parts []string, packSyncContract bool) string {
	for index := 0; index+2 < len(parts); index++ {
		if parts[index] == "packs" && parts[index+2] == "pack.json" && (packSyncContract || index > 0 && parts[index-1] == "bundle") {
			if _, real := analyzer.inventory.packIDs[parts[index+1]]; real {
				return fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", parts[index+1])
			}
		}
	}
	return ""
}

func isRepositoryRelativeManifest(parts []string) bool {
	return len(parts) >= 4 && parts[0] == "bundle" && parts[1] == "packs" && parts[3] == "pack.json"
}

func anyValueContainsRepositoryRoot(context *realCatalogSSAContext, values []ssa.Value) bool {
	for _, value := range values {
		if context.valueContainsRepositoryRoot(value, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func (analyzer *realCatalogSSAAnalyzer) manifestPathFinding(value string) string {
	clean := filepath.ToSlash(filepath.Clean(value))
	for id := range analyzer.inventory.packIDs {
		if strings.Contains(clean, "bundle/packs/"+id+"/pack.json") {
			return fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", id)
		}
	}
	return ""
}

func isRepositoryRelativeManifestPath(value string) bool {
	clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(value)), "./")
	return strings.HasPrefix(clean, "bundle/packs/")
}

func isRepositoryBundlePath(value string) bool {
	clean := filepath.ToSlash(filepath.Clean(value))
	return clean == "../../bundle" || strings.HasSuffix(clean, "/bundle") && strings.Contains(clean, "../..")
}

func (context *realCatalogSSAContext) catalogUsesLiveBundle(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	defer delete(seen, value)

	for _, bundleRoot := range context.catalogBundleRootValues(value) {
		if context.isLiveBundleRoot(bundleRoot) {
			return true
		}
	}
	if extracted, ok := value.(*ssa.Extract); ok {
		if call, ok := extracted.Tuple.(*ssa.Call); ok && context.discoverUsesLiveBundle(call.Common()) {
			return true
		}
	}
	if call, ok := value.(*ssa.Call); ok {
		if context.discoverUsesLiveBundle(call.Common()) {
			return true
		}
		usesLiveBundle := false
		context.withCallBinding(call.Common(), func() {
			for _, returned := range context.callReturnValues(call.Common(), 0) {
				if context.catalogUsesLiveBundle(returned, seen) {
					usesLiveBundle = true
				}
			}
		})
		if usesLiveBundle {
			return true
		}
	}
	for _, dependency := range context.valueDependencies(value) {
		if context.catalogUsesLiveBundle(dependency, seen) {
			return true
		}
	}
	return false
}

func (context *realCatalogSSAContext) catalogBundleRootValues(value ssa.Value) []ssa.Value {
	named := namedType(value.Type())
	if named == nil || named.Obj().Name() != "Catalog" || named.Obj().Pkg() == nil {
		return nil
	}
	if !context.analyzer.fixture && !strings.HasSuffix(named.Obj().Pkg().Path(), "/internal/capabilitypack") {
		return nil
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for index := 0; index < structure.NumFields(); index++ {
		if structure.Field(index).Name() == "bundleRoot" {
			return context.fieldValues(value, index)
		}
	}
	return nil
}

func (context *realCatalogSSAContext) isLiveBundleRoot(value ssa.Value) bool {
	for candidate := range context.stringValues(value, map[ssa.Value]bool{}) {
		if isRepositoryBundlePath(candidate) {
			return true
		}
	}
	return context.valueContainsRepositoryRoot(value, map[ssa.Value]bool{}) && context.valueContainsPathPart(value, "bundle", map[ssa.Value]bool{})
}

func (context *realCatalogSSAContext) discoverUsesLiveBundle(call *ssa.CallCommon) bool {
	callee := call.StaticCallee()
	if callee == nil || callee.Name() != "Discover" {
		return false
	}
	if !context.analyzer.fixture && !context.analyzer.isNamedFunction(callee, "/internal/capabilitypack", "Discover") {
		return false
	}
	for _, argument := range call.Args {
		if context.isLiveBundleRoot(argument) {
			return true
		}
	}
	return false
}

func (context *realCatalogSSAContext) stringValues(value ssa.Value, seen map[ssa.Value]bool) map[string]struct{} {
	result := map[string]struct{}{}
	if value == nil || seen[value] {
		return result
	}
	seen[value] = true
	defer delete(seen, value)
	merge := func(values map[string]struct{}) {
		for value := range values {
			result[value] = struct{}{}
		}
	}
	switch value := value.(type) {
	case *ssa.Const:
		if value.Value != nil && value.Value.Kind() == constant.String {
			result[constant.StringVal(value.Value)] = struct{}{}
		}
	case *ssa.Phi:
		for _, edge := range value.Edges {
			merge(context.stringValues(edge, seen))
		}
	case *ssa.BinOp:
		if value.Op == token.ADD {
			for left := range context.stringValues(value.X, seen) {
				for right := range context.stringValues(value.Y, seen) {
					result[left+right] = struct{}{}
				}
			}
		}
	case *ssa.ChangeType:
		merge(context.stringValues(value.X, seen))
	case *ssa.Convert:
		merge(context.stringValues(value.X, seen))
	case *ssa.MakeInterface:
		merge(context.stringValues(value.X, seen))
	case *ssa.ChangeInterface:
		merge(context.stringValues(value.X, seen))
	case *ssa.TypeAssert:
		merge(context.stringValues(value.X, seen))
	case *ssa.Extract:
		if call, ok := value.Tuple.(*ssa.Call); ok {
			merge(context.callReturnStrings(call.Common(), value.Index, seen))
		}
	case *ssa.Call:
		callee := value.Common().StaticCallee()
		if callee != nil && context.analyzer.isNamedFunction(callee, "path/filepath", "Join") {
			if len(value.Common().Args) == 1 {
				for _, parts := range context.sequencesSeen(value.Common().Args[0], seen) {
					result[filepath.Join(parts...)] = struct{}{}
				}
				break
			}
			combinations := []string{""}
			for _, argument := range value.Common().Args {
				var next []string
				for _, prefix := range combinations {
					for part := range context.stringValues(argument, seen) {
						next = append(next, filepath.Join(prefix, part))
					}
				}
				combinations = next
			}
			for _, combination := range combinations {
				result[combination] = struct{}{}
			}
		} else {
			merge(context.callReturnStrings(value.Common(), 0, seen))
		}
	case *ssa.Parameter:
		for _, argument := range context.parameterArguments(value) {
			merge(context.stringValues(argument, seen))
		}
	case *ssa.FreeVar:
		for _, binding := range context.freeVarBindings(value) {
			merge(context.stringValues(binding, seen))
		}
	case *ssa.UnOp:
		for _, stored := range context.rootStoredValues(value.X) {
			merge(context.stringValues(stored, seen))
		}
	case *ssa.Field:
		for _, stored := range context.fieldValues(value.X, value.Field) {
			merge(context.stringValues(stored, seen))
		}
	case *ssa.Index:
		for _, stored := range context.indexValues(value.X, value.Index) {
			merge(context.stringValues(stored, seen))
		}
	case *ssa.Lookup:
		for _, stored := range context.mapValues(value.X, value.Index) {
			merge(context.stringValues(stored, seen))
		}
	}
	return result
}

func (context *realCatalogSSAContext) callReturnStrings(call *ssa.CallCommon, index int, seen map[ssa.Value]bool) map[string]struct{} {
	result := map[string]struct{}{}
	context.withCallBinding(call, func() {
		for _, returned := range context.callReturnValues(call, index) {
			for value := range context.stringValues(returned, seen) {
				result[value] = struct{}{}
			}
		}
	})
	return result
}

func (context *realCatalogSSAContext) parameterArguments(parameter *ssa.Parameter) []ssa.Value {
	parent := parameter.Parent()
	index := -1
	for candidate, current := range parent.Params {
		if current == parameter {
			index = candidate
			break
		}
	}
	if index < 0 {
		return nil
	}
	if active := context.activeCalls[parent]; active != nil {
		if index < len(active.Args) {
			return []ssa.Value{active.Args[index]}
		}
		return nil
	}
	calls := context.calls[parent]
	if len(calls) == 1 && index < len(calls[0].Args) {
		return []ssa.Value{calls[0].Args[index]}
	}
	return nil
}

func (context *realCatalogSSAContext) freeVarBindings(variable *ssa.FreeVar) []ssa.Value {
	parent := variable.Parent()
	index := -1
	for candidate, current := range parent.FreeVars {
		if current == variable {
			index = candidate
			break
		}
	}
	if index < 0 {
		return nil
	}
	var result []ssa.Value
	for _, closure := range context.closures[parent] {
		if index < len(closure.Bindings) {
			result = append(result, closure.Bindings[index])
		}
	}
	return result
}

func (context *realCatalogSSAContext) rootStoredValues(address ssa.Value) []ssa.Value {
	root := addressRoot(address)
	var result []ssa.Value
	for _, store := range context.analyzer.storesFor(root) {
		if sameSSAAddress(addressRoot(store.Addr), root) {
			result = append(result, store.Val)
		}
	}
	return result
}

func sameSSAAddress(left, right ssa.Value) bool {
	if left == right {
		return true
	}
	switch left := left.(type) {
	case *ssa.Global:
		right, ok := right.(*ssa.Global)
		return ok && left.Name() == right.Name() && left.Pkg != nil && right.Pkg != nil && left.Pkg.Pkg.Path() == right.Pkg.Pkg.Path()
	case *ssa.FieldAddr:
		right, ok := right.(*ssa.FieldAddr)
		return ok && left.Field == right.Field && sameSSAAddress(left.X, right.X)
	case *ssa.IndexAddr:
		right, ok := right.(*ssa.IndexAddr)
		return ok && sameSSAAddress(left.X, right.X) && sameSSAIndex(left.Index, right.Index)
	case *ssa.Slice:
		return sameSSAAddress(left.X, right)
	}
	if rightSlice, ok := right.(*ssa.Slice); ok {
		return sameSSAAddress(left, rightSlice.X)
	}
	return false
}

func sameSSAIndex(left, right ssa.Value) bool {
	if left == right {
		return true
	}
	leftConstant, leftOK := left.(*ssa.Const)
	rightConstant, rightOK := right.(*ssa.Const)
	return leftOK && rightOK && leftConstant.Value != nil && rightConstant.Value != nil && constant.Compare(leftConstant.Value, token.EQL, rightConstant.Value)
}

func (context *realCatalogSSAContext) fieldValues(value ssa.Value, field int) []ssa.Value {
	base := value
	if loaded, ok := value.(*ssa.UnOp); ok && loaded.Op == token.MUL {
		base = loaded.X
	}
	var result []ssa.Value
	for _, store := range context.analyzer.storesFor(base) {
		address, ok := store.Addr.(*ssa.FieldAddr)
		if ok && address.Field == field && sameSSAAddress(address.X, base) {
			result = append(result, store.Val)
		}
	}
	return result
}

func (context *realCatalogSSAContext) indexValues(value, index ssa.Value) []ssa.Value {
	base := value
	if slice, ok := value.(*ssa.Slice); ok {
		base = slice.X
	}
	var result []ssa.Value
	for _, store := range context.analyzer.storesFor(base) {
		address, ok := store.Addr.(*ssa.IndexAddr)
		if !ok || !sameSSAAddress(address.X, base) {
			continue
		}
		if _, constantIndex := index.(*ssa.Const); !constantIndex || sameSSAIndex(address.Index, index) {
			result = append(result, store.Val)
		}
	}
	return result
}

func (context *realCatalogSSAContext) mapValues(value, key ssa.Value) []ssa.Value {
	var result []ssa.Value
	for function := range context.functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				update, ok := instruction.(*ssa.MapUpdate)
				if ok && update.Map == value && intersectSSAStrings(context, update.Key, key) {
					result = append(result, update.Value)
				}
			}
		}
	}
	return result
}

func intersectSSAStrings(context *realCatalogSSAContext, left, right ssa.Value) bool {
	leftValues := context.stringValues(left, map[ssa.Value]bool{})
	for value := range context.stringValues(right, map[ssa.Value]bool{}) {
		if _, ok := leftValues[value]; ok {
			return true
		}
	}
	return false
}

func (context *realCatalogSSAContext) sequences(value ssa.Value) [][]string {
	return context.sequencesSeen(value, map[ssa.Value]bool{})
}

func (context *realCatalogSSAContext) sequencesSeen(value ssa.Value, seen map[ssa.Value]bool) [][]string {
	if value == nil || seen[value] {
		return nil
	}
	seen[value] = true
	defer delete(seen, value)
	if sequences := context.directSequences(value, seen); len(sequences) > 0 {
		return sequences
	}
	var result [][]string
	switch value := value.(type) {
	case *ssa.Phi:
		for _, edge := range value.Edges {
			result = append(result, context.sequencesSeen(edge, seen)...)
		}
	case *ssa.Parameter:
		for _, argument := range context.parameterArguments(value) {
			result = append(result, context.sequencesSeen(argument, seen)...)
		}
	case *ssa.FreeVar:
		for _, binding := range context.freeVarBindings(value) {
			result = append(result, context.sequencesSeen(binding, seen)...)
		}
	case *ssa.ChangeType:
		result = append(result, context.sequencesSeen(value.X, seen)...)
	case *ssa.Convert:
		result = append(result, context.sequencesSeen(value.X, seen)...)
	case *ssa.MakeInterface:
		result = append(result, context.sequencesSeen(value.X, seen)...)
	case *ssa.Slice:
		result = append(result, context.sequencesSeen(value.X, seen)...)
	case *ssa.UnOp:
		for _, stored := range context.rootStoredValues(value.X) {
			result = append(result, context.sequencesSeen(stored, seen)...)
		}
	case *ssa.Index:
		for _, stored := range context.indexValues(value.X, value.Index) {
			result = append(result, context.sequencesSeen(stored, seen)...)
		}
	case *ssa.Field:
		for _, stored := range context.fieldValues(value.X, value.Field) {
			result = append(result, context.sequencesSeen(stored, seen)...)
		}
	case *ssa.Extract:
		if call, ok := value.Tuple.(*ssa.Call); ok {
			result = append(result, context.callReturnSequences(call.Common(), value.Index, seen)...)
		}
	case *ssa.Call:
		if builtin, ok := value.Common().Value.(*ssa.Builtin); ok && builtin.Name() == "append" && len(value.Common().Args) > 0 {
			for _, base := range context.sequencesSeen(value.Common().Args[0], seen) {
				for addition := range context.stringValues(value.Common().Args[len(value.Common().Args)-1], seen) {
					result = append(result, append(append([]string(nil), base...), addition))
				}
			}
		} else {
			result = append(result, context.callReturnSequences(value.Common(), 0, seen)...)
		}
	}
	if len(result) == 0 {
		for _, dependency := range context.valueDependencies(value) {
			result = append(result, context.sequencesSeen(dependency, seen)...)
		}
	}
	return uniqueSequences(result)
}

func (context *realCatalogSSAContext) directSequences(value ssa.Value, seen map[ssa.Value]bool) [][]string {
	if !isStringSlice(value.Type()) {
		return nil
	}
	base := value
	if slice, ok := value.(*ssa.Slice); ok {
		base = slice.X
	}
	elements := map[int]map[string]struct{}{}
	for _, store := range context.analyzer.storesFor(base) {
		address, ok := store.Addr.(*ssa.IndexAddr)
		if !ok || !sameSSAAddress(address.X, base) {
			continue
		}
		index, ok := ssaInteger(address.Index)
		if !ok {
			continue
		}
		values := context.stringValues(store.Val, seen)
		if elements[index] == nil {
			elements[index] = map[string]struct{}{}
		}
		if len(values) == 0 {
			elements[index][""] = struct{}{}
		}
		for possible := range values {
			elements[index][possible] = struct{}{}
		}
	}
	if len(elements) == 0 {
		return nil
	}
	sequences := [][]string{{}}
	for index := 0; index < len(elements); index++ {
		values, ok := elements[index]
		if !ok || len(values) == 0 {
			return nil
		}
		var next [][]string
		for _, sequence := range sequences {
			for possible := range values {
				next = append(next, append(append([]string(nil), sequence...), possible))
			}
		}
		sequences = next
	}
	return sequences
}

func ssaInteger(value ssa.Value) (int, bool) {
	constantValue, ok := value.(*ssa.Const)
	if !ok || constantValue.Value == nil {
		return 0, false
	}
	integer, exact := constant.Int64Val(constantValue.Value)
	return int(integer), exact
}

func (context *realCatalogSSAContext) callReturnSequences(call *ssa.CallCommon, index int, seen map[ssa.Value]bool) [][]string {
	callee := call.StaticCallee()
	if callee == nil || !context.functions[callee] {
		return nil
	}
	var result [][]string
	context.withCallBinding(call, func() {
		for _, returned := range context.callReturnValues(call, index) {
			result = append(result, context.sequencesSeen(returned, seen)...)
		}
	})
	return result
}

func isStringSlice(value types.Type) bool {
	slice, ok := value.(*types.Slice)
	return ok && types.Identical(slice.Elem(), types.Typ[types.String])
}

func uniqueSequences(sequences [][]string) [][]string {
	seen := map[string]bool{}
	var result [][]string
	for _, sequence := range sequences {
		key := strings.Join(sequence, "\x00")
		if !seen[key] {
			seen[key] = true
			result = append(result, sequence)
		}
	}
	return result
}

func (context *realCatalogSSAContext) valueContainsPathPart(value ssa.Value, part string, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	defer delete(seen, value)
	for candidate := range context.stringValues(value, map[ssa.Value]bool{}) {
		for _, segment := range strings.Split(filepath.ToSlash(candidate), "/") {
			if segment == part {
				return true
			}
		}
	}
	if call, ok := value.(*ssa.Call); ok {
		callee := call.Common().StaticCallee()
		if callee != nil && context.analyzer.isNamedFunction(callee, "path/filepath", "Join") {
			for _, argument := range call.Common().Args {
				for _, sequence := range context.sequences(argument) {
					for _, candidate := range sequence {
						if candidate == part {
							return true
						}
					}
				}
			}
		}
	}
	for _, dependency := range context.valueDependencies(value) {
		if context.valueContainsPathPart(dependency, part, seen) {
			return true
		}
	}
	return false
}

func (context *realCatalogSSAContext) commandUsesLiveOptions(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	defer delete(seen, value)
	if call, ok := value.(*ssa.Call); ok {
		callee := call.Common().StaticCallee()
		if callee != nil && (context.analyzer.isNamedFunction(callee, "/internal/cli", "NewRootCommand") || context.analyzer.fixture && callee.Name() == "NewRootCommand") {
			return len(call.Common().Args) > 0 && context.valueContainsRepositoryRoot(call.Common().Args[0], map[ssa.Value]bool{})
		}
		usesLiveOptions := false
		context.withCallBinding(call.Common(), func() {
			for _, returned := range context.callReturnValues(call.Common(), 0) {
				if context.commandUsesLiveOptions(returned, seen) {
					usesLiveOptions = true
				}
			}
		})
		if usesLiveOptions {
			return true
		}
	}
	for _, dependency := range context.valueDependencies(value) {
		if context.commandUsesLiveOptions(dependency, seen) {
			return true
		}
	}
	return false
}

func (context *realCatalogSSAContext) valueContainsRepositoryRoot(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	defer delete(seen, value)
	if call, ok := value.(*ssa.Call); ok {
		callee := call.Common().StaticCallee()
		if context.functionBuildsRepositoryRoot(callee, map[*ssa.Function]bool{}) {
			return true
		}
		if callee != nil && context.analyzer.isNamedFunction(callee, "path/filepath", "Abs") {
			for _, argument := range call.Common().Args {
				for path := range context.stringValues(argument, map[ssa.Value]bool{}) {
					if isRepositoryRelativePath(path) {
						return true
					}
				}
			}
		}
		containsRoot := false
		context.withCallBinding(call.Common(), func() {
			for _, returned := range context.callReturnValues(call.Common(), 0) {
				if context.valueContainsRepositoryRoot(returned, seen) {
					containsRoot = true
				}
			}
		})
		if containsRoot {
			return true
		}
	}
	if closure, ok := value.(*ssa.MakeClosure); ok {
		if function, ok := closure.Fn.(*ssa.Function); ok {
			for _, returned := range functionReturnValues(function, 0) {
				if context.valueContainsRepositoryRoot(returned, seen) {
					return true
				}
			}
		}
	}
	for _, dependency := range context.valueDependencies(value) {
		if context.valueContainsRepositoryRoot(dependency, seen) {
			return true
		}
	}
	return false
}

func (context *realCatalogSSAContext) functionBuildsRepositoryRoot(function *ssa.Function, seen map[*ssa.Function]bool) bool {
	if function == nil || seen[function] || !context.functions[function] {
		return false
	}
	if function.Name() == "repositoryRoot" {
		return true
	}
	seen[function] = true
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			callInstruction, ok := instruction.(ssa.CallInstruction)
			if !ok {
				continue
			}
			call := callInstruction.Common()
			callee := call.StaticCallee()
			if callee != nil && context.analyzer.isNamedFunction(callee, "path/filepath", "Abs") {
				for _, argument := range call.Args {
					for path := range context.stringValues(argument, map[ssa.Value]bool{}) {
						if isRepositoryRelativePath(path) {
							return true
						}
					}
				}
			}
			if context.functionBuildsRepositoryRoot(callee, seen) {
				return true
			}
		}
	}
	return false
}

func isRepositoryRelativePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean == "../.." || strings.HasPrefix(clean, "../../")
}

func (context *realCatalogSSAContext) valueDependencies(value ssa.Value) []ssa.Value {
	switch value := value.(type) {
	case *ssa.Parameter:
		return context.parameterArguments(value)
	case *ssa.FreeVar:
		return context.freeVarBindings(value)
	case *ssa.UnOp:
		return append([]ssa.Value{value.X}, context.rootStoredValues(value.X)...)
	case *ssa.Field:
		return context.fieldValues(value.X, value.Field)
	case *ssa.Index:
		return context.indexValues(value.X, value.Index)
	case *ssa.Extract:
		if call, ok := value.Tuple.(*ssa.Call); ok {
			return append([]ssa.Value{call}, call.Common().Args...)
		}
	case *ssa.Call:
		return append([]ssa.Value(nil), value.Common().Args...)
	case *ssa.Phi:
		return value.Edges
	case *ssa.ChangeType:
		return []ssa.Value{value.X}
	case *ssa.Convert:
		return []ssa.Value{value.X}
	case *ssa.MakeInterface:
		return []ssa.Value{value.X}
	case *ssa.ChangeInterface:
		return []ssa.Value{value.X}
	case *ssa.MakeClosure:
		return value.Bindings
	case *ssa.FieldAddr:
		return []ssa.Value{value.X}
	case *ssa.IndexAddr:
		return []ssa.Value{value.X, value.Index}
	case *ssa.Slice:
		return []ssa.Value{value.X}
	case *ssa.Alloc, *ssa.MakeMap, *ssa.MakeSlice, *ssa.Global:
		var result []ssa.Value
		for _, store := range context.analyzer.storesFor(value) {
			if sameSSAAddress(addressRoot(store.Addr), value) {
				result = append(result, store.Val)
			}
		}
		return result
	}
	return nil
}

func addressRoot(value ssa.Value) ssa.Value {
	switch value := value.(type) {
	case *ssa.FieldAddr:
		return addressRoot(value.X)
	case *ssa.IndexAddr:
		return addressRoot(value.X)
	case *ssa.Slice:
		return addressRoot(value.X)
	default:
		return value
	}
}

func ssaAddressRootKey(value ssa.Value) ssaRootKey {
	root := addressRoot(value)
	if global, ok := root.(*ssa.Global); ok && global.Pkg != nil {
		return ssaRootKey{global: global.Pkg.Pkg.Path() + "." + global.Name()}
	}
	return ssaRootKey{value: root}
}

func (analyzer *realCatalogSSAAnalyzer) storesFor(address ssa.Value) []*ssa.Store {
	return analyzer.storesByRoot[ssaAddressRootKey(address)]
}

func (context *realCatalogSSAContext) callReturnValues(call *ssa.CallCommon, index int) []ssa.Value {
	callee := call.StaticCallee()
	if callee == nil || !context.functions[callee] {
		return nil
	}
	return functionReturnValues(callee, index)
}

func (context *realCatalogSSAContext) withCallBinding(call *ssa.CallCommon, analyze func()) {
	callee := call.StaticCallee()
	if callee == nil || !context.functions[callee] {
		analyze()
		return
	}
	previous, hadPrevious := context.activeCalls[callee]
	context.activeCalls[callee] = call
	analyze()
	if hadPrevious {
		context.activeCalls[callee] = previous
	} else {
		delete(context.activeCalls, callee)
	}
}

func functionReturnValues(function *ssa.Function, index int) []ssa.Value {
	var result []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if ok && index < len(returned.Results) {
				result = append(result, returned.Results[index])
			}
		}
	}
	return result
}

func validateRealCatalogExceptions(t *testing.T, findings []realCatalogFinding) {
	t.Helper()
	allowedCategories := map[realCatalogExceptionCategory]struct{}{
		realCatalogPublicContract: {}, realCatalogGeneratedEvidence: {}, realCatalogIntegrationSmoke: {},
	}
	foundTests := map[string]struct{}{}
	for _, finding := range findings {
		foundTests[finding.test] = struct{}{}
	}
	integrationSmokes := 0
	for testName, exception := range realCatalogTestExceptions {
		if _, ok := allowedCategories[exception.category]; !ok {
			t.Errorf("real catalog exception %s has invalid category %q", testName, exception.category)
		}
		if strings.TrimSpace(exception.justification) == "" {
			t.Errorf("real catalog exception %s has an empty contract justification", testName)
		}
		if _, ok := foundTests[testName]; !ok {
			t.Errorf("real catalog exception %s is stale: the test has no detected dependency", testName)
		}
		if exception.category == realCatalogIntegrationSmoke {
			integrationSmokes++
		}
	}
	if integrationSmokes > 1 {
		t.Errorf("real catalog exceptions contain %d integration smokes; issue #725 permits exactly one", integrationSmokes)
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
