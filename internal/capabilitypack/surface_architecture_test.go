package capabilitypack

import (
	"fmt"
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

type productionSource struct {
	path string
	text string
}

func internalProductionSources(t *testing.T) []productionSource {
	t.Helper()
	root := ".."
	var sources []productionSource
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources = append(sources, productionSource{path: filepath.ToSlash(path), text: string(data)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

func TestSurfaceAdapterArchitectureCannotRegress(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "opencode"+"activation")); !os.IsNotExist(err) {
		t.Fatal("obsolete OpenCode activation package exists")
	}

	sources := internalProductionSources(t)
	obsolete := []string{
		"Activation" + "Adapter",
		"Activation" + "Observation",
		"ResolutionAware" + "ActivationAdapter",
		"DeactivationAware" + "ActivationAdapter",
		"ReconciliationAware" + "ActivationAdapter",
		"Readiness" + "Inspector",
		"Surface" + "Inspector",
		"Inspect" + "Activation",
		"Inspect" + "Deactivation",
		"Inspect" + "Reconcile",
		"WithReadiness" + "Inspectors",
	}
	concreteAdapters := 0
	inspectionImplementations := 0
	applicationImplementations := 0
	directInspections := 0
	for _, source := range sources {
		if source.path == "../engrambin/engrambin.go" && strings.Contains(source.text, "unsupported executable requirement") {
			t.Fatalf("%s reintroduced fixed-name external requirement rejection", source.path)
		}
		for _, forbidden := range obsolete {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s reintroduced obsolete surface structure %q", source.path, forbidden)
			}
		}
		supportedHost := strings.Contains(source.path, "/codex/") || strings.Contains(source.path, "/opencode/") || strings.Contains(source.path, "/claudecode/")
		adapterDefinitions := strings.Count(source.text, "type SurfaceAdapter struct")
		if adapterDefinitions > 0 && !supportedHost {
			t.Fatalf("%s introduced a concrete surface adapter outside Codex, OpenCode, or Claude Code", source.path)
		}
		if supportedHost {
			dispatches, err := literalIdentityDispatches(source.path, source.text)
			if err != nil {
				t.Fatal(err)
			}
			for _, dispatch := range dispatches {
				t.Errorf("%s dispatches host behavior by literal identity %q", dispatch.position, dispatch.literal)
			}
			concreteAdapters += adapterDefinitions
			inspectionImplementations += strings.Count(source.text, ") InspectSurface(")
			applicationImplementations += strings.Count(source.text, ") ApplyProjections(")
			for _, lifecycle := range []string{"capabilitypack.OperationActivate", "capabilitypack.OperationUpdate", "capabilitypack.OperationDeactivate", "capabilitypack.OperationReconcile", "capabilitypack.ActivationRequest", "capabilitypack.UpdateRequest", "capabilitypack.DeactivationRequest", "capabilitypack.ReconcileRequest"} {
				if strings.Contains(source.text, lifecycle) {
					t.Fatalf("%s redistributed lifecycle policy through %q", source.path, lifecycle)
				}
			}
		}
		if strings.Contains(source.path, "/capabilitypack/") {
			for _, hostPolicy := range []string{"internal/codex", "internal/opencode", "internal/claudecode", "MergeInstructionProjection(", "MergeMCPProjection(", "ValidateInstructionProjection(", "ValidateMCPProjection("} {
				if strings.Contains(source.text, hostPolicy) {
					t.Fatalf("%s redistributed host policy through %q", source.path, hostPolicy)
				}
			}
		}
		if strings.Contains(source.path, "/cli/") {
			for _, policy := range []string{".InspectSurface(", ".ApplyProjections(", "RemovalCandidate(", "surfaceTransitionFacts(", "ProjectionPresent", "ProjectionAbsent"} {
				if strings.Contains(source.text, policy) {
					t.Fatalf("%s redistributed surface policy through %q", source.path, policy)
				}
			}
		}
		count := strings.Count(source.text, ".InspectSurface(")
		if count > 0 && source.path != "../capabilitypack/activation.go" {
			t.Fatalf("%s introduced a parallel production inspection route", source.path)
		}
		directInspections += count
		removalSlots := strings.Count(source.text, "Removal"+"Candidates")
		if removalSlots > 0 {
			t.Fatalf("%s reintroduced a removal-candidate side channel", source.path)
		}
	}
	if concreteAdapters != 3 {
		t.Fatalf("found %d concrete production surface adapters, want Codex, OpenCode, and Claude Code only", concreteAdapters)
	}
	if inspectionImplementations != 3 || applicationImplementations != 3 {
		t.Fatalf("found %d inspection and %d application implementations, want one complete implementation per host", inspectionImplementations, applicationImplementations)
	}
	if directInspections != 1 {
		t.Fatalf("found %d direct production InspectSurface calls, want the private gateway only", directInspections)
	}
}

type literalIdentityDispatch struct {
	position token.Position
	literal  string
}

func literalIdentityDispatches(path, source string) ([]literalIdentityDispatch, error) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, path, source, 0)
	if err != nil {
		return nil, fmt.Errorf("parse adapter %s: %w", path, err)
	}
	var dispatches []literalIdentityDispatch
	record := func(node ast.Node, expression, candidate ast.Expr) {
		if !isIdentityExpression(expression) {
			return
		}
		literal, ok := stringLiteral(candidate)
		if !ok || literal == "" {
			return
		}
		dispatches = append(dispatches, literalIdentityDispatch{position: files.Position(node.Pos()), literal: literal})
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.BinaryExpr:
			if node.Op != token.EQL && node.Op != token.NEQ {
				return true
			}
			record(node, node.X, node.Y)
			record(node, node.Y, node.X)
		case *ast.SwitchStmt:
			if node.Tag == nil || !isIdentityExpression(node.Tag) {
				return true
			}
			for _, statement := range node.Body.List {
				clause, ok := statement.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expression := range clause.List {
					record(expression, node.Tag, expression)
				}
			}
		}
		return true
	})
	return dispatches, nil
}

func isIdentityExpression(expression ast.Expr) bool {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	name := ""
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		// Host runtime evidence has its own protocol identity. It is not a Pack,
		// Pack version, or resource identity and may use a reviewed literal.
		if expression.Sel.Name == "ID" {
			if receiver, ok := expression.X.(*ast.Ident); ok && receiver.Name == "evidence" {
				return false
			}
		}
		name = expression.Sel.Name
	case *ast.Ident:
		name = expression.Name
	}
	name = strings.ToLower(name)
	return name == "id" || name == "packid" || name == "resourceid" || name == "version" || name == "packversion"
}

func stringLiteral(expression ast.Expr) (string, bool) {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func TestLiteralIdentityDispatchGuardIsNarrow(t *testing.T) {
	for name, expression := range map[string]string{
		"pack identity":               `pack.ID == "addy"`,
		"reversed Pack identity":      `"engram" != pack.PackID`,
		"resource identity":           `resource.ID != "memory"`,
		"Pack version":                `pack.Version == "1.0.0"`,
		"local resource identity":     `resourceID == "guide"`,
		"parenthesized Pack identity": `(pack.ID) == ("matty")`,
	} {
		t.Run("reject "+name, func(t *testing.T) {
			source := "package adapter\nfunc dispatch() { if " + expression + " {} }"
			dispatches, err := literalIdentityDispatches(name+".go", source)
			if err != nil || len(dispatches) != 1 {
				t.Fatalf("dispatches = %#v, err=%v", dispatches, err)
			}
		})
	}

	t.Run("reject switch", func(t *testing.T) {
		source := `package adapter
func dispatch() { switch resource.ID { case "guide", "memory": } }`
		dispatches, err := literalIdentityDispatches("switch.go", source)
		if err != nil || len(dispatches) != 2 {
			t.Fatalf("dispatches = %#v, err=%v", dispatches, err)
		}
	})

	for name, expression := range map[string]string{
		"requested Pack lookup":  `pack.ID == requestedID`,
		"ownership":              `owner.PackID != pack.ID`,
		"intent version":         `pack.Version != intent.Version`,
		"resource lookup":        `resource.ID == selected.ID`,
		"resource kind":          `resource.Kind == "skill"`,
		"host evidence identity": `evidence.ID == "project_runtime:claude"`,
		"empty identity":         `pack.ID == ""`,
	} {
		t.Run("allow "+name, func(t *testing.T) {
			source := "package adapter\nfunc compare() { if " + expression + " {} }"
			dispatches, err := literalIdentityDispatches(name+".go", source)
			if err != nil || len(dispatches) != 0 {
				t.Fatalf("dispatches = %#v, err=%v", dispatches, err)
			}
		})
	}
}
