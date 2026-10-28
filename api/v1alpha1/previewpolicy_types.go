package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PreviewPolicySpec holds the settings for preview namespaces. Fields left out are taken from the default PreviewPolicy.
type PreviewPolicySpec struct {
	// +optional
	Apply *ApplySettings `json:"apply,omitempty"`

	// NetworkPolicies are created in each preview namespace. An empty list creates none.
	// +optional
	// +listType=atomic
	NetworkPolicies []NetworkPolicyTemplate `json:"networkPolicies"`

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

type NetworkPolicyTemplate struct {
	Metadata TemplateMetadata `json:"metadata"`

	Spec networkingv1.NetworkPolicySpec `json:"spec"`
}

type TemplateMetadata struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

type NamespaceMetadata struct {
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

type PreviewPolicyStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=kupra
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PreviewPolicy holds admin-controlled settings for previews.
type PreviewPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PreviewPolicySpec `json:"spec,omitempty"`
	// +optional
	Status PreviewPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type PreviewPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewPolicy `json:"items"`
}
