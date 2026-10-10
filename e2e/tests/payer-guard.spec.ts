import { expect, test, type APIRequestContext, type BrowserContext } from '@playwright/test'

/**
 * Garde du payeur (ADR 0003, mise à jour 4) : sans moyen de remboursement, le
 * payeur ne peut pas être validé « par virement » (409 `payer_no_payout`), mais
 * l'est toujours « en espèces ». Alice (hôte, desktop) et Bob préparés par l'API.
 */

const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
const PASSWORD = 'e2e-pass-1234'

interface Session {
  token: string
  record: { id: string; name: string; email: string }
}

async function signup(api: APIRequestContext, name: string): Promise<Session> {
  const email = `${name.toLowerCase()}.payer.${RUN}@e2e.test`
  const created = await api.post('/api/collections/users/records', { data: { name, email, password: PASSWORD, passwordConfirm: PASSWORD } })
  expect(created.ok(), await created.text()).toBeTruthy()
  const auth = await api.post('/api/collections/users/auth-with-password', { data: { identity: email, password: PASSWORD } })
  expect(auth.ok()).toBeTruthy()
  return (await auth.json()) as Session
}

const H = (s: Session) => ({ Authorization: s.token })

async function login(ctx: BrowserContext, s: Session) {
  await ctx.addInitScript((auth) => {
    if (!localStorage.getItem('pocketbase_auth')) localStorage.setItem('pocketbase_auth', JSON.stringify(auth))
  }, { token: s.token, record: s.record })
}

/** Party au récap : Alice et Bob ont chacun commandé un plat sans option. */
async function reviewParty(api: APIRequestContext, alice: Session, bob: Session) {
  const nearby = await (await api.get('/api/occ/restaurants/nearby?lat=50.4542&lng=3.9567&radiusKm=50')).json()
  for (const r of nearby.items as { id: string }[]) {
    const filter = encodeURIComponent(`restaurant = "${r.id}" && available = true`)
    const items = await (await api.get(`/api/collections/menu_items/records?perPage=100&filter=${filter}`)).json()
    const simple = (items.items as { id: string; price: number; option_groups: unknown[] | null }[]).find((i) => i.price > 0 && (!i.option_groups || i.option_groups.length === 0))
    if (!simple) continue
    const party = await (await api.post('/api/collections/parties/records', { headers: H(alice), data: { title: `Payeur ${RUN}` } })).json()
    expect((await api.post('/api/occ/parties/join', { headers: H(bob), data: { code: party.code } })).ok()).toBeTruthy()
    expect((await api.post(`/api/occ/parties/${party.id}/transition`, { headers: H(alice), data: { to: 'ordering', restaurant: r.id } })).ok()).toBeTruthy()
    for (const s of [alice, bob]) {
      const res = await api.post('/api/collections/order_items/records', { headers: H(s), data: { party: party.id, user: s.record.id, menu_item: simple.id, quantity: 1 } })
      expect(res.ok(), await res.text()).toBeTruthy()
    }
    expect((await api.post(`/api/occ/parties/${party.id}/transition`, { headers: H(alice), data: { to: 'review' } })).ok()).toBeTruthy()
    return party as { id: string; code: string }
  }
  throw new Error('Aucun plat simple trouvé près de la position par défaut.')
}

test('payeur sans IBAN : bloqué par virement, validé en espèces', async ({ browser, request, baseURL }) => {
  const alice = await signup(request, 'Alice')
  const bob = await signup(request, 'Bob')
  const party = await reviewParty(request, alice, bob)

  // Serveur : refus explicite et code machine, différent pour soi et pour un·e autre.
  for (const [payer, msg] of [
    [alice.record.id, 'Ajoute ton IBAN (ou Revolut / PayPal) dans ton profil avant de valider la commande.'],
    [bob.record.id, "Bob n'a encore renseigné aucun moyen de remboursement."],
  ] as const) {
    const res = await request.post(`/api/occ/parties/${party.id}/payer`, { headers: H(alice), data: { payer, collectMode: 'transfer' } })
    expect(res.status()).toBe(409)
    const body = await res.json()
    expect(body.message).toBe(msg)
    expect(body.data.payer.code).toBe('payer_no_payout')
  }
  // Aucune donnée privée exposée : seulement des booléens.
  const readiness = await (await request.get(`/api/occ/parties/${party.id}/payout-readiness`, { headers: H(bob) })).json()
  expect(readiness).toMatchObject({ readyCount: 0, total: 2 })

  const ctx = await browser.newContext({ baseURL, viewport: { width: 1280, height: 860 }, locale: 'fr-BE', timezoneId: 'Europe/Brussels' })
  await login(ctx, alice)
  const page = await ctx.newPage()
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  try {
    await page.goto(`/party/${party.id}`)
    await expect(page.getByRole('heading', { name: 'Qui a pris quoi' })).toBeVisible()
    // Alerte douce et compteur de l'hôte.
    await expect(page.getByText(/Ajoute ton IBAN pour être remboursé·e par virement si tu avances la commande/)).toBeVisible()
    await expect(page.getByText(/Personne n.a encore renseigné d.IBAN/)).toBeVisible()

    await page.getByRole('button', { name: 'Qui a payé ?' }).click()
    const ps = page.getByRole('dialog', { name: "Qui a avancé l'argent ?" })
    await expect(ps.getByRole('radio', { name: /Alice \(toi\)/ })).toHaveAttribute('aria-checked', 'true')
    await expect(ps.getByRole('radio', { name: /Alice \(toi\)/ })).toContainText('Aucun moyen de remboursement')
    await expect(ps.getByRole('button', { name: 'Valider la commande' })).toBeDisabled()
    // Ajout rapide proposé sans quitter la party.
    await ps.getByRole('button', { name: 'Ajouter mon IBAN' }).click()
    await expect(ps.getByRole('form', { name: 'Ajouter mon IBAN' }).getByLabel('IBAN')).toBeVisible()
    // Chemin espèces : jamais bloqué.
    await ps.getByRole('button', { name: "Pas d'IBAN ? Valider en espèces" }).click()
    await expect(ps).toBeHidden()
    await expect(page.getByRole('heading', { name: 'Remboursement en espèces' })).toBeVisible()
    await expect(page.getByText('Remboursement en espèces : confirme chaque part dès que tu l’as reçue.')).toBeVisible()
    await expect(page.getByRole('button', { name: /Présenter/ })).toHaveCount(0)

    // Bob : espèces / plus tard seulement ; un virement déclaré est refusé.
    const pays = await (await request.get(`/api/collections/payments/records?filter=${encodeURIComponent(`party = "${party.id}" && debtor = "${bob.record.id}"`)}`, { headers: H(bob) })).json()
    const payment = pays.items[0] as { id: string }
    const qr = await (await request.get(`/api/occ/payments/${payment.id}/qr`, { headers: H(bob) })).json()
    expect(qr).toMatchObject({ collectMode: 'cash', epc: null, links: [], methods: ['cash', 'later'] })
    const refused = await request.post(`/api/occ/payments/${payment.id}/action`, { headers: H(bob), data: { action: 'declare', method: 'qr' } })
    expect(refused.status()).toBe(400)
    expect((await request.post(`/api/occ/payments/${payment.id}/action`, { headers: H(bob), data: { action: 'declare', method: 'cash' } })).ok()).toBeTruthy()

    // Alice confirme la réception depuis « Encaisser » → clôture.
    const card = page.getByRole('list', { name: 'Parts à encaisser' }).getByRole('listitem').filter({ hasText: 'Bob' })
    await expect(card).toContainText('Espèces')
    await card.getByRole('button', { name: 'Confirmer la réception' }).click()
    await expect(page.getByRole('heading', { name: 'Tout est réglé !' })).toBeVisible()
    expect(errors).toEqual([])
  } finally {
    await ctx.close()
  }
})
