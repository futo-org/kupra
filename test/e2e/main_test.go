//go:build e2e

package e2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/controller"
)

const (
	kupraNamespace = "kupra-system"
	operatorName   = "kupra-operator"
)

// k8s is a cluster admin.
var k8s client.Client

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(run(m))
}

func run(m *testing.M) int {
	if testing.Verbose() {
		ctrl.SetLogger(zap.New(zap.WriteTo(os.Stderr)))
	} else {
		ctrl.SetLogger(logr.Discard())
	}

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading kubeconfig: %v\n", err)
		return 1
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kuprav1alpha1.AddToScheme(scheme))
	utilruntime.Must(snapshotv1.AddToScheme(scheme))
	k8s, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating client: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The operator runs in this process, but with its own service account's token, so its RBAC is what's tested.
	operatorCfg, err := serviceAccountConfig(ctx, cfg, kupraNamespace, operatorName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "getting a token for the operator: %v\n", err)
		return 1
	}
	mgr, err := ctrl.NewManager(operatorCfg, ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		LeaderElection:          true,
		LeaderElectionID:        "operator.kupra.futo.tech",
		LeaderElectionNamespace: kupraNamespace,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating manager: %v\n", err)
		return 1
	}
	if err := controller.Setup(ctx, mgr, controller.Options{Namespace: kupraNamespace}); err != nil {
		fmt.Fprintf(os.Stderr, "setting up controllers: %v\n", err)
		return 1
	}
	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "running manager: %v\n", err)
		}
	}()

	return m.Run()
}

func serviceAccountConfig(ctx context.Context, cfg *rest.Config, namespace, name string) (*rest.Config, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	token, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, name, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: new(int64(3600))},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	saCfg := rest.AnonymousClientConfig(cfg)
	saCfg.BearerToken = token.Status.Token
	return saCfg, nil
}

// eventually retries check until it succeeds, and fails the test with its last error after timeout.
func eventually(t *testing.T, timeout time.Duration, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		err := check()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
	}
}
