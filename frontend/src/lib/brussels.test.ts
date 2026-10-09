import { describe, expect, it } from 'vitest'
import { brusselsTimeToISO, formatClock, isoInMinutesRounded } from './brussels'

describe('heures de Bruxelles', () => {
  it('formate à l’heure de Bruxelles (été / hiver)', () => {
    expect(formatClock('2026-10-09 09:45:00.000Z')).toBe('11:45')
    expect(formatClock('2026-01-09T10:45:00Z')).toBe('11:45')
    expect(formatClock('')).toBe('')
    expect(formatClock('n’importe quoi')).toBe('')
  })

  it('« à HH:MM » → prochaine occurrence', () => {
    const now = new Date('2026-10-09T09:30:00Z') // 11:30 à Bruxelles
    expect(brusselsTimeToISO('11:45', now)).toBe('2026-10-09T09:45:00.000Z')
    expect(brusselsTimeToISO('9:05', new Date('2026-01-09T07:00:00Z'))).toBe('2026-01-09T08:05:00.000Z')
    expect(brusselsTimeToISO('11:00', now)).toBeNull() // passée depuis peu
    expect(brusselsTimeToISO('08:00', new Date('2026-10-09T21:00:00Z'))).toBe('2026-10-10T06:00:00.000Z') // demain matin
    expect(brusselsTimeToISO('25:00', now)).toBeNull()
  })

  it('« +N min » arrondi à la minute suivante', () => {
    expect(isoInMinutesRounded(10, new Date('2026-10-09T09:30:20Z'))).toBe('2026-10-09T09:41:00.000Z')
    expect(isoInMinutesRounded(5, new Date('2026-10-09T09:30:00Z'))).toBe('2026-10-09T09:35:00.000Z')
  })
})
