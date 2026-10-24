package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	LabelApp              = "kupra.futo.tech/app"
	LabelPreview          = "kupra.futo.tech/preview"
	LabelPreviewNamespace = "kupra.futo.tech/preview-namespace"
	AnnotationPreviewUID  = "kupra.futo.tech/preview-uid"

	FieldManager = "kupra"
	Finalizer    = "kupra.futo.tech/cleanup"
)

// previewNamespaceName is <namespace>-<preview>-<hash>. The hash keeps pairs like a-b/c and a/b-c apart, and the readable part is shortened to fit.
func previewNamespaceName(namespace, name string) string {
	sum := sha256.Sum256([]byte(namespace + "/" + name))
	hash := hex.EncodeToString(sum[:])[:4]
	readable := namespace + "-" + name
	if max := validation.DNS1123LabelMaxLength - len(hash) - 1; len(readable) > max {
		readable = strings.TrimRight(readable[:max], "-.")
	}
	return strings.ReplaceAll(readable, ".", "-") + "-" + hash
}
