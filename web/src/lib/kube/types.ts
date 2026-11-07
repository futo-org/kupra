// kupra's resources and the Kubernetes types the dashboard uses, from the generated models.

export type { ICondition as Condition } from '@kubernetes-models/apimachinery/apis/meta/v1/Condition';
export type { IObjectMeta as ObjectMeta } from '@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta';
export type { IStatus as Status } from '@kubernetes-models/apimachinery/apis/meta/v1/Status';
export type { IResourceAttributes as ResourceAttributes } from 'kubernetes-models/authorization.k8s.io/v1/ResourceAttributes';
export type { ISelfSubjectAccessReview as SelfSubjectAccessReview } from 'kubernetes-models/authorization.k8s.io/v1/SelfSubjectAccessReview';
export type { IPreview as Preview } from './gen/kupra.futo.tech/v1alpha1/Preview.js';
export type { IPreviewApp as PreviewApp } from './gen/kupra.futo.tech/v1alpha1/PreviewApp.js';
export type { IPreviewSnapshot as PreviewSnapshot } from './gen/kupra.futo.tech/v1alpha1/PreviewSnapshot.js';

/** apiVersion is the API version of kupra's resources. */
export const apiVersion = 'kupra.futo.tech/v1alpha1';
