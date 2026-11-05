//go:build integration

package controller_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
		c := apimeta.FindStatusCondition(got.Status.Conditions, meta.ReadyCondition)
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
	setUpPolicies(t)
	createNamespace(t, "checks", nil)

	valid := createApp(t, "checks", "valid", `
parameters:
  - name: tag
    title: Tag
  - name: replicas
    title: Replicas
    type: integer
    default: "1"
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
    - apiVersion: v1
      kind: Service
      metadata:
        name: server
      spec:
        ports:
          - port: 80
            targetPortExpr: 'true'
`)
	waitForReady(t, invalid, metav1.ConditionFalse, "CheckFailed",
		"spec.template.resources[0].spec.replicasExpr: Invalid value: \"string\": must evaluate to an integer",
		"spec.template.resources[0].spec.template.spec.containers[0].imagExpr: Invalid value: \"imag\": unknown field \"imag\"",
		"spec.template.resources[1].kind: Forbidden: ClusterRole is cluster-scoped",
		"spec.template.resources[2].kind: Forbidden: the policy's ClusterRole kupra-preview-template-base doesn't allow get, create, patch, delete on serviceaccounts",
		`spec.template.resources[3].spec.ports[0].targetPortExpr: Invalid value: "bool": must evaluate to an integer or a string`,
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
	waitForReady(t, custom, metav1.ConditionFalse, "PolicyNotFound", "PreviewPolicy custom not found")

	setUpPolicies(t, &kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: "custom"}})
	waitForReady(t, custom, metav1.ConditionTrue, "Succeeded")
}

func TestPreviewAppRefIsImmutable(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "immutable", nil)
	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "preview", Namespace: "immutable"},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
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

func TestUIHintNeedsItsSettings(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "ui-hints", nil)
	for _, tc := range []struct {
		hint string
		ok   bool
	}{
		{hint: `{type: imageTag, imageTag: {repository: example.com/app}}`, ok: true},
		{hint: `{type: imageTag}`, ok: false},
	} {
		app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{GenerateName: "app-", Namespace: "ui-hints"}}
		if err := yaml.UnmarshalStrict([]byte(`
parameters:
  - name: tag
    title: Tag
    uiHint: `+tc.hint+`
template:
  resources: []
`), &app.Spec); err != nil {
			t.Fatal(err)
		}
		err := k8s.Create(ctx, app, client.DryRunAll)
		if tc.ok && err != nil {
			t.Errorf("uiHint %s: %v", tc.hint, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("uiHint %s was accepted, want it rejected", tc.hint)
		}
	}
}

// TestExamplesAreAccepted checks the example PreviewApps against the CRDs. Fields for later features, such as template.secrets, are pruned rather than rejected.
func TestExamplesAreAccepted(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "examples", nil)
	files, err := filepath.Glob(filepath.Join("..", "..", "docs", "examples", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("finding examples: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, doc := range bytes.Split(data, []byte("\n---\n")) {
			var u unstructured.Unstructured
			if err := yaml.Unmarshal(doc, &u.Object); err != nil {
				t.Fatal(err)
			}
			if u.GetKind() != "PreviewApp" {
				continue
			}
			u.SetNamespace("examples")
			if err := k8s.Create(ctx, &u, client.DryRunAll); err != nil {
				t.Errorf("%s: %v", filepath.Base(file), err)
			}
		}
	}
}

func TestOwnerWithEmptyNamespace(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "owners-empty", nil)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kupra.futo.tech/v1alpha1",
		"kind":       "Preview",
		"metadata":   map[string]any{"name": "preview", "namespace": "owners-empty"},
		"spec": map[string]any{
			"appRef": map[string]any{"name": "app"},
			"owners": []any{map[string]any{"kind": "ServiceAccount", "name": "flux", "namespace": ""}},
		},
	}}
	if err := k8s.Create(ctx, u, client.DryRunAll); err == nil {
		t.Error("a service account owner with an empty namespace was accepted, want it rejected")
	}
}

func TestOwnerValidation(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "owners", nil)
	for _, tc := range []struct {
		owner rbacv1.Subject
		ok    bool
	}{
		{owner: rbacv1.Subject{Kind: rbacv1.GroupKind, Name: "github:octocat"}, ok: true},
		{owner: rbacv1.Subject{Kind: rbacv1.UserKind, Name: "alice"}, ok: true},
		{owner: rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Namespace: "flux-system", Name: "flux"}, ok: true},
		{owner: rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: "flux"}, ok: false},
		{owner: rbacv1.Subject{Kind: rbacv1.UserKind, Name: ""}, ok: false},
		{owner: rbacv1.Subject{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: "reviewers"}, ok: true},
		{owner: rbacv1.Subject{Kind: rbacv1.UserKind, APIGroup: "example.com", Name: "alice"}, ok: false},
		{owner: rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, APIGroup: rbacv1.GroupName, Namespace: "flux-system", Name: "flux"}, ok: false},
		{owner: rbacv1.Subject{Kind: "Robot", Name: "r2d2"}, ok: false},
		{owner: rbacv1.Subject{Kind: rbacv1.GroupKind, Name: "system:unauthenticated"}, ok: false},
		{owner: rbacv1.Subject{Kind: rbacv1.GroupKind, Name: "system:authenticated"}, ok: false},
		{owner: rbacv1.Subject{Kind: rbacv1.UserKind, Name: "system:anonymous"}, ok: false},
	} {
		p := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "preview-", Namespace: "owners"},
			Spec: kuprav1alpha1.PreviewSpec{
				AppRef: meta.LocalObjectReference{Name: "app"},
				Owners: []rbacv1.Subject{tc.owner},
			},
		}
		err := k8s.Create(ctx, p, client.DryRunAll)
		if tc.ok && err != nil {
			t.Errorf("owner %+v: %v", tc.owner, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("owner %+v was accepted, want it rejected", tc.owner)
		}
	}
}

func TestTimeoutValidation(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "timeouts", nil)
	for _, tc := range []struct {
		timeout string
		ok      bool
	}{
		{timeout: "30m", ok: true},
		{timeout: "1h30m", ok: true},
		{timeout: "banana", ok: false},
		{timeout: "-5s", ok: false},
	} {
		app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{GenerateName: "app-", Namespace: "timeouts"}}
		if err := yaml.UnmarshalStrict([]byte("template:\n  resources: []\n"), &app.Spec); err != nil {
			t.Fatal(err)
		}
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
		if err != nil {
			t.Fatal(err)
		}
		obj := &unstructured.Unstructured{Object: u}
		obj.SetGroupVersionKind(kuprav1alpha1.GroupVersion.WithKind("PreviewApp"))
		if err := unstructured.SetNestedField(obj.Object, tc.timeout, "spec", "timeout"); err != nil {
			t.Fatal(err)
		}
		err = k8s.Create(ctx, obj, client.DryRunAll)
		if tc.ok && err != nil {
			t.Errorf("timeout %s: %v", tc.timeout, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("timeout %s was accepted, want it rejected", tc.timeout)
		}
	}
}
