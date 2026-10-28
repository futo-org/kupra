package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSameObject(t *testing.T) {
	ref := func(group, kind, name string) corev1.TypedLocalObjectReference {
		r := corev1.TypedLocalObjectReference{Kind: kind, Name: name}
		if group != "" {
			r.APIGroup = &group
		}
		return r
	}
	for _, tc := range []struct {
		a, b corev1.TypedLocalObjectReference
		want bool
	}{
		{ref("apps", "Deployment", "web"), ref("apps", "Deployment", "web"), true},
		{ref("", "ConfigMap", "config"), ref("", "Secret", "config"), false},
		{ref("", "ConfigMap", "a"), ref("", "ConfigMap", "b"), false},
		{ref("example.com", "Widget", "w"), ref("example.org", "Widget", "w"), false},
	} {
		if got := sameObject(tc.a, tc.b); got != tc.want {
			t.Errorf("sameObject(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
