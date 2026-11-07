import { isForbidden } from '#lib/api/client.ts';
import type { List } from './meta.ts';
import { sorted, type ListWatchState } from './watch.ts';

type KubeObject = { metadata?: { namespace?: string; name?: string } };

export type PollOptions = {
  /** interval is how long to wait between loads. */
  interval?: number;
  sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
};

/** PollState is what's known about a polled value. */
export type PollState<T> = {
  /** value is the last value loaded. */
  value?: T;
  /** loaded is set once the value has been loaded. */
  loaded: boolean;
  /** error is the last error, until the next successful load. */
  error?: Error;
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
 * pollValue loads a value every interval, calling onChange with each new state until stop is called. An error keeps the
 * last value. A 403 Forbidden stops it, as loading again won't help.
 */
export const pollValue = <T>(
  load: (signal: AbortSignal) => Promise<T>,
  onChange: (state: PollState<T>) => void,
  { interval = 30_000, sleep = abortableSleep }: PollOptions = {},
): (() => void) => {
  const controller = new AbortController();
  const { signal } = controller;
  let state: PollState<T> = { loaded: false };

  const run = async () => {
    while (!signal.aborted) {
      try {
        state = { value: await load(signal), loaded: true };
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

/**
 * poll lists a collection every interval, calling onChange with each new state until stop is called. Unlike a watch, it
 * holds no connection open between lists, so many collections can be kept up to date at once.
 */
export const poll = <T extends KubeObject>(
  list: (signal: AbortSignal) => Promise<List<T>>,
  onChange: (state: ListWatchState<T>) => void,
  options: PollOptions = {},
): (() => void) =>
  pollValue(
    list,
    ({ value, loaded, error }) => onChange({ items: value ? sorted(value.items) : [], loaded, error }),
    options,
  );
