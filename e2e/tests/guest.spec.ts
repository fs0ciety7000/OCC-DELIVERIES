import { expect, test, type APIRequestContext } from '@playwright/test'

/**
 * Invitée sans compte : Léa ouvre le lien /j/:code, choisit « Continuer en invité·e »,
 * entre juste son prénom, commande un plat et apparaît dans le récapitulatif de l'hôte.
 * L'hôte (Alice) et la commande sont préparés par l'API ; le plat est choisi dans les données du serveur.
 */

const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
const DEFAULT_LAT = 50.4542
const DEFAULT_LNG = 3.9567

interface ApiGroup { min: number }
interface ApiItem { id: string; name: string; price: number; option_groups: ApiGroup[] | null }
interface ApiRestaurant { id: string; name: string }

async function json<T>(res: Awaited<ReturnType<APIRequestContext['get']>>): Promise<T> {
  expect(res.ok(), `${res.url()} → ${res.status()} ${await res.text()}`).toBeTruthy()
  return (await res.json()) as T
}

/** Un restaurant proche et un plat sans option obligatoire dont le nom n'en préfixe aucun autre. */
async function pickDish(request: APIRequestContext) {
  const { items: nearby } = await json<{ items: ApiRestaurant[] }>(await request.get(`/api/occ/restaurants/nearby?lat=${DEFAULT_LAT}&lng=${DEFAULT_LNG}&radiusKm=10`))
  for (const r of nearby) {
    const filter = encodeURIComponent(`restaurant='${r.id}' && available=true`)
    const { items } = await json<{ items: ApiItem[] }>(await request.get(`/api/collections/menu_items/records?perPage=500&filter=${filter}`))
    const { items: cats } = await json<{ items: { name: string }[] }>(await request.get(`/api/collections/menu_categories/records?perPage=200&filter=${encodeURIComponent(`restaurant='${r.id}'`)}`))
    const labels = [...items.map((i) => i.name), ...cats.map((c) => c.name)]
    const dish = items.find((i) => i.price > 0 && !(i.option_groups ?? []).some((g) => g.min >= 1) && labels.filter((l) => l.startsWith(i.name)).length === 1)
    if (dish) return { restaurant: r, dish }
  }
  throw new Error('Aucun plat simple trouvé près de la position par défaut.')
}

test('invitée « Léa » : lien /j/:code, prénom seul, commande, visible dans le récap', async ({ browser, request, baseURL }) => {
  const { restaurant, dish } = await pickDish(request)

  // Alice (compte) crée la commande et fixe le restaurant (API).
  const email = `alice.${RUN}@e2e.test`
  await json(await request.post('/api/collections/users/records', { data: { email, password: 'e2e-pass-1234', passwordConfirm: 'e2e-pass-1234', name: 'Alice' } }))
  const { token } = await json<{ token: string }>(await request.post('/api/collections/users/auth-with-password', { data: { identity: email, password: 'e2e-pass-1234' } }))
  const headers = { Authorization: token }
  const party = await json<{ id: string; code: string }>(await request.post('/api/collections/parties/records', { headers, data: { title: `Midi invité ${RUN}` } }))
  await json(await request.post(`/api/occ/parties/${party.id}/transition`, { headers, data: { to: 'ordering', restaurant: restaurant.id } }))

  // Léa : mobile, aucune session.
  const ctx = await browser.newContext({ baseURL, viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, locale: 'fr-BE', timezoneId: 'Europe/Brussels' })
  const page = await ctx.newPage()
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  try {
    await page.goto(`/j/${party.code}`)
    await expect(page.getByRole('heading', { name: `Midi invité ${RUN}` })).toBeVisible()
    await expect(page.getByText("Alice t'invite")).toBeVisible()
    await page.getByRole('button', { name: 'Continuer en invité·e' }).click()
    await page.getByLabel('Ton prénom').fill('Léa')
    await page.getByRole('button', { name: 'Rejoindre la commande' }).click()
    await page.waitForURL(`**/party/${party.id}`)
    await expect(page.getByRole('heading', { name: `On commande chez ${restaurant.name}` })).toBeVisible()

    // Commande d'un plat (prix calculé par le serveur).
    await page.getByRole('button', { name: new RegExp(`^${dish.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`) }).first().click()
    const sheet = page.getByRole('dialog', { name: dish.name })
    await expect(sheet).toBeVisible()
    await sheet.getByRole('button', { name: /^Ajouter/ }).click()
    await expect(sheet).toBeHidden()
    await expect(page.getByText(`${dish.name} ajouté à ton panier`).first()).toBeVisible()
    await page.getByRole('button', { name: 'Je suis prêt·e' }).click()

    // Léa apparaît dans le récapitulatif de l'hôte, avec sa ligne au prix serveur.
    await expect
      .poll(async () => {
        const s = await json<{ participants: { user: { name: string }; subtotal: number; ready: boolean }[] }>(await request.get(`/api/occ/parties/${party.id}/summary`, { headers }))
        const lea = s.participants.find((p) => p.user.name === 'Léa')
        return lea ? { subtotal: lea.subtotal, ready: lea.ready } : null
      })
      .toEqual({ subtotal: dish.price, ready: true })

    // Profil limité : pas de coordonnées de remboursement, mais « Créer mon compte ».
    await page.goto('/profile?onglet=infos')
    await expect(page.getByRole('heading', { name: 'Créer mon compte' })).toBeVisible()
    await expect(page.getByText('Invité·e — sans compte')).toBeVisible()
    expect(errors).toEqual([])
  } finally {
    await ctx.close()
  }
})
