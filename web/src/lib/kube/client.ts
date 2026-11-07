import type { Client } from '#lib/api/client.ts';
import type { Kinds } from './kupra.gen.ts';
import type { List } from './meta.ts';
import { listPath, watchPath, type Kind, type ListOptions } from './paths.ts';
import type { ListWatchClient } from './watch.ts';

/** Query selects a collection of one of kupra's kinds. */
export type Query<K extends Kind> = ListOptions & {
  kind: K;
  namespace?: string;
};

/** listWatchClient lists and watches the collection that query selects. */
export const listWatchClient = <K extends Kind>(client: Client, query: Query<K>): ListWatchClient<Kinds[K]> => ({
  list: (signal) => client.get<List<Kinds[K]>>(listPath(query.kind, query.namespace, query), signal),
  watch: (resourceVersion, signal) =>
    client.watch<Kinds[K]>(watchPath(query.kind, query.namespace, resourceVersion, query), signal),
});
