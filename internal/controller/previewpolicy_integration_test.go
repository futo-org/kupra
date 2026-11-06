//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	rbacv1 "k8s.io/api/rbac/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
)

func waitForPolicy(t *testing.T, name string, status metav1.ConditionStatus, reason string) {
	t.Helper()
	eventually(t, func() error {
		var p kuprav1alpha1.PreviewPolicy
		if err := k8s.Get(context.Background(), client.ObjectKey{Name: name}, &p); err != nil {
			return err
		}
		c := apimeta.FindStatusCondition(p.Status.Conditions, meta.ReadyCondition)
		switch {
		case p.Status.ObservedGeneration != p.Generation:
			return fmt.Errorf("PreviewPolicy %s isn't reconciled yet", name)
		case c == nil:
			return fmt.Errorf("PreviewPolicy %s has no Ready condition", name)
		case c.Status != status || c.Reason != reason:
			return fmt.Errorf("PreviewPolicy %s is Ready %s (%s), want %s (%s)", name, c.Status, c.Reason, status, reason)
		}
		return nil
	})
}

// A policy says whether previews can use it, so an admin sees a broken one without looking at its previews.
func TestPolicyStatus(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t, &kuprav1alpha1.PreviewPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "missing-role"},
		Spec: kuprav1alpha1.PreviewPolicySpec{
			Apply: &kuprav1alpha1.ApplySettings{ClusterRoleRef: meta.LocalObjectReference{Name: "kupra-test-missing"}},
		},
	})
	waitForPolicy(t, "default", metav1.ConditionTrue, meta.SucceededReason)
	waitForPolicy(t, "missing-role", metav1.ConditionFalse, controller.ReasonClusterRoleNotFound)

	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "kupra-test-missing"}}
	if err := k8s.Create(ctx, role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8s.Delete(context.Background(), role) })
	waitForPolicy(t, "missing-role", metav1.ConditionTrue, meta.SucceededReason)
}
