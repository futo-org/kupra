// The end-to-end tests of the dashboard, which TestDashboard in test/e2e runs against the kind cluster's dashboard.
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: 'e2e',
  // CI keeps it, with the traces and screenshots of failed tests.
  outputDir: '../.e2e/playwright',
  reporter: 'list',
  timeout: 5 * 60_000,
  expect: { timeout: 30_000 },
  use: {
    baseURL: process.env.KUPRA_DASHBOARD_URL ?? 'http://localhost:8090',
    // Dex's certificate is from a CA that e2e:up generates.
    ignoreHTTPSErrors: true,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
});
