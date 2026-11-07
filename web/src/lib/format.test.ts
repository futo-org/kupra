import { describe, expect, it } from 'vitest';
import { age } from './format.ts';

const now = Date.parse('2026-11-07T12:00:00Z');
const ago = (seconds: number) => new Date(now - seconds * 1000).toISOString();

describe(age.name, () => {
  it.each([
    [30, '30s'],
    [119, '119s'],
    [150, '2m30s'],
    [600, '10m'],
    [2 * 3600 + 60, '121m'],
    [3 * 3600 + 15 * 60, '3h15m'],
    [20 * 3600, '20h'],
    [3 * 86_400 + 2 * 3600, '3d2h'],
    [30 * 86_400, '30d'],
    [3 * 365 * 86_400 + 86_400, '3y1d'],
  ])('formats %ss as %s', (seconds, expected) => {
    expect(age(ago(seconds), now)).toBe(expected);
  });

  it('is empty without a timestamp', () => {
    expect(age(undefined, now)).toBe('');
  });
});
