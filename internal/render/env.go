package render

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/util/json"
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
	stringMap := cel.MapType(cel.StringType, cel.StringType)
	opts := []cel.EnvOption{
		cel.Variable("preview.name", cel.StringType),
		cel.Variable("preview.namespace", cel.StringType),
		cel.Variable("preview.labels", stringMap),
		cel.Variable("preview.annotations", stringMap),
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

// native converts an expression's result into a value for an unstructured object. It goes through CEL's JSON mapping, as MutatingAdmissionPolicy does for JSON patch values, and decodes whole numbers as integers the way Kubernetes does.
func native(v ref.Val) (any, error) {
	pb, err := v.ConvertToNative(reflect.TypeOf(&structpb.Value{}))
	if err != nil {
		return nil, err
	}
	data, err := protojson.Marshal(pb.(*structpb.Value))
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
