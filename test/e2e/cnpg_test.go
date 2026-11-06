//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// clusterAppSpec writes its value parameter to an empty database, and is ready while the database holds its expect parameter. The app uses the cluster's image for psql, which the node has already pulled.
const clusterAppSpec = `
parameters:
  - name: value
    title: Value written to an empty database
  - name: expect
    title: Value the database must hold
template:
  resources:
    - apiVersion: postgresql.cnpg.io/v1
      kind: Cluster
      metadata:
        name: database
      spec:
        instances: 1
        imageName: ghcr.io/cloudnative-pg/postgresql:18.6-system-trixie
        storage: {size: 256Mi}
        walStorage: {size: 128Mi}
    - apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: app
      spec:
        selector:
          matchLabels: {app: app}
        template:
          metadata:
            labels: {app: app}
          spec:
            containers:
              - name: app
                image: ghcr.io/cloudnative-pg/postgresql:18.6-system-trixie
                command:
                  - sh
                  - -c
                  - |
                    until psql "$URI" -c 'create table if not exists t (v text)'; do sleep 2; done
                    echo "insert into t select :'v' where not exists (select 1 from t)" | psql "$URI" -v v="$VALUE"
                    exec sleep infinity
                env:
                  - name: URI
                    valueFrom:
                      secretKeyRef: {name: database-app, key: uri}
                  - name: VALUE
                    valueExpr: parameters.value
                  - name: EXPECT
                    valueExpr: parameters.expect
                readinessProbe:
                  exec:
                    command: [sh, -c, 'test "$(psql "$URI" -tAc "select v from t")" = "$EXPECT"']
                  periodSeconds: 2
`

// TestClusterState captures a CloudNativePG cluster, deletes its preview, and restores the capture into another, which connects with its own generated credentials.
func TestClusterState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-cnpg-"}}
	if err := k8s.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := k8s.Delete(context.Background(), ns); err != nil {
			t.Errorf("deleting namespace: %v", err)
		}
	})

	app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns.Name}}
	if err := yaml.UnmarshalStrict([]byte(clusterAppSpec), &app.Spec); err != nil {
		t.Fatal(err)
	}
	if err := k8s.Create(ctx, app); err != nil {
		t.Fatal(err)
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
	waitForRunning(t, ns.Name, "source")

	snap := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: ns.Name},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "source"}}},
	}
	if err := k8s.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}
	waitForSnapshot(t, snap)
	if len(snap.Status.Captures) != 1 || len(snap.Status.Captures[0].Data) != 2 {
		t.Errorf("captures = %+v, want the cluster's data and WAL", snap.Status.Captures)
	}

	// The capture outlives its source.
	if err := k8s.Delete(ctx, source); err != nil {
		t.Fatal(err)
	}
	eventually(t, 5*time.Minute, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(source), &kuprav1alpha1.Preview{}); !apierrors.IsNotFound(err) {
			return fmt.Errorf("Preview source still exists (%v)", err)
		}
		return nil
	})

	// The restored preview is only ready if its database holds "first", from the snapshot.
	restored := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "restored", Namespace: ns.Name},
		Spec: kuprav1alpha1.PreviewSpec{
			AppRef:      meta.LocalObjectReference{Name: "app"},
			Parameters:  map[string]string{"value": "second", "expect": "first"},
			RestoreFrom: &kuprav1alpha1.RestoreSource{SnapshotRef: &meta.LocalObjectReference{Name: "base"}},
		},
	}
	if err := k8s.Create(ctx, restored); err != nil {
		t.Fatal(err)
	}
	waitForRunning(t, ns.Name, "restored")
}
