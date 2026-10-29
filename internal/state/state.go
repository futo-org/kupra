// Package state is kupra's support for kinds of state: resources whose data kupra restores and captures. Each kind is written as pure functions, and the Preview controller does the API calls.
package state

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Kind is support for one kind of state.
type Kind interface {
	// Check finds problems with a template resource as written, with its expressions, before any preview uses it.
	Check(resource map[string]any, fldPath *field.Path) field.ErrorList
	// Restore returns the resource to apply for a preview that starts empty.
	Restore(resource *unstructured.Unstructured) *unstructured.Unstructured
	// Ready reports whether the live resource is ready for the rest of the preview to use.
	Ready(live *unstructured.Unstructured) bool
}

var kinds = map[schema.GroupKind]Kind{
	{Kind: "PersistentVolumeClaim"}: persistentVolumeClaim{},
}

// For returns the support for a kind of resource, if kupra has any.
func For(gk schema.GroupKind) (Kind, bool) {
	k, ok := kinds[gk]
	return k, ok
}
