//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// TestNamespacesWithAppsAreLabelled checks that a namespace is labelled while it contains PreviewApps, and that removing the label, whoever set it, leaves its other labels alone.
func TestNamespacesWithAppsAreLabelled(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "with-apps", map[string]string{"team": "a", kuprav1alpha1.LabelAppNamespace: ""})
	labelled := func(want bool) {
		t.Helper()
		eventually(t, func() error {
			var ns corev1.Namespace
			if err := k8s.Get(ctx, client.ObjectKey{Name: "with-apps"}, &ns); err != nil {
				return err
			}
			value, got := ns.Labels[kuprav1alpha1.LabelAppNamespace]
			switch {
			case got != want:
				return fmt.Errorf("namespace labelled %s is %t, want %t", kuprav1alpha1.LabelAppNamespace, got, want)
			case value != "":
				return fmt.Errorf("namespace's %s label is %q, want it empty", kuprav1alpha1.LabelAppNamespace, value)
			}
			if ns.Labels["team"] != "a" {
				return fmt.Errorf("namespace's team label is %q, want it kept as a", ns.Labels["team"])
			}
			return nil
		})
	}
	labelled(false)

	first := createApp(t, "with-apps", "first", "template: {resources: []}")
	second := createApp(t, "with-apps", "second", "template: {resources: []}")
	labelled(true)

	// A label with a value is put back to the empty one.
	setLabel(t, "with-apps", "stale")
	labelled(true)

	// One app is enough to keep the label: removed while there is one, it comes back.
	if err := k8s.Delete(ctx, first); err != nil {
		t.Fatal(err)
	}
	setLabel(t, "with-apps", nil)
	labelled(true)
	if err := k8s.Delete(ctx, second); err != nil {
		t.Fatal(err)
	}
	labelled(false)
}

// setLabel sets a namespace's app-namespace label to value, or removes it when value is nil, as another field manager.
func setLabel(t *testing.T, name string, value any) {
	t.Helper()
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"labels": map[string]any{kuprav1alpha1.LabelAppNamespace: value}}})
	if err != nil {
		t.Fatal(err)
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := k8s.Patch(context.Background(), ns, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("test")); err != nil {
		t.Fatal(err)
	}
}
