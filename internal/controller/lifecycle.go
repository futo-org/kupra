package controller

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/names"
	"github.com/futo-org/kupra/internal/state"
)

const (
	// AnnotationResetRequestedAt requests a reset whenever it's set to a new value.
	AnnotationResetRequestedAt = "kupra.futo.tech/reset-requested-at"

	// ConditionRestored is true once a preview's state has started, empty or from a snapshot.
	ConditionRestored = "Restored"

	// captureTimeout is how long a capture may take before it fails.
	captureTimeout = 30 * time.Minute

	ReasonCapturing        = "Capturing"
	ReasonResetting        = "Resetting"
	ReasonSnapshotNotFound = "SnapshotNotFound"
	ReasonSnapshotNotReady = "SnapshotNotReady"
	ReasonRestoreFailed    = "RestoreFailed"
	ReasonCaptureFailed    = "CaptureFailed"
	ReasonWaiting          = "Waiting"
)

// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewsnapshots,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewsnapshots/status,verbs=get;update;patch

// snapshotsOf lists a preview's unfinished PreviewSnapshots, oldest first, including those being deleted, whose started captures still finish.
func (r *PreviewReconciler) snapshotsOf(ctx context.Context, p *kuprav1alpha1.Preview) ([]kuprav1alpha1.PreviewSnapshot, error) {
	var list kuprav1alpha1.PreviewSnapshotList
	if err := r.List(ctx, &list, client.InNamespace(p.Namespace), client.MatchingFields{sourceIndex: p.Name}); err != nil {
		return nil, err
	}
	var out []kuprav1alpha1.PreviewSnapshot
	for _, s := range list.Items {
		if s.Status.CompletionTime == nil {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b kuprav1alpha1.PreviewSnapshot) int {
		if c := a.CreationTimestamp.Compare(b.CreationTimestamp.Time); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// runningCapture returns the capture of a preview that has started, if there is one.
func (r *PreviewReconciler) runningCapture(ctx context.Context, p *kuprav1alpha1.Preview) (*kuprav1alpha1.PreviewSnapshot, error) {
	snapshots, err := r.snapshotsOf(ctx, p)
	if err != nil {
		return nil, err
	}
	for _, s := range snapshots {
		if s.Status.StartTime != nil {
			return &s, nil
		}
	}
	return nil, nil
}

// startCapture starts the next capture of a running preview, and reports whether it did.
func (r *PreviewReconciler) startCapture(ctx context.Context, p *kuprav1alpha1.Preview) (bool, error) {
	snapshots, err := r.snapshotsOf(ctx, p)
	if err != nil {
		return false, err
	}
	s := nextCapture(snapshots)
	if s == nil {
		return false, nil
	}
	// The lock fails this if the snapshot's finalizer was removed since it was read.
	patch := client.MergeFromWithOptions(s.DeepCopy(), client.MergeFromWithOptimisticLock{})
	now := metav1.Now()
	s.Status.StartTime = &now
	s.Status.AppRef = &meta.LocalObjectReference{Name: p.Spec.AppRef.Name}
	s.Status.Parameters = p.Spec.Parameters
	s.Status.Phase = kuprav1alpha1.PreviewSnapshotInProgress
	setCondition(&s.Status.Conditions, s.Generation, meta.ReadyCondition, metav1.ConditionFalse, ReasonCapturing, "")
	setCondition(&s.Status.Conditions, s.Generation, meta.ReconcilingCondition, metav1.ConditionTrue, ReasonCapturing, fmt.Sprintf("capturing Preview %s", p.Name))
	return true, client.IgnoreNotFound(r.Status().Patch(ctx, s, patch))
}

// nextCapture returns the oldest snapshot that isn't being deleted, once it has kupra's finalizer, so that deleting it waits for its capture.
func nextCapture(snapshots []kuprav1alpha1.PreviewSnapshot) *kuprav1alpha1.PreviewSnapshot {
	i := slices.IndexFunc(snapshots, func(s kuprav1alpha1.PreviewSnapshot) bool { return s.DeletionTimestamp.IsZero() })
	if i < 0 || !controllerutil.ContainsFinalizer(&snapshots[i], Finalizer) {
		return nil
	}
	return &snapshots[i]
}

// reconcileCapture continues a running capture, holding back template changes so the snapshot matches the parameters it records.
func (r *PreviewReconciler) reconcileCapture(ctx context.Context, p *kuprav1alpha1.Preview, s *kuprav1alpha1.PreviewSnapshot, applier client.Client, namespace string) (ctrl.Result, error) {
	// A capture can't be moved into a namespace that's being deleted.
	var ns corev1.Namespace
	if err := r.APIReader.Get(ctx, client.ObjectKey{Name: s.Namespace}, &ns); err != nil {
		return ctrl.Result{}, err
	}
	if !ns.DeletionTimestamp.IsZero() {
		return ctrl.Result{RequeueAfter: nextStepInterval}, r.finishSnapshot(ctx, s, nil, fmt.Sprintf("namespace %s is being deleted", s.Namespace))
	}
	if s.Spec.Mode != kuprav1alpha1.SnapshotOnline {
		gone, err := r.remove(ctx, applier, namespace, workloadEntries(p.Status.Inventory))
		if err != nil {
			return ctrl.Result{}, err
		}
		if !gone {
			stopping(p, ReasonCapturing, fmt.Sprintf("stopping for PreviewSnapshot %s", s.Name))
			return ctrl.Result{RequeueAfter: startingInterval}, nil
		}
		stopped(p, ReasonCapturing, fmt.Sprintf("stopped for PreviewSnapshot %s", s.Name))
	}

	var captures []kuprav1alpha1.Capture
	var waiting []string
	owner := metav1.NewControllerRef(s, kuprav1alpha1.GroupVersion.WithKind("PreviewSnapshot"))
	for _, entry := range p.Status.Inventory {
		if !entry.State {
			continue
		}
		gk := schema.GroupKind{Group: refGroup(entry.TypedLocalObjectReference), Kind: entry.Kind}
		kind, ok := state.For(gk)
		if !ok {
			continue
		}
		live, err := r.getLive(ctx, applier, gk, namespace, entry.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if live == nil {
			continue
		}
		req := state.CaptureRequest{
			Resource:  live,
			Namespace: s.Namespace,
			Names:     names.ForResource(s.Name, s.UID, gk, entry.Name),
			Labels:    map[string]string{LabelSnapshotUID: string(s.UID)},
		}
		resp, err := r.runKind(ctx, func(observed state.Observed) state.Response {
			req.Observed = observed
			return kind.Capture(req)
		}, s.Namespace, owner)
		if err != nil {
			return ctrl.Result{}, err
		}
		if resp.Failed != "" {
			return ctrl.Result{RequeueAfter: nextStepInterval}, r.finishSnapshot(ctx, s, nil, fmt.Sprintf("capturing %s %s: %s", entry.Kind, entry.Name, resp.Failed))
		}
		if !resp.Done {
			w := entry.Kind + " " + entry.Name
			if resp.Waiting != "" {
				w += ": " + resp.Waiting
			}
			waiting = append(waiting, w)
			continue
		}
		captures = append(captures, kuprav1alpha1.Capture{Group: gk.Group, Kind: gk.Kind, Name: entry.Name, Data: resp.Capture.Data})
	}
	if len(waiting) > 0 {
		// An Offline capture keeps the app stopped, so one that can't finish fails rather than wait forever.
		if time.Since(s.Status.StartTime.Time) > captureTimeout {
			return ctrl.Result{RequeueAfter: nextStepInterval}, r.finishSnapshot(ctx, s, nil, fmt.Sprintf("not finished after %s: waiting for %s", captureTimeout, strings.Join(waiting, "; ")))
		}
		return ctrl.Result{RequeueAfter: startingInterval}, nil
	}
	// The app starts again on the next reconciliation.
	return ctrl.Result{RequeueAfter: nextStepInterval}, r.finishSnapshot(ctx, s, captures, "")
}

// finishSnapshot records a capture's result, which succeeded if failure is empty.
func (r *PreviewReconciler) finishSnapshot(ctx context.Context, s *kuprav1alpha1.PreviewSnapshot, captures []kuprav1alpha1.Capture, failure string) error {
	patch := client.MergeFromWithOptions(s.DeepCopy(), client.MergeFromWithOptimisticLock{})
	now := metav1.Now()
	s.Status.CompletionTime = &now
	s.Status.ObservedGeneration = s.Generation
	apimeta.RemoveStatusCondition(&s.Status.Conditions, meta.ReconcilingCondition)
	if failure != "" {
		s.Status.Phase = kuprav1alpha1.PreviewSnapshotFailed
		setCondition(&s.Status.Conditions, s.Generation, meta.ReadyCondition, metav1.ConditionFalse, ReasonCaptureFailed, failure)
		setCondition(&s.Status.Conditions, s.Generation, meta.StalledCondition, metav1.ConditionTrue, ReasonCaptureFailed, failure)
	} else {
		s.Status.Phase = kuprav1alpha1.PreviewSnapshotSucceeded
		s.Status.ReadyToUse = true
		s.Status.Captures = captures
		setCondition(&s.Status.Conditions, s.Generation, meta.ReadyCondition, metav1.ConditionTrue, meta.SucceededReason, "")
	}
	return client.IgnoreNotFound(r.Status().Patch(ctx, s, patch))
}

// resetRequested reports whether a preview's reset annotation has an unhandled value.
func resetRequested(p *kuprav1alpha1.Preview) bool {
	requested := p.Annotations[AnnotationResetRequestedAt]
	return requested != "" && requested != p.Status.LastHandledResetAt
}

// reconcileReset removes a preview's app, state and restore copies, then records the reset as handled.
func (r *PreviewReconciler) reconcileReset(ctx context.Context, p *kuprav1alpha1.Preview, applier client.Client, namespace string) (ctrl.Result, error) {
	gone, err := r.remove(ctx, applier, namespace, workloadEntries(p.Status.Inventory))
	if err == nil && gone {
		// State goes once its users have.
		gone, err = r.remove(ctx, applier, namespace, stateEntries(p.Status.Inventory))
	}
	if err == nil && gone {
		gone, err = r.finishReset(ctx, p, namespace)
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if !gone {
		stopping(p, ReasonResetting, "removing app and state")
		return ctrl.Result{RequeueAfter: startingInterval}, nil
	}
	provisioning(p, "reset")
	return ctrl.Result{RequeueAfter: nextStepInterval}, nil
}

// finishReset removes a preview's restore copies and snapshot holds, then records the reset as handled. It reports whether it has.
func (r *PreviewReconciler) finishReset(ctx context.Context, p *kuprav1alpha1.Preview, namespace string) (bool, error) {
	if gone, err := r.removeRestoreCopies(ctx, p, namespace); err != nil || !gone {
		return false, err
	}
	if err := r.releaseSnapshots(ctx, p); err != nil {
		return false, err
	}
	p.Status.LastHandledResetAt = p.Annotations[AnnotationResetRequestedAt]
	p.Status.Inventory = nil
	p.Status.RestoredFrom = nil
	apimeta.RemoveStatusCondition(&p.Status.Conditions, ConditionRestored)
	return true, nil
}

// removeRestoreCopies deletes the VolumeSnapshots and VolumeSnapshotContents a preview was restored from, and reports whether they're gone.
func (r *PreviewReconciler) removeRestoreCopies(ctx context.Context, p *kuprav1alpha1.Preview, namespace string) (bool, error) {
	selector := client.MatchingLabels{LabelPreviewUID: string(p.UID)}
	var snapshots snapshotv1.VolumeSnapshotList
	if err := r.APIReader.List(ctx, &snapshots, client.InNamespace(namespace), selector); err != nil && !apimeta.IsNoMatchError(err) {
		return false, err
	}
	var contents snapshotv1.VolumeSnapshotContentList
	if err := r.APIReader.List(ctx, &contents, selector); err != nil && !apimeta.IsNoMatchError(err) {
		return false, err
	}
	for i := range snapshots.Items {
		if err := client.IgnoreNotFound(r.Delete(ctx, &snapshots.Items[i])); err != nil {
			return false, err
		}
	}
	for i := range contents.Items {
		if err := client.IgnoreNotFound(r.Delete(ctx, &contents.Items[i])); err != nil {
			return false, err
		}
	}
	return len(snapshots.Items) == 0 && len(contents.Items) == 0, nil
}

// chooseSnapshot finds the PreviewSnapshot a preview restores from, or a reason there's none yet.
func (r *PreviewReconciler) chooseSnapshot(ctx context.Context, p *kuprav1alpha1.Preview) (*kuprav1alpha1.PreviewSnapshot, string, string, error) {
	from := p.Spec.RestoreFrom
	if from.SnapshotRef != nil {
		var s kuprav1alpha1.PreviewSnapshot
		if err := r.Get(ctx, client.ObjectKey{Namespace: p.Namespace, Name: from.SnapshotRef.Name}, &s); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, ReasonSnapshotNotFound, fmt.Sprintf("PreviewSnapshot %s not found", from.SnapshotRef.Name), nil
			}
			return nil, "", "", err
		}
		if !s.Status.ReadyToUse || !s.DeletionTimestamp.IsZero() {
			return nil, ReasonSnapshotNotReady, fmt.Sprintf("PreviewSnapshot %s is not ready", s.Name), nil
		}
		return &s, "", "", nil
	}

	selector, err := metav1.LabelSelectorAsSelector(from.SnapshotSelector)
	if err != nil {
		return nil, ReasonSnapshotNotFound, err.Error(), nil
	}
	var list kuprav1alpha1.PreviewSnapshotList
	if err := r.List(ctx, &list, client.InNamespace(p.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, "", "", err
	}
	var newest *kuprav1alpha1.PreviewSnapshot
	for i, s := range list.Items {
		if !s.Status.ReadyToUse || !s.DeletionTimestamp.IsZero() {
			continue
		}
		if newest == nil || newerSnapshot(&s, newest) {
			newest = &list.Items[i]
		}
	}
	if newest == nil {
		return nil, ReasonSnapshotNotFound, fmt.Sprintf("no ready PreviewSnapshot matches %s", selectorString(selector)), nil
	}
	return newest, "", "", nil
}

func newerSnapshot(a, b *kuprav1alpha1.PreviewSnapshot) bool {
	at, bt := a.CreationTimestamp, b.CreationTimestamp
	if a.Status.CompletionTime != nil && b.Status.CompletionTime != nil {
		at, bt = *a.Status.CompletionTime, *b.Status.CompletionTime
	}
	if !at.Equal(&bt) {
		return bt.Before(&at)
	}
	return a.Name > b.Name
}

func selectorString(s labels.Selector) string {
	if s.Empty() {
		return "an empty selector"
	}
	return s.String()
}

// findCapture returns a snapshot's capture of a state resource, by kind and name.
func findCapture(s *kuprav1alpha1.PreviewSnapshot, gk schema.GroupKind, name string) *kuprav1alpha1.Capture {
	for i, c := range s.Status.Captures {
		if c.Group == gk.Group && c.Kind == gk.Kind && c.Name == name {
			return &s.Status.Captures[i]
		}
	}
	return nil
}

// unmatchedCaptures lists a snapshot's captures that match no state resource.
func unmatchedCaptures(s *kuprav1alpha1.PreviewSnapshot, states []*unstructured.Unstructured) []string {
	var out []string
	for _, c := range s.Status.Captures {
		if !slices.ContainsFunc(states, func(obj *unstructured.Unstructured) bool {
			gk := obj.GroupVersionKind().GroupKind()
			return gk.Group == c.Group && gk.Kind == c.Kind && obj.GetName() == c.Name
		}) {
			out = append(out, c.Kind+" "+c.Name)
		}
	}
	return out
}

// remove deletes the objects in entries, and reports whether they're gone.
func (r *PreviewReconciler) remove(ctx context.Context, c client.Client, namespace string, entries []kuprav1alpha1.InventoryEntry) (bool, error) {
	gone := true
	for _, entry := range entries {
		gk := schema.GroupKind{Group: refGroup(entry.TypedLocalObjectReference), Kind: entry.Kind}
		live, err := r.getLive(ctx, c, gk, namespace, entry.Name)
		if err != nil {
			return false, err
		}
		if live == nil {
			continue
		}
		gone = false
		if live.GetDeletionTimestamp().IsZero() {
			if err := client.IgnoreNotFound(c.Delete(ctx, live, client.PropagationPolicy(metav1.DeletePropagationForeground))); err != nil {
				return false, fmt.Errorf("deleting %s %s: %w", entry.Kind, entry.Name, err)
			}
		}
	}
	return gone, nil
}

// getLive reads an object, or returns nil if it or its kind doesn't exist.
func (r *PreviewReconciler) getLive(ctx context.Context, c client.Client, gk schema.GroupKind, namespace, name string) (*unstructured.Unstructured, error) {
	mapping, err := r.Mapper.RESTMapping(gk)
	if apimeta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(mapping.GroupVersionKind)
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, live); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return live, nil
}

func workloadEntries(inventory []kuprav1alpha1.InventoryEntry) []kuprav1alpha1.InventoryEntry {
	return slices.DeleteFunc(slices.Clone(inventory), func(e kuprav1alpha1.InventoryEntry) bool { return e.State })
}

func stateEntries(inventory []kuprav1alpha1.InventoryEntry) []kuprav1alpha1.InventoryEntry {
	return slices.DeleteFunc(slices.Clone(inventory), func(e kuprav1alpha1.InventoryEntry) bool { return !e.State })
}

func stopping(p *kuprav1alpha1.Preview, reason, message string) {
	p.Status.Phase = kuprav1alpha1.PreviewStopping
	setCondition(&p.Status.Conditions, p.Generation, meta.ReadyCondition, metav1.ConditionFalse, reason, message)
	setCondition(&p.Status.Conditions, p.Generation, meta.ReconcilingCondition, metav1.ConditionTrue, reason, message)
	apimeta.RemoveStatusCondition(&p.Status.Conditions, meta.StalledCondition)
}

func stopped(p *kuprav1alpha1.Preview, reason, message string) {
	stopping(p, reason, message)
	p.Status.Phase = kuprav1alpha1.PreviewStopped
}

// restorePlan is how a preview's state starts.
type restorePlan struct {
	// resources are the state resources to apply.
	resources []*unstructured.Unstructured
	// done is set when every state resource can be applied.
	done bool
	// summary says how the state started, on a preview's first start after creation or reset.
	summary      string
	fromSnapshot bool
}

// restoreState returns how each state resource starts, or a reason the preview can't go on. Until the state has started, missing resources are restored from the preview's snapshot. After that, new ones start empty.
func (r *PreviewReconciler) restoreState(ctx context.Context, p *kuprav1alpha1.Preview, applier client.Client, namespace string, states []*unstructured.Unstructured) (plan *restorePlan, reason, message string, err error) {
	var resources []*unstructured.Unstructured
	done := true
	restoring := !apimeta.IsStatusConditionTrue(p.Status.Conditions, ConditionRestored)
	var snapshot *kuprav1alpha1.PreviewSnapshot
	if restoring {
		// A restore that has started keeps its snapshot, even if restoreFrom changes. The hold records the choice even if its status write was lost.
		name := ""
		if p.Status.RestoredFrom != nil {
			name = p.Status.RestoredFrom.Name
		}
		if name == "" {
			held, err := r.heldSnapshot(ctx, p)
			if err != nil {
				return nil, "", "", err
			}
			name = held
		}
		if name == "" && p.Spec.RestoreFrom != nil {
			s, reason, message, err := r.chooseSnapshot(ctx, p)
			if err != nil || reason != "" {
				return nil, reason, message, err
			}
			name = s.Name
		}
		if name != "" {
			// Hold the snapshot before restoring anything from it.
			s, reason, message, err := r.holdSnapshot(ctx, p, name)
			if err != nil || reason != "" {
				return nil, reason, message, err
			}
			p.Status.RestoredFrom = &meta.LocalObjectReference{Name: s.Name}
			snapshot = s
		}
	}

	var empty []string
	for _, obj := range states {
		gk := obj.GroupVersionKind().GroupKind()
		kind, _ := state.For(gk)
		live, err := r.getLive(ctx, applier, gk, namespace, obj.GetName())
		if err != nil {
			return nil, "", "", err
		}
		req := state.RestoreRequest{
			Resource: obj,
			Live:     live,
			Names:    names.ForResource(p.Name, p.UID, gk, obj.GetName()),
			Labels:   map[string]string{LabelPreviewUID: string(p.UID)},
		}
		if snapshot != nil {
			c := findCapture(snapshot, gk, obj.GetName())
			switch {
			case c == nil:
				empty = append(empty, obj.GetKind()+" "+obj.GetName())
			case live == nil:
				req.From = &state.Source{Namespace: snapshot.Namespace, Data: c.Data}
			}
		}
		resp, err := r.runKind(ctx, func(observed state.Observed) state.Response {
			req.Observed = observed
			return kind.Restore(req)
		}, "", nil)
		if err != nil {
			return nil, "", "", err
		}
		if resp.Failed != "" {
			return nil, ReasonRestoreFailed, fmt.Sprintf("restoring %s %s: %s", obj.GetKind(), obj.GetName(), resp.Failed), nil
		}
		if !resp.Done {
			done = false
			continue
		}
		resources = append(resources, resp.Resource)
	}

	plan = &restorePlan{resources: resources, done: done}
	if restoring && done {
		plan.fromSnapshot = snapshot != nil
		message := "started empty"
		if snapshot != nil {
			message = "restored from PreviewSnapshot " + snapshot.Name
			if len(empty) > 0 {
				message += ", without captures of " + strings.Join(empty, ", ")
			}
			if unmatched := unmatchedCaptures(snapshot, states); len(unmatched) > 0 {
				message += ", ignoring captures of " + strings.Join(unmatched, ", ")
			}
		}
		plan.summary = message
	}
	return plan, "", "", nil
}

// recordRestore marks a preview's state as restored once its resources exist and no longer need the snapshot, and reports whether that's still pending.
func (r *PreviewReconciler) recordRestore(ctx context.Context, p *kuprav1alpha1.Preview, applier client.Client, plan *restorePlan) (bool, error) {
	if plan.summary == "" {
		return false, nil
	}
	if plan.fromSnapshot {
		var waiting []string
		for _, obj := range plan.resources {
			gk := obj.GroupVersionKind().GroupKind()
			kind, _ := state.For(gk)
			live, err := r.getLive(ctx, applier, gk, obj.GetNamespace(), obj.GetName())
			if err != nil {
				return false, err
			}
			if live == nil || !kind.Restored(live) {
				waiting = append(waiting, obj.GetKind()+" "+obj.GetName())
			}
		}
		if len(waiting) > 0 {
			setCondition(&p.Status.Conditions, p.Generation, ConditionRestored, metav1.ConditionFalse, meta.ProgressingReason, "waiting for "+strings.Join(waiting, ", ")+" to be restored")
			return true, nil
		}
	}
	if err := r.releaseSnapshots(ctx, p); err != nil {
		return false, err
	}
	setCondition(&p.Status.Conditions, p.Generation, ConditionRestored, metav1.ConditionTrue, meta.SucceededReason, plan.summary)
	return false, nil
}

// RestoreFinalizerPrefix, followed by a Preview's UID, holds a PreviewSnapshot that preview restores from.
const RestoreFinalizerPrefix = "kupra.futo.tech/restore-"

func restoreFinalizer(p *kuprav1alpha1.Preview) string {
	return RestoreFinalizerPrefix + string(p.UID)
}

// holdSnapshot adds the preview's finalizer to a snapshot, or returns a reason it can't. The API server refuses new finalizers on objects being deleted, and the lock refuses the write if the snapshot changed since it was read.
func (r *PreviewReconciler) holdSnapshot(ctx context.Context, p *kuprav1alpha1.Preview, name string) (*kuprav1alpha1.PreviewSnapshot, string, string, error) {
	var s kuprav1alpha1.PreviewSnapshot
	if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: p.Namespace, Name: name}, &s); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ReasonSnapshotNotFound, fmt.Sprintf("PreviewSnapshot %s not found", name), nil
		}
		return nil, "", "", err
	}
	finalizer := restoreFinalizer(p)
	if controllerutil.ContainsFinalizer(&s, finalizer) {
		return &s, "", "", nil
	}
	if !s.Status.ReadyToUse || !s.DeletionTimestamp.IsZero() {
		return nil, ReasonSnapshotNotReady, fmt.Sprintf("PreviewSnapshot %s is not ready", name), nil
	}
	patch := client.MergeFromWithOptions(s.DeepCopy(), client.MergeFromWithOptimisticLock{})
	controllerutil.AddFinalizer(&s, finalizer)
	if err := r.Patch(ctx, &s, patch); err != nil {
		return nil, "", "", err
	}
	return &s, "", "", nil
}

// releaseSnapshots removes the preview's hold from the snapshots in its namespace.
func (r *PreviewReconciler) releaseSnapshots(ctx context.Context, p *kuprav1alpha1.Preview) error {
	var list kuprav1alpha1.PreviewSnapshotList
	if err := r.APIReader.List(ctx, &list, client.InNamespace(p.Namespace)); err != nil {
		return err
	}
	finalizer := restoreFinalizer(p)
	for i := range list.Items {
		s := &list.Items[i]
		if !controllerutil.ContainsFinalizer(s, finalizer) {
			continue
		}
		patch := client.MergeFromWithOptions(s.DeepCopy(), client.MergeFromWithOptimisticLock{})
		controllerutil.RemoveFinalizer(s, finalizer)
		if err := client.IgnoreNotFound(r.Patch(ctx, s, patch)); err != nil {
			return err
		}
	}
	return nil
}

// heldSnapshot returns the name of the snapshot the preview holds, if any.
func (r *PreviewReconciler) heldSnapshot(ctx context.Context, p *kuprav1alpha1.Preview) (string, error) {
	var list kuprav1alpha1.PreviewSnapshotList
	if err := r.APIReader.List(ctx, &list, client.InNamespace(p.Namespace)); err != nil {
		return "", err
	}
	finalizer := restoreFinalizer(p)
	for _, s := range list.Items {
		if controllerutil.ContainsFinalizer(&s, finalizer) {
			return s.Name, nil
		}
	}
	return "", nil
}
