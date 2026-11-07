//go:build integration

package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// startAPIServer starts an API server that trusts p's ID tokens for the audience kupra, as users oidc:<sub>.
func startAPIServer(t *testing.T, p *provider) (*rest.Config, client.Client) {
	t.Helper()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.Certificate().Raw})
	config := fmt.Sprintf(`apiVersion: apiserver.config.k8s.io/v1
kind: AuthenticationConfiguration
jwt:
  - issuer:
      url: %s
      audiences: [kupra]
      certificateAuthority: |
        %s
    claimMappings:
      username:
        claim: sub
        prefix: "oidc:"
`, p.URL, strings.ReplaceAll(strings.TrimSpace(string(ca)), "\n", "\n        "))
	path := filepath.Join(t.TempDir(), "authentication.yaml")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "..", "charts", "kupra", "crds")}}
	env.ControlPlane.GetAPIServer().Configure().Set("authentication-config", path)
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Stop() })
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kuprav1alpha1.AddToScheme(scheme)
	k8s, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	return cfg, k8s
}

// stream reads a watch stream's events as they come.
func stream(t *testing.T, f *fixture, query string) (<-chan event, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.dashboard.URL+"/api/watch?"+query, nil)
	resp, err := f.browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watching is %s, want 200", resp.Status)
	}
	events := make(chan event, 100)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(events)
		lines := bufio.NewScanner(resp.Body)
		lines.Buffer(nil, 1<<20)
		for lines.Scan() {
			data, ok := strings.CutPrefix(lines.Text(), "data: ")
			if !ok {
				continue
			}
			// Objects come back as maps, which the test reads by name.
			var e struct {
				event
				Object map[string]any `json:"object"`
			}
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				t.Error(err)
				return
			}
			e.event.Object = nil
			if e.Object != nil {
				e.Collection += " " + fmt.Sprint(e.Object["metadata"].(map[string]any)["name"])
			}
			events <- e.event
		}
	}()
	return events, cancel
}

// next waits for an event of the collection and type.
func next(t *testing.T, events <-chan event, collection, typ string) {
	t.Helper()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case e, ok := <-events:
			if !ok {
				t.Fatalf("stream ended, want %s %s", typ, collection)
			}
			if e.Collection == collection && e.Type == typ {
				return
			}
		case <-timeout:
			t.Fatalf("no %s %s after 30s", typ, collection)
		}
	}
}

// TestWatchAsUser checks the stream end to end: the user signs in with the provider, the API server trusts their ID token, and they see changes in the namespaces RBAC lets them read, and an error for others.
func TestWatchAsUser(t *testing.T) {
	ctx := context.Background()
	p := newProvider(t)
	cfg, k8s := startAPIServer(t, p)

	for _, ns := range []string{"readable", "forbidden"} {
		obj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns, Labels: map[string]string{kuprav1alpha1.LabelAppNamespace: ""}}}
		if err := k8s.Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Namespace: "readable", Name: "previews"},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{kuprav1alpha1.GroupVersion.Group}, Resources: []string{"previews"}, Verbs: []string{"get", "list", "watch"}}},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: "readable", Name: "alice"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "previews"},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: "oidc:alice"}},
	}
	for _, obj := range []client.Object{role, binding} {
		if err := k8s.Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}

	f := newFixtureWith(t, p, cfg)
	f.signIn(t, "/")
	// The API server fetches the provider's keys in the background, so the first tokens may not be trusted yet.
	deadline := time.Now().Add(30 * time.Second)
	for {
		code, body := f.do(t, http.MethodGet, "/api/session", nil)
		if code == http.StatusOK && strings.Contains(body, `"oidc:alice"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session is %d %s, want oidc:alice", code, body)
		}
		time.Sleep(time.Second)
	}

	events, stop := stream(t, f, "c=previews/readable&c=previews/forbidden&c=app-namespaces")
	defer stop()
	next(t, events, "previews/forbidden", "ERROR")
	next(t, events, "app-namespaces readable", "ADDED")
	next(t, events, "app-namespaces", "SYNCED")
	next(t, events, "previews/readable", "SYNCED")

	preview := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Namespace: "readable", Name: "pr-1"},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
	}
	if err := k8s.Create(ctx, preview); err != nil {
		t.Fatal(err)
	}
	next(t, events, "previews/readable pr-1", "ADDED")
	if err := k8s.Delete(ctx, preview); err != nil {
		t.Fatal(err)
	}
	next(t, events, "previews/readable pr-1", "DELETED")
}
