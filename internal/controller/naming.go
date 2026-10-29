package controller

import "github.com/futo-org/kupra/internal/names"

const (
	LabelApp              = "kupra.futo.tech/app"
	LabelPreview          = "kupra.futo.tech/preview"
	LabelPreviewNamespace = "kupra.futo.tech/preview-namespace"
	AnnotationPreviewUID  = "kupra.futo.tech/preview-uid"

	FieldManager = "kupra"
	Finalizer    = "kupra.futo.tech/cleanup"
)

// previewNamespaceName is <namespace>-<preview>-<hash>, with the readable part shortened to fit. The hash keeps apart pairs like a-b/c and a/b-c.
func previewNamespaceName(namespace, name string) string {
	return names.Hashed(namespace+"-"+name, namespace+"/"+name)
}
