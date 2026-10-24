package controller

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestPreviewNamespaceName(t *testing.T) {
	if a, b := previewNamespaceName("a-b", "c"), previewNamespaceName("a", "b-c"); a == b {
		t.Errorf("a-b/c and a/b-c both give %s", a)
	}

	got := previewNamespaceName("immich-previews", "pr-1234")
	if !strings.HasPrefix(got, "immich-previews-pr-1234-") || len(got) != len("immich-previews-pr-1234-")+4 {
		t.Errorf("got %s, want immich-previews-pr-1234-<hash>", got)
	}

	for _, tc := range []struct{ namespace, name string }{
		{strings.Repeat("a", 63), strings.Repeat("b", 63)},
		{"previews", "release.v1.2.3"},
		{"previews", strings.Repeat("a", 46) + "-" + strings.Repeat("b", 10)},
	} {
		got := previewNamespaceName(tc.namespace, tc.name)
		if errs := validation.IsDNS1123Label(got); len(errs) > 0 {
			t.Errorf("namespace for %s/%s is %s: %v", tc.namespace, tc.name, got, errs)
		}
	}
}
