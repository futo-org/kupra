package controller

// Conditions kupra sets besides Flux's Ready, Reconciling and Stalled.
const (
	// ConditionRestored is true once a preview's state has started, empty or from a snapshot.
	ConditionRestored = "Restored"
)

// Reasons for kupra's conditions.
const (
	// PreviewApp checks, and PreviewPolicies.
	ReasonInvalid             = "Invalid"
	ReasonCheckFailed         = "CheckFailed"
	ReasonPolicyNotFound      = "PolicyNotFound"
	ReasonClusterRoleNotFound = "ClusterRoleNotFound"
	ReasonNoTemplateRole      = "NoTemplateRole"
	ReasonCannotBindRole      = "CannotBindRole"

	// Previews.
	ReasonAppNotFound          = "AppNotFound"
	ReasonAppNotReady          = "AppNotReady"
	ReasonNamespaceConflict    = "NamespaceConflict"
	ReasonNamespaceTerminating = "NamespaceTerminating"
	ReasonRenderFailed         = "RenderFailed"
	ReasonApplyFailed          = "ApplyFailed"
	ReasonTimeout              = "Timeout"
	ReasonTerminating          = "Terminating"

	// Captures, restores and resets.
	ReasonCapturing        = "Capturing"
	ReasonResetting        = "Resetting"
	ReasonSnapshotNotFound = "SnapshotNotFound"
	ReasonSnapshotNotReady = "SnapshotNotReady"
	ReasonSnapshotUnusable = "SnapshotUnusable"
	ReasonRestoreFailed    = "RestoreFailed"
	ReasonCaptureFailed    = "CaptureFailed"
	ReasonWaiting          = "Waiting"

	// PreviewSnapshots.
	ReasonPreviewNotFound = "PreviewNotFound"
	ReasonPreviewDeleted  = "PreviewDeleted"
	ReasonInUse           = "InUse"
)
