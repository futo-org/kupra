package controller

import (
	"context"
	"errors"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	rbacv1 "k8s.io/api/rbac/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/policy"
)

// PreviewPolicyReconciler reports whether previews can use each PreviewPolicy, so an admin sees a broken policy on the policy itself.
type PreviewPolicyReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewpolicies/status,verbs=get;update;patch

func (r *PreviewPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var p kuprav1alpha1.PreviewPolicy
	if err := r.Get(ctx, req.NamespacedName, &p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reason, message, err := r.check(ctx, &p)
	if err != nil {
		return ctrl.Result{}, err
	}

	patch := client.MergeFromWithOptions(p.DeepCopy(), client.MergeFromWithOptimisticLock{})
	p.Status.ObservedGeneration = p.Generation
	if reason == meta.SucceededReason {
		conditions.MarkTrue(&p, meta.ReadyCondition, reason, "")
		conditions.Delete(&p, meta.StalledCondition)
	} else {
		conditions.MarkFalse(&p, meta.ReadyCondition, reason, "%s", message)
		conditions.MarkTrue(&p, meta.StalledCondition, reason, "%s", message)
	}
	if err := r.Status().Patch(ctx, &p, patch); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Whether kupra may bind a role depends on bindings it doesn't watch, so it's checked again.
	if reason != meta.SucceededReason {
		return ctrl.Result{RequeueAfter: recheckInterval}, nil
	}
	return ctrl.Result{RequeueAfter: readyRecheckInterval}, nil
}

// readyRecheckInterval is how often a policy previews can use is checked again.
const readyRecheckInterval = 10 * time.Minute

// check works out whether previews can use a policy: it must name a template role, or fall back to one, which exists and kupra may bind.
func (r *PreviewPolicyReconciler) check(ctx context.Context, p *kuprav1alpha1.PreviewPolicy) (reason, message string, err error) {
	settings, err := policy.Settings(ctx, r.Client, p)
	if reason, ok := policyProblem(err); ok {
		return reason, err.Error(), nil
	}
	if err != nil {
		return "", "", err
	}
	if _, reason, message, err := templateRole(ctx, r.Client, settings.Apply.ClusterRoleRef.Name); err != nil || reason != "" {
		return reason, message, err
	}
	return meta.SucceededReason, "", nil
}

// policyProblem returns the reason for a PreviewPolicy that previews can't use, if err is one.
func policyProblem(err error) (string, bool) {
	if notFound := (*policy.NotFoundError)(nil); errors.As(err, &notFound) {
		return ReasonPolicyNotFound, true
	}
	if noRole := (*policy.NoTemplateRoleError)(nil); errors.As(err, &noRole) {
		return ReasonNoTemplateRole, true
	}
	return "", false
}

func (r *PreviewPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuprav1alpha1.PreviewPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Other policies fall back to the default one, and the policies' roles can change or be approved.
		Watches(&kuprav1alpha1.PreviewPolicy{}, handler.EnqueueRequestsFromMapFunc(r.allPolicies), builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&rbacv1.ClusterRole{}, handler.EnqueueRequestsFromMapFunc(r.allPolicies)).
		Complete(r)
}

func (r *PreviewPolicyReconciler) allPolicies(ctx context.Context, _ client.Object) []reconcile.Request {
	var list kuprav1alpha1.PreviewPolicyList
	if err := r.List(ctx, &list); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "listing PreviewPolicies")
		return nil
	}
	requests := make([]reconcile.Request, len(list.Items))
	for i, p := range list.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&p)}
	}
	return requests
}
