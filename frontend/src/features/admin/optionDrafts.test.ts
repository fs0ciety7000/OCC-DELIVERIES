import { describe, expect, it } from 'vitest'
import { emptyGroup, fromDrafts, toDrafts } from './optionDrafts'

describe('option groups drafts', () => {
  it('aller-retour avec prix en euros', () => {
    const groups = [{ id: 'size', name: 'Taille', min: 1, max: 1, choices: [{ id: 'm', name: 'Moyenne', price: 0 }, { id: 'l', name: 'Large', price: 300 }] }]
    const drafts = toDrafts(groups)
    expect(drafts[0]!.choices[1]!.price).toBe('3,00')
    expect(fromDrafts(drafts)).toEqual({ groups, errors: [] })
  })

  it('génère les identifiants et convertit les décimales françaises', () => {
    const g = emptyGroup()
    g.name = 'Suppléments'
    g.max = '3'
    g.choices = [
      { key: 'a', id: '', name: 'Fromage râpé', price: '1,5' },
      { key: 'b', id: '', name: 'Fromage râpé', price: '0,80 €' },
      { key: 'c', id: '', name: '', price: '' },
    ]
    const { groups, errors } = fromDrafts([g])
    expect(errors).toEqual([])
    expect(groups[0]).toMatchObject({ id: 'supplements', min: 0, max: 3 })
    expect(groups[0]!.choices).toEqual([
      { id: 'fromage-rape', name: 'Fromage râpé', price: 150 },
      { id: 'fromage-rape-2', name: 'Fromage râpé', price: 80 },
    ])
  })

  it('signale les erreurs', () => {
    const g = emptyGroup()
    g.name = 'Sauce'
    g.min = '2'
    g.max = '1'
    g.choices = [{ key: 'a', id: '', name: 'Mayo', price: 'gratuit' }]
    const { errors } = fromDrafts([g])
    expect(errors.join(' ')).toMatch(/minimum dépasse/)
    expect(errors.join(' ')).toMatch(/supplément invalide/)
    expect(errors.join(' ')).toMatch(/plus de choix/)
  })
})
