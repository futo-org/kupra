import { isForbidden } from '#lib/api/client.ts';
import type { PreviewApp } from './kupra.gen.ts';
import type { ListWatchState } from './watch.ts';

/** AppGroup is the PreviewApps in one namespace. */
export type AppGroup = {
  namespace: string;
  apps: PreviewApp[];
  error?: Error;
};

/** AppGroups is what the dashboard knows about the apps the user can see. */
export type AppGroups = {
  groups: AppGroup[];
  /** loaded is set once every namespace has been listed, or has failed to. */
  loaded: boolean;
};

/**
 * groupApps groups the PreviewApps the user can see by namespace. Namespaces the user may not list PreviewApps in, and
 * those without any, are left out.
 */
export const groupApps = (states: Record<string, ListWatchState<PreviewApp>>): AppGroups => {
  const entries = Object.entries(states).toSorted(([a], [b]) => a.localeCompare(b));
  const groups = entries
    .filter(([, { items, error }]) => !isForbidden(error) && (items.length > 0 || error))
    .map(([namespace, { items, error }]) => ({ namespace, apps: items, error }));
  return { groups, loaded: entries.every(([, { loaded, error }]) => loaded || error) };
};
