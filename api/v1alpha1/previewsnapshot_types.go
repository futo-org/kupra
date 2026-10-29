package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreviewSnapshotSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="source is immutable"
	Source SnapshotSource `json:"source"`

	// Mode chooses how the snapshot is kept consistent. Offline stops the preview's app while its state is captured. Online captures while it runs, so captures may not line up with each other.
	// +optional
	// +kubebuilder:default=Offline
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="mode is immutable"
	Mode SnapshotMode `json:"mode,omitempty"`
}

type SnapshotSource struct {
	// PreviewRef names the Preview to capture, in the snapshot's namespace.
	PreviewRef meta.LocalObjectReference `json:"previewRef"`
}

// +kubebuilder:validation:Enum=Offline;Online
type SnapshotMode string

const (
	SnapshotOffline SnapshotMode = "Offline"
	SnapshotOnline  SnapshotMode = "Online"
)

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

	// StartTime is when kupra started capturing.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when capturing succeeded or failed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// AppRef is the PreviewApp the source preview ran.
	// +optional
	AppRef *meta.LocalObjectReference `json:"appRef,omitempty"`

	// Parameters are the source preview's parameters when it was captured.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// Captures are the state resources captured, matched by name and kind when restoring.
	// +optional
	// +listType=atomic
	Captures []Capture `json:"captures,omitempty"`

	// +optional
	ReadyToUse bool `json:"readyToUse,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Capture is a captured state resource.
type Capture struct {
	// +optional
	Group string `json:"group"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	// Data are the objects in the snapshot's namespace that hold the captured data, such as VolumeSnapshots.
	// +listType=atomic
	Data []corev1.TypedLocalObjectReference `json:"data"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=kupra
// +kubebuilder:printcolumn:name="Preview",type=string,JSONPath=`.spec.source.previewRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.readyToUse`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PreviewSnapshot is a captured copy of a preview's state.
type PreviewSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PreviewSnapshotSpec `json:"spec"`
	// +optional
	Status PreviewSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewSnapshot `json:"items"`
}
