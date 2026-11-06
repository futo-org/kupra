package v1alpha1

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

// kubectl explain shows each field's description, so every field has one.
func TestFieldsAreDescribed(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "charts", "kupra", "crds", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("finding CRDs: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var crd struct {
			Spec struct {
				Names    struct{ Kind string }
				Versions []struct {
					Schema struct {
						OpenAPIV3Schema map[string]any `json:"openAPIV3Schema"`
					}
				}
			}
		}
		if err := yaml.Unmarshal(data, &crd); err != nil {
			t.Fatal(err)
		}
		for _, v := range crd.Spec.Versions {
			// The standard fields at the resource's root are described by Kubernetes, not kupra.
			root := v.Schema.OpenAPIV3Schema
			properties, _ := root["properties"].(map[string]any)
			for _, name := range []string{"apiVersion", "kind", "metadata"} {
				delete(properties, name)
			}
			undescribed(t, crd.Spec.Names.Kind, root)
		}
	}
}

func undescribed(t *testing.T, path string, schema map[string]any) {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	for name, p := range properties {
		property, _ := p.(map[string]any)
		if description, _ := property["description"].(string); description == "" {
			t.Errorf("%s.%s has no description", path, name)
		}
		undescribed(t, path+"."+name, property)
	}
	if items, ok := schema["items"].(map[string]any); ok {
		undescribed(t, path+"[]", items)
	}
}
