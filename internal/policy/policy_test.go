package policy

import (
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

var defaults = kuprav1alpha1.PreviewPolicySpec{
	Apply:           &kuprav1alpha1.ApplySettings{ClusterRoleRef: meta.LocalObjectReference{Name: "kupra-preview-template"}},
	NetworkPolicies: []kuprav1alpha1.NetworkPolicyTemplate{{Metadata: kuprav1alpha1.TemplateMetadata{Name: "restrict-egress"}}},
	ResourceQuota:   &corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourceRequestsCPU: resource.MustParse("4")}},
	LimitRange:      &corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer}}},
	NamespaceMetadata: &kuprav1alpha1.NamespaceMetadata{
		Labels: map[string]string{"pod-security.kubernetes.io/enforce": "baseline"},
	},
}

func TestEffectiveFallsBack(t *testing.T) {
	got := Effective(kuprav1alpha1.PreviewPolicySpec{}, &defaults)
	if got.Apply.ClusterRoleRef.Name != "kupra-preview-template" || len(got.NetworkPolicies) != 1 || got.ResourceQuota == nil || got.LimitRange == nil {
		t.Errorf("fields left out weren't filled in from the fallback: %+v", got)
	}
	if got.NamespaceMetadata.Labels["pod-security.kubernetes.io/enforce"] != "baseline" {
		t.Errorf("labels = %v, want the fallback's", got.NamespaceMetadata.Labels)
	}
}

func TestEffectiveKeepsOwnSettings(t *testing.T) {
	spec := kuprav1alpha1.PreviewPolicySpec{
		NetworkPolicies: []kuprav1alpha1.NetworkPolicyTemplate{},
		ResourceQuota:   &corev1.ResourceQuotaSpec{},
		NamespaceMetadata: &kuprav1alpha1.NamespaceMetadata{
			Labels: map[string]string{
				"example.com/gateway-access":         "previews",
				"pod-security.kubernetes.io/enforce": "restricted",
			},
		},
	}
	got := Effective(spec, &defaults)
	if got.NetworkPolicies == nil || len(got.NetworkPolicies) != 0 {
		t.Errorf("network policies = %v, want the policy's explicitly empty list", got.NetworkPolicies)
	}
	if len(got.ResourceQuota.Hard) != 0 {
		t.Errorf("resource quota = %v, want the policy's empty one", got.ResourceQuota)
	}
	want := map[string]string{
		"pod-security.kubernetes.io/enforce": "restricted",
		"example.com/gateway-access":         "previews",
	}
	if len(got.NamespaceMetadata.Labels) != len(want) {
		t.Errorf("labels = %v, want %v", got.NamespaceMetadata.Labels, want)
	}
	for k, v := range want {
		if got.NamespaceMetadata.Labels[k] != v {
			t.Errorf("label %s = %q, want %q", k, got.NamespaceMetadata.Labels[k], v)
		}
	}
	if defaults.NamespaceMetadata.Labels["pod-security.kubernetes.io/enforce"] != "baseline" || spec.NamespaceMetadata.Labels["pod-security.kubernetes.io/enforce"] != "restricted" {
		t.Error("Effective modified its inputs")
	}
}

func TestEffectiveWithoutFallback(t *testing.T) {
	got := Effective(kuprav1alpha1.PreviewPolicySpec{}, nil)
	if got.Apply != nil || got.NetworkPolicies != nil || got.ResourceQuota != nil || got.NamespaceMetadata != nil {
		t.Errorf("got %+v, want nothing filled in", got)
	}
}
