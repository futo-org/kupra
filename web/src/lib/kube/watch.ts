import { ApiError, isGone } from '#lib/api/client.ts';
import type { List, ObjectMeta, WatchEvent } from './meta.ts';

type KubeObject = { metadata?: ObjectMeta };

/** ListWatchClient lists a collection and watches it for changes. */
export type ListWatchClient<T> = {
  list(signal: AbortSignal): Promise<List<T>>;
  /** watch resolves once the watch has started. */
  watch(resourceVersion: string, signal: AbortSignal): Promise<AsyncIterable<WatchEvent<T>>>;
};

/** ListWatchState is what's known about a collection. */
export type ListWatchState<T> = {
  /** items are the collection's objects, sorted by namespace and name. */
  items: T[];
  /** loaded is set once the collection has been listed. */
  loaded: boolean;
  /** error is the last error, until the next successful list or watch event. */
  error?: Error;
};

export type ListWatchOptions = {
  /** delay returns how long to wait before the given retry. */
  delay?: (attempt: number) => number;
  sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
};

/** initialState is the state of a collection that hasn't been listed yet. */
export const initialState = <T>(): ListWatchState<T> => ({ items: [], loaded: false });

/** backoff doubles from one second up to thirty. */
export const backoff = (attempt: number) => Math.min(1000 * 2 ** attempt, 30_000);

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

/** isFinal reports whether an error won't go away by retrying, such as a 403 Forbidden. */
const isFinal = (error: unknown) => error instanceof ApiError && [400, 403, 404].includes(error.status);

const key = ({ metadata }: KubeObject) => `${metadata?.namespace ?? ''}/${metadata?.name ?? ''}`;

/** sorted returns the objects sorted by namespace and name. */
export const sorted = <T extends KubeObject>(objects: Iterable<T>) =>
  [...objects].toSorted((a, b) => key(a).localeCompare(key(b)));

/** applyEvent applies a watch event to objects keyed by namespace and name, and returns whether they changed. */
export const applyEvent = <T extends KubeObject>(objects: Map<string, T>, event: WatchEvent<T>) => {
  switch (event.type) {
    case 'ADDED':
    case 'MODIFIED': {
      objects.set(key(event.object), event.object);
      return true;
    }
    case 'DELETED': {
      return objects.delete(key(event.object));
    }
    case 'BOOKMARK': {
      return false;
    }
    case 'ERROR': {
      const { code = 500, message = 'watch failed', reason } = event.object;
      throw new ApiError(code, message, reason);
    }
  }
};

/**
 * listWatch lists a collection, then watches it, calling onChange with each new state until stop is called. A watch
 * that ends is resumed from the last resource version. When that version is too old, the collection is listed again.
 * Errors are retried with a backoff, except those that retrying won't fix, such as 403 Forbidden.
 */
export const listWatch = <T extends KubeObject>(
  client: ListWatchClient<T>,
  onChange: (state: ListWatchState<T>) => void,
  { delay = backoff, sleep = abortableSleep }: ListWatchOptions = {},
): (() => void) => {
  const controller = new AbortController();
  const { signal } = controller;
  const objects = new Map<string, T>();
  let loaded = false;
  let error: Error | undefined;
  let attempt = 0;

  const emit = () => {
    if (!signal.aborted) {
      onChange({ items: sorted(objects.values()), loaded, error });
    }
  };

  // recovered clears the last error. The backoff is only reset by progress, since a watch can start and then fail.
  const recovered = () => {
    if (error) {
      error = undefined;
      emit();
    }
  };

  // fail reports error and returns whether to try again.
  const fail = async (cause: unknown) => {
    if (signal.aborted) {
      return false;
    }
    error = cause instanceof Error ? cause : new Error(String(cause));
    emit();
    if (isFinal(cause)) {
      return false;
    }
    await sleep(delay(attempt++), signal);
    return !signal.aborted;
  };

  // watch watches from resourceVersion until the version is too old, and returns whether to list again.
  const watch = async (resourceVersion: string) => {
    while (!signal.aborted) {
      let received = false;
      try {
        const events = await client.watch(resourceVersion, signal);
        recovered();
        for await (const event of events) {
          const changed = applyEvent(objects, event);
          resourceVersion = event.object.metadata?.resourceVersion ?? resourceVersion;
          if (changed) {
            emit();
          }
          received = true;
          attempt = 0;
        }
        if (!received) {
          // A watch that ends without any events is retried with a backoff, so it can't loop.
          await sleep(delay(attempt++), signal);
        }
      } catch (cause) {
        if (isGone(cause)) {
          return true;
        }
        if (!(await fail(cause))) {
          return false;
        }
      }
    }
    return false;
  };

  const run = async () => {
    while (!signal.aborted) {
      let resourceVersion: string;
      try {
        const list = await client.list(signal);
        objects.clear();
        for (const item of list.items) {
          objects.set(key(item), item);
        }
        resourceVersion = list.metadata.resourceVersion ?? '';
        loaded = true;
        attempt = 0;
        error = undefined;
        emit();
      } catch (cause) {
        if (await fail(cause)) {
          continue;
        }
        return;
      }
      if (!(await watch(resourceVersion))) {
        return;
      }
    }
  };

  void run();
  return () => controller.abort();
};
