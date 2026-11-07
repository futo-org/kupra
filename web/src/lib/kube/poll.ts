import { isForbidden } from '#lib/api/client.ts';
import type { List } from './meta.ts';
import { sorted, type ListWatchState } from './watch.ts';

type KubeObject = { metadata?: { namespace?: string; name?: string } };

export type PollOptions = {
  /** interval is how long to wait between lists. */
  interval?: number;
  sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
};

const abortableSleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve) => {
    const timer = setTimeout(done, ms);
    signal.addEventListener('abort', done, { once: true });
    function done() {
      clearTimeout(timer);
      signal.removeEventListener('abort', done);
      resolve();
    }
  });

/**
 * poll lists a collection every interval, calling onChange with each new state until stop is called. Unlike a watch, it
 * holds no connection open between lists, so many collections can be kept up to date at once. A 403 Forbidden stops it.
 */
export const poll = <T extends KubeObject>(
  list: (signal: AbortSignal) => Promise<List<T>>,
  onChange: (state: ListWatchState<T>) => void,
  { interval = 30_000, sleep = abortableSleep }: PollOptions = {},
): (() => void) => {
  const controller = new AbortController();
  const { signal } = controller;
  let state: ListWatchState<T> = { items: [], loaded: false };

  const run = async () => {
    while (!signal.aborted) {
      try {
        const result = await list(signal);
        state = { items: sorted(result.items), loaded: true };
      } catch (cause) {
        if (signal.aborted) {
          return;
        }
        state = { ...state, error: cause instanceof Error ? cause : new Error(String(cause)) };
      }
      onChange(state);
      if (isForbidden(state.error)) {
        return;
      }
      await sleep(interval, signal);
    }
  };

  void run();
  return () => controller.abort();
};
