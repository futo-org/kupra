import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import type { ProxyOptions } from 'vite';
import { defineConfig } from 'vitest/config';

const output = '../cmd/dashboard/kodata';

// KUPRA_DASHBOARD_URL is the dashboard backend that the dev server passes /api and /auth to.
const backend: ProxyOptions = {
  target: process.env.KUPRA_DASHBOARD_URL || 'http://localhost:8080',
  changeOrigin: true,
};

export default defineConfig({
  server: {
    proxy: { '/api': backend, '/auth': backend },
  },
  preview: {
    proxy: { '/api': backend, '/auth': backend },
  },
  plugins: [
    tailwindcss(),
    sveltekit({
      compilerOptions: {
        runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true),
      },
      // ko ships kodata in the dashboard's image, where the backend serves it.
      adapter: adapter({ pages: output, assets: output, fallback: 'index.html' }),
      // The backend serves the page without a CSP of its own, apart from frame-ancestors.
      csp: {
        mode: 'hash',
        directives: {
          'default-src': ['self'],
          'script-src': ['self'],
          // Svelte and @immich/ui set inline styles.
          'style-src': ['self', 'unsafe-inline'],
          'img-src': ['self', 'data:'],
          'connect-src': ['self'],
          'object-src': ['none'],
          'base-uri': ['self'],
          'form-action': ['self'],
        },
      },
    }),
  ],
  test: {
    include: ['src/**/*.test.ts'],
  },
});
