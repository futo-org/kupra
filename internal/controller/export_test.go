package controller

import (
	"context"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

func (r *PreviewReconciler) EnsureNamespace(ctx context.Context, p *kuprav1alpha1.Preview, name string, settings kuprav1alpha1.PreviewPolicySpec) (string, string, error) {
	return r.ensureNamespace(ctx, p, name, settings)
}

func (r *PreviewReconciler) ReconcileDelete(ctx context.Context, p *kuprav1alpha1.Preview) (bool, error) {
	_, cleanedUp, err := r.reconcileDelete(ctx, p)
	return cleanedUp, err
}

func (r *PreviewReconciler) EnsureAccess(ctx context.Context, p *kuprav1alpha1.Preview, namespace string) (bool, error) {
	return r.ensureAccess(ctx, p, namespace)
}

func (r *PreviewReconciler) HoldSnapshot(ctx context.Context, p *kuprav1alpha1.Preview, name string) (string, error) {
	_, reason, _, err := r.holdSnapshot(ctx, p, name)
	return reason, err
}
