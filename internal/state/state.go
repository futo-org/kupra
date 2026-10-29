// Package state supports kinds of state: resources whose data kupra restores and captures.
//
// A kind is pure functions. Like a Crossplane composition function, it returns the objects it wants and the objects it needs to read, and the controller calls it again with those until it has nothing new to read.
package state

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/futo-org/kupra/internal/names"
)

// Kind is support for one kind of state.
type Kind interface {
	// Check validates a template resource, with its expressions.
	Check(resource map[string]any, fldPath *field.Path) field.ErrorList
	// Restore returns how a state resource starts, empty or from a capture.
	Restore(RestoreRequest) Response
	// Ready reports whether the rest of the preview can use a live resource.
	Ready(live *unstructured.Unstructured) bool
	// Restored reports whether a live resource no longer needs the capture it was restored from.
	Restored(live *unstructured.Unstructured) bool
	// Capture returns how to capture a state resource.
	Capture(CaptureRequest) Response
}

// Ref identifies an object. Namespace is empty for cluster-scoped objects.
type Ref struct {
	schema.GroupVersionKind
	Namespace string
	Name      string
}

// RefOf returns an object's Ref.
func RefOf(obj *unstructured.Unstructured) Ref {
	return Ref{GroupVersionKind: obj.GroupVersionKind(), Namespace: obj.GetNamespace(), Name: obj.GetName()}
}

// Observed holds the objects a kind asked to read, or nil for those that don't exist.
type Observed map[Ref]*unstructured.Unstructured

// Response is what a kind wants done.
type Response struct {
	// Resource is the state resource to apply, as the preview's template account.
	Resource *unstructured.Unstructured
	// Apply are objects to apply as kupra, before Resource.
	Apply []*unstructured.Unstructured
	// Delete are objects to delete as kupra.
	Delete []Ref
	// Observe are objects to read for the next call.
	Observe []Ref
	// Waiting says what the kind is waiting for, such as a VolumeSnapshot error that the snapshot controller retries.
	Waiting string
	// Done is set when the restore or capture needs nothing more.
	Done bool
	// Capture is the result of a finished capture.
	Capture *Capture
	// Failed is set when retrying won't help.
	Failed string
}

// RestoreRequest asks how a state resource starts.
type RestoreRequest struct {
	// Resource is the rendered resource.
	Resource *unstructured.Unstructured
	// Live is the resource as it exists, or nil.
	Live *unstructured.Unstructured
	// From is the capture to restore from, or nil to start empty.
	From     *Source
	Names    names.Resource
	Labels   map[string]string
	Observed Observed
}

// Source is a capture in a PreviewSnapshot.
type Source struct {
	// Namespace is the PreviewSnapshot's namespace.
	Namespace string
	Data      []corev1.TypedLocalObjectReference
}

// CaptureRequest asks how to capture a state resource.
type CaptureRequest struct {
	// Resource is the live resource.
	Resource *unstructured.Unstructured
	// Namespace is the PreviewSnapshot's namespace, where the capture is moved to.
	Namespace string
	Names     names.Resource
	Labels    map[string]string
	Observed  Observed
}

// Capture lists the objects in the PreviewSnapshot's namespace that hold a capture's data.
type Capture struct {
	Data []corev1.TypedLocalObjectReference
}

var kinds = map[schema.GroupKind]Kind{
	{Kind: "PersistentVolumeClaim"}: persistentVolumeClaim{},
}

// For returns the support for a kind, if there is any.
func For(gk schema.GroupKind) (Kind, bool) {
	k, ok := kinds[gk]
	return k, ok
}
