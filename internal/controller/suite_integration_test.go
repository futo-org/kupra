//go:build integration

package controller_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
	"github.com/futo-org/kupra/internal/policy"
)

const kupraNamespace = "kupra-system"

var (
	k8s        client.Client
	restConfig *rest.Config
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctrl.SetLogger(logr.Discard())
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "charts", "kupra", "crds")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting envtest: %v\n", err)
		return 1
	}
	restConfig = cfg
	defer func() {
		if err := env.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "stopping envtest: %v\n", err)
		}
	}()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kuprav1alpha1.AddToScheme(scheme))

	k8s, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating client: %v\n", err)
		return 1
	}
	if err := k8s.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: kupraNamespace}}); err != nil {
		fmt.Fprintf(os.Stderr, "creating kupra's namespace: %v\n", err)
		return 1
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating manager: %v\n", err)
		return 1
	}
	if err := controller.Setup(context.Background(), mgr, controller.Options{Namespace: kupraNamespace}); err != nil {
		fmt.Fprintf(os.Stderr, "setting up controllers: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "running manager: %v\n", err)
		}
	}()

	return m.Run()
}

// apply server-side applies the YAML documents in a file.
func apply(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	applyYAML(t, data)
}

func applyYAML(t *testing.T, data []byte) {
	t.Helper()
	for _, doc := range bytes.Split(data, []byte("\n---\n")) {
		var u unstructured.Unstructured
		if err := yaml.Unmarshal(doc, &u.Object); err != nil {
			t.Fatal(err)
		}
		if len(u.Object) == 0 {
			continue
		}
		if err := k8s.Apply(context.Background(), client.ApplyConfigurationFromUnstructured(&u), client.FieldOwner("test"), client.ForceOwnership); err != nil {
			t.Fatalf("applying %s %s: %v", u.GetKind(), u.GetName(), err)
		}
	}
}

// eventually retries check until it succeeds, and fails the test with its last error after a while.
func eventually(t *testing.T, check func() error) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		err := check()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// setUpPolicies applies kupra's template roles and the chart's default PreviewPolicy, and creates any other PreviewPolicies given, unless they exist already. envtest has no controller-manager to aggregate ClusterRoles, so the default policy points at the rules directly.
func setUpPolicies(t *testing.T, policies ...*kuprav1alpha1.PreviewPolicy) {
	t.Helper()
	chart := filepath.Join("..", "..", "charts", "kupra")
	apply(t, filepath.Join(chart, "templates", "template-clusterroles.yaml"))
	data, err := os.ReadFile(filepath.Join(chart, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		DefaultPolicy kuprav1alpha1.PreviewPolicySpec `json:"defaultPolicy"`
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	def := &kuprav1alpha1.PreviewPolicy{ObjectMeta: metav1.ObjectMeta{Name: policy.DefaultName}, Spec: values.DefaultPolicy}
	def.Spec.Apply.ClusterRoleRef.Name = "kupra-preview-template-base"
	if err := k8s.Create(context.Background(), def); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	for _, p := range policies {
		if err := k8s.Create(context.Background(), p); err != nil && !apierrors.IsAlreadyExists(err) {
			t.Fatal(err)
		}
	}
}
