package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
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
		filepath.Join("..", "cli", "env.go"):                 {"Getenv": true},
		filepath.Join("..", "cli", "root.go"):                {"Getwd": true},
		filepath.Join("..", "testprocess", "environment.go"): {"Getenv": true},
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
	for {
		parenthesized, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = parenthesized.X
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != functionName {
		return false
	}
	return isImportedSelector(selector, imports, importPath)
}

func isImportedSelector(selector *ast.SelectorExpr, imports map[string]string, importPath string) bool {
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Obj == nil && imports[identifier.Name] == importPath
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

func TestCapabilityPackIsTheOnlySemanticCallerOfSurfaceProjectionApplication(t *testing.T) {
	for _, source := range productionGoSourcesBelow(t, "..") {
		rel, err := filepath.Rel("..", source.name)
		if err != nil {
			t.Fatal(err)
		}
		if rel == "capabilitypack" || strings.HasPrefix(rel, "capabilitypack"+string(filepath.Separator)) {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source.name, source.text, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", source.name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "ApplyProjections" {
				t.Errorf("%s references ApplyProjections outside internal/capabilitypack; adapters may implement projection application, but only capability-pack lifecycle may call it", source.name)
			}
			return true
		})
	}
}

func productionGoSourcesBelow(t *testing.T, root string) []cliSource {
	t.Helper()
	var sources []cliSource
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources = append(sources, cliSource{name: filepath.Clean(path), text: string(data)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sources
}
