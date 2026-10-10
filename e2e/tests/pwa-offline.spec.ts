import { expect, test, type APIRequestContext, type BrowserContext, type Page } from '@playwright/test'

/**
 * PWA : manifeste installable, service worker (coquille hors ligne), file d'actions hors
 * ligne rejouée au retour du réseau (dédoublonnée par `client_key`), notifications push
 * (API) et heures limites automatiques (planificateur serveur, cron chaque minute).
 *
 * Lancer contre un serveur dont le SPA est un build de production (dist → pb_public) :
 *   E2E_BASE_URL=http://127.0.0.1:8101 npx playwright test tests/pwa-offline.spec.ts --output=/tmp/pw-push
 */

const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
const PASSWORD = 'e2e-pass-1234'

interface Session {
  token: string
  record: { id: string; name: string; email: string }
}

async function signup(api: APIRequestContext, name: string): Promise<Session> {
  const email = `${name.toLowerCase()}.${RUN}@e2e.test`
  const created = await api.post('/api/collections/users/records', { data: { name, email, password: PASSWORD, passwordConfirm: PASSWORD } })
  expect(created.ok(), await created.text()).toBeTruthy()
  const auth = await api.post('/api/collections/users/auth-with-password', { data: { identity: email, password: PASSWORD } })
  expect(auth.ok()).toBeTruthy()
  return (await auth.json()) as Session
}

const H = (s: Session) => ({ Authorization: s.token })

/** Connecte le navigateur avec la session (stockage du SDK PocketBase). */
async function login(ctx: BrowserContext, s: Session) {
  await ctx.addInitScript((auth) => {
    if (!localStorage.getItem('pocketbase_auth')) localStorage.setItem('pocketbase_auth', JSON.stringify(auth))
  }, { token: s.token, record: s.record })
}

/** Party en prise de commande dans un resto qui a au moins un plat disponible. */
async function orderingParty(api: APIRequestContext, host: Session) {
  const nearby = await (await api.get('/api/occ/restaurants/nearby?lat=50.4542&lng=3.9567&radiusKm=50')).json()
  let restaurant: { id: string; name: string } | undefined
  let item: { id: string; name: string } | undefined
  for (const r of nearby.items as { id: string; name: string }[]) {
    const filter = encodeURIComponent(`restaurant = "${r.id}" && available = true`)
    const items = await (await api.get(`/api/collections/menu_items/records?perPage=100&filter=${filter}`)).json()
    const simple = (items.items as { id: string; name: string; option_groups: unknown[] | null }[]).find(
      (i) => !i.option_groups || i.option_groups.length === 0,
    )
    if (simple) {
      restaurant = r
      item = simple
      break
    }
  }
  expect(restaurant, 'un resto avec un plat sans option').toBeTruthy()
  const party = await (await api.post('/api/collections/parties/records', { headers: H(host), data: { title: `PWA ${RUN}` } })).json()
  const tr = await api.post(`/api/occ/parties/${party.id}/transition`, { headers: H(host), data: { to: 'ordering', restaurant: restaurant!.id } })
  expect(tr.ok(), await tr.text()).toBeTruthy()
  return { party: party as { id: string; code: string }, restaurant: restaurant!, item: item! }
}

async function swReady(page: Page) {
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready
  })
  // la page est contrôlée après clients.claim() ou au rechargement suivant
  for (let i = 0; i < 4; i++) {
    if (await page.evaluate(() => !!navigator.serviceWorker.controller)) return
    await page.waitForTimeout(500)
    await page.reload()
  }
  expect(await page.evaluate(() => !!navigator.serviceWorker.controller)).toBe(true)
}

test('manifeste installable et service worker', async ({ request, page }) => {
  const manifest = await (await request.get('/manifest.webmanifest')).json()
  expect(manifest.display).toBe('standalone')
  expect(manifest.start_url).toMatch(/^\//)
  const sizes = (manifest.icons as { sizes: string; purpose: string; src: string }[]).map((i) => `${i.sizes}/${i.purpose}`)
  expect(sizes).toEqual(expect.arrayContaining(['192x192/any', '512x512/any', '192x192/maskable', '512x512/maskable']))
  for (const icon of manifest.icons as { src: string }[]) expect((await request.get(icon.src)).ok()).toBeTruthy()

  const sw = await (await request.get('/sw.js')).text()
  expect(sw).toContain('const PRECACHE = ["/"')
  await page.goto('/')
  await swReady(page)
  const caches = await page.evaluate(async () => caches.keys())
  expect(caches.some((c) => c.startsWith('occ-shell-'))).toBeTruthy()
})

test('hors ligne : bandeau, coquille en cache, ajout au panier rejoué sans doublon', async ({ browser, request, baseURL }) => {
  const host = await signup(request, 'Hote')
  const { party, item } = await orderingParty(request, host)
  // heure limite dans 30 min, sans clôture automatique pendant ce test
  await request.patch(`/api/collections/parties/records/${party.id}`, {
    headers: H(host),
    data: { ordering_ends_at: new Date(Date.now() + 30 * 60_000).toISOString(), auto_close_disabled: true },
  })

  const ctx = await browser.newContext({ baseURL, locale: 'fr-BE', timezoneId: 'Europe/Brussels', viewport: { width: 1280, height: 860 } })
  await login(ctx, host)
  const page = await ctx.newPage()
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  await page.goto(`/party/${party.id}`)
  await swReady(page)
  await expect(page.getByText(/Fin de la commande à \d{2}:\d{2}/)).toBeVisible()
  await expect(page.getByText('Clôture par l’hôte')).toBeVisible()
  await expect(page.getByRole('button', { name: item.name }).first()).toBeVisible()

  // --- hors ligne -------------------------------------------------------------
  await ctx.setOffline(true)
  await expect(page.getByRole('status').filter({ hasText: 'Hors ligne' })).toContainText('les données affichées peuvent dater')
  await page.getByRole('button', { name: item.name }).first().click()
  await page.getByRole('dialog').getByRole('button', { name: /Ajouter/ }).click()
  await expect(page.getByText(`${item.name} sera ajouté dès le retour du réseau`)).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'Hors ligne' })).toContainText("1 action en attente d'envoi")
  // actions non rejouables désactivées
  await expect(page.getByRole('button', { name: 'Passer au récap' })).toBeDisabled()

  // rechargement hors ligne : la coquille et les données en cache s'affichent
  await page.reload()
  await expect(page.getByText(/Fin de la commande à \d{2}:\d{2}/)).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'Hors ligne' })).toBeVisible()

  // --- retour du réseau : rejeu --------------------------------------------------
  await ctx.setOffline(false)
  await expect(page.getByText(`Synchronisé : Ajout : ${item.name}`)).toBeVisible({ timeout: 20_000 })
  const items = await (await request.get(`/api/collections/order_items/records?filter=${encodeURIComponent(`party = "${party.id}"`)}`, { headers: H(host) })).json()
  expect(items.totalItems).toBe(1)
  expect(items.items[0].client_key).toMatch(/^[A-Za-z0-9_-]+$/)
  // rejouer la même clé ne crée rien
  const replay = await request.post('/api/collections/order_items/records', {
    headers: H(host),
    data: { party: party.id, user: host.record.id, menu_item: item.id, quantity: 3, client_key: items.items[0].client_key },
  })
  expect((await replay.json()).id).toBe(items.items[0].id)
  expect(errors).toEqual([])
  await ctx.close()
})

test('notifications push : clé publique, abonnement, préférences, test', async ({ request }) => {
  const u = await signup(request, 'Pushy')
  const key = await (await request.get('/api/occ/push/public-key')).json()
  expect(key.enabled).toBe(true)
  expect(key.publicKey).toMatch(/^[A-Za-z0-9_-]{80,}$/)
  expect((await request.post('/api/occ/push/test', { headers: H(u) })).status()).toBe(400)
  // clés d'un vrai navigateur (P-256 + secret 16 octets), service fictif
  const p256dh = 'BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM'
  const sub = await request.post('/api/occ/push/subscribe', {
    headers: H(u),
    data: { endpoint: `https://push.invalid/${RUN}`, keys: { p256dh, auth: 'tBHItJI5svbpez7KI4CCXg' } },
  })
  expect(sub.ok(), await sub.text()).toBeTruthy()
  expect((await request.post('/api/occ/push/test', { headers: H(u) })).ok()).toBeTruthy()
  const prefs = await (await request.patch('/api/occ/push/prefs', { headers: H(u), data: { reminders: false } })).json()
  expect(prefs.prefs).toEqual({ party: true, payments: true, reminders: false, emails: true })
  expect((await request.delete('/api/occ/push/subscribe', { headers: H(u), data: { endpoint: `https://push.invalid/${RUN}` } })).ok()).toBeTruthy()
})

test('heure limite : clôture automatique visible dans la party', async ({ browser, request, baseURL }) => {
  test.setTimeout(170_000)
  const host = await signup(request, 'Chrono')
  const guest = await signup(request, 'Invite')
  const { party, item } = await orderingParty(request, host)
  await request.post('/api/occ/parties/join', { headers: H(guest), data: { code: party.code } })
  await request.post('/api/collections/order_items/records', { headers: H(host), data: { party: party.id, user: host.record.id, menu_item: item.id, quantity: 1 } })
  // prochaine minute pleine : le cron (chaque minute) clôture (rappels : tests Go, échéance trop proche ici)
  const end = new Date(Math.ceil((Date.now() + 5_000) / 60_000) * 60_000)
  const set = await request.patch(`/api/collections/parties/records/${party.id}`, { headers: H(host), data: { ordering_ends_at: end.toISOString() } })
  expect(set.ok(), await set.text()).toBeTruthy()

  const ctx = await browser.newContext({ baseURL, locale: 'fr-BE', timezoneId: 'Europe/Brussels' })
  await login(ctx, guest)
  const page = await ctx.newPage()
  await page.goto(`/party/${party.id}`)
  await expect(page.getByText('Clôture automatique')).toBeVisible()
  // le serveur passe la commande au récap tout seul et le journal l'affiche (temps réel)
  await expect(page.getByText(/Commande clôturée automatiquement à \d{2}:\d{2}/)).toBeVisible({ timeout: 140_000 })
  const p = await (await request.get(`/api/collections/parties/records/${party.id}`, { headers: H(host) })).json()
  expect(p.status).toBe('review')
  await ctx.close()
})
