//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
	"github.com/futo-org/kupra/internal/testutil"
)

// finishDeletions removes the finalizers that the garbage collector and PVC protection would, which envtest doesn't run.
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

// takeSnapshot plays the snapshot controller: it creates a VolumeSnapshotContent for vs, and marks both ready.
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

// waitForVolumeSnapshot waits for the one VolumeSnapshot in a namespace with a label.
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
			return fmt.Errorf("PreviewSnapshot %s is %s, with conditions %s; want %s", name, s.Status.Phase, testutil.Conditions(s.Status.Conditions), phase)
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
		t.Errorf("Restored is %s, want it to say the preview started empty", testutil.Condition(c))
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
		waitForPhase(t, "snapshots", "source", kuprav1alpha1.PreviewStopped, controller.ReasonCapturing)

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
			t.Errorf("moved VolumeSnapshot is owned by %s, want only PreviewSnapshot %s", testutil.Owners(owners), snap.Name)
		}
		bindSnapshot(t, moved, *moved.Spec.Source.VolumeSnapshotContentName)

		s := waitForSnapshotPhase(t, "snapshots", "base-1", kuprav1alpha1.PreviewSnapshotSucceeded)
		if !s.Status.ReadyToUse || len(s.Status.Captures) != 1 || s.Status.Captures[0].Name != "data" || s.Status.Captures[0].Data[0].Name != moved.Name {
			t.Errorf("PreviewSnapshot is ready %v and captured %s; want PersistentVolumeClaim data in %s", s.Status.ReadyToUse, testutil.Captures(s.Status.Captures), moved.Name)
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
		waitForPhase(t, "snapshots", "source", kuprav1alpha1.PreviewStopped, controller.ReasonCapturing)
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

	t.Run("reset requested during a capture", func(t *testing.T) {
		during := &kuprav1alpha1.PreviewSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "during", Namespace: "snapshots"},
			Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
		}
		if err := k8s.Create(ctx, during); err != nil {
			t.Fatal(err)
		}
		captured := waitForVolumeSnapshot(t, sourceNS, controller.LabelSnapshotUID, string(during.UID))
		// kubectl annotate sets the request.
		eventually(t, func() error {
			p := getPreview(t, "snapshots", "source")
			if p.Annotations == nil {
				p.Annotations = map[string]string{}
			}
			p.Annotations[controller.AnnotationResetRequestedAt] = "during"
			return k8s.Update(ctx, p, client.FieldOwner("kubectl-annotate"))
		})
		eventually(t, func() error {
			p := getPreview(t, "snapshots", "source")
			for _, e := range p.ManagedFields {
				if e.Manager == controller.FieldManager && e.Operation == metav1.ManagedFieldsOperationApply && strings.Contains(e.FieldsV1.GetRawString(), controller.AnnotationResetRequestedAt) {
					return nil
				}
			}
			return fmt.Errorf("kupra hasn't claimed the reset request")
		})
		// Flux, which owns the Preview, takes over kubectl's fields and applies the Preview without the annotation.
		// This is what fluxcd/pkg/ssa does with kubectl's managed fields before it applies.
		eventually(t, func() error {
			p := getPreview(t, "snapshots", "source")
			fields := p.GetManagedFields()
			for i, e := range fields {
				if strings.HasPrefix(e.Manager, "kubectl") && e.Operation == metav1.ManagedFieldsOperationUpdate {
					fields[i].Manager, fields[i].Operation = "flux", metav1.ManagedFieldsOperationApply
				}
			}
			p.SetManagedFields(fields)
			return k8s.Update(ctx, p)
		})
		manifest := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": kuprav1alpha1.GroupVersion.String(),
			"kind":       "Preview",
			"metadata":   map[string]any{"name": "source", "namespace": "snapshots"},
			"spec":       map[string]any{"appRef": map[string]any{"name": "app"}},
		}}
		if err := k8s.Apply(ctx, client.ApplyConfigurationFromUnstructured(manifest), client.FieldOwner("flux"), client.ForceOwnership); err != nil {
			t.Fatal(err)
		}
		if p := getPreview(t, "snapshots", "source"); p.Annotations[controller.AnnotationResetRequestedAt] != "during" {
			t.Errorf("annotations = %v after Flux applied the Preview, want the reset request kept", p.Annotations)
		}
		takeSnapshot(t, captured, "handle-during")
		moved := waitForVolumeSnapshot(t, "snapshots", controller.LabelSnapshotUID, string(during.UID))
		bindSnapshot(t, moved, *moved.Spec.Source.VolumeSnapshotContentName)
		waitForSnapshotPhase(t, "snapshots", "during", kuprav1alpha1.PreviewSnapshotSucceeded)
		eventually(t, func() error {
			finishDeletions(t, sourceNS)
			p := getPreview(t, "snapshots", "source")
			if p.Status.LastHandledResetAt != "during" || p.Status.Phase != kuprav1alpha1.PreviewRunning {
				return fmt.Errorf("lastHandledResetAt %q, phase %s; want the reset handled", p.Status.LastHandledResetAt, p.Status.Phase)
			}
			return nil
		})
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
			t.Errorf("restore copy's content has deletion policy %s and snapshot handle %q, want Retain and handle-1", content.Spec.DeletionPolicy, *content.Spec.Source.SnapshotHandle)
		}
		var pvc corev1.PersistentVolumeClaim
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: p.Status.Namespace, Name: "data"}, &pvc); err != nil {
			t.Fatal(err)
		}
		if ref := pvc.Spec.DataSourceRef; ref == nil || ref.Kind != "VolumeSnapshot" || ref.Name != copied.Name {
			t.Errorf("claim's data source is %s, want VolumeSnapshot %s", testutil.DataSource(ref), copied.Name)
		}

		// The app starts before the claim is bound, but the restore isn't over until it is.
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
			t.Errorf("Reconciling is %s, want it to name Preview pinned", testutil.Condition(c))
		}
		ready := apimeta.FindStatusCondition(s.Status.Conditions, meta.ReadyCondition)
		if ready == nil {
			t.Fatal("PreviewSnapshot base-1 has no Ready condition")
		}
		if ready.Status != metav1.ConditionFalse || ready.Reason != controller.ReasonInUse {
			t.Errorf("Ready is %s (%s), want False (%s)", ready.Status, ready.Reason, controller.ReasonInUse)
		}
		if !strings.Contains(ready.Message, "Preview pinned") {
			t.Errorf("Ready message %q doesn't name Preview pinned", ready.Message)
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
			// The restore copies of restored, which hold this snapshot's data, go with it.
			restoredPreview := getPreview(t, "snapshots", "restored")
			var copies snapshotv1.VolumeSnapshotList
			if err := k8s.List(ctx, &copies, client.InNamespace(restoredPreview.Status.Namespace), client.MatchingLabels{controller.LabelPreviewUID: string(restoredPreview.UID)}); err != nil {
				return err
			}
			if len(copies.Items) > 0 {
				return fmt.Errorf("%d restore copies of the deleted snapshot are left", len(copies.Items))
			}
			var contents snapshotv1.VolumeSnapshotContentList
			if err := k8s.List(ctx, &contents, client.MatchingLabels{controller.LabelPreviewUID: string(restoredPreview.UID)}); err != nil {
				return err
			}
			if len(contents.Items) > 0 {
				return fmt.Errorf("%d retained contents of the deleted snapshot's restore copies are left", len(contents.Items))
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
				return fmt.Errorf("lastHandledResetAt %q, phase %s, Ready %s; want 2, Pending with SnapshotNotFound", got.Status.LastHandledResetAt, got.Status.Phase, testutil.Condition(c))
			}
			return nil
		})
	})
}

// bindClaim plays the provisioner, and marks a claim as bound.
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
			return fmt.Errorf("Preview %s is Restored %s, want %s with %q", name, testutil.Condition(c), status, message)
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
    - apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: extra
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 1Gi
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

	// The API server refuses the claim, so it isn't restored yet. Depending on how it refuses it, kupra fails the preview or retries.
	var p *kuprav1alpha1.Preview
	eventually(t, func() error {
		p = getPreview(t, "restore-retries", "retry")
		c := apimeta.FindStatusCondition(p.Status.Conditions, meta.ReadyCondition)
		if c == nil || c.Reason != controller.ReasonApplyFailed {
			return fmt.Errorf("Preview retry is Ready %s, want reason %s", testutil.Condition(c), controller.ReasonApplyFailed)
		}
		return nil
	})
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
		t.Errorf("claim's data source is %s, want the restored VolumeSnapshot", testutil.DataSource(pvc.Spec.DataSourceRef))
	}

	// While the claim is provisioning, the snapshot it restores from is held.
	waitForRestored(t, "restore-retries", "retry", metav1.ConditionFalse, "PersistentVolumeClaim data")
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(snap.Finalizers, controller.RestoreFinalizerPrefix+string(p.UID)) {
		t.Errorf("snapshot finalizers = %v, want the preview's hold", snap.Finalizers)
	}
	// With its status lost, the preview restores from the snapshot it holds, not a newer one.
	newer := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "newer", Namespace: "restore-retries", Labels: map[string]string{"kupra.futo.tech/channel": "base"}},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "gone"}}},
	}
	if err := k8s.Create(ctx, newer); err != nil {
		t.Fatal(err)
	}
	later := metav1.NewTime(now.Add(time.Hour))
	eventually(t, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(newer), newer); err != nil {
			return err
		}
		newer.Status = kuprav1alpha1.PreviewSnapshotStatus{StartTime: &later, CompletionTime: &later, Phase: kuprav1alpha1.PreviewSnapshotSucceeded, ReadyToUse: true, Captures: status.Captures}
		return k8s.Status().Update(ctx, newer)
	})
	eventually(t, func() error {
		got := getPreview(t, "restore-retries", "retry")
		got.Status.RestoredFrom = nil
		return k8s.Status().Update(ctx, got)
	})
	var recovered string
	eventually(t, func() error {
		got := getPreview(t, "restore-retries", "retry")
		if got.Status.RestoredFrom == nil {
			return fmt.Errorf("restoredFrom isn't recorded again yet")
		}
		recovered = got.Status.RestoredFrom.Name
		return nil
	})
	if recovered != "base" {
		t.Errorf("restoredFrom = %s, want base, which the preview holds", recovered)
	}
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(newer), newer); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(newer.Finalizers, controller.RestoreFinalizerPrefix+string(p.UID)) {
		t.Errorf("snapshot newer is held too: %v", newer.Finalizers)
	}

	// Removing restoreFrom doesn't end a restore that has started.
	eventually(t, func() error {
		got := getPreview(t, "restore-retries", "retry")
		got.Spec.RestoreFrom = nil
		return k8s.Update(ctx, got)
	})
	eventually(t, func() error {
		got := getPreview(t, "restore-retries", "retry")
		if got.Status.ObservedGeneration != got.Generation {
			return fmt.Errorf("Preview retry hasn't seen its new generation")
		}
		return nil
	})
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(snap.Finalizers, controller.RestoreFinalizerPrefix+string(p.UID)) {
		t.Errorf("snapshot finalizers = %v after restoreFrom was removed, want the preview's hold", snap.Finalizers)
	}

	if err := k8s.Delete(ctx, snap); err != nil {
		t.Fatal(err)
	}
	s := waitForSnapshotPhase(t, "restore-retries", "base", kuprav1alpha1.PreviewSnapshotDeleting)
	if c := apimeta.FindStatusCondition(s.Status.Conditions, meta.ReconcilingCondition); c == nil || !strings.Contains(c.Message, "Preview retry") {
		t.Errorf("Reconciling is %s, want it to name Preview retry", testutil.Condition(c))
	}

	bindClaim(t, p.Status.Namespace, "data")
	bindClaim(t, p.Status.Namespace, "extra")
	waitForRestored(t, "restore-retries", "retry", metav1.ConditionTrue, "restored from PreviewSnapshot base, without captures of PersistentVolumeClaim extra")
	eventually(t, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), &kuprav1alpha1.PreviewSnapshot{}); !apierrors.IsNotFound(err) {
			return fmt.Errorf("PreviewSnapshot base still exists (%v)", err)
		}
		return nil
	})
}

// A snapshot deleted between a preview choosing it and holding it isn't held.
func TestHoldSnapshotBeingDeleted(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "holds", nil)
	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: "holds", Finalizers: []string{"test.kupra.futo.tech/keep"}},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "gone"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	eventually(t, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
			return err
		}
		snap.Status = kuprav1alpha1.PreviewSnapshotStatus{StartTime: &now, CompletionTime: &now, Phase: kuprav1alpha1.PreviewSnapshotSucceeded, ReadyToUse: true}
		return k8s.Status().Update(ctx, snap)
	})
	if err := k8s.Delete(ctx, snap); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err == nil {
			snap.Finalizers = nil
			_ = k8s.Update(ctx, snap)
		}
	})

	p := stalePreview("chooser", "chooser-uid")
	p.Namespace = "holds"
	r := staleReconciler(t, "", nil)
	reason, err := r.HoldSnapshot(ctx, p, "base")
	if err != nil {
		t.Fatal(err)
	}
	if reason != controller.ReasonSnapshotUnusable {
		t.Errorf("reason = %q, want %s", reason, controller.ReasonSnapshotUnusable)
	}
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(snap.Finalizers, controller.RestoreFinalizerPrefix+"chooser-uid") {
		t.Errorf("snapshot finalizers = %v, want no hold on a snapshot being deleted", snap.Finalizers)
	}
}

func TestCaptureInDeletedNamespace(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t)
	createNamespace(t, "going", nil)
	createApp(t, "going", "app", `
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
            storage: 1Gi
`)
	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "going"},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
	}
	if err := k8s.Create(ctx, preview); err != nil {
		t.Fatal(err)
	}
	p := waitForPhase(t, "going", "source", kuprav1alpha1.PreviewRunning, "Succeeded")
	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: "going"},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}
	waitForVolumeSnapshot(t, p.Status.Namespace, controller.LabelSnapshotUID, string(snap.UID))

	// envtest has no namespace controller, so the namespace's objects are deleted here.
	for _, obj := range []client.Object{&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "going"}}, snap, preview} {
		if err := k8s.Delete(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), &kuprav1alpha1.PreviewSnapshot{}); !apierrors.IsNotFound(err) {
			return fmt.Errorf("PreviewSnapshot snap still exists (%v)", err)
		}
		return nil
	})
	waitForPhase(t, "going", "source", kuprav1alpha1.PreviewTerminating, controller.ReasonTerminating)
}

// A reset of a preview whose namespace is gone still clears its restore and releases its snapshot.
func TestResetWithoutNamespace(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "resets", nil)
	p := stalePreview("gone", "gone-uid")
	p.Namespace = "resets"
	p.Annotations = map[string]string{controller.AnnotationResetRequestedAt: "1"}
	p.Status.RestoredFrom = &meta.LocalObjectReference{Name: "base"}
	p.Status.Inventory = []kuprav1alpha1.InventoryEntry{{TypedLocalObjectReference: corev1.TypedLocalObjectReference{Kind: "PersistentVolumeClaim", Name: "data"}, State: true}}
	apimeta.SetStatusCondition(&p.Status.Conditions, metav1.Condition{Type: controller.ConditionRestored, Status: metav1.ConditionTrue, Reason: "Succeeded"})
	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: "resets", Finalizers: []string{controller.RestoreFinalizerPrefix + "gone-uid"}},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "gone"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}

	// The PreviewSnapshot controller adds its finalizer meanwhile, which conflicts with releasing the hold. Reconciling retries, as this does.
	eventually(t, func() error {
		done, err := staleReconciler(t, "", nil).FinishReset(ctx, p, "resets-gone-missing")
		if err != nil || !done {
			return fmt.Errorf("FinishReset = %v, %v; want done", done, err)
		}
		return nil
	})
	if p.Status.LastHandledResetAt != "1" || p.Status.RestoredFrom != nil || len(p.Status.Inventory) != 0 || apimeta.FindStatusCondition(p.Status.Conditions, controller.ConditionRestored) != nil {
		t.Errorf("lastHandledResetAt %q, restoredFrom %v, inventory %s, Restored %s; want the reset handled and the restore cleared", p.Status.LastHandledResetAt, p.Status.RestoredFrom, testutil.Inventory(p.Status.Inventory), testutil.Condition(apimeta.FindStatusCondition(p.Status.Conditions, controller.ConditionRestored)))
	}
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(snap.Finalizers, controller.RestoreFinalizerPrefix+"gone-uid") {
		t.Errorf("snapshot finalizers = %v, want the hold released", snap.Finalizers)
	}
}

// A tool that server-side applies the Preview, without forcing, can request one reset after another: kupra gives up the request once it's handled.
func TestResetsByServerSideApply(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t)
	createNamespace(t, "ssa-resets", nil)
	createApp(t, "ssa-resets", "app", `
template:
  resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: config
`)
	apply := func(reset string) error {
		u := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": kuprav1alpha1.GroupVersion.String(),
			"kind":       "Preview",
			"metadata": map[string]any{
				"name":        "pr",
				"namespace":   "ssa-resets",
				"annotations": map[string]any{controller.AnnotationResetRequestedAt: reset},
			},
			"spec": map[string]any{"appRef": map[string]any{"name": "app"}},
		}}
		return k8s.Apply(ctx, client.ApplyConfigurationFromUnstructured(u), client.FieldOwner("gitops"))
	}
	handled := func(reset string) {
		t.Helper()
		eventually(t, func() error {
			p := getPreview(t, "ssa-resets", "pr")
			if p.Status.Namespace != "" {
				finishDeletions(t, p.Status.Namespace)
			}
			if p.Status.LastHandledResetAt != reset {
				return fmt.Errorf("lastHandledResetAt = %q, want %s", p.Status.LastHandledResetAt, reset)
			}
			for _, e := range p.ManagedFields {
				if e.Manager == controller.FieldManager && strings.Contains(e.FieldsV1.GetRawString(), controller.AnnotationResetRequestedAt) {
					return fmt.Errorf("kupra still owns the handled reset request")
				}
			}
			return nil
		})
	}

	if err := apply("a"); err != nil {
		t.Fatal(err)
	}
	handled("a")
	if err := apply("b"); err != nil {
		t.Fatalf("applying the next reset request: %v", err)
	}
	handled("b")
}

// A reset request removed after kupra read the Preview isn't put back by kupra's claim on it.
func TestClaimAfterRemoval(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "claims", nil)
	p := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "pr", Namespace: "claims", Annotations: map[string]string{controller.AnnotationResetRequestedAt: "x"}},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "missing"}},
	}
	if err := k8s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	stale := p.DeepCopy()
	eventually(t, func() error {
		got := getPreview(t, "claims", "pr")
		delete(got.Annotations, controller.AnnotationResetRequestedAt)
		return k8s.Update(ctx, got)
	})

	r := &controller.PreviewReconciler{Client: k8s}
	if _, err := r.ClaimResetRequest(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if got := getPreview(t, "claims", "pr"); got.Annotations[controller.AnnotationResetRequestedAt] != "" {
		t.Errorf("annotations = %v, want the removed reset request to stay removed", got.Annotations)
	}
}

// A claim waits for its restore copy to be bound, and its preview says what the copy is waiting for.
func TestRestoreWaitsForCopy(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t)
	createNamespace(t, "copies", nil)
	createApp(t, "copies", "app", `
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
            storage: 1Gi
`)
	captured := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "captured", Namespace: "copies"},
		Spec:       snapshotv1.VolumeSnapshotSpec{Source: snapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: ptr("data")}},
	}
	if err := k8s.Create(ctx, captured); err != nil {
		t.Fatal(err)
	}
	takeSnapshot(t, captured, "handle-copies")
	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: "copies"},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "gone"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	group := snapshotv1.GroupName
	eventually(t, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
			return err
		}
		snap.Status = kuprav1alpha1.PreviewSnapshotStatus{
			StartTime: &now, CompletionTime: &now, Phase: kuprav1alpha1.PreviewSnapshotSucceeded, ReadyToUse: true,
			AppRef:   &meta.LocalObjectReference{Name: "app"},
			Captures: []kuprav1alpha1.Capture{{Kind: "PersistentVolumeClaim", Name: "data", Data: []corev1.TypedLocalObjectReference{{APIGroup: &group, Kind: "VolumeSnapshot", Name: "captured"}}}},
		}
		return k8s.Status().Update(ctx, snap)
	})

	holdRestoreCopies.Store(true)
	t.Cleanup(func() { holdRestoreCopies.Store(false) })
	restored := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "restored", Namespace: "copies"},
		Spec: kuprav1alpha1.PreviewSpec{
			AppRef:      meta.LocalObjectReference{Name: "app"},
			RestoreFrom: &kuprav1alpha1.RestoreSource{SnapshotRef: &meta.LocalObjectReference{Name: "base"}},
		},
	}
	if err := k8s.Create(ctx, restored); err != nil {
		t.Fatal(err)
	}
	ns := waitForNamespace(t, "copies", "restored")
	p := getPreview(t, "copies", "restored")
	copied := waitForVolumeSnapshot(t, ns, controller.LabelPreviewUID, string(p.UID))
	copied.Status = &snapshotv1.VolumeSnapshotStatus{Error: &snapshotv1.VolumeSnapshotError{Message: ptr("no snapshot class")}}
	if err := k8s.Status().Update(ctx, copied); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() error {
		c := apimeta.FindStatusCondition(getPreview(t, "copies", "restored").Status.Conditions, meta.ReadyCondition)
		if c == nil || !strings.Contains(c.Message, "no snapshot class") {
			return fmt.Errorf("Ready is %s, want it to say what the copy is waiting for", testutil.Condition(c))
		}
		return nil
	})
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: ns, Name: "data"}, &corev1.PersistentVolumeClaim{}); !apierrors.IsNotFound(err) {
		t.Errorf("claim: %v, want it created only once its copy is bound", err)
	}

	// The snapshot controller binds the copy, recording its UID in its content.
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(copied), copied); err != nil {
		t.Fatal(err)
	}
	var content snapshotv1.VolumeSnapshotContent
	if err := k8s.Get(ctx, client.ObjectKey{Name: *copied.Spec.Source.VolumeSnapshotContentName}, &content); err != nil {
		t.Fatal(err)
	}
	content.Spec.VolumeSnapshotRef.UID = copied.UID
	if err := k8s.Update(ctx, &content); err != nil {
		t.Fatal(err)
	}
	bindSnapshot(t, copied, content.Name)
	eventually(t, func() error {
		return k8s.Get(ctx, client.ObjectKey{Namespace: ns, Name: "data"}, &corev1.PersistentVolumeClaim{})
	})

	// The provisioner refuses a copy whose content names another snapshot, so later reconciliations leave the binding alone.
	time.Sleep(3 * time.Second)
	if err := k8s.Get(ctx, client.ObjectKeyFromObject(&content), &content); err != nil {
		t.Fatal(err)
	}
	if content.Spec.VolumeSnapshotRef.UID != copied.UID {
		t.Errorf("copy's content is bound to UID %q, want %q", content.Spec.VolumeSnapshotRef.UID, copied.UID)
	}
}
