package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreviewSnapshotSpec struct{}

// +kubebuilder:validation:Enum=InProgress;Succeeded;Failed;Deleting
type PreviewSnapshotPhase string

const (
	PreviewSnapshotInProgress PreviewSnapshotPhase = "InProgress"
	PreviewSnapshotSucceeded  PreviewSnapshotPhase = "Succeeded"
	PreviewSnapshotFailed     PreviewSnapshotPhase = "Failed"
	PreviewSnapshotDeleting   PreviewSnapshotPhase = "Deleting"
)

type PreviewSnapshotStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase summarises the state for display. Tools should use conditions instead.
	// +optional
	Phase PreviewSnapshotPhase `json:"phase,omitempty"`

	// +optional
	ReadyToUse bool `json:"readyToUse,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=kupra
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.readyToUse`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PreviewSnapshot is a captured copy of a preview's state.
type PreviewSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PreviewSnapshotSpec `json:"spec,omitempty"`
	// +optional
	Status PreviewSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewSnapshot `json:"items"`
}
