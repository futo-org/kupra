import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import type { List, ObjectMeta } from './meta.ts';
import { poll, pollValue, type PollState } from './poll.ts';
import type { ListWatchState } from './watch.ts';

type Item = { metadata: ObjectMeta };

const item = (name: string): Item => ({ metadata: { namespace: 'ns', name } });

// run polls with lists answered from steps, and returns the states it reported once the steps run out.
const run = async (steps: (List<Item> | Error)[]) => {
  const states: ListWatchState<Item>[] = [];
  let lists = 0;
  let wake = () => {};
  const done = new Promise<void>((resolve) => (wake = resolve));
  const stop = poll<Item>(
    async () => {
      const step = steps[lists++];
      if (step === undefined) {
        // Once the steps run out, the list never answers.
        return new Promise<List<Item>>(() => {});
      }
      if (step instanceof Error) {
        throw step;
      }
      return step;
    },
    (state) => {
      states.push(state);
      if (states.length === steps.length) {
        wake();
      }
    },
    { sleep: () => Promise.resolve() },
  );
  await done;
  stop();
  return { states, lists };
};

describe('poll', () => {
  it('lists again after each interval, sorted', async () => {
    const { states } = await run([
      { metadata: {}, items: [item('b'), item('a')] },
      { metadata: {}, items: [item('a')] },
    ]);
    expect(states.map((s) => s.items.map((i) => i.metadata.name))).toEqual([['a', 'b'], ['a']]);
    expect(states.every((s) => s.loaded)).toBe(true);
  });

  it('keeps the last items through an error, and clears it on the next list', async () => {
    const { states } = await run([
      { metadata: {}, items: [item('a')] },
      new ApiError(500, 'unavailable'),
      { metadata: {}, items: [item('a')] },
    ]);
    expect(states[1]?.items).toHaveLength(1);
    expect(states[1]?.error?.message).toBe('unavailable');
    expect(states[2]?.error).toBeUndefined();
  });

  it('stops at 403 Forbidden', async () => {
    const states: ListWatchState<Item>[] = [];
    let lists = 0;
    poll<Item>(
      () => {
        lists++;
        return Promise.reject(new ApiError(403, 'forbidden'));
      },
      (state) => {
        states.push(state);
      },
      { sleep: () => Promise.resolve() },
    );
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(lists).toBe(1);
    expect(states).toHaveLength(1);
  });
});

describe('pollValue', () => {
  it('keeps the last value through an error, and clears it once loading works again', async () => {
    const states: PollState<string[]>[] = [];
    const steps: (string[] | Error)[] = [new ApiError(502, 'down'), ['a'], new ApiError(502, 'down'), ['a', 'b']];
    let loads = 0;
    let wake = () => {};
    const done = new Promise<void>((resolve) => (wake = resolve));
    const stop = pollValue<string[]>(
      () => {
        const step = steps[loads++];
        if (step === undefined) {
          return new Promise<string[]>(() => {});
        }
        return step instanceof Error ? Promise.reject(step) : Promise.resolve(step);
      },
      (state) => {
        states.push(state);
        if (states.length === steps.length) {
          wake();
        }
      },
      { sleep: () => Promise.resolve() },
    );
    await done;
    stop();
    expect(states).toEqual([
      { loaded: false, error: steps[0] },
      { value: ['a'], loaded: true },
      { value: ['a'], loaded: true, error: steps[2] },
      { value: ['a', 'b'], loaded: true },
    ]);
  });
});
