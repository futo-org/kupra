package controller

import (
	"context"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

var PreviewNamespaceName = previewNamespaceName

func (r *PreviewReconciler) EnsureNamespace(ctx context.Context, p *kuprav1alpha1.Preview, name string, settings kuprav1alpha1.PreviewPolicySpec) (string, string, error) {
	return r.ensureNamespace(ctx, p, name, settings)
}

func (r *PreviewReconciler) ReconcileDelete(ctx context.Context, p *kuprav1alpha1.Preview) (bool, error) {
	_, cleanedUp, err := r.reconcileDelete(ctx, p)
	return cleanedUp, err
}
