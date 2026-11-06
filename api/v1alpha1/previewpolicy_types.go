package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PreviewPolicySpec holds the settings for preview namespaces. Fields left out are taken from the default PreviewPolicy.
type PreviewPolicySpec struct {
	// Apply is how templates are applied in preview namespaces.
	// +optional
	Apply *ApplySettings `json:"apply,omitempty"`

	// NetworkPolicies are created in each preview namespace. An empty list creates none.
	// +optional
	// +listType=atomic
	NetworkPolicies []NetworkPolicyTemplate `json:"networkPolicies"`

	// APIServerAccess lets some pods in each preview namespace reach the Kubernetes API server, which NetworkPolicies can only allow by address. kupra creates the NetworkPolicy kupra-api-server with the addresses and ports of the kubernetes Service's endpoints, and keeps them up to date.
	// +optional
	APIServerAccess *APIServerAccess `json:"apiServerAccess,omitempty"`

	// ResourceQuota is created in each preview namespace.
	// +optional
	ResourceQuota *corev1.ResourceQuotaSpec `json:"resourceQuota,omitempty"`

	// LimitRange is created in each preview namespace.
	// +optional
	LimitRange *corev1.LimitRangeSpec `json:"limitRange,omitempty"`

	// NamespaceMetadata adds labels and annotations to each preview namespace, merged over the default PreviewPolicy's.
	// +optional
	NamespaceMetadata *NamespaceMetadata `json:"namespaceMetadata,omitempty"`
}

type ApplySettings struct {
	// ClusterRoleRef names the ClusterRole templates are applied with.
	ClusterRoleRef meta.LocalObjectReference `json:"clusterRoleRef"`
}

type APIServerAccess struct {
	// PodSelector selects the pods that may reach the API server. An empty selector selects all pods, and leaving it out selects none.
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
}

type NetworkPolicyTemplate struct {
	// Metadata is the NetworkPolicy's name, labels and annotations.
	Metadata TemplateMetadata `json:"metadata"`

	// Spec is the NetworkPolicy's spec.
	Spec networkingv1.NetworkPolicySpec `json:"spec"`
}

type TemplateMetadata struct {
	// Name is the object's name in each preview namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Labels are added to the object.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are added to the object.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

type NamespaceMetadata struct {
	// Labels are added to each preview namespace.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are added to each preview namespace.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

type PreviewPolicyStatus struct {
	// ObservedGeneration is the generation the status describes.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions say whether previews can use the policy (Ready), and if not, why (Stalled).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=kupra
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 63",message="metadata.name must be no more than 63 characters"

// PreviewPolicy holds admin-controlled settings for previews.
type PreviewPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the settings for preview namespaces.
	Spec PreviewPolicySpec `json:"spec,omitempty"`
	// Status says whether previews can use the policy.
	// +optional
	Status PreviewPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewPolicy `json:"items"`
}

// GetConditions returns the status conditions of the object.
func (in *PreviewPolicy) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the status conditions on the object.
func (in *PreviewPolicy) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}
