// Takes screenshots of the dashboard, in light and dark mode, from the built app with a fake backend.
// Run `pnpm run build` first, then `pnpm run mockup:screenshots`.

import { chromium, type Page, type Route } from 'playwright';
import { readFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import path from 'node:path';
import type { ResourceAttributes, SelfSubjectAccessReview } from '#lib/kube/types.ts';
import type { WatchMessage } from '#lib/watch.svelte.ts';
import { allowed, fixtures, user } from './fixtures.ts';

const root = path.resolve(import.meta.dirname, '../../../cmd/dashboard/kodata');
const output = path.resolve(import.meta.dirname, '../../mockup-screenshots');

const types: Record<string, string> = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.svg': 'image/svg+xml',
};

// The app is a single page, so paths without a file get index.html. /api/watch streams the fixtures and stays open,
// like the backend's.
const server = createServer((request, response) => {
  const url = new URL(request.url ?? '/', 'http://localhost');
  if (url.pathname === '/api/watch') {
    response.writeHead(200, { 'content-type': 'text/event-stream' }).write(watch(url));
    return;
  }
  const pathname = decodeURIComponent(url.pathname);
  const file = path.join(root, path.normalize(pathname));
  readFile(file)
    .catch(() => readFile(path.join(root, 'index.html')))
    .then((body) => {
      const type = types[path.extname(file)] ?? 'text/html';
      response.writeHead(200, { 'content-type': type }).end(body);
    })
    .catch(() => response.writeHead(404).end());
});
await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;

const namespaces = [...new Set(fixtures.previewapps.map((app) => app.metadata?.namespace ?? ''))];

/** watch answers GET /api/watch with each collection's objects, then SYNCED, as the backend streams them. */
const watch = (url: URL) => {
  const messages: WatchMessage[] = url.searchParams.getAll('c').flatMap((collection): WatchMessage[] => {
    const [resource = '', namespace] = collection.split('/', 2);
    const objects =
      resource === 'app-namespaces'
        ? namespaces.map((name) => ({ metadata: { name } }))
        : (fixtures[resource as keyof typeof fixtures] ?? []).filter(
            (object) => object.metadata?.namespace === namespace,
          );
    return [
      ...objects.map((object) => ({ collection, type: 'ADDED' as const, object })),
      { collection, type: 'SYNCED' },
    ];
  });
  return messages.map((message) => `data: ${JSON.stringify(message)}\n\n`).join('');
};

const api = async (route: Route) => {
  const request = route.request();
  const url = new URL(request.url());
  if (url.pathname === '/api/session') {
    return route.fulfill({ json: user });
  }
  if (url.pathname.endsWith('/selfsubjectaccessreviews')) {
    const review = request.postDataJSON() as SelfSubjectAccessReview;
    const attributes: ResourceAttributes = review.spec.resourceAttributes ?? {};
    return route.fulfill({ json: { ...review, status: { allowed: allowed(attributes) } } });
  }
  return route.fulfill({ json: request.postDataJSON() as unknown });
};

type View = { name: string; path: string; act?: (page: Page) => Promise<void>; mobile?: boolean };

const app = '/apps/immich-previews/immich';
const views: View[] = [
  { name: 'home', path: '/' },
  { name: 'home-mobile', path: '/', mobile: true },
  { name: 'app-previews', path: app },
  {
    name: 'app-new-preview',
    path: app,
    act: (page) => page.getByRole('button', { name: 'New preview' }).click(),
  },
  { name: 'app-snapshots', path: `${app}/snapshots` },
  { name: 'app-viewer', path: '/apps/futo-previews/grayjay-web' },
  { name: 'preview-running', path: `${app}/previews/search-fix` },
  {
    name: 'preview-running-menu',
    path: `${app}/previews/search-fix`,
    act: (page) => page.getByRole('button', { name: 'Actions' }).click(),
  },
  { name: 'preview-failed', path: `${app}/previews/ml-upgrade` },
  { name: 'preview-pending', path: `${app}/previews/pr-24350` },
  {
    name: 'preview-managed-menu',
    path: `${app}/previews/pr-24311`,
    act: (page) => page.getByRole('button', { name: 'Actions' }).click(),
  },
];

const browser = await chromium.launch();
for (const theme of ['light', 'dark']) {
  for (const view of views) {
    const width = view.mobile ? 390 : 1440;
    const context = await browser.newContext({
      viewport: { width, height: 900 },
      deviceScaleFactor: view.mobile ? 2 : 1,
      colorScheme: theme === 'dark' ? 'dark' : 'light',
    });
    await context.addInitScript((theme) => localStorage.setItem('immich-ui-theme', JSON.stringify(theme)), theme);
    await context.route((url) => url.pathname.startsWith('/api/') && url.pathname !== '/api/watch', api);
    const page = await context.newPage();
    page.on('pageerror', (error) => console.error(`${view.name}: ${error.stack}`));
    page.on('console', (message) => message.type() === 'error' && console.error(`${view.name}: ${message.text()}`));
    await page.goto(base + view.path);
    await page.waitForLoadState('networkidle');
    // The app shell scrolls inside the page, so the page is made as tall as its content to capture all of it.
    const height = await page.evaluate(() =>
      Math.max(
        ...[...document.querySelectorAll('div')].map(
          (element) => element.getBoundingClientRect().top + element.scrollHeight,
        ),
      ),
    );
    await page.setViewportSize({ width, height: Math.max(view.mobile ? 844 : 900, height) });
    await view.act?.(page);
    await page.waitForTimeout(500);
    await page.screenshot({ path: path.join(output, `${view.name}-${theme}.png`) });
    await context.close();
    console.log(`${view.name}-${theme}.png`);
  }
}
await browser.close();
server.closeAllConnections();
server.close();
