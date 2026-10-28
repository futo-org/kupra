package controller

import (
	"context"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

const (
	AnnotationCreatedBy = "kupra.futo.tech/created-by"

	// VerbSnapshot is the custom verb that allows taking a snapshot of a Preview.
	VerbSnapshot = "snapshot"

	adminClusterRole  = "kupra-admin"
	accessClusterRole = "kupra-preview-access"
	accessBindingName = "kupra-access"
)

// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previews,verbs=delete;snapshot
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=create;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=list;watch

// ensureAccess lets a preview's owners manage it, and lets them and the app's admins into its namespace, if that exists yet. It runs before anything else, so access is revoked even while a preview is blocked. It reports whether the preview's namespace exists.
func (r *PreviewReconciler) ensureAccess(ctx context.Context, p *kuprav1alpha1.Preview, namespace string) (namespaceExists bool, err error) {
	owners := ownerSubjects(p)

	name := "kupra-preview-" + p.Name
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace},
		Rules: []rbacv1.PolicyRule{{
			APIGroups:     []string{kuprav1alpha1.GroupVersion.Group},
			Resources:     []string{"previews"},
			ResourceNames: []string{p.Name},
			Verbs:         []string{"get", "update", "patch", "delete", VerbSnapshot},
		}},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name},
		Subjects:   owners,
	}
	for _, obj := range []client.Object{role, binding} {
		if err := controllerutil.SetControllerReference(p, obj, r.Scheme()); err != nil {
			return false, err
		}
		if err := r.applyObject(ctx, r.Client, obj); err != nil {
			return false, err
		}
	}

	var ns corev1.Namespace
	switch err := r.Get(ctx, client.ObjectKey{Name: namespace}, &ns); {
	case apierrors.IsNotFound(err):
		return false, nil
	case err != nil:
		return false, err
	case ns.Annotations[AnnotationPreviewUID] != string(p.UID) || !ns.DeletionTimestamp.IsZero():
		return false, nil
	}
	return true, r.bindNamespaceAccess(ctx, p, namespace)
}

// bindNamespaceAccess lets a preview's owners and the app's admins into its namespace.
func (r *PreviewReconciler) bindNamespaceAccess(ctx context.Context, p *kuprav1alpha1.Preview, namespace string) error {
	admins, err := r.appAdmins(ctx, p.Namespace)
	if err != nil {
		return err
	}
	access := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: accessBindingName, Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: accessClusterRole},
		Subjects:   union(ownerSubjects(p), admins),
	}
	return r.applyObject(ctx, r.Client, access)
}

// ownerSubjects are a Preview's creator and the owners its spec lists.
func ownerSubjects(p *kuprav1alpha1.Preview) []rbacv1.Subject {
	var subjects []rbacv1.Subject
	if creator := p.Annotations[AnnotationCreatedBy]; creator != "" {
		subjects = append(subjects, userSubject(creator))
	}
	for _, o := range p.Spec.Owners {
		if o.APIGroup == "" && (o.Kind == rbacv1.UserKind || o.Kind == rbacv1.GroupKind) {
			o.APIGroup = rbacv1.GroupName
		}
		subjects = append(subjects, o)
	}
	return union(nil, subjects)
}

// userSubject turns a username into a subject. Service accounts have usernames like system:serviceaccount:<namespace>:<name>, but RBAC binds them as ServiceAccount subjects.
func userSubject(username string) rbacv1.Subject {
	if rest, ok := strings.CutPrefix(username, "system:serviceaccount:"); ok {
		if ns, name, ok := strings.Cut(rest, ":"); ok {
			return rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Namespace: ns, Name: name}
		}
	}
	return rbacv1.Subject{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: username}
}

// appAdmins are the subjects bound to kupra-admin in an app namespace.
func (r *PreviewReconciler) appAdmins(ctx context.Context, namespace string) ([]rbacv1.Subject, error) {
	var bindings rbacv1.RoleBindingList
	if err := r.List(ctx, &bindings, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var admins []rbacv1.Subject
	for _, b := range bindings.Items {
		if b.RoleRef.Kind != "ClusterRole" || b.RoleRef.Name != adminClusterRole {
			continue
		}
		for _, s := range b.Subjects {
			// A service account without a namespace is in the binding's own namespace, which isn't the preview's.
			if s.Kind == rbacv1.ServiceAccountKind && s.Namespace == "" {
				s.Namespace = b.Namespace
			}
			admins = append(admins, s)
		}
	}
	return admins, nil
}

// previewsForAdminBinding maps a change to an app namespace's kupra-admin bindings to that namespace's Previews.
func (r *PreviewReconciler) previewsForAdminBinding(ctx context.Context, obj client.Object) []reconcile.Request {
	b, ok := obj.(*rbacv1.RoleBinding)
	if !ok || b.RoleRef.Kind != "ClusterRole" || b.RoleRef.Name != adminClusterRole {
		return nil
	}
	return r.previews(ctx, client.InNamespace(b.Namespace))
}

func union[T comparable](a, b []T) []T {
	out := slices.Clone(a)
	for _, v := range b {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
