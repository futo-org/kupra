package controller

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// kubectl shows the Ready condition's message as a preview's or snapshot's Status, so every Ready=False says why.
func TestNotReadySaysWhy(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 4 {
				return true
			}
			fn, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || fn.Sel.Name != "MarkFalse" {
				return true
			}
			if pkg, ok := fn.X.(*ast.Ident); !ok || pkg.Name != "conditions" {
				return true
			}
			if lit, ok := call.Args[3].(*ast.BasicLit); ok && lit.Value == `""` {
				t.Errorf("%s: MarkFalse without a message", fset.Position(call.Pos()))
			}
			return true
		})
	}
}
