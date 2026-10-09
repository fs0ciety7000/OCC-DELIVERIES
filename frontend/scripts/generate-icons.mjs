// Génère les icônes PWA (PNG) à partir de public/favicon.svg, rendues par le Chromium
// de Playwright (installé pour les tests e2e : `cd e2e && npm ci`).
// Les PNG produits sont commités : ce script ne tourne pas pendant le build.
//   node scripts/generate-icons.mjs
import { mkdirSync, readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(join(root, '..', 'e2e', 'package.json'))
const { chromium } = require('playwright')

const svg = readFileSync(join(root, 'public', 'favicon.svg'), 'utf8')
const out = join(root, 'public', 'icons')
mkdirSync(out, { recursive: true })

const bg = '<rect width="64" height="64" rx="16" fill="#121217"/>'
if (!svg.includes(bg)) throw new Error('favicon.svg : fond attendu introuvable, adapter le script')
// « maskable » : fond bord à bord, flamme réduite dans la zone sûre (cercle de 80 %).
const maskable = svg
  .replace(bg, '<rect width="64" height="64" fill="#121217"/><g transform="translate(32 32) scale(0.72) translate(-32 -32)">')
  .replace('</svg>', '</g></svg>')
// Badge Android (barre d'état) : seule l'opacité compte → flamme blanche sur transparent.
const badge = svg
  .replace(bg, '')
  .replace('fill="url(#ember)"', 'fill="#FFFFFF"')
  .replace(/<circle[^>]*\/>/, '')
  .replace(/<rect x="16"[^>]*\/>/, '')

const jobs = [
  { file: 'icon-192.png', svg, size: 192 },
  { file: 'icon-512.png', svg, size: 512 },
  { file: 'icon-maskable-192.png', svg: maskable, size: 192 },
  { file: 'icon-maskable-512.png', svg: maskable, size: 512 },
  { file: 'apple-touch-icon.png', svg: maskable, size: 180 },
  { file: 'badge-72.png', svg: badge, size: 72 },
]

const browser = await chromium.launch()
try {
  for (const j of jobs) {
    const page = await browser.newPage({ viewport: { width: j.size, height: j.size } })
    const sized = j.svg.replace('<svg ', `<svg width="${j.size}" height="${j.size}" `)
    await page.setContent(`<html><body style="margin:0;background:transparent">${sized}</body></html>`)
    await page.screenshot({ path: join(out, j.file), omitBackground: !j.file.startsWith('apple'), clip: { x: 0, y: 0, width: j.size, height: j.size } })
    await page.close()
  }
} finally {
  await browser.close()
}
console.log(`Icônes générées dans ${out}`)
