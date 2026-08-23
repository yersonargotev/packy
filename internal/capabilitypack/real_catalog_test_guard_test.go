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
	calls              []realCatalogCall
	enumeratesCatalog  bool
	repositoryRoot     bool
	configuredBundle   bool
	defaultWorkingTree bool
	liveCatalogSource  bool
}

type realCatalogCall struct {
	name         string
	receiverType string
	method       bool
}

type realCatalogFunction struct {
	path  string
	name  string
	facts realCatalogFunctionFacts
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
func catalogHelper() string { return filepath.Join("bundle", "packs", "live-pack", "pack.json") }
func TestThroughHelper(t *testing.T) { _ = catalogHelper() }
func TestDirectManifestLiteral(t *testing.T) { path := "bundle/packs/live-pack/pack.json"; _ = path }
func TestTypedLifecycle(t *testing.T) { _ = ActivationRequest{PackID: "live-pack"} }
func TestCLILifecycle(t *testing.T) { run("activate", "live-"+"pack", "--surface", "codex") }
func TestRealResourceAlias(t *testing.T) { _ = SurfaceAlias{Kind: "skill", ID: "guide", Name: "live-guide"} }
func TestUnrelatedLiteral(t *testing.T) { _ = "live-pack" }
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
	for _, want := range []string{"sample_test.go:TestThroughHelper", "sample_test.go:TestDirectManifestLiteral", "sample_test.go:TestTypedLifecycle", "sample_test.go:TestCLILifecycle", "sample_test.go:TestRealResourceAlias"} {
		if !got[want] {
			t.Errorf("missing finding for %s: %+v", want, findings)
		}
	}
	if got["sample_test.go:TestUnrelatedLiteral"] {
		t.Fatalf("unrelated real-ID literal was classified as a dependency: %+v", findings)
	}
}

func TestRealCatalogDependencyScannerFollowsPackageHelpersAndMethodsAndClassifiesLiveListOnly(t *testing.T) {
	sources := map[string][]byte{
		"sample/helpers_test.go": []byte(`package sample
import "path/filepath"
func crossFileManifest() string { return filepath.Join("bundle", "packs", "live-pack", "pack.json") }
type fixture struct{}
func (fixture) live() string { return filepath.Join("bundle", "packs", "live-pack", "pack.json") }
func liveOptions(t *testing.T) Options {
	repositoryRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	return Options{Getwd: func() (string, error) { return repositoryRoot, nil }}
}
func syntheticOptions(t *testing.T) Options {
	bundleRoot := filepath.Join(t.TempDir(), "bundle")
	return Options{Env: MapEnv{"PACKY_SKILLS_SOURCE": filepath.Join(bundleRoot, "skills")}}
}`),
		"sample/scenarios_test.go": []byte(`package sample
func TestCrossFileHelper(t *testing.T) { _ = crossFileManifest() }
func TestReceiverMethod(t *testing.T) { _ = fixture{}.live() }
func TestLiveCatalogList(t *testing.T) { executeCommand(t, NewRootCommand(liveOptions(t)), "list") }
func TestSyntheticCatalogList(t *testing.T) { executeCommand(t, NewRootCommand(syntheticOptions(t)), "list") }
`),
	}
	inventory := realCatalogInventory{packIDs: map[string]struct{}{"live-pack": {}}, resourceAliases: map[realCatalogResourceAlias]struct{}{}}
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
		"sample/scenarios_test.go:TestReceiverMethod",
		"sample/scenarios_test.go:TestLiveCatalogList",
	} {
		if !got[want] {
			t.Errorf("missing finding for %s: %+v", want, findings)
		}
	}
	if got["sample/scenarios_test.go:TestSyntheticCatalogList"] {
		t.Fatalf("synthetic temporary catalog list was classified as live: %+v", findings)
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
	packages := map[string]map[string]realCatalogFunction{}
	for path, source := range sources {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		packageKey := filepath.ToSlash(filepath.Dir(path)) + ":" + parsed.Name.Name
		functions := packages[packageKey]
		if functions == nil {
			functions = map[string]realCatalogFunction{}
			packages[packageKey] = functions
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			key := "func:" + function.Name.Name
			if receiver := declaredReceiverType(function); receiver != "" {
				key = "method:" + receiver + "." + function.Name.Name
			}
			facts := inspectRealCatalogFunction(function, inventory)
			functions[key] = realCatalogFunction{path: path, name: function.Name.Name, facts: facts}
		}
	}

	var findings []realCatalogFinding
	for _, functions := range packages {
		for key, function := range functions {
			if !strings.HasPrefix(key, "func:Test") {
				continue
			}
			facts := collectRealCatalogFunctionFacts(key, functions, map[string]bool{})
			details := append([]string(nil), facts.direct...)
			if facts.enumeratesCatalog && facts.liveCatalogSource {
				details = append(details, "enumerates the checked-in Pack catalog through implicit discovery/list")
			}
			for _, detail := range uniqueStrings(details) {
				findings = append(findings, realCatalogFinding{test: function.path + ":" + function.name, detail: detail})
			}
		}
	}
	return findings, nil
}

func inspectRealCatalogFunction(function *ast.FuncDecl, inventory realCatalogInventory) realCatalogFunctionFacts {
	facts := realCatalogFunctionFacts{}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if expression, ok := node.(ast.Expr); ok {
			if detail := realCatalogPathDependency(expression, inventory.packIDs); detail != "" {
				facts.direct = append(facts.direct, detail)
			}
		}
		switch node := node.(type) {
		case *ast.CallExpr:
			if call, ok := realCatalogCallEdge(node); ok {
				facts.calls = append(facts.calls, call)
			}
			facts.enumeratesCatalog = facts.enumeratesCatalog || realCatalogEnumerationCall(node)
			facts.repositoryRoot = facts.repositoryRoot || realCatalogRepositoryRootCall(node)
			facts.liveCatalogSource = facts.liveCatalogSource || realCatalogRepositoryBundleCall(node)
			if detail := realCatalogCLILifecycleDependency(node, inventory.packIDs); detail != "" {
				facts.direct = append(facts.direct, detail)
			}
			if detail := realCatalogLookupDependency(node, inventory.packIDs); detail != "" {
				facts.direct = append(facts.direct, detail)
			}
		case *ast.CompositeLit:
			strings := literalStrings(node)
			facts.configuredBundle = facts.configuredBundle || containsLiteral(strings, "PACKY_SKILLS_SOURCE") && containsAdjacentLiterals(strings, "bundle", "skills")
			facts.defaultWorkingTree = facts.defaultWorkingTree || keyedFieldPresent(node, "Getwd")
			if detail := realCatalogCLILifecycleDependency(node, inventory.packIDs); detail != "" {
				facts.direct = append(facts.direct, detail)
			}
			if detail := realCatalogTypedLifecycleDependency(node, inventory.packIDs); detail != "" {
				facts.direct = append(facts.direct, detail)
			}
			if detail := realCatalogAliasDependency(node, inventory.resourceAliases); detail != "" {
				facts.direct = append(facts.direct, detail)
			}
		}
		return true
	})
	facts.liveCatalogSource = facts.liveCatalogSource || facts.repositoryRoot && (facts.configuredBundle || facts.defaultWorkingTree)
	return facts
}

func collectRealCatalogFunctionFacts(key string, functions map[string]realCatalogFunction, visiting map[string]bool) realCatalogFunctionFacts {
	if visiting[key] {
		return realCatalogFunctionFacts{}
	}
	function, ok := functions[key]
	if !ok {
		return realCatalogFunctionFacts{}
	}
	visiting[key] = true
	result := function.facts
	for _, call := range function.facts.calls {
		keys := []string{"func:" + call.name}
		if call.method {
			keys = nil
			if call.receiverType != "" {
				keys = append(keys, "method:"+call.receiverType+"."+call.name)
			} else {
				for candidate := range functions {
					if strings.HasPrefix(candidate, "method:") && strings.HasSuffix(candidate, "."+call.name) {
						keys = append(keys, candidate)
					}
				}
			}
		}
		for _, calledKey := range keys {
			called := collectRealCatalogFunctionFacts(calledKey, functions, visiting)
			result.direct = append(result.direct, called.direct...)
			result.enumeratesCatalog = result.enumeratesCatalog || called.enumeratesCatalog
			result.liveCatalogSource = result.liveCatalogSource || called.liveCatalogSource
		}
	}
	delete(visiting, key)
	result.direct = uniqueStrings(result.direct)
	return result
}

func realCatalogCallEdge(call *ast.CallExpr) (realCatalogCall, bool) {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return realCatalogCall{name: function.Name}, true
	case *ast.SelectorExpr:
		return realCatalogCall{name: function.Sel.Name, receiverType: receiverExpressionType(function.X), method: true}, true
	default:
		return realCatalogCall{}, false
	}
}

func declaredReceiverType(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	return receiverExpressionType(function.Recv.List[0].Type)
}

func receiverExpressionType(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return receiverExpressionType(expression.X)
	case *ast.ParenExpr:
		return receiverExpressionType(expression.X)
	case *ast.CompositeLit:
		return expressionName(expression.Type)
	case *ast.UnaryExpr:
		return receiverExpressionType(expression.X)
	default:
		return ""
	}
}

func realCatalogEnumerationCall(call *ast.CallExpr) bool {
	name := expressionName(call.Fun)
	if name == "Discover" || name == "ListCurrent" {
		return true
	}
	if name != "executeCommand" {
		return false
	}
	values := literalStrings(call)
	for index, value := range values {
		if value != "list" {
			continue
		}
		if index > 0 && values[index-1] == "pack" || index+1 < len(values) && values[index+1] == "--help" {
			continue
		}
		return true
	}
	return false
}

func realCatalogRepositoryRootCall(call *ast.CallExpr) bool {
	return expressionName(call.Fun) == "Abs" && containsAdjacentLiterals(literalStrings(call), "..", "..")
}

func realCatalogRepositoryBundleCall(call *ast.CallExpr) bool {
	values := literalStrings(call)
	for index := 0; index+2 < len(values); index++ {
		if values[index] == ".." && values[index+1] == ".." && values[index+2] == "bundle" {
			return true
		}
	}
	return false
}

func keyedFieldPresent(literal *ast.CompositeLit, field string) bool {
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := keyValue.Key.(*ast.Ident)
		if ok && key.Name == field {
			return true
		}
	}
	return false
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

func realCatalogPathDependency(node ast.Node, packIDs map[string]struct{}) string {
	parts := literalStrings(node)
	for index := range parts {
		if index+2 >= len(parts) || parts[index] != "packs" || parts[index+2] != "pack.json" {
			continue
		}
		if _, ok := packIDs[parts[index+1]]; ok {
			return fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", parts[index+1])
		}
	}
	for _, part := range parts {
		canonical := filepath.ToSlash(filepath.Clean(part))
		for id := range packIDs {
			if strings.Contains(canonical, "bundle/packs/"+id+"/pack.json") {
				return fmt.Sprintf("opens live manifest bundle/packs/%s/pack.json", id)
			}
		}
	}
	return ""
}

func realCatalogCLILifecycleDependency(node ast.Node, packIDs map[string]struct{}) string {
	values := literalStrings(node)
	verbs := map[string]struct{}{"activate": {}, "deactivate": {}, "install": {}, "uninstall": {}, "update": {}, "status": {}, "show": {}, "check": {}}
	for index := 0; index+1 < len(values); index++ {
		if _, ok := verbs[values[index]]; !ok {
			continue
		}
		if _, ok := packIDs[values[index+1]]; ok {
			return fmt.Sprintf("passes real Pack %q to CLI lifecycle verb %q", values[index+1], values[index])
		}
	}
	return ""
}

func realCatalogLookupDependency(call *ast.CallExpr, packIDs map[string]struct{}) string {
	name := expressionName(call.Fun)
	lookups := map[string]struct{}{"Show": {}, "findTUIPack": {}, "checkedInPackVersion": {}}
	if _, ok := lookups[name]; !ok {
		return ""
	}
	for _, value := range literalStrings(call) {
		if _, ok := packIDs[value]; ok {
			return fmt.Sprintf("looks up real Pack %q through %s", value, name)
		}
	}
	return ""
}

func realCatalogTypedLifecycleDependency(literal *ast.CompositeLit, packIDs map[string]struct{}) string {
	typeName := expressionName(literal.Type)
	lifecycleTypes := map[string]struct{}{
		"ActivationRequest": {}, "UpdateRequest": {}, "DeactivationRequest": {}, "ReconcileRequest": {}, "StatusRequest": {}, "ControlledCheckRequest": {},
		"ProjectInstallRequest": {}, "ProjectUpdateRequest": {}, "ProjectUninstallRequest": {}, "ProjectStatusRequest": {}, "ProjectActivationRequest": {}, "ActivationIntent": {},
	}
	if _, ok := lifecycleTypes[typeName]; !ok {
		return ""
	}
	fields := keyedLiteralStrings(literal)
	id := fields["PackID"]
	if _, ok := packIDs[id]; !ok {
		return ""
	}
	return fmt.Sprintf("passes real Pack %q through %s.PackID", id, typeName)
}

func realCatalogAliasDependency(literal *ast.CompositeLit, aliases map[realCatalogResourceAlias]struct{}) string {
	if expressionName(literal.Type) != "SurfaceAlias" {
		return ""
	}
	fields := keyedLiteralStrings(literal)
	alias := realCatalogResourceAlias{kind: fields["Kind"], id: fields["ID"], name: fields["Name"]}
	if _, ok := aliases[alias]; !ok {
		return ""
	}
	return fmt.Sprintf("uses live resource alias %s:%s=%s", alias.kind, alias.id, alias.name)
}

func literalStrings(node ast.Node) []string {
	var values []string
	ast.Inspect(node, func(node ast.Node) bool {
		if expression, ok := node.(ast.Expr); ok {
			value, constant := constantString(expression)
			if constant {
				values = append(values, value)
				return false
			}
		}
		return true
	})
	return values
}

func keyedLiteralStrings(literal *ast.CompositeLit) map[string]string {
	fields := map[string]string{}
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			continue
		}
		fields[key.Name], _ = constantString(keyValue.Value)
	}
	return fields
}

func constantString(expression ast.Expr) (string, bool) {
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
		left, leftOK := constantString(expression.X)
		right, rightOK := constantString(expression.Y)
		return left + right, leftOK && rightOK
	default:
		return "", false
	}
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
