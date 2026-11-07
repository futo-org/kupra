// What the mockup works out from its fake data. Once wired up, permissions come from the API server instead.

import type { Preview, PreviewApp, PreviewSnapshot } from '#lib/kube/kupra.gen.ts';
import { createdByAnnotation } from '#lib/kube/preview.ts';
import { channelLabel, sourceAnnotation, user } from './data.ts';
import { access, list } from './store.svelte.ts';

/** AppGroup is a namespace's apps. */
export type AppGroup = { namespace: string; apps: PreviewApp[] };

/** appGroups returns the apps the user can see, by namespace. */
export const appGroups = (): AppGroup[] =>
  list('app-namespaces')
    .map(({ metadata }) => metadata?.name ?? '')
    .toSorted((a, b) => a.localeCompare(b))
    .map((namespace) => ({ namespace, apps: list('previewapps', namespace) }));

const namespaces = () => list('app-namespaces').map(({ metadata }) => metadata?.name ?? '');

/** findApp returns the app with this name in this namespace. */
export const findApp = (namespace: string, name: string) =>
  list('previewapps', namespace).find((app) => app.metadata?.name === name);

/** findPreview returns the preview with this name in this namespace. */
export const findPreview = (namespace: string, name: string) =>
  list('previews', namespace).find((preview) => preview.metadata?.name === name);

/** previewsOf returns an app's previews, newest first. */
export const previewsOf = (namespace: string, app: string) =>
  newestFirst(list('previews', namespace).filter((preview) => preview.spec.appRef.name === app));

/** snapshotsIn returns a namespace's snapshots, newest first. */
export const snapshotsIn = (namespace: string) => newestFirst(list('previewsnapshots', namespace));

const newestFirst = <T extends Preview | PreviewSnapshot>(objects: T[]) =>
  objects.toSorted((a, b) => (b.metadata?.creationTimestamp ?? '').localeCompare(a.metadata?.creationTimestamp ?? ''));

/** isOwner reports whether the user created or owns a preview. */
export const isOwner = (preview: Preview) =>
  preview.metadata?.annotations?.[createdByAnnotation] === user.username ||
  (preview.spec.owners ?? []).some(
    ({ kind, name }) => (kind === 'User' && name === user.username) || (kind === 'Group' && user.groups.includes(name)),
  );

/** myPreviews returns the previews the user created or owns, across apps, newest first. */
export const myPreviews = () =>
  newestFirst(
    namespaces()
      .flatMap((namespace) => list('previews', namespace))
      .filter(isOwner),
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

/** channel returns a snapshot's channel. */
export const channel = (snapshot: PreviewSnapshot) => snapshot.metadata?.labels?.[channelLabel];

/** channels returns the channels of a namespace's snapshots. */
export const channels = (namespace: string) => [
  ...new Set(snapshotsIn(namespace).flatMap((snapshot) => channel(snapshot) ?? [])),
];

/** restoreLabel says what a preview's state starts from, as it's set in its spec. */
export const restoreLabel = (preview: Preview) => {
  const restoreFrom = preview.spec.restoreFrom;
  if (restoreFrom?.snapshotRef) {
    return restoreFrom.snapshotRef.name;
  }
  const selected = restoreFrom?.snapshotSelector?.matchLabels?.[channelLabel];
  if (selected) {
    return `Newest in ${selected}`;
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

/** Permissions are what the user may do with a preview. */
export type Permissions = { edit: boolean; delete: boolean; reset: boolean; snapshot: boolean };

/** permissions returns what the user may do with a preview, following ADR 6's roles and ownership. */
export const permissions = (preview: Preview): Permissions => {
  const role = access.roles[preview.metadata?.namespace ?? ''];
  const manages = role === 'admin' || isOwner(preview);
  const managed = managedBy(preview) !== undefined;
  return {
    edit: manages && !managed,
    delete: manages && !managed,
    reset: manages,
    snapshot: manages && (role === 'admin' || role === 'creator'),
  };
};

/** canCreate reports whether the user may create previews in a namespace. */
export const canCreate = (namespace: string) => ['creator', 'admin'].includes(access.roles[namespace] ?? '');

/** canDeleteSnapshots reports whether the user may delete snapshots in a namespace. */
export const canDeleteSnapshots = (namespace: string) => access.roles[namespace] === 'admin';
