package state

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	volumeSnapshotGVK        = snapshotv1.SchemeGroupVersion.WithKind("VolumeSnapshot")
	volumeSnapshotContentGVK = snapshotv1.SchemeGroupVersion.WithKind("VolumeSnapshotContent")
)

// boundSnapshot is a VolumeSnapshot with the content it's bound to.
type boundSnapshot struct {
	snapshot *snapshotv1.VolumeSnapshot
	content  *snapshotv1.VolumeSnapshotContent
}

// readBound reads a captured VolumeSnapshot and its content. It returns the objects to read until the content has a storage snapshot, or why it can't be used.
func readBound(ref Ref, observed Observed) (*boundSnapshot, []Ref, string) {
	obj, ok := observed[ref]
	if !ok {
		return nil, []Ref{ref}, ""
	}
	if obj == nil {
		return nil, nil, fmt.Sprintf("VolumeSnapshot %s/%s not found", ref.Namespace, ref.Name)
	}
	var vs snapshotv1.VolumeSnapshot
	if err := fromUnstructured(obj, &vs); err != nil {
		return nil, nil, err.Error()
	}
	if vs.Status == nil || vs.Status.BoundVolumeSnapshotContentName == nil {
		return nil, []Ref{ref}, ""
	}
	contentRef := Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: *vs.Status.BoundVolumeSnapshotContentName}
	contentObj, ok := observed[contentRef]
	if !ok || contentObj == nil {
		return nil, []Ref{ref, contentRef}, ""
	}
	var content snapshotv1.VolumeSnapshotContent
	if err := fromUnstructured(contentObj, &content); err != nil {
		return nil, nil, err.Error()
	}
	if snapshotHandle(&content) == nil {
		return nil, []Ref{ref, contentRef}, ""
	}
	return &boundSnapshot{snapshot: &vs, content: &content}, []Ref{ref, contentRef}, ""
}

// move moves a ready VolumeSnapshot's storage snapshot to target: it binds it to a new VolumeSnapshotContent there, the only one that deletes it. It returns the objects to apply and read.
func move(source *snapshotv1.VolumeSnapshot, target metav1.ObjectMeta, contentName string, labels map[string]string, observed Observed) ([]*unstructured.Unstructured, []Ref, string) {
	if source.Status == nil || source.Status.BoundVolumeSnapshotContentName == nil {
		return nil, nil, ""
	}
	contentRef := Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: *source.Status.BoundVolumeSnapshotContentName}
	contentObj, ok := observed[contentRef]
	if !ok || contentObj == nil {
		return nil, []Ref{contentRef}, ""
	}
	var content snapshotv1.VolumeSnapshotContent
	if err := fromUnstructured(contentObj, &content); err != nil {
		return nil, nil, err.Error()
	}
	// The source's content must outlive the source, and be found again to be deleted with the PreviewSnapshot.
	if content.Spec.DeletionPolicy != snapshotv1.VolumeSnapshotContentRetain || !hasLabels(content.Labels, labels) {
		u := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": content.Name, "labels": stringMap(labels)},
			"spec":     map[string]any{"deletionPolicy": string(snapshotv1.VolumeSnapshotContentRetain)},
		}}
		u.SetGroupVersionKind(volumeSnapshotContentGVK)
		return []*unstructured.Unstructured{u}, []Ref{contentRef}, ""
	}
	if snapshotHandle(&content) == nil {
		return nil, []Ref{contentRef}, ""
	}
	return bind(&content, contentName, target, snapshotv1.VolumeSnapshotContentDelete, labels), nil, ""
}

// bind returns a VolumeSnapshotContent for the storage snapshot behind content, and a VolumeSnapshot bound to it. The content gets labels, and the VolumeSnapshot its own metadata.
func bind(content *snapshotv1.VolumeSnapshotContent, contentName string, snapshot metav1.ObjectMeta, policy snapshotv1.DeletionPolicy, labels map[string]string) []*unstructured.Unstructured {
	c := &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: contentName, Labels: labels},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			DeletionPolicy:          policy,
			Driver:                  content.Spec.Driver,
			Source:                  snapshotv1.VolumeSnapshotContentSource{SnapshotHandle: snapshotHandle(content)},
			VolumeSnapshotClassName: content.Spec.VolumeSnapshotClassName,
			SourceVolumeMode:        content.Spec.SourceVolumeMode,
			VolumeSnapshotRef:       corev1.ObjectReference{Namespace: snapshot.Namespace, Name: snapshot.Name},
		},
	}
	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: snapshot,
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source:                  snapshotv1.VolumeSnapshotSource{VolumeSnapshotContentName: &contentName},
			VolumeSnapshotClassName: content.Spec.VolumeSnapshotClassName,
		},
	}
	return []*unstructured.Unstructured{toUnstructured(c, volumeSnapshotContentGVK), toUnstructured(vs, volumeSnapshotGVK)}
}

// readyToUse reads a VolumeSnapshot and reports whether it's ready, or what it's waiting for.
func readyToUse(obj *unstructured.Unstructured) (bool, string, string) {
	var vs snapshotv1.VolumeSnapshot
	if err := fromUnstructured(obj, &vs); err != nil {
		return false, "", err.Error()
	}
	if vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		return false, snapshotError(&vs), ""
	}
	return true, "", ""
}

func snapshotError(vs *snapshotv1.VolumeSnapshot) string {
	if vs.Status == nil || vs.Status.Error == nil || vs.Status.Error.Message == nil {
		return ""
	}
	return fmt.Sprintf("VolumeSnapshot %s: %s", vs.Name, *vs.Status.Error.Message)
}

// snapshotHandle is a content's storage snapshot: in its spec if it was bound to an existing one, otherwise in its status.
func snapshotHandle(content *snapshotv1.VolumeSnapshotContent) *string {
	if content.Spec.Source.SnapshotHandle != nil {
		return content.Spec.Source.SnapshotHandle
	}
	if content.Status != nil {
		return content.Status.SnapshotHandle
	}
	return nil
}

func hasLabels(labels, want map[string]string) bool {
	for k, v := range want {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func snapshotRef(name string) corev1.TypedLocalObjectReference {
	return corev1.TypedLocalObjectReference{APIGroup: &volumeSnapshotGVK.Group, Kind: volumeSnapshotGVK.Kind, Name: name}
}

// isSnapshot reports whether a capture's data is a VolumeSnapshot.
func isSnapshot(ref corev1.TypedLocalObjectReference) bool {
	return ref.Kind == volumeSnapshotGVK.Kind && ref.APIGroup != nil && *ref.APIGroup == volumeSnapshotGVK.Group
}

// allReady reports whether the VolumeSnapshots at refs all exist, whether they're all ready to use, and if not, what they're waiting for.
func allReady(refs []Ref, observed Observed) (exist, ready bool, waiting, failed string) {
	var messages []string
	ready = true
	for _, ref := range refs {
		obj := observed[ref]
		if obj == nil {
			return false, false, "", ""
		}
		r, w, failed := readyToUse(obj)
		if failed != "" {
			return false, false, "", failed
		}
		ready = ready && r
		if w != "" {
			messages = append(messages, w)
		}
	}
	slices.Sort(messages)
	return true, ready, strings.Join(messages, "; "), ""
}

// restoreCopies returns the restore copies read and found missing, to apply, and whether all of them are ready to use, or what they're waiting for. A copy that exists isn't applied again: its content's volumeSnapshotRef is replaced as a whole, which would drop the UID the snapshot controller bound it with.
func restoreCopies(copies map[Ref][]*unstructured.Unstructured, observed Observed) (apply []*unstructured.Unstructured, observe []Ref, ready bool, waiting, failed string) {
	var messages []string
	ready = true
	for _, ref := range slices.SortedFunc(maps.Keys(copies), func(a, b Ref) int { return strings.Compare(a.Name, b.Name) }) {
		observe = append(observe, ref)
		obj, seen := observed[ref]
		if !seen {
			// Each reconciliation starts with nothing read, so a copy is only known to be missing once it's been read.
			ready = false
			continue
		}
		if obj == nil {
			apply = append(apply, copies[ref]...)
			ready = false
			continue
		}
		r, w, f := readyToUse(obj)
		if f != "" {
			return nil, nil, false, "", f
		}
		ready = ready && r
		if w != "" {
			messages = append(messages, w)
		}
	}
	return apply, observe, ready, strings.Join(messages, "; "), ""
}
