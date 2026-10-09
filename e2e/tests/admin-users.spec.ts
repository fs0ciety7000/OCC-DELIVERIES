import { expect, test, type Page } from '@playwright/test'

/**
 * Administration des comptes : un·e admin suspend puis réactive un·e collègue.
 * Nécessite un serveur démarré avec OCC_ADMINS=<E2E_ADMIN_EMAIL> (le compte naît admin) :
 *   OCC_ADMINS=admin@e2e.test … go run . serve
 *   E2E_ADMIN_EMAIL=admin@e2e.test npx playwright test
 */

const ADMIN = process.env.E2E_ADMIN_EMAIL
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
const PASSWORD = 'e2e-pass-1234'

async function register(page: Page, name: string, email: string) {
  await page.goto('/register')
  await page.getByLabel('Prénom (ou surnom)').fill(name)
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Mot de passe').fill(PASSWORD)
  await page.getByRole('button', { name: 'Créer mon compte' }).click()
  await expect(page.getByText('Bienvenue à bord').first()).toBeVisible()
}

async function signIn(page: Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Mot de passe').fill(PASSWORD)
  await page.getByRole('button', { name: 'Se connecter' }).click()
}

test('un·e admin suspend puis réactive un compte', async ({ browser, baseURL }) => {
  test.skip(!ADMIN, 'E2E_ADMIN_EMAIL non défini (serveur sans OCC_ADMINS dédié)')
  const adminCtx = await browser.newContext({ baseURL, locale: 'fr-BE' })
  const userCtx = await browser.newContext({ baseURL, locale: 'fr-BE', viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })
  const admin = await adminCtx.newPage()
  const victim = await userCtx.newPage()
  const victimEmail = `eve.${RUN}@e2e.test`

  // le compte admin (créé au premier passage, sinon connexion)
  await signIn(admin, ADMIN!)
  if (!(await admin.getByText('Content de te revoir').first().isVisible({ timeout: 5_000 }).catch(() => false))) {
    await register(admin, 'Admin E2E', ADMIN!)
  }
  await register(victim, 'Eve', victimEmail)

  // suspension depuis /admin/utilisateurs
  await admin.goto('/admin/utilisateurs')
  await admin.getByLabel('Rechercher un utilisateur').fill(victimEmail)
  await admin.getByRole('button', { name: 'Actions pour Eve' }).click()
  await admin.getByRole('dialog').getByRole('button', { name: /Suspendre le compte/ }).click()
  const confirm = admin.getByRole('dialog', { name: 'Suspendre Eve ?' })
  await confirm.getByLabel(/Motif/).fill('test e2e')
  await confirm.getByRole('button', { name: 'Suspendre' }).click()
  await expect(admin.getByText('Eve est suspendu·e et déconnecté·e')).toBeVisible()
  await expect(admin.getByText('Suspendu', { exact: true })).toBeVisible()

  // la session d'Eve est coupée, et la reconnexion refusée avec un message clair
  await victim.goto('/profile')
  await expect(victim).toHaveURL(/\/login/)
  await signIn(victim, victimEmail)
  await expect(victim.getByText(/Compte suspendu/)).toBeVisible()
  await expect(victim.getByRole('link', { name: 'Mot de passe oublié ?' })).toBeVisible()

  // réactivation : Eve se reconnecte
  await admin.getByRole('button', { name: 'Actions pour Eve' }).click()
  await admin.getByRole('dialog').getByRole('button', { name: /Réactiver le compte/ }).click()
  await expect(admin.getByText('Eve est réactivé·e')).toBeVisible()
  await signIn(victim, victimEmail)
  await expect(victim.getByText('Content de te revoir').first()).toBeVisible()

  // sans SMTP : état affiché, pas de bouton d'e-mail de test (masqué)
  await expect(admin.getByText('Désactivés')).toBeVisible()
  await expect(admin.getByRole('button', { name: 'Envoyer un e-mail de test' })).toHaveCount(0)

  await adminCtx.close()
  await userCtx.close()
})
