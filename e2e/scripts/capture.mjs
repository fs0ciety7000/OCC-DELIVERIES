// Usage: node capture.mjs <baseUrl> <outDir>
import { chromium } from '@playwright/test'
import fs from 'node:fs'

const BASE = process.argv[2] || 'http://localhost:8090'
const OUT = process.argv[3] || '/tmp/claude-0/design/shots'
const ONLY = process.argv[4] ? process.argv[4].split(',') : null
fs.mkdirSync(OUT, { recursive: true })

const rnd = Math.random().toString(36).slice(2, 8)
async function api(path, { method = 'GET', body, token } = {}) {
  const res = await fetch(BASE + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: token } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const txt = await res.text()
  if (!res.ok) throw new Error(`${method} ${path} ${res.status} ${txt}`)
  return txt ? JSON.parse(txt) : null
}
async function mkUser(name) {
  const email = `design-${rnd}-${name.toLowerCase()}@example.com`
  const password = 'design-pass-123'
  await api('/api/collections/users/records', { method: 'POST', body: { email, password, passwordConfirm: password, name } })
  const auth = await api('/api/collections/users/auth-with-password', { method: 'POST', body: { identity: email, password } })
  return { ...auth, email, password }
}

const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium-1194/chrome-linux/chrome' })
const VARIANTS = [
  { vp: 'mobile', w: 390, h: 844 },
  { vp: 'desktop', w: 1280, h: 800 },
]
const THEMES = ['dark', 'light']

async function ctxFor(auth, theme, vp) {
  const ctx = await browser.newContext({ viewport: { width: vp.w, height: vp.h }, deviceScaleFactor: 1, locale: 'fr-BE', reducedMotion: 'no-preference' })
  await ctx.addInitScript(
    ([a, t]) => {
      localStorage.setItem('occ-theme', t)
      if (a) localStorage.setItem('pocketbase_auth', JSON.stringify({ token: a.token, record: a.record }))
    },
    [auth ? { token: auth.token, record: auth.record } : null, theme],
  )
  return ctx
}

const errors = []
async function shoot(name, auth, path, action, { full = false } = {}) {
  if (ONLY && !ONLY.includes(name)) return
  for (const vp of VARIANTS)
    for (const theme of THEMES) {
      const ctx = await ctxFor(auth, theme, vp)
      const page = await ctx.newPage()
      page.on('console', (m) => m.type() === 'error' && errors.push(`${name}/${vp.vp}/${theme}: ${m.text()}`))
      page.on('pageerror', (e) => errors.push(`${name}: ${e.message}`))
      await page.goto(BASE + path, { waitUntil: 'load' })
      await page.waitForTimeout(2200)
      if (action) await action(page, vp)
      await page.waitForTimeout(900)
      // overflow check
      const ov = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
      if (ov > 0) errors.push(`OVERFLOW ${name}/${vp.vp}: ${ov}px`)
      const broken = await page.evaluate(() => [...document.images].filter((i) => i.complete && i.naturalWidth === 0).map((i) => i.src))
      if (broken.length) errors.push(`BROKEN IMG ${name}: ${broken.join(',')}`)
      await page.screenshot({ path: `${OUT}/${name}-${vp.vp}-${theme}.png`, fullPage: full || !!process.env.FULL })
      await ctx.close()
    }
  console.log('shot', name)
}

const host = await mkUser('Camille')
const guest = await mkUser('Yassine')
const H = host.token, G = guest.token

await api('/api/collections/payout_profiles/records', {
  method: 'POST', token: H,
  body: { user: host.record.id, holder_name: 'Camille Dubois', iban: 'BE68539007547034', wero_id: '+32470123456', bancontact_phone: '+32470123456' },
})

const rests = (await api('/api/collections/restaurants/records?perPage=50')).items
const bella = rests.find((r) => r.name === 'La Bella Nonna')
const cands = [bella.id, rests.find((r) => r.name.includes('Burger')).id, rests.find((r) => r.name.includes('Hanami')).id]

await shoot('home-loggedout', null, '/', null)
await shoot('login', null, '/login', null)
await shoot('register', null, '/register', null)
await shoot('home', host, '/', null)
await shoot('restaurants', host, '/restaurants', null)
await shoot('menu-options', host, `/restaurants/${bella.id}`, async (page) => {
  await page.getByRole('button', { name: /Margherita/ }).first().click()
  await page.waitForTimeout(700)
})

const party = await api('/api/collections/parties/records', { method: 'POST', token: H, body: { title: 'Midi du vendredi', host: host.record.id, candidates: cands } })
await api('/api/occ/parties/join', { method: 'POST', token: G, body: { code: party.code } })
await shoot('party-lobby', host, `/party/${party.id}`, null)

await api(`/api/occ/parties/${party.id}/transition`, { method: 'POST', token: H, body: { to: 'voting' } })
for (const [t, u, r] of [[G, guest, cands[0]], [H, host, cands[0]], [G, guest, cands[2]]])
  await api('/api/collections/votes/records', { method: 'POST', token: t, body: { party: party.id, user: u.record.id, restaurant: r } })
await shoot('party-voting', guest, `/party/${party.id}`, null)

await api(`/api/occ/parties/${party.id}/transition`, { method: 'POST', token: H, body: { to: 'ordering', restaurant: bella.id } })
const items = (await api(`/api/collections/menu_items/records?perPage=50&filter=${encodeURIComponent(`restaurant='${bella.id}'`)}`)).items
const marg = items.find((i) => i.name === 'Margherita')
const other = items.filter((i) => !i.option_groups?.some((g) => g.min > 0))
await api('/api/collections/order_items/records', { method: 'POST', token: G, body: { party: party.id, user: guest.record.id, menu_item: marg.id, quantity: 1, selected_options: [{ group: 'size', choices: ['l'] }, { group: 'extras', choices: ['burrata'] }], note: 'Bien cuite svp' } })
if (other[0]) await api('/api/collections/order_items/records', { method: 'POST', token: G, body: { party: party.id, user: guest.record.id, menu_item: other[0].id, quantity: 2, selected_options: [] } })
await api('/api/collections/order_items/records', { method: 'POST', token: H, body: { party: party.id, user: host.record.id, menu_item: marg.id, quantity: 1, selected_options: [{ group: 'size', choices: ['m'] }] } })
await api(`/api/occ/parties/${party.id}/ready`, { method: 'POST', token: H, body: { ready: true } })
await shoot('party-ordering', guest, `/party/${party.id}`, null)
await shoot('party-cart', guest, `/party/${party.id}`, async (page) => {
  await page.getByRole('button', { name: /Mon panier/ }).first().click()
  await page.waitForTimeout(700)
})

await api(`/api/occ/parties/${party.id}/transition`, { method: 'POST', token: H, body: { to: 'review' } })
await shoot('party-review', host, `/party/${party.id}`, null)

await api(`/api/occ/parties/${party.id}/payer`, { method: 'POST', token: H, body: { payer: host.record.id } })
await shoot('party-paying', guest, `/party/${party.id}`, async (page) => {
  const wero = page.getByRole('radio', { name: /Wero/ }).first()
  if (await wero.count()) await wero.click()
  await page.waitForTimeout(600)
})
await shoot('party-paying-host', host, `/party/${party.id}`, null)

await api(`/api/occ/parties/${party.id}/transition`, { method: 'POST', token: H, body: { to: 'closed' } })
await shoot('party-closed', guest, `/party/${party.id}`, async (page) => page.waitForTimeout(1500))

console.log('party', party.id, 'host', host.email, 'guest', guest.email)
console.log(errors.join('\n'))
await browser.close()
