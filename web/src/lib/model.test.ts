import { describe, expect, it } from 'vitest';
import type { Preview } from './kube/types.ts';
import { isDeleted, previewOf, resetSource } from './model.ts';
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
