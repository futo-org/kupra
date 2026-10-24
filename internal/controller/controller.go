package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// Setup registers the controllers with the manager.
func Setup(mgr ctrl.Manager) error {
	for _, obj := range []client.Object{
		&kuprav1alpha1.PreviewApp{},
		&kuprav1alpha1.Preview{},
		&kuprav1alpha1.PreviewSnapshot{},
		&kuprav1alpha1.PreviewPolicy{},
	} {
		if err := ctrl.NewControllerManagedBy(mgr).For(obj).Complete(noopReconciler{}); err != nil {
			return err
		}
	}
	return nil
}

type noopReconciler struct{}

func (noopReconciler) Reconcile(context.Context, ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
