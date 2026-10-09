import { describe, expect, it } from 'vitest'
import { AVATAR_COLORS, AVATAR_FALLBACK_PALETTE, avatarColor, distinctColors, fallbackColor, hashString, isHexColor, readableOn, spreadHash } from './colors'

const NAMES = ['Alice', 'Bob', 'Chloé', 'Dan', 'Emma', 'Lucas', 'Léa', 'Hugo', 'Nina', 'Tom']

describe('palette', () => {
  it('ne contient que des couleurs #RRGGBB, sans doublon, sélecteur en tête', () => {
    expect(AVATAR_FALLBACK_PALETTE.every((c) => isHexColor(c))).toBe(true)
    expect(new Set(AVATAR_FALLBACK_PALETTE.map((c) => c.toUpperCase())).size).toBe(AVATAR_FALLBACK_PALETTE.length)
    expect(AVATAR_FALLBACK_PALETTE.slice(0, AVATAR_COLORS.length)).toEqual([...AVATAR_COLORS])
  })
})

describe('hachage', () => {
  it('hashString reste stable (vignettes de restaurants)', () => {
    expect(hashString('')).toBe(0)
    expect(hashString('ab')).toBe(97 * 31 + 98)
  })

  it('spreadHash est déterministe et répartit des graines voisines', () => {
    expect(spreadHash('bob')).toBe(spreadHash('bob'))
    const seeds = Array.from({ length: 2200 }, (_, i) => `u${i.toString(36).padStart(14, '0')}`)
    const counts = new Map<string, number>()
    for (const s of seeds) counts.set(fallbackColor(s), (counts.get(fallbackColor(s)) ?? 0) + 1)
    // toutes les couleurs servent, aucune ne domine (attendu ≈ 100 chacune)
    expect(counts.size).toBe(AVATAR_FALLBACK_PALETTE.length)
    for (const n of counts.values()) expect(n).toBeGreaterThan(50)
  })

  it('des prénoms courants reçoivent des couleurs de repli variées', () => {
    const colors = NAMES.map((n) => fallbackColor(n))
    expect(new Set(colors).size).toBeGreaterThanOrEqual(8)
    expect(fallbackColor('Bob')).not.toBe(fallbackColor('Chloé'))
  })
})

describe('avatarColor', () => {
  it('garde la couleur choisie, sinon un repli stable', () => {
    expect(avatarColor({ id: 'u1', color: '#123456' })).toBe('#123456')
    expect(avatarColor({ id: 'u1', color: '' })).toBe(fallbackColor('u1'))
    expect(avatarColor({ id: 'u1', color: 'rouge' })).toBe(fallbackColor('u1'))
  })
})

describe('distinctColors', () => {
  it('deux voisins de même couleur (Bob et Chloé) sont différenciés, le premier garde la sienne', () => {
    const m = distinctColors([
      { id: 'alice', color: '#FF6A3D' },
      { id: 'bob', color: '#2A9D8F' },
      { id: 'chloe', color: '#2a9d8f' },
    ])
    expect(m.get('alice')).toBe('#FF6A3D')
    expect(m.get('bob')).toBe('#2A9D8F')
    expect(m.get('chloe')?.toUpperCase()).not.toBe('#2A9D8F')
    expect(new Set([...m.values()].map((c) => c.toUpperCase())).size).toBe(3)
  })

  it('est déterministe et sans doublon tant que la palette suffit', () => {
    const group = Array.from({ length: AVATAR_FALLBACK_PALETTE.length }, (_, i) => ({ id: `u${i}`, color: '#FF6A3D' }))
    const a = distinctColors(group)
    expect([...a.entries()]).toEqual([...distinctColors(group).entries()])
    expect(new Set(a.values()).size).toBe(group.length)
  })

  it('au-delà de la palette : une couleur pour chacun, doublons tolérés', () => {
    const group = Array.from({ length: AVATAR_FALLBACK_PALETTE.length + 3 }, (_, i) => ({ id: `u${i}` }))
    const m = distinctColors(group)
    expect(m.size).toBe(group.length)
    expect([...m.values()].every((c) => isHexColor(c))).toBe(true)
  })
})

describe('readableOn', () => {
  it('choisit un texte lisible', () => {
    expect(readableOn('#FFE500')).toBe('ink')
    expect(readableOn('#29335C')).toBe('paper')
  })
})
