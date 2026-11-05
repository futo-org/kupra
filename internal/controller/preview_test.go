package controller

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
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

func TestStallAfterTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		since   time.Duration
		timeout *metav1.Duration
		stalled bool
	}{
		{name: "default timeout not reached", since: 29 * time.Minute},
		{name: "default timeout passed", since: 31 * time.Minute, stalled: true},
		{name: "app's timeout not reached", since: 31 * time.Minute, timeout: &metav1.Duration{Duration: time.Hour}},
		{name: "app's timeout passed", since: 2 * time.Minute, timeout: &metav1.Duration{Duration: time.Minute}, stalled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &kuprav1alpha1.Preview{Status: kuprav1alpha1.PreviewStatus{Conditions: []metav1.Condition{{
				Type:               meta.ReconcilingCondition,
				Status:             metav1.ConditionTrue,
				Reason:             meta.ProgressingReason,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-tc.since)),
			}}}}
			app := &kuprav1alpha1.PreviewApp{Spec: kuprav1alpha1.PreviewAppSpec{Timeout: tc.timeout}}
			stallAfterTimeout(p, app, "waiting for Deployment web")

			c := apimeta.FindStatusCondition(p.Status.Conditions, meta.StalledCondition)
			if stalled := c != nil && c.Status == metav1.ConditionTrue; stalled != tc.stalled {
				t.Fatalf("Stalled = %+v, want stalled %v", c, tc.stalled)
			}
			if tc.stalled && (c.Reason != ReasonTimeout || !strings.Contains(c.Message, "waiting for Deployment web")) {
				t.Errorf("Stalled = %+v, want reason %s and the readiness message", c, ReasonTimeout)
			}
			if !apimeta.IsStatusConditionTrue(p.Status.Conditions, meta.ReconcilingCondition) {
				t.Error("Reconciling isn't True any more, but kupra keeps trying after the timeout")
			}
		})
	}
}

func TestOwnerSubjects(t *testing.T) {
	p := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"kupra.futo.tech/created-by": "system:serviceaccount:flux-system:flux"}},
		Spec: kuprav1alpha1.PreviewSpec{Owners: []rbacv1.Subject{
			{Kind: rbacv1.GroupKind, Name: "reviewers"},
			{Kind: rbacv1.ServiceAccountKind, Namespace: "ci", Name: "Not_Valid"},
			{Kind: rbacv1.ServiceAccountKind, Namespace: "ci", Name: "deployer"},
		}},
	}
	want := []rbacv1.Subject{
		{Kind: rbacv1.ServiceAccountKind, Namespace: "flux-system", Name: "flux"},
		{APIGroup: rbacv1.GroupName, Kind: rbacv1.GroupKind, Name: "reviewers"},
		{Kind: rbacv1.ServiceAccountKind, Namespace: "ci", Name: "deployer"},
	}
	if got := ownerSubjects(p); !slices.Equal(got, want) {
		t.Errorf("ownerSubjects = %+v, want %+v", got, want)
	}
}

func TestNextCapture(t *testing.T) {
	now := metav1.Now()
	snapshot := func(name string, finalizer, deleting bool) kuprav1alpha1.PreviewSnapshot {
		s := kuprav1alpha1.PreviewSnapshot{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if finalizer {
			s.Finalizers = []string{Finalizer}
		}
		if deleting {
			s.DeletionTimestamp = &now
		}
		return s
	}
	for _, tc := range []struct {
		name      string
		snapshots []kuprav1alpha1.PreviewSnapshot
		want      string
	}{
		{"none", nil, ""},
		{"ready", []kuprav1alpha1.PreviewSnapshot{snapshot("a", true, false), snapshot("b", true, false)}, "a"},
		{"oldest has no finalizer yet", []kuprav1alpha1.PreviewSnapshot{snapshot("a", false, false), snapshot("b", true, false)}, ""},
		{"oldest is being deleted", []kuprav1alpha1.PreviewSnapshot{snapshot("a", true, true), snapshot("b", true, false)}, "b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			if s := nextCapture(tc.snapshots); s != nil {
				got = s.Name
			}
			if got != tc.want {
				t.Errorf("next capture = %q, want %q", got, tc.want)
			}
		})
	}
}
