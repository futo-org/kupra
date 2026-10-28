package render

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

func appSpec(t *testing.T, doc string) *kuprav1alpha1.PreviewAppSpec {
	t.Helper()
	var spec kuprav1alpha1.PreviewAppSpec
	if err := yaml.UnmarshalStrict([]byte(doc), &spec); err != nil {
		t.Fatalf("parsing spec: %v", err)
	}
	return &spec
}

func values(t *testing.T, doc string) map[string]string {
	t.Helper()
	var v map[string]string
	if err := yaml.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("parsing values: %v", err)
	}
	return v
}

var preview = Preview{
	Name:      "pr-1234",
	Namespace: "apps-pr-1234-7f3a",
	Labels:    map[string]string{"example.com/pull-request": "1234"},
}

func TestRender(t *testing.T) {
	spec := appSpec(t, `
parameters:
  - name: tag
    title: Tag
    required: true
  - name: replicas
    title: Replicas
    type: integer
    default: "2"
  - name: logLevel
    title: Log level
    enum: [debug, log]
    default: log
  - name: worker
    title: Worker
    type: boolean
variables:
  - name: host
    expression: preview.name + ".example.com"
  - name: url
    expression: '"https://" + variables.host'
links:
  - name: Web
    urlExpr: variables.url
  - name: Pull request
    urlExpr: '"https://github.com/example/app/pull/" + preview.labels["example.com/pull-request"]'
template:
  resources:
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: server
      spec:
        replicasExpr: parameters.replicas
        template:
          spec:
            containers:
              - name: server
                imageExpr: '"example.com/server:" + parameters.tag'
                argsExpr: '["--log-level", parameters.logLevel]'
                env:
                  - name: URL
                    valueExpr: variables.url
                  - name: NAMESPACE
                    valueExpr: preview.namespace
                  - name: LOWER
                    valueExpr: '"ABC".lowerAscii()'
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
      dataExpr: '{"settingExpr": variables.host}'
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: worker
        annotations:
          kupra.futo.tech/include: parameters.worker
`)
	app, errs := Compile(spec)
	if len(errs) > 0 {
		t.Fatalf("compiling: %v", errs.ToAggregate())
	}
	result, err := app.Render(preview, values(t, `tag: pr-1234`))
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}

	wantLinks := []kuprav1alpha1.LinkStatus{
		{Name: "Web", URL: "https://pr-1234.example.com"},
		{Name: "Pull request", URL: "https://github.com/example/app/pull/1234"},
	}
	if !reflect.DeepEqual(result.Links, wantLinks) {
		t.Errorf("links = %v, want %v", result.Links, wantLinks)
	}

	want := []map[string]any{
		{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]any{"name": "server", "namespace": preview.Namespace},
			"spec": map[string]any{
				"replicas": int64(2),
				"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{
					"name":  "server",
					"image": "example.com/server:pr-1234",
					"args":  []any{"--log-level", "log"},
					"env": []any{
						map[string]any{"name": "URL", "value": "https://pr-1234.example.com"},
						map[string]any{"name": "NAMESPACE", "value": preview.Namespace},
						map[string]any{"name": "LOWER", "value": "abc"},
					},
				}}}},
			},
		},
		{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "config", "namespace": preview.Namespace},
			"data":       map[string]any{"settingExpr": "pr-1234.example.com"},
		},
	}
	var got []map[string]any
	for _, obj := range result.Objects {
		got = append(got, obj.Object)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("objects =\n%v\nwant\n%v", got, want)
	}
}

func TestCompileErrors(t *testing.T) {
	for _, tc := range []struct {
		name, spec, want string
	}{
		{
			name: "value and expression",
			spec: `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata: {name: config}
      data: {}
      dataExpr: '{}'
`,
			want: `spec.template.resources[0].dataExpr: Invalid value: "{}": can't be given along with data`,
		},
		{
			name: "invalid expression",
			spec: `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata: {name: config}
      dataExpr: '{"a": parameters.missing}'
`,
			want: "spec.template.resources[0].dataExpr",
		},
		{
			name: "expression that isn't a string",
			spec: `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata: {name: config}
      dataExpr: 1
`,
			want: "spec.template.resources[0].dataExpr: Invalid value: 1: must be a CEL expression",
		},
		{
			name: "link that isn't a string",
			spec: `
links:
  - name: Web
    urlExpr: "1"
template:
  resources: []
`,
			want: "spec.links[Web].urlExpr: Invalid value: \"1\": must evaluate to string, not int",
		},
		{
			name: "variable using a later one",
			spec: `
variables:
  - name: a
    expression: variables.b
  - name: b
    expression: '"b"'
template:
  resources: []
`,
			want: "spec.variables[a].expression",
		},
		{
			name: "include that isn't a boolean",
			spec: `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
        annotations:
          kupra.futo.tech/include: '"yes"'
`,
			want: "must evaluate to bool, not string",
		},
		{
			name: "include given with Expr",
			spec: `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
        annotations:
          kupra.futo.tech/includeExpr: '"true"'
`,
			want: "kupra.futo.tech/include already holds an expression, so give it as a plain annotation",
		},
		{
			name: "namespace",
			spec: `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata: {name: config, namespace: other}
`,
			want: "spec.template.resources[0].metadata.namespace: Forbidden: kupra sets each resource's namespace",
		},
		{
			name: "kind as an expression",
			spec: `
template:
  resources:
    - apiVersion: v1
      kindExpr: '"ConfigMap"'
      metadata: {name: config}
`,
			want: "spec.template.resources[0].kindExpr: Forbidden: kind can't be given as an expression",
		},
		{
			name: "metadata as an expression",
			spec: `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadataExpr: '{"name": "config"}'
`,
			want: "spec.template.resources[0].metadataExpr: Forbidden: metadata can't be given as an expression",
		},
		{
			name: "missing name and kind",
			spec: `
template:
  resources:
    - apiVersion: v1
      metadata: {}
`,
			want: "spec.template.resources[0].kind: Required value, spec.template.resources[0].metadata.name: Required value",
		},
		{
			name: "default of the wrong type",
			spec: `
parameters:
  - name: replicas
    title: Replicas
    type: integer
    default: "two"
template:
  resources: []
`,
			want: `spec.parameters[replicas].default: Invalid value: "two": must be an integer`,
		},
		{
			name: "default outside the enum",
			spec: `
parameters:
  - name: level
    title: Level
    enum: [debug, log]
    default: verbose
template:
  resources: []
`,
			want: "spec.parameters[level].default: Unsupported value: \"verbose\": supported values: \"debug\", \"log\"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Compile(appSpec(t, tc.spec))
			if len(errs) == 0 {
				t.Fatalf("compiled without errors, want %q", tc.want)
			}
			if got := errs.ToAggregate().Error(); !strings.Contains(got, tc.want) {
				t.Errorf("errors = %q, want them to contain %q", got, tc.want)
			}
		})
	}
}

func TestRenderErrors(t *testing.T) {
	spec := appSpec(t, `
parameters:
  - name: tag
    title: Tag
    required: true
  - name: level
    title: Level
    enum: [debug, log]
    default: log
  - name: replicas
    title: Replicas
    type: integer
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        nameExpr: '"config"'
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        nameExpr: parameters.tag
`)
	app, errs := Compile(spec)
	if len(errs) > 0 {
		t.Fatalf("compiling: %v", errs.ToAggregate())
	}
	for _, tc := range []struct {
		name, values, want string
	}{
		{name: "missing required parameter", values: `{}`, want: "spec.parameters[tag]: Required value"},
		{name: "unknown parameter", values: `{tag: a, colour: red}`, want: "spec.parameters[colour]: Not found"},
		{name: "wrong type", values: `{tag: a, replicas: two}`, want: `spec.parameters[replicas]: Invalid value: "two": must be an integer`},
		{name: "not in the enum", values: `{tag: a, level: verbose}`, want: "spec.parameters[level]: Unsupported value: \"verbose\""},
		{name: "duplicate resources", values: `{tag: config}`, want: "more than one ConfigMap named config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := app.Render(preview, values(t, tc.values))
			if err == nil {
				t.Fatalf("rendered without errors, want %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestExamples(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "docs", "examples", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("finding examples: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var app *kuprav1alpha1.PreviewApp
			var previews []kuprav1alpha1.Preview
			for _, doc := range bytes.Split(data, []byte("\n---\n")) {
				var u unstructured.Unstructured
				if err := yaml.Unmarshal(doc, &u.Object); err != nil {
					t.Fatal(err)
				}
				switch u.GetKind() {
				case "PreviewApp":
					app = &kuprav1alpha1.PreviewApp{}
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, app); err != nil {
						t.Fatal(err)
					}
				case "Preview":
					var p kuprav1alpha1.Preview
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &p); err != nil {
						t.Fatal(err)
					}
					previews = append(previews, p)
				}
			}
			if app == nil || len(previews) == 0 {
				t.Fatal("example has no PreviewApp or Preview")
			}

			compiled, errs := Compile(&app.Spec)
			if len(errs) > 0 {
				t.Fatalf("compiling: %v", errs.ToAggregate())
			}
			for _, p := range previews {
				result, err := compiled.Render(Preview{Name: p.Name, Namespace: "previews-" + p.Name}, p.Spec.Parameters)
				if err != nil {
					t.Fatalf("rendering %s: %v", p.Name, err)
				}
				if len(result.Objects) == 0 || len(result.Links) == 0 {
					t.Errorf("rendering %s gave %d objects and %d links", p.Name, len(result.Objects), len(result.Links))
				}
			}
		})
	}
}

func TestRenderEdgeCases(t *testing.T) {
	render := func(t *testing.T, spec, vals string) (*Result, error) {
		t.Helper()
		app, errs := Compile(appSpec(t, spec))
		if len(errs) > 0 {
			t.Fatalf("compiling: %v", errs.ToAggregate())
		}
		return app.Render(preview, values(t, vals))
	}

	t.Run("required parameter with a default", func(t *testing.T) {
		result, err := render(t, `
parameters:
  - name: tag
    title: Tag
    required: true
    default: latest
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        nameExpr: parameters.tag
`, `{}`)
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Objects[0].GetName(); got != "latest" {
			t.Errorf("name = %q, want the default", got)
		}
	})

	t.Run("omitted enum parameter without a default", func(t *testing.T) {
		_, err := render(t, `
parameters:
  - name: level
    title: Level
    enum: [debug, log]
template:
  resources: []
`, `{}`)
		if err == nil || !strings.Contains(err.Error(), "spec.parameters[level]: Required value: must be one of debug, log") {
			t.Errorf("error = %v, want it to say a value is required", err)
		}
	})

	t.Run("include from an expression", func(t *testing.T) {
		_, err := render(t, `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
        annotationsExpr: '{"kupra.futo.tech/include": "false"}'
`, `{}`)
		if err == nil || !strings.Contains(err.Error(), "kupra.futo.tech/include can't be given by an expression") {
			t.Errorf("error = %v, want it to say the include can't come from an expression", err)
		}
	})

	t.Run("result types", func(t *testing.T) {
		result, err := render(t, `
template:
  resources:
    - apiVersion: example.com/v1
      kind: Widget
      metadata:
        name: widget
      spec:
        sizeExpr: '9007199254740993'
        timeoutExpr: duration("90m")
        startExpr: timestamp("2026-01-01T00:00:00Z")
        emptyExpr: '[]'
`, `{}`)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"size": int64(9007199254740993), "timeout": "1h30m0s", "start": "2026-01-01T00:00:00Z", "empty": []any{}}
		if got := result.Objects[0].Object["spec"]; !reflect.DeepEqual(got, want) {
			t.Errorf("spec = %#v, want %#v", got, want)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		_, err := render(t, `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
      binaryDataExpr: '{"key": b"value"}'
`, `{}`)
		if err == nil || !strings.Contains(err.Error(), "convert it, for example with string()") {
			t.Errorf("error = %v, want it to say bytes need converting", err)
		}
	})

	t.Run("dotted keys", func(t *testing.T) {
		result, err := render(t, `
template:
  resources:
    - apiVersion: example.com/v1
      kind: Release
      metadata:
        name: app
      values:
        a:
          bExpr: '"nested"'
        a.bExpr: '"flat"'
`, `{}`)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"a": map[string]any{"b": "nested"}, "a.b": "flat"}
		if got := result.Objects[0].Object["values"]; !reflect.DeepEqual(got, want) {
			t.Errorf("values = %v, want %v", got, want)
		}
	})

	t.Run("dynamic include that isn't a boolean", func(t *testing.T) {
		_, err := render(t, `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
        annotations:
          kupra.futo.tech/include: dyn("false")
`, `{}`)
		if err == nil || !strings.Contains(err.Error(), "must evaluate to bool, not string") {
			t.Errorf("error = %v, want it to say the include must be a boolean", err)
		}
	})

	t.Run("dynamic link that isn't a string", func(t *testing.T) {
		_, err := render(t, `
links:
  - name: Web
    urlExpr: dyn(1)
template:
  resources: []
`, `{}`)
		if err == nil || !strings.Contains(err.Error(), "spec.links[Web]: must evaluate to string, not int") {
			t.Errorf("error = %v, want it to say the link must be a string", err)
		}
	})
}
