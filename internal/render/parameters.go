package render

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// checkParameters checks a PreviewApp's parameter definitions.
func checkParameters(params []kuprav1alpha1.Parameter, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	for _, p := range params {
		pPath := fldPath.Key(string(p.Name))
		for i, raw := range p.Enum {
			if _, err := parameterValue(p.Type, raw); err != nil {
				errs = append(errs, field.Invalid(pPath.Child("enum").Index(i), raw, err.Error()))
			}
		}
		if p.Default != nil {
			v, err := parameterValue(p.Type, *p.Default)
			if err != nil {
				errs = append(errs, field.Invalid(pPath.Child("default"), *p.Default, err.Error()))
			} else if len(p.Enum) > 0 && !slices.Contains(enumValues(p), v) {
				errs = append(errs, field.NotSupported(pPath.Child("default"), *p.Default, p.Enum))
			}
		}
	}
	return errs
}

// resolveParameters checks a Preview's parameter values against the PreviewApp's definitions, and fills in defaults.
func resolveParameters(params []kuprav1alpha1.Parameter, values map[string]string, fldPath *field.Path) (map[string]any, field.ErrorList) {
	var errs field.ErrorList
	for name := range values {
		if !slices.ContainsFunc(params, func(p kuprav1alpha1.Parameter) bool { return string(p.Name) == name }) {
			errs = append(errs, field.NotFound(fldPath.Key(name), name))
		}
	}

	out := map[string]any{}
	for _, p := range params {
		pPath := fldPath.Key(string(p.Name))
		enum := enumValues(p)
		raw, ok := values[string(p.Name)]
		switch {
		case ok:
		case p.Default != nil:
			raw = *p.Default
		case p.Required:
			errs = append(errs, field.Required(pPath, ""))
			continue
		default:
			v := zeroValue(p.Type)
			if len(enum) > 0 && !slices.Contains(enum, v) {
				errs = append(errs, field.Required(pPath, "must be one of "+strings.Join(p.Enum, ", ")))
				continue
			}
			out[string(p.Name)] = v
			continue
		}

		v, err := parameterValue(p.Type, raw)
		if err != nil {
			errs = append(errs, field.Invalid(pPath, raw, err.Error()))
			continue
		}
		if len(enum) > 0 && !slices.Contains(enum, v) {
			errs = append(errs, field.NotSupported(pPath, raw, p.Enum))
			continue
		}
		out[string(p.Name)] = v
	}
	return out, errs
}

// parameterValue parses a parameter's value, which is always given as a string, by the parameter's type.
func parameterValue(t kuprav1alpha1.ParameterType, raw string) (any, error) {
	switch t {
	case kuprav1alpha1.ParameterInteger:
		i, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("must be an integer")
		}
		return i, nil
	case kuprav1alpha1.ParameterBoolean:
		switch raw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, fmt.Errorf(`must be "true" or "false"`)
	default:
		return raw, nil
	}
}

func enumValues(p kuprav1alpha1.Parameter) []any {
	var enum []any
	for _, e := range p.Enum {
		if v, err := parameterValue(p.Type, e); err == nil {
			enum = append(enum, v)
		}
	}
	return enum
}

func zeroValue(t kuprav1alpha1.ParameterType) any {
	switch t {
	case kuprav1alpha1.ParameterInteger:
		return int64(0)
	case kuprav1alpha1.ParameterBoolean:
		return false
	default:
		return ""
	}
}
