// Package check finds problems with a PreviewApp's template before any preview uses it.
package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	rbacvalidation "k8s.io/component-helpers/auth/rbac/validation"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/futo-org/kupra/internal/render"
)

// ApplyVerbs are what kupra needs on each kind of resource in a template: server-side apply creates and patches, and pruning deletes.
var ApplyVerbs = []string{"get", "create", "patch", "delete"}

type Checker struct {
	Mapper  meta.RESTMapper
	Schemas resolver.SchemaResolver
}

// Template checks that the template only contains namespaced kinds the role lets kupra apply, and that each expression's result fits the field it fills in.
func (c *Checker) Template(app *render.App, roleName string, rules []rbacv1.PolicyRule) field.ErrorList {
	var errs field.ErrorList
	for _, r := range app.Resources() {
		gvk := r.GroupVersionKind
		mapping, err := c.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			if meta.IsNoMatchError(err) {
				errs = append(errs, field.Invalid(r.Path.Child("kind"), gvk.Kind, err.Error()))
				continue
			}
			errs = append(errs, field.InternalError(r.Path, err))
			continue
		}
		if mapping.Scope.Name() == meta.RESTScopeNameRoot {
			errs = append(errs, field.Forbidden(r.Path.Child("kind"), fmt.Sprintf("%s is cluster-scoped, and templates can only contain namespaced resources", gvk.Kind)))
			continue
		}
		resource := mapping.Resource
		if ok, missing := rbacvalidation.Covers(rules, []rbacv1.PolicyRule{{
			APIGroups: []string{resource.Group},
			Resources: []string{resource.Resource},
			Verbs:     ApplyVerbs,
		}}); !ok {
			var verbs []string
			for _, rule := range missing {
				verbs = append(verbs, rule.Verbs...)
			}
			errs = append(errs, field.Forbidden(r.Path.Child("kind"), fmt.Sprintf("the policy's ClusterRole %s doesn't allow %s on %s", roleName, strings.Join(verbs, ", "), resource.GroupResource())))
		}

		if len(r.Expressions) == 0 {
			continue
		}
		s, err := c.Schemas.ResolveSchema(gvk)
		if err != nil {
			errs = append(errs, field.InternalError(r.Path, fmt.Errorf("getting the schema for %s: %w", gvk, err)))
			continue
		}
		for _, e := range r.Expressions {
			fs, err := fieldSchema(s, e.Field)
			if err != nil {
				errs = append(errs, field.Invalid(e.Path, e.Field[len(e.Field)-1], err.Error()))
				continue
			}
			if !fits(fs, e.OutputType) {
				errs = append(errs, field.TypeInvalid(e.Path, e.OutputType.String(), fmt.Sprintf("must evaluate to %s", describe(fs))))
			}
		}
	}
	return errs
}

// fieldSchema finds the schema for a field. It returns nil when nothing is known about the field's type.
func fieldSchema(s *spec.Schema, path []any) (*spec.Schema, error) {
	for i, elem := range path {
		if s == nil {
			return nil, nil
		}
		switch elem := elem.(type) {
		case string:
			if prop, ok := s.Properties[elem]; ok {
				s = &prop
				continue
			}
			if s.AdditionalProperties != nil {
				s = s.AdditionalProperties.Schema
				continue
			}
			if preservesUnknownFields(s) {
				return nil, nil
			}
			if i == len(path)-1 {
				return nil, fmt.Errorf("there's no field %s", elem)
			}
			return nil, fmt.Errorf("there's no field %s at %s", elem, formatPath(path[:i+1]))
		case int:
			if s.Items == nil {
				return nil, nil
			}
			s = s.Items.Schema
		}
	}
	return s, nil
}

func preservesUnknownFields(s *spec.Schema) bool {
	v, _ := s.Extensions.GetBool("x-kubernetes-preserve-unknown-fields")
	return v
}

func intOrString(s *spec.Schema) bool {
	v, _ := s.Extensions.GetBool("x-kubernetes-int-or-string")
	return v
}

// fits reports whether a value of type t can fill in a field with schema s. apiserver's SchemaDeclType, which CRD validation rules use, ignores x-kubernetes-preserve-unknown-fields and types objects as CEL objects that a map can't be assigned to, so this works on the OpenAPI schema directly.
func fits(s *spec.Schema, t *cel.Type) bool {
	if s == nil || t == nil || t.Kind() == types.DynKind || t.Kind() == types.AnyKind {
		return true
	}
	if t.Kind() == types.NullTypeKind {
		return s.Nullable
	}
	if intOrString(s) {
		return slices.Contains([]types.Kind{types.IntKind, types.UintKind, types.StringKind}, t.Kind())
	}
	switch schemaType(s) {
	case "string":
		switch s.Format {
		case "date-time":
			return t.Kind() == types.StringKind || t.Kind() == types.TimestampKind
		case "duration":
			return t.Kind() == types.StringKind || t.Kind() == types.DurationKind
		}
		return t.Kind() == types.StringKind
	case "integer":
		return t.Kind() == types.IntKind || t.Kind() == types.UintKind
	case "number":
		return slices.Contains([]types.Kind{types.IntKind, types.UintKind, types.DoubleKind}, t.Kind())
	case "boolean":
		return t.Kind() == types.BoolKind
	case "array":
		if t.Kind() != types.ListKind {
			return false
		}
		if s.Items == nil {
			return true
		}
		return fits(s.Items.Schema, t.Parameters()[0])
	case "object":
		if t.Kind() != types.MapKind {
			return false
		}
		key, value := t.Parameters()[0], t.Parameters()[1]
		if key.Kind() != types.StringKind && key.Kind() != types.DynKind {
			return false
		}
		if s.AdditionalProperties != nil {
			return fits(s.AdditionalProperties.Schema, value)
		}
		return true
	}
	return true
}

func schemaType(s *spec.Schema) string {
	if len(s.Type) > 0 {
		return s.Type[0]
	}
	if len(s.Properties) > 0 || s.AdditionalProperties != nil {
		return "object"
	}
	return ""
}

func describe(s *spec.Schema) string {
	if intOrString(s) {
		return "an integer or a string"
	}
	switch schemaType(s) {
	case "string":
		return "a string"
	case "integer":
		return "an integer"
	case "number":
		return "a number"
	case "boolean":
		return "a boolean"
	case "array":
		return "a list"
	case "object":
		return "a map"
	}
	return "a value of the field's type"
}

func formatPath(path []any) string {
	var b strings.Builder
	for _, elem := range path {
		switch elem := elem.(type) {
		case string:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(elem)
		case int:
			fmt.Fprintf(&b, "[%d]", elem)
		}
	}
	return b.String()
}
