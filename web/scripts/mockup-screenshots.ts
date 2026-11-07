// Takes screenshots of the dashboard's mockup, in light and dark mode, from the built app.
// Run `pnpm run build` first, then `node scripts/mockup-screenshots.ts`.

import { chromium, type Page } from 'playwright';
import { readFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '../../cmd/dashboard/kodata');
const output = path.resolve(import.meta.dirname, '../mockup-screenshots');

const types: Record<string, string> = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.svg': 'image/svg+xml',
};

// The app is a single page, so paths without a file get index.html.
const server = createServer((request, response) => {
  const pathname = decodeURIComponent(new URL(request.url ?? '/', 'http://localhost').pathname);
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

type View = { name: string; path: string; act?: (page: Page) => Promise<void>; mobile?: boolean };

const app = '/mock/apps/immich-previews/immich';
const views: View[] = [
  { name: 'home', path: '/mock' },
  { name: 'home-mobile', path: '/mock', mobile: true },
  { name: 'app-previews', path: app },
  {
    name: 'app-new-preview',
    path: app,
    act: (page) => page.getByRole('button', { name: 'New preview' }).click(),
  },
  { name: 'app-snapshots', path: `${app}/snapshots` },
  { name: 'app-viewer', path: '/mock/apps/futo-previews/grayjay-web' },
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
    const page = await context.newPage();
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
server.close();
