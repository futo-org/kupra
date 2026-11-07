//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// TestDashboard signs in to the installed dashboard through Dex, with Playwright, as people bound to kupra's roles. e2e:up sets up Dex's users and the dashboard; web/e2e has the browser's side.
func TestDashboard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-dashboard-"}}
	if err := k8s.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := k8s.Delete(context.Background(), ns); err != nil {
			t.Errorf("deleting namespace: %v", err)
		}
	})

	for user, role := range map[string]string{"creator@example.com": "kupra-creator", "viewer@example.com": "kupra-viewer"} {
		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, Name: role},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
			Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: "oidc:" + user}},
		}
		if err := k8s.Create(ctx, binding); err != nil {
			t.Fatal(err)
		}
	}

	app := &kuprav1alpha1.PreviewApp{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns.Name}}
	if err := yaml.UnmarshalStrict([]byte(appSpec), &app.Spec); err != nil {
		t.Fatal(err)
	}
	if err := k8s.Create(ctx, app); err != nil {
		t.Fatal(err)
	}
	shared := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: ns.Name},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
	}
	if err := k8s.Create(ctx, shared); err != nil {
		t.Fatal(err)
	}
	eventually(t, 3*time.Minute, func() error {
		if err := k8s.Get(ctx, client.ObjectKeyFromObject(shared), shared); err != nil {
			return err
		}
		if shared.Status.Phase != kuprav1alpha1.PreviewRunning {
			return fmt.Errorf("Preview %s is %s", shared.Name, shared.Status.Phase)
		}
		return nil
	})

	cmd := exec.CommandContext(ctx, "pnpm", "exec", "playwright", "test")
	cmd.Dir = "../../web"
	cmd.Env = append(os.Environ(), "KUPRA_E2E_NAMESPACE="+ns.Name)
	out, err := cmd.CombinedOutput()
	t.Logf("playwright:\n%s", out)
	if err != nil {
		t.Fatalf("playwright: %v", err)
	}

	eventually(t, time.Minute, func() error {
		err := k8s.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: "pr-1"}, &kuprav1alpha1.Preview{})
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("getting the Preview deleted from the dashboard: got %v, want not found", err)
		}
		return nil
	})
}
