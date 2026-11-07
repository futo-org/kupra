// What the dashboard works out from the watched resources.

import type { Session } from './api.ts';
import { createdByAnnotation, sourceAnnotation } from './kube/preview.ts';
import type { Preview, PreviewApp, PreviewSnapshot } from './kube/types.ts';
import { store } from './watch.svelte.ts';

const { list } = store;

/** AppGroup is a namespace's apps. */
export type AppGroup = { namespace: string; apps: PreviewApp[] };

/** appGroups returns the apps the user can see, by namespace, leaving out namespaces where they can see none. */
export const appGroups = (): AppGroup[] =>
  namespaces()
    .map((namespace) => ({ namespace, apps: list('previewapps', namespace) }))
    .filter(({ apps }) => apps.length > 0);

/** namespaces returns the namespaces that contain apps. */
export const namespaces = () =>
  list('app-namespaces')
    .map(({ metadata }) => metadata?.name ?? '')
    .toSorted((a, b) => a.localeCompare(b));

/** findApp returns the app with this name in this namespace. */
export const findApp = (namespace: string, name: string) =>
  list('previewapps', namespace).find((app) => app.metadata?.name === name);

/** findPreview returns the preview with this name in this namespace. */
export const findPreview = (namespace: string, name: string) =>
  list('previews', namespace).find((preview) => preview.metadata?.name === name);

/** previewsOf returns an app's previews, newest first. */
export const previewsOf = (namespace: string, app: string) =>
  newestFirst(list('previews', namespace).filter((preview) => preview.spec.appRef.name === app));

/** snapshotsOf returns the snapshots taken of an app's previews, newest first. */
export const snapshotsOf = (namespace: string, app: string) =>
  newestFirst(list('previewsnapshots', namespace).filter((snapshot) => snapshot.status?.appRef?.name === app));

const newestFirst = <T extends Preview | PreviewSnapshot>(objects: T[]) =>
  objects.toSorted((a, b) => (b.metadata?.creationTimestamp ?? '').localeCompare(a.metadata?.creationTimestamp ?? ''));

/** isOwner reports whether the user created or owns a preview. */
export const isOwner = (preview: Preview, user: Session) =>
  preview.metadata?.annotations?.[createdByAnnotation] === user.username ||
  (preview.spec.owners ?? []).some(
    ({ kind, name }) => (kind === 'User' && name === user.username) || (kind === 'Group' && user.groups.includes(name)),
  );

/** myPreviews returns the previews the user created or owns, across apps, newest first. */
export const myPreviews = (user: Session) =>
  newestFirst(
    namespaces()
      .flatMap((namespace) => list('previews', namespace))
      .filter((preview) => isOwner(preview, user)),
  );

/** Manager is the automation that manages a preview. */
export type Manager = { tool: string; kind: string; name: string; namespace: string };

/** managedBy returns the automation that manages a preview, from the labels it sets on what it applies. */
export const managedBy = (preview: Preview): Manager | undefined => {
  const labels = preview.metadata?.labels ?? {};
  const kinds = [
    { tool: 'Flux', kind: 'Kustomization', prefix: 'kustomize.toolkit.fluxcd.io' },
    { tool: 'Flux', kind: 'HelmRelease', prefix: 'helm.toolkit.fluxcd.io' },
    { tool: 'Flux', kind: 'ResourceSet', prefix: 'resourceset.fluxcd.controlplane.io' },
  ];
  for (const { tool, kind, prefix } of kinds) {
    const name = labels[`${prefix}/name`];
    if (name) {
      return { tool, kind, name, namespace: labels[`${prefix}/namespace`] ?? '' };
    }
  }
  return undefined;
};

/** Source is where a preview came from, such as a pull request. */
export type Source = { title: string; url: string };

/** source returns where a preview came from, if it says. */
export const source = (preview: Preview): Source | undefined => {
  const url = preview.metadata?.annotations?.[sourceAnnotation];
  if (!url) {
    return undefined;
  }
  const pull = /^https:\/\/github\.com\/([^/]+\/[^/]+)\/pull\/(\d+)$/.exec(url);
  if (pull) {
    return { title: `${pull[1]}#${pull[2]}`, url };
  }
  const branch = /^https:\/\/github\.com\/([^/]+\/[^/]+)\/tree\/(.+)$/.exec(url);
  if (branch) {
    return { title: `${branch[1]}@${branch[2]}`, url };
  }
  return { title: url.replace(/^https?:\/\//, ''), url };
};

/** restoreLabel says what a preview's state starts from, as it's set in its spec. */
export const restoreLabel = (preview: Preview) => {
  const restoreFrom = preview.spec.restoreFrom;
  if (restoreFrom?.snapshotRef) {
    return restoreFrom.snapshotRef.name;
  }
  return restoreFrom ? 'Newest matching snapshot' : 'Empty';
};

/** heldBy returns what keeps a deleted snapshot from being removed. */
export const heldBy = (snapshot: PreviewSnapshot) =>
  snapshot.status?.phase === 'Deleting'
    ? snapshot.status.conditions?.find((condition) => condition.type === 'Ready')?.message
    : undefined;

/** isFailing reports whether a preview failed or stalled. */
export const isFailing = (preview: Preview) =>
  preview.status?.phase === 'Failed' ||
  (preview.status?.conditions ?? []).some(({ type, status }) => type === 'Stalled' && status === 'True');

/** collectionsIn returns the collections of kupra's resources in a namespace, which an app's pages show. */
export const collectionsIn = (namespace: string) =>
  ['previewapps', 'previews', 'previewsnapshots'].map((resource) => `${resource}/${namespace}`);

/** previewOf returns an app's preview with this name, or undefined if the name belongs to another app's preview. */
export const previewOf = (namespace: string, app: string, name: string) => {
  const preview = findPreview(namespace, name);
  return preview?.spec.appRef.name === app ? preview : undefined;
};

/** resetSource says what a reset restores a preview's state from, if its spec says. */
export const resetSource = (preview: Preview) => (preview.spec.restoreFrom ? restoreLabel(preview) : undefined);

/** isDeleted reports whether a preview is known to be gone: its namespace's previews were listed and it isn't among them. */
export const isDeleted = (namespace: string, name: string) => {
  const previews = store.collection(`previews/${namespace}`);
  return previews?.synced === true && !previews.error && findPreview(namespace, name) === undefined;
};
