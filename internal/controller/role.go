package controller

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// templateRole gets the ClusterRole a policy has templates applied with, and checks that kupra may bind it. It returns a reason when the role can't be used.
func templateRole(ctx context.Context, c client.Client, name string) (*rbacv1.ClusterRole, string, string, error) {
	var role rbacv1.ClusterRole
	if err := c.Get(ctx, client.ObjectKey{Name: name}, &role); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ReasonClusterRoleNotFound, fmt.Sprintf("ClusterRole %s not found", name), nil
		}
		return nil, "", "", err
	}

	review := &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{
		ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: "bind", Group: rbacv1.GroupName, Resource: "clusterroles", Name: name},
	}}
	if err := c.Create(ctx, review); err != nil {
		return nil, "", "", err
	}
	if !review.Status.Allowed {
		return nil, ReasonCannotBindRole, fmt.Sprintf("kupra may not bind ClusterRole %s", name), nil
	}
	return &role, "", "", nil
}
