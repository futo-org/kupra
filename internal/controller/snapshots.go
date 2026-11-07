package controller

import (
	"context"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// deleteSnapshots deletes the VolumeSnapshots and VolumeSnapshotContents that opts select, and reports whether there were any. Contents that delete their storage snapshot go with the VolumeSnapshot bound to them.
func deleteSnapshots(ctx context.Context, reader client.Reader, c client.Writer, opts ...client.ListOption) (bool, error) {
	var snapshots snapshotv1.VolumeSnapshotList
	if err := reader.List(ctx, &snapshots, opts...); err != nil && !apimeta.IsNoMatchError(err) {
		return false, err
	}
	for i := range snapshots.Items {
		if err := client.IgnoreNotFound(c.Delete(ctx, &snapshots.Items[i])); err != nil {
			return false, err
		}
	}
	// Contents are cluster-scoped.
	var contentOpts []client.ListOption
	for _, opt := range opts {
		if _, ok := opt.(client.InNamespace); !ok {
			contentOpts = append(contentOpts, opt)
		}
	}
	var contents snapshotv1.VolumeSnapshotContentList
	if err := reader.List(ctx, &contents, contentOpts...); err != nil && !apimeta.IsNoMatchError(err) {
		return false, err
	}
	found := len(snapshots.Items) > 0
	for i := range contents.Items {
		if contents.Items[i].Spec.DeletionPolicy == snapshotv1.VolumeSnapshotContentDelete {
			continue
		}
		found = true
		if err := client.IgnoreNotFound(c.Delete(ctx, &contents.Items[i])); err != nil {
			return false, err
		}
	}
	return found, nil
}
