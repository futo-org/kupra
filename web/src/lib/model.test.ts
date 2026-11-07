import { describe, expect, it } from 'vitest';
import type { Preview } from './kube/types.ts';
import { appGroups, isDeleted, previewOf, resetSource } from './model.ts';
import { store } from './watch.svelte.ts';

const preview = (name: string, app: string, restoreFrom?: Preview['spec']['restoreFrom']): Preview => ({
  apiVersion: 'kupra.futo.tech/v1alpha1',
  kind: 'Preview',
  metadata: { name, namespace: 'ns' },
  spec: { appRef: { name: app }, restoreFrom },
});

store.apply({ collection: 'previews/ns', type: 'ADDED', object: preview('pr-1', 'immich') });
store.apply({ collection: 'previews/ns', type: 'SYNCED' });

describe(previewOf.name, () => {
  it("finds an app's preview", () => {
    expect(previewOf('ns', 'immich', 'pr-1')?.metadata?.name).toBe('pr-1');
  });

  it("doesn't find another app's preview of the same name", () => {
    expect(previewOf('ns', 'immich-demo', 'pr-1')).toBeUndefined();
  });
});

describe(resetSource.name, () => {
  it('says what a reset restores from, by reference or by selector', () => {
    expect(resetSource(preview('a', 'app', { snapshotRef: { name: 'nightly' } }))).toBe('nightly');
    expect(resetSource(preview('a', 'app', { snapshotSelector: { matchLabels: { base: 'true' } } }))).toBe(
      'Newest matching snapshot',
    );
    expect(resetSource(preview('a', 'app'))).toBeUndefined();
  });
});

describe(isDeleted.name, () => {
  it('knows a preview is gone once its namespace is listed without it', () => {
    expect(isDeleted('ns', 'gone')).toBe(true);
    expect(isDeleted('ns', 'pr-1')).toBe(false);
  });

  it("can't tell when the previews can't be listed", () => {
    store.apply({ collection: 'previews/forbidden', type: 'ERROR', error: { code: 403, message: 'forbidden' } });
    expect(isDeleted('forbidden', 'gone')).toBe(false);
    expect(isDeleted('unwatched', 'gone')).toBe(false);
  });
});

describe(appGroups.name, () => {
  it('leaves out namespaces with no apps the user can see', () => {
    for (const name of ['apps', 'forbidden-apps']) {
      store.apply({ collection: 'app-namespaces', type: 'ADDED', object: { metadata: { name } } });
    }
    store.apply({ collection: 'app-namespaces', type: 'SYNCED' });
    store.apply({
      collection: 'previewapps/apps',
      type: 'ADDED',
      object: {
        apiVersion: 'kupra.futo.tech/v1alpha1',
        kind: 'PreviewApp',
        metadata: { name: 'immich', namespace: 'apps' },
        spec: { template: { resources: [] } },
      },
    });
    store.apply({ collection: 'previewapps/apps', type: 'SYNCED' });
    store.apply({
      collection: 'previewapps/forbidden-apps',
      type: 'ERROR',
      error: { code: 403, message: 'forbidden' },
    });

    expect(appGroups().map(({ namespace, apps }) => [namespace, apps.map((app) => app.metadata?.name)])).toEqual([
      ['apps', ['immich']],
    ]);
  });
});
