# Bearings web interface

React + TypeScript single-page app served by `bearings serve`. Data comes from the local JSON API
(`/api/v1`, same origin, session cookie); nothing is stored in the browser except the chosen language
(`localStorage`, key `bearings.language`).

## Develop

```sh
cd web
npm ci
npm run typecheck && npm run lint && npm test
BEARINGS_DEV_API=http://127.0.0.1:<port> npm run dev   # proxies /api to a running `bearings serve`
```

Open the `/auth?token=...` URL printed by `bearings serve` once so the browser gets the session cookie
(use the same origin as the dev server for the cookie to be sent, or test through the Go server).

## Build into the Go binary

The build output is committed in `cli/internal/server/webdist/` so that `go build` needs Go only.
After changing anything in `web/`, regenerate it (needs Node 22):

```sh
cd cli && go generate ./internal/server/...
```

This runs `npm ci --ignore-scripts` and `npm run build` and copies `web/dist` to `webdist/`. File names carry a
content hash, so the result is reproducible; CI can check it with `go generate` then `git diff --exit-code`.

## Security rules

- CSP forbids inline scripts and styles: no `<style>`, no `style=""` in HTML, no CSS-in-JS. Only bar widths use
  React `style` props (set through the CSSOM).
- Markdown is parsed with raw HTML disabled and rendered to React elements (never to an HTML string); only
  http, https and mailto links are clickable; images are never loaded.
- Fonts (Nunito, Fredoka, SIL OFL) are bundled; licences are in `public/licenses/`.
- All user-facing text lives in `src/i18n/en.ts` and `es.ts` (a test checks that keys and placeholders match).
