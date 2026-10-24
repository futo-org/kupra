package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreviewSpec struct{}

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

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=kupra
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Preview is a running instance of a PreviewApp.
type Preview struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PreviewSpec `json:"spec,omitempty"`
	// +optional
	Status PreviewStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Preview `json:"items"`
}
