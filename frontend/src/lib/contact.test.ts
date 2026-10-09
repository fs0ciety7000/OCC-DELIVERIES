import { describe, expect, it } from 'vitest'
import { formatPhone, mapHref, telHref } from './format'

describe('formatPhone', () => {
  it.each([
    ['+3265352964', '+32 65 35 29 64'],
    ['+32495466512', '+32 495 46 65 12'],
    ['+3221234567', '+32 2 123 45 67'],
    ['+3242223344', '+32 4 222 33 44'],
    ['+3280012345', '+32 800 12 345'],
    ['+33327123456', '+33327123456'],
    ['065 35 29 64', '065 35 29 64'],
    ['', ''],
  ])('%s → %s', (input, want) => {
    expect(formatPhone(input)).toBe(want)
  })
  it('tolère null / undefined', () => {
    expect(formatPhone(undefined)).toBe('')
  })
})

describe('telHref / mapHref', () => {
  it('compose un lien tel: sans séparateurs', () => {
    expect(telHref('065/35.29.64')).toBe('tel:065352964')
    expect(telHref('+3265352964')).toBe('tel:+3265352964')
  })
  it('pointe la position exacte, sinon cherche l’adresse', () => {
    expect(mapHref({ lat: 50.45, lng: 3.95 })).toBe('https://www.openstreetmap.org/?mlat=50.45&mlon=3.95#map=18/50.45/3.95')
    expect(mapHref({ lat: 50.45, lng: 3.95, geo_approx: true, address: 'Rue de Nimy 17, 7000 Mons' })).toBe(
      'https://www.openstreetmap.org/search?query=Rue%20de%20Nimy%2017%2C%207000%20Mons',
    )
  })
})
