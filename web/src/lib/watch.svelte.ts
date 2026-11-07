// Live data: the page's collections, streamed from the backend's GET /api/watch over one EventSource.

import { untrack } from 'svelte';
import { request } from './api.ts';
import type { ObjectMeta, Preview, PreviewApp, PreviewSnapshot, Status } from './kube/types.ts';

/** Objects are the objects in each kind of collection. Collections are named like previews/<namespace>. */
export type Objects = {
  previews: Preview;
  previewsnapshots: PreviewSnapshot;
  previewapps: PreviewApp;
  'app-namespaces': { metadata?: ObjectMeta };
};

type KubeObject = Objects[keyof Objects];

/** WatchMessage is a message on the watch stream. */
export type WatchMessage = {
  collection: string;
  type: 'ADDED' | 'MODIFIED' | 'DELETED' | 'SYNCED' | 'ERROR';
  object?: KubeObject;
  error?: Status;
};

/** Collection is a collection's objects, by namespace/name, whether it has been listed, and why it can't be watched. */
export type Collection<T> = { synced: boolean; error?: Status; objects: Record<string, T> };

const key = ({ metadata }: KubeObject) => `${metadata?.namespace ?? ''}/${metadata?.name ?? ''}`;

const backoff = (attempt: number) => Math.min(1000 * 2 ** attempt, 30_000);

/** Store holds the collections that the page's components ask for, and keeps them up to date. */
class Store {
  #collections = $state<Record<string, Collection<KubeObject>>>({});
  #wanted = $state<Record<string, number>>({});
  /** disconnected is set while the stream is down and being retried. */
  disconnected = $state(false);
  // Keys seen in each collection since the stream (re)connected. Listing again removes those that weren't.
  #seen = new Map<string, Set<string>>();

  /** collection returns a collection, or undefined until it's watched. */
  collection = (name: string) => this.#collections[name];

  /** list returns the objects in a collection. */
  list = <R extends keyof Objects>(resource: R, namespace?: string) =>
    Object.values(this.#collections[namespace ? `${resource}/${namespace}` : resource]?.objects ?? {}) as Objects[R][];

  /** wanted returns the collections that components ask for, sorted. */
  wanted = () => Object.keys(this.#wanted).toSorted((a, b) => a.localeCompare(b));

  /** want asks for collections until the calling component is destroyed. */
  want = (names: () => string[]) => {
    $effect(() => {
      const current = [...new Set(names())];
      // Counting reads what it writes, which mustn't make the effect depend on it.
      untrack(() => {
        for (const name of current) {
          this.#wanted[name] = (this.#wanted[name] ?? 0) + 1;
        }
      });
      return () => {
        for (const name of current) {
          const count = (this.#wanted[name] ?? 1) - 1;
          if (count > 0) {
            this.#wanted[name] = count;
          } else {
            delete this.#wanted[name];
          }
        }
      };
    });
  };

  /** apply applies a message from the stream. */
  apply = ({ collection: name, type, object, error }: WatchMessage) => {
    const collection = (this.#collections[name] ??= { synced: false, objects: {} });
    switch (type) {
      case 'ADDED':
      case 'MODIFIED': {
        if (object) {
          collection.objects[key(object)] = object;
          this.#seen.get(name)?.add(key(object));
        }
        break;
      }
      case 'DELETED': {
        if (object) {
          delete collection.objects[key(object)];
        }
        break;
      }
      case 'SYNCED': {
        const seen = this.#seen.get(name);
        for (const stale of Object.keys(collection.objects)) {
          if (seen && !seen.has(stale)) {
            delete collection.objects[stale];
          }
        }
        this.#seen.delete(name);
        collection.synced = true;
        collection.error = undefined;
        break;
      }
      case 'ERROR': {
        collection.error = error;
        break;
      }
    }
  };

  /** connect streams these collections until the returned function is called. */
  connect = (names: string[]) => {
    let source: EventSource | undefined;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let attempt = 0;
    let stopped = false;

    const open = () => {
      const query = new URLSearchParams(names.map((name) => ['c', name]));
      source = new EventSource(`/api/watch?${query}`);
      source.addEventListener('open', () => {
        attempt = 0;
        this.disconnected = false;
        // Objects deleted while the stream was down are dropped once each collection is listed again.
        for (const name of names) {
          this.#seen.set(name, new Set());
        }
      });
      source.addEventListener('message', (event: MessageEvent<string>) => {
        this.apply(JSON.parse(event.data) as WatchMessage);
      });
      source.addEventListener('error', () => {
        // EventSource reconnects by itself when the stream ends, but gives up on an error status, such as a 401.
        if (source?.readyState !== EventSource.CLOSED || stopped) {
          return;
        }
        this.disconnected = true;
        // Asking for the session sends the browser to sign in if that's why it failed.
        request('/api/session')
          .catch(() => {})
          .finally(() => {
            if (!stopped) {
              retry = setTimeout(open, backoff(attempt++));
            }
          });
      });
    };
    // Collections are often asked for a few at a time, such as an app's once its namespace is known, so opening waits a moment.
    retry = setTimeout(open, 50);

    return () => {
      stopped = true;
      clearTimeout(retry);
      source?.close();
    };
  };
}

/** store is the page's live data. */
export const store = new Store();
