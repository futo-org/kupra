//go:build integration

package controller_test

import (
	"context"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

func TestCRDsInstall(t *testing.T) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "charts", "kupra", "crds")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Errorf("stopping envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kuprav1alpha1.AddToScheme(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "smoke"}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("creating namespace: %v", err)
	}

	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "preview", Namespace: ns.Name},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: kuprav1alpha1.LocalObjectReference{Name: "app"}},
	}
	for _, obj := range []client.Object{
		&kuprav1alpha1.PreviewApp{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns.Name},
			Spec:       kuprav1alpha1.PreviewAppSpec{Template: kuprav1alpha1.Template{Resources: []runtime.RawExtension{}}},
		},
		preview,
		&kuprav1alpha1.PreviewSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: ns.Name}},
		&kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	} {
		if err := c.Create(ctx, obj); err != nil {
			t.Errorf("creating %T: %v", obj, err)
		}
	}

	preview.Spec.AppRef.Name = "other"
	if err := c.Update(ctx, preview); err == nil {
		t.Errorf("changing a Preview's appRef succeeded, want it rejected")
	}
}
