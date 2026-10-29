// Package names derives the names of the objects kupra creates.
package names

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

// PreviewNamespace is the namespace a preview's resources run in: <namespace>-<preview>-<hash>.
func PreviewNamespace(namespace, preview string) string {
	return hashed(namespace+"-"+preview, namespace+"/"+preview)
}

// Resource names the objects kupra creates for one state resource, on behalf of an owner such as the PreviewSnapshot that captures it.
type Resource struct {
	owner, resource, key string
}

func ForResource(owner string, ownerUID types.UID, gk schema.GroupKind, resource string) Resource {
	return Resource{owner: owner, resource: resource, key: string(ownerUID) + "/" + gk.String() + "/" + resource}
}

// Name returns a name for a namespaced object, given its purpose, such as "capture".
func (r Resource) Name(purpose string) string {
	return hashed(r.owner+"-"+r.resource+"-"+purpose, r.key+"/"+purpose)
}

// ClusterName returns a name for a cluster-scoped object, given its purpose.
func (r Resource) ClusterName(purpose string) string {
	return hashed("kupra-"+r.owner+"-"+r.resource+"-"+purpose, r.key+"/"+purpose)
}

// hashed is readable followed by a hash of key, shortened to fit a DNS label. The hash tells apart names that only differ in their key, or after the shortened part.
func hashed(readable, key string) string {
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])[:8]
	if max := validation.DNS1123LabelMaxLength - len(hash) - 1; len(readable) > max {
		readable = strings.TrimRight(readable[:max], "-.")
	}
	return strings.ReplaceAll(readable, ".", "-") + "-" + hash
}
