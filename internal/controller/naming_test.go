package controller

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestPreviewNamespaceName(t *testing.T) {
	// Pairs that collided with a 4-character hash, through dots becoming dashes, a-b/c against a/b-c, and shortening.
	for _, pair := range [][2][2]string{
		{{"previews", "pr-125809"}, {"previews", "pr.125809"}},
		{{"immich-previews", "pr-72950"}, {"immich", "previews-pr-72950"}},
		{{"immich-previews", "feature-add-support-for-something-long-pr-82"}, {"immich-previews", "feature-add-support-for-something-long-pr-314"}},
		{{"platform-engineering-pull-request-preview-environments-prod", "pr-164"}, {"platform-engineering-pull-request-preview-environments-prod", "pr-287"}},
	} {
		a, b := previewNamespaceName(pair[0][0], pair[0][1]), previewNamespaceName(pair[1][0], pair[1][1])
		if a == b {
			t.Errorf("%s/%s and %s/%s both give %s", pair[0][0], pair[0][1], pair[1][0], pair[1][1], a)
		}
	}

	got := previewNamespaceName("immich-previews", "pr-1234")
	if !strings.HasPrefix(got, "immich-previews-pr-1234-") || len(got) != len("immich-previews-pr-1234-")+8 {
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
