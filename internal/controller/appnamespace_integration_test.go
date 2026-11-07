//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
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
			if _, got := ns.Labels[kuprav1alpha1.LabelAppNamespace]; got != want {
				return fmt.Errorf("namespace labelled %s is %t, want %t", kuprav1alpha1.LabelAppNamespace, got, want)
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

	if err := k8s.Delete(ctx, first); err != nil {
		t.Fatal(err)
	}
	labelled(true)
	if err := k8s.Delete(ctx, second); err != nil {
		t.Fatal(err)
	}
	labelled(false)
}
