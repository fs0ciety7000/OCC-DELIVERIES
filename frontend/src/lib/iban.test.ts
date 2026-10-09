import { describe, expect, it } from 'vitest'
import { formatIban, isValidBic, isValidIban, normalizeIban } from './iban'
import { isValidMobile, isValidWeroId, normalizeMobile, normalizeWeroId } from './wallet'

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

describe('Wero / Bancontact', () => {
  it('normalise les mobiles belges', () => {
    expect(normalizeMobile('0470 12 34 56')).toBe('+32470123456')
    expect(normalizeMobile('0032470123456')).toBe('+32470123456')
    expect(isValidMobile('0470/12.34.56')).toBe(true)
    expect(isValidMobile('12')).toBe(false)
  })
  it('accepte mobile ou e-mail pour Wero', () => {
    expect(normalizeWeroId('Alice@Mail.BE')).toBe('alice@mail.be')
    expect(isValidWeroId('alice@mail.be')).toBe(true)
    expect(isValidWeroId('0470123456')).toBe(true)
    expect(isValidWeroId('alice@')).toBe(false)
    expect(isValidWeroId('')).toBe(true)
  })
})
