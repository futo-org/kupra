package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
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
