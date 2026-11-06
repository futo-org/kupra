package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreviewSpec struct {
	// AppRef names the PreviewApp in the Preview's namespace.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="appRef is immutable"
	AppRef meta.LocalObjectReference `json:"appRef"`

	// Parameters are values for the PreviewApp's parameters. Like environment variables, they're strings, whatever the parameter's type.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// Owners can manage the preview, along with whoever created it. As in RoleBindings, apiGroup defaults to rbac.authorization.k8s.io for users and groups.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:XValidation:rule="self.all(s, s.kind in ['User', 'Group', 'ServiceAccount'])",message="owners must be users, groups or service accounts"
	// +kubebuilder:validation:XValidation:rule="self.all(s, s.name != '')",message="owners must have a name"
	// +kubebuilder:validation:XValidation:rule="self.all(s, s.kind != 'ServiceAccount' || (has(s.__namespace__) && s.__namespace__ != ''))",message="service account owners must have a namespace"
	// +kubebuilder:validation:XValidation:rule="self.all(s, !has(s.apiGroup) || s.apiGroup == '' || (s.kind != 'ServiceAccount' && s.apiGroup == 'rbac.authorization.k8s.io'))",message="apiGroup must be rbac.authorization.k8s.io for users and groups, and empty for service accounts"
	// +kubebuilder:validation:XValidation:rule="self.all(s, s.kind == 'ServiceAccount' || !s.name.startsWith('system:'))",message="owners may not be system users or groups"
	Owners []rbacv1.Subject `json:"owners,omitempty"`

	// RestoreFrom is the snapshot the preview's state starts from, when it starts or is reset. Without it, state starts empty.
	// +optional
	RestoreFrom *RestoreSource `json:"restoreFrom,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="has(self.snapshotRef) != has(self.snapshotSelector)",message="exactly one of snapshotRef and snapshotSelector must be set"
type RestoreSource struct {
	// SnapshotRef names a PreviewSnapshot in the preview's namespace.
	// +optional
	SnapshotRef *meta.LocalObjectReference `json:"snapshotRef,omitempty"`
	// SnapshotSelector selects the newest ready PreviewSnapshot in the preview's namespace.
	// +optional
	SnapshotSelector *metav1.LabelSelector `json:"snapshotSelector,omitempty"`
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
	// ObservedGeneration is the last observed generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase summarises the state for display. Tools should use conditions instead.
	// +optional
	Phase PreviewPhase `json:"phase,omitempty"`

	// Namespace is the namespace the preview's resources run in.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// RestoredFrom is the PreviewSnapshot the preview's state started from.
	// +optional
	RestoredFrom *meta.LocalObjectReference `json:"restoredFrom,omitempty"`

	// LastHandledResetAt is the last handled value of the kupra.futo.tech/reset-requested-at annotation.
	// +optional
	LastHandledResetAt string `json:"lastHandledResetAt,omitempty"`

	// Links are the PreviewApp's links for this preview.
	// +optional
	// +listType=map
	// +listMapKey=name
	Links []LinkStatus `json:"links,omitempty"`

	// Inventory lists the resources kupra applied for the preview.
	// +optional
	// +listType=atomic
	Inventory []InventoryEntry `json:"inventory,omitempty"`

	// Conditions holds the conditions for the Preview.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// InventoryEntry is a resource kupra applied for a preview.
type InventoryEntry struct {
	corev1.TypedLocalObjectReference `json:",inline"`
	// State is set for state, which is kept when it's no longer rendered.
	// +optional
	State bool `json:"state,omitempty"`
	// Retained is set for state that's no longer rendered, until the preview is reset.
	// +optional
	Retained bool `json:"retained,omitempty"`
}

// LinkStatus is one of a preview's links.
type LinkStatus struct {
	// Name is the link's label.
	Name string `json:"name"`
	// URL is the link's URL.
	URL string `json:"url"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=kupra
// +kubebuilder:printcolumn:name="App",type=string,JSONPath=`.spec.appRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 63",message="metadata.name must be no more than 63 characters"

// Preview is a running instance of a PreviewApp.
type Preview struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the Preview.
	Spec PreviewSpec `json:"spec"`
	// Status is the observed state of the Preview.
	// +optional
	Status PreviewStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Preview `json:"items"`
}

// GetConditions returns the status conditions of the object.
func (in *Preview) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the status conditions on the object.
func (in *Preview) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}
