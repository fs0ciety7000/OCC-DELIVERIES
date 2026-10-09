import { expect, test } from '@playwright/test'

/**
 * Recherche globale : « / » ouvre la palette, une saisie avec faute de casse/accents
 * trouve un plat, Entrée ouvre son restaurant défilé jusqu'au plat (?plat=), qui a le focus.
 * Le plat est choisi via l'API (catalogue réel ou démo).
 */
test('palette : « / », recherche d’un plat, ouverture ancrée sur le plat', async ({ page, request }) => {
  let dish: { id: string; name: string; restaurant: { id: string; name: string } } | undefined
  let query = ''
  for (const q of ['margherita', 'pizza', 'burger', 'poulet', 'frites']) {
    const res = await request.get(`/api/occ/search?q=${encodeURIComponent(q)}&limit=1`)
    expect(res.ok()).toBeTruthy()
    const body = (await res.json()) as { dishes: NonNullable<typeof dish>[] }
    if (body.dishes.length) {
      dish = body.dishes[0]
      query = q
      break
    }
  }
  test.skip(!dish, 'aucun plat dans le catalogue')
  if (!dish) return

  await page.goto('/restaurants')
  await page.locator('body').click({ position: { x: 5, y: 5 } })
  await page.keyboard.press('/')
  const dialog = page.getByRole('dialog', { name: 'Recherche' })
  await expect(dialog).toBeVisible()
  const input = dialog.getByRole('combobox')
  await expect(input).toBeFocused()

  // majuscules : le serveur plie casse et accents
  await input.fill(query.toUpperCase())
  const option = dialog.getByRole('group', { name: 'Plats' }).getByRole('option').first()
  await expect(option).toContainText(dish.name)
  await expect(option.locator('mark').first()).toBeVisible()

  // ↓ jusqu'au premier plat puis Entrée
  const options = dialog.getByRole('option')
  const count = await options.count()
  for (let i = 0; i < count; i++) {
    if ((await options.nth(i).getAttribute('aria-selected')) === 'true' && (await options.nth(i).textContent())?.includes(dish.name)) break
    await page.keyboard.press('ArrowDown')
  }
  await page.keyboard.press('Enter')

  await expect(page).toHaveURL(new RegExp(`/restaurants/${dish.restaurant.id}\\?plat=${dish.id}$`))
  await expect(dialog).toBeHidden()
  const target = page.locator(`[data-dish="${dish.id}"]`).last()
  await expect(target).toBeInViewport()
  await expect(target.getByRole('button')).toBeFocused()

  // Ctrl K rouvre, Échap ferme ; la recherche est mémorisée
  await page.keyboard.press('Control+k')
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('group', { name: /recherches récentes/i })).toContainText(query.toUpperCase())
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
})

test('mobile : plein écran depuis l’icône de l’en-tête', async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 375, height: 740 }, hasTouch: true, isMobile: true })
  const page = await ctx.newPage()
  await page.goto('/')
  await page.getByRole('button', { name: /^Rechercher/ }).click()
  const dialog = page.getByRole('dialog', { name: 'Recherche' })
  await expect(dialog).toBeVisible()
  const box = await dialog.boundingBox()
  expect(box?.width).toBe(375)
  await expect(dialog.getByRole('group', { name: 'Suggestions' })).toBeVisible()
  await dialog.getByRole('button', { name: 'Fermer la recherche' }).first().click()
  await expect(dialog).toBeHidden()
  await ctx.close()
})
