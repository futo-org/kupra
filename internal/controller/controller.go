package controller

import (
	"context"

	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/check"
)

// Setup registers the controllers with the manager.
func Setup(mgr ctrl.Manager) error {
	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	cached := memory.NewMemCacheClient(dc)

	apps := &PreviewAppReconciler{
		Client: mgr.GetClient(),
		Checker: &check.Checker{
			Mapper:  mgr.GetRESTMapper(),
			Schemas: &resolver.ClientDiscoveryResolver{Discovery: cached},
		},
		Discovery: cached,
	}
	if err := apps.SetupWithManager(mgr); err != nil {
		return err
	}

	for _, obj := range []client.Object{
		&kuprav1alpha1.Preview{},
		&kuprav1alpha1.PreviewSnapshot{},
	} {
		if err := ctrl.NewControllerManagedBy(mgr).For(obj).Complete(noopReconciler{}); err != nil {
			return err
		}
	}
	return nil
}

// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previews;previewsnapshots,verbs=get;list;watch

type noopReconciler struct{}

func (noopReconciler) Reconcile(context.Context, ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
