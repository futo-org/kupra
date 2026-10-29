//go:build integration

package controller_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
	"github.com/futo-org/kupra/internal/names"
)

// staleReconciler is a PreviewReconciler whose reads of a namespace return what's given, the way a cache that hasn't caught up would.
func staleReconciler(t *testing.T, name string, stale *corev1.Namespace) *controller.PreviewReconciler {
	t.Helper()
	c, err := client.NewWithWatch(restConfig, client.Options{Scheme: k8s.Scheme()})
	if err != nil {
		t.Fatal(err)
	}
	return &controller.PreviewReconciler{
		Client: interceptor.NewClient(c, interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				ns, ok := obj.(*corev1.Namespace)
				if !ok || key.Name != name {
					return c.Get(ctx, key, obj, opts...)
				}
				if stale == nil {
					return apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name)
				}
				stale.DeepCopyInto(ns)
				return nil
			},
		}),
		APIReader: c,
		Namespace: kupraNamespace,
	}
}

func stalePreview(name string, uid types.UID) *kuprav1alpha1.Preview {
	return &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Namespace: "stale", Name: name, UID: uid},
	}
}

func createOwnedNamespace(t *testing.T, name string, uid types.UID) *corev1.Namespace {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{controller.AnnotationPreviewUID: string(uid)}}}
	if err := k8s.Create(context.Background(), ns); err != nil {
		t.Fatal(err)
	}
	return ns
}

func previewUID(t *testing.T, name string) string {
	t.Helper()
	var ns corev1.Namespace
	if err := k8s.Get(context.Background(), client.ObjectKey{Name: name}, &ns); err != nil {
		t.Fatal(err)
	}
	return ns.Annotations[controller.AnnotationPreviewUID]
}

func TestNamespaceCreatedSinceRead(t *testing.T) {
	name := "stale-created"
	createOwnedNamespace(t, name, "other")

	r := staleReconciler(t, name, nil)
	reason, _, err := r.EnsureNamespace(context.Background(), stalePreview("created", "mine"), name, kuprav1alpha1.PreviewPolicySpec{})
	if err != nil {
		t.Fatal(err)
	}
	if reason != controller.ReasonNamespaceConflict {
		t.Errorf("reason %q, want %s", reason, controller.ReasonNamespaceConflict)
	}
	if uid := previewUID(t, name); uid != "other" {
		t.Errorf("namespace belongs to %q, want it left to other", uid)
	}
}

func TestNamespaceReplacedSinceRead(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		run  func(*controller.PreviewReconciler, *kuprav1alpha1.Preview, string) error
	}{
		{"ensure", func(r *controller.PreviewReconciler, p *kuprav1alpha1.Preview, name string) error {
			_, _, err := r.EnsureNamespace(ctx, p, name, kuprav1alpha1.PreviewPolicySpec{})
			return err
		}},
		{"delete", func(r *controller.PreviewReconciler, p *kuprav1alpha1.Preview, _ string) error {
			_, err := r.ReconcileDelete(ctx, p)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := stalePreview("replaced-"+tc.name, "mine")
			name := names.PreviewNamespace(p.Namespace, p.Name)
			// A namespace the preview once had, which has since been replaced by another one of the same name.
			stale := createOwnedNamespace(t, name, "mine")
			replaceNamespace(t, name, "other")

			r := staleReconciler(t, name, stale)
			if err := tc.run(r, p, name); err == nil {
				t.Error("want an error from the stale read")
			}
			var ns corev1.Namespace
			if err := k8s.Get(ctx, client.ObjectKey{Name: name}, &ns); err != nil {
				t.Fatal(err)
			}
			if !ns.DeletionTimestamp.IsZero() {
				t.Error("the replacement namespace is being deleted")
			}
			if uid := ns.Annotations[controller.AnnotationPreviewUID]; uid != "other" {
				t.Errorf("namespace belongs to %q, want it left to other", uid)
			}
		})
	}
}

// replaceNamespace deletes a namespace, and creates another of the same name, owned by another preview.
func replaceNamespace(t *testing.T, name string, uid types.UID) {
	t.Helper()
	ctx := context.Background()
	if err := k8s.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() error {
		// envtest has no namespace controller, so finish the deletion by hand.
		var ns corev1.Namespace
		if err := k8s.Get(ctx, client.ObjectKey{Name: name}, &ns); apierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
		ns.Spec.Finalizers = nil
		return k8s.SubResource("finalize").Update(ctx, &ns)
	})
	createOwnedNamespace(t, name, uid)
}

func TestAccessToReplacedNamespace(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "stale", nil)
	p := stalePreview("replaced-access", "mine")
	p.Annotations = map[string]string{controller.AnnotationCreatedBy: "alice"}
	name := names.PreviewNamespace(p.Namespace, p.Name)
	stale := createOwnedNamespace(t, name, "mine")
	replaceNamespace(t, name, "other")

	r := staleReconciler(t, name, stale)
	exists, err := r.EnsureAccess(ctx, p, name)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("the replacement namespace is taken for the preview's")
	}
	var binding rbacv1.RoleBinding
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: name, Name: "kupra-access"}, &binding); !apierrors.IsNotFound(err) {
		t.Errorf("kupra-access in the replacement namespace: %v, %+v; want none", err, binding.Subjects)
	}
}
