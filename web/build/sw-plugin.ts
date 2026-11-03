/**
 * Emits `sw.js`, a small service worker that precaches exactly the files of
 * this build (the app's own assets plus `public/`) and serves them offline.
 * It never caches anything else: no runtime caching, no cross-origin requests.
 */
import { createHash } from 'node:crypto';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import type { Plugin } from 'vite';

function listPublic(dir: string): string[] {
  try {
    return readdirSync(dir, { recursive: true })
      .map(String)
      .filter((f) => statSync(join(dir, f)).isFile())
      .map((f) => f.split('\\').join('/'));
  } catch {
    return [];
  }
}

export function serviceWorkerPlugin(templatePath: string): Plugin {
  let publicDir = '';
  return {
    name: 'bearings-service-worker',
    apply: 'build',
    configResolved(config) {
      publicDir = config.publicDir;
    },
    generateBundle(_options, bundle) {
      const hash = createHash('sha256');
      const bundled = Object.keys(bundle).filter((f) => !f.endsWith('.map') && f !== 'sw.js');
      for (const f of bundled.sort()) {
        const item = bundle[f];
        if (!item) continue;
        hash.update(f);
        hash.update(item.type === 'chunk' ? item.code : item.source);
      }
      const pub = listPublic(publicDir);
      for (const f of pub.sort()) {
        hash.update(f);
        hash.update(readFileSync(join(publicDir, f)));
      }
      const files = [...new Set([...bundled, ...pub])].filter((f) => f !== 'index.html').sort();
      const assets = ['./', ...files.map((f) => `./${f}`)];
      const version = hash.digest('hex').slice(0, 16);
      const source = readFileSync(templatePath, 'utf8')
        .replace('__BEARINGS_VERSION__', JSON.stringify(version))
        .replace('__BEARINGS_ASSETS__', JSON.stringify(assets));
      this.emitFile({ type: 'asset', fileName: 'sw.js', source });
    },
  };
}
