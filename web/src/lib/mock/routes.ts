import { resolve } from '$app/paths';

/** Route builds the mockup's paths. */
export const Route = {
  home: () => resolve('/mock'),
  app: (namespace: string, app: string) => resolve('/mock/apps/[namespace]/[app]', { namespace, app }),
  snapshots: (namespace: string, app: string) => resolve('/mock/apps/[namespace]/[app]/snapshots', { namespace, app }),
  preview: (namespace: string, app: string, preview: string) =>
    resolve('/mock/apps/[namespace]/[app]/previews/[preview]', { namespace, app, preview }),
};
