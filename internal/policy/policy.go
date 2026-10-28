// Package policy works out the settings a PreviewPolicy gives preview namespaces.
package policy

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

const (
	// Label selects an app namespace's PreviewPolicy.
	Label = "kupra.futo.tech/policy"

	// DefaultName is the PreviewPolicy used without the label, and the one other policies fall back to.
	DefaultName = "default"
)

// NotFoundError means an app namespace names a PreviewPolicy that doesn't exist.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("PreviewPolicy %s doesn't exist", e.Name)
}

// NoTemplateRoleError means neither a namespace's PreviewPolicy nor the default one names the ClusterRole templates are applied with.
type NoTemplateRoleError struct {
	Name string
}

func (e *NoTemplateRoleError) Error() string {
	if e.Name == DefaultName {
		return fmt.Sprintf("PreviewPolicy %s doesn't set apply.clusterRoleRef", e.Name)
	}
	return fmt.Sprintf("PreviewPolicy %s doesn't set apply.clusterRoleRef, and neither does PreviewPolicy %s", e.Name, DefaultName)
}

// Resolve returns the settings for an app namespace's previews: its PreviewPolicy's, with the default PreviewPolicy's filled in for any field it leaves out.
func Resolve(ctx context.Context, c client.Reader, namespace string) (string, kuprav1alpha1.PreviewPolicySpec, error) {
	p, err := ForNamespace(ctx, c, namespace)
	if err != nil {
		return "", kuprav1alpha1.PreviewPolicySpec{}, err
	}
	var fallback *kuprav1alpha1.PreviewPolicySpec
	if p.Name != DefaultName {
		var d kuprav1alpha1.PreviewPolicy
		switch err := c.Get(ctx, client.ObjectKey{Name: DefaultName}, &d); {
		case apierrors.IsNotFound(err):
		case err != nil:
			return "", kuprav1alpha1.PreviewPolicySpec{}, err
		default:
			fallback = &d.Spec
		}
	}
	settings := Effective(p.Spec, fallback)
	if settings.Apply == nil {
		return p.Name, settings, &NoTemplateRoleError{Name: p.Name}
	}
	return p.Name, settings, nil
}

// Effective fills in the fields a policy leaves out from the fallback. Labels and annotations are merged, with the policy's own over the fallback's.
func Effective(spec kuprav1alpha1.PreviewPolicySpec, fallback *kuprav1alpha1.PreviewPolicySpec) kuprav1alpha1.PreviewPolicySpec {
	out := *spec.DeepCopy()
	if fallback == nil {
		return out
	}
	fb := fallback.DeepCopy()
	if out.Apply == nil {
		out.Apply = fb.Apply
	}
	if out.NetworkPolicies == nil {
		out.NetworkPolicies = fb.NetworkPolicies
	}
	if out.ResourceQuota == nil {
		out.ResourceQuota = fb.ResourceQuota
	}
	if out.LimitRange == nil {
		out.LimitRange = fb.LimitRange
	}
	if fb.NamespaceMetadata != nil {
		merged := fb.NamespaceMetadata
		if own := out.NamespaceMetadata; own != nil {
			merged.Labels = merge(merged.Labels, own.Labels)
			merged.Annotations = merge(merged.Annotations, own.Annotations)
		}
		out.NamespaceMetadata = merged
	}
	return out
}

func merge(base, over map[string]string) map[string]string {
	if base == nil {
		return over
	}
	maps.Copy(base, over)
	return base
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
