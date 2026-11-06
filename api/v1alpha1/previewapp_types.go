package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type PreviewAppSpec struct {
	// Parameters are a preview's inputs, in the order the dashboard shows them.
	// +optional
	// +listType=map
	// +listMapKey=name
	Parameters []Parameter `json:"parameters,omitempty"`

	// Variables are values computed once per preview, each from the parameters, the preview and the variables before it.
	// +optional
	// +listType=map
	// +listMapKey=name
	Variables []Variable `json:"variables,omitempty"`

	// Links are shown for each preview, evaluated into its status.
	// +optional
	// +listType=map
	// +listMapKey=name
	Links []Link `json:"links,omitempty"`

	// Timeout is how long a preview may take to become ready before it's marked as stalled. Defaults to 30m.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$"
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Template is what each preview runs.
	Template Template `json:"template"`
}

// Identifier is a name that expressions refer to, such as the tag in parameters.tag.
// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
// +kubebuilder:validation:MaxLength=63
type Identifier string

// +kubebuilder:validation:Enum=string;integer;boolean
type ParameterType string

const (
	ParameterString  ParameterType = "string"
	ParameterInteger ParameterType = "integer"
	ParameterBoolean ParameterType = "boolean"
)

type Parameter struct {
	// Name is the parameter's name in expressions and in a Preview's parameters.
	Name Identifier `json:"name"`

	// Title is the parameter's label in the dashboard.
	Title string `json:"title"`

	// Description is the parameter's help text in the dashboard.
	// +optional
	Description string `json:"description,omitempty"`

	// Type is the parameter's value type.
	// +optional
	// +kubebuilder:default=string
	Type ParameterType `json:"type,omitempty"`

	// Enum limits the parameter to these values.
	// +optional
	// +listType=atomic
	Enum []string `json:"enum,omitempty"`

	// Default is used when a Preview gives no value. Optional parameters without a default are the empty value of their type.
	// +optional
	Default *string `json:"default,omitempty"`

	// Required means a Preview must give the parameter a value, unless it has a default.
	// +optional
	Required bool `json:"required,omitempty"`

	// UIHint asks the dashboard for a special input, such as a picker for an image's tags.
	// +optional
	UIHint *UIHint `json:"uiHint,omitempty"`
}

// +kubebuilder:validation:Enum=imageTag
type UIHintType string

const UIHintImageTag UIHintType = "imageTag"

// +kubebuilder:validation:XValidation:rule="self.type != 'imageTag' || has(self.imageTag)",message="imageTag is required when type is imageTag"
// +kubebuilder:validation:XValidation:rule="self.type == 'imageTag' || !has(self.imageTag)",message="imageTag may not be set unless type is imageTag"
type UIHint struct {
	// Type is the kind of input the dashboard shows.
	Type UIHintType `json:"type"`

	// ImageTag configures the imageTag hint.
	// +optional
	ImageTag *ImageTagHint `json:"imageTag,omitempty"`
}

type ImageTagHint struct {
	// Repository is the image repository whose tags are offered.
	Repository string `json:"repository"`
}

type Variable struct {
	// Name is the variable's name in expressions.
	Name Identifier `json:"name"`

	// Expression is a CEL expression.
	Expression string `json:"expression"`
}

type Link struct {
	// Name is the link's label.
	Name string `json:"name"`

	// URLExpr is a CEL expression that evaluates to the link's URL.
	URLExpr string `json:"urlExpr"`
}

type Template struct {
	// Resources are the manifests rendered for each preview. Any field can be given as a CEL expression by adding Expr to its name.
	// +listType=atomic
	Resources []runtime.RawExtension `json:"resources"`
}

type PreviewAppStatus struct {
	// ObservedGeneration is the last observed generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions holds the conditions for the PreviewApp.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=kupra
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 63",message="metadata.name must be no more than 63 characters"

// PreviewApp describes how to run an app as a preview.
type PreviewApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the PreviewApp.
	Spec PreviewAppSpec `json:"spec"`
	// Status is the observed state of the PreviewApp.
	// +optional
	Status PreviewAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewApp `json:"items"`
}

// GetConditions returns the status conditions of the object.
func (in *PreviewApp) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the status conditions on the object.
func (in *PreviewApp) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}
