//go:build integration

package controller_test

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/futo-org/kupra/internal/controller"
)

// TestNamespacesWithAppsAreLabelled checks that a namespace is labelled while it contains PreviewApps, and that removing the label, whoever set it, leaves its other labels alone.
func TestNamespacesWithAppsAreLabelled(t *testing.T) {
	ctx := context.Background()
	createNamespace(t, "has-apps", map[string]string{"team": "a", controller.LabelHasApps: "true"})
	labelled := func(want bool) {
		t.Helper()
		eventually(t, func() error {
			var ns corev1.Namespace
			if err := k8s.Get(ctx, client.ObjectKey{Name: "has-apps"}, &ns); err != nil {
				return err
			}
			if _, got := ns.Labels[controller.LabelHasApps]; got != want {
				return fmt.Errorf("namespace has %s %t, want %t", controller.LabelHasApps, got, want)
			}
			if ns.Labels["team"] != "a" {
				return fmt.Errorf("namespace's team label is %q, want it kept as a", ns.Labels["team"])
			}
			return nil
		})
	}
	labelled(false)

	first := createApp(t, "has-apps", "first", "template: {resources: []}")
	second := createApp(t, "has-apps", "second", "template: {resources: []}")
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
