import { describe, expect, it } from 'vitest'
import { formatCountdown, formatDistance, formatEta, formatMoney, formatRelativeTime, initials, parseDate, parseMoneyToCents, plural } from './format'

const norm = (s: string) => s.replace(/[\u00a0\u202f]/g, ' ')

describe('formatMoney', () => {
  it('formate des centimes en euros fr-BE', () => {
    expect(norm(formatMoney(1250))).toBe('12,50 €')
    expect(norm(formatMoney(0))).toBe('0,00 €')
    expect(norm(formatMoney(123456))).toMatch(/^1.?234,56 €$/)
  })
  it('tolère null/undefined', () => {
    expect(norm(formatMoney(undefined))).toBe('0,00 €')
  })
})

describe('parseMoneyToCents', () => {
  it.each([
    ['2,50', 250],
    ['2.5', 250],
    ['3 €', 300],
    ['', 0],
    ['0,99', 99],
  ])('%s → %i', (input, cents) => expect(parseMoneyToCents(input)).toBe(cents))
  it('refuse les saisies invalides', () => {
    expect(parseMoneyToCents('abc')).toBeNull()
    expect(parseMoneyToCents('1,234')).toBeNull()
    expect(parseMoneyToCents('-2')).toBeNull()
  })
})

describe('formatDistance', () => {
  it('mètres sous 1 km, km au-delà', () => {
    expect(formatDistance(0.347)).toBe('350 m')
    expect(formatDistance(1.24)).toBe('1,2 km')
    expect(formatDistance(undefined)).toBe('')
  })
})

describe('formatRelativeTime', () => {
  const now = new Date('2026-10-09T12:00:00Z')
  it('gère passé et futur', () => {
    expect(formatRelativeTime(new Date('2026-10-09T11:59:40Z'), now)).toBe("à l'instant")
    expect(formatRelativeTime(new Date('2026-10-09T11:55:00Z'), now)).toBe('il y a 5 minutes')
    expect(formatRelativeTime(new Date('2026-10-09T14:00:00Z'), now)).toBe('dans 2 heures')
  })
  it('accepte le format de date PocketBase', () => {
    expect(parseDate('2026-10-09 11:00:00.000Z')?.toISOString()).toBe('2026-10-09T11:00:00.000Z')
    expect(formatRelativeTime('2026-10-08 12:00:00.000Z', now)).toBe('hier')
  })
})

describe('divers', () => {
  it('countdown', () => {
    expect(formatCountdown(65_000)).toBe('01:05')
    expect(formatCountdown(-5)).toBe('00:00')
    expect(formatCountdown(3_725_000)).toBe('1:02:05')
  })
  it('eta, initiales, pluriel', () => {
    expect(formatEta(25, 35)).toBe('25–35 min')
    expect(formatEta(20, 20)).toBe('20 min')
    expect(initials('alice martin')).toBe('AM')
    expect(initials('')).toBe('?')
    expect(plural(1, 'article')).toBe('1 article')
    expect(plural(3, 'article')).toBe('3 articles')
  })
})
