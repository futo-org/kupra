package policy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

func TestEffectiveDefaults(t *testing.T) {
	got := Effective(kuprav1alpha1.PreviewPolicySpec{})
	if got.Apply.ClusterRoleRef.Name != DefaultClusterRole {
		t.Errorf("cluster role = %q, want %q", got.Apply.ClusterRoleRef.Name, DefaultClusterRole)
	}
	if len(got.NetworkPolicies) != 1 || got.ResourceQuota == nil || got.LimitRange == nil {
		t.Errorf("missing defaults: %+v", got)
	}
	if got.NamespaceMetadata.Labels["pod-security.kubernetes.io/enforce"] != "baseline" {
		t.Errorf("labels = %v, want Pod Security baseline", got.NamespaceMetadata.Labels)
	}
}

func TestEffectiveOverrides(t *testing.T) {
	spec := kuprav1alpha1.PreviewPolicySpec{
		NetworkPolicies: []kuprav1alpha1.NetworkPolicyTemplate{},
		ResourceQuota:   &corev1.ResourceQuotaSpec{},
		NamespaceMetadata: &kuprav1alpha1.NamespaceMetadata{
			Labels: map[string]string{"example.com/gateway-access": "previews"},
		},
	}
	got := Effective(spec)
	if got.NetworkPolicies == nil || len(got.NetworkPolicies) != 0 {
		t.Errorf("network policies = %v, want an explicitly empty list kept", got.NetworkPolicies)
	}
	if len(got.ResourceQuota.Hard) != 0 {
		t.Errorf("resource quota = %v, want the policy's empty one", got.ResourceQuota)
	}
	want := map[string]string{
		"pod-security.kubernetes.io/enforce": "baseline",
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
	if spec.NamespaceMetadata.Labels["pod-security.kubernetes.io/enforce"] != "" {
		t.Error("Effective modified its input")
	}
}
