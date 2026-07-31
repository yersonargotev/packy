package cli

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type cliSource struct {
	name string
	text string
}

func TestCLIWorkstationLayoutOwnershipIsContracted(t *testing.T) {
	if _, err := os.Stat("paths.go"); !os.IsNotExist(err) {
		t.Fatalf("obsolete shared CLI layout file paths.go still exists")
	}

	knownArtifactParts := map[string]bool{
		".packy": true, "config.json": true, "packs.json": true,
		".agents": true, "skills": true,
		".local": true, "share": true, "packy": true,
		".codex": true, "config.toml": true, "AGENTS.md": true,
		"opencode": true, "opencode.json": true, "packy.md": true,
		"bin": true, "engram": true,
	}
	knownArtifactText := map[string]bool{
		".packy": true, "config.json": true, "packs.json": true,
		".agents": true, ".codex": true, "config.toml": true,
		"AGENTS.md": true, "opencode.json": true, "packy.md": true,
	}

	for _, source := range cliGoSources(t) {
		if strings.HasSuffix(source.name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(token.NewFileSet(), source.name, source.text, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", source.name, err)
		}
		imports := importedPackages(file)
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if map[string]bool{"ResolvePaths": true, "DefaultInstalledSourceRoot": true, "resolveSkillSourceRoot": true}[declaration.Name.Name] {
					t.Errorf("%s reintroduced obsolete shared layout function %s", source.name, declaration.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if ok && map[string]bool{"Paths": true, "SkillSource": true, "SkillSourceOrigin": true}[typeSpec.Name.Name] {
						t.Errorf("%s reintroduced obsolete shared layout type %s", source.name, typeSpec.Name.Name)
					}
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CallExpr:
				if isImportedCall(node.Fun, imports, "path/filepath", "Join") {
					for _, arg := range node.Args {
						literal, ok := arg.(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING {
							continue
						}
						value, err := strconv.Unquote(literal.Value)
						if err == nil && knownArtifactParts[value] {
							t.Errorf("%s derives known artifact layout in CLI through filepath.Join component %q", source.name, value)
						}
					}
				}
				if isImportedCall(node.Fun, imports, "fmt", "Sprintf") && containsArtifactLiteral(node, knownArtifactText) {
					t.Errorf("%s derives known artifact layout in CLI through fmt.Sprintf", source.name)
				}
			case *ast.BinaryExpr:
				if node.Op == token.ADD && containsArtifactLiteral(node, knownArtifactText) {
					t.Errorf("%s derives known artifact layout in CLI through string concatenation", source.name)
				}
			}
			return true
		})
	}
}

func TestCLISourceSelectionHasOneSharedProductionRoute(t *testing.T) {
	var installedSourceResolutions, skillSourceResolutions int
	for _, source := range cliGoSources(t) {
		if strings.HasSuffix(source.name, "_test.go") {
			continue
		}
		installedSourceResolutions += strings.Count(source.text, "bootstrap.ResolveInstalledSource(")
		skillSourceResolutions += strings.Count(source.text, "skillbundle.ResolveSource(")
	}
	if installedSourceResolutions != 2 {
		t.Fatalf("CLI has %d Installed Source resolution routes, want init plus one shared command route", installedSourceResolutions)
	}
	if skillSourceResolutions != 1 {
		t.Fatalf("CLI has %d Skill Source selection routes, want one shared command route", skillSourceResolutions)
	}
}

func containsArtifactLiteral(node ast.Node, known map[string]bool) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		for artifact := range known {
			if value == artifact || strings.Contains(value, "/"+artifact) || strings.Contains(value, artifact+"/") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func TestAmbientWorkstationReadsStayAtApprovedProcessEdges(t *testing.T) {
	allowed := map[string]map[string]bool{
		filepath.Join("..", "cli", "env.go"):  {"Getenv": true},
		filepath.Join("..", "cli", "root.go"): {"Getwd": true},
	}
	files, err := filepath.Glob(filepath.Join("..", "*", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		imports := importedPackages(file)
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !isImportedSelector(selector, imports, "os") {
				return true
			}
			if !map[string]bool{"Getenv": true, "UserHomeDir": true, "Getwd": true}[selector.Sel.Name] {
				return true
			}
			if !allowed[filepath.Clean(path)][selector.Sel.Name] {
				t.Errorf("%s reads ambient workstation state outside the approved process edge through os.%s", path, selector.Sel.Name)
			}
			return true
		})
	}
}

func importedPackages(file *ast.File) map[string]string {
	imports := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports
}

func isImportedCall(expr ast.Expr, imports map[string]string, importPath, functionName string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != functionName {
		return false
	}
	return isImportedSelector(selector, imports, importPath)
}

func isImportedSelector(selector *ast.SelectorExpr, imports map[string]string, importPath string) bool {
	identifier, ok := selector.X.(*ast.Ident)
	return ok && imports[identifier.Name] == importPath
}

func cliGoSources(t *testing.T) []cliSource {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]cliSource, 0, len(files))
	for _, file := range files {
		if file == "architecture_test.go" {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, cliSource{name: file, text: string(source)})
	}
	return sources
}

func TestClassicLifecycleDeletionDoesNotRedistributePolicyInCLI(t *testing.T) {
	for _, obsolete := range []string{"plan.go", "skills.go"} {
		if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
			t.Fatalf("obsolete CLI lifecycle module %s still exists", obsolete)
		}
	}

	for _, source := range cliGoSources(t) {
		file := source.name
		for _, forbidden := range []string{
			"type Plan struct",
			"type PlannedAction struct",
			"type ActionKind string",
			"func DiscoverManagedSkills(",
			"func plannedSkillLinkAction(",
			"func inspectSkillLink(",
			"skillLinkBehaviors",
			"unmanagedSymlinkSkipSummary",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s retained or redistributed obsolete classic lifecycle structure %q", file, forbidden)
			}
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, forbidden := range []string{"os.Lstat(", "os.Readlink(", "os.Symlink(", "os.Remove(", "skillbundle.Discover(", "corelifecycle.LoadState(", "corelifecycle.SaveState("} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s redistributed classic lifecycle policy through %q", file, forbidden)
			}
		}
	}

	root, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	if problems := classicLifecycleArchitectureProblems(root); len(problems) > 0 {
		t.Fatalf("classic lifecycle architecture drifted:\n- %s", strings.Join(problems, "\n- "))
	}
}

func TestClassicLifecycleArchitectureGuardUsesGoSyntax(t *testing.T) {
	root, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := format.Source(root)
	if err != nil {
		t.Fatal(err)
	}
	noise := append(append([]byte(nil), formatted...), []byte(`

// lifecycle.Preview(operation); lifecycle.Apply(ctx, plan)
var classicLifecycleArchitectureNoise = "corelifecycle.NewFacade(); return executeClassicLifecycle(corelifecycle.Uninstall)"
`)...)
	if problems := classicLifecycleArchitectureProblems(noise); len(problems) > 0 {
		t.Fatalf("formatting, comments, or string literals affected the semantic guard:\n- %s", strings.Join(problems, "\n- "))
	}

	mutations := []struct {
		name   string
		mutate func(*ast.File) bool
	}{
		{name: "bypass shared executor", mutate: func(file *ast.File) bool {
			call := classicLifecycleRouteCall(file, "newInstallCommand")
			if call == nil {
				return false
			}
			call.Fun = ast.NewIdent("bypassClassicLifecycle")
			return true
		}},
		{name: "shadow shared executor", mutate: func(file *ast.File) bool {
			runE := classicLifecycleRunE(file, "newInstallCommand")
			if runE == nil {
				return false
			}
			shadow, err := parser.ParseExpr("func(args ...any) error { return nil }")
			if err != nil {
				return false
			}
			runE.Body.List = append([]ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("executeClassicLifecycle")},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{shadow},
			}}, runE.Body.List...)
			return true
		}},
		{name: "duplicate delegation through alias", mutate: func(file *ast.File) bool {
			runE := classicLifecycleRunE(file, "newInstallCommand")
			call := classicLifecycleRouteCall(file, "newInstallCommand")
			if runE == nil || call == nil {
				return false
			}
			runE.Body.List = append([]ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent("classicLifecycleDelegate")},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{ast.NewIdent("executeClassicLifecycle")},
				},
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun:  ast.NewIdent("classicLifecycleDelegate"),
					Args: call.Args,
				}},
			}, runE.Body.List...)
			return true
		}},
		{name: "duplicate delegation", mutate: func(file *ast.File) bool {
			function := classicLifecycleFunction(file, "newInstallCommand")
			call := classicLifecycleRouteCall(file, "newInstallCommand")
			if function == nil || call == nil {
				return false
			}
			function.Body.List = append(function.Body.List, &ast.ExprStmt{X: call})
			return true
		}},
		{name: "misroute update", mutate: func(file *ast.File) bool {
			call := classicLifecycleRouteCall(file, "newUpdateCommand")
			if call == nil || len(call.Args) <= 3 {
				return false
			}
			selector, ok := call.Args[3].(*ast.SelectorExpr)
			if !ok {
				return false
			}
			selector.Sel = ast.NewIdent("Install")
			return true
		}},
		{name: "shadow operation package", mutate: func(file *ast.File) bool {
			runE := classicLifecycleRunE(file, "newUpdateCommand")
			if runE == nil {
				return false
			}
			shadow, err := parser.ParseExpr(
				"struct{ Update corelifecycle.Operation }{Update: corelifecycle.Install}",
			)
			if err != nil {
				return false
			}
			runE.Body.List = append([]ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("corelifecycle")},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{shadow},
			}}, runE.Body.List...)
			return true
		}},
		{name: "nested return cannot satisfy route", mutate: func(file *ast.File) bool {
			runE := classicLifecycleRunE(file, "newInstallCommand")
			call := classicLifecycleRouteCall(file, "newInstallCommand")
			if runE == nil || call == nil {
				return false
			}
			nestedCall := &ast.CallExpr{Fun: ast.NewIdent("executeClassicLifecycle"), Args: call.Args}
			call.Fun = ast.NewIdent("bypassClassicLifecycle")
			runE.Body.List = append([]ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.FuncLit{
					Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{
						List: []*ast.Field{{Type: ast.NewIdent("error")}},
					}},
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{
						Results: []ast.Expr{nestedCall},
					}}},
				}},
			}}, runE.Body.List...)
			return true
		}},
		{name: "facade outside executor", mutate: func(file *ast.File) bool {
			function := classicLifecycleFunction(file, "newInstallCommand")
			call := classicLifecycleFacadeCreation(file)
			if function == nil || call == nil {
				return false
			}
			function.Body.List = append(function.Body.List, &ast.ExprStmt{X: call})
			return true
		}},
		{name: "duplicate preview through alias", mutate: func(file *ast.File) bool {
			function := classicLifecycleFunction(file, "executeClassicLifecycle")
			if function == nil {
				return false
			}
			function.Body.List = append(function.Body.List,
				&ast.DeclStmt{Decl: &ast.GenDecl{
					Tok: token.VAR,
					Specs: []ast.Spec{&ast.ValueSpec{
						Names:  []*ast.Ident{ast.NewIdent("lifecycleAlias")},
						Values: []ast.Expr{ast.NewIdent("lifecycle")},
					}},
				}},
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun:  &ast.SelectorExpr{X: ast.NewIdent("lifecycleAlias"), Sel: ast.NewIdent("Preview")},
					Args: []ast.Expr{ast.NewIdent("operation")},
				}},
			)
			return true
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := mutateClassicLifecycleSource(t, noise, mutation.mutate)
			if problems := classicLifecycleArchitectureProblems(mutated); len(problems) == 0 {
				t.Fatal("semantic architecture guard accepted the deliberate mutation")
			}
		})
	}
	if problems := classicLifecycleArchitectureProblems(root); len(problems) > 0 {
		t.Fatalf("restored production source did not pass:\n- %s", strings.Join(problems, "\n- "))
	}
}

const coreLifecycleImportPath = "github.com/yersonargotev/packy/internal/corelifecycle"

func classicLifecycleArchitectureProblems(source []byte) []string {
	file, err := parser.ParseFile(token.NewFileSet(), "root.go", source, parser.ParseComments)
	if err != nil {
		return []string{fmt.Sprintf("parse root.go: %v", err)}
	}
	imports := importedPackages(file)
	expectedRoutes := map[string]string{
		"newInstallCommand":   "Install",
		"newUpdateCommand":    "Update",
		"newUninstallCommand": "Uninstall",
	}
	routes := map[string][]string{}
	runERoutes := map[string][]string{}
	facadeCreations := map[string]int{}
	previewCalls := map[string]int{}
	applyCalls := map[string]int{}
	executorDeclarations := 0
	var executorObject *ast.Object

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "executeClassicLifecycle" {
			executorDeclarations++
			executorObject = function.Name.Obj
		}
	}

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		owner := function.Name.Name
		facadeVariables := classicLifecycleFacadeVariables(function, imports)
		executorObjects := classicLifecycleExecutorObjects(function, executorObject)
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isImportedCall(call.Fun, imports, coreLifecycleImportPath, "NewFacade") {
				facadeCreations[owner]++
			}
			if classicLifecycleCallsObject(call, executorObjects) {
				routes[owner] = append(routes[owner], classicLifecycleOperation(call, imports))
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if !classicLifecycleFacadeExpression(selector.X, facadeVariables) {
				return true
			}
			switch selector.Sel.Name {
			case "Preview":
				previewCalls[owner]++
			case "Apply":
				applyCalls[owner]++
			}
			return true
		})
		if runE := classicLifecycleRunE(file, owner); runE != nil {
			for _, node := range runE.Body.List {
				statement, ok := node.(*ast.ReturnStmt)
				if !ok || len(statement.Results) != 1 {
					continue
				}
				call, ok := statement.Results[0].(*ast.CallExpr)
				if ok && classicLifecycleCallsObject(call, executorObjects) {
					runERoutes[owner] = append(runERoutes[owner], classicLifecycleOperation(call, imports))
				}
			}
		}
	}

	var problems []string
	if executorDeclarations != 1 {
		problems = append(problems, fmt.Sprintf("found %d executeClassicLifecycle declarations, want 1", executorDeclarations))
	}
	for owner, count := range facadeCreations {
		if owner != "executeClassicLifecycle" || count != 1 {
			problems = append(problems, fmt.Sprintf("%s creates %d core lifecycle facades", owner, count))
		}
	}
	if facadeCreations["executeClassicLifecycle"] != 1 {
		problems = append(problems, fmt.Sprintf(
			"executeClassicLifecycle creates %d core lifecycle facades, want 1",
			facadeCreations["executeClassicLifecycle"],
		))
	}
	for owner, count := range previewCalls {
		if owner != "executeClassicLifecycle" || count != 1 {
			problems = append(problems, fmt.Sprintf("%s calls Preview %d times on a core lifecycle facade", owner, count))
		}
	}
	for owner, count := range applyCalls {
		if owner != "executeClassicLifecycle" || count != 1 {
			problems = append(problems, fmt.Sprintf("%s calls Apply %d times on a core lifecycle facade", owner, count))
		}
	}
	if previewCalls["executeClassicLifecycle"] != 1 {
		problems = append(problems, fmt.Sprintf(
			"executeClassicLifecycle calls Preview %d times, want 1",
			previewCalls["executeClassicLifecycle"],
		))
	}
	if applyCalls["executeClassicLifecycle"] != 1 {
		problems = append(problems, fmt.Sprintf(
			"executeClassicLifecycle calls Apply %d times, want 1",
			applyCalls["executeClassicLifecycle"],
		))
	}
	for owner := range routes {
		if _, allowed := expectedRoutes[owner]; !allowed {
			problems = append(problems, fmt.Sprintf("%s delegates through executeClassicLifecycle", owner))
		}
	}
	for owner, expected := range expectedRoutes {
		operations := routes[owner]
		if len(operations) != 1 || operations[0] != expected {
			problems = append(problems, fmt.Sprintf("%s delegates with %v, want exactly [%s]", owner, operations, expected))
		}
		runEOperations := runERoutes[owner]
		if len(runEOperations) != 1 || runEOperations[0] != expected {
			problems = append(problems, fmt.Sprintf(
				"%s RunE returns delegations %v, want exactly [%s]",
				owner, runEOperations, expected,
			))
		}
	}
	sort.Strings(problems)
	return problems
}

func mutateClassicLifecycleSource(t *testing.T, source []byte, mutate func(*ast.File) bool) []byte {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "root.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if !mutate(file) {
		t.Fatal("could not apply semantic mutation")
	}
	var output bytes.Buffer
	if err := format.Node(&output, fileSet, file); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func classicLifecycleFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == name {
			return function
		}
	}
	return nil
}

func classicLifecycleRunE(file *ast.File, functionName string) *ast.FuncLit {
	function := classicLifecycleFunction(file, functionName)
	if function == nil {
		return nil
	}
	var runE *ast.FuncLit
	ast.Inspect(function.Body, func(node ast.Node) bool {
		field, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := field.Key.(*ast.Ident)
		value, valueOK := field.Value.(*ast.FuncLit)
		if ok && valueOK && key.Name == "RunE" {
			runE = value
			return false
		}
		return true
	})
	return runE
}

func classicLifecycleRouteCall(file *ast.File, functionName string) *ast.CallExpr {
	runE := classicLifecycleRunE(file, functionName)
	if runE == nil {
		return nil
	}
	var route *ast.CallExpr
	ast.Inspect(runE.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		identifier, identifierOK := callFunctionIdentifier(call)
		if ok && identifierOK && identifier.Name == "executeClassicLifecycle" {
			route = call
			return false
		}
		return true
	})
	return route
}

func classicLifecycleFacadeCreation(file *ast.File) *ast.CallExpr {
	imports := importedPackages(file)
	function := classicLifecycleFunction(file, "executeClassicLifecycle")
	if function == nil {
		return nil
	}
	var creation *ast.CallExpr
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && isImportedCall(call.Fun, imports, coreLifecycleImportPath, "NewFacade") {
			creation = call
			return false
		}
		return true
	})
	return creation
}

func classicLifecycleFacadeVariables(function *ast.FuncDecl, imports map[string]string) map[string]bool {
	variables := map[string]bool{}
	if function.Type.Params != nil {
		for _, field := range function.Type.Params.List {
			if !classicLifecycleFacadeType(field.Type, imports) {
				continue
			}
			for _, name := range field.Names {
				variables[name.Name] = true
			}
		}
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		declaration, ok := node.(*ast.ValueSpec)
		if !ok || !classicLifecycleFacadeType(declaration.Type, imports) {
			return true
		}
		for _, name := range declaration.Names {
			variables[name.Name] = true
		}
		return true
	})
	for changed := true; changed; {
		changed = false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				for index, expression := range node.Rhs {
					if index >= len(node.Lhs) || !classicLifecycleFacadeValue(expression, variables, imports) {
						continue
					}
					identifier, ok := node.Lhs[index].(*ast.Ident)
					if ok && !variables[identifier.Name] {
						variables[identifier.Name] = true
						changed = true
					}
				}
			case *ast.ValueSpec:
				for index, expression := range node.Values {
					if index >= len(node.Names) || !classicLifecycleFacadeValue(expression, variables, imports) {
						continue
					}
					if !variables[node.Names[index].Name] {
						variables[node.Names[index].Name] = true
						changed = true
					}
				}
			}
			return true
		})
	}
	return variables
}

func classicLifecycleFacadeValue(expression ast.Expr, variables map[string]bool, imports map[string]string) bool {
	call, ok := expression.(*ast.CallExpr)
	if ok && isImportedCall(call.Fun, imports, coreLifecycleImportPath, "NewFacade") {
		return true
	}
	return classicLifecycleFacadeExpression(expression, variables)
}

func classicLifecycleFacadeExpression(expression ast.Expr, variables map[string]bool) bool {
	switch expression := expression.(type) {
	case *ast.Ident:
		return variables[expression.Name]
	case *ast.ParenExpr:
		return classicLifecycleFacadeExpression(expression.X, variables)
	default:
		return false
	}
}

func classicLifecycleFacadeType(expression ast.Expr, imports map[string]string) bool {
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Facade" && isImportedSelector(selector, imports, coreLifecycleImportPath)
}

func classicLifecycleExecutorObjects(function *ast.FuncDecl, executor *ast.Object) map[*ast.Object]bool {
	objects := map[*ast.Object]bool{}
	if executor != nil {
		objects[executor] = true
	}
	for changed := true; changed; {
		changed = false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			var names []*ast.Ident
			var values []ast.Expr
			switch node := node.(type) {
			case *ast.AssignStmt:
				for _, expression := range node.Lhs {
					identifier, ok := expression.(*ast.Ident)
					if !ok {
						return true
					}
					names = append(names, identifier)
				}
				values = node.Rhs
			case *ast.ValueSpec:
				names, values = node.Names, node.Values
			default:
				return true
			}
			for index, expression := range values {
				if index >= len(names) {
					continue
				}
				identifier, ok := expression.(*ast.Ident)
				if !ok || !objects[identifier.Obj] || names[index].Obj == nil || objects[names[index].Obj] {
					continue
				}
				objects[names[index].Obj] = true
				changed = true
			}
			return true
		})
	}
	return objects
}

func classicLifecycleCallsObject(call *ast.CallExpr, objects map[*ast.Object]bool) bool {
	identifier, ok := callFunctionIdentifier(call)
	return ok && identifier.Obj != nil && objects[identifier.Obj]
}

func callFunctionIdentifier(call *ast.CallExpr) (*ast.Ident, bool) {
	if call == nil {
		return nil, false
	}
	identifier, ok := call.Fun.(*ast.Ident)
	return identifier, ok
}

func classicLifecycleOperation(call *ast.CallExpr, imports map[string]string) string {
	if len(call.Args) <= 3 {
		return "<invalid>"
	}
	selector, ok := call.Args[3].(*ast.SelectorExpr)
	identifier, identifierOK := selector.X.(*ast.Ident)
	if !ok || !identifierOK || identifier.Obj != nil ||
		!isImportedSelector(selector, imports, coreLifecycleImportPath) {
		return "<invalid>"
	}
	return selector.Sel.Name
}

func TestSetupHealthDeletionDoesNotRedistributeDiagnosisPolicyInCLI(t *testing.T) {
	for _, obsolete := range []string{"doctor.go", "doctor_test.go"} {
		if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
			t.Fatalf("obsolete CLI setup-health file %s still exists", obsolete)
		}
	}

	for _, source := range cliGoSources(t) {
		file := source.name
		for _, forbidden := range []string{
			"BuildDoctorReport",
			"buildDoctorReport",
			"RunDoctor",
			"type DoctorReport",
			"type DoctorSummary",
			"DoctorReport =",
			"DoctorSummary =",
			"doctorCheck",
			"doctorStatus",
			"stateCheck(",
			"skillChecks(",
			"engramChecks(",
			"codexChecks(",
			"openCodeChecks(",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s retained or redistributed obsolete setup-health structure %q", file, forbidden)
			}
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, forbidden := range []string{
			"corelifecycle.ObserveState(",
			"corelifecycle.ObserveManagedSkillLinks(",
			"engrambin.Diagnose",
			"opencode.Inspect(",
			"prompt.DetectExternalManagedBlocks(",
			"\"packy-state\"",
			"\"engram-binary\"",
			"\"codex-config\"",
			"\"opencode-config\"",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s redistributed setup-health diagnosis policy through %q", file, forbidden)
			}
		}
	}

	root, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	for call, want := range map[string]int{
		"setuphealth.Diagnose(":      1,
		"opts.SetupHealthDiagnose()": 1,
	} {
		if got := strings.Count(string(root), call); got != want {
			t.Fatalf("root.go has %d occurrences of %q, want %d", got, call, want)
		}
	}
}
