import { resources, type Kinds } from './kupra.gen.ts';

/** Kind is one of kupra's kinds. */
export type Kind = keyof Kinds;

/** ListOptions narrow a list or watch, as in Kubernetes' ListOptions. */
export type ListOptions = {
  fieldSelector?: string;
  labelSelector?: string;
};

/** collectionPath returns the dashboard's path for a collection of kupra's resources, in one namespace or all of them. */
export const collectionPath = (kind: Kind, namespace?: string) => {
  const { group, version, plural, namespaced } = resources[kind];
  const scope = namespaced && namespace !== undefined ? `namespaces/${encodeURIComponent(namespace)}/` : '';
  return `/api/kube/apis/${group}/${version}/${scope}${plural}`;
};

/** listPath returns the path that lists a collection. */
export const listPath = (kind: Kind, namespace: string | undefined, options: ListOptions = {}) =>
  withQuery(collectionPath(kind, namespace), options);

/** watchPath returns the path that watches a collection from resourceVersion. */
export const watchPath = (
  kind: Kind,
  namespace: string | undefined,
  resourceVersion: string,
  options: ListOptions = {},
) =>
  withQuery(collectionPath(kind, namespace), {
    ...options,
    watch: 'true',
    allowWatchBookmarks: 'true',
    resourceVersion,
  });

/** nameSelector returns a field selector for the object with this name. */
export const nameSelector = (name: string) => `metadata.name=${name}`;

const withQuery = (path: string, query: Record<string, string | undefined>) => {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined) {
      params.set(key, value);
    }
  }
  const search = params.toString();
  return search ? `${path}?${search}` : path;
};
