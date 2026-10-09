import { describe, expect, it } from 'vitest'
import { defaultSelections, linePrice, optionsLabel, toggleChoice, toSelectedOptions, unitPrice, validateSelections } from './price'
import type { OptionGroup } from './types'

const size: OptionGroup = {
  id: 'size',
  name: 'Taille',
  min: 1,
  max: 1,
  choices: [
    { id: 'm', name: 'Moyenne', price: 0 },
    { id: 'l', name: 'Large', price: 300 },
  ],
}
const extras: OptionGroup = {
  id: 'extras',
  name: 'Suppléments',
  min: 0,
  max: 2,
  choices: [
    { id: 'cheese', name: 'Fromage', price: 150 },
    { id: 'ham', name: 'Jambon', price: 200 },
    { id: 'egg', name: 'Œuf', price: 100 },
  ],
}
const item = { price: 1250, option_groups: [size, extras] }

describe('aperçu de prix', () => {
  it('pré-sélectionne le premier choix des groupes radio obligatoires', () => {
    expect(defaultSelections(item)).toEqual({ size: ['m'], extras: [] })
  })

  it('additionne base + options × quantité', () => {
    let sel = defaultSelections(item)
    sel = toggleChoice(sel, size, 'l')
    sel = toggleChoice(sel, extras, 'cheese')
    expect(unitPrice(item, sel)).toBe(1250 + 300 + 150)
    expect(linePrice(item, sel, 3)).toBe(1700 * 3)
    expect(optionsLabel(item, sel)).toBe('Large, Fromage')
  })

  it('respecte max (cases à cocher) et radio', () => {
    let sel = defaultSelections(item)
    sel = toggleChoice(sel, extras, 'cheese')
    sel = toggleChoice(sel, extras, 'ham')
    sel = toggleChoice(sel, extras, 'egg') // ignoré : max 2
    expect(sel.extras).toEqual(['cheese', 'ham'])
    sel = toggleChoice(sel, size, 'm') // radio obligatoire : ne se vide pas
    expect(sel.size).toEqual(['m'])
  })

  it('valide min/max', () => {
    expect(validateSelections(item, { size: [], extras: [] })).toHaveLength(1)
    expect(validateSelections(item, { size: ['m'], extras: ['cheese', 'ham', 'egg'] })[0]?.group).toBe('extras')
    expect(validateSelections(item, { size: ['l'], extras: [] })).toEqual([])
  })

  it('sérialise selected_options sans groupes vides ni ids inconnus', () => {
    expect(toSelectedOptions(item, { size: ['l'], extras: ['bogus'] })).toEqual([{ group: 'size', choices: ['l'] }])
  })

  it('gère option_groups null', () => {
    expect(unitPrice({ price: 500, option_groups: null }, {})).toBe(500)
  })
})
