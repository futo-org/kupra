//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
)

// stateAppSpec writes its value parameter to a claim the first time it starts, and is only ready while the claim holds the expected value. A preview restored from a snapshot is therefore only ready if the data really came across.
const stateAppSpec = `
parameters:
  - name: value
    title: Value written to an empty claim
  - name: expect
    title: Value the claim must hold
template:
  resources:
    - apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: data
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 16Mi
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: app
      spec:
        selector:
          matchLabels: {app: app}
        strategy: {type: Recreate}
        template:
          metadata:
            labels: {app: app}
          spec:
            containers:
              - name: app
                image: registry.k8s.io/e2e-test-images/busybox:1.36.1-1
                command: [sh, -c, 'test -f /data/value || echo "$VALUE" > /data/value; exec sleep 3600']
                env:
                  - name: VALUE
                    valueExpr: parameters.value
                  - name: EXPECT
                    valueExpr: parameters.expect
                readinessProbe:
                  exec:
                    command: [sh, -c, 'grep -qx "$EXPECT" /data/value']
                  periodSeconds: 2
                volumeMounts:
                  - name: data
                    mountPath: /data
            volumes:
              - name: data
                persistentVolumeClaim:
                  claimName: data
`

func TestState(t *testing.T) {
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-state-"}}
	if err := k8s.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := k8s.Delete(context.Background(), ns); err != nil {
			t.Errorf("deleting namespace: %v", err)
		}
	})

	app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns.Name}}
	if err := yaml.UnmarshalStrict([]byte(stateAppSpec), &app.Spec); err != nil {
		t.Fatal(err)
	}
	if err := k8s.Create(ctx, app); err != nil {
		t.Fatal(err)
	}

	running := func(name string) *kuprav1alpha1.Preview {
		t.Helper()
		var p kuprav1alpha1.Preview
		eventually(t, 5*time.Minute, func() error {
			if err := k8s.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: name}, &p); err != nil {
				return err
			}
			if p.Status.ObservedGeneration != p.Generation || p.Status.Phase != kuprav1alpha1.PreviewRunning {
				return fmt.Errorf("Preview %s is %s, Ready %+v", name, p.Status.Phase, apimeta.FindStatusCondition(p.Status.Conditions, meta.ReadyCondition))
			}
			return nil
		})
		return &p
	}

	source := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: ns.Name},
		Spec: kuprav1alpha1.PreviewSpec{
			AppRef:     meta.LocalObjectReference{Name: "app"},
			Parameters: map[string]string{"value": "first", "expect": "first"},
		},
	}
	if err := k8s.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	running("source")

	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: ns.Name, Labels: map[string]string{"kupra.futo.tech/channel": "base"}},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}
	eventually(t, 5*time.Minute, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(snap), snap); err != nil {
			return err
		}
		if !snap.Status.ReadyToUse {
			return fmt.Errorf("PreviewSnapshot is %s, Ready %+v", snap.Status.Phase, apimeta.FindStatusCondition(snap.Status.Conditions, meta.ReadyCondition))
		}
		return nil
	})
	// The source comes back after an Offline capture.
	running("source")

	// The restored preview would write "second" to an empty claim, so it's only ready if it got "first" from the snapshot.
	restored := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "restored", Namespace: ns.Name},
		Spec: kuprav1alpha1.PreviewSpec{
			AppRef:     meta.LocalObjectReference{Name: "app"},
			Parameters: map[string]string{"value": "second", "expect": "first"},
			RestoreFrom: &kuprav1alpha1.RestoreSource{
				SnapshotSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kupra.futo.tech/channel": "base"}},
			},
		},
	}
	if err := k8s.Create(ctx, restored); err != nil {
		t.Fatal(err)
	}
	p := running("restored")
	if p.Status.RestoredFrom == nil || p.Status.RestoredFrom.Name != "base" {
		t.Errorf("restoredFrom = %v, want base", p.Status.RestoredFrom)
	}

	// Deleting the source leaves the snapshot intact, and a reset restores from it again.
	if err := k8s.Delete(ctx, source); err != nil {
		t.Fatal(err)
	}
	if p.Annotations == nil {
		p.Annotations = map[string]string{}
	}
	p.Annotations[controller.AnnotationResetRequestedAt] = time.Now().Format(time.RFC3339)
	if err := k8s.Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	eventually(t, 5*time.Minute, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(p), p); err != nil {
			return err
		}
		if p.Status.LastHandledResetAt != p.Annotations[controller.AnnotationResetRequestedAt] {
			return fmt.Errorf("reset not handled yet: Preview is %s", p.Status.Phase)
		}
		return nil
	})
	running("restored")
}
