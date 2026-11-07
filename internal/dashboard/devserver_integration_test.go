//go:build integration

package dashboard

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// TestDevServer serves the built dashboard at http://localhost:8080 against a local API server with sample data, signed in as oidc:alice, until it's interrupted. It's for trying the dashboard by hand or with Playwright: `mise run dashboard:dev`. The identity provider's certificate is self-signed, so the browser has to accept it once.
func TestDevServer(t *testing.T) {
	if os.Getenv("KUPRA_DASHBOARD_DEV") == "" {
		t.Skip("set KUPRA_DASHBOARD_DEV to run the dev server")
	}
	ctx := context.Background()
	p := newProvider(t)
	cfg, k8s := startAPIServer(t, p)
	seed(t, ctx, k8s)

	handler, err := New(ctx, Options{
		PublicURL:    "http://localhost:8080",
		IssuerURL:    p.URL,
		ClientID:     "kupra",
		ClientSecret: "secret",
		Scopes:       []string{"openid", "offline_access"},
		Config:       cfg,
		StaticDir:    filepath.Join("..", "..", "cmd", "dashboard", "kodata"),
		HTTPClient:   p.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("serving the dashboard at http://localhost:8080, signed in as oidc:alice through %s; stop with Ctrl-C", p.URL)
	server := &http.Server{Addr: "localhost:8080", Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	// Stopping on a signal, rather than being killed, lets the test stop the API server.
	stop, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-stop.Done()
		_ = server.Close()
	}()
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		t.Fatal(err)
	}
}

// seed creates an app with previews in several states, a snapshot, and access for oidc:alice. Nothing reconciles them, so their statuses are set here.
func seed(t *testing.T, ctx context.Context, k8s client.Client) {
	t.Helper()
	create := func(obj client.Object) {
		t.Helper()
		status := statusOf(obj)
		if err := k8s.Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
		if status != nil {
			restoreStatus(obj, status)
			if err := k8s.Status().Update(ctx, obj); err != nil {
				t.Fatal(err)
			}
		}
	}
	create(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "immich-previews", Labels: map[string]string{kuprav1alpha1.LabelAppNamespace: ""}}})
	create(&rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Namespace: "immich-previews", Name: "kupra"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{kuprav1alpha1.GroupVersion.Group}, Resources: []string{"*"}, Verbs: []string{"*"}},
		},
	})
	create(&rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: "immich-previews", Name: "alice"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "kupra"},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: "oidc:alice"}},
	})

	app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Namespace: "immich-previews", Name: "immich"}}
	if err := yaml.UnmarshalStrict([]byte(`
parameters:
  - name: tag
    title: Server image
    description: Tag of ghcr.io/immich-app/immich-server
    required: true
    uiHint: {type: imageTag, imageTag: {repository: ghcr.io/immich-app/immich-server}}
  - name: logLevel
    title: Log level
    enum: [debug, log, warn]
    default: log
  - name: workers
    title: Server workers
    type: integer
    default: "1"
links:
  - name: Web
    urlExpr: '"https://" + preview.name + ".immich.example.com"'
template:
  resources: []
`), &app.Spec); err != nil {
		t.Fatal(err)
	}
	app.Status.Conditions = []metav1.Condition{{Type: meta.ReadyCondition, Status: metav1.ConditionTrue, Reason: meta.SucceededReason, LastTransitionTime: metav1.Now()}}
	create(app)

	now := metav1.Now()
	preview := func(name string, phase kuprav1alpha1.PreviewPhase, ready metav1.ConditionStatus, reason, message string, annotations map[string]string) {
		p := &kuprav1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{Namespace: "immich-previews", Name: name, Annotations: annotations},
			Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "immich"}, Parameters: map[string]string{"tag": name}},
		}
		p.Status = kuprav1alpha1.PreviewStatus{
			Phase:     phase,
			Namespace: "immich-previews-" + name,
			Links:     []kuprav1alpha1.LinkStatus{{Name: "Web", URL: "https://" + name + ".immich.example.com"}},
			Inventory: []kuprav1alpha1.InventoryEntry{{TypedLocalObjectReference: corev1.TypedLocalObjectReference{APIGroup: ptr.To("apps"), Kind: "Deployment", Name: "immich-server"}}, {TypedLocalObjectReference: corev1.TypedLocalObjectReference{Kind: "PersistentVolumeClaim", Name: "library"}, State: true}},
			Conditions: []metav1.Condition{{
				Type: meta.ReadyCondition, Status: ready, Reason: reason, Message: message, LastTransitionTime: now,
			}},
		}
		create(p)
	}
	preview("pr-24311", kuprav1alpha1.PreviewRunning, metav1.ConditionTrue, meta.SucceededReason, "", map[string]string{
		"kupra.futo.tech/created-by": "oidc:alice",
		"kupra.futo.tech/source":     "https://github.com/immich-app/immich/pull/24311",
	})
	preview("ml-upgrade", kuprav1alpha1.PreviewFailed, metav1.ConditionFalse, "ProgressDeadlineExceeded", "Deployment/immich-machine-learning: 0 of 1 replicas available after 30m", map[string]string{
		"kupra.futo.tech/created-by": "oidc:alice",
	})
	preview("pr-24350", kuprav1alpha1.PreviewProvisioning, metav1.ConditionUnknown, "Progressing", "restoring PersistentVolumeClaim/library", map[string]string{
		"kupra.futo.tech/created-by": "oidc:bob",
	})
}

// statusOf takes an object's status out before it's created, since creating ignores it.
func statusOf(obj client.Object) map[string]any {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil
	}
	status, _ := u["status"].(map[string]any)
	if len(status) == 0 {
		return nil
	}
	return status
}

// restoreStatus puts a status taken by statusOf back on the created object.
func restoreStatus(obj client.Object, status map[string]any) {
	u, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	u["status"] = status
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(u, obj)
}
