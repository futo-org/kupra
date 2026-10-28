// Package render turns a PreviewApp's template into the resources of one preview.
package render

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/validation/field"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// IncludeAnnotation holds a CEL expression that decides whether a template resource is rendered.
const IncludeAnnotation = "kupra.futo.tech/include"

const exprSuffix = "Expr"

// App is a PreviewApp with its expressions compiled.
type App struct {
	parameters []kuprav1alpha1.Parameter
	variables  []namedProgram
	links      []namedProgram
	resources  []resource
}

type namedProgram struct {
	name string
	*program
}

type resource struct {
	object      map[string]any
	path        *field.Path
	gvk         schema.GroupVersionKind
	include     *program
	exprs       map[string]*program
	expressions []Expression
}

// Resource is a template resource, as far as checks need it.
type Resource struct {
	Path             *field.Path
	GroupVersionKind schema.GroupVersionKind
	Expressions      []Expression
}

// Expression is a field given as an expression.
type Expression struct {
	// Path is the expression's own field in the PreviewApp.
	Path *field.Path
	// Field is the path to the field it fills in, within the resource: map keys and list indices.
	Field      []any
	OutputType *cel.Type
}

// Resources lists the template's resources.
func (a *App) Resources() []Resource {
	out := make([]Resource, len(a.resources))
	for i, r := range a.resources {
		out[i] = Resource{Path: r.path, GroupVersionKind: r.gvk, Expressions: r.expressions}
	}
	return out
}

// Preview is what expressions see of a preview.
type Preview struct {
	Name string
	// Namespace is the namespace the preview's resources run in.
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
}

// Result is a rendered preview.
type Result struct {
	Objects []*unstructured.Unstructured
	Links   []kuprav1alpha1.LinkStatus
}

// Compile compiles and type-checks a PreviewApp's expressions, and checks its parameters and template.
func Compile(spec *kuprav1alpha1.PreviewAppSpec) (*App, field.ErrorList) {
	specPath := field.NewPath("spec")
	errs := checkParameters(spec.Parameters, specPath.Child("parameters"))

	e, err := newEnv(spec.Parameters)
	if err != nil {
		return nil, append(errs, field.InternalError(specPath, err))
	}
	app := &App{parameters: spec.Parameters}

	for _, v := range spec.Variables {
		vPath := specPath.Child("variables").Key(string(v.Name))
		prg, err := e.compile(v.Expression, nil)
		outputType := cel.DynType
		if err != nil {
			errs = append(errs, field.Invalid(vPath.Child("expression"), v.Expression, err.Error()))
		} else {
			outputType = prg.outputType
			app.variables = append(app.variables, namedProgram{string(v.Name), prg})
		}
		if err := e.addVariable(string(v.Name), outputType); err != nil {
			errs = append(errs, field.Invalid(vPath.Child("name"), v.Name, err.Error()))
		}
	}

	for _, l := range spec.Links {
		prg, err := e.compile(l.URLExpr, cel.StringType)
		if err != nil {
			errs = append(errs, field.Invalid(specPath.Child("links").Key(l.Name).Child("urlExpr"), l.URLExpr, err.Error()))
			continue
		}
		app.links = append(app.links, namedProgram{l.Name, prg})
	}

	for i, raw := range spec.Template.Resources {
		r, rErrs := compileResource(e, raw, specPath.Child("template", "resources").Index(i))
		errs = append(errs, rErrs...)
		if r != nil {
			app.resources = append(app.resources, *r)
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return app, nil
}

func compileResource(e *env, raw runtime.RawExtension, fldPath *field.Path) (*resource, field.ErrorList) {
	var obj map[string]any
	if err := json.Unmarshal(raw.Raw, &obj); err != nil {
		return nil, field.ErrorList{field.Invalid(fldPath, string(raw.Raw), err.Error())}
	}

	var errs field.ErrorList
	for _, key := range []string{"apiVersion", "kind"} {
		if s, _ := obj[key].(string); s == "" {
			errs = append(errs, field.Required(fldPath.Child(key), ""))
		}
	}
	metadata, ok := obj["metadata"].(map[string]any)
	if !ok {
		return nil, append(errs, field.Required(fldPath.Child("metadata"), ""))
	}
	metaPath := fldPath.Child("metadata")
	if _, ok := metadata["name"]; !ok {
		if _, ok := metadata["name"+exprSuffix]; !ok {
			errs = append(errs, field.Required(metaPath.Child("name"), ""))
		}
	}
	for _, key := range []string{"namespace", "namespace" + exprSuffix} {
		if _, ok := metadata[key]; ok {
			errs = append(errs, field.Forbidden(metaPath.Child(key), "kupra sets each resource's namespace"))
		}
	}

	apiVersion, _ := obj["apiVersion"].(string)
	kind, _ := obj["kind"].(string)
	r := &resource{
		object: obj,
		path:   fldPath,
		gvk:    schema.FromAPIVersionAndKind(apiVersion, kind),
		exprs:  map[string]*program{},
	}
	if annotations, ok := metadata["annotations"].(map[string]any); ok {
		if include, ok := annotations[IncludeAnnotation].(string); ok {
			prg, err := e.compile(include, cel.BoolType)
			if err != nil {
				errs = append(errs, field.Invalid(metaPath.Child("annotations").Key(IncludeAnnotation), include, err.Error()))
			}
			r.include = prg
		}
	}
	errs = append(errs, compileFields(e, obj, fldPath, nil, r)...)
	return r, errs
}

func compileFields(e *env, value any, fldPath *field.Path, target []any, r *resource) field.ErrorList {
	var errs field.ErrorList
	switch value := value.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			keyPath := fldPath.Child(key)
			name, isExpr := strings.CutSuffix(key, exprSuffix)
			if !isExpr || name == "" {
				errs = append(errs, compileFields(e, value[key], keyPath, append(slices.Clone(target), key), r)...)
				continue
			}
			expression, ok := value[key].(string)
			if !ok {
				errs = append(errs, field.TypeInvalid(keyPath, value[key], "must be a CEL expression"))
				continue
			}
			if _, ok := value[name]; ok {
				errs = append(errs, field.Invalid(keyPath, expression, fmt.Sprintf("can't be given along with %s", name)))
				continue
			}
			prg, err := e.compile(expression, nil)
			if err != nil {
				errs = append(errs, field.Invalid(keyPath, expression, err.Error()))
				continue
			}
			r.exprs[pathKey(append(slices.Clone(target), key))] = prg
			r.expressions = append(r.expressions, Expression{Path: keyPath, Field: append(slices.Clone(target), name), OutputType: prg.outputType})
		}
	case []any:
		for i, item := range value {
			errs = append(errs, compileFields(e, item, fldPath.Index(i), append(slices.Clone(target), i), r)...)
		}
	}
	return errs
}

// Render renders a preview's resources and links from the Preview's parameter values.
func (a *App) Render(preview Preview, values map[string]apiextensionsv1.JSON) (*Result, error) {
	params, errs := resolveParameters(a.parameters, values, field.NewPath("spec", "parameters"))
	if len(errs) > 0 {
		return nil, errs.ToAggregate()
	}

	activation := map[string]any{
		"preview.name":        preview.Name,
		"preview.namespace":   preview.Namespace,
		"preview.labels":      nonNil(preview.Labels),
		"preview.annotations": nonNil(preview.Annotations),
	}
	for name, v := range params {
		activation["parameters."+name] = v
	}
	for _, v := range a.variables {
		out, _, err := v.Eval(activation)
		if err != nil {
			return nil, fmt.Errorf("spec.variables[%s]: %w", v.name, err)
		}
		activation["variables."+v.name] = out
	}

	result := &Result{}
	for _, l := range a.links {
		out, _, err := l.Eval(activation)
		if err != nil {
			return nil, fmt.Errorf("spec.links[%s]: %w", l.name, err)
		}
		url, ok := out.(types.String)
		if !ok {
			return nil, fmt.Errorf("spec.links[%s]: must evaluate to string, not %s", l.name, out.Type().TypeName())
		}
		result.Links = append(result.Links, kuprav1alpha1.LinkStatus{Name: l.name, URL: string(url)})
	}

	seen := map[string]bool{}
	for _, r := range a.resources {
		if r.include != nil {
			includePath := r.path.Child("metadata", "annotations").Key(IncludeAnnotation)
			out, _, err := r.include.Eval(activation)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", includePath, err)
			}
			include, ok := out.(types.Bool)
			if !ok {
				return nil, fmt.Errorf("%s: must evaluate to bool, not %s", includePath, out.Type().TypeName())
			}
			if !include {
				continue
			}
		}

		obj := runtime.DeepCopyJSONValue(r.object).(map[string]any)
		if err := evalFields(obj, r.path, nil, r.exprs, activation); err != nil {
			return nil, err
		}
		u := &unstructured.Unstructured{Object: obj}
		if u.GetName() == "" {
			return nil, fmt.Errorf("%s: name is required", r.path.Child("metadata", "name"))
		}
		u.SetNamespace(preview.Namespace)

		key := schema.FromAPIVersionAndKind(u.GetAPIVersion(), u.GetKind()).GroupKind().String() + "/" + u.GetName()
		if seen[key] {
			return nil, fmt.Errorf("%s: more than one %s named %s", r.path, u.GetKind(), u.GetName())
		}
		seen[key] = true
		result.Objects = append(result.Objects, u)
	}
	return result, nil
}

func evalFields(value any, fldPath *field.Path, target []any, exprs map[string]*program, activation map[string]any) error {
	switch value := value.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			keyPath := fldPath.Child(key)
			keyTarget := append(slices.Clone(target), key)
			prg, ok := exprs[pathKey(keyTarget)]
			if !ok {
				if err := evalFields(value[key], keyPath, keyTarget, exprs, activation); err != nil {
					return err
				}
				continue
			}
			out, _, err := prg.Eval(activation)
			if err != nil {
				return fmt.Errorf("%s: %w", keyPath, err)
			}
			v, err := native(out)
			if err != nil {
				return fmt.Errorf("%s: %w", keyPath, err)
			}
			delete(value, key)
			value[strings.TrimSuffix(key, exprSuffix)] = v
		}
	case []any:
		for i, item := range value {
			if err := evalFields(item, fldPath.Index(i), append(slices.Clone(target), i), exprs, activation); err != nil {
				return err
			}
		}
	}
	return nil
}

// pathKey identifies a field by its keys and indices. Unlike a field.Path's string, it keeps a key containing a dot apart from nested keys.
func pathKey(path []any) string {
	b, _ := json.Marshal(path)
	return string(b)
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
