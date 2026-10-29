package controller

import (
	"context"

	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/futo-org/kupra/internal/check"
)

const (
	LabelApp              = "kupra.futo.tech/app"
	LabelPreview          = "kupra.futo.tech/preview"
	LabelPreviewNamespace = "kupra.futo.tech/preview-namespace"
	AnnotationPreviewUID  = "kupra.futo.tech/preview-uid"

	FieldManager = "kupra"
	Finalizer    = "kupra.futo.tech/cleanup"
)

// Leader election:
// +kubebuilder:rbac:groups=coordination.k8s.io,namespace=kupra-system,resources=leases,verbs=get;create;update
// +kubebuilder:rbac:groups="",namespace=kupra-system,resources=events,verbs=create;patch

type Options struct {
	// Namespace is kupra's own namespace.
	Namespace string
}

// Setup registers the controllers with the manager.
func Setup(ctx context.Context, mgr ctrl.Manager, opts Options) error {
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

	previews := &PreviewReconciler{
		Client:    mgr.GetClient(),
		APIReader: mgr.GetAPIReader(),
		Config:    mgr.GetConfig(),
		Mapper:    mgr.GetRESTMapper(),
		Namespace: opts.Namespace,
	}
	if err := previews.SetupWithManager(ctx, mgr); err != nil {
		return err
	}

	snapshots := &PreviewSnapshotReconciler{
		Client:    mgr.GetClient(),
		APIReader: mgr.GetAPIReader(),
	}
	return snapshots.SetupWithManager(mgr)
}
