import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import type { Plugin } from 'vite'

/** Fichiers de `public/` ajoutés à la coquille hors ligne. */
const PUBLIC_PRECACHE = ['/favicon.svg', '/manifest.webmanifest', '/icons/icon-192.png', '/icons/badge-72.png', '/brand/occ-mons-studios.webp']

/**
 * Fichiers du build à précacher : JS, CSS et polices latines (les autres sous-ensembles
 * de polices et les sourcemaps restent hors coquille).
 */
export function precacheable(file: string): boolean {
  if (file.endsWith('.map') || file === 'sw.js' || file.startsWith('outils/')) return false
  if (file.endsWith('.woff2')) return /-latin(-ext)?-/.test(file)
  return /\.(js|css|svg|png|webp)$/.test(file)
}

/** Liste triée (stable) des URL précachées. */
export function precacheList(files: string[]): string[] {
  return ['/', ...[...new Set(files.filter(precacheable).map((f) => `/${f}`))].sort(), ...PUBLIC_PRECACHE]
}

const read = (rel: string) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8')

/** Assemble dist/sw.js : constantes du build + règles (pwa/sw-routes.js) + gabarit (pwa/sw.js). */
export function buildServiceWorker(files: string[]): string {
  const list = precacheList(files)
  const routes = read('./sw-routes.js').replace(/^export /gm, '')
  const main = read('./sw.js')
  const version = createHash('sha256').update(list.join('\n')).update(routes).update(main).digest('hex').slice(0, 12)
  return [
    '/* OCC Deliveries — service worker généré par pwa/plugin.ts : ne pas modifier. */',
    `const SW_VERSION = ${JSON.stringify(version)};`,
    `const PRECACHE = ${JSON.stringify(list)};`,
    routes,
    main,
  ].join('\n')
}

/** Plugin Vite : émet `sw.js` à la racine du build (aucun service worker en dev). */
export function serviceWorker(): Plugin {
  return {
    name: 'occ-service-worker',
    apply: 'build',
    enforce: 'post',
    generateBundle(_options, bundle) {
      this.emitFile({ type: 'asset', fileName: 'sw.js', source: buildServiceWorker(Object.keys(bundle)) })
    },
  }
}
