import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { maxCollections } from './collections.ts';
import type { Preview } from './kube/types.ts';
import { Store } from './watch.svelte.ts';

const preview = (name: string): Preview => ({
  apiVersion: 'kupra.futo.tech/v1alpha1',
  kind: 'Preview',
  metadata: { name, namespace: 'ns' },
  spec: { appRef: { name: 'app' } },
});

/** FakeEventSource stands in for the browser's EventSource, which tests drive by hand. */
class FakeEventSource extends EventTarget {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  static sources: FakeEventSource[] = [];
  readyState = FakeEventSource.CONNECTING;
  readonly url: string;

  constructor(url: string) {
    super();
    this.url = url;
    FakeEventSource.sources.push(this);
  }

  open() {
    this.readyState = FakeEventSource.OPEN;
    this.dispatchEvent(new Event('open'));
  }

  fail(readyState: number) {
    this.readyState = readyState;
    this.dispatchEvent(new Event('error'));
  }

  close() {
    this.readyState = FakeEventSource.CLOSED;
  }
}

beforeEach(() => {
  vi.useFakeTimers();
  FakeEventSource.sources = [];
  vi.stubGlobal('EventSource', FakeEventSource);
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response('{}'))),
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('Store.apply', () => {
  it('drops a collection that can no longer be watched', () => {
    const store = new Store();
    store.apply({ collection: 'previews/ns', type: 'ADDED', object: preview('a') });
    store.apply({ collection: 'previews/ns', type: 'SYNCED' });
    const error = { code: 403, message: 'previews is forbidden' };
    store.apply({ collection: 'previews/ns', type: 'ERROR', error });

    expect(store.list('previews', 'ns')).toEqual([]);
    expect(store.collection('previews/ns')).toMatchObject({ synced: false, error });
  });
});

describe('Store.connect', () => {
  it('is disconnected while EventSource reconnects by itself, until it opens again', () => {
    const store = new Store();
    const stop = store.connect(['previews/ns']);
    vi.advanceTimersByTime(50);
    const [source] = FakeEventSource.sources;
    source?.open();
    expect(store.disconnected).toBe(false);

    source?.fail(FakeEventSource.CONNECTING);
    expect(store.disconnected).toBe(true);

    source?.open();
    expect(store.disconnected).toBe(false);
    stop();
  });

  it('stays disconnected while one stream is down, whatever the others do', () => {
    const store = new Store();
    const names = Array.from({ length: maxCollections + 1 }, (_, i) => `previews/ns-${i}`);
    const stop = store.connect(names);
    vi.advanceTimersByTime(50);
    const [first, second] = FakeEventSource.sources;
    expect(FakeEventSource.sources).toHaveLength(2);

    first?.fail(FakeEventSource.CONNECTING);
    second?.open();
    expect(store.disconnected).toBe(true);

    first?.open();
    expect(store.disconnected).toBe(false);

    first?.fail(FakeEventSource.CONNECTING);
    stop();
    expect(store.disconnected).toBe(false);
  });
});
