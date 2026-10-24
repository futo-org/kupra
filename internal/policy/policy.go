// Package policy works out the settings a PreviewPolicy gives preview namespaces.
package policy

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

const (
	// Label selects an app namespace's PreviewPolicy.
	Label = "kupra.futo.tech/policy"

	// DefaultName is the PreviewPolicy used without the label.
	DefaultName = "default"

	DefaultClusterRole = "kupra-preview-template"
)

// Effective returns the policy's settings with kupra's built-in defaults filled in.
func Effective(spec kuprav1alpha1.PreviewPolicySpec) kuprav1alpha1.PreviewPolicySpec {
	out := *spec.DeepCopy()
	if out.Apply == nil {
		out.Apply = &kuprav1alpha1.ApplySettings{ClusterRoleRef: kuprav1alpha1.LocalObjectReference{Name: DefaultClusterRole}}
	}
	if out.NetworkPolicies == nil {
		out.NetworkPolicies = defaultNetworkPolicies()
	}
	if out.ResourceQuota == nil {
		out.ResourceQuota = &corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
			corev1.ResourceRequestsCPU:    resource.MustParse("4"),
			corev1.ResourceRequestsMemory: resource.MustParse("16Gi"),
		}}
	}
	if out.LimitRange == nil {
		out.LimitRange = &corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			DefaultRequest: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Default: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}}}
	}

	metadata := kuprav1alpha1.NamespaceMetadata{
		Labels: map[string]string{"pod-security.kubernetes.io/enforce": "baseline"},
	}
	if out.NamespaceMetadata != nil {
		maps.Copy(metadata.Labels, out.NamespaceMetadata.Labels)
		metadata.Annotations = out.NamespaceMetadata.Annotations
	}
	out.NamespaceMetadata = &metadata
	return out
}

// defaultNetworkPolicies let previews reach their own namespace, DNS and the public internet, but no private, link-local or other internal addresses.
func defaultNetworkPolicies() []kuprav1alpha1.NetworkPolicyTemplate {
	return []kuprav1alpha1.NetworkPolicyTemplate{{
		Metadata: kuprav1alpha1.TemplateMetadata{Name: "restrict-egress"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{}}}},
				{To: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"}},
					PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "kube-dns"}},
				}}},
				{To: []networkingv1.NetworkPolicyPeer{
					{IPBlock: &networkingv1.IPBlock{
						CIDR:   "0.0.0.0/0",
						Except: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "169.254.0.0/16"},
					}},
					{IPBlock: &networkingv1.IPBlock{
						CIDR:   "::/0",
						Except: []string{"fc00::/7", "fe80::/10"},
					}},
				}},
			},
		},
	}}
}

// NotFoundError means an app namespace names a PreviewPolicy that doesn't exist.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("PreviewPolicy %s doesn't exist", e.Name)
}

// ForNamespace returns the PreviewPolicy that applies to an app namespace.
func ForNamespace(ctx context.Context, c client.Reader, namespace string) (*kuprav1alpha1.PreviewPolicy, error) {
	var ns corev1.Namespace
	if err := c.Get(ctx, client.ObjectKey{Name: namespace}, &ns); err != nil {
		return nil, err
	}
	name := ns.Labels[Label]
	if name == "" {
		name = DefaultName
	}
	var p kuprav1alpha1.PreviewPolicy
	if err := c.Get(ctx, client.ObjectKey{Name: name}, &p); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &NotFoundError{Name: name}
		}
		return nil, err
	}
	return &p, nil
}
