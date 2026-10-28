package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// checkParameters checks a PreviewApp's parameter definitions.
func checkParameters(params []kuprav1alpha1.Parameter, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	for _, p := range params {
		pPath := fldPath.Key(string(p.Name))
		var enum []any
		for i, raw := range p.Enum {
			v, err := parameterValue(p.Type, raw)
			if err != nil {
				errs = append(errs, field.Invalid(pPath.Child("enum").Index(i), string(raw.Raw), err.Error()))
				continue
			}
			enum = append(enum, v)
		}
		if p.Default != nil {
			v, err := parameterValue(p.Type, *p.Default)
			if err != nil {
				errs = append(errs, field.Invalid(pPath.Child("default"), string(p.Default.Raw), err.Error()))
			} else if len(p.Enum) > 0 && !slices.Contains(enum, v) {
				errs = append(errs, field.NotSupported(pPath.Child("default"), v, display(enum)))
			}
		}
	}
	return errs
}

// resolveParameters checks a Preview's parameter values against the PreviewApp's definitions, and fills in defaults.
func resolveParameters(params []kuprav1alpha1.Parameter, values map[string]apiextensionsv1.JSON, fldPath *field.Path) (map[string]any, field.ErrorList) {
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
				errs = append(errs, field.Required(pPath, "must be one of "+strings.Join(display(enum), ", ")))
				continue
			}
			out[string(p.Name)] = v
			continue
		}

		v, err := parameterValue(p.Type, raw)
		if err != nil {
			errs = append(errs, field.Invalid(pPath, string(raw.Raw), err.Error()))
			continue
		}
		if len(enum) > 0 && !slices.Contains(enum, v) {
			errs = append(errs, field.NotSupported(pPath, v, display(enum)))
			continue
		}
		out[string(p.Name)] = v
	}
	return out, errs
}

func parameterValue(t kuprav1alpha1.ParameterType, raw apiextensionsv1.JSON) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw.Raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	switch t {
	case kuprav1alpha1.ParameterInteger:
		n, ok := v.(json.Number)
		if !ok {
			return nil, fmt.Errorf("must be an integer")
		}
		i, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("must be an integer")
		}
		return i, nil
	case kuprav1alpha1.ParameterBoolean:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("must be a boolean")
		}
		return b, nil
	default:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		return s, nil
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

func display(values []any) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = fmt.Sprint(v)
	}
	return out
}
