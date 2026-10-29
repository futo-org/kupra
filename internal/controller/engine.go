package controller

import (
	"context"
	"fmt"
	"maps"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/futo-org/kupra/internal/state"
)

const (
	// LabelPreviewUID marks objects kupra creates outside a preview's namespace on its behalf, such as the VolumeSnapshotContents it restores from.
	LabelPreviewUID = "kupra.futo.tech/preview-uid"
	// LabelSnapshotUID marks objects kupra creates for a PreviewSnapshot.
	LabelSnapshotUID = "kupra.futo.tech/snapshot-uid"

	// maxKindRounds bounds how often a kind is called in one reconciliation. Kinds ask to observe a few objects at a time, so a handful of rounds gets as far as the cluster allows.
	maxKindRounds = 8
)

// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;create;patch;delete
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotcontents,verbs=get;list;create;patch;delete

// runKind calls a kind of state with what it asked to observe, and applies and deletes what it asks for, until it's done, fails, or has nothing new to go on. Objects in ownedNamespace get owner as their owner, so they go with it.
func (r *PreviewReconciler) runKind(ctx context.Context, call func(state.Observed) state.Response, ownedNamespace string, owner *metav1.OwnerReference) (state.Response, error) {
	observed := state.Observed{}
	for range maxKindRounds {
		resp := call(observed)
		if resp.Failed != "" {
			return resp, nil
		}
		for _, obj := range resp.Apply {
			if owner != nil && obj.GetNamespace() == ownedNamespace {
				obj.SetOwnerReferences([]metav1.OwnerReference{*owner})
			}
			if err := r.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.FieldOwner(FieldManager), client.ForceOwnership); err != nil {
				return resp, fmt.Errorf("applying %s %s: %w", obj.GetKind(), obj.GetName(), err)
			}
		}
		for _, ref := range resp.Delete {
			if err := client.IgnoreNotFound(r.Delete(ctx, refObject(ref))); err != nil {
				return resp, fmt.Errorf("deleting %s %s: %w", ref.Kind, ref.Name, err)
			}
		}
		if resp.Done {
			return resp, nil
		}

		next := state.Observed{}
		for _, ref := range resp.Observe {
			live := refObject(ref)
			switch err := r.Get(ctx, client.ObjectKeyFromObject(live), live); {
			case apierrors.IsNotFound(err):
				next[ref] = nil
			case err != nil:
				return resp, err
			default:
				next[ref] = live
			}
		}
		if len(resp.Apply) == 0 && len(resp.Delete) == 0 && sameObserved(observed, next) {
			return resp, nil
		}
		observed = next
	}
	return state.Response{}, nil
}

func refObject(ref state.Ref) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(ref.GroupVersionKind)
	u.SetNamespace(ref.Namespace)
	u.SetName(ref.Name)
	return u
}

// sameObserved reports whether two observations saw the same objects, at the same versions.
func sameObserved(a, b state.Observed) bool {
	return maps.EqualFunc(a, b, func(x, y *unstructured.Unstructured) bool {
		if x == nil || y == nil {
			return x == y
		}
		return x.GetResourceVersion() == y.GetResourceVersion()
	})
}
