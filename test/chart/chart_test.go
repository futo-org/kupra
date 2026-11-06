// Package chart checks kupra's Helm chart as rendered.
package chart

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

// Every object the chart templates carries its labels, so a release's objects can be found together. Helm applies CRDs without templating them.
func TestObjectsAreLabelled(t *testing.T) {
	for _, u := range render(t, "--set", "commonLabels.example=yes") {
		labels := u.GetLabels()
		if labels["app.kubernetes.io/part-of"] != "kupra" {
			t.Errorf("%s %s isn't labelled app.kubernetes.io/part-of: kupra", u.GetKind(), u.GetName())
		}
		if labels["example"] != "yes" {
			t.Errorf("%s %s doesn't have the commonLabels", u.GetKind(), u.GetName())
		}
	}
}

// commonLabels can't replace the labels the operator's Deployment and Service select its pods by.
func TestCommonLabelsKeepSelectors(t *testing.T) {
	objs := render(t, "--set", "commonLabels.app\\.kubernetes\\.io/name=custom", "--set", "commonLabels.app\\.kubernetes\\.io/instance=custom")
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
			if d.GetKind() == "Deployment" {
				pods, _, _ = unstructured.NestedStringMap(d.Object, "spec", "template", "metadata", "labels")
			}
		}
		for k, v := range selector {
			if pods[k] != v {
				t.Errorf("%s %s selects %s=%s, but the operator's pods have %q", u.GetKind(), u.GetName(), k, v, pods[k])
			}
		}
	}
}

// operator.podLabels win over commonLabels on the operator's pods.
func TestPodLabelsWin(t *testing.T) {
	for _, u := range render(t, "--set", "commonLabels.team=common", "--set", "operator.podLabels.team=pods") {
		if u.GetKind() != "Deployment" {
			continue
		}
		pods, _, _ := unstructured.NestedStringMap(u.Object, "spec", "template", "metadata", "labels")
		if pods["team"] != "pods" {
			t.Errorf("the operator's pods are labelled team %q, want podLabels' pods", pods["team"])
		}
	}
}

// commonLabels go on every ClusterRole, so they may not be aggregation labels, which would add the chart's roles, and their access to Secrets, to other roles.
func TestCommonLabelsRejectAggregation(t *testing.T) {
	for _, key := range []string{"rbac\\.authorization\\.k8s\\.io/aggregate-to-view", "kupra\\.futo\\.tech/aggregate-to-template", "kupra\\.futo\\.tech/aggregate-to-operator"} {
		if out, err := exec.Command("helm", "template", "kupra", "../../charts/kupra", "--set-string", "commonLabels."+key+"=true").CombinedOutput(); err == nil || !bytes.Contains(out, []byte("invalid propertyName")) {
			t.Errorf("commonLabels %s was accepted, want the values schema to reject it", key)
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
