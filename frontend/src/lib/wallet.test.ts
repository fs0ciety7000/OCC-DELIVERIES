import { describe, expect, it } from 'vitest'
import { normalizePaymentLink, normalizePayPalMe, normalizeRevolutTag } from './wallet'

describe('liens de paiement', () => {
  it.each([
    ['', ''],
    ['@Bob', 'bob'],
    ['revolut.me/bob', 'bob'],
    ['https://www.revolut.me/@bob/eur5', 'bob'],
    ['https://paypal.me/bob', null],
    ['bob smith', null],
  ])('revtag %s → %s', (input, want) => expect(normalizeRevolutTag(input)).toBe(want))

  it.each([
    ['', ''],
    ['jdoe', 'jdoe'],
    ['paypal.me/jdoe/12EUR', 'jdoe'],
    ['https://www.paypal.com/paypalme/jdoe', 'jdoe'],
    ['https://revolut.me/jdoe', null],
  ])('paypal %s → %s', (input, want) => expect(normalizePayPalMe(input)).toBe(want))

  it.each([
    ['', ''],
    ['wise.com/pay/business/acme', 'https://wise.com/pay/business/acme'],
    ['javascript:alert(1)', null],
    ['pas un lien', null],
  ])('lien %s → %s', (input, want) => expect(normalizePaymentLink(input)).toBe(want))
})
