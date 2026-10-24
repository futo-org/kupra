//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

func create(t *testing.T, obj client.Object) {
	t.Helper()
	if err := k8s.Create(context.Background(), obj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.IgnoreNotFound(k8s.Delete(context.Background(), obj)); err != nil {
			t.Errorf("deleting %s: %v", obj.GetName(), err)
		}
	})
}

// TestCustomTemplateRole checks that kupra only binds a custom template role once an admin allows it.
func TestCustomTemplateRole(t *testing.T) {
	ctx := context.Background()
	suffix := rand.String(5)

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-template-" + suffix},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "create", "patch", "delete"},
		}},
	}
	create(t, role)
	create(t, &kuprav1alpha1.PreviewPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + suffix},
		Spec: kuprav1alpha1.PreviewPolicySpec{
			Apply: &kuprav1alpha1.ApplySettings{ClusterRoleRef: kuprav1alpha1.LocalObjectReference{Name: role.Name}},
		},
	})
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "e2e-custom-role-" + suffix,
		Labels: map[string]string{"kupra.futo.tech/policy": "e2e-" + suffix},
	}}
	create(t, ns)

	app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns.Name}}
	if err := yaml.UnmarshalStrict([]byte(`
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
`), &app.Spec); err != nil {
		t.Fatal(err)
	}
	create(t, app)

	ready := func(status metav1.ConditionStatus, reason string) {
		t.Helper()
		eventually(t, 2*time.Minute, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
				return err
			}
			c := meta.FindStatusCondition(app.Status.Conditions, kuprav1alpha1.ConditionReady)
			if c == nil || c.ObservedGeneration != app.Generation || c.Status != status || c.Reason != reason {
				return fmt.Errorf("PreviewApp Ready = %+v, want %s with reason %s", c, status, reason)
			}
			return nil
		})
	}
	ready(metav1.ConditionFalse, "CannotBindRole")

	create(t, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "e2e-allow-" + suffix,
			Labels: map[string]string{"kupra.futo.tech/aggregate-to-operator": "true"},
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups:     []string{rbacv1.GroupName},
			Resources:     []string{"clusterroles"},
			Verbs:         []string{"bind"},
			ResourceNames: []string{role.Name},
		}},
	})
	ready(metav1.ConditionTrue, "Succeeded")

	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "preview", Namespace: ns.Name},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: kuprav1alpha1.LocalObjectReference{Name: "app"}},
	}
	create(t, preview)
	eventually(t, 2*time.Minute, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(preview), preview); err != nil {
			return err
		}
		if preview.Status.Phase != kuprav1alpha1.PreviewRunning {
			return fmt.Errorf("Preview is %s: %+v", preview.Status.Phase, preview.Status.Conditions)
		}
		return nil
	})
}
