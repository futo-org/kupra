package names

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestPreviewNamespace(t *testing.T) {
	// Pairs that collided with a 4-character hash, through dots becoming dashes, a-b/c against a/b-c, and shortening.
	for _, pair := range [][2][2]string{
		{{"previews", "pr-125809"}, {"previews", "pr.125809"}},
		{{"immich-previews", "pr-72950"}, {"immich", "previews-pr-72950"}},
		{{"immich-previews", "feature-add-support-for-something-long-pr-82"}, {"immich-previews", "feature-add-support-for-something-long-pr-314"}},
		{{"platform-engineering-pull-request-preview-environments-prod", "pr-164"}, {"platform-engineering-pull-request-preview-environments-prod", "pr-287"}},
	} {
		a, b := PreviewNamespace(pair[0][0], pair[0][1]), PreviewNamespace(pair[1][0], pair[1][1])
		if a == b {
			t.Errorf("%s/%s and %s/%s both give %s", pair[0][0], pair[0][1], pair[1][0], pair[1][1], a)
		}
	}

	got := PreviewNamespace("immich-previews", "pr-1234")
	if !strings.HasPrefix(got, "immich-previews-pr-1234-") || len(got) != len("immich-previews-pr-1234-")+8 {
		t.Errorf("got %s, want immich-previews-pr-1234-<hash>", got)
	}

	for _, tc := range []struct{ namespace, name string }{
		{strings.Repeat("a", 63), strings.Repeat("b", 63)},
		{"previews", "release.v1.2.3"},
		{"previews", strings.Repeat("a", 46) + "-" + strings.Repeat("b", 10)},
	} {
		got := PreviewNamespace(tc.namespace, tc.name)
		if errs := validation.IsDNS1123Label(got); len(errs) > 0 {
			t.Errorf("namespace for %s/%s is %s: %v", tc.namespace, tc.name, got, errs)
		}
	}
}

func TestResource(t *testing.T) {
	claim := schema.GroupKind{Kind: "PersistentVolumeClaim"}
	base := ForResource("snap", "uid-1", claim, "data")
	// Each part of a name's identity gives a different name.
	names := map[string]string{
		"base":     base.Name("capture"),
		"purpose":  base.Name("data"),
		"owner":    ForResource("other", "uid-1", claim, "data").Name("capture"),
		"uid":      ForResource("snap", "uid-2", claim, "data").Name("capture"),
		"kind":     ForResource("snap", "uid-1", schema.GroupKind{Group: "postgresql.cnpg.io", Kind: "Cluster"}, "data").Name("capture"),
		"resource": ForResource("snap", "uid-1", claim, "db").Name("capture"),
		"cluster":  base.ClusterName("capture"),
	}
	seen := map[string]string{}
	for what, name := range names {
		if other, ok := seen[name]; ok {
			t.Errorf("%s and %s both give %s", what, other, name)
		}
		seen[name] = what
	}
	if !strings.HasPrefix(names["base"], "snap-data-capture-") || !strings.HasPrefix(names["cluster"], "kupra-snap-data-capture-") {
		t.Errorf("names = %v, want readable prefixes", names)
	}

	long := ForResource(strings.Repeat("s", 63), "uid", claim, strings.Repeat("r", 63))
	for _, name := range []string{long.Name("capture"), long.ClusterName("capture")} {
		if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
			t.Errorf("%s: %v", name, errs)
		}
	}
}
