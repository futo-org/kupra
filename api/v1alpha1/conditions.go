package v1alpha1

// Condition types, following the conventions kstatus and Flux use.
const (
	ConditionReady       = "Ready"
	ConditionReconciling = "Reconciling"
	ConditionStalled     = "Stalled"
)
