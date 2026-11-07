// The mockup's store, shaped like the dashboard's watch stream (GET /api/watch), so it can be swapped for it.

import type { Preview, PreviewApp, PreviewSnapshot } from '#lib/kube/kupra.gen.ts';
import type { ObjectMeta, Status } from '#lib/kube/meta.ts';
import { roles as initialRoles, seed } from './data.ts';

/** Objects are the objects in each kind of collection. Collections are named like previews/<namespace>. */
export type Objects = {
  previews: Preview;
  previewsnapshots: PreviewSnapshot;
  previewapps: PreviewApp;
  'app-namespaces': { metadata?: ObjectMeta };
};

type KubeObject = Objects[keyof Objects];

/** WatchMessage is a message on the watch stream. */
export type WatchMessage = {
  collection: string;
  type: 'ADDED' | 'MODIFIED' | 'DELETED' | 'SYNCED' | 'ERROR';
  object?: KubeObject;
  error?: Status;
};

/** Collection is a collection's objects, by namespace/name, and whether it's caught up. */
export type Collection<T> = { synced: boolean; error?: Status; objects: Record<string, T> };

const collections = $state<Record<string, Collection<KubeObject>>>({});

const key = ({ metadata }: KubeObject) => `${metadata?.namespace ?? ''}/${metadata?.name ?? ''}`;

/** apply applies a watch message to the store. */
export const apply = ({ collection, type, object, error }: WatchMessage) => {
  const current = (collections[collection] ??= { synced: false, objects: {} });
  switch (type) {
    case 'ADDED':
    case 'MODIFIED': {
      if (object) {
        current.objects[key(object)] = object;
      }
      break;
    }
    case 'DELETED': {
      if (object) {
        delete current.objects[key(object)];
      }
      break;
    }
    case 'SYNCED': {
      current.synced = true;
      current.error = undefined;
      break;
    }
    case 'ERROR': {
      current.error = error;
      break;
    }
  }
};

/** list returns the objects in a collection. */
export const list = <R extends keyof Objects>(resource: R, namespace?: string) =>
  Object.values(collections[namespace ? `${resource}/${namespace}` : resource]?.objects ?? {}) as Objects[R][];

/** send applies a change to an object, as the stream would report it. */
export const send = <R extends Exclude<keyof Objects, 'app-namespaces'>>(
  resource: R,
  type: 'ADDED' | 'MODIFIED' | 'DELETED',
  object: Objects[R],
) => apply({ collection: `${resource}/${object.metadata?.namespace}`, type, object });

/** access is the user's role in each namespace. The mockup lets the user switch it. */
export const access = $state({ roles: { ...initialRoles } });

// The mockup starts with everything synced.
for (const { resource, objects } of [
  { resource: 'previewapps', objects: seed.apps },
  { resource: 'previews', objects: seed.previews },
  { resource: 'previewsnapshots', objects: seed.snapshots },
] as const) {
  for (const object of objects) {
    apply({ collection: `${resource}/${object.metadata?.namespace}`, type: 'ADDED', object });
  }
}
for (const namespace of new Set(seed.apps.map((app) => app.metadata?.namespace ?? ''))) {
  apply({ collection: 'app-namespaces', type: 'ADDED', object: { metadata: { name: namespace } } });
  for (const resource of ['previewapps', 'previews', 'previewsnapshots']) {
    apply({ collection: `${resource}/${namespace}`, type: 'SYNCED' });
  }
}
apply({ collection: 'app-namespaces', type: 'SYNCED' });
