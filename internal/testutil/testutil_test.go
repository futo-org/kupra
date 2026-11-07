package testutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// Test failures say what they got in words, not as Go structs: %+v prints pointers, nested structs and timestamps that bury what went wrong.
func TestNoStructDumps(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(strings.ReplaceAll(lit.Value, "%%", ""), "%"+"+v") {
				t.Errorf("%s: test message formats a value with %%+v", fset.Position(lit.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDescriptionsTellApart checks that descriptions differ where the tests using them compare.
func TestDescriptionsTellApart(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		{
			name: "subjects differing in API group",
			a:    Subject(rbacv1.Subject{Kind: "ServiceAccount", Namespace: "apps", Name: "ci"}),
			b:    Subject(rbacv1.Subject{Kind: "ServiceAccount", APIGroup: rbacv1.GroupName, Namespace: "apps", Name: "ci"}),
		},
		{
			name: "owners differing in UID",
			a:    Owners([]metav1.OwnerReference{{Kind: "PreviewSnapshot", Name: "base", UID: "1"}}),
			b:    Owners([]metav1.OwnerReference{{Kind: "PreviewSnapshot", Name: "base", UID: "2"}}),
		},
		{
			name: "inventory entries differing in retention",
			a:    Inventory([]kuprav1alpha1.InventoryEntry{{Kind: "PersistentVolumeClaim", Name: "data", State: true}}),
			b:    Inventory([]kuprav1alpha1.InventoryEntry{{Kind: "PersistentVolumeClaim", Name: "data", State: true, Retained: true}}),
		},
	} {
		if tc.a == tc.b {
			t.Errorf("%s are both described as %q", tc.name, tc.a)
		}
	}
}
