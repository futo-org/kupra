// Fake data for the dashboard's mockup under /mock. Nothing here talks to a backend.

import { DateTime, type DurationLike } from 'luxon';
import type { Preview, PreviewApp, PreviewSnapshot } from '#lib/kube/kupra.gen.ts';
import type { Condition } from '#lib/kube/meta.ts';
import { createdByAnnotation } from '#lib/kube/preview.ts';

/** Role is the kupra ClusterRole the signed-in user holds in a namespace. */
export type Role = 'viewer' | 'creator' | 'admin';

/** sourceAnnotation is where the mockup assumes a preview records where it came from. kupra has no such field yet. */
export const sourceAnnotation = 'kupra.futo.tech/source';

/** channelLabel is the conventional label for a snapshot's channel. */
export const channelLabel = 'kupra.futo.tech/channel';

const ago = (duration: DurationLike) => DateTime.now().minus(duration).toUTC().toISO();

const condition = (
  type: string,
  status: Condition['status'],
  reason: string,
  message: string,
  since: DurationLike,
): Condition => ({ type, status, reason, message, lastTransitionTime: ago(since) });

const ready = (since: DurationLike) => [
  condition('Ready', 'True', 'Succeeded', 'preview is ready', since),
  condition('Reconciling', 'False', 'Succeeded', '', since),
];

const inventory = [
  { apiGroup: 'postgresql.cnpg.io', kind: 'Cluster', name: 'database', state: true },
  { kind: 'PersistentVolumeClaim', name: 'library', state: true },
  { apiGroup: 'apps', kind: 'Deployment', name: 'immich-server' },
  { apiGroup: 'apps', kind: 'Deployment', name: 'immich-machine-learning' },
  { apiGroup: 'apps', kind: 'Deployment', name: 'valkey' },
  { kind: 'Service', name: 'immich-server' },
  { kind: 'Service', name: 'valkey' },
  { apiGroup: 'gateway.networking.k8s.io', kind: 'HTTPRoute', name: 'immich' },
];

const immichLinks = (host: string) => [
  { name: 'Web', url: `https://${host}.immich.preview.futo.dev` },
  { name: 'API docs', url: `https://${host}.immich.preview.futo.dev/doc` },
];

/** user is the signed-in user. */
export const user = { username: 'oidc:alice', groups: ['oidc:immich-developers', 'github:alicedev'] };

/** imageTags are the tags the imageTag picker offers, by repository. */
export const imageTags: Record<string, string[]> = {
  'ghcr.io/immich-app/immich-server': ['release', 'main', 'v2.3.1', 'v2.3.0', 'v2.2.3', 'pr-24311', 'pr-24350'],
  'ghcr.io/futo-org/grayjay-web': ['main', 'v12', 'v11'],
};

const apps: PreviewApp[] = [
  {
    metadata: { name: 'immich', namespace: 'immich-previews', creationTimestamp: ago({ days: 41 }) },
    spec: {
      parameters: [
        {
          name: 'tag',
          title: 'Server image',
          description: 'Tag of ghcr.io/immich-app/immich-server',
          required: true,
          default: 'main',
          uiHint: { type: 'imageTag', imageTag: { repository: 'ghcr.io/immich-app/immich-server' } },
        },
        {
          name: 'logLevel',
          title: 'Log level',
          enum: ['verbose', 'debug', 'log', 'warn', 'error'],
          default: 'log',
        },
        {
          name: 'machineLearning',
          title: 'Machine learning',
          description: 'Runs immich-machine-learning for smart search and faces',
          type: 'boolean',
          default: 'true',
        },
        { name: 'workers', title: 'Server workers', type: 'integer', default: '1' },
      ],
      links: [
        { name: 'Web', urlExpr: "'https://' + preview.name + '.immich.preview.futo.dev'" },
        { name: 'API docs', urlExpr: "'https://' + preview.name + '.immich.preview.futo.dev/doc'" },
      ],
      template: { resources: [] },
    },
    status: { conditions: [condition('Ready', 'True', 'Succeeded', 'app is valid', { days: 2 })] },
  },
  {
    metadata: { name: 'immich-demo', namespace: 'immich-previews', creationTimestamp: ago({ days: 30 }) },
    spec: {
      parameters: [{ name: 'tag', title: 'Server image', default: 'release' }],
      links: [{ name: 'Demo', urlExpr: "'https://demo.immich.app'" }],
      template: { resources: [] },
    },
    status: { conditions: [condition('Ready', 'True', 'Succeeded', 'app is valid', { days: 30 })] },
  },
  {
    metadata: { name: 'grayjay-web', namespace: 'futo-previews', creationTimestamp: ago({ days: 12 }) },
    spec: {
      parameters: [
        {
          name: 'tag',
          title: 'Image',
          required: true,
          uiHint: { type: 'imageTag', imageTag: { repository: 'ghcr.io/futo-org/grayjay-web' } },
        },
      ],
      links: [{ name: 'Web', urlExpr: "'https://' + preview.name + '.grayjay.preview.futo.dev'" }],
      template: { resources: [] },
    },
    status: { conditions: [condition('Ready', 'True', 'Succeeded', 'app is valid', { days: 12 })] },
  },
];

const previews: Preview[] = [
  {
    metadata: {
      name: 'pr-24311',
      namespace: 'immich-previews',
      creationTimestamp: ago({ hours: 26 }),
      labels: {
        'resourceset.fluxcd.controlplane.io/name': 'immich-pull-requests',
        'resourceset.fluxcd.controlplane.io/namespace': 'flux-system',
      },
      annotations: { [sourceAnnotation]: 'https://github.com/immich-app/immich/pull/24311' },
    },
    spec: {
      appRef: { name: 'immich' },
      parameters: { tag: 'pr-24311' },
      owners: [{ kind: 'Group', name: 'github:alicedev' }],
      restoreFrom: { snapshotSelector: { matchLabels: { [channelLabel]: 'nightly' } } },
    },
    status: {
      phase: 'Running',
      namespace: 'immich-previews-pr-24311-7f3a',
      restoredFrom: { name: 'nightly-2026-11-06' },
      links: immichLinks('pr-24311'),
      inventory,
      conditions: ready({ hours: 25 }),
    },
  },
  {
    metadata: {
      name: 'search-fix',
      namespace: 'immich-previews',
      creationTimestamp: ago({ days: 3, hours: 4 }),
      annotations: { [createdByAnnotation]: 'oidc:alice' },
    },
    spec: {
      appRef: { name: 'immich' },
      parameters: { tag: 'main', logLevel: 'debug' },
      restoreFrom: { snapshotSelector: { matchLabels: { [channelLabel]: 'nightly' } } },
    },
    status: {
      phase: 'Running',
      namespace: 'immich-previews-search-fix-c41d',
      restoredFrom: { name: 'nightly-2026-11-04' },
      links: immichLinks('search-fix'),
      inventory,
      conditions: ready({ days: 3 }),
    },
  },
  {
    metadata: {
      name: 'ml-upgrade',
      namespace: 'immich-previews',
      creationTimestamp: ago({ hours: 2 }),
      annotations: { [createdByAnnotation]: 'oidc:alice' },
    },
    spec: {
      appRef: { name: 'immich' },
      parameters: { tag: 'v2.3.1', machineLearning: 'true', workers: '2' },
      owners: [{ kind: 'User', name: 'oidc:bob' }],
      restoreFrom: { snapshotRef: { name: 'bug-24298' } },
    },
    status: {
      phase: 'Failed',
      namespace: 'immich-previews-ml-upgrade-0b9e',
      restoredFrom: { name: 'bug-24298' },
      links: immichLinks('ml-upgrade'),
      inventory,
      conditions: [
        condition(
          'Ready',
          'False',
          'ProgressDeadlineExceeded',
          'Deployment/immich-machine-learning: 0 of 1 replicas available after 30m',
          { hours: 1, minutes: 30 },
        ),
        condition('Stalled', 'True', 'ProgressDeadlineExceeded', 'timed out waiting for resources', {
          hours: 1,
          minutes: 30,
        }),
        condition('Reconciling', 'False', 'ProgressDeadlineExceeded', '', { hours: 1, minutes: 30 }),
      ],
    },
  },
  {
    metadata: {
      name: 'pr-24350',
      namespace: 'immich-previews',
      creationTimestamp: ago({ minutes: 3 }),
      labels: {
        'resourceset.fluxcd.controlplane.io/name': 'immich-pull-requests',
        'resourceset.fluxcd.controlplane.io/namespace': 'flux-system',
      },
      annotations: { [sourceAnnotation]: 'https://github.com/immich-app/immich/pull/24350' },
    },
    spec: {
      appRef: { name: 'immich' },
      parameters: { tag: 'pr-24350' },
      owners: [{ kind: 'Group', name: 'github:bobdev' }],
      restoreFrom: { snapshotSelector: { matchLabels: { [channelLabel]: 'nightly' } } },
    },
    status: {
      phase: 'Provisioning',
      namespace: 'immich-previews-pr-24350-e8a1',
      restoredFrom: { name: 'nightly-2026-11-07' },
      inventory: inventory.slice(0, 2),
      conditions: [
        condition('Ready', 'False', 'Progressing', 'restoring Cluster/database from nightly-2026-11-07', {
          minutes: 3,
        }),
        condition('Reconciling', 'True', 'Progressing', 'restoring state', { minutes: 3 }),
      ],
    },
  },
  {
    metadata: {
      name: 'main',
      namespace: 'immich-previews',
      creationTimestamp: ago({ days: 40 }),
      labels: {
        'kustomize.toolkit.fluxcd.io/name': 'immich-previews',
        'kustomize.toolkit.fluxcd.io/namespace': 'flux-system',
      },
      annotations: { [sourceAnnotation]: 'https://github.com/immich-app/immich/tree/main' },
    },
    spec: {
      appRef: { name: 'immich' },
      parameters: { tag: 'main' },
      owners: [{ kind: 'Group', name: 'oidc:immich-developers' }],
    },
    status: {
      phase: 'Running',
      namespace: 'immich-previews-main-1c2d',
      links: immichLinks('main'),
      inventory,
      conditions: ready({ hours: 9 }),
    },
  },
  {
    metadata: {
      name: 'bob-albums',
      namespace: 'immich-previews',
      creationTimestamp: ago({ days: 6 }),
      annotations: { [createdByAnnotation]: 'oidc:bob' },
    },
    spec: {
      appRef: { name: 'immich' },
      parameters: { tag: 'main' },
      restoreFrom: { snapshotSelector: { matchLabels: { [channelLabel]: 'nightly' } } },
    },
    status: {
      phase: 'Running',
      namespace: 'immich-previews-bob-albums-77aa',
      restoredFrom: { name: 'nightly-2026-11-01' },
      links: immichLinks('bob-albums'),
      inventory,
      conditions: ready({ days: 6 }),
    },
  },
  {
    metadata: {
      name: 'demo',
      namespace: 'immich-previews',
      creationTimestamp: ago({ days: 30 }),
      labels: {
        'kustomize.toolkit.fluxcd.io/name': 'immich-previews',
        'kustomize.toolkit.fluxcd.io/namespace': 'flux-system',
      },
    },
    spec: {
      appRef: { name: 'immich-demo' },
      parameters: { tag: 'release' },
      restoreFrom: { snapshotRef: { name: 'nightly-2026-10-31' } },
    },
    status: {
      phase: 'Running',
      namespace: 'immich-previews-demo-5e5e',
      restoredFrom: { name: 'nightly-2026-10-31' },
      links: [{ name: 'Demo', url: 'https://demo.immich.app' }],
      inventory,
      conditions: ready({ days: 1 }),
    },
  },
  {
    metadata: {
      name: 'v12-beta',
      namespace: 'futo-previews',
      creationTimestamp: ago({ days: 9 }),
      annotations: { [createdByAnnotation]: 'oidc:carol' },
    },
    spec: { appRef: { name: 'grayjay-web' }, parameters: { tag: 'v12' } },
    status: {
      phase: 'Running',
      namespace: 'futo-previews-v12-beta-a0f1',
      links: [{ name: 'Web', url: 'https://v12-beta.grayjay.preview.futo.dev' }],
      inventory: [
        { apiGroup: 'apps', kind: 'Deployment', name: 'grayjay-web' },
        { kind: 'Service', name: 'grayjay-web' },
      ],
      conditions: ready({ days: 9 }),
    },
  },
];

const nightly = (date: string, age: DurationLike, phase: 'Succeeded' | 'Deleting' = 'Succeeded'): PreviewSnapshot => ({
  metadata: {
    name: `nightly-${date}`,
    namespace: 'immich-previews',
    creationTimestamp: ago(age),
    labels: { [channelLabel]: 'nightly' },
    ...(phase === 'Deleting' && { deletionTimestamp: ago({ hours: 7 }) }),
  },
  spec: { source: { previewRef: { name: 'main' } }, mode: 'Offline' },
  status: {
    phase,
    readyToUse: true,
    appRef: { name: 'immich' },
    parameters: { tag: 'main' },
    conditions: [
      phase === 'Deleting'
        ? condition('Ready', 'True', 'InUse', 'in use by Preview demo', { hours: 7 })
        : condition('Ready', 'True', 'Succeeded', 'snapshot is ready to use', age),
    ],
  },
});

const snapshots: PreviewSnapshot[] = [
  nightly('2026-11-07', { hours: 9 }),
  nightly('2026-11-06', { days: 1, hours: 9 }),
  nightly('2026-11-05', { days: 2, hours: 9 }),
  nightly('2026-11-04', { days: 3, hours: 9 }),
  nightly('2026-11-01', { days: 6, hours: 9 }),
  nightly('2026-10-31', { days: 7, hours: 9 }, 'Deleting'),
  {
    metadata: {
      name: 'bug-24298',
      namespace: 'immich-previews',
      creationTimestamp: ago({ days: 2 }),
      labels: { [channelLabel]: 'manual' },
      annotations: { [createdByAnnotation]: 'oidc:alice' },
    },
    spec: { source: { previewRef: { name: 'search-fix' } }, mode: 'Offline' },
    status: {
      phase: 'Succeeded',
      readyToUse: true,
      appRef: { name: 'immich' },
      parameters: { tag: 'main', logLevel: 'debug' },
      conditions: [condition('Ready', 'True', 'Succeeded', 'snapshot is ready to use', { days: 2 })],
    },
  },
  {
    metadata: {
      name: 'pr-24311-before-migration',
      namespace: 'immich-previews',
      creationTimestamp: ago({ minutes: 1 }),
      labels: { [channelLabel]: 'manual' },
      annotations: { [createdByAnnotation]: 'oidc:alice' },
    },
    spec: { source: { previewRef: { name: 'pr-24311' } }, mode: 'Offline' },
    status: {
      phase: 'InProgress',
      appRef: { name: 'immich' },
      parameters: { tag: 'pr-24311' },
      conditions: [
        condition('Ready', 'False', 'Progressing', 'capturing PersistentVolumeClaim/library', { minutes: 1 }),
      ],
    },
  },
  {
    metadata: {
      name: 'faces-broken',
      namespace: 'immich-previews',
      creationTimestamp: ago({ days: 4 }),
      labels: { [channelLabel]: 'manual' },
      annotations: { [createdByAnnotation]: 'oidc:bob' },
    },
    spec: { source: { previewRef: { name: 'bob-albums' } }, mode: 'Online' },
    status: {
      phase: 'Failed',
      appRef: { name: 'immich' },
      conditions: [
        condition(
          'Ready',
          'False',
          'CaptureFailed',
          'VolumeSnapshot/library-4kq2x: failed to take snapshot: rpc error: code = ResourceExhausted',
          { days: 4 },
        ),
      ],
    },
  },
];

/** seed is the mockup's starting data. */
export const seed = { apps, previews, snapshots };

/** roles are the ClusterRoles the user starts with, by namespace. */
export const roles: Record<string, Role> = { 'immich-previews': 'creator', 'futo-previews': 'viewer' };

/** now returns the current time as the API server would write it. */
export const now = () => DateTime.now().toUTC().toISO();
