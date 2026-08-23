package capabilitypack

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
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

type realCatalogFunctionFacts struct {
	direct             []string
	enumeratesCatalog  bool
	repositoryRoot     bool
	configuredBundle   bool
	defaultWorkingTree bool
	liveCatalogSource  bool
}

type realCatalogFunctionDeclaration struct {
	path     string
	key      string
	function *ast.FuncDecl
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
import "path/filepath"
const catalogPackID = "live-"+"pack"
var packagePackID = "live-pack"
func catalogHelper() string { return filepath.Join("bundle", "packs", "live-pack", "pack.json") }
func TestThroughHelper(t *testing.T) { _ = catalogHelper() }
func TestDirectManifestLiteral(t *testing.T) { path := "bundle/packs/live-pack/pack.json"; _ = path }
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
func TestUnrelatedLiteral(t *testing.T) { _ = "live-pack" }
func TestUnrelatedVariable(t *testing.T) { packID := "live-pack"; _ = packID }
func TestSyntheticVariable(t *testing.T) { packID := "synthetic-pack"; executeCommand(t, NewRootCommand(Options{}), "activate", packID) }
func TestUnrelatedSlice(t *testing.T) { _ = []string{"activate", "live-pack"} }
func TestUnrelatedShow(t *testing.T) { unrelated := unrelatedView{}; unrelated.Show(ctx, "live-pack") }
`
	inventory := realCatalogInventory{
		packIDs: map[string]struct{}{"live-pack": {}},
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
	} {
		if got[unwanted] {
			t.Fatalf("unrelated or synthetic variable was classified as a dependency: %s: %+v", unwanted, findings)
		}
	}
}

func TestRealCatalogDependencyScannerFollowsPackageHelpersAndMethodsAndClassifiesLiveListOnly(t *testing.T) {
	sources := map[string][]byte{
		"sample/helpers_test.go": []byte(`package sample
import "path/filepath"
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
func syntheticOptions(t *testing.T) Options {
	bundleRoot := filepath.Join(t.TempDir(), "bundle")
	return Options{Env: MapEnv{"PACKY_SKILLS_SOURCE": filepath.Join(bundleRoot, "skills")}}
}`),
		"sample/scenarios_test.go": []byte(`package sample
func TestCrossFileHelper(t *testing.T) { _ = crossFileManifest() }
func TestParameterizedManifestHelper(t *testing.T) { _ = manifestFor("live-pack") }
func TestParameterizedTypedRequestHelper(t *testing.T) { _ = requestFor("live-pack") }
func TestParameterizedAliasHelper(t *testing.T) { _ = aliasFor("skill", "guide", "live-guide") }
func TestParameterizedLookupHelper(t *testing.T) { catalog, _ := Discover(ctx, "bundle"); showPack(catalog, "live-pack") }
func TestParameterizedLifecycleHelper(t *testing.T) { executeLifecycle(t, "activate", "live-pack") }
func TestReceiverMethod(t *testing.T) { _ = fixture{}.live() }
func TestVariableReceiverMethod(t *testing.T) { f := fixture{}; _ = f.live() }
func TestParameterizedReceiverMethod(t *testing.T) { fixture{}.activate(t, "live-pack") }
func TestSyntheticFactoryReceiverMethod(t *testing.T) { _ = newSyntheticFixture().live() }
func TestLiveCatalogList(t *testing.T) { root := repositoryRoot(); options := liveOptions(root); listWithOptions(t, options) }
func TestSyntheticCatalogList(t *testing.T) { executeCommand(t, NewRootCommand(syntheticOptions(t)), "list") }
func TestReceiverShadowing(t *testing.T) {
	f := fixture{}
	_ = f.live()
	t.Run("synthetic", func(t *testing.T) { f := syntheticFixture{}; _ = f.live() })
}
func TestReceiverShadowingDoesNotReuseOuterType(t *testing.T) {
	f := fixture{}
	t.Run("synthetic", func(t *testing.T) { f := syntheticFixture{}; _ = f.live() })
	_ = f
}
`),
	}
	inventory := realCatalogInventory{
		packIDs: map[string]struct{}{"live-pack": {}},
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
		"sample/scenarios_test.go:TestReceiverShadowing",
	} {
		if !got[want] {
			t.Errorf("missing finding for %s: %+v", want, findings)
		}
	}
	if got["sample/scenarios_test.go:TestSyntheticCatalogList"] {
		t.Fatalf("synthetic temporary catalog list was classified as live: %+v", findings)
	}
	if got["sample/scenarios_test.go:TestSyntheticFactoryReceiverMethod"] {
		t.Fatalf("synthetic same-name receiver inherited the live method finding: %+v", findings)
	}
	if got["sample/scenarios_test.go:TestReceiverShadowingDoesNotReuseOuterType"] {
		t.Fatalf("inner synthetic receiver inherited the shadowed outer receiver type: %+v", findings)
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
	sources := map[string][]byte{}
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "real_catalog_test_guard_test.go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		sources[filepath.ToSlash(relative)] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return scanRealCatalogSources(sources, inventory)
}

func scanRealCatalogSource(path string, source []byte, inventory realCatalogInventory) ([]realCatalogFinding, error) {
	return scanRealCatalogSources(map[string][]byte{path: source}, inventory)
}

func scanRealCatalogSources(sources map[string][]byte, inventory realCatalogInventory) ([]realCatalogFinding, error) {
	packageDeclarations := map[string][]realCatalogFunctionDeclaration{}
	packageConstantCandidates := map[string]map[string][]ast.Expr{}
	for path, source := range sources {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		packageKey := filepath.ToSlash(filepath.Dir(path)) + ":" + parsed.Name.Name
		if packageConstantCandidates[packageKey] == nil {
			packageConstantCandidates[packageKey] = map[string][]ast.Expr{}
		}
		collectPackageStringConstantCandidates(parsed, packageConstantCandidates[packageKey])
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			key := "func:" + function.Name.Name
			if receiver := declaredReceiverType(function); receiver != "" {
				key = "method:" + receiver + "." + function.Name.Name
			}
			packageDeclarations[packageKey] = append(packageDeclarations[packageKey], realCatalogFunctionDeclaration{path: path, key: key, function: function})
		}
	}

	var findings []realCatalogFinding
	for packageKey, declarations := range packageDeclarations {
		packageConstants := resolveStableStringCandidates(packageConstantCandidates[packageKey], nil)
		factoryReturns := map[string]string{}
		for _, declaration := range declarations {
			if strings.HasPrefix(declaration.key, "func:") {
				if result := declaredFactoryReturnType(declaration.function); result != "" {
					factoryReturns[declaration.function.Name.Name] = result
				}
			}
		}
		functions := map[string]realCatalogFunctionDeclaration{}
		for _, declaration := range declarations {
			functions[declaration.key] = declaration
		}
		analyzer := realCatalogAnalyzer{
			inventory:       inventory,
			functions:       functions,
			factoryReturns:  factoryReturns,
			packageBindings: realCatalogPackageScope(packageConstants),
		}
		for key, declaration := range functions {
			if !strings.HasPrefix(key, "func:Test") {
				continue
			}
			facts, _ := analyzer.analyzeFunction(key, nil, realCatalogValue{}, map[string]bool{})
			facts.liveCatalogSource = facts.liveCatalogSource || facts.repositoryRoot && (facts.configuredBundle || facts.defaultWorkingTree)
			details := append([]string(nil), facts.direct...)
			if facts.enumeratesCatalog && facts.liveCatalogSource {
				details = append(details, "enumerates the checked-in Pack catalog through implicit discovery/list")
			}
			for _, detail := range uniqueStrings(details) {
				findings = append(findings, realCatalogFinding{test: declaration.path + ":" + declaration.function.Name.Name, detail: detail})
			}
		}
	}
	return findings, nil
}

type realCatalogValue struct {
	stringValue   string
	stringKnown   bool
	sequenceText  string
	sequenceKnown bool
	typeName      string
	catalog       bool
}

// realCatalogScope keeps stable values lexical instead of collapsing a function
// into one last-write map.
type realCatalogScope struct {
	parent *realCatalogScope
	values map[string]realCatalogValue
}

func realCatalogPackageScope(bindings map[string]string) *realCatalogScope {
	scope := &realCatalogScope{values: map[string]realCatalogValue{}}
	for name, value := range bindings {
		scope.values[name] = realCatalogValue{stringValue: value, stringKnown: true}
	}
	return scope
}

func (scope *realCatalogScope) child() *realCatalogScope {
	return &realCatalogScope{parent: scope, values: map[string]realCatalogValue{}}
}

func (scope *realCatalogScope) snapshot() *realCatalogScope {
	result := &realCatalogScope{values: map[string]realCatalogValue{}}
	var chain []*realCatalogScope
	for current := scope; current != nil; current = current.parent {
		chain = append(chain, current)
	}
	for index := len(chain) - 1; index >= 0; index-- {
		for name, value := range chain[index].values {
			result.values[name] = value
		}
	}
	return result
}

func (scope *realCatalogScope) lookup(name string) (realCatalogValue, bool) {
	for current := scope; current != nil; current = current.parent {
		value, ok := current.values[name]
		if ok {
			return value, true
		}
	}
	return realCatalogValue{}, false
}

func (scope *realCatalogScope) assign(name string, value realCatalogValue, define bool) {
	if define {
		scope.values[name] = value
		return
	}
	for current := scope; current != nil; current = current.parent {
		if _, ok := current.values[name]; ok {
			current.values[name] = value
			return
		}
	}
	scope.values[name] = value
}

type realCatalogAnalysis struct {
	facts       realCatalogFunctionFacts
	returnValue realCatalogValue
	hasReturn   bool
	// commandVectors is limited to the typed claudesmoke Evidence builder. It
	// preserves that generated-evidence contract without treating arbitrary
	// []string values as CLI invocations.
	commandVectors bool
}

// realCatalogAnalyzer symbolically evaluates stable values from each Test root
// and follows same-package helpers with their actual arguments.
type realCatalogAnalyzer struct {
	inventory       realCatalogInventory
	functions       map[string]realCatalogFunctionDeclaration
	factoryReturns  map[string]string
	packageBindings *realCatalogScope
}

func (analyzer realCatalogAnalyzer) analyzeFunction(key string, arguments []realCatalogValue, receiver realCatalogValue, visiting map[string]bool) (realCatalogFunctionFacts, realCatalogValue) {
	declaration, ok := analyzer.functions[key]
	if !ok || visiting[key] {
		return realCatalogFunctionFacts{}, realCatalogValue{}
	}
	visiting[key] = true
	defer delete(visiting, key)

	function := declaration.function
	scope := analyzer.packageBindings.snapshot()
	if function.Recv != nil {
		analyzer.bindFields(scope, function.Recv, []realCatalogValue{receiver})
	}
	analyzer.bindFields(scope, function.Type.Params, arguments)
	analysis := &realCatalogAnalysis{commandVectors: declaredFactoryReturnType(function) == "Evidence"}
	analyzer.analyzeBlock(function.Body, scope, analysis, visiting, false)
	analysis.facts.direct = uniqueStrings(analysis.facts.direct)
	if !analysis.hasReturn {
		analysis.returnValue.typeName = declaredFactoryReturnType(function)
	}
	return analysis.facts, analysis.returnValue
}

func (analyzer realCatalogAnalyzer) bindFields(scope *realCatalogScope, fields *ast.FieldList, arguments []realCatalogValue) {
	if fields == nil {
		return
	}
	argument := 0
	for _, field := range fields.List {
		for _, name := range field.Names {
			value := realCatalogValue{typeName: declaredTypeName(field.Type)}
			if argument < len(arguments) {
				value = arguments[argument]
				if value.typeName == "" {
					value.typeName = declaredTypeName(field.Type)
				}
			}
			scope.values[name.Name] = value
			argument++
		}
	}
}

func (analyzer realCatalogAnalyzer) analyzeBlock(block *ast.BlockStmt, parent *realCatalogScope, analysis *realCatalogAnalysis, visiting map[string]bool, nested bool) {
	if block == nil {
		return
	}
	scope := parent
	if nested {
		scope = parent.child()
	}
	for _, statement := range block.List {
		analyzer.analyzeStatement(statement, scope, analysis, visiting)
	}
}

func (analyzer realCatalogAnalyzer) analyzeStatement(statement ast.Stmt, scope *realCatalogScope, analysis *realCatalogAnalysis, visiting map[string]bool) {
	switch statement := statement.(type) {
	case *ast.ExprStmt:
		analyzer.evaluateExpression(statement.X, scope, analysis, visiting)
	case *ast.AssignStmt:
		values := make([]realCatalogValue, len(statement.Rhs))
		for index, expression := range statement.Rhs {
			values[index] = analyzer.evaluateExpression(expression, scope, analysis, visiting)
		}
		for index, expression := range statement.Lhs {
			name, ok := expression.(*ast.Ident)
			if !ok || name.Name == "_" {
				analyzer.evaluateExpression(expression, scope, analysis, visiting)
				continue
			}
			value := realCatalogValue{}
			if index < len(values) {
				value = values[index]
			} else if len(values) == 1 && index == 0 {
				value = values[0]
			}
			define := statement.Tok == token.DEFINE
			if define {
				_, define = scope.values[name.Name]
				define = !define
			}
			scope.assign(name.Name, value, define)
		}
	case *ast.DeclStmt:
		declaration, ok := statement.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, spec := range declaration.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			values := make([]realCatalogValue, len(valueSpec.Values))
			for index, expression := range valueSpec.Values {
				values[index] = analyzer.evaluateExpression(expression, scope, analysis, visiting)
			}
			for index, name := range valueSpec.Names {
				value := realCatalogValue{typeName: declaredTypeName(valueSpec.Type)}
				if index < len(values) {
					value = values[index]
					if value.typeName == "" {
						value.typeName = declaredTypeName(valueSpec.Type)
					}
				}
				scope.assign(name.Name, value, true)
			}
		}
	case *ast.ReturnStmt:
		for index, expression := range statement.Results {
			value := analyzer.evaluateExpression(expression, scope, analysis, visiting)
			if index == 0 {
				analyzer.mergeReturn(analysis, value)
			}
		}
	case *ast.BlockStmt:
		analyzer.analyzeBlock(statement, scope, analysis, visiting, true)
	case *ast.IfStmt:
		branch := scope.snapshot()
		if statement.Init != nil {
			analyzer.analyzeStatement(statement.Init, branch, analysis, visiting)
		}
		analyzer.evaluateExpression(statement.Cond, branch, analysis, visiting)
		analyzer.analyzeBlock(statement.Body, branch, analysis, visiting, true)
		if statement.Else != nil {
			analyzer.analyzeStatement(statement.Else, scope.snapshot(), analysis, visiting)
		}
	case *ast.ForStmt:
		loop := scope.snapshot()
		if statement.Init != nil {
			analyzer.analyzeStatement(statement.Init, loop, analysis, visiting)
		}
		analyzer.evaluateExpression(statement.Cond, loop, analysis, visiting)
		analyzer.analyzeBlock(statement.Body, loop, analysis, visiting, true)
		if statement.Post != nil {
			analyzer.analyzeStatement(statement.Post, loop, analysis, visiting)
		}
	case *ast.RangeStmt:
		loop := scope.snapshot()
		rangeValue := analyzer.evaluateExpression(statement.X, loop, analysis, visiting)
		for _, expression := range []ast.Expr{statement.Key, statement.Value} {
			if name, ok := expression.(*ast.Ident); ok && name.Name != "_" {
				loop.assign(name.Name, rangeValue, statement.Tok == token.DEFINE)
			}
		}
		analyzer.analyzeBlock(statement.Body, loop, analysis, visiting, true)
	case *ast.SwitchStmt:
		branch := scope.snapshot()
		if statement.Init != nil {
			analyzer.analyzeStatement(statement.Init, branch, analysis, visiting)
		}
		analyzer.evaluateExpression(statement.Tag, branch, analysis, visiting)
		analyzer.analyzeCaseClauses(statement.Body, branch, analysis, visiting)
	case *ast.TypeSwitchStmt:
		branch := scope.snapshot()
		if statement.Init != nil {
			analyzer.analyzeStatement(statement.Init, branch, analysis, visiting)
		}
		analyzer.analyzeStatement(statement.Assign, branch, analysis, visiting)
		analyzer.analyzeCaseClauses(statement.Body, branch, analysis, visiting)
	case *ast.SelectStmt:
		analyzer.analyzeCaseClauses(statement.Body, scope.snapshot(), analysis, visiting)
	case *ast.GoStmt:
		analyzer.evaluateExpression(statement.Call, scope, analysis, visiting)
	case *ast.DeferStmt:
		analyzer.evaluateExpression(statement.Call, scope, analysis, visiting)
	case *ast.SendStmt:
		analyzer.evaluateExpression(statement.Chan, scope, analysis, visiting)
		analyzer.evaluateExpression(statement.Value, scope, analysis, visiting)
	case *ast.LabeledStmt:
		analyzer.analyzeStatement(statement.Stmt, scope, analysis, visiting)
	}
}

func (analyzer realCatalogAnalyzer) analyzeCaseClauses(body *ast.BlockStmt, scope *realCatalogScope, analysis *realCatalogAnalysis, visiting map[string]bool) {
	if body == nil {
		return
	}
	for _, statement := range body.List {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		branch := scope.child()
		for _, expression := range clause.List {
			analyzer.evaluateExpression(expression, branch, analysis, visiting)
		}
		for _, bodyStatement := range clause.Body {
			analyzer.analyzeStatement(bodyStatement, branch, analysis, visiting)
		}
	}
}

func (analyzer realCatalogAnalyzer) mergeReturn(analysis *realCatalogAnalysis, value realCatalogValue) {
	if !analysis.hasReturn {
		analysis.returnValue = value
		analysis.hasReturn = true
		return
	}
	if analysis.returnValue != value {
		analysis.returnValue = realCatalogValue{}
	}
}

func (analyzer realCatalogAnalyzer) evaluateExpression(expression ast.Expr, scope *realCatalogScope, analysis *realCatalogAnalysis, visiting map[string]bool) realCatalogValue {
	if expression == nil {
		return realCatalogValue{}
	}
	switch expression := expression.(type) {
	case *ast.BasicLit:
		if expression.Kind != token.STRING {
			return realCatalogValue{}
		}
		value, err := strconv.Unquote(expression.Value)
		if err != nil {
			return realCatalogValue{}
		}
		result := realCatalogValue{stringValue: value, stringKnown: true}
		analyzer.recordPathDependency(result, analysis)
		return result
	case *ast.Ident:
		value, _ := scope.lookup(expression.Name)
		return value
	case *ast.BinaryExpr:
		left := analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
		right := analyzer.evaluateExpression(expression.Y, scope, analysis, visiting)
		if expression.Op == token.ADD && left.stringKnown && right.stringKnown {
			result := realCatalogValue{stringValue: left.stringValue + right.stringValue, stringKnown: true}
			analyzer.recordPathDependency(result, analysis)
			return result
		}
		return realCatalogValue{}
	case *ast.ParenExpr:
		return analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
	case *ast.UnaryExpr:
		return analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
	case *ast.CallExpr:
		return analyzer.evaluateCall(expression, scope, analysis, visiting)
	case *ast.CompositeLit:
		return analyzer.evaluateComposite(expression, scope, analysis, visiting)
	case *ast.FuncLit:
		closure := scope.snapshot()
		analyzer.bindFields(closure, expression.Type.Params, nil)
		closureAnalysis := &realCatalogAnalysis{commandVectors: analysis.commandVectors}
		analyzer.analyzeBlock(expression.Body, closure, closureAnalysis, visiting, false)
		mergeRealCatalogFacts(&analysis.facts, closureAnalysis.facts)
		return realCatalogValue{typeName: "func"}
	case *ast.SelectorExpr:
		analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
		return realCatalogValue{}
	case *ast.IndexExpr:
		analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
		analyzer.evaluateExpression(expression.Index, scope, analysis, visiting)
		return realCatalogValue{}
	case *ast.IndexListExpr:
		analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
		for _, index := range expression.Indices {
			analyzer.evaluateExpression(index, scope, analysis, visiting)
		}
		return realCatalogValue{}
	case *ast.SliceExpr:
		analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
		analyzer.evaluateExpression(expression.Low, scope, analysis, visiting)
		analyzer.evaluateExpression(expression.High, scope, analysis, visiting)
		analyzer.evaluateExpression(expression.Max, scope, analysis, visiting)
		return realCatalogValue{}
	case *ast.TypeAssertExpr:
		return analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
	case *ast.StarExpr:
		return analyzer.evaluateExpression(expression.X, scope, analysis, visiting)
	case *ast.KeyValueExpr:
		analyzer.evaluateExpression(expression.Key, scope, analysis, visiting)
		return analyzer.evaluateExpression(expression.Value, scope, analysis, visiting)
	default:
		return realCatalogValue{}
	}
}

func (analyzer realCatalogAnalyzer) evaluateCall(call *ast.CallExpr, scope *realCatalogScope, analysis *realCatalogAnalysis, visiting map[string]bool) realCatalogValue {
	var receiver realCatalogValue
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
		receiver = analyzer.evaluateExpression(selector.X, scope, analysis, visiting)
	}
	arguments := make([]realCatalogValue, len(call.Args))
	for index, argument := range call.Args {
		arguments[index] = analyzer.evaluateExpression(argument, scope, analysis, visiting)
	}

	name := expressionName(call.Fun)
	if isFilepathJoin(call.Fun) {
		analyzer.recordPathParts(arguments, analysis)
		parts := make([]string, len(arguments))
		for index, argument := range arguments {
			if !argument.stringKnown {
				return realCatalogValue{}
			}
			parts[index] = argument.stringValue
		}
		result := realCatalogValue{stringValue: filepath.Join(parts...), stringKnown: true}
		analyzer.recordPathDependency(result, analysis)
		analyzer.recordLiveCatalogPath(result.stringValue, analysis)
		return result
	}
	if name == "Abs" {
		for _, argument := range arguments {
			if argument.stringKnown && filepath.ToSlash(filepath.Clean(argument.stringValue)) == "../.." {
				analysis.facts.repositoryRoot = true
			}
		}
	}
	if name == "Discover" {
		analysis.facts.enumeratesCatalog = true
		result := realCatalogValue{catalog: realCatalogDiscoverCall(call), typeName: "Catalog"}
		return result
	}
	if name == "ListCurrent" {
		analysis.facts.enumeratesCatalog = true
	}
	if name == "executeCommand" {
		analyzer.recordCommandDependencies(arguments, analysis)
	}
	if name == "AllowedCommand" || name == "runAllowed" || name == "runInteractiveRestricted" {
		analyzer.recordCommandLiteralDependencies(call, scope, arguments, analysis)
	}
	if detail := analyzer.lookupDependency(call, receiver, arguments); detail != "" {
		analysis.facts.direct = append(analysis.facts.direct, detail)
	}

	key := ""
	switch function := call.Fun.(type) {
	case *ast.Ident:
		key = "func:" + function.Name
	case *ast.SelectorExpr:
		if receiver.typeName != "" {
			key = "method:" + receiver.typeName + "." + function.Sel.Name
		}
	}
	if key != "" {
		if _, ok := analyzer.functions[key]; ok {
			facts, result := analyzer.analyzeFunction(key, arguments, receiver, visiting)
			mergeRealCatalogFacts(&analysis.facts, facts)
			if result.typeName == "" {
				result.typeName = analyzer.factoryReturns[name]
			}
			return result
		}
	}
	return realCatalogValue{typeName: analyzer.factoryReturns[name]}
}

func realCatalogDiscoverCall(call *ast.CallExpr) bool {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return function.Name == "Discover"
	case *ast.SelectorExpr:
		qualifier, ok := function.X.(*ast.Ident)
		return ok && qualifier.Name == "capabilitypack" && function.Sel.Name == "Discover"
	default:
		return false
	}
}

func (analyzer realCatalogAnalyzer) evaluateComposite(literal *ast.CompositeLit, scope *realCatalogScope, analysis *realCatalogAnalysis, visiting map[string]bool) realCatalogValue {
	fields := map[string]realCatalogValue{}
	for _, element := range literal.Elts {
		if keyValue, ok := element.(*ast.KeyValueExpr); ok {
			value := analyzer.evaluateExpression(keyValue.Value, scope, analysis, visiting)
			if key, ok := keyValue.Key.(*ast.Ident); ok {
				fields[key.Name] = value
			}
			continue
		}
		analyzer.evaluateExpression(element, scope, analysis, visiting)
	}

	typeName := expressionName(literal.Type)
	if analysis.commandVectors && realCatalogStringSequenceType(literal.Type) {
		analyzer.recordCommandStrings(literalStringsWithBindings(literal, analyzer.scopeStrings(scope)), analysis)
	}
	if typeName == "Options" {
		if _, ok := fields["Getwd"]; ok {
			analysis.facts.defaultWorkingTree = true
		}
	}
	bindings := analyzer.scopeStrings(scope)
	stringsInLiteral := literalStringsWithBindings(literal, bindings)
	if containsLiteral(stringsInLiteral, "PACKY_SKILLS_SOURCE") && (containsAdjacentLiterals(stringsInLiteral, "bundle", "skills") || containsCleanPath(stringsInLiteral, "bundle/skills")) {
		analysis.facts.configuredBundle = true
	}
	if detail := analyzer.typedLifecycleDependency(typeName, fields); detail != "" {
		analysis.facts.direct = append(analysis.facts.direct, detail)
	}
	if detail := analyzer.aliasDependency(typeName, fields); detail != "" {
		analysis.facts.direct = append(analysis.facts.direct, detail)
	}
	result := realCatalogValue{typeName: declaredTypeName(literal.Type)}
	if realCatalogStringSequenceType(literal.Type) && len(stringsInLiteral) > 0 {
		result.sequenceText = strings.Join(stringsInLiteral, "\x00")
		result.sequenceKnown = true
	}
	return result
}

func (analyzer realCatalogAnalyzer) typedLifecycleDependency(typeName string, fields map[string]realCatalogValue) string {
	lifecycleTypes := map[string]struct{}{
		"ActivationRequest": {}, "UpdateRequest": {}, "DeactivationRequest": {}, "ReconcileRequest": {}, "StatusRequest": {}, "ControlledCheckRequest": {},
		"ProjectInstallRequest": {}, "ProjectUpdateRequest": {}, "ProjectUninstallRequest": {}, "ProjectStatusRequest": {}, "ProjectActivationRequest": {}, "ActivationIntent": {},
	}
	if _, ok := lifecycleTypes[typeName]; !ok {
		return ""
	}
	id := fields["PackID"]
	if !id.stringKnown {
		return ""
	}
	if _, ok := analyzer.inventory.packIDs[id.stringValue]; !ok {
		return ""
	}
	return fmt.Sprintf("passes real Pack %q through %s.PackID", id.stringValue, typeName)
}

func (analyzer realCatalogAnalyzer) aliasDependency(typeName string, fields map[string]realCatalogValue) string {
	if typeName != "SurfaceAlias" {
		return ""
	}
	kind, id, name := fields["Kind"], fields["ID"], fields["Name"]
	if !kind.stringKnown || !id.stringKnown || !name.stringKnown {
		return ""
	}
	alias := realCatalogResourceAlias{kind: kind.stringValue, id: id.stringValue, name: name.stringValue}
	if _, ok := analyzer.inventory.resourceAliases[alias]; !ok {
		return ""
	}
	return fmt.Sprintf("uses live resource alias %s:%s=%s", alias.kind, alias.id, alias.name)
}

func (analyzer realCatalogAnalyzer) recordCommandDependencies(arguments []realCatalogValue, analysis *realCatalogAnalysis) {
	values := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.stringKnown {
			values = append(values, argument.stringValue)
		} else {
			values = append(values, "")
		}
	}
	analyzer.recordCommandStrings(values, analysis)
}

func (analyzer realCatalogAnalyzer) recordCommandLiteralDependencies(call *ast.CallExpr, scope *realCatalogScope, arguments []realCatalogValue, analysis *realCatalogAnalysis) {
	for _, argument := range arguments {
		if argument.sequenceKnown {
			analyzer.recordCommandStrings(strings.Split(argument.sequenceText, "\x00"), analysis)
		}
	}
	for _, argument := range call.Args {
		literal, ok := argument.(*ast.CompositeLit)
		if !ok || !realCatalogStringSequenceType(literal.Type) {
			continue
		}
		analyzer.recordCommandStrings(literalStringsWithBindings(literal, analyzer.scopeStrings(scope)), analysis)
	}
}

func (analyzer realCatalogAnalyzer) recordCommandStrings(values []string, analysis *realCatalogAnalysis) {
	verbs := map[string]struct{}{"activate": {}, "deactivate": {}, "install": {}, "uninstall": {}, "update": {}, "status": {}, "show": {}, "check": {}}
	for index := 0; index+1 < len(values); index++ {
		verb, id := values[index], values[index+1]
		if _, ok := verbs[verb]; !ok {
			continue
		}
		if _, ok := analyzer.inventory.packIDs[id]; ok {
			analysis.facts.direct = append(analysis.facts.direct, fmt.Sprintf("passes real Pack %q to CLI lifecycle verb %q", id, verb))
		}
	}
	for index, argument := range values {
		if argument != "list" {
			continue
		}
		if index > 0 && values[index-1] == "pack" {
			continue
		}
		if index+1 < len(values) && values[index+1] == "--help" {
			continue
		}
		analysis.facts.enumeratesCatalog = true
	}
}

func realCatalogStringSequenceType(expression ast.Expr) bool {
	array, ok := expression.(*ast.ArrayType)
	if !ok {
		return false
	}
	if declaredTypeName(array.Elt) == "string" {
		return true
	}
	return realCatalogStringSequenceType(array.Elt)
}

func (analyzer realCatalogAnalyzer) lookupDependency(call *ast.CallExpr, receiver realCatalogValue, arguments []realCatalogValue) string {
	name := expressionName(call.Fun)
	knownLookup := name == "checkedInPackVersion" || name == "findTUIPack"
	if name == "Show" {
		knownLookup = receiver.catalog
	}
	if !knownLookup {
		return ""
	}
	for _, argument := range arguments {
		if !argument.stringKnown {
			continue
		}
		if _, ok := analyzer.inventory.packIDs[argument.stringValue]; ok {
			return fmt.Sprintf("looks up real Pack %q through %s", argument.stringValue, name)
		}
	}
	return ""
}

func (analyzer realCatalogAnalyzer) recordPathDependency(value realCatalogValue, analysis *realCatalogAnalysis) {
	if !value.stringKnown {
		return
	}
	canonical := filepath.ToSlash(filepath.Clean(value.stringValue))
	for id := range analyzer.inventory.packIDs {
		if strings.Contains(canonical, "bundle/packs/"+id+"/pack.json") {
			analysis.facts.direct = append(analysis.facts.direct, fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", id))
		}
	}
}

func (analyzer realCatalogAnalyzer) recordPathParts(parts []realCatalogValue, analysis *realCatalogAnalysis) {
	for index := 0; index+2 < len(parts); index++ {
		if !parts[index].stringKnown || !parts[index+1].stringKnown || !parts[index+2].stringKnown {
			continue
		}
		if parts[index].stringValue != "packs" || parts[index+2].stringValue != "pack.json" {
			continue
		}
		id := parts[index+1].stringValue
		if _, ok := analyzer.inventory.packIDs[id]; ok {
			analysis.facts.direct = append(analysis.facts.direct, fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", id))
		}
	}
}

func (analyzer realCatalogAnalyzer) recordLiveCatalogPath(value string, analysis *realCatalogAnalysis) {
	canonical := filepath.ToSlash(filepath.Clean(value))
	if canonical == "../../bundle" || strings.HasSuffix(canonical, "/../../bundle") {
		analysis.facts.liveCatalogSource = true
	}
}

func (analyzer realCatalogAnalyzer) scopeStrings(scope *realCatalogScope) map[string]string {
	bindings := map[string]string{}
	var chain []*realCatalogScope
	for current := scope; current != nil; current = current.parent {
		chain = append(chain, current)
	}
	for index := len(chain) - 1; index >= 0; index-- {
		for name, value := range chain[index].values {
			if value.stringKnown {
				bindings[name] = value.stringValue
			} else {
				delete(bindings, name)
			}
		}
	}
	return bindings
}

func mergeRealCatalogFacts(target *realCatalogFunctionFacts, source realCatalogFunctionFacts) {
	target.direct = append(target.direct, source.direct...)
	target.enumeratesCatalog = target.enumeratesCatalog || source.enumeratesCatalog
	target.repositoryRoot = target.repositoryRoot || source.repositoryRoot
	target.configuredBundle = target.configuredBundle || source.configuredBundle
	target.defaultWorkingTree = target.defaultWorkingTree || source.defaultWorkingTree
	target.liveCatalogSource = target.liveCatalogSource || source.liveCatalogSource
}

func declaredReceiverType(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	return declaredTypeName(function.Recv.List[0].Type)
}

func declaredTypeName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return declaredTypeName(expression.X)
	case *ast.ParenExpr:
		return declaredTypeName(expression.X)
	case *ast.SelectorExpr:
		return expression.Sel.Name
	case *ast.IndexExpr:
		return declaredTypeName(expression.X)
	case *ast.IndexListExpr:
		return declaredTypeName(expression.X)
	default:
		return ""
	}
}

func declaredFactoryReturnType(function *ast.FuncDecl) string {
	if function.Type.Results == nil || len(function.Type.Results.List) != 1 {
		return ""
	}
	return declaredTypeName(function.Type.Results.List[0].Type)
}

func collectPackageStringConstantCandidates(file *ast.File, candidates map[string][]ast.Expr) {
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST && group.Tok != token.VAR {
			continue
		}
		for _, spec := range group.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range value.Names {
				if index < len(value.Values) {
					candidates[name.Name] = append(candidates[name.Name], value.Values[index])
				}
			}
		}
	}
}

func resolveStableStringCandidates(candidates map[string][]ast.Expr, seed map[string]string) map[string]string {
	bindings := map[string]string{}
	for name, value := range seed {
		bindings[name] = value
	}
	for name := range candidates {
		delete(bindings, name)
	}
	for progress := true; progress; {
		progress = false
		for name, expressions := range candidates {
			if len(expressions) != 1 {
				continue
			}
			value, ok := constantStringWithBindings(expressions[0], bindings)
			current, exists := bindings[name]
			if !ok || exists && current == value {
				continue
			}
			bindings[name] = value
			progress = true
		}
	}
	return bindings
}

func containsLiteral(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAdjacentLiterals(values []string, first, second string) bool {
	for index := 0; index+1 < len(values); index++ {
		if values[index] == first && values[index+1] == second {
			return true
		}
	}
	return false
}

func containsCleanPath(values []string, want string) bool {
	for _, value := range values {
		if filepath.ToSlash(filepath.Clean(value)) == want {
			return true
		}
	}
	return false
}

func literalStringsWithBindings(node ast.Node, bindings map[string]string) []string {
	var values []string
	ast.Inspect(node, func(node ast.Node) bool {
		if expression, ok := node.(ast.Expr); ok {
			value, constant := constantStringWithBindings(expression, bindings)
			if constant {
				values = append(values, value)
				return false
			}
		}
		return true
	})
	return values
}

func constantStringWithBindings(expression ast.Expr, bindings map[string]string) (string, bool) {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		if expression.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(expression.Value)
		return value, err == nil
	case *ast.BinaryExpr:
		if expression.Op != token.ADD {
			return "", false
		}
		left, leftOK := constantStringWithBindings(expression.X, bindings)
		right, rightOK := constantStringWithBindings(expression.Y, bindings)
		return left + right, leftOK && rightOK
	case *ast.Ident:
		value, ok := bindings[expression.Name]
		return value, ok
	case *ast.ParenExpr:
		return constantStringWithBindings(expression.X, bindings)
	case *ast.CallExpr:
		if !isFilepathJoin(expression.Fun) || len(expression.Args) == 0 {
			return "", false
		}
		parts := make([]string, len(expression.Args))
		for index, argument := range expression.Args {
			part, ok := constantStringWithBindings(argument, bindings)
			if !ok {
				return "", false
			}
			parts[index] = part
		}
		return filepath.Join(parts...), true
	default:
		return "", false
	}
}

func isFilepathJoin(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Join" {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Name == "filepath"
}

func expressionName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return expression.Sel.Name
	default:
		return ""
	}
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
