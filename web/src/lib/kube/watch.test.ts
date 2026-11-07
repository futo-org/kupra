import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import type { List, ObjectMeta, WatchEvent } from './meta.ts';
import { applyEvent, backoff, listWatch, type ListWatchClient, type ListWatchState } from './watch.ts';

type Item = { metadata: ObjectMeta; value?: number };

const item = (name: string, resourceVersion: string, value?: number): Item => ({
  metadata: { namespace: 'ns', name, resourceVersion },
  value,
});

const list = (resourceVersion: string, ...items: Item[]): List<Item> => ({ metadata: { resourceVersion }, items });

// hang is a watch that stays open until it's aborted.
const hang = (signal: AbortSignal): AsyncIterable<WatchEvent<Item>> => ({
  [Symbol.asyncIterator]: () => ({
    next: () =>
      new Promise((resolve) =>
        signal.addEventListener('abort', () => resolve({ done: true, value: undefined }), { once: true }),
      ),
  }),
});

const events = (...values: WatchEvent<Item>[]): AsyncIterable<WatchEvent<Item>> => ({
  async *[Symbol.asyncIterator]() {
    for (const value of values) {
      await Promise.resolve();
      yield value;
    }
  },
});

type Step<T> = T | Error | ((signal: AbortSignal) => T | Promise<T>);

// fakeClient answers lists and watches from scripts, and then hangs.
const fakeClient = (lists: Step<List<Item>>[], watches: Step<AsyncIterable<WatchEvent<Item>>>[]) => {
  const calls: string[] = [];
  const answer = <T>(step: Step<T> | undefined, signal: AbortSignal, fallback: () => Promise<T>) => {
    if (step === undefined) {
      return fallback();
    }
    if (step instanceof Error) {
      return Promise.reject(step);
    }
    return Promise.resolve(
      typeof step === 'function' ? (step as (signal: AbortSignal) => T | Promise<T>)(signal) : step,
    );
  };
  const client: ListWatchClient<Item> = {
    list: (signal) => {
      calls.push('list');
      return answer(lists.shift(), signal, () => new Promise(() => {}));
    },
    watch: (resourceVersion, signal) => {
      calls.push(`watch ${resourceVersion}`);
      return answer(watches.shift(), signal, () => Promise.resolve(hang(signal)));
    },
  };
  return { client, calls };
};

const run = (client: ListWatchClient<Item>) => {
  const states: ListWatchState<Item>[] = [];
  const delays: number[] = [];
  const stop = listWatch(client, (state) => void states.push(state), {
    delay: (attempt) => {
      delays.push(attempt);
      return 0;
    },
    sleep: () => Promise.resolve(),
  });
  const last = () => states.at(-1);
  const names = () => last()?.items.map(({ metadata }) => metadata.name);
  return { states, delays, stop, last, names };
};

describe(applyEvent.name, () => {
  it('adds, modifies and deletes objects', () => {
    const objects = new Map<string, Item>();
    expect(applyEvent(objects, { type: 'ADDED', object: item('a', '1', 1) })).toBe(true);
    expect(applyEvent(objects, { type: 'MODIFIED', object: item('a', '2', 2) })).toBe(true);
    expect(objects.values().toArray()).toEqual([item('a', '2', 2)]);
    expect(applyEvent(objects, { type: 'DELETED', object: item('a', '3') })).toBe(true);
    expect(objects.size).toBe(0);
  });

  it("ignores bookmarks and deletions of objects it doesn't have", () => {
    const objects = new Map<string, Item>();
    expect(applyEvent(objects, { type: 'BOOKMARK', object: item('', '5') })).toBe(false);
    expect(applyEvent(objects, { type: 'DELETED', object: item('a', '5') })).toBe(false);
  });

  it('throws error events as ApiErrors', () => {
    const event: WatchEvent<Item> = {
      type: 'ERROR',
      object: { kind: 'Status', code: 410, reason: 'Expired', message: 'too old resource version' },
    };
    expect(() => applyEvent(new Map(), event)).toThrow(new ApiError(410, 'too old resource version', 'Expired'));
  });
});

describe(backoff.name, () => {
  it('doubles up to thirty seconds', () => {
    expect([0, 1, 2, 5, 10].map((attempt) => backoff(attempt))).toEqual([1000, 2000, 4000, 30_000, 30_000]);
  });
});

describe(listWatch.name, () => {
  it('lists, then applies watch events from the list resource version', async () => {
    const { client, calls } = fakeClient(
      [list('10', item('b', '9'), item('a', '8'))],
      [
        events(
          { type: 'ADDED', object: item('c', '11') },
          { type: 'BOOKMARK', object: item('', '12') },
          { type: 'DELETED', object: item('a', '13') },
        ),
      ],
    );
    const { states, names, stop } = run(client);
    await vi.waitFor(() => expect(names()).toEqual(['b', 'c']));
    expect(states[0]).toMatchObject({ loaded: true, error: undefined });
    expect(names()).toEqual(['b', 'c']);
    // The watch ended, so it's resumed from the last event's resource version.
    await vi.waitFor(() => expect(calls).toEqual(['list', 'watch 10', 'watch 13']));
    stop();
  });

  it('sorts objects by namespace and name', async () => {
    const { client } = fakeClient([list('1', item('b', '1'), item('c', '1'), item('a', '1'))], []);
    const { names, stop } = run(client);
    await vi.waitFor(() => expect(names()).toEqual(['a', 'b', 'c']));
    stop();
  });

  it('lists again when the resource version is too old', async () => {
    const { client, calls } = fakeClient(
      [list('10', item('a', '10')), list('20', item('b', '20'))],
      [
        events({ type: 'ERROR', object: { kind: 'Status', code: 410, reason: 'Expired', message: 'too old' } }),
        new ApiError(410, 'gone'),
      ],
    );
    const { names, last, stop } = run(client);
    await vi.waitFor(() => expect(calls).toEqual(['list', 'watch 10', 'list', 'watch 20', 'list']));
    expect(names()).toEqual(['b']);
    expect(last()?.error).toBeUndefined();
    stop();
  });

  it('stops at 403 Forbidden', async () => {
    const forbidden = new ApiError(403, 'previews.kupra.futo.tech is forbidden', 'Forbidden');
    const { client, calls } = fakeClient([forbidden], []);
    const { states, delays } = run(client);
    await vi.waitFor(() => expect(states).toEqual([{ items: [], loaded: false, error: forbidden }]));
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(calls).toEqual(['list']);
    expect(delays).toEqual([]);
  });

  it('retries a failed list with a backoff', async () => {
    const failure = new ApiError(503, 'unavailable');
    const { client } = fakeClient([failure, failure, list('1', item('a', '1'))], []);
    const { states, delays, stop } = run(client);
    await vi.waitFor(() => expect(states.at(-1)).toMatchObject({ loaded: true, error: undefined }));
    expect(states.slice(0, 2)).toEqual([
      { items: [], loaded: false, error: failure },
      { items: [], loaded: false, error: failure },
    ]);
    expect(delays).toEqual([0, 1]);
    stop();
  });

  it('keeps the objects while a watch fails, and clears the error once it resumes', async () => {
    const failure = new TypeError('network error');
    const { client, calls } = fakeClient([list('5', item('a', '5'))], [failure]);
    const { states, stop } = run(client);
    await vi.waitFor(() => expect(calls).toEqual(['list', 'watch 5', 'watch 5']));
    await vi.waitFor(() => expect(states.at(-1)?.error).toBeUndefined());
    expect(states.map(({ items, error }) => [items.length, error])).toEqual([
      [1, undefined],
      [1, failure],
      [1, undefined],
    ]);
    stop();
  });

  it('backs off a watch that ends without events', async () => {
    const { client, calls } = fakeClient([list('5')], [events(), events(), events()]);
    const { delays, stop } = run(client);
    await vi.waitFor(() => expect(calls).toEqual(['list', 'watch 5', 'watch 5', 'watch 5', 'watch 5']));
    expect(delays).toEqual([0, 1, 2]);
    stop();
  });

  it('backs off watches that start and then fail, until one makes progress', async () => {
    const failing = (): AsyncIterable<WatchEvent<Item>> => ({
      [Symbol.asyncIterator]: () => ({ next: () => Promise.reject(new TypeError('network error')) }),
    });
    const { client, calls } = fakeClient(
      [list('5')],
      [failing(), failing(), events({ type: 'ADDED', object: item('a', '6') }), failing()],
    );
    const { delays, stop } = run(client);
    await vi.waitFor(() => expect(calls).toHaveLength(6));
    expect(delays).toEqual([0, 1, 0]);
    stop();
  });

  it('stops calling onChange once stopped', async () => {
    let resolveList: (value: List<Item>) => void = () => {};
    const { client } = fakeClient([() => new Promise<List<Item>>((resolve) => (resolveList = resolve))], []);
    const { states, stop } = run(client);
    stop();
    resolveList(list('1', item('a', '1')));
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(states).toEqual([]);
  });
});
