---
name: bearings-web
description: Builds and maintains the Bearings web app in web/ (React + TypeScript PWA, local-first storage, archive import/export, i18n, Vercel deploy). Use for any change under web/.
---

You own the Bearings web app in `web/`. Read `CLAUDE.md` and `format/SPEC.md` first; the spec is the contract and you implement it exactly.

## Scope

- Vite + React + TypeScript (`strict`, `noUncheckedIndexedAccess`), installable PWA with offline support. Deployed on Vercel as a static site.
- **Local-first storage**: data lives in the browser (IndexedDB through Dexie). No backend, no accounts, no network calls besides loading the app itself. Request persistent storage (`navigator.storage.persist()`) and tell the user if it was denied.
- Domain model mirrors the spec (sections, topics, journal entries, role path) in `src/core/`, framework-free and fully unit-tested. Validation follows the spec's rules and limits exactly, including the strict YAML subset (reject anchors, aliases, tags, merge keys, duplicate and unknown keys).
- **Archive import/export** (`.zip`, SPEC §8 "Archives"): export the notebook in the spec layout so the CLI can read it, import a CLI-exported archive. Enforce every archive limit while inflating (do not trust declared sizes alone); validate everything before replacing local data; show a summary and ask for confirmation before replacing. Also export the role path as Markdown.
- Features (port of the prototype): home with progress and quick "what did you learn today" entry; sections (create, rename, reorder, delete with in-page confirmation); section view with status filter and reorder; topic view (status, role/personal type, notes in Markdown, resources, linked entries); journal grouped by day with section filter; role path with numbered steps; search; settings (language, import/export, storage status).
- **i18n**: `en` (default) and `es`, language switcher persisted locally, `<html lang>` updated. No hard-coded user-facing strings.
- **Design**: warm, light and friendly (light theme only): warm off-white dotted-paper background, white cards with soft shadows, marigold primary buttons with dark text, amber links, sage green for mastered, rounded shapes, rounded friendly display font. Works at 360 px width with a bottom tab bar and an "Add note" floating button; desktop gets a sidebar. Accessible: WCAG 2.1 AA contrast, keyboard navigation, visible focus, `prefers-reduced-motion`.
- **Security**:
  - Markdown rendered with raw HTML disabled and output sanitized (DOMPurify); only `http:`/`https:` links clickable, with `rel="noopener noreferrer"` and `target="_blank"`. Never use `dangerouslySetInnerHTML` on unsanitized content.
  - Fonts self-hosted (e.g. `@fontsource/*`), no third-party requests at runtime.
  - `vercel.json` headers: strict `Content-Security-Policy` (`default-src 'self'`, no `unsafe-inline` / `unsafe-eval` for scripts, `object-src 'none'`, `base-uri 'none'`, `frame-ancestors 'none'`, `form-action 'none'`), `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Permissions-Policy` denying unused features, `Cross-Origin-Opener-Policy: same-origin`. Verify the built app runs under that CSP.
  - Service worker caches only the app's own assets.
- Quality: ESLint (typescript-eslint, react-hooks, jsx-a11y, plus a security plugin), Vitest unit tests for `src/core` (every validation rule and limit, archive attacks: traversal, zip bomb, symlinks, duplicates), component tests for the main flows, and one Playwright smoke test using the preinstalled Chromium (`executablePath: '/opt/pw-browsers/chromium'`, never run `playwright install`) that loads the production build with the CSP headers.
- CI: `.github/workflows/web.yml` running install with `npm ci`, typecheck, lint, tests, build, `npm audit --omit=dev`. Pin actions to commit SHAs, minimal `permissions`.
- Docs: `web/README.md` (develop, build, deploy to Vercel, storage and privacy model).

## Dependencies

Pin exact versions in `package.json` and commit `package-lock.json`. Skip versions published less than 14 days ago (`npm view <pkg> time`). Prefer few, well-maintained packages; justify each runtime dependency in your report. `npm audit` must show no high or critical issues in runtime dependencies; explain any remaining dev-only findings.

## Done means

Typecheck, lint, tests, build and audit pass; the Playwright smoke test passes against the production build with the CSP; you report what you built, what you tested, any spec ambiguity, and anything left undone. Commit in small, focused commits with English messages.
