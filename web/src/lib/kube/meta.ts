// Types from Kubernetes' apimachinery that kupra's resources and the API use. The CRDs leave metadata untyped.

/** ObjectMeta is metadata that all persisted resources must have. */
export type ObjectMeta = {
  name?: string;
  generateName?: string;
  namespace?: string;
  uid?: string;
  resourceVersion?: string;
  generation?: number;
  creationTimestamp?: string;
  deletionTimestamp?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  finalizers?: string[];
  ownerReferences?: OwnerReference[];
  managedFields?: unknown[];
};

/** OwnerReference identifies an owning object. */
export type OwnerReference = {
  apiVersion: string;
  kind: string;
  name: string;
  uid: string;
  controller?: boolean;
  blockOwnerDeletion?: boolean;
};

/** ListMeta is metadata that synthetic resources must have, such as lists. */
export type ListMeta = {
  resourceVersion?: string;
  continue?: string;
  remainingItemCount?: number;
};

/** List is a list of objects of one kind. */
export type List<T> = {
  apiVersion?: string;
  kind?: string;
  metadata: ListMeta;
  items: T[];
};

/** Status is the API server's return value for calls that don't return other objects, such as errors. */
export type Status = {
  apiVersion?: string;
  kind: 'Status';
  metadata?: ListMeta;
  status?: 'Success' | 'Failure';
  message?: string;
  reason?: string;
  code?: number;
};

/** Condition is one aspect of the current state of a resource. */
export type Condition = {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  observedGeneration?: number;
  lastTransitionTime: string;
  reason: string;
  message: string;
};

/** WatchEvent is an event in a watch stream. */
export type WatchEvent<T> =
  { type: 'ADDED' | 'MODIFIED' | 'DELETED' | 'BOOKMARK'; object: T } | { type: 'ERROR'; object: Status };
