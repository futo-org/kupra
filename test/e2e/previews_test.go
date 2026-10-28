//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// The kind node image already has the pause image, as its pod sandbox.
const appSpec = `
parameters:
  - name: replicas
    title: Replicas
    type: integer
    default: "1"
variables:
  - name: host
    expression: preview.name + ".example.com"
links:
  - name: Web
    urlExpr: '"https://" + variables.host'
template:
  resources:
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: web
      spec:
        replicasExpr: parameters.replicas
        selector:
          matchLabels: {app: web}
        template:
          metadata:
            labels: {app: web}
          spec:
            containers:
              - name: web
                image: registry.k8s.io/pause:3.10
    - apiVersion: v1
      kind: Service
      metadata:
        name: web
      spec:
        selector: {app: web}
        ports:
          - port: 80
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
      dataExpr: '{"host": variables.host}'
`

func TestPreviewLifecycle(t *testing.T) {
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-previews-"}}
	if err := k8s.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := k8s.Delete(context.Background(), ns); err != nil {
			t.Errorf("deleting namespace: %v", err)
		}
	})

	app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns.Name}}
	if err := yaml.UnmarshalStrict([]byte(appSpec), &app.Spec); err != nil {
		t.Fatal(err)
	}
	if err := k8s.Create(ctx, app); err != nil {
		t.Fatal(err)
	}

	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-1", Namespace: ns.Name},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
	}
	if err := k8s.Create(ctx, preview); err != nil {
		t.Fatal(err)
	}
	if preview.Annotations["kupra.futo.tech/created-by"] == "" {
		t.Errorf("Preview has no created-by annotation: %v", preview.Annotations)
	}

	running := func(replicas int32) {
		t.Helper()
		eventually(t, 3*time.Minute, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(preview), preview); err != nil {
				return err
			}
			ready := apimeta.FindStatusCondition(preview.Status.Conditions, meta.ReadyCondition)
			if preview.Status.ObservedGeneration != preview.Generation || preview.Status.Phase != kuprav1alpha1.PreviewRunning {
				return fmt.Errorf("Preview is %s, Ready %+v", preview.Status.Phase, ready)
			}
			var d appsv1.Deployment
			if err := k8s.Get(ctx, client.ObjectKey{Namespace: preview.Status.Namespace, Name: "web"}, &d); err != nil {
				return err
			}
			if d.Status.ReadyReplicas != replicas {
				return fmt.Errorf("Deployment has %d ready replicas, want %d", d.Status.ReadyReplicas, replicas)
			}
			return nil
		})
	}
	running(1)
	previewNamespace := preview.Status.Namespace

	if len(preview.Status.Links) != 1 || preview.Status.Links[0].URL != "https://pr-1.example.com" {
		t.Errorf("links = %v", preview.Status.Links)
	}
	var np networkingv1.NetworkPolicy
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: previewNamespace, Name: "restrict-egress"}, &np); err != nil {
		t.Errorf("default network policy: %v", err)
	}

	preview.Spec.Parameters = map[string]string{"replicas": "2"}
	if err := k8s.Update(ctx, preview); err != nil {
		t.Fatal(err)
	}
	running(2)

	if err := k8s.Delete(ctx, preview); err != nil {
		t.Fatal(err)
	}
	eventually(t, 3*time.Minute, func() error {
		for _, obj := range []client.Object{
			&kuprav1alpha1.Preview{ObjectMeta: metav1.ObjectMeta{Name: preview.Name, Namespace: preview.Namespace}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: previewNamespace}},
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: previewNamespace, Namespace: kupraNamespace}},
		} {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(obj), obj); !apierrors.IsNotFound(err) {
				return fmt.Errorf("%T %s still exists (%v)", obj, obj.GetName(), err)
			}
		}
		return nil
	})
}
