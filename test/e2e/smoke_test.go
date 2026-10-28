//go:build e2e

package e2e

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

func TestClusterIsReady(t *testing.T) {
	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("loading kubeconfig: %v", err)
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kuprav1alpha1.AddToScheme(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	ctx := context.Background()

	for _, gvk := range []schema.GroupVersionKind{
		{Group: "snapshot.storage.k8s.io", Version: "v1", Kind: "VolumeSnapshotClassList"},
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
	} {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		if err := c.List(ctx, list); err != nil {
			t.Errorf("listing %s: %v", gvk.Kind, err)
		}
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "smoke-"}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("creating namespace: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Delete(context.Background(), ns); err != nil {
			t.Errorf("deleting namespace: %v", err)
		}
	})

	preview := &kuprav1alpha1.Preview{ObjectMeta: metav1.ObjectMeta{Name: "smoke", Namespace: ns.Name}}
	if err := c.Create(ctx, preview); err != nil {
		t.Errorf("creating a Preview: %v", err)
	}
}
