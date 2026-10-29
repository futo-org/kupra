package state

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// persistentVolumeClaim supports plain PersistentVolumeClaims, through the VolumeSnapshot API.
type persistentVolumeClaim struct{}

// Check refuses claims that set their own data source, since kupra sets it to restore them.
func (persistentVolumeClaim) Check(resource map[string]any, fldPath *field.Path) field.ErrorList {
	spec, _ := resource["spec"].(map[string]any)
	var errs field.ErrorList
	for _, key := range []string{"dataSource", "dataSourceRef", "dataSourceExpr", "dataSourceRefExpr"} {
		if _, ok := spec[key]; ok {
			errs = append(errs, field.Forbidden(fldPath.Child("spec", key), "kupra sets the data source of claims it handles as state; opt the claim out of being state to set your own"))
		}
	}
	if _, ok := resource["specExpr"]; ok {
		errs = append(errs, field.Forbidden(fldPath.Child("specExpr"), "kupra sets parts of the spec of claims it handles as state, so it can't be given as an expression"))
	}
	return errs
}

func (persistentVolumeClaim) Restore(resource *unstructured.Unstructured) *unstructured.Unstructured {
	return resource
}

// Ready is true once the claim exists: storage that binds volumes when a pod first uses them would otherwise never be ready.
func (persistentVolumeClaim) Ready(live *unstructured.Unstructured) bool {
	return live != nil
}
