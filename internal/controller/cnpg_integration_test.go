//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
)

func getCluster(t *testing.T, namespace, name string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("postgresql.cnpg.io/v1")
	u.SetKind("Cluster")
	eventually(t, func() error {
		return k8s.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, u)
	})
	return u
}

// markClusterReady plays CloudNativePG, which reports a cluster ready once its instances are up.
func markClusterReady(t *testing.T, namespace, name string) {
	t.Helper()
	eventually(t, func() error {
		u := getCluster(t, namespace, name)
		u.Object["status"] = map[string]any{"conditions": []any{map[string]any{
			"type": "Ready", "status": "True", "reason": "ClusterIsReady", "message": "Cluster is Ready", "lastTransitionTime": metav1.Now().UTC().Format("2006-01-02T15:04:05Z"),
		}}}
		return k8s.Status().Update(context.Background(), u)
	})
}

// waitForVolumeSnapshots waits for n VolumeSnapshots in a namespace with a label.
func waitForVolumeSnapshots(t *testing.T, namespace, label, uid string, n int) []snapshotv1.VolumeSnapshot {
	t.Helper()
	var list snapshotv1.VolumeSnapshotList
	eventually(t, func() error {
		if err := k8s.List(context.Background(), &list, client.InNamespace(namespace), client.MatchingLabels{label: uid}); err != nil {
			return err
		}
		if len(list.Items) != n {
			return fmt.Errorf("%d VolumeSnapshots in %s labelled %s, want %d", len(list.Items), namespace, label, n)
		}
		return nil
	})
	return list.Items
}

// waitForBackup waits for the Backup a PreviewSnapshot's capture asks CloudNativePG for.
func waitForBackup(t *testing.T, namespace string, snap *kuprav1alpha1.PreviewSnapshot) *unstructured.Unstructured {
	t.Helper()
	var backup *unstructured.Unstructured
	eventually(t, func() error {
		var list unstructured.UnstructuredList
		list.SetAPIVersion("postgresql.cnpg.io/v1")
		list.SetKind("BackupList")
		if err := k8s.List(context.Background(), &list, client.InNamespace(namespace), client.MatchingLabels{controller.LabelSnapshotUID: string(snap.UID)}); err != nil {
			return err
		}
		if len(list.Items) != 1 {
			return fmt.Errorf("%d Backups, want 1", len(list.Items))
		}
		backup = &list.Items[0]
		return nil
	})
	return backup
}

// waitForNamespace waits for a preview's namespace to be recorded.
func waitForNamespace(t *testing.T, namespace, name string) string {
	t.Helper()
	var ns string
	eventually(t, func() error {
		ns = getPreview(t, namespace, name).Status.Namespace
		if ns == "" {
			return fmt.Errorf("Preview %s has no namespace yet", name)
		}
		return nil
	})
	return ns
}

func TestClusterState(t *testing.T) {
	ctx := context.Background()
	setUpPolicies(t)
	createNamespace(t, "cnpg", nil)
	createApp(t, "cnpg", "app", `
template:
  resources:
    - apiVersion: postgresql.cnpg.io/v1
      kind: Cluster
      metadata:
        name: database
      spec:
        instances: 1
        storage: {size: 1Gi}
        walStorage: {size: 1Gi}
`)
	source := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "cnpg"},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}, Parameters: map[string]string{}},
	}
	if err := k8s.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	sourceNS := waitForNamespace(t, "cnpg", "source")
	database := getCluster(t, sourceNS, "database")
	if owner, _, _ := unstructured.NestedString(database.Object, "spec", "backup", "volumeSnapshot", "snapshotOwnerReference"); owner != "backup" {
		t.Errorf("cluster's snapshot owner reference = %q, want backup", owner)
	}
	markClusterReady(t, sourceNS, "database")
	waitForPhase(t, "cnpg", "source", kuprav1alpha1.PreviewRunning, "Succeeded")

	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: "cnpg"},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}

	var moved []snapshotv1.VolumeSnapshot
	if !t.Run("capture", func(t *testing.T) {
		backup := waitForBackup(t, sourceNS, snap)
		if online, _, _ := unstructured.NestedBool(backup.Object, "spec", "online"); online {
			t.Error("Backup is online, want it offline for an Offline capture")
		}

		// CloudNativePG backs the cluster up.
		var elements []any
		for _, role := range []string{"PG_DATA", "PG_WAL"} {
			vs := &snapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "database-" + strings.ReplaceAll(strings.ToLower(role), "_", "-"),
					Namespace:   sourceNS,
					Labels:      map[string]string{"cnpg.io/cluster": "database", "cnpg.io/backupName": backup.GetName(), "cnpg.io/pvcRole": role},
					Annotations: map[string]string{"cnpg.io/pvcRole": role},
				},
				Spec: snapshotv1.VolumeSnapshotSpec{Source: snapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: ptr("database-1")}},
			}
			if err := k8s.Create(ctx, vs); err != nil {
				t.Fatal(err)
			}
			takeSnapshot(t, vs, "cnpg-"+strings.ReplaceAll(strings.ToLower(role), "_", "-"))
			elements = append(elements, map[string]any{"name": vs.Name, "type": role})
		}
		backup.Object["status"] = map[string]any{"phase": "completed", "snapshotBackupStatus": map[string]any{"elements": elements}}
		if err := k8s.Status().Update(ctx, backup); err != nil {
			t.Fatal(err)
		}

		moved = waitForVolumeSnapshots(t, "cnpg", controller.LabelSnapshotUID, string(snap.UID), 2)
		for i := range moved {
			if moved[i].Labels["cnpg.io/backupName"] != backup.GetName() || moved[i].Labels["cnpg.io/cluster"] != "" {
				t.Errorf("moved VolumeSnapshot's labels = %v, want CloudNativePG's, without the cluster's", moved[i].Labels)
			}
			bindSnapshot(t, &moved[i], *moved[i].Spec.Source.VolumeSnapshotContentName)
		}
		s := waitForSnapshotPhase(t, "cnpg", "base", kuprav1alpha1.PreviewSnapshotSucceeded)
		if len(s.Status.Captures) != 1 || s.Status.Captures[0].Kind != "Cluster" || len(s.Status.Captures[0].Data) != 2 {
			t.Errorf("captures = %+v, want the cluster's two VolumeSnapshots", s.Status.Captures)
		}
		eventually(t, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(backup), backup); !apierrors.IsNotFound(err) {
				return fmt.Errorf("Backup still exists (%v)", err)
			}
			return nil
		})
	}) {
		t.FailNow()
	}

	t.Run("restore", func(t *testing.T) {
		restored := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Name: "restored", Namespace: "cnpg"},
			Spec: kuprav1alpha1.PreviewSpec{
				AppRef:      meta.LocalObjectReference{Name: "app"},
				RestoreFrom: &kuprav1alpha1.RestoreSource{SnapshotRef: &meta.LocalObjectReference{Name: "base"}},
			},
		}
		if err := k8s.Create(ctx, restored); err != nil {
			t.Fatal(err)
		}
		ns := waitForNamespace(t, "cnpg", "restored")
		p := getPreview(t, "cnpg", "restored")
		database := getCluster(t, ns, "database")
		copies := waitForVolumeSnapshots(t, ns, controller.LabelPreviewUID, string(p.UID), 2)
		for field, role := range map[string]string{"storage": "PG_DATA", "walStorage": "PG_WAL"} {
			name, _, _ := unstructured.NestedString(database.Object, "spec", "bootstrap", "recovery", "volumeSnapshots", field, "name")
			i := slices.IndexFunc(copies, func(vs snapshotv1.VolumeSnapshot) bool { return vs.Name == name })
			if i < 0 || copies[i].Labels["cnpg.io/pvcRole"] != role {
				t.Errorf("recovery %s = %q, want the copy of the %s VolumeSnapshot", field, name, role)
			}
		}
		waitForRestored(t, "cnpg", "restored", metav1.ConditionFalse, "waiting for Cluster database to be restored")
		markClusterReady(t, ns, "database")
		waitForRestored(t, "cnpg", "restored", metav1.ConditionTrue, "restored from PreviewSnapshot base")
		waitForPhase(t, "cnpg", "restored", kuprav1alpha1.PreviewRunning, "Succeeded")
	})

	t.Run("failed capture", func(t *testing.T) {
		failed := &kuprav1alpha1.PreviewSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "failed", Namespace: "cnpg"},
			Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
		}
		if err := k8s.Create(ctx, failed); err != nil {
			t.Fatal(err)
		}
		backup := waitForBackup(t, sourceNS, failed)
		backup.Object["status"] = map[string]any{"phase": "failed", "error": "no space left"}
		if err := k8s.Status().Update(ctx, backup); err != nil {
			t.Fatal(err)
		}
		waitForSnapshotPhase(t, "cnpg", "failed", kuprav1alpha1.PreviewSnapshotFailed)

		// Deleting the PreviewSnapshot deletes its Backup, and with it the VolumeSnapshots CloudNativePG took.
		if err := k8s.Delete(ctx, failed); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() error {
			if err := k8s.Get(ctx, client.ObjectKeyFromObject(backup), backup); !apierrors.IsNotFound(err) {
				return fmt.Errorf("Backup still exists (%v)", err)
			}
			return nil
		})
	})
}
