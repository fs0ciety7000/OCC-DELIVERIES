import { readFile } from 'node:fs/promises'
import { expect, test, type Browser, type BrowserContext, type BrowserContextOptions, type Locator, type Page } from '@playwright/test'

/**
 * Scénario de référence OCC DELIVERIES piloté par l'UI réelle :
 * Alice (hôte, desktop) + Bob et Chloé (invités, mobile 390×844), puis Dave (non-membre).
 */

const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
const PASSWORD = 'e2e-pass-1234'

// Position par défaut de l'app (OCC_DEFAULT_LAT / OCC_DEFAULT_LNG) et rayon du salon.
const DEFAULT_LAT = 50.4542
const DEFAULT_LNG = 3.9567

const MOBILE: BrowserContextOptions = { viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true }
const DESKTOP: BrowserContextOptions = { viewport: { width: 1280, height: 860 } }

// ---------------------------------------------------------------------------
// Utilitaires

/** « 1 234,50 € » (espaces insécables inclus) → 123450 centimes. */
function cents(text: string): number {
  const m = /(-?\d{1,3}(?:[\s  .]\d{3})*|\d+),(\d{2})\s*€/.exec(text.replace(/[  ]/g, ' '))
  if (!m) throw new Error(`Montant introuvable dans « ${text} »`)
  return parseInt(m[1]!.replace(/[\s.]/g, ''), 10) * 100 + parseInt(m[2]!, 10)
}

function euro(c: number): RegExp {
  return new RegExp(`${Math.floor(c / 100)},${String(c % 100).padStart(2, '0')}[\\s\\u00a0\\u202f]*€`)
}

interface Actor {
  name: string
  email: string
  ctx: BrowserContext
  page: Page
  problems: string[]
  /** Motifs d'erreurs attendues pour cet acteur (ex. 404 du non-membre). */
  allow: RegExp[]
}

/**
 * Bruit réseau légitime : flux SSE coupé à la navigation / fermeture, requêtes annulées par TanStack,
 * images de couverture distantes (CDN des plateformes, données synchronisées) annulées par une navigation.
 */
const GLOBAL_ALLOW: RegExp[] = [
  /^\[requestfailed\] GET \S+\/api\/realtime net::ERR_ABORTED$/,
  // « Mes commandes en cours » (bandeau du shell, sur chaque page) annulée par une navigation immédiate.
  /^\[requestfailed\] GET \S+\/api\/collections\/parties\/records\?\S* net::ERR_ABORTED$/,
  /^\[requestfailed\] GET https:\/\/(?!127\.0\.0\.1|localhost)[^/\s]+\/\S*\.(?:jpe?g|png|webp|gif)(?:\?\S*)? net::ERR_ABORTED$/,
]

async function newActor(browser: Browser, name: string, opts: BrowserContextOptions, baseURL: string): Promise<Actor> {
  const ctx = await browser.newContext({ ...opts, baseURL, locale: 'fr-BE', timezoneId: 'Europe/Brussels', acceptDownloads: true })
  await ctx.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin })
  const page = await ctx.newPage()
  const actor: Actor = { name, email: `${name.normalize('NFD').replace(/[^a-z]/gi, '').toLowerCase()}.${RUN}@e2e.test`, ctx, page, problems: [], allow: [] }
  page.on('console', (msg) => {
    if (msg.type() === 'error') actor.problems.push(`[console] ${msg.text()} @ ${msg.location().url}`)
  })
  page.on('pageerror', (err) => actor.problems.push(`[pageerror] ${err.message}`))
  page.on('requestfailed', (req) => actor.problems.push(`[requestfailed] ${req.method()} ${req.url()} ${req.failure()?.errorText ?? ''}`))
  page.on('response', (res) => {
    if (res.status() >= 400) actor.problems.push(`[http ${res.status()}] ${res.request().method()} ${res.url()}`)
  })
  return actor
}

function unexpectedProblems(a: Actor): string[] {
  return a.problems.filter((p) => ![...GLOBAL_ALLOW, ...a.allow].some((re) => re.test(p)))
}

async function register(a: Actor, startPath = '/register') {
  const { page } = a
  if (!page.url().includes('/register')) await page.goto(startPath)
  await page.getByLabel('Prénom (ou surnom)').fill(a.name)
  await page.getByLabel('E-mail').fill(a.email)
  await page.getByLabel('Mot de passe').fill(PASSWORD)
  await page.getByRole('button', { name: 'Créer mon compte' }).click()
  await expect(page.getByText('Bienvenue à bord').first()).toBeVisible()
}

/** Captures pleine page des étapes clés (E2E_DEBUG=1) pour la revue visuelle. */
async function shot(actors: Actor[], label: string) {
  if (!process.env.E2E_DEBUG) return
  for (const a of actors) await a.page.screenshot({ path: `test-results/shots/${label}-${a.name}.png`, fullPage: true })
}

function dialog(page: Page, title: string | RegExp): Locator {
  return page.getByRole('dialog', { name: title })
}

/** Ajoute un article depuis le menu de la party, vérifie le prix affiché avant l'envoi. */
async function addItem(
  page: Page,
  item: string,
  opts: { pick?: string[]; quantity?: number; note: string; expectedLine: number; requiredGroup?: string },
) {
  await page.getByRole('button', { name: new RegExp(`^${esc(item)}`) }).first().click()
  const sheet = dialog(page, item)
  await expect(sheet).toBeVisible()
  if (opts.requiredGroup) {
    const group = sheet.getByRole('group', { name: new RegExp(opts.requiredGroup) })
    await expect(group).toContainText('Obligatoire')
  }
  for (const choice of opts.pick ?? []) await sheet.locator('label').filter({ hasText: choice }).click()
  for (let q = 1; q < (opts.quantity ?? 1); q++) await sheet.getByRole('button', { name: 'Augmenter' }).click()
  await sheet.getByLabel('Une précision ?').fill(opts.note)
  const submit = sheet.getByRole('button', { name: /^Ajouter/ })
  await expect(submit).toHaveText(euro(opts.expectedLine))
  await submit.click()
  await expect(sheet).toBeHidden()
  await expect(page.getByText(`${item} ajouté à ton panier`).first()).toBeVisible()
}


// ---------------------------------------------------------------------------
// Scénario construit depuis les données réelles du serveur (aucun nom codé en dur)

interface ApiChoice { id: string; name: string; price: number }
interface ApiGroup { id: string; name: string; min: number; max: number; choices: ApiChoice[] }
interface ApiItem { id: string; name: string; price: number; category: string; available: boolean; option_groups: ApiGroup[] | null }
interface ApiRestaurant { id: string; name: string; delivery_fee: number }

/** Un article à commander : options obligatoires à cocher et prix unitaire attendu (serveur). */
interface Pick {
  name: string
  /** Groupe radio obligatoire (affiche « Obligatoire »), s'il y en a un. */
  requiredGroup?: string
  /** Libellés de choix à cocher (groupes obligatoires à choix multiples). */
  pick: string[]
  unit: number
}

const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

function pickOf(item: ApiItem): Pick {
  let unit = item.price
  let requiredGroup: string | undefined
  const pick: string[] = []
  for (const g of item.option_groups ?? []) {
    if (g.min < 1) continue
    if (g.max === 1) {
      // l'app présélectionne le premier choix des groupes radio obligatoires
      requiredGroup ??= g.name
      unit += g.choices[0]!.price
    } else {
      for (const c of g.choices.slice(0, g.min)) {
        pick.push(c.name)
        unit += c.price
      }
    }
  }
  return { name: item.name, requiredGroup, pick, unit }
}

async function api<T>(base: string, path: string): Promise<T> {
  const res = await fetch(new URL(path, base))
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return (await res.json()) as T
}

/** Choisit 3 restaurants proches et 4 articles du gagnant (le 1er avec une option obligatoire s'il existe). */
async function buildScenario(base: string) {
  const { items: nearby } = await api<{ items: ApiRestaurant[] }>(base, `/api/occ/restaurants/nearby?lat=${DEFAULT_LAT}&lng=${DEFAULT_LNG}&radiusKm=10`)
  // noms non ambigus : aucun n'est contenu dans un autre (sélecteurs par texte)
  const unambiguous = nearby.filter((r) => !nearby.some((o) => o.id !== r.id && o.name.toLowerCase().includes(r.name.toLowerCase())))
  for (const winner of unambiguous) {
    const { items } = await api<{ items: ApiItem[] }>(base, `/api/collections/menu_items/records?perPage=500&sort=position,name&filter=${encodeURIComponent(`restaurant='${winner.id}' && available=true`)}`)
    const { items: cats } = await api<{ items: { name: string }[] }>(base, `/api/collections/menu_categories/records?perPage=200&filter=${encodeURIComponent(`restaurant='${winner.id}'`)}`)
    // un nom d'article ne doit préfixer ni un autre article ni un onglet de catégorie (sélecteur « ^nom »)
    const labels = [...items.map((o) => ({ id: o.id, name: o.name })), ...cats.map((c) => ({ id: '', name: c.name }))]
    const usable = items.filter((it) => !labels.some((o) => o.id !== it.id && o.name.startsWith(it.name)))
    if (usable.length < 4) continue
    const withRequired = usable.find((it) => (it.option_groups ?? []).some((g) => g.min >= 1))
    const first = withRequired ?? usable[0]!
    const rest = usable.filter((it) => it.id !== first.id)
    const others = unambiguous.filter((r) => r.id !== winner.id).slice(0, 2)
    if (others.length < 2) break
    return {
      winner,
      candidates: [winner.name, ...others.map((r) => r.name)],
      alice: pickOf(first),
      bob: pickOf(rest[0]!),
      chloe: [pickOf(rest[1]!), pickOf(rest[2]!)] as const,
    }
  }
  throw new Error('Pas assez de restaurants / articles près de la position par défaut pour le scénario E2E.')
}

// ---------------------------------------------------------------------------

test('commande groupée complète : vote → paniers → récap → dispatch → remboursements', async ({ browser, baseURL }) => {
  const base = baseURL!
  const sc = await buildScenario(base)
  const CANDIDATES = sc.candidates
  const WINNER = sc.winner.name
  const DELIVERY_FEE = sc.winner.delivery_fee
  const alice = await newActor(browser, 'Alice', DESKTOP, base)
  const bob = await newActor(browser, 'Bob', MOBILE, base)
  const chloe = await newActor(browser, 'Chloé', MOBILE, base)
  const dave = await newActor(browser, 'Dave', MOBILE, base)
  const actors = [alice, bob, chloe, dave]
  const members = [alice, bob, chloe]
  let partyPath = ''
  let code = ''
  const shares: Record<string, number> = {}
  let grand = 0

  try {
    await test.step('1. Alice crée la party ; Bob rejoint par lien, Chloé par code', async () => {
      await register(alice)
      await alice.page.getByRole('button', { name: 'Lancer une commande' }).first().click()
      const sheet = dialog(alice.page, 'Lancer une commande')
      await sheet.getByLabel('Nom de la commande').fill(`Midi E2E ${RUN}`)
      await sheet.getByLabel('Adresse de livraison').fill('Grand-Place 1, Mons')
      await alice.page.getByRole('button', { name: 'Créer le salon' }).click()
      await alice.page.waitForURL(/\/party\/[a-z0-9]+$/)
      partyPath = new URL(alice.page.url()).pathname
      const codeEl = alice.page.locator('header').getByLabel(/^Code [A-Z0-9]{6}$/)
      code = (await codeEl.textContent())!.trim()
      expect(code).toMatch(/^[A-HJ-NP-Z2-9]{6}$/)
      await expect(alice.page.getByText(`/j/${code}`)).toBeVisible()

      // Bob : lien d'invitation, non connecté → page d'invitation (connexion / invité·e) → inscription
      // → retour sur /j/:code → salle.
      await bob.page.goto(`/j/${code}`)
      await expect(bob.page.getByRole('heading', { name: `Midi E2E ${RUN}` })).toBeVisible()
      await bob.page.getByRole('link', { name: 'Créer un compte' }).click()
      await bob.page.waitForURL(/\/register\?next=%2Fj%2F/)
      await register(bob)
      await bob.page.waitForURL(`**${partyPath}`)
      await expect(bob.page.getByRole('heading', { name: `Midi E2E ${RUN}` })).toBeVisible()

      // Chloé : saisit le code sur l'accueil.
      await register(chloe)
      await chloe.page.getByLabel('Code de la commande (6 caractères)').fill(code.toLowerCase())
      await chloe.page.getByRole('button', { name: 'Rejoindre', exact: true }).click()
      await chloe.page.waitForURL(`**${partyPath}`)

      // L'hôte voit les arrivées en direct.
      await expect(alice.page.getByText('3 membres').first()).toBeVisible()
      for (const a of members) await expect(a.page.getByText('3 membres').first()).toBeVisible()
    })

    await shot(members, '1-lobby')
    await test.step('2. Trois candidats, vote en direct, clôture → gagnant', async () => {
      const ap = alice.page
      for (const r of CANDIDATES) {
        const card = ap.locator('button[aria-pressed]').filter({ hasText: r })
        await card.click()
        await expect(card).toHaveAttribute('aria-pressed', 'true')
      }
      // Les invités voient les candidats en direct.
      for (const g of [bob, chloe]) {
        for (const r of CANDIDATES) await expect(g.page.getByRole('list', { name: 'Restos candidats' })).toContainText(r)
      }
      // Bob quitte la salle (page Restos) : le bandeau « Commande en cours » lui permet de revenir.
      const bp = bob.page
      await bp.getByRole('link', { name: "Retour à l'accueil" }).click()
      await expect(bp.getByRole('link', { name: `Reprendre la commande « Midi E2E ${RUN} » — Salon` })).toBeVisible()
      await bp.goto('/restaurants')
      const resume = bp.getByRole('complementary', { name: 'Commande en cours' }).getByRole('link', { name: new RegExp(`^Reprendre la commande « Midi E2E ${RUN} »`) })
      await expect(resume).toBeVisible()
      await expect(resume).toContainText('Étape 1/5 · Salon')

      await ap.getByRole('button', { name: 'Lancer le vote (3)' }).click()
      // Notification globale (realtime) alors que Bob est ailleurs dans l'app, bandeau mis à jour en direct.
      await expect(bp.getByText(`Le vote est ouvert — « Midi E2E ${RUN} »`)).toBeVisible()
      await expect(resume).toContainText('Étape 2/5 · Vote')
      await resume.click()
      await bp.waitForURL(`**${partyPath}`)
      for (const a of members) await expect(a.page.getByRole('heading', { name: 'Vote pour tes restos préférés' })).toBeVisible()
      await expect(bp.getByRole('complementary', { name: 'Commande en cours' })).toHaveCount(0) // masqué dans la salle

      const meter = (p: Page, n: number, r: string) => p.getByRole('meter', { name: `${n} vote(s) pour ${r}`, exact: true })

      await bob.page.getByRole('button', { name: `Voter pour ${WINNER}`, exact: true }).click()
      await bob.page.getByRole('button', { name: `Voter pour ${CANDIDATES[1]}`, exact: true }).click()
      // Visible chez Alice et Chloé sans rechargement.
      for (const a of [alice, chloe]) {
        await expect(meter(a.page, 1, WINNER)).toBeVisible()
        await expect(meter(a.page, 1, CANDIDATES[1]!)).toBeVisible()
      }
      await chloe.page.getByRole('button', { name: `Voter pour ${WINNER}`, exact: true }).click()
      await chloe.page.getByRole('button', { name: `Voter pour ${CANDIDATES[2]}`, exact: true }).click()
      await chloe.page.getByRole('button', { name: `Retirer mon vote pour ${CANDIDATES[2]}`, exact: true }).click()
      await expect(meter(alice.page, 2, WINNER)).toBeVisible()
      await expect(meter(bob.page, 2, WINNER)).toBeVisible()
      await expect(meter(bob.page, 0, CANDIDATES[2]!)).toBeVisible()

      await ap.getByRole('button', { name: `Voter pour ${WINNER}`, exact: true }).click()
      for (const a of members) {
        await expect(meter(a.page, 3, WINNER)).toBeVisible()
        await expect(a.page.getByText('3/3 ont voté')).toBeVisible()
      }

      await ap.getByRole('button', { name: 'Clore le vote' }).click()
      await ap.getByRole('button', { name: `Valider — ${WINNER}` }).click()
      for (const a of members) await expect(a.page.getByRole('heading', { name: `On commande chez ${WINNER}` })).toBeVisible()
    })

    await shot(members, '2-voting-closed')
    // Lignes attendues (prix serveur) : base + options, × quantité.
    const expected = {
      Alice: sc.alice.unit,
      Bob: sc.bob.unit * 2,
      Chloé: sc.chloe[0].unit + sc.chloe[1].unit,
    }

    await test.step('3. Paniers avec options + note, prêts visibles en direct', async () => {
      const [c1, c2] = sc.chloe
      await addItem(alice.page, sc.alice.name, { requiredGroup: sc.alice.requiredGroup, pick: sc.alice.pick, note: 'Sans basilic svp', expectedLine: expected.Alice })
      await addItem(bob.page, sc.bob.name, { requiredGroup: sc.bob.requiredGroup, pick: sc.bob.pick, quantity: 2, note: 'Bien cuite', expectedLine: expected.Bob })
      await addItem(chloe.page, c1.name, { requiredGroup: c1.requiredGroup, pick: c1.pick, note: 'Bien fraîche', expectedLine: c1.unit })
      await addItem(chloe.page, c2.name, { requiredGroup: c2.requiredGroup, pick: c2.pick, note: 'Deux cuillères', expectedLine: c2.unit })

      // Prix serveur : le panier (sous-total) reprend les totaux calculés côté Go.
      for (const a of members) {
        const cartBtn = a.page.getByRole('button', { name: /^Mon panier/ })
        await expect(cartBtn).toHaveText(euro(expected[a.name as keyof typeof expected]))
      }
      await chloe.page.getByRole('button', { name: /^Mon panier/ }).click()
      const cart = dialog(chloe.page, 'Mon panier')
      await expect(cart).toContainText('« Bien fraîche »')
      await expect(cart).toContainText('« Deux cuillères »')
      await cart.getByRole('button', { name: 'Fermer' }).click()
      await expect(cart).toBeHidden()

      // L'équipe voit les paniers des autres en direct.
      await expect(alice.page.getByText('2 articles')).toHaveCount(2) // Bob (2×) et Chloé (2 lignes)

      await bob.page.getByRole('button', { name: 'Je suis prêt·e' }).click()
      for (const a of [alice, chloe]) await expect(a.page.getByText('1/3 prêts')).toBeVisible()
      await chloe.page.getByRole('button', { name: 'Je suis prêt·e' }).click()
      await alice.page.getByRole('button', { name: 'Je suis prêt·e' }).click()
      for (const a of members) {
        await expect(a.page.getByText('3/3 prêts')).toBeVisible()
        await expect(a.page.getByText('3 membres · 3 prêts')).toBeVisible()
      }
    })

    await shot(members, '3-ordering')
    const [c1n, c2n] = [sc.chloe[0].name, sc.chloe[1].name]
    await test.step('4. Récap : Σ parts = total ; dispatch Uber Eats ; export CSV', async () => {
      const ap = alice.page
      await ap.getByRole('button', { name: 'Passer au récap' }).click()
      await dialog(ap, 'Passer au récap ?').getByRole('button', { name: 'Verrouiller les paniers' }).click()
      for (const a of members) await expect(a.page.getByRole('heading', { name: 'Qui a pris quoi' })).toBeVisible()

      const rows = ap.locator('details > summary')
      await expect(rows).toHaveCount(3)
      const byName: Record<string, { total: number; fee: number }> = {}
      for (const s of await rows.all()) {
        const who = (await s.locator('p').first().textContent())!.trim()
        const detail = (await s.locator('p').nth(1).textContent())!
        const [, feeText] = detail.split('+')
        byName[who === 'Toi' ? 'Alice' : who] = { total: cents((await s.locator('span.font-display').textContent())!), fee: feeText ? cents(feeText) : 0 } // « + … de frais » absent quand les frais sont nuls
      }
      const grandTotal = cents((await ap.locator('dl > div').filter({ has: ap.locator('dt', { hasText: /^Total$/ }) }).locator('dd').textContent())!)
      const itemsSubtotal = expected.Alice + expected.Bob + expected.Chloé
      expect(grandTotal).toBe(itemsSubtotal + DELIVERY_FEE)
      expect(Object.keys(byName).sort()).toEqual(['Alice', 'Bob', 'Chloé'])
      expect(Object.values(byName).reduce((x, y) => x + y.total, 0), 'Σ parts = total général').toBe(grandTotal)
      // Frais de livraison partagés à parts égales, au centime près (plus grand reste).
      const share = Math.floor(DELIVERY_FEE / 3)
      const fees = [0, 1, 2].map((i) => share + (i < DELIVERY_FEE % 3 ? 1 : 0))
      expect(Object.values(byName).map((v) => v.fee).sort()).toEqual(fees.sort())
      for (const [who, v] of Object.entries(byName)) {
        expect(v.total - v.fee, `sous-total ${who}`).toBe(expected[who as keyof typeof expected])
        shares[who] = v.total
      }
      grand = grandTotal

      // Les invités voient le même total.
      for (const g of [bob, chloe]) await expect(g.page.locator('dl > div').filter({ has: g.page.locator('dt', { hasText: /^Total$/ }) }).locator('dd')).toHaveText(euro(grandTotal))

      // Dispatch Uber Eats.
      await ap.getByRole('button', { name: /Uber Eats.*Lien du resto/ }).click()
      const ue = dialog(ap, 'Envoyer via Uber Eats')
      await expect(ue).toBeVisible()
      expect(await ue.locator('ol > li').count()).toBeGreaterThan(0)
      await expect(ue.getByRole('link', { name: /Ouvrir Uber Eats/ })).toHaveAttribute('href', /^https:\/\/www\.ubereats\.com\//)
      const recap = (await ue.locator('pre').textContent())!
      for (const word of [sc.alice.name, sc.bob.name, c1n, c2n]) expect(recap).toContain(word)
      await ue.getByRole('button', { name: 'Copier le récap' }).click()
      await expect(ap.getByText('Récap copié').first()).toBeVisible()
      expect(await ap.evaluate(() => navigator.clipboard.readText())).toBe(recap)
      await ue.getByRole('button', { name: 'Fermer' }).click()
      await expect(ue).toBeHidden()
      for (const g of [bob, chloe]) await expect(g.page.getByText(/Commande envoyée via Uber Eats/)).toBeVisible()

      // Export CSV.
      await ap.getByRole('button', { name: /Exporter.*CSV, TXT ou JSON/ }).click()
      const ex = dialog(ap, 'Envoyer via Export')
      const [download] = await Promise.all([ap.waitForEvent('download'), ex.getByRole('button', { name: 'CSV', exact: true }).click()])
      expect(download.suggestedFilename()).toMatch(/\.csv$/)
      const csv = await readFile((await download.path())!, 'utf8')
      for (const word of ['Alice', 'Bob', 'Chloé', sc.alice.name, sc.bob.name, c2n, 'Sans basilic svp']) expect(csv).toContain(word)
      await expect(ap.getByText('Export CSV téléchargé').first()).toBeVisible()
      await ex.getByRole('button', { name: 'Fermer' }).click()
      await expect(ex).toBeHidden()
    })

    await shot(members, '4-review')
    await test.step('5. Bob renseigne son profil (IBAN, Revolut, PayPal, Wero), devient payeur ; Wero + espèces → clôture', async () => {
      const bp = bob.page
      await bp.goto('/profile?onglet=infos') // onglet « Mes infos » (« Mes commandes » par défaut)
      await bp.getByLabel('Mobile ou e-mail Wero').fill('+32470123456')
      await bp.getByLabel('Titulaire du compte').fill('Bob Martin')
      await bp.getByLabel('IBAN').fill('BE71096123456769')
      await expect(bp.getByText('IBAN valide ✓')).toBeVisible()
      await bp.getByLabel('Revtag Revolut').fill('@BobM')
      await bp.getByLabel('PayPal.me').fill('paypal.me/bobmartin')
      await bp.getByRole('button', { name: 'Enregistrer', exact: true }).last().click()
      await expect(bp.getByText('Coordonnées de remboursement enregistrées').first()).toBeVisible()
      await expect(bp.getByLabel('Revtag Revolut')).toHaveValue('bobm') // normalisé par le serveur
      await expect(bp.getByLabel('PayPal.me')).toHaveValue('bobmartin')
      await bp.goto(partyPath)
      await expect(bp.getByRole('heading', { name: 'Qui a pris quoi' })).toBeVisible()

      const ap = alice.page
      await ap.getByRole('button', { name: 'Qui a payé ?' }).click()
      const ps = dialog(ap, "Qui a avancé l'argent ?")
      await ps.getByRole('radio', { name: /Bob/ }).click()
      await expect(ps.getByRole('radio', { name: /Bob/ })).toHaveAttribute('aria-checked', 'true')
      await ps.getByRole('button', { name: 'Valider et passer aux remboursements' }).click()

      for (const debtor of [alice, chloe]) {
        const p = debtor.page
        await expect(p.getByRole('heading', { name: 'Ma part' })).toBeVisible()
        await expect(p.locator('div', { has: p.getByRole('heading', { name: 'Ma part' }) }).last().locator('..')).toContainText(euro(shares[debtor.name]!))
        const tiles = p.getByRole('radiogroup', { name: 'Moyen de remboursement' })
        const cents = shares[debtor.name]!
        if (debtor === alice) {
          // Desktop : le QR virement (montant + communication) d'abord, en grand.
          await expect(tiles.getByRole('radio').first()).toHaveAccessibleName(/Virement QR/)
          await expect(p.getByRole('img', { name: /QR virement SEPA de .* vers Bob Martin/ })).toBeVisible()
        } else {
          // Mobile : le téléphone ne peut pas scanner son propre écran → liens d'abord, QR à la demande.
          await expect(tiles.getByRole('radio').first()).toHaveAccessibleName(/Revolut/)
          await tiles.getByRole('radio', { name: /Virement QR/ }).click()
          await expect(p.getByRole('img', { name: /QR virement SEPA/ })).toBeHidden()
          await p.getByRole('button', { name: 'Afficher le QR pour un collègue' }).click()
          await expect(p.getByRole('img', { name: /QR virement SEPA de .* vers Bob Martin/ })).toBeVisible()
        }
        await expect(p.getByText('BE71 0961 2345 6769')).toBeVisible()
        await expect(p.getByText('IBAN de Bob Martin')).toBeVisible()
        // Liens Revolut / PayPal avec le montant exact de CE débiteur.
        await tiles.getByRole('radio', { name: /Revolut/ }).click()
        await expect(p.getByRole('link', { name: /^Payer .* avec Revolut/ })).toHaveAttribute('href', new RegExp(`^https://revolut\\.me/bobm\\?amount=${cents}&currency=EUR&note=OCC\\+`))
        await tiles.getByRole('radio', { name: /PayPal/ }).click()
        await expect(p.getByRole('link', { name: /^Payer .* avec PayPal/ })).toHaveAttribute('href', `https://paypal.me/bobmartin/${(cents / 100).toFixed(2)}EUR`)
        const wero = tiles.getByRole('radio', { name: /Wero/ })
        await expect(wero).toBeVisible()
        await wero.click()
        await expect(p.getByText('+32470123456')).toBeVisible()
        await tiles.getByRole('radio', { name: /Virement QR/ }).click()
        await expect(p.getByText('BE71 0961 2345 6769')).toBeVisible()
      }

      await alice.page.getByRole('radiogroup', { name: 'Moyen de remboursement' }).getByRole('radio', { name: /Wero/ }).click()
      await alice.page.getByRole('button', { name: "J'ai payé avec Wero" }).click()
      await expect(alice.page.getByText('En attente de confirmation par le payeur.')).toBeVisible()

      await chloe.page.getByRole('radiogroup', { name: 'Moyen de remboursement' }).getByRole('radio', { name: /Espèces/ }).click()
      await chloe.page.getByRole('button', { name: 'Je paie en espèces' }).click()
      await expect(chloe.page.getByText('En attente de confirmation par le payeur.')).toBeVisible()

      await shot(members, '4b-paying')
      // Bob voit les déclarations en direct puis confirme.
      const parts = bp.locator('li').filter({ has: bp.getByRole('button', { name: 'Confirmer' }) })
      await expect(bp.getByText(/avancé l.argent/)).toBeVisible()
      await expect(bp.getByRole('list', { name: 'Moyens proposés' })).toContainText('Revolut')
      await expect(bp.getByRole('list', { name: 'Moyens proposés' })).toContainText('PayPal')
      await expect(parts.filter({ hasText: 'Alice' })).toContainText('Wero')
      await expect(parts.filter({ hasText: 'Alice' })).toContainText('Déclaré')
      await expect(parts.filter({ hasText: 'Chloé' })).toContainText('Espèces')
      await parts.filter({ hasText: 'Alice' }).getByRole('button', { name: 'Confirmer' }).click()
      await expect(alice.page.getByText("C'est réglé, merci !")).toBeVisible()
      await bp.locator('li').filter({ hasText: 'Chloé' }).getByRole('button', { name: 'Confirmer' }).click()

      for (const a of members) {
        await expect(a.page.getByRole('heading', { name: 'Tout est réglé !' })).toBeVisible()
        await expect(a.page.getByText('Terminée', { exact: true }).first()).toBeVisible()
        const closed = a.page.locator('div', { has: a.page.getByText('Total commande', { exact: true }) }).last().locator('..')
        await expect(closed).toContainText(euro(grand))
        await expect(closed).toContainText(euro(shares[a.name]!))
      }
    })

    await shot(members, '5-closed')
    await test.step('5b. Alice retrouve la commande clôturée dans Profil → Mes commandes, avec sa part exacte', async () => {
      const ap = alice.page
      await ap.goto('/profile')
      await expect(ap.getByRole('tab', { name: 'Mes commandes' })).toHaveAttribute('aria-selected', 'true')
      const card = ap.getByRole('listitem').filter({ has: ap.getByRole('heading', { name: WINNER, exact: true }) }).first()
      await expect(card).toBeVisible()
      await expect(card).toContainText(euro(shares.Alice!))
      await expect(card).toContainText('Remboursé')
      await card.getByRole('button', { name: /^Mes plats/ }).click()
      await expect(card.getByRole('list', { name: 'Mes plats' })).toContainText(sc.alice.name)
      await expect(card).toContainText(euro(grand))
      const stats = ap.getByLabel('Mes statistiques')
      await expect(stats).toContainText(euro(shares.Alice!))
      await expect(stats).toContainText(WINNER)
      // La commande terminée n'apparaît plus dans le bandeau « Commande en cours ».
      await expect(ap.getByRole('link', { name: /^Reprendre la commande/ })).toHaveCount(0)
    })

    await test.step("6. Un non-membre qui ouvre la party reçoit une erreur propre", async () => {
      await register(dave)
      // La vue d'une party dont on n'est pas membre répond 404 (rules PocketBase) : attendu.
      dave.allow.push(/\[http 400\] POST .*\/api\/occ\/parties\/join$/, /status of 400 .*\/api\/occ\/parties\/join$/)
      dave.allow.push(/\[http 404\] GET .*\/api\/collections\/(parties|party_members)\//, /Failed to load resource: the server responded with a status of 404/)
      await dave.page.goto(partyPath)
      await expect(dave.page.getByRole('heading', { name: 'Commande introuvable' })).toBeVisible()
      await expect(dave.page.getByText("tu n'en fais pas partie")).toBeVisible()
      await expect(dave.page.getByRole('link', { name: "Retour à l'accueil" }).last()).toBeVisible()
      // Il peut toujours la rejoindre par code ? Non : la party est clôturée.
      await dave.page.goto(`/j/${code}`)
      await expect(dave.page.getByRole('heading', { name: 'Impossible de rejoindre' })).toBeVisible()
    })
  } finally {
    const report = actors.map((a) => ({ actor: a.name, problems: unexpectedProblems(a) }))
    for (const a of actors) await a.ctx.close()
    if (process.env.E2E_DEBUG) console.log(JSON.stringify(actors.map((a) => ({ actor: a.name, all: a.problems })), null, 2))
    const bad = report.filter((r) => r.problems.length)
    if (bad.length) console.log('Problèmes navigateur inattendus :\n' + JSON.stringify(bad, null, 2))
    expect.soft(bad, 'erreurs console / requêtes en échec inattendues').toEqual([])
  }
})
