package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

var defaults = kuprav1alpha1.PreviewPolicySpec{
	Apply:           &kuprav1alpha1.ApplySettings{ClusterRoleRef: meta.LocalObjectReference{Name: "kupra-preview-template"}},
	NetworkPolicies: []kuprav1alpha1.NetworkPolicyTemplate{{Metadata: kuprav1alpha1.TemplateMetadata{Name: "restrict-egress"}}},
	APIServerAccess: &kuprav1alpha1.APIServerAccess{PodSelector: &metav1.LabelSelector{}},
	ResourceQuota:   &corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourceRequestsCPU: resource.MustParse("4")}},
	LimitRange:      &corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer}}},
	NamespaceMetadata: &kuprav1alpha1.NamespaceMetadata{
		Labels: map[string]string{"pod-security.kubernetes.io/enforce": "baseline"},
	},
}

func TestEffectiveFallsBack(t *testing.T) {
	got := Effective(kuprav1alpha1.PreviewPolicySpec{}, &defaults)
	for field, filled := range map[string]bool{
		"apply":           got.Apply != nil && got.Apply.ClusterRoleRef.Name == "kupra-preview-template",
		"networkPolicies": len(got.NetworkPolicies) == 1,
		"apiServerAccess": got.APIServerAccess != nil,
		"resourceQuota":   got.ResourceQuota != nil,
		"limitRange":      got.LimitRange != nil,
	} {
		if !filled {
			t.Errorf("%s wasn't filled in from the fallback", field)
		}
	}
	if got.NamespaceMetadata.Labels["pod-security.kubernetes.io/enforce"] != "baseline" {
		t.Errorf("labels = %v, want the fallback's", got.NamespaceMetadata.Labels)
	}
}

func TestEffectiveKeepsOwnSettings(t *testing.T) {
	spec := kuprav1alpha1.PreviewPolicySpec{
		NetworkPolicies: []kuprav1alpha1.NetworkPolicyTemplate{},
		APIServerAccess: &kuprav1alpha1.APIServerAccess{},
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
	if got.APIServerAccess == nil || got.APIServerAccess.PodSelector != nil {
		t.Error("API server access isn't the policy's own, which selects no pods")
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
	for field, filled := range map[string]bool{
		"apply":             got.Apply != nil,
		"networkPolicies":   got.NetworkPolicies != nil,
		"resourceQuota":     got.ResourceQuota != nil,
		"namespaceMetadata": got.NamespaceMetadata != nil,
	} {
		if filled {
			t.Errorf("%s was filled in without a fallback", field)
		}
	}
}

func TestResolveNeedsTheDefaultPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kuprav1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps", Labels: map[string]string{Label: "custom"}}},
		&kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: "custom"}},
	).Build()

	_, _, err := Resolve(context.Background(), c, "apps")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) || notFound.Name != DefaultName {
		t.Errorf("Resolve gave %v, want the default policy not found", err)
	}
}
