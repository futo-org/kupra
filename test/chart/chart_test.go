// Package chart checks kupra's Helm chart as rendered.
package chart

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// render renders the chart's templates, which don't include its CRDs, and decodes them strictly, so duplicate keys fail.
func render(t *testing.T, args ...string) []unstructured.Unstructured {
	t.Helper()
	out, err := exec.Command("helm", append([]string{"template", "kupra", "../../charts/kupra", "--namespace", "kupra-system"}, args...)...).Output()
	if err != nil {
		t.Fatalf("rendering the chart: %v", err)
	}
	var objs []unstructured.Unstructured
	for _, doc := range bytes.Split(out, []byte("\n---\n")) {
		var u unstructured.Unstructured
		if err := yaml.UnmarshalStrict(doc, &u.Object); err != nil {
			t.Fatal(err)
		}
		if len(u.Object) > 0 {
			objs = append(objs, u)
		}
	}
	return objs
}

// dashboard turns the dashboard on, so tests cover its objects too.
var dashboard = []string{
	"--set", "dashboard.enabled=true",
	"--set", "dashboard.publicURL=https://previews.example.com",
	"--set", "dashboard.oidc.issuerURL=https://id.example.com",
	"--set", "dashboard.oidc.clientID=kupra",
	"--set", "dashboard.existingSecret=kupra-dashboard",
}

// Every object the chart templates carries its labels, so a release's objects can be found together. Helm applies CRDs without templating them. ClusterRoles don't get commonLabels, which other ClusterRoles could aggregate them by.
func TestObjectsAreLabelled(t *testing.T) {
	for _, u := range render(t, append(dashboard, "--set", "commonLabels.example=yes")...) {
		labels := u.GetLabels()
		if labels["app.kubernetes.io/part-of"] != "kupra" {
			t.Errorf("%s %s isn't labelled app.kubernetes.io/part-of: kupra", u.GetKind(), u.GetName())
		}
		switch {
		case u.GetKind() == "ClusterRole" && labels["example"] != "":
			t.Errorf("ClusterRole %s has the commonLabels", u.GetName())
		case u.GetKind() != "ClusterRole" && labels["example"] != "yes":
			t.Errorf("%s %s doesn't have the commonLabels", u.GetKind(), u.GetName())
		}
	}
}

// commonLabels can't replace the labels Deployments and Services select their pods by.
func TestCommonLabelsKeepSelectors(t *testing.T) {
	objs := render(t, append(dashboard, "--set", "commonLabels.app\\.kubernetes\\.io/name=custom", "--set", "commonLabels.app\\.kubernetes\\.io/instance=custom")...)
	for _, u := range objs {
		var selector map[string]string
		switch u.GetKind() {
		case "Deployment":
			selector, _, _ = unstructured.NestedStringMap(u.Object, "spec", "selector", "matchLabels")
		case "Service":
			selector, _, _ = unstructured.NestedStringMap(u.Object, "spec", "selector")
		default:
			continue
		}
		var pods map[string]string
		for _, d := range objs {
			if d.GetKind() == "Deployment" && d.GetName() == strings.TrimSuffix(u.GetName(), "-metrics") {
				pods, _, _ = unstructured.NestedStringMap(d.Object, "spec", "template", "metadata", "labels")
			}
		}
		for k, v := range selector {
			if pods[k] != v {
				t.Errorf("%s %s selects %s=%s, but its pods have %q", u.GetKind(), u.GetName(), k, v, pods[k])
			}
		}
	}
}

// podLabels win over commonLabels on each component's pods.
func TestPodLabelsWin(t *testing.T) {
	for _, u := range render(t, append(dashboard, "--set", "commonLabels.team=common", "--set", "operator.podLabels.team=pods", "--set", "dashboard.podLabels.team=pods")...) {
		if u.GetKind() != "Deployment" {
			continue
		}
		pods, _, _ := unstructured.NestedStringMap(u.Object, "spec", "template", "metadata", "labels")
		if pods["team"] != "pods" {
			t.Errorf("%s's pods are labelled team %q, want podLabels' pods", u.GetName(), pods["team"])
		}
	}
}

// kupra can be a dependency of another chart, which gives it Helm's global values.
func TestRendersAsDependency(t *testing.T) {
	umbrella := t.TempDir()
	if err := os.CopyFS(filepath.Join(umbrella, "charts", "kupra"), os.DirFS("../../charts/kupra")); err != nil {
		t.Fatal(err)
	}
	chart := "apiVersion: v2\nname: umbrella\nversion: 0.1.0\ndependencies:\n  - name: kupra\n    version: \">=0.0.0-0\"\n"
	if err := os.WriteFile(filepath.Join(umbrella, "Chart.yaml"), []byte(chart), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("helm", "template", "umbrella", umbrella, "--set", "global.example=yes").CombinedOutput(); err != nil {
		t.Errorf("rendering kupra as a dependency: %v\n%s", err, out)
	}
}

// The dashboard needs its settings, and says which is missing.
func TestDashboardNeedsSettings(t *testing.T) {
	out, err := exec.Command("helm", "template", "kupra", "../../charts/kupra", "--set", "dashboard.enabled=true").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "dashboard.publicURL is required") {
		t.Errorf("rendering the dashboard without settings gives %q, want dashboard.publicURL required", out)
	}
}

// The dashboard runs one pod at a time, also while updating, as pods don't share refreshes of a session.
func TestDashboardRunsOnePod(t *testing.T) {
	for _, u := range render(t, dashboard...) {
		if u.GetKind() != "Deployment" || u.GetName() != "kupra-dashboard" {
			continue
		}
		replicas, _, _ := unstructured.NestedFieldNoCopy(u.Object, "spec", "replicas")
		strategy, _, _ := unstructured.NestedString(u.Object, "spec", "strategy", "type")
		if fmt.Sprint(replicas) != "1" || strategy != "Recreate" {
			t.Errorf("dashboard runs %v replicas with strategy %q, want 1 with Recreate", replicas, strategy)
		}
		return
	}
	t.Error("the chart has no kupra-dashboard Deployment")
}
