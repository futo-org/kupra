import { api } from '#lib/api/index.ts';
import { listWatchClient, type Query } from './client.ts';
import type { Kinds } from './kupra.gen.ts';
import type { Kind } from './paths.ts';
import { initialState, listWatch, type ListWatchState } from './watch.ts';

/**
 * watchList keeps the collection that query selects up to date, for as long as the calling component lives. When the
 * query changes, the collection is listed again. Without a query, nothing is watched.
 */
export const watchList = <K extends Kind>(query: () => Query<K> | undefined) => {
  let state = $state.raw<ListWatchState<Kinds[K]>>(initialState());
  $effect(() => {
    const current = query();
    state = initialState();
    if (!current) {
      return;
    }
    return listWatch(listWatchClient(api, current), (next) => (state = next));
  });
  return {
    get current() {
      return state;
    },
  };
};

/** watchLists keeps a collection up to date in each of the namespaces, for as long as the calling component lives. */
export const watchLists = <K extends Kind>(kind: K, namespaces: () => string[]) => {
  let states = $state.raw<Record<string, ListWatchState<Kinds[K]>>>({});
  $effect(() => {
    const list = namespaces();
    states = Object.fromEntries(list.map((namespace) => [namespace, initialState()]));
    const stops = list.map((namespace) =>
      listWatch(listWatchClient(api, { kind, namespace }), (next) => (states = { ...states, [namespace]: next })),
    );
    return () => {
      for (const stop of stops) {
        stop();
      }
    };
  });
  return {
    get current() {
      return states;
    },
  };
};
