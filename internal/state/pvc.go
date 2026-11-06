package state

import (
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

// persistentVolumeClaim supports PersistentVolumeClaims, through VolumeSnapshots.
type persistentVolumeClaim struct{}

func (persistentVolumeClaim) Check(resource map[string]any, fldPath *field.Path) field.ErrorList {
	spec, _ := resource["spec"].(map[string]any)
	var errs field.ErrorList
	for _, key := range []string{"dataSource", "dataSourceRef", "dataSourceExpr", "dataSourceRefExpr"} {
		if _, ok := spec[key]; ok {
			errs = append(errs, field.Forbidden(fldPath.Child("spec", key), "may not be set on a claim with managed state"))
		}
	}
	if _, ok := resource["specExpr"]; ok {
		errs = append(errs, field.Forbidden(fldPath.Child("specExpr"), "may not be set on a claim with managed state"))
	}
	return errs
}

func (persistentVolumeClaim) Restore(req RestoreRequest) Response {
	if req.Live != nil {
		return Response{Resource: keepRestored(req.Resource, req.Live), Done: true}
	}
	if req.From == nil {
		return Response{Resource: req.Resource, Done: true}
	}
	if len(req.From.Data) != 1 || !isSnapshot(req.From.Data[0]) {
		return Response{Failed: fmt.Sprintf("capture must be one VolumeSnapshot, not %v", req.From.Data)}
	}

	captured := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.From.Namespace, Name: req.From.Data[0].Name}
	bound, observe, failed := readBound(captured, req.Observed)
	if failed != "" {
		return Response{Failed: failed}
	}
	if bound == nil {
		return Response{Observe: observe}
	}

	namespace := req.Resource.GetNamespace()
	name := req.Names.Name("restore")
	copied := bind(bound.content, req.Names.ClusterName("restore"), metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: req.Labels}, snapshotv1.VolumeSnapshotContentRetain, req.Labels)

	claim := req.Resource.DeepCopy()
	ref := map[string]any{"apiGroup": volumeSnapshotGVK.Group, "kind": volumeSnapshotGVK.Kind, "name": name}
	if err := unstructured.SetNestedMap(claim.Object, ref, "spec", "dataSourceRef"); err != nil {
		return Response{Failed: err.Error()}
	}
	// A claim can't be smaller than the snapshot it's restored from.
	if err := atLeast(claim, bound.snapshot.Status.RestoreSize, "spec", "resources", "requests", "storage"); err != nil {
		return Response{Failed: err.Error()}
	}
	return Response{Resource: claim, Apply: copied, Done: true}
}

// Ready is true once the claim exists, since storage that binds on first use doesn't bind before pods start.
func (persistentVolumeClaim) Ready(live *unstructured.Unstructured) bool {
	return live != nil
}

// Restored is true once the claim is bound, since its volume is only provisioned then.
func (persistentVolumeClaim) Restored(live *unstructured.Unstructured) bool {
	phase, _, _ := unstructured.NestedString(live.Object, "status", "phase")
	return phase == string(corev1.ClaimBound)
}

// Capture takes a VolumeSnapshot of the claim, then moves it next to the PreviewSnapshot.
func (persistentVolumeClaim) Capture(req CaptureRequest) Response {
	source := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.Resource.GetNamespace(), Name: req.Names.Name("capture")}
	target := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.Namespace, Name: req.Names.Name("data")}
	sourceObj, sourceObserved := req.Observed[source]
	targetObj, targetObserved := req.Observed[target]
	if !sourceObserved || !targetObserved {
		return Response{Observe: []Ref{source, target}}
	}

	if targetObj != nil {
		ready, waiting, failed := readyToUse(targetObj)
		if failed != "" {
			return Response{Failed: failed}
		}
		if !ready {
			return Response{Observe: []Ref{source, target}, Waiting: waiting}
		}
		if sourceObj != nil {
			return Response{Delete: []Ref{source}, Observe: []Ref{source, target}}
		}
		return Response{Done: true, Capture: &Capture{Data: []corev1.TypedLocalObjectReference{snapshotRef(target.Name)}}}
	}

	if sourceObj == nil {
		vs := &snapshotv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: source.Name, Namespace: source.Namespace, Labels: req.Labels},
			Spec: snapshotv1.VolumeSnapshotSpec{
				Source: snapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: ptr.To(req.Resource.GetName())},
			},
		}
		return Response{Apply: []*unstructured.Unstructured{toUnstructured(vs, volumeSnapshotGVK)}, Observe: []Ref{source, target}}
	}
	var vs snapshotv1.VolumeSnapshot
	if err := fromUnstructured(sourceObj, &vs); err != nil {
		return Response{Failed: err.Error()}
	}
	if vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		return Response{Observe: []Ref{source, target}, Waiting: snapshotError(&vs)}
	}
	apply, observe, failed := move(&vs, metav1.ObjectMeta{Name: target.Name, Namespace: target.Namespace, Labels: req.Labels}, req.Names.ClusterName("data"), req.Labels, req.Observed)
	if failed != "" {
		return Response{Failed: failed}
	}
	return Response{Apply: apply, Observe: append([]Ref{source, target}, observe...)}
}

// keepRestored keeps a live claim's data source and size, which can't change and can't shrink.
func keepRestored(rendered, live *unstructured.Unstructured) *unstructured.Unstructured {
	claim := rendered.DeepCopy()
	if ref, ok, _ := unstructured.NestedMap(live.Object, "spec", "dataSourceRef"); ok {
		_ = unstructured.SetNestedMap(claim.Object, ref, "spec", "dataSourceRef")
	}
	if ref, ok, _ := unstructured.NestedMap(live.Object, "spec", "dataSource"); ok {
		_ = unstructured.SetNestedMap(claim.Object, ref, "spec", "dataSource")
	}
	keepSize(claim, live, "spec", "resources", "requests", "storage")
	return claim
}

// keepSize keeps a live object's size at fields if the rendered one is smaller.
func keepSize(rendered, live *unstructured.Unstructured, fields ...string) {
	liveSize, _, _ := unstructured.NestedString(live.Object, fields...)
	if l, err := resource.ParseQuantity(liveSize); err == nil {
		_ = atLeast(rendered, &l, fields...)
	}
}

// atLeast raises the size at fields to min, if it's smaller or not set.
func atLeast(obj *unstructured.Unstructured, min *resource.Quantity, fields ...string) error {
	if min == nil {
		return nil
	}
	size, _, _ := unstructured.NestedString(obj.Object, fields...)
	if q, err := resource.ParseQuantity(size); err == nil && q.Cmp(*min) >= 0 {
		return nil
	}
	return unstructured.SetNestedField(obj.Object, min.String(), fields...)
}
