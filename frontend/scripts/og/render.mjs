// Rendu de l'image Open Graph : public/og/og-image.jpg (1200×630, JPEG, ≤ 300 Ko).
// Rejouable : cd frontend && NODE_PATH=<dossier contenant playwright> node scripts/og/render.mjs
// Playwright n'est PAS une dépendance du projet (aucun ajout) : il est résolu par NODE_PATH
// (ou installé globalement), avec le Chromium préinstallé de l'environnement.
import { createRequire } from 'node:module'
import { mkdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'

const require = createRequire(import.meta.url)
const { chromium } = require('playwright')

const root = fileURLToPath(new URL('../..', import.meta.url))
const out = fileURLToPath(new URL('../../public/og/og-image.jpg', import.meta.url))
const MAX_BYTES = 300 * 1024

const server = await createServer({ root, configFile: `${root}vite.config.ts`, server: { port: 0, strictPort: false }, logLevel: 'warn' })
await server.listen()
const base = server.resolvedUrls?.local[0]
if (!base) throw new Error('serveur Vite sans URL locale')

const browser = await chromium.launch()
try {
  const page = await browser.newPage({ viewport: { width: 1200, height: 630 }, deviceScaleFactor: 1, colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.goto(new URL('scripts/og/index.html', base).href, { waitUntil: 'networkidle' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForFunction(() => [...document.images].every((i) => i.complete && i.naturalWidth > 0))
  mkdirSync(fileURLToPath(new URL('../../public/og/', import.meta.url)), { recursive: true })
  // qualité décroissante jusqu'à tenir dans le budget
  for (const quality of [88, 84, 80, 75, 70]) {
    await page.screenshot({ path: out, type: 'jpeg', quality, clip: { x: 0, y: 0, width: 1200, height: 630 } })
    const size = statSync(out).size
    if (size <= MAX_BYTES) {
      console.log(`og-image.jpg : 1200×630, qualité ${quality}, ${(size / 1024).toFixed(0)} Ko`)
      break
    }
  }
  if (statSync(out).size > MAX_BYTES) throw new Error('image OG > 300 Ko')
} finally {
  await browser.close()
  await server.close()
}
