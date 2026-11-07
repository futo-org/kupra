// @vitest-environment jsdom
// Effects only run in Svelte's client code, which a browser-like environment loads.
import { flushSync, tick } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Clock } from './clock.svelte.ts';

beforeEach(() => {
  vi.useFakeTimers({ now: 0 });
});

afterEach(() => {
  vi.useRealTimers();
});

it('moves on while read in an effect, and stops its timer once nothing reads it', async () => {
  const clock = new Clock(1000);
  const seen: number[] = [];
  const stop = $effect.root(() => {
    $effect(() => {
      seen.push(clock.now);
    });
  });
  flushSync();
  for (let i = 0; i < 2; i++) {
    vi.advanceTimersByTime(1000);
    flushSync();
  }
  expect(seen).toEqual([0, 1000, 2000]);

  stop();
  // Svelte lets go of the subscription once the effect's teardown settles.
  await tick();
  expect(vi.getTimerCount()).toBe(0);
});
