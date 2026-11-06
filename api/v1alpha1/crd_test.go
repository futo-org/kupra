package v1alpha1

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
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
		if _, ok := property["description"]; !ok {
			t.Errorf("%s.%s has no description", path, name)
		}
		undescribed(t, path+"."+name, property)
	}
	if items, ok := schema["items"].(map[string]any); ok {
		undescribed(t, path+"[]", items)
	}
}

// The chart's values schema lists the PreviewPolicy spec's fields for its defaultPolicy, so a typo fails the install.
func TestValuesSchemaHasPolicyFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "charts", "kupra", "crds", "kupra.futo.tech_previewpolicies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var crd struct {
		Spec struct {
			Versions []struct {
				Schema struct {
					OpenAPIV3Schema struct {
						Properties struct {
							Spec struct {
								Properties map[string]any
							}
						}
					} `json:"openAPIV3Schema"`
				}
			}
		}
	}
	if err := yaml.Unmarshal(data, &crd); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "charts", "kupra", "values.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		Properties struct {
			DefaultPolicy struct {
				Properties map[string]any
			} `json:"defaultPolicy"`
		}
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	want := slices.Sorted(maps.Keys(crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties.Spec.Properties))
	got := slices.Sorted(maps.Keys(values.Properties.DefaultPolicy.Properties))
	if !slices.Equal(got, want) {
		t.Errorf("values schema's defaultPolicy has %v, want the PreviewPolicy spec's %v", got, want)
	}
}
