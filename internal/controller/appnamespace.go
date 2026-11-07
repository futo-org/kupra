package controller

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

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
	if _, labelled := ns.Labels[kuprav1alpha1.LabelHasApps]; labelled == hasApps {
		return ctrl.Result{}, nil
	}
	// A merge patch sets or removes just this label, whoever set it.
	value := any(nil)
	if hasApps {
		value = "true"
	}
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"labels": map[string]any{kuprav1alpha1.LabelHasApps: value}}})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, client.IgnoreNotFound(r.Patch(ctx, &ns, client.RawPatch(types.MergePatchType, patch), client.FieldOwner(FieldManager)))
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
