package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreviewSnapshotSpec struct {
	// Source is the preview to capture.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="source is immutable"
	Source SnapshotSource `json:"source"`

	// Mode is how the snapshot is kept consistent: Offline stops the preview's app while capturing, and Online doesn't.
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
	// ObservedGeneration is the generation the status describes.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase summarises the state for display. Tools should use conditions instead.
	// +optional
	Phase PreviewSnapshotPhase `json:"phase,omitempty"`

	// StartTime is when the capture started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the capture succeeded or failed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// AppRef is the PreviewApp the source preview ran.
	// +optional
	AppRef *meta.LocalObjectReference `json:"appRef,omitempty"`

	// Parameters are the source preview's parameters when captured.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// Captures are the captured state resources.
	// +optional
	// +listType=atomic
	Captures []Capture `json:"captures,omitempty"`

	// ReadyToUse is set once the capture succeeded, and previews can start from the snapshot.
	// +optional
	ReadyToUse bool `json:"readyToUse,omitempty"`

	// Conditions say whether the snapshot is ready (Ready), still capturing or held from deletion (Reconciling), or failed (Stalled).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Capture is a captured state resource.
type Capture struct {
	// Group is the resource's API group, empty for the core group.
	Group string `json:"group"`
	// Kind is the resource's kind.
	Kind string `json:"kind"`
	// Name is the resource's name in the preview's template, which a restored resource is matched to.
	Name string `json:"name"`
	// Data are the objects in the snapshot's namespace that hold the capture, such as VolumeSnapshots.
	// +listType=atomic
	Data []corev1.TypedLocalObjectReference `json:"data"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=kupra
// +kubebuilder:printcolumn:name="Preview",type=string,JSONPath=`.spec.source.previewRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.readyToUse`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PreviewSnapshot is a captured copy of a preview's state.
type PreviewSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is what to capture and how.
	Spec PreviewSnapshotSpec `json:"spec"`
	// Status is how the capture went, and what it holds.
	// +optional
	Status PreviewSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewSnapshot `json:"items"`
}

// GetConditions returns the status conditions of the object.
func (in *PreviewSnapshot) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the status conditions on the object.
func (in *PreviewSnapshot) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}
