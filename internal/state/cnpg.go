package state

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

var backupGVK = cnpgv1.SchemeGroupVersion.WithKind("Backup")

const (
	// cnpgPrefix starts the labels and annotations CloudNativePG puts on a backup's VolumeSnapshots, which it reads when recovering from them.
	cnpgPrefix = "cnpg.io/"
	// cnpgClusterLabel marks a cluster's own backups. Copies don't get it, so they aren't counted as backups of a cluster with the same name.
	cnpgClusterLabel = "cnpg.io/cluster"
	cnpgRoleLabel    = "cnpg.io/pvcRole"

	roleData = "PG_DATA"
	roleWAL  = "PG_WAL"
)

// volume is a kind of volume a cluster has.
type volume struct {
	// role is CloudNativePG's name for it, on the VolumeSnapshots of its backups.
	role string
	// field configures it in the cluster's spec.
	field string
	// purpose names the objects kupra creates for it.
	purpose string
}

// volumes are the volumes of a cluster that kupra captures.
var volumes = []volume{
	{roleData, "storage", "data"},
	{roleWAL, "walStorage", "wal"},
}

// in reports whether a cluster has a volume. Every cluster has a data volume.
func (v volume) in(c *unstructured.Unstructured) bool {
	_, ok, _ := unstructured.NestedMap(c.Object, "spec", v.field)
	return ok || v.role == roleData
}

// cnpgCluster supports CloudNativePG clusters, through CloudNativePG's volume snapshot backups and recovery.
type cnpgCluster struct{}

func (cnpgCluster) Check(resource map[string]any, fldPath *field.Path) field.ErrorList {
	const forbidden = "may not be set on a cluster that's state"
	var errs field.ErrorList
	if _, ok := resource["specExpr"]; ok {
		errs = append(errs, field.Forbidden(fldPath.Child("specExpr"), forbidden))
	}
	spec, _ := resource["spec"].(map[string]any)
	specPath := fldPath.Child("spec")
	for _, key := range []string{"backupExpr", "bootstrapExpr", "pluginsExpr", "replica", "replicaExpr", "tablespaces", "tablespacesExpr"} {
		if _, ok := spec[key]; ok {
			errs = append(errs, field.Forbidden(specPath.Child(key), forbidden))
		}
	}

	// kupra configures volume snapshot backups. A template only chooses their snapshot classes.
	backup, _ := spec["backup"].(map[string]any)
	for _, key := range slices.Sorted(maps.Keys(backup)) {
		if key != "volumeSnapshot" {
			errs = append(errs, field.Forbidden(specPath.Child("backup", key), forbidden))
		}
	}
	volumeSnapshot, _ := backup["volumeSnapshot"].(map[string]any)
	for _, key := range slices.Sorted(maps.Keys(volumeSnapshot)) {
		switch key {
		case "className", "classNameExpr", "walClassName", "walClassNameExpr":
		default:
			errs = append(errs, field.Forbidden(specPath.Child("backup", "volumeSnapshot", key), forbidden))
		}
	}

	// A restored cluster keeps its source's database, owner and password, so the template can't choose others.
	bootstrap, _ := spec["bootstrap"].(map[string]any)
	for _, key := range slices.Sorted(maps.Keys(bootstrap)) {
		if key != "initdb" {
			errs = append(errs, field.Forbidden(specPath.Child("bootstrap", key), forbidden))
		}
	}
	initdb, _ := bootstrap["initdb"].(map[string]any)
	initdbPath := specPath.Child("bootstrap", "initdb")
	for _, key := range []string{"database", "owner"} {
		if v, ok := initdb[key]; ok && v != "app" {
			errs = append(errs, field.NotSupported(initdbPath.Child(key), v, []string{"app"}))
		}
	}
	for _, key := range []string{"databaseExpr", "ownerExpr", "secret", "secretExpr"} {
		if _, ok := initdb[key]; ok {
			errs = append(errs, field.Forbidden(initdbPath.Child(key), forbidden))
		}
	}

	// WAL archiving would make a restored cluster archive into its source's archive.
	plugins, _ := spec["plugins"].([]any)
	for i, p := range plugins {
		plugin, _ := p.(map[string]any)
		if _, ok := plugin["isWALArchiverExpr"]; ok {
			errs = append(errs, field.Forbidden(specPath.Child("plugins").Index(i).Child("isWALArchiverExpr"), forbidden))
		}
		if plugin["isWALArchiver"] == true {
			errs = append(errs, field.Forbidden(specPath.Child("plugins").Index(i).Child("isWALArchiver"), forbidden))
		}
	}
	return errs
}

func (cnpgCluster) Restore(req RestoreRequest) Response {
	c := req.Resource.DeepCopy()
	// Backups delete their VolumeSnapshots when they're deleted, once their data has been moved.
	if err := unstructured.SetNestedField(c.Object, string(cnpgv1.SnapshotOwnerReferenceBackup), "spec", "backup", "volumeSnapshot", "snapshotOwnerReference"); err != nil {
		return Response{Failed: err.Error()}
	}
	if req.Live != nil {
		// CloudNativePG only reads the bootstrap when it creates the cluster.
		if bootstrap, ok, _ := unstructured.NestedMap(req.Live.Object, "spec", "bootstrap"); ok {
			_ = unstructured.SetNestedMap(c.Object, bootstrap, "spec", "bootstrap")
		}
		for _, v := range volumes {
			keepSize(c, req.Live, "spec", v.field, "size")
		}
		return Response{Resource: c, Done: true}
	}
	if req.From == nil {
		return Response{Resource: c, Done: true}
	}

	var captured []Ref
	for _, d := range req.From.Data {
		if !isSnapshot(d) {
			return Response{Failed: fmt.Sprintf("capture must be VolumeSnapshots, not %v", req.From.Data)}
		}
		captured = append(captured, Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.From.Namespace, Name: d.Name})
	}
	var observe []Ref
	bound := map[string]*boundSnapshot{}
	for _, ref := range captured {
		b, o, failed := readBound(ref, req.Observed)
		if failed != "" {
			return Response{Failed: failed}
		}
		observe = append(observe, o...)
		if b == nil {
			continue
		}
		role := b.snapshot.Labels[cnpgRoleLabel]
		if _, ok := bound[role]; ok || !slices.ContainsFunc(volumes, func(v volume) bool { return v.role == role }) {
			return Response{Failed: fmt.Sprintf("VolumeSnapshot %s has role %q", ref.Name, role)}
		}
		bound[role] = b
	}
	if len(bound) < len(captured) {
		return Response{Observe: observe}
	}

	sources := map[string]any{}
	var copies []*unstructured.Unstructured
	for _, v := range volumes {
		b := bound[v.role]
		switch {
		case b == nil && v.in(c):
			return Response{Failed: fmt.Sprintf("capture has no %s VolumeSnapshot", v.role)}
		case b == nil:
			continue
		case !v.in(c):
			return Response{Failed: fmt.Sprintf("capture has a %s VolumeSnapshot, but spec.%s is not set", v.role, v.field)}
		}
		name := req.Names.Name("restore-" + v.purpose)
		snapshot := metav1.ObjectMeta{Name: name, Namespace: c.GetNamespace(), Labels: cnpgMetadata(b.snapshot.Labels), Annotations: cnpgMetadata(b.snapshot.Annotations)}
		maps.Copy(snapshot.Labels, req.Labels)
		copies = append(copies, bind(b.content, req.Names.ClusterName("restore-"+v.purpose), snapshot, snapshotv1.VolumeSnapshotContentRetain, req.Labels)...)
		sources[v.field] = map[string]any{"apiGroup": volumeSnapshotGVK.Group, "kind": volumeSnapshotGVK.Kind, "name": name}
		// A volume can't be smaller than the snapshot it's restored from.
		if err := atLeast(c, b.snapshot.Status.RestoreSize, "spec", v.field, "size"); err != nil {
			return Response{Failed: err.Error()}
		}
	}
	bootstrap := map[string]any{"recovery": map[string]any{"volumeSnapshots": sources}}
	if err := unstructured.SetNestedMap(c.Object, bootstrap, "spec", "bootstrap"); err != nil {
		return Response{Failed: err.Error()}
	}
	return Response{Resource: c, Apply: copies, Done: true}
}

// Ready is true once CloudNativePG reports the cluster ready.
func (cnpgCluster) Ready(live *unstructured.Unstructured) bool {
	if live == nil {
		return false
	}
	conditions, _, _ := unstructured.NestedSlice(live.Object, "status", "conditions")
	for _, c := range conditions {
		c, _ := c.(map[string]any)
		if c["type"] == string(cnpgv1.ConditionClusterReady) {
			return c["status"] == string(metav1.ConditionTrue)
		}
	}
	return false
}

// Restored is true once the cluster is ready, since its recovery has finished then.
func (c cnpgCluster) Restored(live *unstructured.Unstructured) bool {
	return c.Ready(live)
}

// Capture asks CloudNativePG for a volume snapshot backup, then moves its VolumeSnapshots next to the PreviewSnapshot, with what CloudNativePG needs to recover from them.
func (cnpgCluster) Capture(req CaptureRequest) Response {
	backup := Ref{GroupVersionKind: backupGVK, Namespace: req.Resource.GetNamespace(), Name: req.Names.Name("capture")}
	targets := map[string]Ref{}
	observe := []Ref{backup}
	for _, v := range volumes {
		if v.in(req.Resource) {
			targets[v.role] = Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: req.Namespace, Name: req.Names.Name(v.purpose)}
			observe = append(observe, targets[v.role])
		}
	}
	for _, ref := range observe {
		if _, ok := req.Observed[ref]; !ok {
			return Response{Observe: observe}
		}
	}

	backupObj := req.Observed[backup]
	moved, ready, waiting, failed := allReady(slices.Collect(maps.Values(targets)), req.Observed)
	switch {
	case failed != "":
		return Response{Failed: failed}
	case moved && !ready:
		return Response{Observe: observe, Waiting: waiting}
	case moved && backupObj != nil:
		return Response{Delete: []Ref{backup}, Observe: observe}
	case moved:
		var data []corev1.TypedLocalObjectReference
		for _, v := range volumes {
			if t, ok := targets[v.role]; ok {
				data = append(data, snapshotRef(t.Name))
			}
		}
		return Response{Done: true, Capture: &Capture{Data: data}}
	}

	if backupObj == nil {
		b := &cnpgv1.Backup{
			ObjectMeta: metav1.ObjectMeta{Name: backup.Name, Namespace: backup.Namespace, Labels: req.Labels},
			Spec: cnpgv1.BackupSpec{
				Cluster: cnpgv1.LocalObjectReference{Name: req.Resource.GetName()},
				Method:  cnpgv1.BackupMethodVolumeSnapshot,
				Online:  ptr.To(req.Online),
			},
		}
		if req.Online {
			b.Spec.OnlineConfiguration = &cnpgv1.OnlineConfiguration{ImmediateCheckpoint: ptr.To(true)}
		}
		return Response{Apply: []*unstructured.Unstructured{toUnstructured(b, backupGVK)}, Observe: observe}
	}
	var b cnpgv1.Backup
	if err := fromUnstructured(backupObj, &b); err != nil {
		return Response{Failed: err.Error()}
	}
	switch b.Status.Phase {
	case cnpgv1.BackupPhaseCompleted:
	case cnpgv1.BackupPhaseFailed, cnpgv1.BackupPhaseWalArchivingFailing, cnpgv1.BackupPhaseDefinitionInvalid:
		return Response{Failed: fmt.Sprintf("Backup %s failed: %s", b.Name, b.Status.Error)}
	default:
		return Response{Observe: observe}
	}
	elements := b.Status.BackupSnapshotStatus.Elements
	if len(elements) != len(targets) {
		return Response{Failed: fmt.Sprintf("Backup %s has %d VolumeSnapshots, not %d", b.Name, len(elements), len(targets))}
	}

	var apply []*unstructured.Unstructured
	for _, e := range elements {
		i := slices.IndexFunc(volumes, func(v volume) bool { return v.role == e.Type })
		target, ok := targets[e.Type]
		if i < 0 || !ok {
			return Response{Failed: fmt.Sprintf("Backup %s has a %s VolumeSnapshot", b.Name, e.Type)}
		}
		source := Ref{GroupVersionKind: volumeSnapshotGVK, Namespace: backup.Namespace, Name: e.Name}
		observe = append(observe, source)
		sourceObj, ok := req.Observed[source]
		if !ok {
			continue
		}
		if sourceObj == nil {
			return Response{Failed: fmt.Sprintf("VolumeSnapshot %s not found", e.Name)}
		}
		var vs snapshotv1.VolumeSnapshot
		if err := fromUnstructured(sourceObj, &vs); err != nil {
			return Response{Failed: err.Error()}
		}
		meta := metav1.ObjectMeta{Name: target.Name, Namespace: target.Namespace, Labels: cnpgMetadata(vs.Labels), Annotations: cnpgMetadata(vs.Annotations)}
		maps.Copy(meta.Labels, req.Labels)
		a, o, failed := move(&vs, meta, req.Names.ClusterName(volumes[i].purpose), req.Labels, req.Observed)
		if failed != "" {
			return Response{Failed: failed}
		}
		apply = append(apply, a...)
		observe = append(observe, o...)
	}
	return Response{Apply: apply, Observe: observe}
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

// cnpgMetadata returns the CloudNativePG labels or annotations in m, except the cluster label.
func cnpgMetadata(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if strings.HasPrefix(k, cnpgPrefix) && k != cnpgClusterLabel {
			out[k] = v
		}
	}
	return out
}
