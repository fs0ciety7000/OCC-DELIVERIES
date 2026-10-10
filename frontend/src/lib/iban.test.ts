import { describe, expect, it } from 'vitest'
import { formatIban, isValidBic, isValidIban, normalizeIban } from './iban'

describe('IBAN', () => {
  it('valide mod-97', () => {
    expect(isValidIban('BE71 0961 2345 6769')).toBe(true)
    expect(isValidIban('FR14 2004 1010 0505 0001 3M02 606')).toBe(true)
    expect(isValidIban('BE71 0961 2345 6768')).toBe(false)
    expect(isValidIban('pas un iban')).toBe(false)
  })
  it('normalise et formate', () => {
    expect(normalizeIban('be71 0961-2345 6769')).toBe('BE71096123456769')
    expect(formatIban('BE71096123456769')).toBe('BE71 0961 2345 6769')
  })
  it('BIC', () => {
    expect(isValidBic('GKCCBEBB')).toBe(true)
    expect(isValidBic('GEBABEBB36A')).toBe(true)
    expect(isValidBic('XX')).toBe(false)
  })
})

