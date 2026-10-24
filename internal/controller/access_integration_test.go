//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/policy"
)

func bind(t *testing.T, namespace, clusterRole, user string) {
	t.Helper()
	b := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRole + "-" + user, Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: clusterRole},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: user}},
	}
	if err := k8s.Create(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

func as(t *testing.T, user string) client.Client {
	t.Helper()
	cfg := rest.CopyConfig(restConfig)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: user}
	c, err := client.New(cfg, client.Options{Scheme: k8s.Scheme()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func allowed(t *testing.T, user string, attrs authorizationv1.ResourceAttributes) bool {
	t.Helper()
	sar := &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{User: user, ResourceAttributes: &attrs}}
	if err := k8s.Create(context.Background(), sar); err != nil {
		t.Fatal(err)
	}
	return sar.Status.Allowed
}

func TestAccess(t *testing.T) {
	ctx := context.Background()
	templates := filepath.Join("..", "..", "charts", "kupra", "templates")
	apply(t, filepath.Join(templates, "access-clusterroles.yaml"))
	apply(t, filepath.Join(templates, "created-by.yaml"))
	setUpPolicy(t, &kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: policy.DefaultName}})

	createNamespace(t, "access", nil)
	createApp(t, "access", "app", `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
`)
	bind(t, "access", "kupra-creator", "alice")
	bind(t, "access", "kupra-creator", "carol")
	bind(t, "access", "kupra-admin", "bob")

	// The admission policy may take a moment to come into effect.
	var preview *kuprav1alpha1.Preview
	eventually(t, func() error {
		p := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "alice-", Namespace: "access", Annotations: map[string]string{"kupra.futo.tech/created-by": "mallory"}},
			Spec:       kuprav1alpha1.PreviewSpec{AppRef: kuprav1alpha1.LocalObjectReference{Name: "app"}},
		}
		if err := as(t, "alice").Create(ctx, p); err != nil {
			return err
		}
		if got := p.Annotations["kupra.futo.tech/created-by"]; got != "alice" {
			_ = k8s.Delete(ctx, p)
			return fmt.Errorf("created-by = %q, want alice", got)
		}
		preview = p
		return nil
	})
	p := waitForPhase(t, "access", preview.Name, kuprav1alpha1.PreviewRunning, "Succeeded")

	t.Run("created-by can't be changed", func(t *testing.T) {
		p := getPreview(t, "access", preview.Name)
		p.Annotations["kupra.futo.tech/created-by"] = "mallory"
		if err := as(t, "alice").Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got := p.Annotations["kupra.futo.tech/created-by"]; got != "alice" {
			t.Errorf("created-by = %q after an update, want alice", got)
		}
	})

	previewAttrs := func(verb string) authorizationv1.ResourceAttributes {
		return authorizationv1.ResourceAttributes{Namespace: "access", Verb: verb, Group: "kupra.futo.tech", Resource: "previews", Name: preview.Name}
	}
	execAttrs := authorizationv1.ResourceAttributes{Namespace: p.Status.Namespace, Verb: "create", Resource: "pods", Subresource: "exec"}

	eventually(t, func() error {
		for _, tc := range []struct {
			user  string
			attrs authorizationv1.ResourceAttributes
			want  bool
		}{
			{"alice", previewAttrs("delete"), true},
			{"alice", previewAttrs("snapshot"), true},
			{"carol", previewAttrs("delete"), false},
			{"carol", previewAttrs("snapshot"), false},
			{"bob", previewAttrs("delete"), true},
			{"alice", execAttrs, true},
			{"bob", execAttrs, true},
			{"carol", execAttrs, false},
		} {
			if got := allowed(t, tc.user, tc.attrs); got != tc.want {
				return fmt.Errorf("%s may %s %s/%s: %v, want %v", tc.user, tc.attrs.Verb, tc.attrs.Resource, tc.attrs.Subresource, got, tc.want)
			}
		}
		return nil
	})

	t.Run("owners and admins added later", func(t *testing.T) {
		p := getPreview(t, "access", preview.Name)
		p.Spec.Owners = []kuprav1alpha1.Owner{{Kind: kuprav1alpha1.OwnerGroup, Name: "reviewers"}}
		if err := k8s.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		bind(t, "access", "kupra-admin", "dave")
		eventually(t, func() error {
			sar := &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{
				User: "erin", Groups: []string{"reviewers"}, ResourceAttributes: ptr(previewAttrs("update")),
			}}
			if err := k8s.Create(ctx, sar); err != nil {
				return err
			}
			if !sar.Status.Allowed {
				return fmt.Errorf("a member of reviewers can't update the Preview")
			}
			if !allowed(t, "dave", execAttrs) {
				return fmt.Errorf("dave, a new admin, can't exec into the preview")
			}
			return nil
		})
	})

	t.Run("service account admins", func(t *testing.T) {
		b := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "kupra-admin-deployer", Namespace: "access"},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "kupra-admin"},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "deployer"}},
		}
		if err := k8s.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			if !allowed(t, "system:serviceaccount:access:deployer", execAttrs) {
				return fmt.Errorf("access/deployer can't exec into the preview")
			}
			return nil
		})
		if allowed(t, "system:serviceaccount:"+p.Status.Namespace+":deployer", execAttrs) {
			t.Error("a deployer account in the preview's own namespace can exec into it")
		}
	})

	t.Run("revoked while the app is missing", func(t *testing.T) {
		if err := k8s.Delete(ctx, &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "access"}}); err != nil {
			t.Fatal(err)
		}
		waitForPhase(t, "access", preview.Name, kuprav1alpha1.PreviewPending, "AppNotFound")

		p := getPreview(t, "access", preview.Name)
		p.Spec.Owners = nil
		if err := k8s.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err := k8s.Delete(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "kupra-admin-dave", Namespace: "access"}}); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			sar := &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{
				User: "erin", Groups: []string{"reviewers"}, ResourceAttributes: ptr(previewAttrs("update")),
			}}
			if err := k8s.Create(ctx, sar); err != nil {
				return err
			}
			if sar.Status.Allowed {
				return fmt.Errorf("a member of reviewers can still update the Preview")
			}
			if allowed(t, "dave", execAttrs) {
				return fmt.Errorf("dave can still exec into the preview")
			}
			return nil
		})
	})
}

func ptr[T any](v T) *T {
	return &v
}
