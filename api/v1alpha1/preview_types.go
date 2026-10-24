package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreviewSpec struct {
	// AppRef names the PreviewApp in the Preview's namespace.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="appRef is immutable"
	AppRef LocalObjectReference `json:"appRef"`

	// Parameters are values for the PreviewApp's parameters.
	// +optional
	Parameters map[string]apiextensionsv1.JSON `json:"parameters,omitempty"`

	// Owners can manage the preview, along with whoever created it.
	// +optional
	// +listType=atomic
	Owners []Owner `json:"owners,omitempty"`
}

type LocalObjectReference struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// +kubebuilder:validation:Enum=User;Group
type OwnerKind string

const (
	OwnerUser  OwnerKind = "User"
	OwnerGroup OwnerKind = "Group"
)

type Owner struct {
	Kind OwnerKind `json:"kind"`

	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// +kubebuilder:validation:Enum=Pending;Provisioning;Starting;Running;Stopping;Stopped;Terminating;Failed
type PreviewPhase string

const (
	PreviewPending      PreviewPhase = "Pending"
	PreviewProvisioning PreviewPhase = "Provisioning"
	PreviewStarting     PreviewPhase = "Starting"
	PreviewRunning      PreviewPhase = "Running"
	PreviewStopping     PreviewPhase = "Stopping"
	PreviewStopped      PreviewPhase = "Stopped"
	PreviewTerminating  PreviewPhase = "Terminating"
	PreviewFailed       PreviewPhase = "Failed"
)

type PreviewStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase summarises the state for display. Tools should use conditions instead.
	// +optional
	Phase PreviewPhase `json:"phase,omitempty"`

	// Namespace is the namespace the preview's resources run in.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name
	Links []LinkStatus `json:"links,omitempty"`

	// Inventory lists the resources kupra applied for the preview.
	// +optional
	// +listType=atomic
	Inventory []ResourceRef `json:"inventory,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type LinkStatus struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=kupra
// +kubebuilder:printcolumn:name="App",type=string,JSONPath=`.spec.appRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 63",message="name must be no more than 63 characters, so it fits in a label"

// Preview is a running instance of a PreviewApp.
type Preview struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PreviewSpec `json:"spec"`
	// +optional
	Status PreviewStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Preview `json:"items"`
}
