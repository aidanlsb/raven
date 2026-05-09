package codes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestEnvelopeCodeLiteralsAreDocumented(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fset := token.NewFileSet()

	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.CallExpr:
				checkErrorEnvelopeCall(t, fset, path, typed)
			case *ast.CompositeLit:
				checkWarningComposite(t, fset, path, file.Name.Name, typed)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkErrorEnvelopeCall(t *testing.T, fset *token.FileSet, path string, call *ast.CallExpr) {
	if len(call.Args) == 0 || !isErrorEnvelopeBuilder(call.Fun) {
		return
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return
	}
	code, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Fatalf("%s: invalid code literal %s", position(fset, path, lit.Pos()), lit.Value)
	}
	if !IsErrorCode(code) {
		t.Fatalf("%s: error code %q is not documented in internal/codes", position(fset, path, lit.Pos()), code)
	}
}

func checkWarningComposite(t *testing.T, fset *token.FileSet, path, pkg string, lit *ast.CompositeLit) {
	if !isWarningComposite(pkg, lit.Type) {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Code" {
			continue
		}
		codeLit, ok := kv.Value.(*ast.BasicLit)
		if !ok || codeLit.Kind != token.STRING {
			continue
		}
		code, err := strconv.Unquote(codeLit.Value)
		if err != nil {
			t.Fatalf("%s: invalid warning code literal %s", position(fset, path, codeLit.Pos()), codeLit.Value)
		}
		if !IsWarningCode(code) {
			t.Fatalf("%s: warning code %q is not documented in internal/codes", position(fset, path, codeLit.Pos()), code)
		}
	}
}

func isErrorEnvelopeBuilder(fun ast.Expr) bool {
	switch typed := fun.(type) {
	case *ast.SelectorExpr:
		return typed.Sel.Name == "Failure"
	case *ast.Ident:
		switch typed.Name {
		case "Failure", "errorEnvelope", "fallbackEnvelopeJSON":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func isWarningComposite(pkg string, expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		return typed.Sel.Name == "Warning"
	case *ast.Ident:
		return typed.Name == "Warning" && (pkg == "commandexec" || pkg == "schemasvc" || pkg == "templatesvc" || pkg == "initsvc")
	default:
		return false
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func position(fset *token.FileSet, path string, pos token.Pos) string {
	p := fset.Position(pos)
	if rel, err := filepath.Rel(filepath.Dir(filepath.Dir(path)), p.Filename); err == nil {
		return filepath.ToSlash(rel) + ":" + strconv.Itoa(p.Line)
	}
	return filepath.ToSlash(p.Filename) + ":" + strconv.Itoa(p.Line)
}
