package controller

import (
	"context"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// Losing permission to bind a ready policy's role changes nothing kupra watches, so the policy is checked again.
func TestPolicyLosesBindPermission(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := kuprav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	p := &kuprav1alpha1.PreviewPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec:       kuprav1alpha1.PreviewPolicySpec{Apply: &kuprav1alpha1.ApplySettings{ClusterRoleRef: meta.LocalObjectReference{Name: "template"}}},
	}
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "template"}}
	mayBind := true
	c := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).WithObjects(p, role).WithStatusSubresource(p).Build(), interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if review, ok := obj.(*authorizationv1.SelfSubjectAccessReview); ok {
				review.Status.Allowed = mayBind
				return nil
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	r := &PreviewPolicyReconciler{Client: c}
	reconcile := func() (ctrl.Result, *metav1.Condition) {
		t.Helper()
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(p)})
		if err != nil {
			t.Fatal(err)
		}
		var got kuprav1alpha1.PreviewPolicy
		if err := c.Get(ctx, client.ObjectKeyFromObject(p), &got); err != nil {
			t.Fatal(err)
		}
		return result, conditions.Get(&got, meta.ReadyCondition)
	}

	result, ready := reconcile()
	if ready.Status != metav1.ConditionTrue {
		t.Fatalf("Ready is %s (%s), want True", ready.Status, ready.Reason)
	}
	if result.RequeueAfter == 0 {
		t.Error("a ready policy isn't checked again")
	}

	mayBind = false
	if _, ready := reconcile(); ready.Status != metav1.ConditionFalse || ready.Reason != ReasonCannotBindRole {
		t.Errorf("Ready is %s (%s), want False (%s)", ready.Status, ready.Reason, ReasonCannotBindRole)
	}
}
