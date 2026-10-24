//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/policy"
)

func createApp(t *testing.T, namespace, name, spec string) *kuprav1alpha1.PreviewApp {
	t.Helper()
	app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if err := yaml.UnmarshalStrict([]byte(spec), &app.Spec); err != nil {
		t.Fatal(err)
	}
	if err := k8s.Create(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	return app
}

func createNamespace(t *testing.T, name string, labels map[string]string) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
	if err := k8s.Create(context.Background(), ns); err != nil {
		t.Fatal(err)
	}
}

func waitForReady(t *testing.T, app *kuprav1alpha1.PreviewApp, status metav1.ConditionStatus, reason string, messages ...string) {
	t.Helper()
	eventually(t, func() error {
		var got kuprav1alpha1.PreviewApp
		if err := k8s.Get(context.Background(), client.ObjectKeyFromObject(app), &got); err != nil {
			return err
		}
		c := meta.FindStatusCondition(got.Status.Conditions, kuprav1alpha1.ConditionReady)
		if c == nil || c.ObservedGeneration != got.Generation || c.Status != status || c.Reason != reason {
			return fmt.Errorf("PreviewApp %s: Ready = %+v, want %s with reason %s", app.Name, c, status, reason)
		}
		for _, m := range messages {
			if !strings.Contains(c.Message, m) {
				return fmt.Errorf("PreviewApp %s: message %q doesn't contain %q", app.Name, c.Message, m)
			}
		}
		return nil
	})
}

func TestPreviewAppChecks(t *testing.T) {
	setUpPolicy(t, &kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: policy.DefaultName}})
	createNamespace(t, "checks", nil)

	valid := createApp(t, "checks", "valid", `
parameters:
  - name: tag
    title: Tag
  - name: replicas
    title: Replicas
    type: integer
    default: 1
variables:
  - name: host
    expression: preview.name + ".example.com"
template:
  resources:
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: server
        labelsExpr: '{"example.com/preview": preview.name}'
      spec:
        replicasExpr: parameters.replicas
        selector:
          matchLabels: {app: server}
        template:
          metadata:
            labels: {app: server}
          spec:
            containers:
              - name: server
                imageExpr: '"example.com/server:" + parameters.tag'
                argsExpr: '["--host", variables.host]'
                resources:
                  limitsExpr: '{"memory": "1Gi"}'
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
      dataExpr: '{"host": variables.host}'
`)
	waitForReady(t, valid, metav1.ConditionTrue, "Succeeded")

	invalid := createApp(t, "checks", "invalid", `
template:
  resources:
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: server
      spec:
        replicasExpr: '"two"'
        selector:
          matchLabels: {app: server}
        template:
          spec:
            containers:
              - name: server
                imagExpr: '"example.com/server"'
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRole
      metadata:
        name: escalate
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: server
`)
	waitForReady(t, invalid, metav1.ConditionFalse, "CheckFailed",
		"spec.template.resources[0].spec.replicasExpr: Invalid value: \"string\": must evaluate to an integer",
		"spec.template.resources[0].spec.template.spec.containers[0].imagExpr: Invalid value: \"imag\": there's no field imag",
		"spec.template.resources[1].kind: Forbidden: ClusterRole is cluster-scoped",
		"spec.template.resources[2].kind: Forbidden: the policy's ClusterRole kupra-preview-template-base doesn't allow get, create, patch, delete on serviceaccounts",
	)

	uncompiled := createApp(t, "checks", "uncompiled", `
variables:
  - name: host
    expression: preview.nme
template:
  resources: []
`)
	waitForReady(t, uncompiled, metav1.ConditionFalse, "Invalid", "spec.variables[host].expression")

	createNamespace(t, "custom-policy", map[string]string{policy.Label: "custom"})
	custom := createApp(t, "custom-policy", "app", `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
`)
	waitForReady(t, custom, metav1.ConditionFalse, "PolicyNotFound", "PreviewPolicy custom doesn't exist")

	setUpPolicy(t, &kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: "custom"}})
	waitForReady(t, custom, metav1.ConditionTrue, "Succeeded")
}

func TestPreviewAppRefIsImmutable(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "immutable", nil)
	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "preview", Namespace: "immutable"},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: kuprav1alpha1.LocalObjectReference{Name: "app"}},
	}
	if err := k8s.Create(ctx, preview); err != nil {
		t.Fatal(err)
	}
	preview.Spec.AppRef.Name = "other"
	if err := k8s.Update(ctx, preview); err == nil {
		t.Error("changing a Preview's appRef succeeded, want it rejected")
	}

	// Removing the whole spec, then adding it back with another app, would get around the rule.
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(kuprav1alpha1.GroupVersion.WithKind("Preview"))
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(preview), u); err != nil {
		t.Fatal(err)
	}
	unstructured.RemoveNestedField(u.Object, "spec")
	if err := k8s.Update(ctx, u); err == nil {
		t.Error("removing a Preview's spec succeeded, want it rejected")
	}
}
