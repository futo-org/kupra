package check

import (
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/render"
)

func mustSchema(t *testing.T, doc string) *spec.Schema {
	t.Helper()
	var s spec.Schema
	if err := yaml.Unmarshal([]byte(doc), &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

const deploymentSchema = `
type: object
properties:
  spec:
    type: object
    properties:
      replicas:
        type: integer
      template:
        type: object
        properties:
          metadata:
            type: object
            properties:
              labels:
                type: object
                additionalProperties:
                  type: string
          spec:
            type: object
            properties:
              containers:
                type: array
                items:
                  type: object
                  properties:
                    image:
                      type: string
                    args:
                      type: array
                      items:
                        type: string
                    resources:
                      type: object
                      properties:
                        limits:
                          type: object
                          additionalProperties:
                            x-kubernetes-int-or-string: true
  status:
    type: object
    x-kubernetes-preserve-unknown-fields: true
`

func TestFieldSchemaAndFits(t *testing.T) {
	s := mustSchema(t, deploymentSchema)
	for _, tc := range []struct {
		path    []any
		t       *cel.Type
		fits    bool
		wantErr string
	}{
		{path: []any{"spec", "replicas"}, t: cel.IntType, fits: true},
		{path: []any{"spec", "replicas"}, t: cel.StringType, fits: false},
		{path: []any{"spec", "replicas"}, t: cel.DynType, fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "image"}, t: cel.StringType, fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "args"}, t: cel.ListType(cel.StringType), fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "args"}, t: cel.ListType(cel.IntType), fits: false},
		{path: []any{"spec", "template", "spec", "containers", 0, "args"}, t: cel.StringType, fits: false},
		{path: []any{"spec", "template", "metadata", "labels"}, t: cel.MapType(cel.StringType, cel.StringType), fits: true},
		{path: []any{"spec", "template", "metadata", "labels"}, t: cel.MapType(cel.StringType, cel.IntType), fits: false},
		{path: []any{"spec", "template", "metadata", "labels", "app"}, t: cel.StringType, fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "resources"}, t: cel.MapType(cel.StringType, cel.DynType), fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "resources", "limits", "memory"}, t: cel.StringType, fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "resources", "limits", "memory"}, t: cel.IntType, fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "resources", "limits", "memory"}, t: cel.BoolType, fits: false},
		{path: []any{"status", "anything", "goes"}, t: cel.BoolType, fits: true},
		{path: []any{"spec", "template", "spec", "containers", 0, "imag"}, wantErr: "there's no field imag"},
		{path: []any{"spec", "templat", "spec"}, wantErr: "there's no field templat at spec.templat"},
	} {
		fs, err := fieldSchema(s, tc.path)
		if tc.wantErr != "" {
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("fieldSchema(%v) error = %v, want %q", tc.path, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("fieldSchema(%v): %v", tc.path, err)
			continue
		}
		if got := fits(fs, tc.t); got != tc.fits {
			t.Errorf("fits(%v, %s) = %v, want %v", tc.path, tc.t, got, tc.fits)
		}
	}
}

type schemas map[schema.GroupVersionKind]*spec.Schema

func (s schemas) ResolveSchema(gvk schema.GroupVersionKind) (*spec.Schema, error) {
	return s[gvk], nil
}

func TestTemplate(t *testing.T) {
	deployment := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	configMap := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	clusterRole := schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(deployment, meta.RESTScopeNamespace)
	mapper.Add(configMap, meta.RESTScopeNamespace)
	mapper.Add(clusterRole, meta.RESTScopeRoot)

	var spec kuprav1alpha1.PreviewAppSpec
	if err := yaml.UnmarshalStrict([]byte(`
template:
  resources:
    - apiVersion: apps/v1
      kind: Deployment
      metadata: {name: server}
      spec:
        replicasExpr: '"two"'
        template:
          spec:
            containers:
              - name: server
                imagExpr: '"example.com/server"'
    - apiVersion: v1
      kind: ConfigMap
      metadata: {name: config}
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRole
      metadata: {name: escalate}
    - apiVersion: example.com/v1
      kind: Widget
      metadata: {name: widget}
`), &spec); err != nil {
		t.Fatal(err)
	}
	app, errs := render.Compile(&spec)
	if len(errs) > 0 {
		t.Fatal(errs.ToAggregate())
	}

	c := &Checker{Mapper: mapper, Schemas: schemas{deployment: mustSchema(t, deploymentSchema)}}
	rules := []rbacv1.PolicyRule{{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: ApplyVerbs}}
	got := c.Template(app, "kupra-preview-template", rules).ToAggregate().Error()
	for _, want := range []string{
		`spec.template.resources[0].spec.replicasExpr: Invalid value: "string": must evaluate to an integer`,
		`spec.template.resources[0].spec.template.spec.containers[0].imagExpr: Invalid value: "imag": there's no field imag`,
		`spec.template.resources[1].kind: Forbidden: the policy's ClusterRole kupra-preview-template doesn't allow get, create, patch, delete on configmaps`,
		`spec.template.resources[2].kind: Forbidden: ClusterRole is cluster-scoped, and templates can only contain namespaced resources`,
		`spec.template.resources[3].kind: Invalid value: "Widget": no matches for kind "Widget" in version "example.com/v1"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("errors = %q\nwant them to contain %q", got, want)
		}
	}
}
