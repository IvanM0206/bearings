/* Bearings service worker. Generated at build time from build/sw-template.js. */
/* eslint-disable */
const VERSION = __BEARINGS_VERSION__;
const ASSETS = __BEARINGS_ASSETS__;
const CACHE = `bearings-${VERSION}`;

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((cache) => cache.addAll(ASSETS.map((a) => new Request(a, { cache: 'reload' }))))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith('bearings-') && k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

const scope = new URL(self.registration.scope);
const precached = new Set(ASSETS.map((a) => new URL(a, scope).href));

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  // Only the app's own origin; everything else goes to the network untouched.
  if (url.origin !== scope.origin) return;
  if (req.mode === 'navigate') {
    // Single-page app: every navigation is served by the cached shell.
    event.respondWith(
      fetch(req).catch(() => caches.match(new URL('./', scope).href).then((r) => r || Response.error())),
    );
    return;
  }
  url.search = '';
  url.hash = '';
  if (!precached.has(url.href)) return;
  event.respondWith(caches.match(url.href).then((r) => r || fetch(req)));
});
