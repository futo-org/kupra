package controller

import (
	"testing"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

func TestSameObject(t *testing.T) {
	ref := func(apiVersion, kind, name string) kuprav1alpha1.ResourceRef {
		return kuprav1alpha1.ResourceRef{APIVersion: apiVersion, Kind: kind, Name: name}
	}
	for _, tc := range []struct {
		a, b kuprav1alpha1.ResourceRef
		want bool
	}{
		{ref("apps/v1", "Deployment", "web"), ref("apps/v1", "Deployment", "web"), true},
		{ref("example.com/v1beta1", "Widget", "w"), ref("example.com/v1", "Widget", "w"), true},
		{ref("v1", "ConfigMap", "config"), ref("v1", "Secret", "config"), false},
		{ref("v1", "ConfigMap", "a"), ref("v1", "ConfigMap", "b"), false},
		{ref("example.com/v1", "Widget", "w"), ref("example.org/v1", "Widget", "w"), false},
	} {
		if got := sameObject(tc.a, tc.b); got != tc.want {
			t.Errorf("sameObject(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
