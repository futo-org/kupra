import { describe, expect, it } from 'vitest';
import { chunk, maxCollections } from './collections.ts';

describe(chunk.name, () => {
  it('keeps collections within the limit of one stream', () => {
    const names = Array.from({ length: 51 }, (_, i) => `previews/ns-${i}`);
    const groups = chunk(names, maxCollections);
    expect(groups.map((group) => group.length)).toEqual([50, 1]);
    expect(groups.flat()).toEqual(names);
  });

  it('makes no groups of nothing', () => {
    expect(chunk([], maxCollections)).toEqual([]);
  });
});
