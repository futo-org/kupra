// Package chart checks kupra's Helm chart as rendered.
package chart

import (
	"bytes"
	"os/exec"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// Every object the chart creates carries its labels, so a release's objects can be found together.
func TestObjectsAreLabelled(t *testing.T) {
	out, err := exec.Command("helm", "template", "kupra", "../../charts/kupra", "--namespace", "kupra-system", "--set", "commonLabels.example=yes").Output()
	if err != nil {
		t.Fatalf("rendering the chart: %v", err)
	}
	for _, doc := range bytes.Split(out, []byte("\n---\n")) {
		var u unstructured.Unstructured
		if err := yaml.Unmarshal(doc, &u.Object); err != nil {
			t.Fatal(err)
		}
		if len(u.Object) == 0 {
			continue
		}
		labels := u.GetLabels()
		if labels["app.kubernetes.io/part-of"] != "kupra" {
			t.Errorf("%s %s isn't labelled app.kubernetes.io/part-of: kupra", u.GetKind(), u.GetName())
		}
		if labels["example"] != "yes" {
			t.Errorf("%s %s doesn't have the commonLabels", u.GetKind(), u.GetName())
		}
	}
}
