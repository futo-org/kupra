package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/futo-org/kupra/internal/state"
)

// A kind that waits, applying the same object each time, is called again only while its writes change what it reads, and its waiting reason isn't lost.
func TestRunKindStopsWhenNothingChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	// The API server doesn't change an object, or its resourceVersion, when it's applied again unchanged. The fake client does, so only its first apply is passed on.
	applied := false
	r := &PreviewReconciler{Client: interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
		Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
			if applied {
				return nil
			}
			applied = true
			return c.Apply(ctx, obj, opts...)
		},
	})}
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm.SetNamespace("ns")
	cm.SetName("copy")
	ref := state.RefOf(cm)
	calls := 0
	resp, err := r.runKind(context.Background(), func(state.Observed) state.Response {
		calls++
		return state.Response{Apply: []*unstructured.Unstructured{cm.DeepCopy()}, Observe: []state.Ref{ref}, Waiting: "copy isn't bound"}
	}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls >= maxKindRounds {
		t.Errorf("kind was called %d times, want it to stop once applying changed nothing", calls)
	}
	if resp.Waiting != "copy isn't bound" {
		t.Errorf("waiting = %q, want the kind's", resp.Waiting)
	}
}
