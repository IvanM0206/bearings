import { fileURLToPath } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';
import { serviceWorkerPlugin } from './build/sw-plugin';
import { templatesPlugin } from './build/templates-plugin';

const here = (p: string): string => fileURLToPath(new URL(p, import.meta.url));

export default defineConfig({
  base: '/',
  plugins: [
    react(),
    templatesPlugin(here('../templates')),
    serviceWorkerPlugin(here('./build/sw-template.js')),
  ],
  server: {
    fs: { allow: [here('.'), here('../templates')] },
  },
  build: {
    target: 'es2023',
    sourcemap: false,
    // Never inline assets as data: URIs; the CSP only allows 'self'.
    assetsInlineLimit: 0,
    // The polyfill is injected as inline code, which the CSP forbids.
    modulePreload: { polyfill: false },
    cssCodeSplit: false,
  },
  test: {
    globals: true,
    projects: [
      {
        extends: true,
        test: {
          name: 'core',
          environment: 'node',
          testTimeout: 30_000,
          include: ['src/core/**/*.test.ts', 'build/**/*.test.ts'],
        },
      },
      {
        extends: true,
        test: {
          name: 'ui',
          environment: 'jsdom',
          include: ['src/**/*.test.tsx', 'src/storage/**/*.test.ts', 'src/ui/**/*.test.ts'],
          setupFiles: ['./src/test/setup.ts'],
        },
      },
    ],
  },
});
