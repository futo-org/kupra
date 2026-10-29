package state

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func object(t *testing.T, doc string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(doc), &u.Object); err != nil {
		t.Fatal(err)
	}
	return u
}

var claimGK = schema.GroupKind{Kind: "PersistentVolumeClaim"}

const claim = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: data, namespace: preview}
spec:
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 1Gi}}
`

// cluster plays the API server and the snapshot controller for a kind: it applies and deletes what the kind asks for, and answers what it observes.
type cluster map[Ref]*unstructured.Unstructured

func (c cluster) step(resp Response) Observed {
	for _, obj := range resp.Apply {
		ref := RefOf(obj)
		if live, ok := c[ref]; ok {
			// Applying a partial object, like the deletion policy alone, merges into what's there.
			merged := live.DeepCopy()
			spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
			for k, v := range spec {
				_ = unstructured.SetNestedField(merged.Object, v, "spec", k)
			}
			labels := merged.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			for k, v := range obj.GetLabels() {
				labels[k] = v
			}
			merged.SetLabels(labels)
			c[ref] = merged
			continue
		}
		c[ref] = obj.DeepCopy()
	}
	for _, ref := range resp.Delete {
		delete(c, ref)
	}
	observed := Observed{}
	for _, ref := range resp.Observe {
		observed[ref] = c[ref]
	}
	return observed
}

func (c cluster) find(kind, namespace string) *unstructured.Unstructured {
	for ref, obj := range c {
		if ref.Kind == kind && ref.Namespace == namespace {
			return obj
		}
	}
	return nil
}

func TestPersistentVolumeClaimCapture(t *testing.T) {
	kind, _ := For(claimGK)
	c := cluster{}
	req := CaptureRequest{
		Resource:  object(t, claim),
		Namespace: "app",
		Names:     NewNames("snap", "uid", claimGK, "data"),
		Labels:    map[string]string{"kupra.futo.tech/snapshot-uid": "uid"},
	}
	run := func() Response {
		t.Helper()
		resp := kind.Capture(req)
		if resp.Failed != "" {
			t.Fatal(resp.Failed)
		}
		req.Observed = c.step(resp)
		return resp
	}

	run()
	run()
	source := c.find("VolumeSnapshot", "preview")
	if source == nil {
		t.Fatal("no VolumeSnapshot of the claim")
	}
	if pvc, _, _ := unstructured.NestedString(source.Object, "spec", "source", "persistentVolumeClaimName"); pvc != "data" {
		t.Errorf("VolumeSnapshot source = %q, want the claim", pvc)
	}
	if resp := run(); len(resp.Apply) > 0 {
		t.Fatalf("applied %v before the VolumeSnapshot was ready", resp.Apply)
	}

	// The snapshot controller takes the snapshot.
	c[Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: "content-1"}] = object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotContent
metadata: {name: content-1}
spec:
  deletionPolicy: Delete
  driver: hostpath.csi.k8s.io
  volumeSnapshotClassName: csi-hostpath-snapclass
  source: {volumeHandle: vol-1}
  volumeSnapshotRef: {namespace: preview, name: `+source.GetName()+`}
status: {snapshotHandle: handle-1, readyToUse: true}
`)
	_ = unstructured.SetNestedField(source.Object, map[string]any{"boundVolumeSnapshotContentName": "content-1", "readyToUse": true}, "status")

	run()
	run()
	if policy, _, _ := unstructured.NestedString(c[Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: "content-1"}].Object, "spec", "deletionPolicy"); policy != "Retain" {
		t.Fatalf("source content's deletion policy = %s, want Retain before anything else", policy)
	}
	run()
	run()
	target := c.find("VolumeSnapshot", "app")
	if target == nil {
		t.Fatal("no VolumeSnapshot next to the PreviewSnapshot")
	}
	contentName, _, _ := unstructured.NestedString(target.Object, "spec", "source", "volumeSnapshotContentName")
	content := c[Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: contentName}]
	if content == nil {
		t.Fatalf("no VolumeSnapshotContent %s for the moved capture", contentName)
	}
	for _, want := range [][2]string{
		{"spec.deletionPolicy", "Delete"},
		{"spec.source.snapshotHandle", "handle-1"},
		{"spec.driver", "hostpath.csi.k8s.io"},
		{"spec.volumeSnapshotRef.namespace", "app"},
		{"spec.volumeSnapshotRef.name", target.GetName()},
	} {
		if got, _, _ := unstructured.NestedString(content.Object, strings.Split(want[0], ".")...); got != want[1] {
			t.Errorf("moved content %s = %q, want %q", want[0], got, want[1])
		}
	}
	if target.GetLabels()["kupra.futo.tech/snapshot-uid"] != "uid" || content.GetLabels()["kupra.futo.tech/snapshot-uid"] != "uid" {
		t.Error("moved objects aren't labelled")
	}

	// The snapshot controller binds the copy.
	_ = unstructured.SetNestedField(target.Object, map[string]any{"boundVolumeSnapshotContentName": contentName, "readyToUse": true}, "status")
	var resp Response
	for range 3 {
		if resp = run(); resp.Done {
			break
		}
	}
	if !resp.Done {
		t.Fatal("capture isn't done once the copy is ready")
	}
	if c.find("VolumeSnapshot", "preview") != nil {
		t.Error("the source VolumeSnapshot is left in the preview's namespace")
	}
	if len(resp.Capture.Data) != 1 || resp.Capture.Data[0].Name != target.GetName() || resp.Capture.Data[0].Kind != "VolumeSnapshot" {
		t.Errorf("capture = %+v, want the moved VolumeSnapshot", resp.Capture)
	}
}

func TestPersistentVolumeClaimCaptureFails(t *testing.T) {
	kind, _ := For(claimGK)
	names := NewNames("snap", "uid", claimGK, "data")
	source := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: "preview", Name: names.Name("capture")}
	target := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: "app", Name: names.Name("data")}
	failed := object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata: {name: x, namespace: preview}
status: {error: {message: no snapshot class}}
`)
	resp := kind.Capture(CaptureRequest{Resource: object(t, claim), Namespace: "app", Names: names, Observed: Observed{source: failed, target: nil}})
	if !strings.Contains(resp.Failed, "no snapshot class") {
		t.Errorf("failure = %q, want the VolumeSnapshot's error", resp.Failed)
	}
}

func TestPersistentVolumeClaimRestore(t *testing.T) {
	kind, _ := For(claimGK)
	group := "snapshot.storage.k8s.io"
	c := cluster{
		{GroupVersionKind: volumeSnapshotGVK, Namespace: "app", Name: "captured"}: object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata: {name: captured, namespace: app}
status: {boundVolumeSnapshotContentName: content-2, readyToUse: true, restoreSize: 2Gi}
`),
		{GroupVersionKind: volumeSnapshotContentGVK, Name: "content-2"}: object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotContent
metadata: {name: content-2}
spec:
  deletionPolicy: Delete
  driver: hostpath.csi.k8s.io
  source: {snapshotHandle: handle-2}
  volumeSnapshotRef: {namespace: app, name: captured}
status: {snapshotHandle: handle-2, readyToUse: true}
`),
	}
	req := RestoreRequest{
		Resource: object(t, claim),
		From:     &Source{Namespace: "app", Data: []corev1.TypedLocalObjectReference{{APIGroup: &group, Kind: "VolumeSnapshot", Name: "captured"}}},
		Names:    NewNames("pr-1", "uid", claimGK, "data"),
		Labels:   map[string]string{"kupra.futo.tech/preview-uid": "uid"},
	}
	var resp Response
	for range 4 {
		resp = kind.Restore(req)
		if resp.Failed != "" {
			t.Fatal(resp.Failed)
		}
		if resp.Done {
			break
		}
		req.Observed = c.step(resp)
	}
	if !resp.Done {
		t.Fatal("restore isn't done")
	}
	c.step(resp)

	restored := c.find("VolumeSnapshot", "preview")
	if restored == nil {
		t.Fatal("no VolumeSnapshot in the preview's namespace")
	}
	contentName, _, _ := unstructured.NestedString(restored.Object, "spec", "source", "volumeSnapshotContentName")
	content := c[Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: contentName}]
	if content == nil {
		t.Fatal("no VolumeSnapshotContent bound to it")
	}
	if policy, _, _ := unstructured.NestedString(content.Object, "spec", "deletionPolicy"); policy != "Retain" {
		t.Errorf("restore copy's deletion policy = %s, want Retain, so only the PreviewSnapshot deletes the storage snapshot", policy)
	}
	if handle, _, _ := unstructured.NestedString(content.Object, "spec", "source", "snapshotHandle"); handle != "handle-2" {
		t.Errorf("restore copy's snapshot handle = %s, want handle-2", handle)
	}
	if name, _, _ := unstructured.NestedString(resp.Resource.Object, "spec", "dataSourceRef", "name"); name != restored.GetName() {
		t.Errorf("claim's data source = %q, want %s", name, restored.GetName())
	}
	if size, _, _ := unstructured.NestedString(resp.Resource.Object, "spec", "resources", "requests", "storage"); size != "2Gi" {
		t.Errorf("claim's size = %s, want the snapshot's 2Gi", size)
	}

	// Once the claim exists, updates keep its data source and size, without looking at the snapshot again.
	live := resp.Resource
	rendered := object(t, claim)
	again := kind.Restore(RestoreRequest{Resource: rendered, Live: live, From: req.From, Names: req.Names})
	if !again.Done || len(again.Apply) > 0 || len(again.Observe) > 0 {
		t.Fatalf("restoring an existing claim = %+v, want it done straight away", again)
	}
	if name, _, _ := unstructured.NestedString(again.Resource.Object, "spec", "dataSourceRef", "name"); name != restored.GetName() {
		t.Errorf("updated claim's data source = %q, want it kept", name)
	}
	if size, _, _ := unstructured.NestedString(again.Resource.Object, "spec", "resources", "requests", "storage"); size != "2Gi" {
		t.Errorf("updated claim's size = %s, want it kept at 2Gi", size)
	}
}

func TestPersistentVolumeClaimRestoreFromMissingSnapshot(t *testing.T) {
	kind, _ := For(claimGK)
	group := "snapshot.storage.k8s.io"
	captured := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: "app", Name: "captured"}
	resp := kind.Restore(RestoreRequest{
		Resource: object(t, claim),
		From:     &Source{Namespace: "app", Data: []corev1.TypedLocalObjectReference{{APIGroup: &group, Kind: "VolumeSnapshot", Name: "captured"}}},
		Names:    NewNames("pr-1", "uid", claimGK, "data"),
		Observed: Observed{captured: nil},
	})
	if !strings.Contains(resp.Failed, "doesn't exist") {
		t.Errorf("failure = %q, want the missing VolumeSnapshot reported", resp.Failed)
	}
}

// A VolumeSnapshotClass can already retain what it snapshots. The source's content is still labelled, so it can be cleaned up with the PreviewSnapshot.
func TestPersistentVolumeClaimCaptureWithRetainClass(t *testing.T) {
	kind, _ := For(claimGK)
	names := NewNames("snap", "uid", claimGK, "data")
	source := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: "preview", Name: names.Name("capture")}
	target := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: "app", Name: names.Name("data")}
	contentRef := Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: "content-1"}
	c := cluster{
		source: object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata: {name: x, namespace: preview}
status: {boundVolumeSnapshotContentName: content-1, readyToUse: true}
`),
		contentRef: object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotContent
metadata: {name: content-1}
spec:
  deletionPolicy: Retain
  driver: hostpath.csi.k8s.io
  source: {volumeHandle: vol-1}
  volumeSnapshotRef: {namespace: preview, name: x}
status: {snapshotHandle: handle-1, readyToUse: true}
`),
	}
	req := CaptureRequest{
		Resource:  object(t, claim),
		Namespace: "app",
		Names:     names,
		Labels:    map[string]string{"kupra.futo.tech/snapshot-uid": "uid"},
		Observed:  Observed{source: c[source], target: nil, contentRef: c[contentRef]},
	}
	resp := kind.Capture(req)
	c.step(resp)
	if got := c[contentRef].GetLabels()["kupra.futo.tech/snapshot-uid"]; got != "uid" {
		t.Errorf("source content's label = %q, want it labelled even though it was already retained", got)
	}
}
