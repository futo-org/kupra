//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"
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

// apiServerPolicy lets CloudNativePG's pods reach the API server, at the addresses and port of the kubernetes Service's endpoints.
func apiServerPolicy(t *testing.T) kuprav1alpha1.NetworkPolicyTemplate {
	t.Helper()
	var endpoints discoveryv1.EndpointSliceList
	if err := k8s.List(context.Background(), &endpoints, client.InNamespace("default"), client.MatchingLabels{discoveryv1.LabelServiceName: "kubernetes"}); err != nil {
		t.Fatal(err)
	}
	var rule networkingv1.NetworkPolicyEgressRule
	for _, s := range endpoints.Items {
		for _, e := range s.Endpoints {
			for _, address := range e.Addresses {
				cidr := address + "/32"
				if s.AddressType == discoveryv1.AddressTypeIPv6 {
					cidr = address + "/128"
				}
				rule.To = append(rule.To, networkingv1.NetworkPolicyPeer{IPBlock: &networkingv1.IPBlock{CIDR: cidr}})
			}
		}
		for _, p := range s.Ports {
			port := intstr.FromInt32(ptr.Deref(p.Port, 443))
			rule.Ports = append(rule.Ports, networkingv1.NetworkPolicyPort{Protocol: p.Protocol, Port: &port})
		}
	}
	if len(rule.To) == 0 {
		t.Fatal("the kubernetes Service has no endpoints")
	}
	return kuprav1alpha1.NetworkPolicyTemplate{
		Metadata: kuprav1alpha1.TemplateMetadata{Name: "allow-cnpg-api-server"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "cnpg.io/cluster", Operator: metav1.LabelSelectorOpExists}}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      []networkingv1.NetworkPolicyEgressRule{rule},
		},
	}
}

// TestClusterState captures a CloudNativePG cluster, deletes its preview, and restores the capture into another, which connects with its own generated credentials.
func TestClusterState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// The default policy's egress rules keep CloudNativePG from the API server. This follows docs/install.md to allow it.
	policy := &kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: "e2e-cnpg-" + rand.String(5)}}
	var def kuprav1alpha1.PreviewPolicy
	if err := k8s.Get(ctx, client.ObjectKey{Name: "default"}, &def); err != nil {
		t.Fatal(err)
	}
	def.Spec.DeepCopyInto(&policy.Spec)
	policy.Spec.NetworkPolicies = append(policy.Spec.NetworkPolicies, apiServerPolicy(t))
	create(t, policy)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-cnpg-", Labels: map[string]string{"kupra.futo.tech/policy": policy.Name}}}
	if err := k8s.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// A failed test's namespace is kept for CI's dump of the cluster.
		if t.Failed() {
			return
		}
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
