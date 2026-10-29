// Package state is kupra's support for kinds of state: resources whose data kupra restores and captures.
//
// Each kind is written as pure functions. Given a state resource and the objects it asked to observe, a kind returns the objects it wants to exist, the ones it wants gone, the ones to observe next, and whether it's done. The controllers apply that and call it again, so progress lives in cluster objects rather than in memory.
package state

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/futo-org/kupra/internal/names"
)

// Kind is support for one kind of state.
type Kind interface {
	// Check finds problems with a template resource as written, with its expressions, before any preview uses it.
	Check(resource map[string]any, fldPath *field.Path) field.ErrorList
	// Restore works out how a state resource starts, empty or from a capture.
	Restore(RestoreRequest) Response
	// Ready reports whether the live resource is ready for the rest of the preview to use.
	Ready(live *unstructured.Unstructured) bool
	// Capture works out how to capture a state resource into a snapshot.
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

// Observed holds the live objects a kind asked to observe. Objects that don't exist are missing.
type Observed map[Ref]*unstructured.Unstructured

// Response is what a kind wants done.
type Response struct {
	// Resource is the state resource to apply, when restoring. It's applied as the preview's template account.
	Resource *unstructured.Unstructured
	// Apply are objects kupra applies with its own permissions, before the resource.
	Apply []*unstructured.Unstructured
	// Delete are objects kupra deletes with its own permissions.
	Delete []Ref
	// Observe are objects to read and pass back in the next request.
	Observe []Ref
	// Done is set once restoring has set up everything the resource needs, or capturing has finished.
	Done bool
	// Capture is what a finished capture recorded.
	Capture *Capture
	// Failed explains a failure that retrying won't fix.
	Failed string
}

// RestoreRequest asks how a state resource starts.
type RestoreRequest struct {
	// Resource is the rendered state resource.
	Resource *unstructured.Unstructured
	// Live is the resource as it exists, if it does. A resource that exists has already been restored.
	Live *unstructured.Unstructured
	// From is the capture to restore from, or nil to start empty.
	From *Source
	// Names names the objects the kind creates.
	Names Names
	// Labels go on every object the kind creates, so they can be found and cleaned up.
	Labels   map[string]string
	Observed Observed
}

// Source is a capture in a PreviewSnapshot.
type Source struct {
	// Namespace is the PreviewSnapshot's namespace, where its data is.
	Namespace string
	Data      []corev1.TypedLocalObjectReference
}

// CaptureRequest asks how to capture a state resource.
type CaptureRequest struct {
	// Resource is the state resource, as it exists in the preview's namespace.
	Resource *unstructured.Unstructured
	// Namespace is the PreviewSnapshot's namespace, where captured data is moved to.
	Namespace string
	// Names names the objects the kind creates.
	Names Names
	// Labels go on every object the kind creates, so they can be found and cleaned up.
	Labels   map[string]string
	Observed Observed
}

// Capture is what a capture recorded: the objects in the PreviewSnapshot's namespace that hold its data.
type Capture struct {
	Data []corev1.TypedLocalObjectReference
}

// Names derives names for the objects a kind creates for one state resource. Different resources, previews and snapshots get different names.
type Names interface {
	// Name returns a name for a namespaced object, unique within the resource's work, made from a short purpose such as "capture".
	Name(purpose string) string
	// ClusterName returns a name for a cluster-scoped object, unique in the cluster.
	ClusterName(purpose string) string
}

var kinds = map[schema.GroupKind]Kind{
	{Kind: "PersistentVolumeClaim"}: persistentVolumeClaim{},
}

// For returns the support for a kind of resource, if kupra has any.
func For(gk schema.GroupKind) (Kind, bool) {
	k, ok := kinds[gk]
	return k, ok
}

// NewNames returns the Names for a state resource's work on behalf of an owner, such as a PreviewSnapshot capturing it.
func NewNames(owner string, ownerUID types.UID, gk schema.GroupKind, resource string) Names {
	return resourceNames{owner: owner, key: string(ownerUID) + "/" + gk.String() + "/" + resource, resource: resource}
}

type resourceNames struct {
	owner, key, resource string
}

func (n resourceNames) Name(purpose string) string {
	return names.Hashed(n.owner+"-"+n.resource+"-"+purpose, n.key+"/"+purpose)
}

func (n resourceNames) ClusterName(purpose string) string {
	return names.Hashed("kupra-"+n.owner+"-"+n.resource+"-"+purpose, n.key+"/"+purpose)
}
