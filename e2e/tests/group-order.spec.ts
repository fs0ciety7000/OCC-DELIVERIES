import { readFile } from 'node:fs/promises'
import { expect, test, type Browser, type BrowserContext, type BrowserContextOptions, type Locator, type Page } from '@playwright/test'

/**
 * Scénario de référence OCC DELIVERIES piloté par l'UI réelle :
 * Alice (hôte, desktop) + Bob et Chloé (invités, mobile 390×844), puis Dave (non-membre).
 */

const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
const PASSWORD = 'e2e-pass-1234'

const CANDIDATES = ['La Bella Nonna', 'Friterie du Beffroi', 'Les Jardins du Cèdre'] as const
const WINNER = 'La Bella Nonna'
const DELIVERY_FEE = 299

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

/** Bruit réseau légitime : flux SSE coupé à la navigation / fermeture, requêtes annulées par TanStack. */
const GLOBAL_ALLOW: RegExp[] = [/^\[requestfailed\] GET \S+\/api\/realtime net::ERR_ABORTED$/]

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
  await page.getByRole('button', { name: new RegExp(`^${item}`) }).first().click()
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

test('commande groupée complète : vote → paniers → récap → dispatch → remboursements', async ({ browser, baseURL }) => {
  const base = baseURL!
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

      // Bob : lien d'invitation, non connecté → login → inscription → retour sur /j/:code → salle.
      await bob.page.goto(`/j/${code}`)
      await bob.page.waitForURL(/\/login\?next=/)
      await bob.page.getByRole('link', { name: 'Crée-le en 20 secondes' }).click()
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
      await ap.getByRole('button', { name: 'Lancer le vote (3)' }).click()
      for (const a of members) await expect(a.page.getByRole('heading', { name: 'Vote pour tes restos préférés' })).toBeVisible()

      const meter = (p: Page, n: number, r: string) => p.getByRole('meter', { name: `${n} vote(s) pour ${r}` })

      await bob.page.getByRole('button', { name: `Voter pour ${WINNER}` }).click()
      await bob.page.getByRole('button', { name: 'Voter pour Friterie du Beffroi' }).click()
      // Visible chez Alice et Chloé sans rechargement.
      for (const a of [alice, chloe]) {
        await expect(meter(a.page, 1, WINNER)).toBeVisible()
        await expect(meter(a.page, 1, 'Friterie du Beffroi')).toBeVisible()
      }
      await chloe.page.getByRole('button', { name: `Voter pour ${WINNER}` }).click()
      await chloe.page.getByRole('button', { name: 'Voter pour Les Jardins du Cèdre' }).click()
      await chloe.page.getByRole('button', { name: 'Retirer mon vote pour Les Jardins du Cèdre' }).click()
      await expect(meter(alice.page, 2, WINNER)).toBeVisible()
      await expect(meter(bob.page, 2, WINNER)).toBeVisible()
      await expect(meter(bob.page, 0, 'Les Jardins du Cèdre')).toBeVisible()

      await ap.getByRole('button', { name: `Voter pour ${WINNER}` }).click()
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
      Alice: 1050 + 300 + 150, // Margherita Large + Mozzarella
      Bob: (1350 + 100) * 2, // 2 × Regina Moyenne + Champignons
      Chloé: 300 + 100 + 650, // San Pellegrino 50 cl + Tiramisu
    }

    await test.step('3. Paniers avec options + note, prêts visibles en direct', async () => {
      await addItem(alice.page, 'Margherita', { requiredGroup: 'Taille', pick: ['Large (34 cm)', 'Mozzarella'], note: 'Sans basilic svp', expectedLine: 1500 })
      await addItem(bob.page, 'Regina', { requiredGroup: 'Taille', pick: ['Champignons'], quantity: 2, note: 'Bien cuite', expectedLine: 2900 })
      await addItem(chloe.page, 'San Pellegrino', { requiredGroup: 'Format', pick: ['50 cl'], note: 'Bien fraîche', expectedLine: 400 })
      await addItem(chloe.page, 'Tiramisu maison', { note: 'Deux cuillères', expectedLine: 650 })

      // Prix serveur : le panier (sous-total) reprend les totaux calculés côté Go.
      for (const a of members) {
        const cartBtn = a.page.getByRole('button', { name: /^Mon panier/ })
        await expect(cartBtn).toHaveText(euro(expected[a.name as keyof typeof expected]))
      }
      await chloe.page.getByRole('button', { name: /^Mon panier/ }).click()
      const cart = dialog(chloe.page, 'Mon panier')
      await expect(cart).toContainText('50 cl')
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
        byName[who === 'Toi' ? 'Alice' : who] = { total: cents((await s.locator('span.font-display').textContent())!), fee: cents(feeText!) }
      }
      const grandTotal = cents((await ap.locator('dl > div').filter({ has: ap.locator('dt', { hasText: /^Total$/ }) }).locator('dd').textContent())!)
      const itemsSubtotal = expected.Alice + expected.Bob + expected.Chloé
      expect(grandTotal).toBe(itemsSubtotal + DELIVERY_FEE)
      expect(Object.keys(byName).sort()).toEqual(['Alice', 'Bob', 'Chloé'])
      expect(Object.values(byName).reduce((x, y) => x + y.total, 0), 'Σ parts = total général').toBe(grandTotal)
      // Frais de livraison partagés à parts égales, au centime près (plus grand reste) : 100 / 100 / 99.
      expect(Object.values(byName).map((v) => v.fee).sort()).toEqual([100, 100, 99].sort())
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
      for (const word of ['Margherita', 'Regina', 'San Pellegrino', 'Tiramisu']) expect(recap).toContain(word)
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
      for (const word of ['Alice', 'Bob', 'Chloé', 'Margherita', 'Regina', 'Tiramisu', 'Sans basilic svp']) expect(csv).toContain(word)
      await expect(ap.getByText('Export CSV téléchargé').first()).toBeVisible()
      await ex.getByRole('button', { name: 'Fermer' }).click()
      await expect(ex).toBeHidden()
    })

    await shot(members, '4-review')
    await test.step('5. Bob renseigne son profil, devient payeur ; Wero + espèces → clôture', async () => {
      const bp = bob.page
      await bp.goto('/profile')
      await bp.getByLabel('Mobile ou e-mail Wero').fill('+32470123456')
      await bp.getByLabel('Titulaire du compte').fill('Bob Martin')
      await bp.getByLabel('IBAN').fill('BE71096123456769')
      await expect(bp.getByText('IBAN valide ✓')).toBeVisible()
      await bp.getByRole('button', { name: 'Enregistrer', exact: true }).last().click()
      await expect(bp.getByText('Coordonnées de remboursement enregistrées').first()).toBeVisible()
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
        await expect(p.getByText('Bob Martin').first()).toBeVisible()
        const tiles = p.getByRole('radiogroup', { name: 'Moyen de remboursement' })
        const wero = tiles.getByRole('radio', { name: /Wero/ })
        await expect(wero).toBeVisible()
        await wero.click()
        await expect(p.getByText('+32470123456')).toBeVisible()
        await tiles.getByRole('radio', { name: /Virement QR/ }).click()
        await expect(p.getByRole('img', { name: /QR virement SEPA de .* vers Bob Martin/ })).toBeVisible()
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
