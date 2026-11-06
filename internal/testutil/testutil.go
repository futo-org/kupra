// Package testutil holds helpers for kupra's tests.
package testutil

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// Condition describes a condition for a test's failure message, as its status, reason and message.
func Condition(c *metav1.Condition) string {
	if c == nil {
		return "missing"
	}
	s := fmt.Sprintf("%s (%s)", c.Status, c.Reason)
	if c.Message != "" {
		s += ": " + c.Message
	}
	return s
}

// Conditions describes a list of conditions for a test's failure message.
func Conditions(conditions []metav1.Condition) string {
	if len(conditions) == 0 {
		return "none"
	}
	var parts []string
	for i := range conditions {
		parts = append(parts, conditions[i].Type+" "+Condition(&conditions[i]))
	}
	return strings.Join(parts, "; ")
}

// Inventory describes a preview's inventory for a test's failure message.
func Inventory(entries []kuprav1alpha1.InventoryEntry) string {
	if len(entries) == 0 {
		return "empty"
	}
	var parts []string
	for _, e := range entries {
		s := e.Kind + " " + e.Name
		if e.State {
			s += " (state)"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// Subject describes an RBAC subject for a test's failure message.
func Subject(s rbacv1.Subject) string {
	if s.Namespace != "" {
		return s.Kind + " " + s.Namespace + "/" + s.Name
	}
	return s.Kind + " " + s.Name
}

// DataSource describes a claim's data source for a test's failure message.
func DataSource(ref *corev1.TypedObjectReference) string {
	if ref == nil {
		return "none"
	}
	return ref.Kind + " " + ref.Name
}
