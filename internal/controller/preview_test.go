package controller

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
	"github.com/futo-org/kupra/internal/names"
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
			// A new message doesn't restart the timeout.
			starting(p, meta.ProgressingReason, "waiting for Deployment web")
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

// A reconciliation that starts from a stale read of a Preview doesn't overwrite what changed since: its inventory, or another controller's finalizer.
func TestReconcileFromStaleRead(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := kuprav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		stale func(*kuprav1alpha1.Preview)
	}{
		{"before kupra's finalizer", func(p *kuprav1alpha1.Preview) { p.Finalizers = nil }},
		{"with kupra's finalizer", func(p *kuprav1alpha1.Preview) { p.Finalizers = []string{Finalizer} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			live := &kuprav1alpha1.Preview{
				ObjectMeta: metav1.ObjectMeta{Name: "pr", Namespace: "ns", Finalizers: []string{Finalizer, "example.com/other"}},
				Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}},
				Status: kuprav1alpha1.PreviewStatus{Inventory: []kuprav1alpha1.InventoryEntry{
					{TypedLocalObjectReference: corev1.TypedLocalObjectReference{Kind: "ConfigMap", Name: "a"}},
				}},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).WithStatusSubresource(live).Build()
			if err := c.Get(ctx, client.ObjectKeyFromObject(live), live); err != nil {
				t.Fatal(err)
			}
			// The read the reconciliation starts from is a version behind.
			stale := live.DeepCopy()
			stale.Status.Inventory = nil
			tc.stale(stale)
			rv, _ := strconv.Atoi(live.ResourceVersion)
			stale.ResourceVersion = strconv.Itoa(rv - 1)
			// Every status write must be conditional on the version that was read.
			var statusPatches []string
			r := &PreviewReconciler{Client: interceptor.NewClient(c, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if p, ok := obj.(*kuprav1alpha1.Preview); ok {
						stale.DeepCopyInto(p)
						return nil
					}
					return c.Get(ctx, key, obj, opts...)
				},
				SubResourcePatch: func(ctx context.Context, c client.Client, subResource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
					data, err := patch.Data(obj)
					if err != nil {
						return err
					}
					statusPatches = append(statusPatches, string(data))
					return c.SubResource(subResource).Patch(ctx, obj, patch, opts...)
				},
			})}
			r.APIReader = c
			_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(live)})

			var got kuprav1alpha1.Preview
			if err := c.Get(ctx, client.ObjectKeyFromObject(live), &got); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(got.Finalizers, "example.com/other") {
				t.Errorf("finalizers = %v, want the other controller's kept", got.Finalizers)
			}
			if len(got.Status.Inventory) != 1 {
				t.Errorf("inventory = %v, want it kept", got.Status.Inventory)
			}
			if slices.Contains(stale.Finalizers, Finalizer) && len(statusPatches) == 0 {
				t.Error("no status was written, so its precondition wasn't checked")
			}
			for _, patch := range statusPatches {
				if !strings.Contains(patch, `"resourceVersion":"`+stale.ResourceVersion+`"`) {
					t.Errorf("status patch %s isn't conditional on the version that was read", patch)
				}
			}
		})
	}
}

func TestListMetadataDrifted(t *testing.T) {
	object := func(finalizers []string, applied string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetFinalizers(finalizers)
		if applied != "" {
			u.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: FieldManager, Operation: metav1.ManagedFieldsOperationApply, FieldsV1: metav1.NewFieldsV1(applied)}})
		}
		return u
	}
	appliedA := `{"f:metadata":{"f:finalizers":{".":{},"v:\"example.com/a\"":{}}}}`
	withOwner := func(u *unstructured.Unstructured, controller bool) *unstructured.Unstructured {
		u.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: "owner-uid", Controller: &controller}})
		return u
	}
	for _, tc := range []struct {
		name      string
		obj, live *unstructured.Unstructured
		want      bool
	}{
		{"unchanged", object([]string{"example.com/a"}, ""), object([]string{"example.com/a"}, appliedA), false},
		{"added", object([]string{"example.com/a"}, ""), object(nil, ""), true},
		{"removed from the template", object(nil, ""), object([]string{"example.com/a"}, appliedA), true},
		{"another controller's", object(nil, ""), object([]string{"kubernetes.io/pvc-protection"}, ""), false},
		{"owner reference changed", withOwner(object(nil, ""), true), withOwner(object(nil, ""), false), true},
		{"owner reference unchanged", withOwner(object(nil, ""), true), withOwner(object(nil, ""), true), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := listMetadataDrifted(tc.obj, tc.live); got != tc.want {
				t.Errorf("listMetadataDrifted = %v, want %v", got, tc.want)
			}
		})
	}
}

// A capture records the parameter defaults of the PreviewApp that rendered the preview, even if the stored one has changed since.
func TestStartCaptureRecordsRenderedDefaults(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := kuprav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	v1, v2 := "v1", "v2"
	stored := &kuprav1alpha1.PreviewApp{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "ns"},
		Spec:       kuprav1alpha1.PreviewAppSpec{Parameters: []kuprav1alpha1.Parameter{{Name: "tag", Default: &v2}}},
	}
	rendered := stored.DeepCopy()
	rendered.Spec.Parameters[0].Default = &v1
	p := &kuprav1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{Name: "pr", Namespace: "ns"},
		Spec:       kuprav1alpha1.PreviewSpec{AppRef: meta.LocalObjectReference{Name: "app"}, Parameters: map[string]string{"title": "main"}},
	}
	s := &kuprav1alpha1.PreviewSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: "ns", Finalizers: []string{Finalizer}},
		Spec:       kuprav1alpha1.PreviewSnapshotSpec{Source: kuprav1alpha1.SnapshotSource{PreviewRef: meta.LocalObjectReference{Name: "pr"}}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored, s).WithStatusSubresource(s).
		WithIndex(&kuprav1alpha1.PreviewSnapshot{}, sourceIndex, func(obj client.Object) []string {
			return []string{obj.(*kuprav1alpha1.PreviewSnapshot).Spec.Source.PreviewRef.Name}
		}).Build()
	r := &PreviewReconciler{Client: c}

	started, err := r.startCapture(ctx, p, rendered)
	if err != nil || !started {
		t.Fatalf("startCapture = %v, %v; want started", started, err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(s), s); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"tag": "v1", "title": "main"}; !maps.Equal(s.Status.Parameters, want) {
		t.Errorf("parameters = %v, want %v", s.Status.Parameters, want)
	}
}

func TestOwnsAnnotation(t *testing.T) {
	entry := func(manager string, op metav1.ManagedFieldsOperationType, fields string) metav1.ManagedFieldsEntry {
		return metav1.ManagedFieldsEntry{Manager: manager, Operation: op, FieldsV1: metav1.NewFieldsV1(fields)}
	}
	owned := `{"f:metadata":{"f:annotations":{"f:kupra.futo.tech/reset-requested-at":{}}}}`
	for _, tc := range []struct {
		name    string
		entries []metav1.ManagedFieldsEntry
		want    bool
	}{
		{"applied by kupra", []metav1.ManagedFieldsEntry{entry(FieldManager, metav1.ManagedFieldsOperationApply, owned)}, true},
		{"set by kubectl", []metav1.ManagedFieldsEntry{entry("kubectl-annotate", metav1.ManagedFieldsOperationUpdate, owned)}, false},
		{"other fields", []metav1.ManagedFieldsEntry{entry(FieldManager, metav1.ManagedFieldsOperationApply, `{"f:metadata":{"f:labels":{}}}`)}, false},
		{"none", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := &metav1.ObjectMeta{ManagedFields: tc.entries}
			if got := ownsAnnotation(obj, FieldManager, AnnotationResetRequestedAt); got != tc.want {
				t.Errorf("ownsAnnotation = %v, want %v", got, tc.want)
			}
		})
	}
}

// A preview deleted before the cache has seen its namespace still deletes it.
func TestDeleteBeforeNamespaceCached(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := kuprav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	p := &kuprav1alpha1.Preview{ObjectMeta: metav1.ObjectMeta{Name: "pr", Namespace: "ns", UID: "uid"}}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:        names.PreviewNamespace(p.Namespace, p.Name),
		Annotations: map[string]string{AnnotationPreviewUID: string(p.UID)},
	}}
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).
		WithIndex(&kuprav1alpha1.PreviewSnapshot{}, sourceIndex, func(obj client.Object) []string {
			return []string{obj.(*kuprav1alpha1.PreviewSnapshot).Spec.Source.PreviewRef.Name}
		}).Build()
	cache := interceptor.NewClient(live, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Namespace); ok {
				return apierrors.NewNotFound(corev1.Resource("namespaces"), key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	r := &PreviewReconciler{Client: cache, APIReader: live}
	if _, done, err := r.reconcileDelete(ctx, p); err != nil || done {
		t.Fatalf("reconcileDelete = done %v, %v; want it waiting for the namespace", done, err)
	}
	if err := live.Get(ctx, client.ObjectKeyFromObject(ns), &corev1.Namespace{}); !apierrors.IsNotFound(err) {
		t.Errorf("namespace: %v, want it deleted", err)
	}
}
