package controller

import (
	"context"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/check"
	"github.com/futo-org/kupra/internal/policy"
	"github.com/futo-org/kupra/internal/render"
)

const (
	ReasonInvalid             = "Invalid"
	ReasonCheckFailed         = "CheckFailed"
	ReasonPolicyNotFound      = "PolicyNotFound"
	ReasonClusterRoleNotFound = "ClusterRoleNotFound"
	ReasonNoTemplateRole      = "NoTemplateRole"
	ReasonCannotBindRole      = "CannotBindRole"

	// LabelAggregateToOperator approves a ClusterRole's rules for kupra. Admins use it to let kupra bind a custom template role.
	LabelAggregateToOperator = "kupra.futo.tech/aggregate-to-operator"
)

// recheckInterval is how often failed checks are repeated, since a CRD being installed or a role changing can fix them.
const recheckInterval = time.Minute

// PreviewAppReconciler checks PreviewApps before any preview uses them.
type PreviewAppReconciler struct {
	client.Client
	Checker *check.Checker
	// Discovery is invalidated before a failed check is repeated, in case it failed on a stale schema.
	Discovery discovery.CachedDiscoveryInterface
}

// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewapps,verbs=get;list;watch
// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch

func (r *PreviewAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var app kuprav1alpha1.PreviewApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reason, message, err := r.check(ctx, &app)
	if err != nil {
		return ctrl.Result{}, err
	}

	patcher := patch.NewSerialPatcher(&app, r.Client)
	app.Status.ObservedGeneration = app.Generation
	if reason == meta.SucceededReason {
		conditions.MarkTrue(&app, meta.ReadyCondition, reason, "")
		conditions.Delete(&app, meta.StalledCondition)
	} else {
		conditions.MarkFalse(&app, meta.ReadyCondition, reason, "%s", message)
		conditions.MarkTrue(&app, meta.StalledCondition, reason, "%s", message)
	}
	owned := patch.WithOwnedConditions{Conditions: []string{meta.ReadyCondition, meta.StalledCondition}}
	if err := patcher.Patch(ctx, &app, owned, patch.WithFieldOwner(FieldManager)); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if reason != meta.SucceededReason && reason != ReasonInvalid {
		return ctrl.Result{RequeueAfter: recheckInterval}, nil
	}
	return ctrl.Result{}, nil
}

func (r *PreviewAppReconciler) check(ctx context.Context, app *kuprav1alpha1.PreviewApp) (reason, message string, err error) {
	compiled, errs := render.Compile(&app.Spec)
	if len(errs) > 0 {
		return ReasonInvalid, errs.ToAggregate().Error(), nil
	}

	_, settings, err := policy.Resolve(ctx, r.Client, app.Namespace)
	if reason, ok := policyProblem(err); ok {
		return reason, err.Error(), nil
	}
	if err != nil {
		return "", "", err
	}

	roleName := settings.Apply.ClusterRoleRef.Name
	role, reason, message, err := templateRole(ctx, r.Client, roleName)
	if err != nil || reason != "" {
		return reason, message, err
	}

	errs = r.Checker.Template(compiled, roleName, role.Rules)
	if len(errs) > 0 && r.Discovery != nil {
		r.Discovery.Invalidate()
		errs = r.Checker.Template(compiled, roleName, role.Rules)
	}
	if len(errs) > 0 {
		return ReasonCheckFailed, errs.ToAggregate().Error(), nil
	}
	return meta.SucceededReason, "", nil
}

func (r *PreviewAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuprav1alpha1.PreviewApp{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&kuprav1alpha1.PreviewPolicy{}, handler.EnqueueRequestsFromMapFunc(r.allApps)).
		Watches(&rbacv1.ClusterRole{}, handler.EnqueueRequestsFromMapFunc(r.allApps)).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.appsInNamespace), builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}

func (r *PreviewAppReconciler) allApps(ctx context.Context, _ client.Object) []reconcile.Request {
	return r.apps(ctx)
}

func (r *PreviewAppReconciler) appsInNamespace(ctx context.Context, ns client.Object) []reconcile.Request {
	return r.apps(ctx, client.InNamespace(ns.GetName()))
}

func (r *PreviewAppReconciler) apps(ctx context.Context, opts ...client.ListOption) []reconcile.Request {
	var apps kuprav1alpha1.PreviewAppList
	if err := r.List(ctx, &apps, opts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "listing PreviewApps")
		return nil
	}
	requests := make([]reconcile.Request, len(apps.Items))
	for i, app := range apps.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&app)}
	}
	return requests
}
