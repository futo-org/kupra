package render

import (
	"fmt"
	"math"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/cel/environment"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// env is the CEL environment for one PreviewApp: Kubernetes' CEL libraries, with each parameter, preview field and variable declared as a qualified identifier, such as parameters.tag.
type env struct {
	cel *cel.Env
}

func newEnv(parameters []kuprav1alpha1.Parameter) (*env, error) {
	base, err := environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion()).Env(environment.NewExpressions)
	if err != nil {
		return nil, err
	}
	var opts []cel.EnvOption
	for _, f := range previewFields {
		opts = append(opts, cel.Variable("preview."+f.name, f.typ))
	}
	for _, p := range parameters {
		opts = append(opts, cel.Variable("parameters."+string(p.Name), parameterType(p.Type)))
	}
	celEnv, err := base.Extend(opts...)
	if err != nil {
		return nil, err
	}
	return &env{cel: celEnv}, nil
}

// previewFields are what expressions see of a preview, as preview.<name>.
var previewFields = []struct {
	name  string
	typ   *cel.Type
	value func(Preview) any
}{
	{"name", cel.StringType, func(p Preview) any { return p.Name }},
	{"namespace", cel.StringType, func(p Preview) any { return p.Namespace }},
	{"labels", cel.MapType(cel.StringType, cel.StringType), func(p Preview) any { return nonNil(p.Labels) }},
	{"annotations", cel.MapType(cel.StringType, cel.StringType), func(p Preview) any { return nonNil(p.Annotations) }},
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func parameterType(t kuprav1alpha1.ParameterType) *cel.Type {
	switch t {
	case kuprav1alpha1.ParameterInteger:
		return cel.IntType
	case kuprav1alpha1.ParameterBoolean:
		return cel.BoolType
	default:
		return cel.StringType
	}
}

// addVariable declares a variable for the expressions compiled after it.
func (e *env) addVariable(name string, t *cel.Type) error {
	extended, err := e.cel.Extend(cel.Variable("variables."+name, t))
	if err != nil {
		return err
	}
	e.cel = extended
	return nil
}

type program struct {
	cel.Program
	outputType *cel.Type
}

func (e *env) compile(expression string, want *cel.Type) (*program, error) {
	ast, issues := e.cel.Compile(expression)
	if issues.Err() != nil {
		return nil, issues.Err()
	}
	if want != nil && !want.IsAssignableType(ast.OutputType()) && ast.OutputType() != cel.DynType {
		return nil, fmt.Errorf("must evaluate to %s, not %s", want, ast.OutputType())
	}
	prg, err := e.cel.Program(ast, cel.CostLimit(celconfig.PerCallLimit), cel.InterruptCheckFrequency(celconfig.CheckFrequency))
	if err != nil {
		return nil, err
	}
	return &program{Program: prg, outputType: ast.OutputType()}, nil
}

// native converts an expression's result into a value for an unstructured object, the way kro's conversion.GoNativeType does. Integers stay exact, and durations and timestamps become Kubernetes' usual strings.
func native(v ref.Val) (any, error) {
	switch v.Type() {
	case types.NullType:
		return nil, nil
	case types.BoolType, types.IntType, types.DoubleType, types.StringType:
		return v.Value(), nil
	case types.UintType:
		u := v.Value().(uint64)
		if u > math.MaxInt64 {
			return nil, fmt.Errorf("%d is out of range", u)
		}
		return int64(u), nil
	case types.DurationType:
		return metav1.Duration{Duration: v.Value().(time.Duration)}.ToUnstructured(), nil
	case types.TimestampType:
		return metav1.Time{Time: v.Value().(time.Time)}.ToUnstructured(), nil
	case types.ListType:
		out := []any{}
		for it := v.(traits.Lister).Iterator(); it.HasNext() == types.True; {
			item, err := native(it.Next())
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	case types.MapType:
		m := v.(traits.Mapper)
		out := map[string]any{}
		for it := m.Iterator(); it.HasNext() == types.True; {
			key := it.Next()
			k, ok := key.(types.String)
			if !ok {
				return nil, fmt.Errorf("map keys must be strings, not %s", key.Type().TypeName())
			}
			item, err := native(m.Get(key))
			if err != nil {
				return nil, err
			}
			out[string(k)] = item
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported type %s", v.Type().TypeName())
}
