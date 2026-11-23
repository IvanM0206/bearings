import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// The build output is embedded in the Go binary and served with a strict
// Content-Security-Policy (no inline scripts or styles), so everything must
// be an external file.
export default defineConfig({
  base: '/',
  plugins: [react()],
  server: {
    // `npm run dev` proxies the API to a running `bearings serve`; set
    // BEARINGS_DEV_API to its address (e.g. http://127.0.0.1:7777).
    proxy: process.env['BEARINGS_DEV_API'] ? { '/api': process.env['BEARINGS_DEV_API'] } : undefined,
  },
  build: {
    target: 'es2023',
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    // Never inline assets as data: URIs.
    assetsInlineLimit: 0,
    // The polyfill is injected as inline code, which the CSP forbids.
    modulePreload: { polyfill: false },
    cssCodeSplit: false,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
});
