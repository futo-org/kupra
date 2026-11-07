// Signs in to the dashboard that `mise run dashboard:dev` serves, and screenshots its pages in light and dark mode:
// `pnpm run screenshots <dir>`. It prints the page errors it saw.
import { chromium } from 'playwright';

const base = process.env.KUPRA_DASHBOARD_URL ?? 'http://localhost:8080';
const output = process.argv[2] ?? 'screenshots';
const pages = [
  ['home', '/'],
  ['app', '/apps/immich-previews/immich'],
  ['snapshots', '/apps/immich-previews/immich/snapshots'],
  ['preview-running', '/apps/immich-previews/immich/previews/pr-24311'],
  ['preview-failed', '/apps/immich-previews/immich/previews/ml-upgrade'],
];

const browser = await chromium.launch();
const errors: string[] = [];
for (const scheme of ['light', 'dark'] as const) {
  // The dev server's identity provider has a self-signed certificate.
  const context = await browser.newContext({
    ignoreHTTPSErrors: true,
    colorScheme: scheme,
    viewport: { width: 1440, height: 900 },
  });
  const page = await context.newPage();
  page.on('pageerror', (error) => {
    errors.push(`${scheme} ${page.url()}: ${error.message}`);
  });
  for (const [name, path] of pages) {
    await page.goto(base + path);
    // The page streams its data, so the network never goes idle; give it a moment instead.
    await page.waitForTimeout(1500);
    await page.screenshot({ path: `${output}/${name}-${scheme}.png`, fullPage: true });
  }
  await page.goto(base + '/apps/immich-previews/immich');
  await page.getByRole('button', { name: /new preview/i }).click();
  await page.screenshot({ path: `${output}/new-preview-${scheme}.png` });
  await context.close();
}
await browser.close();
console.log(errors.length > 0 ? errors.join('\n') : 'no page errors');
