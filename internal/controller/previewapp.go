package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	ReasonSucceeded           = "Succeeded"
	ReasonInvalid             = "Invalid"
	ReasonCheckFailed         = "CheckFailed"
	ReasonPolicyNotFound      = "PolicyNotFound"
	ReasonClusterRoleNotFound = "ClusterRoleNotFound"
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

	patch := client.MergeFrom(app.DeepCopy())
	app.Status.ObservedGeneration = app.Generation
	if reason == ReasonSucceeded {
		setCondition(&app.Status.Conditions, app.Generation, kuprav1alpha1.ConditionReady, metav1.ConditionTrue, reason, "")
		meta.RemoveStatusCondition(&app.Status.Conditions, kuprav1alpha1.ConditionStalled)
	} else {
		setCondition(&app.Status.Conditions, app.Generation, kuprav1alpha1.ConditionReady, metav1.ConditionFalse, reason, message)
		setCondition(&app.Status.Conditions, app.Generation, kuprav1alpha1.ConditionStalled, metav1.ConditionTrue, reason, message)
	}
	if err := r.Status().Patch(ctx, &app, patch); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if reason != ReasonSucceeded && reason != ReasonInvalid {
		return ctrl.Result{RequeueAfter: recheckInterval}, nil
	}
	return ctrl.Result{}, nil
}

func (r *PreviewAppReconciler) check(ctx context.Context, app *kuprav1alpha1.PreviewApp) (reason, message string, err error) {
	compiled, errs := render.Compile(&app.Spec)
	if len(errs) > 0 {
		return ReasonInvalid, errs.ToAggregate().Error(), nil
	}

	p, err := policy.ForNamespace(ctx, r.Client, app.Namespace)
	if notFound := (*policy.NotFoundError)(nil); errors.As(err, &notFound) {
		return ReasonPolicyNotFound, err.Error(), nil
	}
	if err != nil {
		return "", "", err
	}

	roleName := policy.Effective(p.Spec).Apply.ClusterRoleRef.Name
	var role rbacv1.ClusterRole
	if err := r.Get(ctx, client.ObjectKey{Name: roleName}, &role); err != nil {
		if apierrors.IsNotFound(err) {
			return ReasonClusterRoleNotFound, fmt.Sprintf("the policy's ClusterRole %s doesn't exist", roleName), nil
		}
		return "", "", err
	}

	review := &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{
		ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: "bind", Group: rbacv1.GroupName, Resource: "clusterroles", Name: roleName},
	}}
	if err := r.Create(ctx, review); err != nil {
		return "", "", err
	}
	if !review.Status.Allowed {
		return ReasonCannotBindRole, fmt.Sprintf("kupra isn't allowed to bind the policy's ClusterRole %s. Allow it with a ClusterRole labelled %s that grants bind on it.", roleName, LabelAggregateToOperator), nil
	}

	errs = r.Checker.Template(compiled, roleName, role.Rules)
	if len(errs) > 0 && r.Discovery != nil {
		r.Discovery.Invalidate()
		errs = r.Checker.Template(compiled, roleName, role.Rules)
	}
	if len(errs) > 0 {
		return ReasonCheckFailed, errs.ToAggregate().Error(), nil
	}
	return ReasonSucceeded, "", nil
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

func setCondition(conditions *[]metav1.Condition, generation int64, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: generation,
		Reason:             reason,
		Message:            message,
	})
}
