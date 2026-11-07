import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import { groupApps } from './apps.ts';
import type { PreviewApp } from './kupra.gen.ts';
import { initialState, type ListWatchState } from './watch.ts';

const app = (namespace: string, name: string): PreviewApp => ({
  metadata: { namespace, name },
  spec: { template: { resources: [] } },
});

const loaded = (...items: PreviewApp[]): ListWatchState<PreviewApp> => ({ items, loaded: true });

describe(groupApps.name, () => {
  it('groups apps by namespace, sorted', () => {
    const result = groupApps({ b: loaded(app('b', 'x')), a: loaded(app('a', 'y')) });
    expect(result.loaded).toBe(true);
    expect(result.groups.map(({ namespace }) => namespace)).toEqual(['a', 'b']);
  });

  it('leaves out namespaces the user may not see, and those without apps', () => {
    const forbidden = { ...initialState<PreviewApp>(), error: new ApiError(403, 'forbidden') };
    const result = groupApps({ a: forbidden, b: loaded(), c: loaded(app('c', 'x')) });
    expect(result).toEqual({ loaded: true, groups: [{ namespace: 'c', apps: [app('c', 'x')], error: undefined }] });
  });

  it('keeps namespaces that failed for other reasons', () => {
    const error = new ApiError(500, 'internal error');
    const result = groupApps({ a: { ...initialState<PreviewApp>(), error } });
    expect(result.groups).toEqual([{ namespace: 'a', apps: [], error }]);
  });

  it("isn't loaded until every namespace is", () => {
    expect(groupApps({ a: loaded(app('a', 'x')), b: initialState() }).loaded).toBe(false);
  });

  it('is loaded and empty without namespaces', () => {
    expect(groupApps({})).toEqual({ groups: [], loaded: true });
  });
});
