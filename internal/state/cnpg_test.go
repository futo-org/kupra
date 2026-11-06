package state

import (
	"strings"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"

	"github.com/futo-org/kupra/internal/names"
)

var clusterGK = schema.GroupKind{Group: "postgresql.cnpg.io", Kind: "Cluster"}

const database = `
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata: {name: database, namespace: preview}
spec:
  instances: 1
  storage: {size: 1Gi}
  walStorage: {size: 512Mi}
  bootstrap:
    initdb: {encoding: UTF8}
`

func TestClusterCheck(t *testing.T) {
	kind, ok := For(clusterGK)
	if !ok {
		t.Fatal("clusters aren't supported")
	}
	for _, tc := range []struct {
		name     string
		resource string
		want     []string
	}{
		{"plain", `spec: {instances: 1, storage: {size: 1Gi}}`, nil},
		{"initdb options", `spec: {bootstrap: {initdb: {database: app, owner: app, localeCollate: C}}}`, nil},
		{"snapshot classes", `spec: {backup: {volumeSnapshot: {className: a, walClassNameExpr: '"b"'}}}`, nil},
		{"spec expression", `specExpr: '{}'`, []string{"specExpr"}},
		{"own backups", `spec: {backup: {barmanObjectStore: {}, volumeSnapshot: {online: false}}}`, []string{"spec.backup.barmanObjectStore", "spec.backup.volumeSnapshot.online"}},
		{"backup expression", `spec: {backupExpr: '{}'}`, []string{"spec.backupExpr"}},
		{"other bootstrap", `spec: {bootstrap: {recovery: {}, pg_basebackup: {}}}`, []string{"spec.bootstrap.pg_basebackup", "spec.bootstrap.recovery"}},
		{"renamed", `spec: {bootstrap: {initdb: {database: immich, ownerExpr: '"immich"', secret: {name: s}}}}`, []string{"spec.bootstrap.initdb.database", "spec.bootstrap.initdb.ownerExpr", "spec.bootstrap.initdb.secret"}},
		{"WAL archiver", `spec: {plugins: [{name: barman-cloud.cloudnative-pg.io}, {name: barman-cloud.cloudnative-pg.io, isWALArchiver: true}]}`, []string{"spec.plugins[1].isWALArchiver"}},
		{"tablespaces", `spec: {tablespaces: [{name: t}]}`, []string{"spec.tablespaces"}},
		{"replica", `spec: {replica: {enabled: true}}`, []string{"spec.replica"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resource map[string]any
			if err := yaml.Unmarshal([]byte(tc.resource), &resource); err != nil {
				t.Fatal(err)
			}
			errs := kind.Check(resource, field.NewPath("r"))
			if len(errs) != len(tc.want) {
				t.Fatalf("errors = %v, want %d", errs, len(tc.want))
			}
			for i, want := range tc.want {
				if errs[i].Field != "r."+want {
					t.Errorf("error %d is about %s, want %s", i, errs[i].Field, want)
				}
			}
		})
	}
}

func TestClusterReady(t *testing.T) {
	kind, _ := For(clusterGK)
	if kind.Ready(nil) {
		t.Error("a missing cluster is ready")
	}
	for status, want := range map[string]bool{"True": true, "False": false} {
		live := object(t, `status: {conditions: [{type: Ready, status: "`+status+`"}]}`)
		if got := kind.Ready(live); got != want {
			t.Errorf("Ready with condition %s = %v, want %v", status, got, want)
		}
		if got := kind.Restored(live); got != want {
			t.Errorf("Restored with condition %s = %v, want %v", status, got, want)
		}
	}
}

// backupSnapshot is a VolumeSnapshot CloudNativePG took for a backup, bound to a ready content.
func backupSnapshot(t *testing.T, c cluster, name, role string) {
	t.Helper()
	c[Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: "preview", Name: name}] = object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: `+name+`
  namespace: preview
  labels: {cnpg.io/cluster: database, cnpg.io/backupName: backup, app.kubernetes.io/name: postgresql}
  annotations: {cnpg.io/pvcRole: `+role+`, cnpg.io/backupLabelFile: bGFiZWw=, kubectl.kubernetes.io/last-applied-configuration: x}
status: {boundVolumeSnapshotContentName: content-`+name+`, readyToUse: true, restoreSize: 2Gi}
`)
	c[Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: "content-" + name}] = object(t, `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotContent
metadata: {name: content-`+name+`}
spec:
  deletionPolicy: Delete
  driver: hostpath.csi.k8s.io
  source: {volumeHandle: vol-`+name+`}
  volumeSnapshotRef: {namespace: preview, name: `+name+`}
status: {snapshotHandle: handle-`+name+`, readyToUse: true}
`)
}

func TestClusterCapture(t *testing.T) {
	kind, _ := For(clusterGK)
	c := cluster{}
	req := CaptureRequest{
		Resource:  object(t, database),
		Namespace: "app",
		Names:     names.ForResource("snap", "uid", clusterGK, "database"),
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
	backup := c.find("Backup", "preview")
	if backup == nil {
		t.Fatal("no Backup of the cluster")
	}
	for _, want := range [][2]any{
		{"spec.cluster.name", "database"},
		{"spec.method", "volumeSnapshot"},
		{"spec.online", false},
	} {
		if got, _, _ := unstructured.NestedFieldNoCopy(backup.Object, strings.Split(want[0].(string), ".")...); got != want[1] {
			t.Errorf("Backup %s = %v, want %v", want[0], got, want[1])
		}
	}
	if resp := run(); len(resp.Apply) > 0 {
		t.Fatalf("applied %v before the Backup completed", resp.Apply)
	}

	// CloudNativePG takes the backup.
	backupSnapshot(t, c, "database-1", "PG_DATA")
	backupSnapshot(t, c, "database-1-wal", "PG_WAL")
	_ = unstructured.SetNestedField(backup.Object, map[string]any{
		"phase": "completed",
		"snapshotBackupStatus": map[string]any{"elements": []any{
			map[string]any{"name": "database-1", "type": "PG_DATA"},
			map[string]any{"name": "database-1-wal", "type": "PG_WAL"},
		}},
	}, "status")

	for range 4 {
		run()
	}
	for _, name := range []string{"database-1", "database-1-wal"} {
		content := c[Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: "content-" + name}]
		if policy, _, _ := unstructured.NestedString(content.Object, "spec", "deletionPolicy"); policy != "Retain" {
			t.Errorf("content of %s has deletion policy %s, want Retain", name, policy)
		}
	}
	targets := map[string]*unstructured.Unstructured{}
	for ref, obj := range c {
		if ref.Kind == "VolumeSnapshot" && ref.Namespace == "app" {
			targets[obj.GetAnnotations()["cnpg.io/pvcRole"]] = obj
		}
	}
	if len(targets) != 2 || targets["PG_DATA"] == nil || targets["PG_WAL"] == nil {
		t.Fatalf("moved VolumeSnapshots by role = %v, want PG_DATA and PG_WAL", targets)
	}
	data := targets["PG_DATA"]
	if data.GetLabels()["cnpg.io/backupName"] != "backup" || data.GetAnnotations()["cnpg.io/backupLabelFile"] != "bGFiZWw=" || data.GetLabels()["kupra.futo.tech/snapshot-uid"] != "uid" {
		t.Errorf("moved VolumeSnapshot has labels %v and annotations %v, want CloudNativePG's and kupra's", data.GetLabels(), data.GetAnnotations())
	}
	for _, other := range []string{data.GetLabels()["cnpg.io/cluster"], data.GetLabels()["app.kubernetes.io/name"], data.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]} {
		if other != "" {
			t.Errorf("moved VolumeSnapshot has labels %v and annotations %v, want only CloudNativePG's and kupra's, without the cluster's", data.GetLabels(), data.GetAnnotations())
		}
	}

	// The snapshot controller binds the copies.
	for _, target := range targets {
		_ = unstructured.SetNestedField(target.Object, map[string]any{"readyToUse": true}, "status")
	}
	var resp Response
	for range 3 {
		if resp = run(); resp.Done {
			break
		}
	}
	if !resp.Done {
		t.Fatal("capture isn't done once the copies are ready")
	}
	if c.find("Backup", "preview") != nil {
		t.Error("the Backup is left in the preview's namespace")
	}
	if len(resp.Capture.Data) != 2 || resp.Capture.Data[0].Name != targets["PG_DATA"].GetName() || resp.Capture.Data[1].Name != targets["PG_WAL"].GetName() {
		t.Errorf("capture = %+v, want the moved VolumeSnapshots", resp.Capture)
	}
}

func TestClusterCaptureOnline(t *testing.T) {
	kind, _ := For(clusterGK)
	resp := kind.Capture(CaptureRequest{Resource: object(t, database), Namespace: "app", Online: true, Names: names.ForResource("snap", "uid", clusterGK, "database")})
	c := cluster{}
	resp = kind.Capture(CaptureRequest{Resource: object(t, database), Namespace: "app", Online: true, Names: names.ForResource("snap", "uid", clusterGK, "database"), Observed: c.step(resp)})
	if len(resp.Apply) != 1 {
		t.Fatalf("applied %v, want a Backup", resp.Apply)
	}
	backup := resp.Apply[0]
	online, _, _ := unstructured.NestedBool(backup.Object, "spec", "online")
	immediate, _, _ := unstructured.NestedBool(backup.Object, "spec", "onlineConfiguration", "immediateCheckpoint")
	if !online || !immediate {
		t.Errorf("Backup spec = %v, want an online backup with an immediate checkpoint", backup.Object["spec"])
	}
}

func TestClusterCaptureFailedBackup(t *testing.T) {
	kind, _ := For(clusterGK)
	n := names.ForResource("snap", "uid", clusterGK, "database")
	backup := Ref{GroupVersionKind: backupGVK, Namespace: "preview", Name: n.Name("capture")}
	observed := Observed{
		backup: object(t, `status: {phase: failed, error: no volume snapshot class}`),
		{GroupVersionKind: volumeSnapshotGVK, Namespace: "app", Name: n.Name("pg-data")}: nil,
		{GroupVersionKind: volumeSnapshotGVK, Namespace: "app", Name: n.Name("pg-wal")}:  nil,
	}
	resp := kind.Capture(CaptureRequest{Resource: object(t, database), Namespace: "app", Names: n, Observed: observed})
	if !strings.Contains(resp.Failed, "no volume snapshot class") {
		t.Errorf("failure = %q, want the Backup's error", resp.Failed)
	}
}

func TestClusterRestore(t *testing.T) {
	kind, _ := For(clusterGK)
	group := "snapshot.storage.k8s.io"
	c := cluster{}
	backupSnapshot(t, c, "data", "PG_DATA")
	backupSnapshot(t, c, "wal", "PG_WAL")
	req := RestoreRequest{
		Resource: object(t, database),
		From: &Source{Namespace: "preview", Data: []corev1.TypedLocalObjectReference{
			{APIGroup: &group, Kind: "VolumeSnapshot", Name: "data"},
			{APIGroup: &group, Kind: "VolumeSnapshot", Name: "wal"},
		}},
		Names:  names.ForResource("pr-1", "uid", clusterGK, "database"),
		Labels: map[string]string{"kupra.futo.tech/preview-uid": "uid"},
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

	sources, _, _ := unstructured.NestedMap(resp.Resource.Object, "spec", "bootstrap")
	recovery, _, _ := unstructured.NestedMap(sources, "recovery", "volumeSnapshots")
	if len(sources) != 1 || len(recovery) != 2 {
		t.Fatalf("bootstrap = %v, want a recovery from the storage and WAL VolumeSnapshots alone", sources)
	}
	for field, role := range map[string]string{"storage": "PG_DATA", "walStorage": "PG_WAL"} {
		name, _, _ := unstructured.NestedString(recovery, field, "name")
		copied := c[Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: "preview", Name: name}]
		if copied == nil {
			t.Errorf("no VolumeSnapshot %s for %s", name, field)
			continue
		}
		if copied.GetAnnotations()["cnpg.io/pvcRole"] != role || copied.GetAnnotations()["cnpg.io/backupLabelFile"] == "" || copied.GetLabels()["kupra.futo.tech/preview-uid"] != "uid" {
			t.Errorf("%s copy has labels %v and annotations %v, want CloudNativePG's and kupra's", field, copied.GetLabels(), copied.GetAnnotations())
		}
		contentName, _, _ := unstructured.NestedString(copied.Object, "spec", "source", "volumeSnapshotContentName")
		content := c[Ref{GroupVersionKind: volumeSnapshotContentGVK, Name: contentName}]
		if policy, _, _ := unstructured.NestedString(content.Object, "spec", "deletionPolicy"); policy != "Retain" {
			t.Errorf("%s copy's deletion policy = %s, want Retain", field, policy)
		}
		if size, _, _ := unstructured.NestedString(resp.Resource.Object, "spec", field, "size"); size != "2Gi" {
			t.Errorf("%s size = %s, want the snapshot's 2Gi", field, size)
		}
	}
	if owner, _, _ := unstructured.NestedString(resp.Resource.Object, "spec", "backup", "volumeSnapshot", "snapshotOwnerReference"); owner != "backup" {
		t.Errorf("snapshot owner reference = %q, want backup", owner)
	}

	// Once the cluster exists, updates keep its bootstrap and sizes.
	again := kind.Restore(RestoreRequest{Resource: object(t, database), Live: resp.Resource, From: req.From, Names: req.Names})
	if !again.Done || len(again.Apply) > 0 || len(again.Observe) > 0 {
		t.Fatalf("restoring an existing cluster = %+v, want it done straight away", again)
	}
	if _, ok, _ := unstructured.NestedMap(again.Resource.Object, "spec", "bootstrap", "recovery"); !ok {
		t.Error("updated cluster's recovery bootstrap isn't kept")
	}
	if size, _, _ := unstructured.NestedString(again.Resource.Object, "spec", "walStorage", "size"); size != "2Gi" {
		t.Errorf("updated cluster's WAL size = %s, want it kept at 2Gi", size)
	}
}

func TestClusterRestoreWithoutWALStorage(t *testing.T) {
	kind, _ := For(clusterGK)
	group := "snapshot.storage.k8s.io"
	c := cluster{}
	backupSnapshot(t, c, "data", "PG_DATA")
	resource := object(t, database)
	unstructured.RemoveNestedField(resource.Object, "spec", "walStorage")
	req := RestoreRequest{
		Resource: resource,
		From:     &Source{Namespace: "preview", Data: []corev1.TypedLocalObjectReference{{APIGroup: &group, Kind: "VolumeSnapshot", Name: "data"}}},
		Names:    names.ForResource("pr-1", "uid", clusterGK, "database"),
	}
	var resp Response
	for range 4 {
		if resp = kind.Restore(req); resp.Done || resp.Failed != "" {
			break
		}
		req.Observed = c.step(resp)
	}
	if !resp.Done {
		t.Fatalf("restore = %+v, want it done", resp)
	}
	if _, ok, _ := unstructured.NestedMap(resp.Resource.Object, "spec", "bootstrap", "recovery", "volumeSnapshots", "walStorage"); ok {
		t.Error("recovery has WAL storage the cluster doesn't")
	}

	// A cluster with WAL storage needs a capture of it.
	req.Resource = object(t, database)
	if resp := kind.Restore(req); !strings.Contains(resp.Failed, "no PG_WAL VolumeSnapshot") {
		t.Errorf("failure = %q, want the missing WAL capture reported", resp.Failed)
	}
}

// CloudNativePG moved the role from a label to an annotation, and reads the label first.
func TestRoleOf(t *testing.T) {
	for _, tc := range []struct {
		name                string
		labels, annotations map[string]string
		want                string
	}{
		{"annotation", nil, map[string]string{"cnpg.io/pvcRole": "PG_WAL"}, "PG_WAL"},
		{"label", map[string]string{"cnpg.io/pvcRole": "PG_DATA"}, nil, "PG_DATA"},
		{"both", map[string]string{"cnpg.io/pvcRole": "PG_DATA"}, map[string]string{"cnpg.io/pvcRole": "PG_WAL"}, "PG_DATA"},
	} {
		vs := &snapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Labels: tc.labels, Annotations: tc.annotations}}
		if got := roleOf(vs); string(got) != tc.want {
			t.Errorf("%s: role %s, want %s", tc.name, got, tc.want)
		}
	}
}
