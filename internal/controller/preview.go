package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/policy"
	"github.com/futo-org/kupra/internal/render"
	"github.com/futo-org/kupra/internal/state"
)

const (
	ReasonAppNotFound          = "AppNotFound"
	ReasonAppNotReady          = "AppNotReady"
	ReasonNamespaceConflict    = "NamespaceConflict"
	ReasonNamespaceTerminating = "NamespaceTerminating"
	ReasonRenderFailed         = "RenderFailed"
	ReasonApplyFailed          = "ApplyFailed"
	ReasonTimeout              = "Timeout"
	ReasonTerminating          = "Terminating"

	// LabelPolicyResource marks the objects kupra creates in a preview namespace from its policy.
	LabelPolicyResource = "kupra.futo.tech/policy-resource"

	templateBindingName = "kupra-template"
	policyObjectName    = "kupra"

	defaultTimeout = 30 * time.Minute

	// Readiness of applied resources is polled, since they can be of any kind.
	startingInterval = 5 * time.Second
	runningInterval  = 5 * time.Minute
	// nextStepInterval is how soon kupra comes back to a preview to take the next step of a capture or reset.
	nextStepInterval = time.Second
)

// PreviewReconciler runs previews: their namespaces, the policy's settings there, and their rendered templates.
type PreviewReconciler struct {
	client.Client
	// APIReader reads objects kupra doesn't cache, such as a preview namespace's RoleBindings.
	APIReader client.Reader
	Config    *rest.Config
	Mapper    apimeta.RESTMapper
	// Namespace is kupra's own namespace, where previews' service accounts live.
	Namespace string
}

// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previews,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previews/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previews/finalizers,verbs=update
// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewapps;previewpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=create;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=list;create;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;create;patch;delete
// +kubebuilder:rbac:groups="",namespace=kupra-system,resources=serviceaccounts,verbs=get;create;patch;delete;impersonate

// Reconcile does the writes: it adds the finalizer, patches whatever status reconcile or reconcileDelete worked out, and removes the finalizer once cleanup is done.
func (r *PreviewReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var preview kuprav1alpha1.Preview
	if err := r.Get(ctx, req.NamespacedName, &preview); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	deleting := !preview.DeletionTimestamp.IsZero()
	switch {
	case deleting && !controllerutil.ContainsFinalizer(&preview, Finalizer):
		return ctrl.Result{}, nil
	case !deleting && !controllerutil.ContainsFinalizer(&preview, Finalizer):
		patch := client.MergeFromWithOptions(preview.DeepCopy(), client.MergeFromWithOptimisticLock{})
		controllerutil.AddFinalizer(&preview, Finalizer)
		if err := r.Patch(ctx, &preview, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	before := preview.DeepCopy()
	var result ctrl.Result
	var cleanedUp bool
	var err error
	if deleting {
		result, cleanedUp, err = r.reconcileDelete(ctx, &preview)
	} else {
		result, err = r.reconcile(ctx, &preview)
	}
	// The lock makes a reconciliation that started from a stale read fail here and start again, rather than write stale status, such as a condition another reconciliation has since changed.
	if patchErr := r.Status().Patch(ctx, &preview, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); patchErr != nil {
		if apierrors.IsConflict(patchErr) && err == nil {
			return ctrl.Result{RequeueAfter: nextStepInterval}, nil
		}
		return ctrl.Result{}, errors.Join(err, patchErr)
	}
	if cleanedUp {
		patch := client.MergeFromWithOptions(preview.DeepCopy(), client.MergeFromWithOptimisticLock{})
		controllerutil.RemoveFinalizer(&preview, Finalizer)
		return ctrl.Result{}, r.Patch(ctx, &preview, patch)
	}
	return result, err
}

func (r *PreviewReconciler) reconcile(ctx context.Context, p *kuprav1alpha1.Preview) (ctrl.Result, error) {
	observedGeneration := p.Status.ObservedGeneration
	p.Status.ObservedGeneration = p.Generation
	nsName := previewNamespaceName(p.Namespace, p.Name)
	p.Status.Namespace = nsName

	namespaceExists, err := r.ensureAccess(ctx, p, nsName)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, settings, err := policy.Resolve(ctx, r.Client, p.Namespace)
	if notFound := (*policy.NotFoundError)(nil); errors.As(err, &notFound) {
		pending(p, ReasonPolicyNotFound, err.Error())
		return ctrl.Result{}, nil
	}
	if noRole := (*policy.NoTemplateRoleError)(nil); errors.As(err, &noRole) {
		pending(p, ReasonNoTemplateRole, err.Error())
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	accountName := nsName

	// An existing preview namespace follows its policy even while the app is missing or failing its checks.
	if namespaceExists {
		if reason, message, err := r.confine(ctx, p, nsName, accountName, settings); err != nil || reason != "" {
			if err != nil {
				return ctrl.Result{}, err
			}
			// Nothing kupra watches tells it when a conflicting namespace goes away, so check again later.
			pending(p, reason, message)
			return ctrl.Result{RequeueAfter: recheckInterval}, nil
		}
		// Requests that act on what's already running come before convergence, in the order ADR 10 gives them.
		applier, err := r.impersonate(accountName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if s, err := r.runningCapture(ctx, p); err != nil || s != nil {
			if err != nil {
				return ctrl.Result{}, err
			}
			// Changes to the spec wait until the capture has finished.
			p.Status.ObservedGeneration = observedGeneration
			return r.reconcileCapture(ctx, p, s, applier, nsName)
		}
		if resetRequested(p) {
			return r.reconcileReset(ctx, p, applier, nsName)
		}
	} else if resetRequested(p) {
		// There's nothing to reset yet.
		p.Status.LastHandledResetAt = p.Annotations[AnnotationResetRequestedAt]
	}

	var app kuprav1alpha1.PreviewApp
	if err := r.Get(ctx, client.ObjectKey{Namespace: p.Namespace, Name: p.Spec.AppRef.Name}, &app); err != nil {
		if apierrors.IsNotFound(err) {
			pending(p, ReasonAppNotFound, fmt.Sprintf("PreviewApp %s doesn't exist", p.Spec.AppRef.Name))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if c := apimeta.FindStatusCondition(app.Status.Conditions, meta.ReadyCondition); c == nil || c.ObservedGeneration != app.Generation || c.Status != metav1.ConditionTrue {
		message := fmt.Sprintf("PreviewApp %s hasn't been checked yet", app.Name)
		if c != nil && c.ObservedGeneration == app.Generation {
			message = fmt.Sprintf("PreviewApp %s failed its checks: %s", app.Name, c.Message)
		}
		pending(p, ReasonAppNotReady, message)
		return ctrl.Result{}, nil
	}

	// Render before creating anything, so a Preview with invalid parameters gets no namespace.
	compiled, errs := render.Compile(&app.Spec)
	if len(errs) > 0 {
		failed(p, ReasonRenderFailed, errs.ToAggregate().Error())
		return ctrl.Result{}, nil
	}
	rendered, err := compiled.Render(render.Preview{
		Name:        p.Name,
		Namespace:   nsName,
		Labels:      p.Labels,
		Annotations: p.Annotations,
	}, p.Spec.Parameters)
	if err != nil {
		failed(p, ReasonRenderFailed, err.Error())
		return ctrl.Result{}, nil
	}
	p.Status.Links = rendered.Links

	if !namespaceExists {
		if reason, message, err := r.confine(ctx, p, nsName, accountName, settings); err != nil || reason != "" {
			if err != nil {
				return ctrl.Result{}, err
			}
			// Nothing kupra watches tells it when a conflicting namespace goes away, so check again later.
			pending(p, reason, message)
			return ctrl.Result{RequeueAfter: recheckInterval}, nil
		}
		if err := r.bindNamespaceAccess(ctx, p, nsName); err != nil {
			return ctrl.Result{}, err
		}
	}

	applier, err := r.impersonate(accountName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// State goes first, so data and claims are in place before anything uses them.
	var states, workload []*unstructured.Unstructured
	for _, obj := range rendered.Objects {
		if _, ok := state.For(obj.GroupVersionKind().GroupKind()); ok && !obj.NotState {
			states = append(states, obj.Unstructured)
			continue
		}
		workload = append(workload, obj.Unstructured)
	}
	plan, reason, message, err := r.restoreState(ctx, p, applier, nsName, states)
	if err != nil {
		return ctrl.Result{}, err
	}
	switch reason {
	case "":
	case ReasonSnapshotNotFound, ReasonSnapshotNotReady:
		// The snapshot may still appear or become ready, and kupra watches PreviewSnapshots.
		pending(p, reason, message)
		return ctrl.Result{}, nil
	default:
		failed(p, reason, message)
		return ctrl.Result{}, nil
	}

	restored := plan.resources
	applied, err := applyAll(ctx, applier, restored, true)
	var restoreSettling bool
	if err == nil {
		restoreSettling, err = r.recordRestore(ctx, p, applier, plan)
	}
	if err == nil && !plan.done {
		p.Status.Inventory = unionInventory(applied, p.Status.Inventory)
		provisioning(p, "restoring state")
		stallAfterTimeout(p, &app, "restoring state")
		return ctrl.Result{RequeueAfter: startingInterval}, nil
	}
	if err == nil {
		var ready bool
		var message string
		ready, message, err = readiness(ctx, applier, restored)
		if err == nil && !ready {
			p.Status.Inventory = unionInventory(applied, p.Status.Inventory)
			provisioning(p, message)
			stallAfterTimeout(p, &app, message)
			return ctrl.Result{RequeueAfter: startingInterval}, nil
		}
	}
	if err == nil {
		var appliedWorkload []kuprav1alpha1.InventoryEntry
		appliedWorkload, err = applyAll(ctx, applier, workload, false)
		applied = append(applied, appliedWorkload...)
	}
	var retained []kuprav1alpha1.InventoryEntry
	if err == nil {
		retained, err = r.prune(ctx, applier, nsName, p.Status.Inventory, applied)
	}
	if err != nil {
		p.Status.Inventory = unionInventory(applied, p.Status.Inventory)
		// An object the API server rejects as invalid stays invalid until the template or parameters change, and those changes requeue the preview.
		if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
			failed(p, ReasonApplyFailed, err.Error())
			return ctrl.Result{RequeueAfter: runningInterval}, nil
		}
		starting(p, ReasonApplyFailed, err.Error())
		stallAfterTimeout(p, &app, err.Error())
		return ctrl.Result{}, err
	}
	p.Status.Inventory = unionInventory(applied, retained)

	ready, message, err := readiness(ctx, applier, workload)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ready {
		running(p)
		if started, err := r.startCapture(ctx, p); err != nil {
			return ctrl.Result{}, err
		} else if started {
			return ctrl.Result{RequeueAfter: nextStepInterval}, nil
		}
		if restoreSettling {
			return ctrl.Result{RequeueAfter: startingInterval}, nil
		}
		return ctrl.Result{RequeueAfter: runningInterval}, nil
	}
	starting(p, meta.ProgressingReason, message)
	stallAfterTimeout(p, &app, message)
	return ctrl.Result{RequeueAfter: startingInterval}, nil
}

// stallAfterTimeout marks a preview as stalled once it has been reconciling for longer than its app's timeout. kupra keeps trying regardless.
func stallAfterTimeout(p *kuprav1alpha1.Preview, app *kuprav1alpha1.PreviewApp, message string) {
	timeout := defaultTimeout
	if app.Spec.Timeout != nil {
		timeout = app.Spec.Timeout.Duration
	}
	if c := apimeta.FindStatusCondition(p.Status.Conditions, meta.ReconcilingCondition); c != nil && time.Since(c.LastTransitionTime.Time) > timeout {
		setCondition(&p.Status.Conditions, p.Generation, meta.StalledCondition, metav1.ConditionTrue, ReasonTimeout,
			fmt.Sprintf("not ready after %s: %s", timeout, message))
	}
}

// confine sets up the preview's namespace as its policy says: the namespace itself, the policy's objects in it, and the account templates are applied as. It returns a reason when the namespace can't be used yet.
func (r *PreviewReconciler) confine(ctx context.Context, p *kuprav1alpha1.Preview, namespace, account string, settings kuprav1alpha1.PreviewPolicySpec) (reason, message string, err error) {
	if reason, message, err := r.ensureNamespace(ctx, p, namespace, settings); err != nil || reason != "" {
		return reason, message, err
	}
	if err := r.applyPolicy(ctx, namespace, settings); err != nil {
		return "", "", err
	}
	return r.ensureApplyAccount(ctx, p, namespace, account, settings.Apply.ClusterRoleRef.Name)
}

// ensureNamespace creates or updates the preview's namespace. It returns a reason when the namespace can't be used yet.
func (r *PreviewReconciler) ensureNamespace(ctx context.Context, p *kuprav1alpha1.Preview, name string, settings kuprav1alpha1.PreviewPolicySpec) (reason, message string, err error) {
	var existing corev1.Namespace
	err = r.Get(ctx, client.ObjectKey{Name: name}, &existing)
	if apierrors.IsNotFound(err) {
		// Create rather than apply, so a namespace that appeared since the read is checked instead of taken over.
		existing = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{AnnotationPreviewUID: string(p.UID)}}}
		err = r.Create(ctx, &existing, client.FieldOwner(FieldManager))
		if apierrors.IsAlreadyExists(err) {
			err = r.APIReader.Get(ctx, client.ObjectKey{Name: name}, &existing)
		}
	}
	switch {
	case err != nil:
		return "", "", err
	case existing.Annotations[AnnotationPreviewUID] != string(p.UID):
		return ReasonNamespaceConflict, fmt.Sprintf("namespace %s already exists, and doesn't belong to this Preview", name), nil
	case !existing.DeletionTimestamp.IsZero():
		return ReasonNamespaceTerminating, fmt.Sprintf("namespace %s is still being deleted", name), nil
	}

	labels := map[string]string{}
	annotations := map[string]string{}
	if md := settings.NamespaceMetadata; md != nil {
		maps.Copy(labels, md.Labels)
		maps.Copy(annotations, md.Annotations)
	}
	labels[LabelApp] = p.Spec.AppRef.Name
	labels[LabelPreview] = p.Name
	labels[LabelPreviewNamespace] = p.Namespace
	annotations[AnnotationPreviewUID] = string(p.UID)

	// The resourceVersion makes the apply fail if the namespace changed since its owner was checked.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, ResourceVersion: existing.ResourceVersion, Labels: labels, Annotations: annotations}}
	return "", "", r.applyObject(ctx, r.Client, ns)
}

// applyPolicy creates the policy's network policies, quota and limit range in a preview namespace, and removes the ones it no longer has.
func (r *PreviewReconciler) applyPolicy(ctx context.Context, namespace string, settings kuprav1alpha1.PreviewPolicySpec) error {
	marker := map[string]string{LabelPolicyResource: "true"}

	want := map[string]bool{}
	for _, np := range settings.NetworkPolicies {
		labels := maps.Clone(np.Metadata.Labels)
		if labels == nil {
			labels = map[string]string{}
		}
		maps.Copy(labels, marker)
		obj := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: np.Metadata.Name, Namespace: namespace, Labels: labels, Annotations: np.Metadata.Annotations},
			Spec:       np.Spec,
		}
		if err := r.applyObject(ctx, r.Client, obj); err != nil {
			return err
		}
		want[np.Metadata.Name] = true
	}
	var existing networkingv1.NetworkPolicyList
	if err := r.APIReader.List(ctx, &existing, client.InNamespace(namespace), client.MatchingLabels(marker)); err != nil {
		return err
	}
	for _, np := range existing.Items {
		if !want[np.Name] {
			if err := client.IgnoreNotFound(r.Delete(ctx, &np)); err != nil {
				return err
			}
		}
	}

	quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: policyObjectName, Namespace: namespace, Labels: marker}}
	if settings.ResourceQuota != nil {
		quota.Spec = *settings.ResourceQuota
	}
	if err := r.applyOrDelete(ctx, quota, len(quota.Spec.Hard) > 0); err != nil {
		return err
	}
	limits := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: policyObjectName, Namespace: namespace, Labels: marker}}
	if settings.LimitRange != nil {
		limits.Spec = *settings.LimitRange
	}
	return r.applyOrDelete(ctx, limits, len(limits.Spec.Limits) > 0)
}

func (r *PreviewReconciler) applyOrDelete(ctx context.Context, obj client.Object, keep bool) error {
	if keep {
		return r.applyObject(ctx, r.Client, obj)
	}
	return client.IgnoreNotFound(r.Delete(ctx, obj))
}

// ensureApplyAccount sets up the service account templates are applied as, bound to the policy's role within the preview namespace. It lives in kupra's namespace, so the preview's own pods can't use it.
func (r *PreviewReconciler) ensureApplyAccount(ctx context.Context, p *kuprav1alpha1.Preview, namespace, name, role string) (reason, message string, err error) {
	account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:        name,
		Namespace:   r.Namespace,
		Labels:      map[string]string{LabelPreviewNamespace: p.Namespace},
		Annotations: map[string]string{AnnotationPreviewUID: string(p.UID)},
	}}
	if err := r.applyObject(ctx, r.Client, account); err != nil {
		return "", "", err
	}

	// A role kupra can't use leaves the existing binding in place, so a broken policy doesn't take running previews down with it.
	if _, reason, message, err := templateRole(ctx, r.Client, role); err != nil || reason != "" {
		return reason, message, err
	}

	var existing rbacv1.RoleBinding
	switch err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: templateBindingName}, &existing); {
	case apierrors.IsNotFound(err):
	case err != nil:
		return "", "", err
	case existing.RoleRef.Name != role:
		// A RoleBinding's roleRef can't be changed, so replace the binding when the policy's role changes.
		if err := client.IgnoreNotFound(r.Delete(ctx, &existing)); err != nil {
			return "", "", err
		}
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: templateBindingName, Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Namespace: r.Namespace, Name: name}},
	}
	return "", "", r.applyObject(ctx, r.Client, binding)
}

func (r *PreviewReconciler) impersonate(account string) (client.Client, error) {
	cfg := rest.CopyConfig(r.Config)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: serviceaccount.MakeUsername(r.Namespace, account)}
	return client.New(cfg, client.Options{Scheme: r.Scheme(), Mapper: r.Mapper})
}

// applyObject server-side applies a typed object as kupra.
func (r *PreviewReconciler) applyObject(ctx context.Context, c client.Client, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, r.Scheme())
	if err != nil {
		return err
	}
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}
	u := &unstructured.Unstructured{Object: content}
	u.SetGroupVersionKind(gvk)
	unstructured.RemoveNestedField(u.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(u.Object, "status")
	if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(u), client.FieldOwner(FieldManager), client.ForceOwnership); err != nil {
		return fmt.Errorf("applying %s %s: %w", gvk.Kind, obj.GetName(), err)
	}
	return nil
}

func applyAll(ctx context.Context, c client.Client, objs []*unstructured.Unstructured, isState bool) ([]kuprav1alpha1.InventoryEntry, error) {
	var applied []kuprav1alpha1.InventoryEntry
	var errs []error
	for _, obj := range objs {
		if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj.DeepCopy()), client.FieldOwner(FieldManager), client.ForceOwnership); err != nil {
			errs = append(errs, fmt.Errorf("applying %s %s: %w", obj.GetKind(), obj.GetName(), err))
			continue
		}
		applied = append(applied, kuprav1alpha1.InventoryEntry{TypedLocalObjectReference: refOf(obj), State: isState})
	}
	return applied, errors.Join(errs...)
}

// prune deletes resources that were applied before, but are no longer rendered. It keeps state, since it holds data the preview has built up, and returns it: it goes when the preview is reset or deleted.
func (r *PreviewReconciler) prune(ctx context.Context, c client.Client, namespace string, previous, current []kuprav1alpha1.InventoryEntry) ([]kuprav1alpha1.InventoryEntry, error) {
	var retained []kuprav1alpha1.InventoryEntry
	var errs []error
	for _, entry := range previous {
		ref := entry.TypedLocalObjectReference
		if slices.ContainsFunc(current, func(c kuprav1alpha1.InventoryEntry) bool { return sameObject(c.TypedLocalObjectReference, ref) }) {
			continue
		}
		if entry.State {
			entry.Retained = true
			retained = append(retained, entry)
			continue
		}
		mapping, err := r.Mapper.RESTMapping(schema.GroupKind{Group: refGroup(ref), Kind: ref.Kind})
		if apimeta.IsNoMatchError(err) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(mapping.GroupVersionKind)
		u.SetNamespace(namespace)
		u.SetName(ref.Name)
		if err := c.Delete(ctx, u, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			errs = append(errs, fmt.Errorf("deleting %s %s: %w", ref.Kind, ref.Name, err))
		}
	}
	return retained, errors.Join(errs...)
}

// readiness judges applied resources: those of a kind kupra supports as state by that kind's own rules, and anything else with kstatus, the way Flux does.
func readiness(ctx context.Context, c client.Client, objs []*unstructured.Unstructured) (bool, string, error) {
	var waiting []string
	for _, obj := range objs {
		live := &unstructured.Unstructured{}
		live.SetGroupVersionKind(obj.GroupVersionKind())
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), live)
		if client.IgnoreNotFound(err) != nil {
			return false, "", err
		}
		if kind, ok := state.For(obj.GroupVersionKind().GroupKind()); ok {
			if apierrors.IsNotFound(err) {
				live = nil
			}
			if !kind.Ready(live) {
				waiting = append(waiting, fmt.Sprintf("%s %s", obj.GetKind(), obj.GetName()))
			}
			continue
		}
		if err != nil {
			return false, "", err
		}
		res, err := status.Compute(live)
		if err != nil {
			return false, "", err
		}
		if res.Status != status.CurrentStatus {
			waiting = append(waiting, fmt.Sprintf("%s %s: %s", obj.GetKind(), obj.GetName(), res.Message))
		}
	}
	if len(waiting) > 0 {
		return false, "waiting for " + strings.Join(waiting, "; "), nil
	}
	return true, "", nil
}

// reconcileDelete removes the preview's namespace and apply account. It reports whether cleanup is done, so the finalizer can go.
func (r *PreviewReconciler) reconcileDelete(ctx context.Context, p *kuprav1alpha1.Preview) (ctrl.Result, bool, error) {
	nsName := previewNamespaceName(p.Namespace, p.Name)

	// A capture that has started finishes first.
	if s, err := r.runningCapture(ctx, p); err != nil || s != nil {
		if err != nil {
			return ctrl.Result{}, false, err
		}
		applier, err := r.impersonate(nsName)
		if err != nil {
			return ctrl.Result{}, false, err
		}
		result, err := r.reconcileCapture(ctx, p, s, applier, nsName)
		return result, false, err
	}

	// A preview that's going away no longer needs the snapshot it restores from.
	if err := r.releaseSnapshots(ctx, p, ""); err != nil {
		return ctrl.Result{}, false, err
	}

	p.Status.Phase = kuprav1alpha1.PreviewTerminating
	setCondition(&p.Status.Conditions, p.Generation, meta.ReadyCondition, metav1.ConditionFalse, ReasonTerminating, "")
	setCondition(&p.Status.Conditions, p.Generation, meta.ReconcilingCondition, metav1.ConditionTrue, ReasonTerminating, fmt.Sprintf("deleting namespace %s", nsName))
	apimeta.RemoveStatusCondition(&p.Status.Conditions, meta.StalledCondition)

	var ns corev1.Namespace
	gone := false
	switch err := r.Get(ctx, client.ObjectKey{Name: nsName}, &ns); {
	case apierrors.IsNotFound(err):
		gone = true
	case err != nil:
		return ctrl.Result{}, false, err
	case ns.Annotations[AnnotationPreviewUID] != string(p.UID):
		gone = true
	case ns.DeletionTimestamp.IsZero():
		// The UID keeps a stale read from deleting a namespace that replaced the preview's.
		if err := client.IgnoreNotFound(r.Delete(ctx, &ns, client.Preconditions{UID: &ns.UID})); err != nil {
			return ctrl.Result{}, false, err
		}
	}
	if !gone {
		return ctrl.Result{RequeueAfter: startingInterval}, false, nil
	}
	if gone, err := r.removeRestoreCopies(ctx, p, nsName); err != nil || !gone {
		return ctrl.Result{RequeueAfter: startingInterval}, false, err
	}

	// Another Preview's account can have the same name, when their namespace names collide.
	var account corev1.ServiceAccount
	switch err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: nsName}, &account); {
	case apierrors.IsNotFound(err):
	case err != nil:
		return ctrl.Result{}, false, err
	case account.Annotations[AnnotationPreviewUID] == string(p.UID):
		if err := client.IgnoreNotFound(r.Delete(ctx, &account, client.Preconditions{UID: &account.UID})); err != nil {
			return ctrl.Result{}, false, err
		}
	}
	return ctrl.Result{}, true, nil
}

func pending(p *kuprav1alpha1.Preview, reason, message string) {
	p.Status.Phase = kuprav1alpha1.PreviewPending
	setCondition(&p.Status.Conditions, p.Generation, meta.ReadyCondition, metav1.ConditionFalse, reason, message)
	setCondition(&p.Status.Conditions, p.Generation, meta.StalledCondition, metav1.ConditionTrue, reason, message)
	apimeta.RemoveStatusCondition(&p.Status.Conditions, meta.ReconcilingCondition)
}

func failed(p *kuprav1alpha1.Preview, reason, message string) {
	pending(p, reason, message)
	p.Status.Phase = kuprav1alpha1.PreviewFailed
}

func provisioning(p *kuprav1alpha1.Preview, message string) {
	starting(p, meta.ProgressingReason, message)
	p.Status.Phase = kuprav1alpha1.PreviewProvisioning
}

func starting(p *kuprav1alpha1.Preview, reason, message string) {
	p.Status.Phase = kuprav1alpha1.PreviewStarting
	setCondition(&p.Status.Conditions, p.Generation, meta.ReadyCondition, metav1.ConditionFalse, reason, message)
	setCondition(&p.Status.Conditions, p.Generation, meta.ReconcilingCondition, metav1.ConditionTrue, meta.ProgressingReason, message)
	apimeta.RemoveStatusCondition(&p.Status.Conditions, meta.StalledCondition)
}

func running(p *kuprav1alpha1.Preview) {
	p.Status.Phase = kuprav1alpha1.PreviewRunning
	setCondition(&p.Status.Conditions, p.Generation, meta.ReadyCondition, metav1.ConditionTrue, meta.SucceededReason, "")
	apimeta.RemoveStatusCondition(&p.Status.Conditions, meta.ReconcilingCondition)
	apimeta.RemoveStatusCondition(&p.Status.Conditions, meta.StalledCondition)
}

func refOf(obj *unstructured.Unstructured) corev1.TypedLocalObjectReference {
	ref := corev1.TypedLocalObjectReference{Kind: obj.GetKind(), Name: obj.GetName()}
	if group := obj.GroupVersionKind().Group; group != "" {
		ref.APIGroup = &group
	}
	return ref
}

func refGroup(ref corev1.TypedLocalObjectReference) string {
	if ref.APIGroup == nil {
		return ""
	}
	return *ref.APIGroup
}

func sameObject(a, b corev1.TypedLocalObjectReference) bool {
	return refGroup(a) == refGroup(b) && a.Kind == b.Kind && a.Name == b.Name
}

// unionInventory adds the entries of b that a doesn't have to a.
func unionInventory(a, b []kuprav1alpha1.InventoryEntry) []kuprav1alpha1.InventoryEntry {
	out := slices.Clone(a)
	for _, entry := range b {
		if !slices.ContainsFunc(out, func(e kuprav1alpha1.InventoryEntry) bool {
			return sameObject(e.TypedLocalObjectReference, entry.TypedLocalObjectReference)
		}) {
			out = append(out, entry)
		}
	}
	return out
}

const (
	appRefIndex = ".spec.appRef.name"
	sourceIndex = ".spec.source.previewRef.name"
)

func (r *PreviewReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &kuprav1alpha1.Preview{}, appRefIndex, func(obj client.Object) []string {
		return []string{obj.(*kuprav1alpha1.Preview).Spec.AppRef.Name}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(ctx, &kuprav1alpha1.PreviewSnapshot{}, sourceIndex, func(obj client.Object) []string {
		return []string{obj.(*kuprav1alpha1.PreviewSnapshot).Spec.Source.PreviewRef.Name}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuprav1alpha1.Preview{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			predicate.LabelChangedPredicate{},
			predicate.AnnotationChangedPredicate{},
			deletingPredicate,
		))).
		Watches(&kuprav1alpha1.PreviewApp{}, handler.EnqueueRequestsFromMapFunc(r.previewsOfApp)).
		Watches(&kuprav1alpha1.PreviewPolicy{}, handler.EnqueueRequestsFromMapFunc(r.allPreviews)).
		Watches(&kuprav1alpha1.PreviewSnapshot{}, handler.EnqueueRequestsFromMapFunc(r.previewsForSnapshot)).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.previewsForNamespace)).
		Watches(&rbacv1.RoleBinding{}, handler.EnqueueRequestsFromMapFunc(r.previewsForAdminBinding)).
		Complete(r)
}

var deletingPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		return !e.ObjectNew.GetDeletionTimestamp().IsZero()
	},
}

func (r *PreviewReconciler) previewsOfApp(ctx context.Context, app client.Object) []reconcile.Request {
	return r.previews(ctx, client.InNamespace(app.GetNamespace()), client.MatchingFields{appRefIndex: app.GetName()})
}

func (r *PreviewReconciler) allPreviews(ctx context.Context, _ client.Object) []reconcile.Request {
	return r.previews(ctx)
}

// previewsForNamespace maps a preview namespace to its Preview, and an app namespace, whose policy may have changed, to its Previews.
func (r *PreviewReconciler) previewsForNamespace(ctx context.Context, ns client.Object) []reconcile.Request {
	owner, ok := ns.GetLabels()[LabelPreviewNamespace]
	if !ok {
		return r.previews(ctx, client.InNamespace(ns.GetName()))
	}
	var requests []reconcile.Request
	for _, req := range r.previews(ctx, client.InNamespace(owner)) {
		if previewNamespaceName(req.Namespace, req.Name) == ns.GetName() {
			requests = append(requests, req)
		}
	}
	return requests
}

func (r *PreviewReconciler) previews(ctx context.Context, opts ...client.ListOption) []reconcile.Request {
	var previews kuprav1alpha1.PreviewList
	if err := r.List(ctx, &previews, opts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "listing Previews")
		return nil
	}
	requests := make([]reconcile.Request, len(previews.Items))
	for i, p := range previews.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&p)}
	}
	return requests
}

// previewsForSnapshot maps a PreviewSnapshot to the preview it captures, and to the previews in its namespace that restore from a snapshot, since it may be the one they're waiting for.
func (r *PreviewReconciler) previewsForSnapshot(ctx context.Context, obj client.Object) []reconcile.Request {
	s, ok := obj.(*kuprav1alpha1.PreviewSnapshot)
	if !ok {
		return nil
	}
	var previews kuprav1alpha1.PreviewList
	if err := r.List(ctx, &previews, client.InNamespace(s.Namespace)); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "listing Previews")
		return nil
	}
	var requests []reconcile.Request
	for _, p := range previews.Items {
		if p.Name == s.Spec.Source.PreviewRef.Name || p.Spec.RestoreFrom != nil {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&p)})
		}
	}
	return requests
}
