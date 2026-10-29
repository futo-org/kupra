package state

import (
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

var (
	volumeSnapshotGVK        = snapshotv1.SchemeGroupVersion.WithKind("VolumeSnapshot")
	volumeSnapshotContentGVK = snapshotv1.SchemeGroupVersion.WithKind("VolumeSnapshotContent")
)

// persistentVolumeClaim supports plain PersistentVolumeClaims, through the VolumeSnapshot API.
//
// A capture is a VolumeSnapshot of the claim, which is then moved next to the PreviewSnapshot: the storage snapshot is bound to a new VolumeSnapshotContent and VolumeSnapshot there, which are the only ones set to delete it. Restoring binds the storage snapshot to another VolumeSnapshot in the preview's namespace, and makes the claim its data source.
type persistentVolumeClaim struct{}

// Check refuses claims that set their own data source, since kupra sets it to restore them.
func (persistentVolumeClaim) Check(resource map[string]any, fldPath *field.Path) field.ErrorList {
	spec, _ := resource["spec"].(map[string]any)
	var errs field.ErrorList
	for _, key := range []string{"dataSource", "dataSourceRef", "dataSourceExpr", "dataSourceRefExpr"} {
		if _, ok := spec[key]; ok {
			errs = append(errs, field.Forbidden(fldPath.Child("spec", key), "kupra sets the data source of claims it handles as state; opt the claim out of being state to set your own"))
		}
	}
	if _, ok := resource["specExpr"]; ok {
		errs = append(errs, field.Forbidden(fldPath.Child("specExpr"), "kupra sets parts of the spec of claims it handles as state, so it can't be given as an expression"))
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
	if len(req.From.Data) != 1 || req.From.Data[0].Kind != volumeSnapshotGVK.Kind || ptr.Deref(req.From.Data[0].APIGroup, "") != volumeSnapshotGVK.Group {
		return Response{Failed: fmt.Sprintf("the capture of %s holds %v, not one VolumeSnapshot", req.Resource.GetName(), req.From.Data)}
	}

	captured := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.From.Namespace, Name: req.From.Data[0].Name}
	vsObj, ok := req.Observed[captured]
	if !ok {
		return Response{Observe: []Ref{captured}}
	}
	if vsObj == nil {
		return Response{Failed: fmt.Sprintf("the captured VolumeSnapshot %s/%s doesn't exist", captured.Namespace, captured.Name)}
	}
	var vs snapshotv1.VolumeSnapshot
	if err := fromUnstructured(vsObj, &vs); err != nil {
		return Response{Failed: err.Error()}
	}
	if vs.Status == nil || vs.Status.BoundVolumeSnapshotContentName == nil {
		return Response{Observe: []Ref{captured}}
	}
	contentRef := Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: *vs.Status.BoundVolumeSnapshotContentName}
	contentObj, ok := req.Observed[contentRef]
	if !ok || contentObj == nil {
		return Response{Observe: []Ref{captured, contentRef}}
	}
	var content snapshotv1.VolumeSnapshotContent
	if err := fromUnstructured(contentObj, &content); err != nil {
		return Response{Failed: err.Error()}
	}
	if snapshotHandle(&content) == nil {
		return Response{Observe: []Ref{captured, contentRef}}
	}

	namespace := req.Resource.GetNamespace()
	name := req.Names.Name("restore")
	copied := bind(&content, req.Names.ClusterName("restore"), namespace, name, snapshotv1.VolumeSnapshotContentRetain, req.Labels)

	claim := req.Resource.DeepCopy()
	apiGroup := volumeSnapshotGVK.Group
	ref := map[string]any{"apiGroup": apiGroup, "kind": volumeSnapshotGVK.Kind, "name": name}
	if err := unstructured.SetNestedMap(claim.Object, ref, "spec", "dataSourceRef"); err != nil {
		return Response{Failed: err.Error()}
	}
	// A claim restored from a snapshot can't be smaller than the snapshot.
	if vs.Status.RestoreSize != nil {
		requested, _, _ := unstructured.NestedString(claim.Object, "spec", "resources", "requests", "storage")
		if q, err := resource.ParseQuantity(requested); err != nil || q.Cmp(*vs.Status.RestoreSize) < 0 {
			if err := unstructured.SetNestedField(claim.Object, vs.Status.RestoreSize.String(), "spec", "resources", "requests", "storage"); err != nil {
				return Response{Failed: err.Error()}
			}
		}
	}
	return Response{Resource: claim, Apply: copied, Done: true}
}

// Ready is true once the claim exists: storage that binds volumes when a pod first uses them would otherwise never be ready.
func (persistentVolumeClaim) Ready(live *unstructured.Unstructured) bool {
	return live != nil
}

func (persistentVolumeClaim) Capture(req CaptureRequest) Response {
	source := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.Resource.GetNamespace(), Name: req.Names.Name("capture")}
	target := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.Namespace, Name: req.Names.Name("data")}
	sourceObj, sourceObserved := req.Observed[source]
	targetObj, targetObserved := req.Observed[target]
	if !sourceObserved || !targetObserved {
		return Response{Observe: []Ref{source, target}}
	}

	// Once the capture has moved, only the copy next to the PreviewSnapshot remains.
	if targetObj != nil {
		var vs snapshotv1.VolumeSnapshot
		if err := fromUnstructured(targetObj, &vs); err != nil {
			return Response{Failed: err.Error()}
		}
		if failed := snapshotError(&vs); failed != "" {
			return Response{Failed: failed}
		}
		if vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
			return Response{Observe: []Ref{source, target}}
		}
		if sourceObj != nil {
			return Response{Delete: []Ref{source}, Observe: []Ref{source, target}}
		}
		return Response{Done: true, Capture: &Capture{Data: []corev1.TypedLocalObjectReference{{APIGroup: &volumeSnapshotGVK.Group, Kind: target.Kind, Name: target.Name}}}}
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
	if failed := snapshotError(&vs); failed != "" {
		return Response{Failed: failed}
	}
	if vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse || vs.Status.BoundVolumeSnapshotContentName == nil {
		return Response{Observe: []Ref{source, target}}
	}
	contentRef := Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: *vs.Status.BoundVolumeSnapshotContentName}
	contentObj, ok := req.Observed[contentRef]
	if !ok || contentObj == nil {
		return Response{Observe: []Ref{source, target, contentRef}}
	}
	var content snapshotv1.VolumeSnapshotContent
	if err := fromUnstructured(contentObj, &content); err != nil {
		return Response{Failed: err.Error()}
	}
	// The source's content has to keep the storage snapshot when the source is deleted, or the preview's namespace is. It's labelled whatever its policy was, since it's left behind and has to be found to be cleaned up.
	if content.Spec.DeletionPolicy != snapshotv1.VolumeSnapshotContentRetain || !hasLabels(content.Labels, req.Labels) {
		// Only the deletion policy and the labels are kupra's to set.
		u := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": content.Name, "labels": stringMap(req.Labels)},
			"spec":     map[string]any{"deletionPolicy": string(snapshotv1.VolumeSnapshotContentRetain)},
		}}
		u.SetGroupVersionKind(volumeSnapshotContentGVK)
		return Response{Apply: []*unstructured.Unstructured{u}, Observe: []Ref{source, target, contentRef}}
	}
	if snapshotHandle(&content) == nil {
		return Response{Observe: []Ref{source, target, contentRef}}
	}
	moved := bind(&content, req.Names.ClusterName("data"), target.Namespace, target.Name, snapshotv1.VolumeSnapshotContentDelete, req.Labels)
	return Response{Apply: moved, Observe: []Ref{source, target}}
}

// bind returns a VolumeSnapshotContent for the storage snapshot behind content, and a VolumeSnapshot bound to it.
func bind(content *snapshotv1.VolumeSnapshotContent, contentName, namespace, name string, policy snapshotv1.DeletionPolicy, labels map[string]string) []*unstructured.Unstructured {
	c := &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: contentName, Labels: labels},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			DeletionPolicy:          policy,
			Driver:                  content.Spec.Driver,
			Source:                  snapshotv1.VolumeSnapshotContentSource{SnapshotHandle: snapshotHandle(content)},
			VolumeSnapshotClassName: content.Spec.VolumeSnapshotClassName,
			SourceVolumeMode:        content.Spec.SourceVolumeMode,
			VolumeSnapshotRef:       corev1.ObjectReference{Namespace: namespace, Name: name},
		},
	}
	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source:                  snapshotv1.VolumeSnapshotSource{VolumeSnapshotContentName: &contentName},
			VolumeSnapshotClassName: content.Spec.VolumeSnapshotClassName,
		},
	}
	return []*unstructured.Unstructured{toUnstructured(c, volumeSnapshotContentGVK), toUnstructured(vs, volumeSnapshotGVK)}
}

func snapshotError(vs *snapshotv1.VolumeSnapshot) string {
	if vs.Status == nil || vs.Status.Error == nil || vs.Status.Error.Message == nil {
		return ""
	}
	return fmt.Sprintf("VolumeSnapshot %s: %s", vs.Name, *vs.Status.Error.Message)
}

func toUnstructured(obj runtime.Object, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		// Converting a typed object only fails on types that can't be represented, which these never are.
		panic(err)
	}
	u := &unstructured.Unstructured{Object: content}
	u.SetGroupVersionKind(gvk)
	unstructured.RemoveNestedField(u.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(u.Object, "status")
	dropNulls(u.Object)
	return u
}

// dropNulls removes the nulls that converting typed objects leaves for unset fields without omitempty, which CRD schemas reject.
func dropNulls(m map[string]any) {
	for k, v := range m {
		switch v := v.(type) {
		case nil:
			delete(m, k)
		case map[string]any:
			dropNulls(v)
		}
	}
}

func stringMap(m map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func fromUnstructured(u *unstructured.Unstructured, obj any) error {
	return runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
}

// keepRestored carries over what restoring set on a claim, since a claim's data source can't change and it can't shrink.
func keepRestored(rendered, live *unstructured.Unstructured) *unstructured.Unstructured {
	claim := rendered.DeepCopy()
	if ref, ok, _ := unstructured.NestedMap(live.Object, "spec", "dataSourceRef"); ok {
		_ = unstructured.SetNestedMap(claim.Object, ref, "spec", "dataSourceRef")
	}
	if ref, ok, _ := unstructured.NestedMap(live.Object, "spec", "dataSource"); ok {
		_ = unstructured.SetNestedMap(claim.Object, ref, "spec", "dataSource")
	}
	liveSize, _, _ := unstructured.NestedString(live.Object, "spec", "resources", "requests", "storage")
	size, _, _ := unstructured.NestedString(claim.Object, "spec", "resources", "requests", "storage")
	if l, err := resource.ParseQuantity(liveSize); err == nil {
		if s, err := resource.ParseQuantity(size); err != nil || s.Cmp(l) < 0 {
			_ = unstructured.SetNestedField(claim.Object, liveSize, "spec", "resources", "requests", "storage")
		}
	}
	return claim
}

// snapshotHandle is the storage snapshot behind a content: in its spec when it was bound to an existing snapshot, and in its status once a new one was taken.
func snapshotHandle(content *snapshotv1.VolumeSnapshotContent) *string {
	if content.Spec.Source.SnapshotHandle != nil {
		return content.Spec.Source.SnapshotHandle
	}
	if content.Status != nil {
		return content.Status.SnapshotHandle
	}
	return nil
}

// Restored is true once the claim is bound, since the volume is only provisioned from the snapshot then.
func (persistentVolumeClaim) Restored(live *unstructured.Unstructured) bool {
	phase, _, _ := unstructured.NestedString(live.Object, "status", "phase")
	return phase == string(corev1.ClaimBound)
}

func hasLabels(labels, want map[string]string) bool {
	for k, v := range want {
		if labels[k] != v {
			return false
		}
	}
	return true
}
