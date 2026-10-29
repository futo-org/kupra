//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
)

// finishDeletions does what envtest has no controllers for: it removes the finalizers that the garbage collector and PVC protection would, from objects being deleted in a namespace.
func finishDeletions(t *testing.T, namespace string) {
	t.Helper()
	ctx := context.Background()
	var cms corev1.ConfigMapList
	var pvcs corev1.PersistentVolumeClaimList
	if err := k8s.List(ctx, &cms, client.InNamespace(namespace)); err != nil {
		t.Fatal(err)
	}
	if err := k8s.List(ctx, &pvcs, client.InNamespace(namespace)); err != nil {
		t.Fatal(err)
	}
	var objs []client.Object
	for i := range cms.Items {
		objs = append(objs, &cms.Items[i])
	}
	for i := range pvcs.Items {
		objs = append(objs, &pvcs.Items[i])
	}
	for _, obj := range objs {
		if obj.GetDeletionTimestamp().IsZero() || len(obj.GetFinalizers()) == 0 {
			continue
		}
		obj.SetFinalizers(nil)
		if err := k8s.Update(ctx, obj); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
			t.Fatal(err)
		}
	}
}

// takeSnapshot plays the snapshot controller for a VolumeSnapshot of a claim: it creates the VolumeSnapshotContent and marks both ready.
func takeSnapshot(t *testing.T, vs *snapshotv1.VolumeSnapshot, handle string) {
	t.Helper()
	ctx := context.Background()
	content := &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: "content-" + handle},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			DeletionPolicy:    snapshotv1.VolumeSnapshotContentDelete,
			Driver:            "hostpath.csi.k8s.io",
			Source:            snapshotv1.VolumeSnapshotContentSource{VolumeHandle: ptr("volume-" + handle)},
			VolumeSnapshotRef: corev1.ObjectReference{Namespace: vs.Namespace, Name: vs.Name},
		},
	}
	if err := k8s.Create(ctx, content); err != nil {
		t.Fatal(err)
	}
	content.Status = &snapshotv1.VolumeSnapshotContentStatus{SnapshotHandle: ptr(handle), ReadyToUse: ptr(true)}
	if err := k8s.Status().Update(ctx, content); err != nil {
		t.Fatal(err)
	}
	bindSnapshot(t, vs, content.Name)
}

// bindSnapshot marks a VolumeSnapshot as bound to a content, and ready.
func bindSnapshot(t *testing.T, vs *snapshotv1.VolumeSnapshot, contentName string) {
	t.Helper()
	vs.Status = &snapshotv1.VolumeSnapshotStatus{
		BoundVolumeSnapshotContentName: ptr(contentName),
		ReadyToUse:                     ptr(true),
	}
	if err := k8s.Status().Update(context.Background(), vs); err != nil {
		t.Fatal(err)
	}
}

// waitForVolumeSnapshot waits for a VolumeSnapshot that a PreviewSnapshot or Preview has labelled as its own, in a namespace.
func waitForVolumeSnapshot(t *testing.T, namespace, label, uid string) *snapshotv1.VolumeSnapshot {
	t.Helper()
	var found snapshotv1.VolumeSnapshot
	eventually(t, func() error {
		finishDeletions(t, namespace)
		var list snapshotv1.VolumeSnapshotList
		if err := k8s.List(context.Background(), &list, client.InNamespace(namespace), client.MatchingLabels{label: uid}); err != nil {
			return err
		}
		if len(list.Items) != 1 {
			return fmt.Errorf("%d VolumeSnapshots in %s labelled %s, want 1", len(list.Items), namespace, label)
		}
		found = list.Items[0]
		return nil
	})
	return &found
}

func waitForSnapshotPhase(t *testing.T, namespace, name string, phase kuprav1alpha1.PreviewSnapshotPhase) *kuprav1alpha1.PreviewSnapshot {
	t.Helper()
	var s kuprav1alpha1.PreviewSnapshot
	eventually(t, func() error {
		if err := k8s.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, &s); err != nil {
			return err
		}
		if s.Status.Phase != phase {
			return fmt.Errorf("PreviewSnapshot %s: phase %s, conditions %+v; want %s", name, s.Status.Phase, s.Status.Conditions, phase)
		}
		return nil
	})
	return &s
}

func TestSnapshots(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t)
	createNamespace(t, "snapshots", nil)
	createApp(t, "snapshots", "app", `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
      data: {greeting: hello}
    - apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: data
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 1Gi
`)
	source := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "snapshots"},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}, Parameters: map[string]string{}},
	}
	if err := k8s.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	p := waitForPhase(t, "snapshots", "source", kuprav1alpha1.PreviewRunning, "Succeeded")
	sourceNS := p.Status.Namespace
	if c := apimeta.FindStatusCondition(p.Status.Conditions, controller.ConditionRestored); c == nil || c.Message != "started empty" {
		t.Errorf("Restored = %+v, want it to say the preview started empty", c)
	}

	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base-1", Namespace: "snapshots", Labels: map[string]string{"kupra.futo.tech/channel": "base"}},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}

	t.Run("capture", func(t *testing.T) {
		// An Offline capture stops the app before it captures anything.
		captured := waitForVolumeSnapshot(t, sourceNS, controller.LabelSnapshotUID, string(snap.UID))
		var cm corev1.ConfigMap
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: sourceNS, Name: "config"}, &cm); !apierrors.IsNotFound(err) {
			t.Errorf("ConfigMap config: %v; want it removed while the preview is captured", err)
		}
		if p := getPreview(t, "snapshots", "source"); p.Status.Phase != kuprav1alpha1.PreviewStopped {
			t.Errorf("source preview phase = %s, want Stopped", p.Status.Phase)
		}

		takeSnapshot(t, captured, "handle-1")
		moved := waitForVolumeSnapshot(t, "snapshots", controller.LabelSnapshotUID, string(snap.UID))
		var sourceContent snapshotv1.VolumeSnapshotContent
		if err := k8s.Get(ctx, client.ObjectKey{Name: "content-handle-1"}, &sourceContent); err != nil {
			t.Fatal(err)
		}
		if sourceContent.Spec.DeletionPolicy != snapshotv1.VolumeSnapshotContentRetain {
			t.Errorf("source content's deletion policy = %s, want Retain", sourceContent.Spec.DeletionPolicy)
		}
		if owners := moved.OwnerReferences; len(owners) != 1 || owners[0].UID != snap.UID {
			t.Errorf("moved VolumeSnapshot's owners = %+v, want the PreviewSnapshot", owners)
		}
		bindSnapshot(t, moved, *moved.Spec.Source.VolumeSnapshotContentName)

		s := waitForSnapshotPhase(t, "snapshots", "base-1", kuprav1alpha1.PreviewSnapshotSucceeded)
		if !s.Status.ReadyToUse || len(s.Status.Captures) != 1 || s.Status.Captures[0].Name != "data" || s.Status.Captures[0].Data[0].Name != moved.Name {
			t.Errorf("snapshot status = %+v, want a capture of data in %s", s.Status, moved.Name)
		}
		if s.Status.AppRef == nil || s.Status.AppRef.Name != "app" {
			t.Errorf("snapshot appRef = %v, want app", s.Status.AppRef)
		}
		eventually(t, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(captured), &snapshotv1.VolumeSnapshot{}); !apierrors.IsNotFound(err) {
				return fmt.Errorf("source VolumeSnapshot still exists (%v)", err)
			}
			return nil
		})
		waitForPhase(t, "snapshots", "source", kuprav1alpha1.PreviewRunning, "Succeeded")
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: sourceNS, Name: "config"}, &cm); err != nil {
			t.Errorf("ConfigMap config after the capture: %v", err)
		}
	})

	t.Run("deleted during a capture", func(t *testing.T) {
		doomed := &kuprav1alpha1.PreviewSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "doomed", Namespace: "snapshots"},
			Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
		}
		if err := k8s.Create(ctx, doomed); err != nil {
			t.Fatal(err)
		}
		captured := waitForVolumeSnapshot(t, sourceNS, controller.LabelSnapshotUID, string(doomed.UID))
		if err := k8s.Delete(ctx, doomed); err != nil {
			t.Fatal(err)
		}
		// The capture that has started still finishes, with the app stopped until it does.
		if p := getPreview(t, "snapshots", "source"); p.Status.Phase != kuprav1alpha1.PreviewStopped {
			t.Errorf("source preview phase = %s, want Stopped", p.Status.Phase)
		}
		takeSnapshot(t, captured, "handle-doomed")
		moved := waitForVolumeSnapshot(t, "snapshots", controller.LabelSnapshotUID, string(doomed.UID))
		bindSnapshot(t, moved, *moved.Spec.Source.VolumeSnapshotContentName)
		eventually(t, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(doomed), &kuprav1alpha1.PreviewSnapshot{}); !apierrors.IsNotFound(err) {
				return fmt.Errorf("PreviewSnapshot doomed still exists (%v)", err)
			}
			return nil
		})
		waitForPhase(t, "snapshots", "source", kuprav1alpha1.PreviewRunning, "Succeeded")
	})

	restored := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "restored", Namespace: "snapshots"},
		Spec: kuprav1alpha1.PreviewSpec{
			AppRef: meta.LocalObjectReference{Name: "app"},
			RestoreFrom: &kuprav1alpha1.RestoreSource{
				SnapshotSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kupra.futo.tech/channel": "base"}},
			},
		},
	}

	t.Run("restore", func(t *testing.T) {
		if err := k8s.Create(ctx, restored); err != nil {
			t.Fatal(err)
		}
		p := waitForPhase(t, "snapshots", "restored", kuprav1alpha1.PreviewRunning, "Succeeded")
		if p.Status.RestoredFrom == nil || p.Status.RestoredFrom.Name != "base-1" {
			t.Errorf("restoredFrom = %v, want base-1", p.Status.RestoredFrom)
		}
		copied := waitForVolumeSnapshot(t, p.Status.Namespace, controller.LabelPreviewUID, string(p.UID))
		var content snapshotv1.VolumeSnapshotContent
		if err := k8s.Get(ctx, client.ObjectKey{Name: *copied.Spec.Source.VolumeSnapshotContentName}, &content); err != nil {
			t.Fatal(err)
		}
		if content.Spec.DeletionPolicy != snapshotv1.VolumeSnapshotContentRetain || *content.Spec.Source.SnapshotHandle != "handle-1" {
			t.Errorf("restore copy = %+v, want handle-1, retained", content.Spec)
		}
		var pvc corev1.PersistentVolumeClaim
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: p.Status.Namespace, Name: "data"}, &pvc); err != nil {
			t.Fatal(err)
		}
		if ref := pvc.Spec.DataSourceRef; ref == nil || ref.Kind != "VolumeSnapshot" || ref.Name != copied.Name {
			t.Errorf("claim's data source = %+v, want %s", ref, copied.Name)
		}

		// The app runs on the claim before it's bound, as storage that binds on first use needs, but the restore isn't over until then.
		waitForRestored(t, "snapshots", "restored", metav1.ConditionFalse, "PersistentVolumeClaim data")
		bindClaim(t, p.Status.Namespace, "data")
		waitForRestored(t, "snapshots", "restored", metav1.ConditionTrue, "restored from PreviewSnapshot base-1")
	})

	t.Run("reset", func(t *testing.T) {
		p := getPreview(t, "snapshots", "restored")
		var before corev1.PersistentVolumeClaim
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: p.Status.Namespace, Name: "data"}, &before); err != nil {
			t.Fatal(err)
		}
		p.Annotations = map[string]string{controller.AnnotationResetRequestedAt: "1"}
		if err := k8s.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			finishDeletions(t, p.Status.Namespace)
			got := getPreview(t, "snapshots", "restored")
			if got.Status.LastHandledResetAt != "1" || got.Status.Phase != kuprav1alpha1.PreviewRunning {
				return fmt.Errorf("lastHandledResetAt %q, phase %s; want 1, Running", got.Status.LastHandledResetAt, got.Status.Phase)
			}
			var after corev1.PersistentVolumeClaim
			if err := k8s.Get(ctx, client.ObjectKey{Namespace: p.Status.Namespace, Name: "data"}, &after); err != nil {
				return err
			}
			if after.UID == before.UID {
				return fmt.Errorf("claim data wasn't recreated")
			}
			if after.Spec.DataSourceRef == nil {
				return fmt.Errorf("claim data wasn't restored")
			}
			return nil
		})
		bindClaim(t, p.Status.Namespace, "data")
		waitForRestored(t, "snapshots", "restored", metav1.ConditionTrue, "restored from PreviewSnapshot base-1")
	})

	t.Run("deletion is held while in use", func(t *testing.T) {
		pinned := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: "pinned", Namespace: "snapshots"},
			Spec: kuprav1alpha1.PreviewSpec{
				AppRef:      meta.LocalObjectReference{Name: "app"},
				RestoreFrom: &kuprav1alpha1.RestoreSource{SnapshotRef: &meta.LocalObjectReference{Name: "base-1"}},
			},
		}
		if err := k8s.Create(ctx, pinned); err != nil {
			t.Fatal(err)
		}
		waitForPhase(t, "snapshots", "pinned", kuprav1alpha1.PreviewRunning, "Succeeded")
		if err := k8s.Delete(ctx, snap); err != nil {
			t.Fatal(err)
		}
		s := waitForSnapshotPhase(t, "snapshots", "base-1", kuprav1alpha1.PreviewSnapshotDeleting)
		if c := apimeta.FindStatusCondition(s.Status.Conditions, meta.ReconcilingCondition); c == nil || !strings.Contains(c.Message, "Preview pinned") {
			t.Errorf("Reconciling = %+v, want it to name Preview pinned", c)
		}
		if err := k8s.Delete(ctx, pinned); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), &kuprav1alpha1.PreviewSnapshot{}); !apierrors.IsNotFound(err) {
				return fmt.Errorf("PreviewSnapshot base-1 still exists (%v)", err)
			}
			if err := k8s.Get(ctx, client.ObjectKey{Name: "content-handle-1"}, &snapshotv1.VolumeSnapshotContent{}); !apierrors.IsNotFound(err) {
				return fmt.Errorf("the source's retained content still exists (%v)", err)
			}
			return nil
		})
	})

	t.Run("reset without a snapshot to restore from", func(t *testing.T) {
		p := getPreview(t, "snapshots", "restored")
		p.Annotations[controller.AnnotationResetRequestedAt] = "2"
		if err := k8s.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			finishDeletions(t, p.Status.Namespace)
			got := getPreview(t, "snapshots", "restored")
			c := apimeta.FindStatusCondition(got.Status.Conditions, meta.ReadyCondition)
			if got.Status.LastHandledResetAt != "2" || got.Status.Phase != kuprav1alpha1.PreviewPending || c == nil || c.Reason != controller.ReasonSnapshotNotFound {
				return fmt.Errorf("lastHandledResetAt %q, phase %s, Ready %+v; want 2, Pending with SnapshotNotFound", got.Status.LastHandledResetAt, got.Status.Phase, c)
			}
			return nil
		})
	})
}

// bindClaim plays the provisioner: it marks a claim as bound, which is when a claim restored from a snapshot has taken in its data.
func bindClaim(t *testing.T, namespace, name string) {
	t.Helper()
	eventually(t, func() error {
		var pvc corev1.PersistentVolumeClaim
		if err := k8s.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, &pvc); err != nil {
			return err
		}
		pvc.Status.Phase = corev1.ClaimBound
		return k8s.Status().Update(context.Background(), &pvc)
	})
}

// waitForRestored waits for a preview's Restored condition to have a status and a message.
func waitForRestored(t *testing.T, namespace, name string, status metav1.ConditionStatus, message string) {
	t.Helper()
	eventually(t, func() error {
		p := getPreview(t, namespace, name)
		c := apimeta.FindStatusCondition(p.Status.Conditions, controller.ConditionRestored)
		if c == nil || c.Status != status || !strings.Contains(c.Message, message) {
			return fmt.Errorf("Preview %s: Restored %+v, want %s with %q", name, c, status, message)
		}
		return nil
	})
}

func TestRestoreRetries(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t)
	createNamespace(t, "restore-retries", nil)
	createApp(t, "restore-retries", "app", `
parameters:
  - name: size
    title: Size
template:
  resources:
    - apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: data
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storageExpr: parameters.size
`)

	// A snapshot that captured the claim, as if kupra had taken it.
	captured := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "captured", Namespace: "restore-retries"},
		Spec:       snapshotv1.VolumeSnapshotSpec{Source: snapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: ptr("data")}},
	}
	if err := k8s.Create(ctx, captured); err != nil {
		t.Fatal(err)
	}
	takeSnapshot(t, captured, "handle-retry")
	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: "restore-retries", Labels: map[string]string{"kupra.futo.tech/channel": "base"}},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "gone"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	status := kuprav1alpha1.PreviewSnapshotStatus{
		StartTime:      &now,
		CompletionTime: &now,
		Phase:          kuprav1alpha1.PreviewSnapshotSucceeded,
		ReadyToUse:     true,
		Captures: []kuprav1alpha1.Capture{{
			Kind: "PersistentVolumeClaim",
			Name: "data",
			Data: []corev1.TypedLocalObjectReference{{APIGroup: ptr("snapshot.storage.k8s.io"), Kind: "VolumeSnapshot", Name: "captured"}},
		}},
	}
	// The snapshot's own controller may write to it meanwhile.
	eventually(t, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
			return err
		}
		snap.Status = status
		return k8s.Status().Update(ctx, snap)
	})

	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "retry", Namespace: "restore-retries"},
		Spec: kuprav1alpha1.PreviewSpec{
			AppRef:     meta.LocalObjectReference{Name: "app"},
			Parameters: map[string]string{"size": "lots"},
			RestoreFrom: &kuprav1alpha1.RestoreSource{
				SnapshotSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kupra.futo.tech/channel": "base"}},
			},
		},
	}
	if err := k8s.Create(ctx, preview); err != nil {
		t.Fatal(err)
	}

	// The API server refuses the claim, so it isn't restored yet.
	p := waitForPhase(t, "restore-retries", "retry", kuprav1alpha1.PreviewStarting, controller.ReasonApplyFailed)
	if apimeta.IsStatusConditionTrue(p.Status.Conditions, controller.ConditionRestored) {
		t.Error("Restored is true before the claim exists")
	}

	// Once the claim is valid, it's still restored from the snapshot.
	p.Spec.Parameters["size"] = "1Gi"
	if err := k8s.Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	var pvc corev1.PersistentVolumeClaim
	eventually(t, func() error {
		return k8s.Get(ctx, client.ObjectKey{Namespace: p.Status.Namespace, Name: "data"}, &pvc)
	})
	if pvc.Spec.DataSourceRef == nil || pvc.Spec.DataSourceRef.Kind != "VolumeSnapshot" {
		t.Errorf("claim's data source = %+v, want the restored VolumeSnapshot", pvc.Spec.DataSourceRef)
	}

	// While the claim is provisioning, the snapshot it restores from is held.
	waitForRestored(t, "restore-retries", "retry", metav1.ConditionFalse, "PersistentVolumeClaim data")
	if err := k8s.Delete(ctx, snap); err != nil {
		t.Fatal(err)
	}
	s := waitForSnapshotPhase(t, "restore-retries", "base", kuprav1alpha1.PreviewSnapshotDeleting)
	if c := apimeta.FindStatusCondition(s.Status.Conditions, meta.ReconcilingCondition); c == nil || !strings.Contains(c.Message, "Preview retry") {
		t.Errorf("Reconciling = %+v, want it to name Preview retry", c)
	}

	bindClaim(t, p.Status.Namespace, "data")
	waitForRestored(t, "restore-retries", "retry", metav1.ConditionTrue, "restored from PreviewSnapshot base")
	eventually(t, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), &kuprav1alpha1.PreviewSnapshot{}); !apierrors.IsNotFound(err) {
			return fmt.Errorf("PreviewSnapshot base still exists (%v)", err)
		}
		return nil
	})
}
