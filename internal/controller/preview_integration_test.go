//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
	"github.com/futo-org/kupra/internal/policy"
)

func getPreview(t *testing.T, namespace, name string) *kuprav1alpha1.Preview {
	t.Helper()
	var p kuprav1alpha1.Preview
	if err := k8s.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func waitForPhase(t *testing.T, namespace, name string, phase kuprav1alpha1.PreviewPhase, reason string) *kuprav1alpha1.Preview {
	t.Helper()
	var p kuprav1alpha1.Preview
	eventually(t, func() error {
		if err := k8s.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, &p); err != nil {
			return err
		}
		c := apimeta.FindStatusCondition(p.Status.Conditions, meta.ReadyCondition)
		if p.Status.ObservedGeneration != p.Generation || p.Status.Phase != phase || c == nil || c.Reason != reason {
			return fmt.Errorf("Preview %s: phase %s, Ready %+v; want %s with reason %s", name, p.Status.Phase, c, phase, reason)
		}
		return nil
	})
	return &p
}

func TestPreviews(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t, &kuprav1alpha1.PreviewPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "previews"},
		Spec: kuprav1alpha1.PreviewPolicySpec{
			NetworkPolicies:   []kuprav1alpha1.NetworkPolicyTemplate{},
			NamespaceMetadata: &kuprav1alpha1.NamespaceMetadata{Labels: map[string]string{"example.com/gateway-access": "previews"}},
		},
	})
	createNamespace(t, "previews", map[string]string{policy.Label: "previews"})
	createApp(t, "previews", "app", `
parameters:
  - name: greeting
    title: Greeting
    default: hello
  - name: service
    title: Service
    type: boolean
    default: "true"
variables:
  - name: host
    expression: preview.name + ".example.com"
links:
  - name: Web
    urlExpr: '"https://" + variables.host'
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
      dataExpr: '{"greeting": parameters.greeting, "pr": preview.labels["example.com/pull-request"], "namespace": preview.namespace}'
    - apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: data
        annotations:
          kupra.futo.tech/include: parameters.service
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 1Gi
    - apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: cache
        annotations:
          kupra.futo.tech/include: parameters.service
          kupra.futo.tech/state: "false"
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 1Gi
    - apiVersion: v1
      kind: Service
      metadata:
        name: web
        annotations:
          kupra.futo.tech/include: parameters.service
      spec:
        selector: {app: web}
        ports:
          - port: 80
`)

	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-1", Namespace: "previews", Labels: map[string]string{"example.com/pull-request": "1"}},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
	}
	if err := k8s.Create(ctx, preview); err != nil {
		t.Fatal(err)
	}
	p := waitForPhase(t, "previews", "pr-1", kuprav1alpha1.PreviewRunning, "Succeeded")
	nsName := p.Status.Namespace

	if want := []kuprav1alpha1.LinkStatus{{Name: "Web", URL: "https://pr-1.example.com"}}; !slices.Equal(p.Status.Links, want) {
		t.Errorf("links = %v, want %v", p.Status.Links, want)
	}
	if len(p.Status.Inventory) != 4 {
		t.Errorf("inventory = %v, want the ConfigMap, both claims and the Service", p.Status.Inventory)
	}

	var ns corev1.Namespace
	if err := k8s.Get(ctx, client.ObjectKey{Name: nsName}, &ns); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"kupra.futo.tech/app":                "app",
		"kupra.futo.tech/preview":            "pr-1",
		"kupra.futo.tech/preview-namespace":  "previews",
		"pod-security.kubernetes.io/enforce": "baseline",
		"example.com/gateway-access":         "previews",
	} {
		if ns.Labels[k] != v {
			t.Errorf("namespace label %s = %q, want %q", k, ns.Labels[k], v)
		}
	}
	if ns.Annotations["kupra.futo.tech/preview-uid"] != string(p.UID) {
		t.Errorf("namespace annotations = %v, want the Preview's UID", ns.Annotations)
	}

	var cm corev1.ConfigMap
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "config"}, &cm); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"greeting": "hello", "pr": "1", "namespace": nsName}; fmt.Sprint(cm.Data) != fmt.Sprint(want) {
		t.Errorf("ConfigMap data = %v, want %v", cm.Data, want)
	}

	var binding rbacv1.RoleBinding
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "kupra-template"}, &binding); err != nil {
		t.Fatal(err)
	}
	if binding.RoleRef.Name != "kupra-preview-template-base" || len(binding.Subjects) != 1 || binding.Subjects[0].Namespace != kupraNamespace || binding.Subjects[0].Name != nsName {
		t.Errorf("template binding = %+v", binding)
	}
	var account corev1.ServiceAccount
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: kupraNamespace, Name: nsName}, &account); err != nil {
		t.Errorf("apply service account: %v", err)
	}

	var quota corev1.ResourceQuota
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "kupra"}, &quota); err != nil {
		t.Errorf("resource quota: %v", err)
	}
	var limits corev1.LimitRange
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "kupra"}, &limits); err != nil {
		t.Errorf("limit range: %v", err)
	}
	var netpols networkingv1.NetworkPolicyList
	if err := k8s.List(ctx, &netpols, client.InNamespace(nsName)); err != nil || len(netpols.Items) != 0 {
		t.Errorf("network policies = %v, %v; want none, as the policy has an empty list", netpols.Items, err)
	}

	t.Run("update", func(t *testing.T) {
		p := getPreview(t, "previews", "pr-1")
		p.Spec.Parameters = map[string]string{"greeting": "hi", "service": "false"}
		if err := k8s.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			var cm corev1.ConfigMap
			if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "config"}, &cm); err != nil {
				return err
			}
			if cm.Data["greeting"] != "hi" {
				return fmt.Errorf("greeting = %q, want hi", cm.Data["greeting"])
			}
			var svc corev1.Service
			if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "web"}, &svc); !apierrors.IsNotFound(err) {
				return fmt.Errorf("Service web still exists (%v)", err)
			}
			return nil
		})
		p = waitForPhase(t, "previews", "pr-1", kuprav1alpha1.PreviewRunning, "Succeeded")
		var retained []string
		for _, e := range p.Status.Inventory {
			if e.Retained {
				retained = append(retained, e.Name)
			}
		}
		if len(p.Status.Inventory) != 2 || !slices.Equal(retained, []string{"data"}) {
			t.Errorf("inventory = %+v, want the ConfigMap and the claim data, which is kept as state", p.Status.Inventory)
		}
		var cache corev1.PersistentVolumeClaim
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "cache"}, &cache); !apierrors.IsNotFound(err) && cache.DeletionTimestamp.IsZero() {
			t.Errorf("claim cache: %v; want it deleted, since it isn't state", err)
		}
		var pvc corev1.PersistentVolumeClaim
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "data"}, &pvc); err != nil || !pvc.DeletionTimestamp.IsZero() {
			t.Errorf("claim data: %v, deleted at %v; want it kept", err, pvc.DeletionTimestamp)
		}
	})

	t.Run("policy change", func(t *testing.T) {
		var pol kuprav1alpha1.PreviewPolicy
		if err := k8s.Get(ctx, client.ObjectKey{Name: "previews"}, &pol); err != nil {
			t.Fatal(err)
		}
		pol.Spec.NetworkPolicies = nil
		if err := k8s.Update(ctx, &pol); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			var np networkingv1.NetworkPolicy
			return k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "restrict-egress"}, &np)
		})
	})

	t.Run("long name", func(t *testing.T) {
		long := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", 64), Namespace: "previews"},
			Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
		}
		if err := k8s.Create(ctx, long); err == nil {
			t.Error("creating a Preview with a 64 character name succeeded, want it rejected")
		}
	})

	t.Run("policy change while the app is missing", func(t *testing.T) {
		createApp(t, "previews", "short-lived", `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
`)
		orphan := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "previews"},
			Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "short-lived"}},
		}
		if err := k8s.Create(ctx, orphan); err != nil {
			t.Fatal(err)
		}
		orphanNamespace := waitForPhase(t, "previews", "orphan", kuprav1alpha1.PreviewRunning, "Succeeded").Status.Namespace
		if err := k8s.Delete(ctx, &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: "short-lived", Namespace: "previews"}}); err != nil {
			t.Fatal(err)
		}
		waitForPhase(t, "previews", "orphan", kuprav1alpha1.PreviewPending, "AppNotFound")

		var pol kuprav1alpha1.PreviewPolicy
		if err := k8s.Get(ctx, client.ObjectKey{Name: "previews"}, &pol); err != nil {
			t.Fatal(err)
		}
		pol.Spec.NamespaceMetadata.Labels["pod-security.kubernetes.io/enforce"] = "restricted"
		if err := k8s.Update(ctx, &pol); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			var ns corev1.Namespace
			if err := k8s.Get(ctx, client.ObjectKey{Name: orphanNamespace}, &ns); err != nil {
				return err
			}
			if got := ns.Labels["pod-security.kubernetes.io/enforce"]; got != "restricted" {
				return fmt.Errorf("Pod Security level = %q, want restricted", got)
			}
			return nil
		})
	})

	t.Run("policy role that doesn't exist", func(t *testing.T) {
		setRole := func(role *kuprav1alpha1.ApplySettings) {
			t.Helper()
			var pol kuprav1alpha1.PreviewPolicy
			if err := k8s.Get(ctx, client.ObjectKey{Name: "previews"}, &pol); err != nil {
				t.Fatal(err)
			}
			pol.Spec.Apply = role
			if err := k8s.Update(ctx, &pol); err != nil {
				t.Fatal(err)
			}
		}
		setRole(&kuprav1alpha1.ApplySettings{ClusterRoleRef: meta.LocalObjectReference{Name: "missing"}})
		waitForPhase(t, "previews", "pr-1", kuprav1alpha1.PreviewPending, "ClusterRoleNotFound")
		var binding rbacv1.RoleBinding
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: nsName, Name: "kupra-template"}, &binding); err != nil || binding.RoleRef.Name != "kupra-preview-template-base" {
			t.Errorf("template binding = %+v, %v; want it left bound to the old role", binding.RoleRef, err)
		}
		setRole(nil)
		waitForPhase(t, "previews", "pr-1", kuprav1alpha1.PreviewRunning, "Succeeded")
	})

	t.Run("namespace conflict", func(t *testing.T) {
		conflict := controller.PreviewNamespaceName("previews", "taken")
		createNamespace(t, conflict, nil)
		taken := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: "taken", Namespace: "previews", Labels: map[string]string{"example.com/pull-request": "2"}},
			Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
		}
		if err := k8s.Create(ctx, taken); err != nil {
			t.Fatal(err)
		}
		waitForPhase(t, "previews", "taken", kuprav1alpha1.PreviewPending, "NamespaceConflict")

		// An account of the same name belongs to whichever Preview owns the namespace, and deleting this one mustn't remove it.
		account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name:        conflict,
			Namespace:   kupraNamespace,
			Annotations: map[string]string{"kupra.futo.tech/preview-uid": "someone-else"},
		}}
		if err := k8s.Create(ctx, account); err != nil {
			t.Fatal(err)
		}
		if err := k8s.Delete(ctx, taken); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(taken), taken); !apierrors.IsNotFound(err) {
				return fmt.Errorf("Preview taken still exists (%v)", err)
			}
			return nil
		})
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(account), account); err != nil {
			t.Errorf("the other Preview's account: %v", err)
		}
	})

	t.Run("invalid parameters", func(t *testing.T) {
		invalid := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid", Namespace: "previews"},
			Spec: kuprav1alpha1.PreviewSpec{
				AppRef:     meta.LocalObjectReference{Name: "app"},
				Parameters: map[string]string{"service": "maybe"},
			},
		}
		if err := k8s.Create(ctx, invalid); err != nil {
			t.Fatal(err)
		}
		p := waitForPhase(t, "previews", "invalid", kuprav1alpha1.PreviewFailed, "RenderFailed")
		var ns corev1.Namespace
		if err := k8s.Get(ctx, client.ObjectKey{Name: p.Status.Namespace}, &ns); !apierrors.IsNotFound(err) {
			t.Errorf("namespace %s exists (%v), want none for a Preview that can't render", p.Status.Namespace, err)
		}
	})

	t.Run("object the API server rejects", func(t *testing.T) {
		createApp(t, "previews", "rejected", `
template:
  resources:
    - apiVersion: v1
      kind: Service
      metadata:
        name: web
      spec:
        ports:
          - port: 70000
`)
		rejected := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: "rejected", Namespace: "previews"},
			Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "rejected"}},
		}
		if err := k8s.Create(ctx, rejected); err != nil {
			t.Fatal(err)
		}
		p := waitForPhase(t, "previews", "rejected", kuprav1alpha1.PreviewFailed, "ApplyFailed")
		if c := apimeta.FindStatusCondition(p.Status.Conditions, meta.StalledCondition); c == nil || c.Status != metav1.ConditionTrue {
			t.Errorf("Stalled = %+v, want True straight away", c)
		}
	})

	t.Run("app that fails its checks", func(t *testing.T) {
		createApp(t, "previews", "broken", `
template:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: server
`)
		broken := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "previews"},
			Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "broken"}},
		}
		if err := k8s.Create(ctx, broken); err != nil {
			t.Fatal(err)
		}
		waitForPhase(t, "previews", "broken", kuprav1alpha1.PreviewPending, "AppNotReady")
	})

	t.Run("deletion", func(t *testing.T) {
		// envtest has no namespace controller, so the namespace never finishes deleting. The kind end-to-end tests cover the rest.
		if err := k8s.Delete(ctx, getPreview(t, "previews", "pr-1")); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			p := getPreview(t, "previews", "pr-1")
			var ns corev1.Namespace
			if err := k8s.Get(ctx, client.ObjectKey{Name: nsName}, &ns); err != nil {
				return err
			}
			if p.Status.Phase != kuprav1alpha1.PreviewTerminating || ns.DeletionTimestamp.IsZero() {
				return fmt.Errorf("phase %s, namespace deleted at %v; want Terminating and the namespace being deleted", p.Status.Phase, ns.DeletionTimestamp)
			}
			return nil
		})
	})
}
