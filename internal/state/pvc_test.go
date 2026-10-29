package state

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

func TestPersistentVolumeClaimCheck(t *testing.T) {
	kind, ok := For(schema.GroupKind{Kind: "PersistentVolumeClaim"})
	if !ok {
		t.Fatal("claims aren't supported")
	}
	for _, tc := range []struct {
		name     string
		resource string
		want     []string
	}{
		{"plain", `spec: {accessModes: [ReadWriteOnce]}`, nil},
		{"data source", `spec: {dataSource: {kind: VolumeSnapshot, name: s}, dataSourceRefExpr: '{}'}`, []string{"spec.dataSource", "spec.dataSourceRefExpr"}},
		{"spec expression", `specExpr: '{}'`, []string{"specExpr"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resource map[string]any
			if err := yaml.Unmarshal([]byte(tc.resource), &resource); err != nil {
				t.Fatal(err)
			}
			errs := kind.Check(resource, field.NewPath("r"))
			if len(errs) != len(tc.want) {
				t.Fatalf("errors = %v, want %d", errs, len(tc.want))
			}
			for i, want := range tc.want {
				if !strings.HasPrefix(errs[i].Field, "r."+want) {
					t.Errorf("error %d is about %s, want %s", i, errs[i].Field, want)
				}
			}
		})
	}
}

func TestPersistentVolumeClaimReady(t *testing.T) {
	kind, _ := For(schema.GroupKind{Kind: "PersistentVolumeClaim"})
	if kind.Ready(nil) {
		t.Error("a missing claim is ready")
	}
	// A claim waiting for its first consumer is still ready.
	if !kind.Ready(&unstructured.Unstructured{Object: map[string]any{"status": map[string]any{"phase": "Pending"}}}) {
		t.Error("a pending claim isn't ready")
	}
}
