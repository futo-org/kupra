package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

const (
	ReasonPreviewNotFound = "PreviewNotFound"
	ReasonPreviewDeleted  = "PreviewDeleted"
	ReasonInUse           = "InUse"

	// heldInterval is how often a snapshot whose deletion is held checks again.
	heldInterval = 30 * time.Second
)

// PreviewSnapshotReconciler looks after PreviewSnapshots before they start and once they're deleted. Capturing is done by the PreviewReconciler, alongside everything else that happens to the source preview.
type PreviewSnapshotReconciler struct {
	client.Client
	// APIReader lists the VolumeSnapshots and VolumeSnapshotContents a snapshot leaves behind, which kupra doesn't cache.
	APIReader client.Reader
}

// +kubebuilder:rbac:groups=kupra.futo.tech,resources=previewsnapshots/finalizers,verbs=update

func (r *PreviewSnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var s kuprav1alpha1.PreviewSnapshot
	if err := r.Get(ctx, req.NamespacedName, &s); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !s.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &s)
	}
	if !controllerutil.ContainsFinalizer(&s, Finalizer) {
		patch := client.MergeFromWithOptions(s.DeepCopy(), client.MergeFromWithOptimisticLock{})
		controllerutil.AddFinalizer(&s, Finalizer)
		if err := r.Patch(ctx, &s, patch); err != nil {
			return ctrl.Result{}, err
		}
	}
	if s.Status.StartTime != nil {
		return ctrl.Result{}, nil
	}

	patch := client.MergeFromWithOptions(s.DeepCopy(), client.MergeFromWithOptimisticLock{})
	s.Status.ObservedGeneration = s.Generation
	var p kuprav1alpha1.Preview
	err := r.Get(ctx, client.ObjectKey{Namespace: s.Namespace, Name: s.Spec.Source.PreviewRef.Name}, &p)
	switch {
	case apierrors.IsNotFound(err):
		waiting(&s, ReasonPreviewNotFound, fmt.Sprintf("Preview %s doesn't exist", s.Spec.Source.PreviewRef.Name))
	case err != nil:
		return ctrl.Result{}, err
	case !p.DeletionTimestamp.IsZero():
		now := metav1.Now()
		s.Status.CompletionTime = &now
		s.Status.Phase = kuprav1alpha1.PreviewSnapshotFailed
		message := fmt.Sprintf("Preview %s was deleted before it could be captured", p.Name)
		setCondition(&s.Status.Conditions, s.Generation, meta.ReadyCondition, metav1.ConditionFalse, ReasonPreviewDeleted, message)
		setCondition(&s.Status.Conditions, s.Generation, meta.StalledCondition, metav1.ConditionTrue, ReasonPreviewDeleted, message)
		apimeta.RemoveStatusCondition(&s.Status.Conditions, meta.ReconcilingCondition)
	default:
		waiting(&s, ReasonWaiting, fmt.Sprintf("waiting for Preview %s to be running, with no other capture or reset of it in progress", p.Name))
	}
	if err := r.Status().Patch(ctx, &s, patch); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return ctrl.Result{}, nil
}

func waiting(s *kuprav1alpha1.PreviewSnapshot, reason, message string) {
	s.Status.Phase = kuprav1alpha1.PreviewSnapshotInProgress
	setCondition(&s.Status.Conditions, s.Generation, meta.ReadyCondition, metav1.ConditionFalse, reason, message)
	setCondition(&s.Status.Conditions, s.Generation, meta.ReconcilingCondition, metav1.ConditionTrue, reason, message)
}

// reconcileDelete holds a snapshot while it's in use, and then removes the data it holds.
func (r *PreviewSnapshotReconciler) reconcileDelete(ctx context.Context, s *kuprav1alpha1.PreviewSnapshot) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(s, Finalizer) {
		return ctrl.Result{}, nil
	}
	holders, err := r.holders(ctx, s)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(holders) > 0 {
		patch := client.MergeFrom(s.DeepCopy())
		s.Status.Phase = kuprav1alpha1.PreviewSnapshotDeleting
		message := "in use by " + strings.Join(holders, ", ")
		setCondition(&s.Status.Conditions, s.Generation, meta.ReconcilingCondition, metav1.ConditionTrue, ReasonInUse, message)
		if err := r.Status().Patch(ctx, s, patch); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{RequeueAfter: heldInterval}, nil
	}

	// Its own VolumeSnapshots go with it through their owner references, and the VolumeSnapshotContents they're bound to delete the storage snapshots. What's left is anything a capture left behind, such as contents kept from its source.
	selector := client.MatchingLabels{LabelSnapshotUID: string(s.UID)}
	var snapshots snapshotv1.VolumeSnapshotList
	if err := r.APIReader.List(ctx, &snapshots, selector); err != nil && !apimeta.IsNoMatchError(err) {
		return ctrl.Result{}, err
	}
	for i := range snapshots.Items {
		if err := client.IgnoreNotFound(r.Delete(ctx, &snapshots.Items[i])); err != nil {
			return ctrl.Result{}, err
		}
	}
	var contents snapshotv1.VolumeSnapshotContentList
	if err := r.APIReader.List(ctx, &contents, selector); err != nil && !apimeta.IsNoMatchError(err) {
		return ctrl.Result{}, err
	}
	for i := range contents.Items {
		// Contents still bound to a VolumeSnapshot go once it does.
		if contents.Items[i].Spec.DeletionPolicy == snapshotv1.VolumeSnapshotContentDelete {
			continue
		}
		if err := client.IgnoreNotFound(r.Delete(ctx, &contents.Items[i])); err != nil {
			return ctrl.Result{}, err
		}
	}

	patch := client.MergeFromWithOptions(s.DeepCopy(), client.MergeFromWithOptimisticLock{})
	controllerutil.RemoveFinalizer(s, Finalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Patch(ctx, s, patch))
}

// holders lists what keeps a snapshot from being deleted: a capture of it that's running, previews that name it, and previews still restoring from it.
func (r *PreviewSnapshotReconciler) holders(ctx context.Context, s *kuprav1alpha1.PreviewSnapshot) ([]string, error) {
	var holders []string
	if s.Status.StartTime != nil && s.Status.CompletionTime == nil {
		holders = append(holders, "its capture, which is running")
	}
	var previews kuprav1alpha1.PreviewList
	if err := r.List(ctx, &previews, client.InNamespace(s.Namespace)); err != nil {
		return nil, err
	}
	for _, p := range previews.Items {
		if !p.DeletionTimestamp.IsZero() {
			continue
		}
		named := p.Spec.RestoreFrom != nil && p.Spec.RestoreFrom.SnapshotRef != nil && p.Spec.RestoreFrom.SnapshotRef.Name == s.Name
		restoring := p.Status.RestoredFrom != nil && p.Status.RestoredFrom.Name == s.Name && !apimeta.IsStatusConditionTrue(p.Status.Conditions, ConditionRestored)
		if named || restoring {
			holders = append(holders, "Preview "+p.Name)
		}
	}
	return holders, nil
}

func (r *PreviewSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuprav1alpha1.PreviewSnapshot{}).
		Watches(&kuprav1alpha1.Preview{}, handler.EnqueueRequestsFromMapFunc(r.snapshotsForPreview)).
		Complete(r)
}

// snapshotsForPreview maps a Preview to the snapshots in its namespace, which may be waiting for it or held by it.
func (r *PreviewSnapshotReconciler) snapshotsForPreview(ctx context.Context, p client.Object) []reconcile.Request {
	var snapshots kuprav1alpha1.PreviewSnapshotList
	if err := r.List(ctx, &snapshots, client.InNamespace(p.GetNamespace())); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "listing PreviewSnapshots")
		return nil
	}
	var requests []reconcile.Request
	for _, s := range snapshots.Items {
		if s.Spec.Source.PreviewRef.Name == p.GetName() || !s.DeletionTimestamp.IsZero() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&s)})
		}
	}
	return requests
}
