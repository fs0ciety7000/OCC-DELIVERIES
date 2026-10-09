import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearRecent, MAX_RECENT, pushRecent, readRecent, removeRecent } from './recent'

describe('recherches récentes', () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => vi.restoreAllMocks())

  it('ajoute en tête, sans doublon (casse et accents ignorés)', () => {
    pushRecent('pizza')
    pushRecent('Râmen')
    expect(pushRecent('  ramen ')).toEqual(['ramen', 'pizza'])
    expect(readRecent()).toEqual(['ramen', 'pizza'])
  })

  it(`garde les ${MAX_RECENT} dernières et ignore les saisies trop courtes`, () => {
    for (let i = 0; i < 10; i++) pushRecent(`plat ${i}`)
    expect(readRecent()).toHaveLength(MAX_RECENT)
    expect(readRecent()[0]).toBe('plat 9')
    expect(pushRecent('x')).toHaveLength(MAX_RECENT)
  })

  it('retire et efface', () => {
    pushRecent('sushi')
    pushRecent('burger')
    expect(removeRecent('sushi')).toEqual(['burger'])
    expect(clearRecent()).toEqual([])
    expect(readRecent()).toEqual([])
  })

  it('résiste à un stockage illisible ou bloqué', () => {
    localStorage.setItem('occ:search:recent', '{pas du json')
    expect(readRecent()).toEqual([])
    localStorage.setItem('occ:search:recent', JSON.stringify(['ok', 3, '', null]))
    expect(readRecent()).toEqual(['ok'])
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })
    expect(readRecent()).toEqual([])
    expect(() => pushRecent('pizza')).not.toThrow()
  })
})
