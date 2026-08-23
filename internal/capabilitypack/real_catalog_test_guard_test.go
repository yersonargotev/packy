package capabilitypack

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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

// Exceptions are scoped to complete tests so every intentional dependency on
// the checked-in catalog names the public contract it protects.
var realCatalogTestExceptions = map[string]realCatalogException{
	"internal/capabilitypack/argote_pack_test.go:TestCheckedInArgotePackHasCollisionFreeNativeRoots":                         {realCatalogPublicContract, "Protects Argote's checked-in version, resource roots, native surface bindings, and published guidance contract."},
	"internal/capabilitypack/pstack_pack_test.go:TestCheckedInPstackPackPreservesCompatibilityMatrix":                        {realCatalogPublicContract, "Protects pstack's checked-in 81-case resource-selection and three-surface compatibility matrix."},
	"internal/capabilitypack/content_validation_test.go:TestCheckedInCurrentManifestsOmitRetiredContractTerms":               {realCatalogGeneratedEvidence, "Protects every checked-in manifest against retired schema and capability terms."},
	"internal/ci/issue703_engram_managed_pack_test.go:TestIssue703EngramManagedPackOwnsCurrentRuntimeContractAndClosure":     {realCatalogPublicContract, "Protects Engram's Managed Pack runtime contract and exact admitted closure."},
	"internal/claudesmoke/runner_test.go:TestAllowedCommandRejectsInteractiveClaudeAndUnknownPacky":                          {realCatalogGeneratedEvidence, "Protects the Addy qualification runner's exact release-command allowlist and rejection boundary."},
	"internal/claudesmoke/runner_test.go:TestEvidenceSchemaV3ProvesInitializationThenExplicitActivation":                     {realCatalogGeneratedEvidence, "Protects Addy publication evidence ordering from initialization through explicit activation."},
	"internal/claudesmoke/runner_test.go:TestRunAllowedUsesCanonicalConfiguredPackyIdentity":                                 {realCatalogGeneratedEvidence, "Protects validation of the configured Packy executable identity in Addy qualification evidence."},
	"internal/claudesmoke/runner_test.go:TestRunInteractiveRestrictedProvidesTTYForExplicitActivation":                       {realCatalogGeneratedEvidence, "Protects the explicit interactive Addy activation required by the publication qualification flow."},
	"internal/claudesmoke/runner_test.go:TestValidateEvidenceRejectsTampering":                                               {realCatalogGeneratedEvidence, "Protects content-bound Addy qualification evidence against command and activation tampering."},
	"internal/claudesmoke/runner_test.go:TestValidationFailureStillWritesDiagnosticEvidence":                                 {realCatalogGeneratedEvidence, "Protects diagnostic Addy qualification evidence when release validation fails."},
	"internal/claudesmoke/addy_qualification_test.go:TestAddyQualificationCanonicalOutput":                                   {realCatalogGeneratedEvidence, "Protects the canonical Addy qualification artifact emitted for release evidence."},
	"internal/claudesmoke/addy_qualification_test.go:TestAddyQualificationProductionBoundary":                                {realCatalogGeneratedEvidence, "Protects the production boundary that binds real Addy qualification observations."},
	"internal/claudesmoke/addy_qualification_test.go:TestAddyQualificationRejectsOneFactSafetyFailures":                      {realCatalogGeneratedEvidence, "Protects Addy qualification rejection when any required release-safety fact fails."},
	"internal/claudesmoke/addy_qualification_test.go:TestBindAddyQualificationUsesInSandboxObservations":                     {realCatalogGeneratedEvidence, "Protects binding of sandboxed Addy lifecycle observations into qualification evidence."},
	"internal/claudesmoke/addy_qualification_test.go:TestProductionAddyQualificationRejectsStaleCollection":                  {realCatalogGeneratedEvidence, "Protects production Addy qualification against stale collected release evidence."},
	"internal/claudesmoke/release_evidence_test.go:TestValidateReleaseAddyQualificationMatrixRequiresOneSyntheticRun":        {realCatalogGeneratedEvidence, "Protects the release evidence matrix combining real Addy qualification with its required synthetic run."},
	"internal/claudesmoke/release_evidence_test.go:TestValidateReleaseEvidenceMatrixUsesCanonicalEvidence":                   {realCatalogGeneratedEvidence, "Protects canonical Addy qualification evidence in the complete release validation matrix."},
	"internal/cli/claude_pack_tracer_test.go:TestClaudeMattyTracerActivatesStatusesAndDeactivatesInSandbox":                  {realCatalogIntegrationSmoke, "Runs the one end-to-end real-catalog smoke across Matty's Claude activation, status, update, and deactivation lifecycle."},
	"internal/cli/issue579_pack_show_inventory_test.go:TestPackShowHumanRendersDeterministicDescriptiveInventory":            {realCatalogPublicContract, "Protects Engram's checked-in descriptive resource inventory and deterministic human rendering."},
	"internal/cli/issue579_pack_show_inventory_test.go:TestPackShowJSONV5IncludesDescriptiveInventory":                       {realCatalogPublicContract, "Protects Engram's checked-in resource inventory in the public structured show contract."},
	"internal/cli/issue683_pack_list_json_test.go:TestPackListHumanOutputRemainsUnchanged":                                   {realCatalogGeneratedEvidence, "Protects the complete checked-in catalog's generated human list ordering, versions, descriptions, and surfaces."},
	"internal/cli/issue683_pack_list_json_test.go:TestPackListJSONReportsValidatedCatalogInCanonicalOrder":                   {realCatalogGeneratedEvidence, "Protects the generated JSON catalog against the validated checked-in catalog and its canonical ordering."},
	"internal/cli/pack_test.go:TestArgoteActivationPreviewIsApplicableOnEverySurface":                                        {realCatalogPublicContract, "Protects Argote's reviewed all-surface activation and collision-free native projection contract."},
	"internal/cli/pack_test.go:TestArgoteCodexActivationSurvivesReceiptReloadAndCanBeDeactivated":                            {realCatalogPublicContract, "Protects Argote's checked-in Codex projection, receipt reload, status, and deactivation contract."},
	"internal/cli/pack_test.go:TestPackListUsesOneCapturedWorkstationForSkillSource":                                         {realCatalogPublicContract, "Protects repository Skill Source discovery and the single captured-workstation boundary while enumerating the checked-in catalog."},
	"internal/cli/pack_test.go:TestCurrentMattyActivationProjectsSurfaceCapabilities":                                        {realCatalogPublicContract, "Protects Matty's current reviewed surface-capability projections rather than generic lifecycle behavior."},
	"internal/cli/pack_test.go:TestMattyCodexActivationDryRunPreservesCurrentPublicContract":                                 {realCatalogPublicContract, "Protects Matty's current checked-in Codex resource inventory and retired-projection exclusions."},
	"internal/cli/pack_test.go:TestMattyOpenCodeActivationDryRunPreservesCurrentPublicContract":                              {realCatalogPublicContract, "Protects Matty's current checked-in OpenCode prompt and skill projection contract."},
	"internal/cli/pack_test.go:TestPackActivateEngramAcquiresOnlyWhenExecutableIsMissing":                                    {realCatalogPublicContract, "Protects Engram's reviewed external-executable acquisition capability and supported-surface contract."},
	"internal/cli/pack_test.go:TestPackActivateEngramDryRunShowsOnlyReviewedSkillAndNoEffects":                               {realCatalogPublicContract, "Protects Engram's exact reviewed skill projection and retired setup exclusions."},
	"internal/cli/pack_test.go:TestPackActivateEngramInstallsOnlyTheReviewedSkill":                                           {realCatalogPublicContract, "Protects Engram's checked-in installed skill and absence of retired host-setup artifacts."},
	"internal/cli/pack_test.go:TestRealPackCatalogListAndShowPreserveArgoteEngramMattyPublicContracts":                       {realCatalogGeneratedEvidence, "Protects reviewed catalog list/show descriptions, inventories, and supported surfaces for real Packs."},
	"internal/cli/pstack_pack_test.go:TestPstackActivationPreviewsProjectThroughEverySurfaceAdapter":                         {realCatalogPublicContract, "Protects pstack's public all-surface projection and dependency-closing preview contract."},
	"internal/cli/tui_backend_test.go:TestTUIProductionBackendUsesPackyOwnersWithoutMutatingState":                           {realCatalogPublicContract, "Protects the selectable Orchestrate catalog entry and its manifest-owned description, resources, and supported-surface matrix in the production TUI backend."},
	"internal/packsync/reconfiguration_test.go:TestCheckedInIssueDeliveryReconfigurationAcceptsExactSelectedReleaseRevision": {realCatalogGeneratedEvidence, "Protects Issue Delivery's checked-in legacy reconfiguration and selected-release admission evidence."},
}

type realCatalogInventory struct {
	packIDs map[string]struct{}
}

type realCatalogFinding struct {
	test   string
	detail string
}

// This guard deliberately checks explicit syntax and direct helper ownership.
// It is not a Go interpreter: generic behavior belongs behind synthetic fixture
// APIs, while the small exception registry owns intentional real-catalog tests.
func TestGenericTestsDoNotDependOnTheRealPackCatalog(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := loadRealCatalogInventory(filepath.Join(repositoryRoot, "bundle", "packs"))
	if err != nil {
		t.Fatal(err)
	}

	scan, err := scanRealCatalogDependencies(repositoryRoot, inventory)
	if err != nil {
		t.Fatal(err)
	}
	validateRealCatalogExceptions(t, scan)

	var unclassified []string
	for _, finding := range scan.findings {
		if _, ok := realCatalogTestExceptions[finding.test]; !ok {
			unclassified = append(unclassified, finding.test+": "+finding.detail)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("generic tests cross the checked-in catalog boundary; use a synthetic fixture or add an exact justified contract exception:\n%s", strings.Join(unclassified, "\n"))
	}
}

func TestRealCatalogGuardFollowsDirectHelpersWithoutInterpretingValueFlow(t *testing.T) {
	sources := map[string][]byte{
		"sample/helpers_test.go": []byte(`package sample
import "path/filepath"
func manifest() string { return filepath.Join("bundle", "packs", "matty", "pack.json") }
func unrelated() string { return "matty" }
`),
		"sample/scenarios_test.go": []byte(`package sample
import (
	"os"
	"path/filepath"
	"testing"
)
func use(read func(string) ([]byte, error), path string) { _, _ = read(path) }
func TestThroughHelper(t *testing.T) { _, _ = os.ReadFile(manifest()) }
func TestFunctionParameterSink(t *testing.T) { use(os.ReadFile, filepath.Join("bundle", "packs", "matty", "pack.json")) }
func TestActivationAlias(t *testing.T) { executeCommand(t, "activate", "matty") }
func TestTemporaryCopy(t *testing.T) { _, _ = os.ReadFile(filepath.Join(t.TempDir(), "bundle", "packs", "matty", "pack.json")) }
func TestUnrelatedLiteral(t *testing.T) { _ = unrelated() }
`),
	}
	scan, err := scanRealCatalogSources(sources, realCatalogInventory{packIDs: map[string]struct{}{"matty": {}}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, finding := range scan.findings {
		got[finding.test] = true
	}
	for _, want := range []string{
		"sample/scenarios_test.go:TestThroughHelper",
		"sample/scenarios_test.go:TestFunctionParameterSink",
		"sample/scenarios_test.go:TestActivationAlias",
	} {
		if !got[want] {
			t.Errorf("missing finding for %s: %+v", want, scan.findings)
		}
	}
	for _, unwanted := range []string{
		"sample/scenarios_test.go:TestTemporaryCopy",
		"sample/scenarios_test.go:TestUnrelatedLiteral",
	} {
		if got[unwanted] {
			t.Errorf("unexpected finding for %s: %+v", unwanted, scan.findings)
		}
	}
}

type realCatalogScan struct {
	findings  []realCatalogFinding
	functions map[string]struct{}
}

type scannedFunction struct {
	key        string
	packageKey string
	name       string
	direct     []string
	calls      []string
}

func loadRealCatalogInventory(packsRoot string) (realCatalogInventory, error) {
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		return realCatalogInventory{}, err
	}
	inventory := realCatalogInventory{packIDs: map[string]struct{}{}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(packsRoot, entry.Name(), "pack.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return realCatalogInventory{}, err
		}
		var manifest struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return realCatalogInventory{}, fmt.Errorf("decode %s: %w", manifestPath, err)
		}
		if manifest.ID == "" || manifest.ID != entry.Name() {
			return realCatalogInventory{}, fmt.Errorf("manifest %s has id %q", manifestPath, manifest.ID)
		}
		inventory.packIDs[manifest.ID] = struct{}{}
	}
	return inventory, nil
}

func scanRealCatalogDependencies(repositoryRoot string, inventory realCatalogInventory) (realCatalogScan, error) {
	sources := map[string][]byte{}
	err := filepath.WalkDir(filepath.Join(repositoryRoot, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") || entry.Name() == "real_catalog_test_guard_test.go" {
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
		return realCatalogScan{}, err
	}
	return scanRealCatalogSources(sources, inventory)
}

func scanRealCatalogSources(sources map[string][]byte, inventory realCatalogInventory) (realCatalogScan, error) {
	files := token.NewFileSet()
	functions := map[string]*scannedFunction{}
	byPackageName := map[string][]string{}
	for path, source := range sources {
		file, err := parser.ParseFile(files, path, source, 0)
		if err != nil {
			return realCatalogScan{}, fmt.Errorf("parse %s: %w", path, err)
		}
		packageKey := filepath.ToSlash(filepath.Dir(path)) + ":" + file.Name.Name
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Recv != nil {
				continue
			}
			key := filepath.ToSlash(path) + ":" + function.Name.Name
			scanned := &scannedFunction{
				key:        key,
				packageKey: packageKey,
				name:       function.Name.Name,
				direct:     directRealCatalogRisks(function.Body, inventory),
				calls:      directFunctionCalls(function.Body),
			}
			functions[key] = scanned
			byPackageName[packageKey+":"+function.Name.Name] = append(byPackageName[packageKey+":"+function.Name.Name], key)
		}
	}

	allFunctions := make(map[string]struct{}, len(functions))
	var findings []realCatalogFinding
	for key, function := range functions {
		allFunctions[key] = struct{}{}
		if !strings.HasPrefix(function.name, "Test") {
			continue
		}
		risks := functionRisks(function, functions, byPackageName, map[string]bool{})
		for _, detail := range uniqueStrings(risks) {
			findings = append(findings, realCatalogFinding{test: key, detail: detail})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].test == findings[j].test {
			return findings[i].detail < findings[j].detail
		}
		return findings[i].test < findings[j].test
	})
	return realCatalogScan{findings: findings, functions: allFunctions}, nil
}

func functionRisks(function *scannedFunction, functions map[string]*scannedFunction, byPackageName map[string][]string, visiting map[string]bool) []string {
	if visiting[function.key] {
		return nil
	}
	visiting[function.key] = true
	defer delete(visiting, function.key)
	risks := append([]string{}, function.direct...)
	for _, called := range function.calls {
		for _, key := range byPackageName[function.packageKey+":"+called] {
			if callee := functions[key]; callee != nil {
				risks = append(risks, functionRisks(callee, functions, byPackageName, visiting)...)
			}
		}
	}
	return risks
}

func directRealCatalogRisks(body *ast.BlockStmt, inventory realCatalogInventory) []string {
	repositoryRoot := false
	catalogSignal := false
	var risks []string
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.CallExpr:
			if isRepositoryRootConstruction(node) {
				repositoryRoot = true
			}
			if detail := checkedInManifestRisk(node, inventory); detail != "" {
				risks = append(risks, detail)
			}
			if detail := lifecycleIdentityRisk(node, inventory); detail != "" {
				risks = append(risks, detail)
			}
			if isCatalogBoundaryCall(node) && containsStaticRepositoryBundle(node) {
				risks = append(risks, "opens the checked-in Pack catalog")
			}
			if isCatalogDiscoveryCall(node) || containsManifestShape(node) {
				catalogSignal = true
			}
		case *ast.CompositeLit:
			if detail := lifecycleIdentityRisk(node, inventory); detail != "" {
				risks = append(risks, detail)
			}
			if compositeBindsStaticRepositoryBundle(node) {
				risks = append(risks, "binds the checked-in Pack catalog")
			}
		case *ast.BasicLit:
			if detail := checkedInManifestRisk(node, inventory); detail != "" {
				risks = append(risks, detail)
			}
			value, ok := basicStringLiteral(node)
			if ok && value == "PACKY_SKILLS_SOURCE" {
				catalogSignal = true
			}
		}
		return true
	})
	if repositoryRoot && catalogSignal {
		risks = append(risks, "constructs the repository's checked-in Pack catalog root")
	}
	return uniqueStrings(risks)
}

func directFunctionCalls(body *ast.BlockStmt) []string {
	var calls []string
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if identifier, ok := call.Fun.(*ast.Ident); ok {
			calls = append(calls, identifier.Name)
		}
		return true
	})
	return uniqueStrings(calls)
}

func checkedInManifestRisk(expression ast.Expr, inventory realCatalogInventory) string {
	parts, allStatic := pathParts(expression)
	index := manifestPartsIndex(parts)
	if index < 0 || containsCallNamed(expression, "TempDir") {
		return ""
	}
	if !allStatic || !repositoryRelativePrefix(parts[:index]) {
		return ""
	}
	packID := parts[index+2]
	if _, ok := inventory.packIDs[packID]; ok {
		return fmt.Sprintf("references checked-in manifest bundle/packs/%s/pack.json", packID)
	}
	return ""
}

func manifestPartsIndex(parts []string) int {
	for index := 0; index+3 < len(parts); index++ {
		if parts[index] == "bundle" && parts[index+1] == "packs" && parts[index+3] == "pack.json" {
			return index
		}
	}
	return -1
}

func repositoryRelativePrefix(parts []string) bool {
	for _, part := range parts {
		if part != "." && part != ".." && part != "" {
			return false
		}
	}
	return true
}

func pathParts(expression ast.Expr) ([]string, bool) {
	if value, ok := constantString(expression); ok {
		return splitPath(value), true
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || qualifiedName(call.Fun) != "filepath.Join" {
		return nil, false
	}
	var parts []string
	allStatic := true
	for _, argument := range call.Args {
		if value, ok := constantString(argument); ok {
			parts = append(parts, splitPath(value)...)
		} else {
			parts = append(parts, "*")
			allStatic = false
		}
	}
	return parts, allStatic
}

func splitPath(value string) []string {
	value = strings.ReplaceAll(value, "\\", "/")
	var parts []string
	for _, part := range strings.Split(value, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func constantString(expression ast.Expr) (string, bool) {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		return basicStringLiteral(expression)
	case *ast.ParenExpr:
		return constantString(expression.X)
	case *ast.BinaryExpr:
		if expression.Op != token.ADD {
			return "", false
		}
		left, leftOK := constantString(expression.X)
		right, rightOK := constantString(expression.Y)
		return left + right, leftOK && rightOK
	}
	return "", false
}

func basicStringLiteral(literal *ast.BasicLit) (string, bool) {
	if literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func lifecycleIdentityRisk(node ast.Node, inventory realCatalogInventory) string {
	values := stringValues(node)
	var packID string
	for _, value := range values {
		if _, ok := inventory.packIDs[value]; ok {
			packID = value
			break
		}
	}
	if packID == "" {
		return ""
	}

	switch node := node.(type) {
	case *ast.CallExpr:
		name := qualifiedName(node.Fun)
		if name == "executeCommand" || name == "run" || strings.HasSuffix(name, ".Run") {
			for _, operation := range []string{"activate", "update", "status", "deactivate", "install", "uninstall", "show"} {
				if slicesContain(values, operation) {
					return fmt.Sprintf("uses real Pack activation alias %q", packID)
				}
			}
		}
		for _, lookup := range []string{"Show", "ShowDetail", "ResolveIntentPack", "ValidatePackContent", "checkedInPackVersion"} {
			if name == lookup || strings.HasSuffix(name, "."+lookup) {
				return fmt.Sprintf("looks up real Pack identity %q", packID)
			}
		}
	case *ast.CompositeLit:
		typeName := qualifiedName(node.Type)
		if typeName == "ActivationRequest" || strings.HasSuffix(typeName, ".ActivationRequest") || typeName == "UpdateRequest" || strings.HasSuffix(typeName, ".UpdateRequest") || typeName == "DeactivationRequest" || strings.HasSuffix(typeName, ".DeactivationRequest") {
			return fmt.Sprintf("binds real Pack lifecycle identity %q", packID)
		}
		if typeName == "[]string" {
			for _, operation := range []string{"activate", "update", "status", "deactivate", "install", "uninstall", "show"} {
				if slicesContain(values, operation) {
					return fmt.Sprintf("declares real Pack command alias %q", packID)
				}
			}
		}
	}
	return ""
}

func stringValues(node ast.Node) []string {
	var values []string
	ast.Inspect(node, func(candidate ast.Node) bool {
		literal, ok := candidate.(*ast.BasicLit)
		if !ok {
			return true
		}
		if value, ok := basicStringLiteral(literal); ok {
			values = append(values, value)
		}
		return true
	})
	return values
}

func qualifiedName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		prefix := qualifiedName(expression.X)
		if prefix == "" {
			return expression.Sel.Name
		}
		return prefix + "." + expression.Sel.Name
	case *ast.ArrayType:
		return "[]" + qualifiedName(expression.Elt)
	case *ast.StarExpr:
		return qualifiedName(expression.X)
	}
	return ""
}

func isRepositoryRootConstruction(call *ast.CallExpr) bool {
	if qualifiedName(call.Fun) != "filepath.Abs" {
		return false
	}
	for _, argument := range call.Args {
		parts, allStatic := pathParts(argument)
		if !allStatic {
			continue
		}
		parents := 0
		for _, part := range parts {
			if part == ".." {
				parents++
			}
		}
		if parents >= 2 {
			return true
		}
	}
	return false
}

func isCatalogBoundaryCall(call *ast.CallExpr) bool {
	name := qualifiedName(call.Fun)
	for _, boundary := range []string{"Discover", "DiscoverForDurableIntents", "ValidatePackContent", "ValidatePortableContent", "LoadCurrentManifest"} {
		if name == boundary || strings.HasSuffix(name, "."+boundary) {
			return true
		}
	}
	return false
}

func isCatalogDiscoveryCall(call *ast.CallExpr) bool {
	name := qualifiedName(call.Fun)
	return name == "Discover" || strings.HasSuffix(name, ".Discover") || name == "DiscoverForDurableIntents" || strings.HasSuffix(name, ".DiscoverForDurableIntents")
}

func containsStaticRepositoryBundle(node ast.Node) bool {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return staticRepositoryBundle(node)
	}
	for _, argument := range call.Args {
		if staticRepositoryBundle(argument) {
			return true
		}
	}
	return false
}

func staticRepositoryBundle(node ast.Node) bool {
	expression, ok := node.(ast.Expr)
	if !ok || containsCallNamed(expression, "TempDir") {
		return false
	}
	parts, allStatic := pathParts(expression)
	if !allStatic {
		return false
	}
	for index, part := range parts {
		if part == "bundle" && repositoryRelativePrefix(parts[:index]) {
			return true
		}
	}
	return false
}

func compositeBindsStaticRepositoryBundle(composite *ast.CompositeLit) bool {
	for _, element := range composite.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if ok && key.Name == "bundleRoot" && staticRepositoryBundle(pair.Value) {
			return true
		}
	}
	return false
}

func containsManifestShape(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		expression, ok := candidate.(ast.Expr)
		if !ok {
			return true
		}
		parts, _ := pathParts(expression)
		if manifestPartsIndex(parts) >= 0 {
			found = true
		}
		return !found
	})
	return found
}

func containsCallNamed(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallExpr)
		if ok {
			called := qualifiedName(call.Fun)
			if called == name || strings.HasSuffix(called, "."+name) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func validateRealCatalogExceptions(t *testing.T, scan realCatalogScan) {
	t.Helper()
	allowedCategories := map[realCatalogExceptionCategory]struct{}{
		realCatalogPublicContract: {}, realCatalogGeneratedEvidence: {}, realCatalogIntegrationSmoke: {},
	}
	integrationSmokes := 0
	for testName, exception := range realCatalogTestExceptions {
		if _, ok := allowedCategories[exception.category]; !ok {
			t.Errorf("real catalog exception %s has invalid category %q", testName, exception.category)
		}
		if strings.TrimSpace(exception.justification) == "" {
			t.Errorf("real catalog exception %s has an empty contract justification", testName)
		}
		if _, ok := scan.functions[testName]; !ok {
			t.Errorf("real catalog exception %s names a missing test", testName)
		}
		if exception.category == realCatalogIntegrationSmoke {
			integrationSmokes++
		}
	}
	if integrationSmokes != 1 {
		t.Errorf("real catalog exceptions contain %d integration smokes; issue #725 requires exactly one", integrationSmokes)
	}
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
