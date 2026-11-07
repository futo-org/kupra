package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// LabelHasApps marks the namespaces that contain PreviewApps, so the dashboard can find apps without reading every namespace's.
const LabelHasApps = "kupra.futo.tech/has-apps"

// appNamespaceFieldManager owns only LabelHasApps, so applying a namespace without it removes the label and nothing else.
const appNamespaceFieldManager = "kupra-apps"

// AppNamespaceReconciler labels the namespaces that contain PreviewApps.
type AppNamespaceReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewapps,verbs=get;list;watch

func (r *AppNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var ns corev1.Namespace
	if err := r.Get(ctx, req.NamespacedName, &ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !ns.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	var apps kuprav1alpha1.PreviewAppList
	if err := r.List(ctx, &apps, client.InNamespace(ns.Name), client.Limit(1)); err != nil {
		return ctrl.Result{}, err
	}
	hasApps := len(apps.Items) > 0
	if _, labelled := ns.Labels[LabelHasApps]; labelled == hasApps {
		return ctrl.Result{}, nil
	}
	apply := corev1ac.Namespace(ns.Name)
	if hasApps {
		apply.WithLabels(map[string]string{LabelHasApps: "true"})
	}
	return ctrl.Result{}, r.Apply(ctx, apply, client.FieldOwner(appNamespaceFieldManager))
}

func (r *AppNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("appnamespace").
		For(&corev1.Namespace{}).
		Watches(&kuprav1alpha1.PreviewApp{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, app client.Object) []reconcile.Request {
			return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: app.GetNamespace()}}}
		})).
		Complete(r)
}
