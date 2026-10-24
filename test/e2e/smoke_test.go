//go:build e2e

package e2e

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestClusterIsReady(t *testing.T) {
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "snapshot.storage.k8s.io", Version: "v1", Kind: "VolumeSnapshotClassList"},
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
	} {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		if err := k8s.List(context.Background(), list); err != nil {
			t.Errorf("listing %s: %v", gvk.Kind, err)
		}
	}
}
