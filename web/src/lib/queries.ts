// One-off requests to the backend, as TanStack queries and mutation functions. Live data comes from the watch stream instead.

import { toastManager } from '@immich/ui';
import { QueryClient, queryOptions } from '@tanstack/svelte-query';
import { kube, request, type Session } from './api.ts';
import {
  apiVersion,
  type Preview,
  type PreviewSnapshot,
  type ResourceAttributes,
  type SelfSubjectAccessReview,
} from './kube/types.ts';

/** queryClient caches the page's one-off requests. A failed action shows its error as a toast. */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 60_000, refetchOnWindowFocus: false },
    mutations: { onError: (error) => toastManager.danger(error.message) },
  },
});

/** sessionQuery is who the signed-in user is. */
export const sessionQuery = queryOptions({
  queryKey: ['session'],
  queryFn: () => request<Session>('/api/session'),
  staleTime: Infinity,
});

/** accessQuery asks the API server whether the user may do something, with a SelfSubjectAccessReview. */
export const accessQuery = (attributes: ResourceAttributes) =>
  queryOptions({
    queryKey: ['access', attributes],
    queryFn: async () => {
      const review = await kube<SelfSubjectAccessReview>(
        '/apis/authorization.k8s.io/v1/selfsubjectaccessreviews',
        'POST',
        {
          apiVersion: 'authorization.k8s.io/v1',
          kind: 'SelfSubjectAccessReview',
          spec: { resourceAttributes: { group: 'kupra.futo.tech', ...attributes } },
        } satisfies SelfSubjectAccessReview,
      );
      return review.status?.allowed ?? false;
    },
  });

/** path returns the API path of kupra's resources of this plural in a namespace, or of one of them. */
export const path = (plural: 'previews' | 'previewsnapshots', namespace: string, name?: string) =>
  `/apis/${apiVersion}/namespaces/${encodeURIComponent(namespace)}/${plural}${name ? `/${encodeURIComponent(name)}` : ''}`;

const namespaceOf = (object: { metadata?: { namespace?: string } }) => object.metadata?.namespace ?? '';
const nameOf = (object: { metadata?: { name?: string } }) => object.metadata?.name ?? '';

/** createPreview creates a preview. */
export const createPreview = (preview: Preview) =>
  kube<Preview>(path('previews', namespaceOf(preview)), 'POST', preview);

/** updatePreview replaces a preview's parameters and the snapshot it restores from. */
export const updatePreview = ({ metadata, spec }: Preview) =>
  kube<Preview>(
    path('previews', metadata?.namespace ?? '', metadata?.name),
    'PATCH',
    [
      { op: 'add', path: '/spec/parameters', value: spec.parameters ?? {} },
      // The API server drops a null restoreFrom, which starts the preview empty.
      { op: 'add', path: '/spec/restoreFrom', value: spec.restoreFrom ?? null },
    ],
    'application/json-patch+json',
  );

/** resetAnnotation requests a reset of a preview when it changes. */
export const resetAnnotation = 'kupra.futo.tech/reset-requested-at';

/** resetPreview requests a reset, the way ADR 3 describes. */
export const resetPreview = (preview: Preview) =>
  kube<Preview>(
    path('previews', namespaceOf(preview), nameOf(preview)),
    'PATCH',
    { metadata: { annotations: { [resetAnnotation]: new Date().toISOString() } } },
    'application/merge-patch+json',
  );

/** deletePreview deletes a preview. */
export const deletePreview = (preview: Preview) =>
  kube<unknown>(path('previews', namespaceOf(preview), nameOf(preview)), 'DELETE');

/** createSnapshot creates a snapshot. */
export const createSnapshot = (snapshot: PreviewSnapshot) =>
  kube<PreviewSnapshot>(path('previewsnapshots', namespaceOf(snapshot)), 'POST', snapshot);

/** deleteSnapshot deletes a snapshot. */
export const deleteSnapshot = (snapshot: PreviewSnapshot) =>
  kube<unknown>(path('previewsnapshots', namespaceOf(snapshot), nameOf(snapshot)), 'DELETE');
