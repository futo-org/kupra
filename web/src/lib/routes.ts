import { resolve } from '$app/paths';

/** Route builds the dashboard's paths. */
export const Route = {
  home: () => resolve('/(app)'),
  app: (namespace: string, app: string) => resolve('/(app)/apps/[namespace]/[app]', { namespace, app }),
  snapshots: (namespace: string, app: string) => resolve('/(app)/apps/[namespace]/[app]/snapshots', { namespace, app }),
  preview: (namespace: string, app: string, preview: string) =>
    resolve('/(app)/apps/[namespace]/[app]/previews/[preview]', { namespace, app, preview }),
};
